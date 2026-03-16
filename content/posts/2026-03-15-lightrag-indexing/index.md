+++
title = "LightRAG 源码阅读 - Indexing"
summary = ""
description = ""
categories = [""]
tags = ["RAG", "AI", "LLM"]
date = 2026-03-15T09:22:03+09:00
draft = false

+++

代码基于 commit-sha `e675598db402da0241f089a22f6939dfbe223437`


## 文件上传 & 内容提取

```Python
# lightrag/api/routers/document_routes.py:2076
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/api/routers/document_routes.py?plain=1#L2076

@router.post(
    "/upload", response_model=InsertResponse, dependencies=[Depends(combined_auth)]
)
async def upload_to_input_dir(
    background_tasks: BackgroundTasks, file: UploadFile = File(...)
):
```

一般的 Web Server 服务器端上传的流程，然后触发一个解析任务

- 文件名清洗 `sanitize_filename`
- 扩展名校验 `doc_manager.is_supported_file`
- 可选的文件大小检查
- 检查是否存在重复文件，不存在则写入文件到指定路径
- 触发任务 `background_tasks.add_task(pipeline_index_file, rag, file_path, track_id)`，并分配一个 `track_id` 来跟踪任务状态

这里的 `background_tasks` 是直接使用的 FastAPI 的 `BackgroundTasks`，运行在 Worker 进程内的，并不是十分可靠

前端拿到 `track_id` 之后会开始轮询我们的任务状态


```Python
# lightrag/api/routers/document_routes.py:1651
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/api/routers/document_routes.py?plain=1#L1651

async def pipeline_index_file(rag: LightRAG, file_path: Path, track_id: str = None):
    try:
        success, returned_track_id = await pipeline_enqueue_file(
            rag, file_path, track_id
        )
        if success:
            await rag.apipeline_process_enqueue_documents()

    except Exception as e:
        logger.error(f"Error indexing file {file_path.name}: {str(e)}")
        logger.error(traceback.format_exc())
```

其中，`pipeline_enqueue_file` 会根据文件的扩展名走一个大的 `match` 逻辑进行分发
比如扩展名是 `pdf` 的时候会根据 `DOCLING` 是否可用作为条件，对于 `pdf` 文件
优先选用 `_convert_with_docling` 提取文本。如果不行，则使用 `_extract_pdf_pypdf` 进行 pdf 文件的文本提取。

- https://github.com/docling-project/docling
- https://github.com/py-pdf/pypdf

提取到文本之后会调用 `apipeline_enqueue_documents`

```Python
# lightrag/lightrag.py:1287
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/lightrag.py?plain=1#L1287

async def apipeline_enqueue_documents(
    self,
    input: str | list[str],
    ids: list[str] | None = None,
    file_paths: str | list[str] | None = None,
    track_id: str | None = None,
) -> str:
```

核心逻辑，将处理对象从 File 转换为 Document

- 内容清洗: `sanitize_text_for_encoding`
- 生成 `doc_id`：`compute_mdhash_id(content, prefix="doc-")`
- 内容去重：相同 content 的 MD5 hash 相同则去重
- 构建 `new_docs` 数据结构，初始状态 `DocStatus.PENDING`
- 重复文档写入 `dup-` 前缀记录，状态置为 `FAILED`，metadata 里保存 `is_duplicate`/`original_doc_id`
- 持久化，存储完整文档内容到 `full_docs` KV，存储文档状态到 `doc_status``


## 文档处理

文档内容提取完成并入库之后，会调用 `apipeline_process_enqueue_documents` 来进行文档处理

```Python
# lightrag/lightrag.py:1672
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/lightrag.py?plain=1#L1672

async def apipeline_process_enqueue_documents(
    self,
    split_by_character: str | None = None,
    split_by_character_only: bool = False,
) -> None:
    """
    Process pending documents by splitting them into chunks, processing
    each chunk for entity and relation extraction, and updating the
    document status.

    1. Get all pending, failed, and abnormally terminated processing documents.
    2. Validate document data consistency and fix any issues
    3. Split document content into chunks
    4. Process each chunk for entity and relation extraction
    5. Update the document status
    """
```

这个函数取待处理文档的逻辑是取全部的，而不仅仅是我们刚才上传的那个，所以它又加了一个 Lock

```Python
# lightrag/lightrag.py:1701
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/lightrag.py?plain=1#L1701

# Check if another process is already processing the queue
async with pipeline_status_lock:
    # Ensure only one worker is processing documents
    if not pipeline_status.get("busy", False):
        processing_docs, failed_docs, pending_docs = await asyncio.gather(
            self.doc_status.get_docs_by_status(DocStatus.PROCESSING),
            self.doc_status.get_docs_by_status(DocStatus.FAILED),
            self.doc_status.get_docs_by_status(DocStatus.PENDING),
        )
```

怀疑是为了做容错，如果有文档处理中的时候 Worker 被 kill 了，下一个上传任务会将这个文档也进行处理。
但可能通过 DB 做队列，然后使用 Consumer 进程的方式会更好一点

为了防止同一个 Worker 内处理的任务太多，这里又加了一个 `Semaphore`，限制同时处理的任务数量

```Python
# lightrag/lightrag.py:1805
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/lightrag.py?plain=1#L1805

# Create a counter to track the number of processed files
processed_count = 0
# Create a semaphore to limit the number of concurrent file processing
semaphore = asyncio.Semaphore(self.max_parallel_insert)

```

核心的文档处理是里面有一个嵌套函数 `process_document`

### Chunking

第一步，我们进行文档的 chunking 操作

```Python
# lightrag/lightrag.py:1877
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/lightrag.py?plain=1#L1877

# Get document content from full_docs
content_data = await self.full_docs.get_by_id(doc_id)
if not content_data:
    raise Exception(
        f"Document content not found in full_docs for doc_id: {doc_id}"
    )
content = content_data["content"]

# Call chunking function, supporting both sync and async implementations
chunking_result = self.chunking_func(
    self.tokenizer,
    content,
    split_by_character,
    split_by_character_only,
    self.chunk_overlap_token_size,
    self.chunk_token_size,
)

# If result is awaitable, await to get actual result
if inspect.isawaitable(chunking_result):
    chunking_result = await chunking_result

# Validate return type
if not isinstance(chunking_result, (list, tuple)):
    raise TypeError(
        f"chunking_func must return a list or tuple of dicts, "
        f"got {type(chunking_result)}"
    )

# Build chunks dictionary
chunks: dict[str, Any] = {
    compute_mdhash_id(dp["content"], prefix="chunk-"): {
        **dp,
        "full_doc_id": doc_id,
        "file_path": file_path,  # Add file path to each chunk
        "llm_cache_list": [],  # Initialize empty LLM cache list for each chunk
    }
    for dp in chunking_result
}
```

- 从 `full_docs` KV 读取我们上一步存储的数据
- 调用 `chunking_func(tokenizer, content, ...)` 
- `chunk_id` = `compute_mdhash_id(content, prefix="chunk-")`
- 并发更新存储数据: `doc_status_task` + `chunks_vdb.upsert(chunks)` + `text_chunks.upsert(chunks)`

这里 `self.chunking_func` 默认是使用 `chunking_by_token_size`

```Python
# lightrag/operate.py:99
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/operate.py?plain=1#L99
def chunking_by_token_size(
    tokenizer: Tokenizer,
    content: str,
    split_by_character: str | None = None,
    split_by_character_only: bool = False,
    chunk_overlap_token_size: int = 100,
    chunk_token_size: int = 1200,
) -> list[dict[str, Any]]:
```

这里的 tokenizer 默认是使用 gpt-4o-mini。借助开源库 https://github.com/openai/tiktoken 来实现。
在 chunking 的过程中使用滑动窗口 overlapping 策略。相邻文本块之间保留一部分重复内容，来减少语义被截断的问题

### 实体和关系的提取

第二步，对 chunks 进行实体和关系的抽取。将实体(entity)作为 Node，关系(relation)作为 Edge，构成一个图

```Python
# lightrag/lightrag.py:1971
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/lightrag.py?plain=1#L1971

# Stage 2: Process entity relation graph (after text_chunks are saved)
entity_relation_task = asyncio.create_task(
    self._process_extract_entities(
        chunks, pipeline_status, pipeline_status_lock
    )
)
chunk_results = await entity_relation_task
```


核心逻辑是 `extract_entities`

```Python
# lightrag/operate.py:2813
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/operate.py?plain=1#L2813

async def extract_entities(
    chunks: dict[str, TextChunkSchema],
    global_config: dict[str, str],
    pipeline_status: dict = None,
    pipeline_status_lock=None,
    llm_response_cache: BaseKVStorage | None = None,
    text_chunks_storage: BaseKVStorage | None = None,
) -> list:
```

对每个 chunk 调用 LLM 抽取实体和关系，解析不稳定输出，支持可选的 gleaning，并通过缓存避免重复调用。

核心逻辑在 `_process_single_content` 中，首先我们构造 `prompt` 去调用 LLM

```Python
# lightrag/operate.py:2881
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/operate.py?plain=1#L2881

# Get initial extraction
# Format system prompt without input_text for each chunk (enables OpenAI prompt caching across chunks)
entity_extraction_system_prompt = PROMPTS[
    "entity_extraction_system_prompt"
].format(**context_base)
# Format user prompts with input_text for each chunk
entity_extraction_user_prompt = PROMPTS["entity_extraction_user_prompt"].format(
    **{**context_base, "input_text": content}
)
entity_continue_extraction_user_prompt = PROMPTS[
    "entity_continue_extraction_user_prompt"
].format(**{**context_base, "input_text": content})

final_result, timestamp = await use_llm_func_with_cache(
    entity_extraction_user_prompt,
    use_llm_func,
    system_prompt=entity_extraction_system_prompt,
    llm_response_cache=llm_response_cache,
    cache_type="extract",
    chunk_id=chunk_key,
    cache_keys_collector=cache_keys_collector,
)
```

对应的 `PROMPT` 定义在 https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/prompt.py?plain=1#L11

system prompt (`entity_extraction_system_prompt`):
- 实体输出格式：`entity<|#|>name<|#|>type<|#|>description`
- 关系输出格式：`relation<|#|>source<|#|>target<|#|>keywords<|#|>description`
- N-ary 关系分解：prompt 明确要求将多实体关系拆解为二元对
- 关系为无向：`Treat all relationships as undirected`
- 第三人称、避免代词
- 完成标志：`<|COMPLETE|>`

user prompt (`entity_extraction_user_prompt`) 包含对应 chunk 的文本内容

输出格式可以看这个 `PROMPTS["entity_extraction_examples"]`

拿到 LLM 的返回之后尝试进行数据格式校验和纠错，并且解析提取的结果

```Python
# lightrag/operate.py:930
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/operate.py?plain=1#L930

async def _process_extraction_result(
    result: str,
    chunk_key: str,
    timestamp: int,
    file_path: str = "unknown_source",
    tuple_delimiter: str = "<|#|>",
    completion_delimiter: str = "<|COMPLETE|>",
) -> tuple[dict, dict]:
```

我们会得到两组数据，一组是 node 的数据，一组是 edge 的数据

比如 LLM 返回

```
("entity"<|>贾宝玉<|>PERSON<|>荣国府贾政之子，衔玉而生，性情温和，与林黛玉青梅竹马)
("entity"<|>林黛玉<|>PERSON<|>贾母外孙女，才华出众，体弱多病，初入贾府)
("entity"<|>甄士隐<|>PERSON<|>姑苏乡绅，开篇人物，梦游太虚幻境)
("relationship"<|>贾宝玉<|>林黛玉<|>初见便觉似曾相识，青梅竹马的恋人，共读西厢记<|>9<|>爱情, 木石前盟)
("relationship"<|>贾宝玉<|>甄士隐<|>甄士隐梦中见宝玉通灵宝玉<|>3<|>梦境, 通灵)
<|COMPLETE|>
```

解析之后得到的 node 和 edge 可以为

```
maybe_nodes = {
    "贾宝玉": [{"entity_name": "贾宝玉", "entity_type": "PERSON",
                "description": "荣国府贾政之子，衔玉而生...",
                "source_id": "chunk-r01", "file_path": "红楼梦.pdf"}],
    "林黛玉": [{"entity_name": "林黛玉", "entity_type": "PERSON",
                "description": "贾母外孙女，才华出众，体弱多病，初入贾府",
                "source_id": "chunk-r01", "file_path": "红楼梦.pdf"}],
    "甄士隐": [{"entity_name": "甄士隐", "entity_type": "PERSON",
                "description": "姑苏乡绅，开篇人物...",
                "source_id": "chunk-r01", "file_path": "红楼梦.pdf"}],
}

maybe_edges = {
    ("贾宝玉", "林黛玉"): [{"src_id": "贾宝玉", "tgt_id": "林黛玉",
                           "description": "初见便觉似曾相识，青梅竹马的恋人...",
                           "keywords": "爱情, 木石前盟", "weight": 9.0,
                           "source_id": "chunk-r01", "file_path": "红楼梦.pdf"}],
    ("贾宝玉", "甄士隐"): [...],
}
```


如果 `entity_extract_max_gleaning` 为 1，那么我们还会进行一轮补充提取(`gleaning`)。

```Python
# lightrag/operate.py:2925
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/operate.py?plain=1#L2925

history_str = json.dumps(history, ensure_ascii=False)
full_context_str = (
    entity_extraction_system_prompt
    + history_str
    + entity_continue_extraction_user_prompt
)
token_count = len(tokenizer.encode(full_context_str))

if token_count > max_input_tokens:
    logger.warning(
        f"Gleaning stopped for chunk {chunk_key}: Input tokens ({token_count}) exceeded limit ({max_input_tokens})."
    )
else:
    glean_result, timestamp = await use_llm_func_with_cache(
        entity_continue_extraction_user_prompt,
        use_llm_func,
        system_prompt=entity_extraction_system_prompt,
        llm_response_cache=llm_response_cache,
        history_messages=history,
        cache_type="extract",
        chunk_id=chunk_key,
        cache_keys_collector=cache_keys_collector,
    )

    # Process gleaning result separately with file path
    glean_nodes, glean_edges = await _process_extraction_result(
        glean_result,
        chunk_key,
        timestamp,
        file_path,
        tuple_delimiter=context_base["tuple_delimiter"],
        completion_delimiter=context_base["completion_delimiter"],
    )
```

基于 `history` 和形如下面这样的 prompt，要求 LLM 补充发现遗漏的实体

```
Based on the last extraction task, identify and extract any **missed or incorrectly formatted** entities and relationships from the input text.
```

### 图合并

```Python
# lightrag/lightrag.py:2065
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/lightrag.py?plain=1#L2065

await merge_nodes_and_edges(
    chunk_results=chunk_results,  # result collected from entity_relation_task
    knowledge_graph_inst=self.chunk_entity_relation_graph,
    entity_vdb=self.entities_vdb,
    relationships_vdb=self.relationships_vdb,
    global_config=asdict(self),
    full_entities_storage=self.full_entities,
    full_relations_storage=self.full_relations,
    doc_id=doc_id,
    pipeline_status=pipeline_status,
    pipeline_status_lock=pipeline_status_lock,
    llm_response_cache=self.llm_response_cache,
    entity_chunks_storage=self.entity_chunks,
    relation_chunks_storage=self.relation_chunks,
    current_file_number=current_file_number,
    total_files=total_files,
    file_path=file_path,
)
```

这个函数定义在 https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/operate.py?plain=1#L2443`

#### 合并 chunk

第一步收集所有 chunk 的提取结果

```Python
# lightrag/operate.py:2493
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/operate.py?plain=1#L2493

# Collect all nodes and edges from all chunks
all_nodes = defaultdict(list)
all_edges = defaultdict(list)

for maybe_nodes, maybe_edges in chunk_results:
    # Collect nodes
    for entity_name, entities in maybe_nodes.items():
        all_nodes[entity_name].extend(entities)

    # Collect edges with sorted keys for undirected graph
    for edge_key, edges in maybe_edges.items():
        sorted_edge_key = tuple(sorted(edge_key))
        all_edges[sorted_edge_key].extend(edges)
```

因为我们可能很多 Chunk 中都会提取到这个实体和这个二元关系，这里分别根据 `entity_name` 和 `edge_key` 进行分组合并

#### 合并实体

第二步进行实体合并 

```Python
# lightrag/operate.py:2583
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/operate.py?plain=1#L2583

entity_tasks = []
for entity_name, entities in all_nodes.items():
    task = asyncio.create_task(_locked_process_entity_name(entity_name, entities))
    entity_tasks.append(task)
```

核心是调用的 `_merge_nodes_then_upsert` 函数 https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/operate.py?plain=1#L1613

1. 找到图中现有的 node
2. 调用 `merge_source_ids` 合并多个 source_id 
3. 调用 `_handle_entity_relation_summary` 进行 description 的合并
4. 数据写入 `knowledge_graph_inst` 和 `entities_vdb`

在处理 `source_id` 和 `description` 的时候，如果数据过长都有相应的策略进行处理。比如 `description` 过长则会使用 LLM 进行合并`


#### 合并关系

```Python
# lightrag/operate.py:2697
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/operate.py?plain=1#L2697

edge_tasks = []
for edge_key, edges in all_edges.items():
    task = asyncio.create_task(_locked_process_edges(edge_key, edges))
    edge_tasks.append(task)
```


核心是调用的 `_merge_edges_then_upsert` 函数 https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/operate.py?plain=1#L1918
这个函数比较长，因为 edge 的数据是包含两个 node 的，如果对应的 node 不存在，那么会进行补全。所以实际上会写入
- `knowledge_graph_inst`
- `entity_vdb`
- `relationships_vdb`


最后一步，更新 `full_entities_storage` 和 `full_relations_storage`


### 更新任务状态持久化


```Python
# lightrag/lightrag.py:2087
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/lightrag.py?plain=1#L2087

await self.doc_status.upsert(
    {
        doc_id: {
            "status": DocStatus.PROCESSED,
            "chunks_count": len(chunks),
            "chunks_list": list(chunks.keys()),
            "content_summary": status_doc.content_summary,
            "content_length": status_doc.content_length,
            "created_at": status_doc.created_at,
            "updated_at": datetime.now(
                timezone.utc
            ).isoformat(),
            "file_path": file_path,
            "track_id": status_doc.track_id,  # Preserve existing track_id
            "metadata": {
                "processing_start_time": processing_start_time,
                "processing_end_time": processing_end_time,
            },
        }
    }
)

# Call _insert_done after processing each file
await self._insert_done()
```


## Summary

整个对于文档上传拆分向量化的逻辑对应了论文中 Graph-based Text Indexing 这一个概念。以下内容来自与论文

这种方法通过构建实体与关系的图结构，使系统能够捕获实体之间复杂的依赖关系，从而生成更加连贯且上下文丰富的回答。

LightRAG 通过将文档切分为较小的片段来增强检索系统。这种策略的优点：
- 更快定位相关信息
- 不需要扫描整个文档
- 降低检索成本

随后使用 LLM 从文本中识别并抽取：
- 实体（entity）
- 实体之间的关系（relationship）

例如：
- 人名
- 日期
- 地点
- 事件

在不同文本段落中可能会出现相同的实体或关系。 LightRAG 会：
- 识别重复实体
- 合并节点
- 合并关系

目的：
- 减少图规模
- 提高检索效率
- 优化图操作

## Reference

- https://github.com/HKUDS/LightRAG
- https://arxiv.org/abs/2410.05779
