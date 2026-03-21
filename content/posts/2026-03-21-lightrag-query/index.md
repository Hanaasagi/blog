+++
title = "LightRAG 源码阅读 - Query"
summary = ""
description = ""
categories = [""]
tags = ["RAG", "AI", "LLM", "LightRAG", "Context Engineering"]
date = 2026-03-21T10:06:09+09:00
draft = false

+++



代码基于 commit-sha `e675598db402da0241f089a22f6939dfbe223437`



## 提问-检索-回答

对于用户提问，LightRAG 提供了两个 Web API `/query` 和 `/query/stream`，不过我们最后都会走到 `aquery_llm` 这个方法里面

```Python
# lightrag/lightrag.py:2768
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/lightrag.py?plain=1#L2768

async def aquery_llm(
    self,
    query: str,
    param: QueryParam = QueryParam(),
    system_prompt: str | None = None,
) -> dict[str, Any]:
```

这里根据 `mode` 参数的不同，支持六种模式

| mode | 入口函数 | 检索方式 |
|---|---|---|
| local | `kg_query()` | 低阶关键词 -> 实体向量检索 -> 一跳邻边扩展 |
| global | `kg_query()` | 高阶关键词 -> 关系向量检索 -> 邻实体扩展 |
| hybrid | `kg_query()` | local + global 合并 |
| mix | `kg_query()` | hybrid + 向量 chunk 检索 |
| naive | `naive_query()` | 纯向量 chunk 检索 |
| bypass | 直接 LLM | 不做检索 |


我们来看核心的 `kq_query`

```Python
# lightrag/operate.py:3084
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/operate.py?plain=1#L3084

async def kg_query(
    query: str,
    knowledge_graph_inst: BaseGraphStorage,
    entities_vdb: BaseVectorStorage,
    relationships_vdb: BaseVectorStorage,
    text_chunks_db: BaseKVStorage,
    query_param: QueryParam,
    global_config: dict[str, str],
    hashing_kv: BaseKVStorage | None = None,
    system_prompt: str | None = None,
    chunks_vdb: BaseVectorStorage = None,
) -> QueryResult | None:
```

### 关键词提取

首先第一步是做关键词提取 `get_keywords_from_query`

```Python
# lightrag/operate.py:3136
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/operate.py?plain=1#L3136

hl_keywords, ll_keywords = await get_keywords_from_query(
    query, query_param, global_config, hashing_kv
)

# Handle empty keywords
if ll_keywords == [] and query_param.mode in ["local", "hybrid", "mix"]:
    logger.warning("low_level_keywords is empty")
if hl_keywords == [] and query_param.mode in ["global", "hybrid", "mix"]:
    logger.warning("high_level_keywords is empty")
if hl_keywords == [] and ll_keywords == []:
    if len(query) < 50:
        logger.warning(f"Forced low_level_keywords to origin query: {query}")
        ll_keywords = [query]
    else:
        return QueryResult(content=PROMPTS["fail_response"])
```


`get_keywords_from_query` 会调用 `extract_keywords_only`，提取关键词。
如果没有提取出来关键词并且 `query` 长度小于 50 则将 `query` 作为低阶关键词

```Python
# lightrag/operate.py:3326
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/operate.py?plain=1#L3326

async def extract_keywords_only(
    text: str,
    param: QueryParam,
    global_config: dict[str, str],
    hashing_kv: BaseKVStorage | None = None,
) -> tuple[list[str], list[str]]:
    """
    Extract high-level and low-level keywords from the given 'text' using the LLM.
    This method does NOT build the final RAG context or provide a final answer.
    It ONLY extracts keywords (hl_keywords, ll_keywords).
    """
```

关键词的提取是依靠 LLM，对应的 prompt 定义在 `PROMPTS["keywords_extraction"]` 中，
这一步会提取出来 `hl_keywords` 和 `ll_keywords` 两组数据

类似这样
```
PROMPTS["keywords_extraction_examples"] = [
    """Example 1:

Query: "How does international trade influence global economic stability?"

Output:
{
  "high_level_keywords": ["International trade", "Global economic stability", "Economic impact"],
  "low_level_keywords": ["Trade agreements", "Tariffs", "Currency exchange", "Imports", "Exports"]
}

""",
]
```

### 构建查询上下文

然后我们会 `_build_query_context` 构建查询上下文

```Python
# lightrag/operate.py:3159
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/operate.py?plain=1#L3159

# Build query context (unified interface)
context_result = await _build_query_context(
    query,
    ll_keywords_str,
    hl_keywords_str,
    knowledge_graph_inst,
    entities_vdb,
    relationships_vdb,
    text_chunks_db,
    query_param,
    chunks_vdb,
)
```

其定义如下

```Python
# lightrag/operate.py:4159
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/operate.py?plain=1#L4159

async def _build_query_context(
    query: str,
    ll_keywords: str,
    hl_keywords: str,
    knowledge_graph_inst: BaseGraphStorage,
    entities_vdb: BaseVectorStorage,
    relationships_vdb: BaseVectorStorage,
    text_chunks_db: BaseKVStorage,
    query_param: QueryParam,
    chunks_vdb: BaseVectorStorage = None,
) -> QueryContextResult | None:
    """
    Main query context building function using the new 4-stage architecture:
    1. Search -> 2. Truncate -> 3. Merge chunks -> 4. Build LLM context

    Returns unified QueryContextResult containing both context and raw_data.
    """

    if not query:
        logger.warning("Query is empty, skipping context building")
        return None

    # Stage 1: Pure search
    search_result = await _perform_kg_search(
        query,
        ll_keywords,
        hl_keywords,
        knowledge_graph_inst,
        entities_vdb,
        relationships_vdb,
        text_chunks_db,
        query_param,
        chunks_vdb,
    )

    if not search_result["final_entities"] and not search_result["final_relations"]:
        if query_param.mode != "mix":
            return None
        else:
            if not search_result["chunk_tracking"]:
                return None

    # Stage 2: Apply token truncation for LLM efficiency
    truncation_result = await _apply_token_truncation(
        search_result,
        query_param,
        text_chunks_db.global_config,
    )

    # Stage 3: Merge chunks using filtered entities/relations
    merged_chunks = await _merge_all_chunks(
        filtered_entities=truncation_result["filtered_entities"],
        filtered_relations=truncation_result["filtered_relations"],
        vector_chunks=search_result["vector_chunks"],
        query=query,
        knowledge_graph_inst=knowledge_graph_inst,
        text_chunks_db=text_chunks_db,
        query_param=query_param,
        chunks_vdb=chunks_vdb,
        chunk_tracking=search_result["chunk_tracking"],
        query_embedding=search_result["query_embedding"],
    )

    if (
        not merged_chunks
        and not truncation_result["entities_context"]
        and not truncation_result["relations_context"]
    ):
        return None

    # Stage 4: Build final LLM context with dynamic token processing
    # _build_context_str now always returns tuple[str, dict]
    context, raw_data = await _build_context_str(
        entities_context=truncation_result["entities_context"],
        relations_context=truncation_result["relations_context"],
        merged_chunks=merged_chunks,
        query=query,
        query_param=query_param,
        global_config=text_chunks_db.global_config,
        chunk_tracking=search_result["chunk_tracking"],
        entity_id_to_original=truncation_result["entity_id_to_original"],
        relation_id_to_original=truncation_result["relation_id_to_original"],
    )

    # Convert keywords strings to lists and add complete metadata to raw_data
    hl_keywords_list = hl_keywords.split(", ") if hl_keywords else []
    ll_keywords_list = ll_keywords.split(", ") if ll_keywords else []

    # Add complete metadata to raw_data (preserve existing metadata including query_mode)
    if "metadata" not in raw_data:
        raw_data["metadata"] = {}

    # Update keywords while preserving existing metadata
    raw_data["metadata"]["keywords"] = {
        "high_level": hl_keywords_list,
        "low_level": ll_keywords_list,
    }
    raw_data["metadata"]["processing_info"] = {
        "total_entities_found": len(search_result.get("final_entities", [])),
        "total_relations_found": len(search_result.get("final_relations", [])),
        "entities_after_truncation": len(
            truncation_result.get("filtered_entities", [])
        ),
        "relations_after_truncation": len(
            truncation_result.get("filtered_relations", [])
        ),
        "merged_chunks_count": len(merged_chunks),
        "final_chunks_count": len(raw_data.get("data", {}).get("chunks", [])),
    }

    return QueryContextResult(context=context, raw_data=raw_data)
```

这里又分为了几步

#### Pure search (`_perform_kg_search`)

在 `_perform_kg_search` 中，首先我们会在检索前一次性计算所需要的 embedding

```Python
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/operate.py?plain=1#L3522

    # Pre-compute embeddings needed by the selected mode in a single batch call.
    # Only embed texts that the active retrieval branches will actually use:
    #   - query        → used by _get_vector_context (chunks VDB)
    #   - ll_keywords  → used by _get_node_data (entities VDB) in local/hybrid/mix
    #   - hl_keywords  → used by _get_edge_data (relationships VDB) in global/hybrid/mix
    # Batching avoids 2-3 sequential API round-trips.
```


然后我们会根据不同的 `mode` 执行不同的检索

```Python
# lightrag/operate.py:3578
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/operate.py?plain=1#L3578

if query_param.mode == "local" and len(ll_keywords) > 0:
    local_entities, local_relations = await _get_node_data(
        ll_keywords,
        knowledge_graph_inst,
        entities_vdb,
        query_param,
        query_embedding=ll_embedding,
    )

elif query_param.mode == "global" and len(hl_keywords) > 0:
    global_relations, global_entities = await _get_edge_data(
        hl_keywords,
        knowledge_graph_inst,
        relationships_vdb,
        query_param,
        query_embedding=hl_embedding,
    )

else:  # hybrid or mix mode
    if len(ll_keywords) > 0:
        local_entities, local_relations = await _get_node_data(
            ll_keywords,
            knowledge_graph_inst,
            entities_vdb,
            query_param,
            query_embedding=ll_embedding,
        )
    if len(hl_keywords) > 0:
        global_relations, global_entities = await _get_edge_data(
            hl_keywords,
            knowledge_graph_inst,
            relationships_vdb,
            query_param,
            query_embedding=hl_embedding,
        )

    # Get vector chunks for mix mode
    if query_param.mode == "mix" and chunks_vdb:
        vector_chunks = await _get_vector_context(
            query,
            chunks_vdb,
            query_param,
            query_embedding,
        )
```

`ll_keywords` 对应 node 维度的检索，`hl_keywords` 对应 edge 维度的检索。下面我们来看使用到的具体的检索

##### `_get_node_data`

```Python
# lightrag/operate.py:4279
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/operate.py?plain=1#L4279

async def _get_node_data(
    query: str,
    knowledge_graph_inst: BaseGraphStorage,
    entities_vdb: BaseVectorStorage,
    query_param: QueryParam,
    query_embedding=None,
):

    results = await entities_vdb.query(
        query, top_k=query_param.top_k, query_embedding=query_embedding
    )

    # Extract all entity IDs from your results list
    node_ids = [r["entity_name"] for r in results]

    # Call the batch node retrieval and degree functions concurrently.
    nodes_dict, degrees_dict = await asyncio.gather(
        knowledge_graph_inst.get_nodes_batch(node_ids),
        knowledge_graph_inst.node_degrees_batch(node_ids),
    )

    # Now, if you need the node data and degree in order:
    node_datas = [nodes_dict.get(nid) for nid in node_ids]
    node_degrees = [degrees_dict.get(nid, 0) for nid in node_ids]

    node_datas = [
        {
            **n,
            "entity_name": k["entity_name"],
            "rank": d,
            "created_at": k.get("created_at"),
        }
        for k, n, d in zip(results, node_datas, node_degrees)
        if n is not None
    ]

    use_relations = await _find_most_related_edges_from_entities(
        node_datas,
        query_param,
        knowledge_graph_inst,
    )

    return node_datas, use_relations
```

- `entities_vdb.query(ll_keywords, top_k)`: 按向量相似度检索 top_k 实体
- `get_nodes_batch` + `node_degrees_batch`: 并发获取节点属性和度
- `_find_most_related_edges_from_entities(entities)`: 从实体出发，批量查询所有相连的边，去重后按 `(rank, weight)` 降序排序

##### `_get_edge_data`

```Python
# lightrag/operate.py:4554
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/operate.py?plain=1#L4554

async def _get_edge_data(
    keywords,
    knowledge_graph_inst: BaseGraphStorage,
    relationships_vdb: BaseVectorStorage,
    query_param: QueryParam,
    query_embedding=None,
):
    results = await relationships_vdb.query(
        keywords, top_k=query_param.top_k, query_embedding=query_embedding
    )

    edge_pairs_dicts = [{"src": r["src_id"], "tgt": r["tgt_id"]} for r in results]
    edge_data_dict = await knowledge_graph_inst.get_edges_batch(edge_pairs_dicts)

    # ...

    use_entities = await _find_most_related_entities_from_relationships(
        edge_datas,
        query_param,
        knowledge_graph_inst,
    )

    return edge_datas, use_entities
```

- `relationships_vdb.query(hl_keywords, top_k)`: 按向量相似度检索关系
- `get_edges_batch` 取边属性，缺失 weight 默认 1.0
- `_find_most_related_entities_from_relationships(relations)`: 取边端点实体

##### `_get_vector_context`

```Python
# lightrag/operate.py:3436
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/operate.py?plain=1#L3436

async def _get_vector_context(
    query: str,
    chunks_vdb: BaseVectorStorage,
    query_param: QueryParam,
    query_embedding: list[float] = None,
) -> list[dict]:
      search_top_k = query_param.chunk_top_k or query_param.top_k
      cosine_threshold = chunks_vdb.cosine_better_than_threshold

      results = await chunks_vdb.query(
          query, top_k=search_top_k, query_embedding=query_embedding
      )
```

这个就是直接检索了 chunks_vdb

回到我们的 `_perform_kg_search` 的流程中，我们对于 `local_entities` 和 `global_entities` 的结果进行交替合并并跳过重复的，
构造出 `final_entities`。同理 `final_relations` 也是这样的，确保两侧都有代表


#### Apply token truncation for LLM efficiency

`_apply_token_truncation` 会按 token 预算截断实体/关系列表

```Python
# lightrag/operate.py:4202
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/operate.py?plain=1#L4202

truncation_result = await _apply_token_truncation(
    search_result,
    query_param,
    text_chunks_db.global_config,
)
```

实体列表按 `truncate_list_by_token_size` 限制在 `max_entity_tokens`, 关系列表按 `max_relation_tokens`截断，
截断后过滤出 `filtered_entities` 和 `filtered_relations`

#### 合并 Chunk

```Python
# lightrag/operate.py:4209
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/operate.py?plain=1#L4209

merged_chunks = await _merge_all_chunks(
    filtered_entities=truncation_result["filtered_entities"],
    filtered_relations=truncation_result["filtered_relations"],
    vector_chunks=search_result["vector_chunks"],
    query=query,
    knowledge_graph_inst=knowledge_graph_inst,
    text_chunks_db=text_chunks_db,
    query_param=query_param,
    chunks_vdb=chunks_vdb,
    chunk_tracking=search_result["chunk_tracking"],
    query_embedding=search_result["query_embedding"],
)
```

`_merge_all_chunks` 定义在 https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/operate.py?plain=1#L3874

- 从 `filtered_entities` 取关联 chunk 做去重计数和加权轮询 (`_find_related_text_unit_from_entities`)
- 从 `filtered_relations` 取关联 chunk，跳过已经在 `entity_chunks` 中出现过的
- 取 vector_chunks，mix mode 下我们在上一步提取过
- 三路 round-robin 合并，按 `chunk_id` 去重。交错保证每种来源的 top 结果都有机会进入有限的 `chunk_top_k` 窗口


#### 构建最终的 LLM 上下文

`_build_context_str` 定义在 https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/operate.py?plain=1#L3976

- 对于 chunk 进行 去重、限制 chunk_top_k、重新排序和标记截断 (`process_chunks_unified`)
- 生成引用列表 (`generate_reference_list_from_chunks`)
- 渲染我们的 prompt 模版


### 调用 LLM

当我们拿到 `_build_query_context` 的结果之后我们会构造 LLM 的输入

```Python
# lightrag/operate.py:3181
# https://github.com/HKUDS/LightRAG/blob/e675598db402da0241f089a22f6939dfbe223437/lightrag/operate.py?plain=1#L3181

user_prompt = f"\n\n{query_param.user_prompt}" if query_param.user_prompt else "n/a"
response_type = (
    query_param.response_type
    if query_param.response_type
    else "Multiple Paragraphs"
)

# Build system prompt
sys_prompt_temp = system_prompt if system_prompt else PROMPTS["rag_response"]
sys_prompt = sys_prompt_temp.format(
    response_type=response_type,
    user_prompt=user_prompt,
    context_data=context_result.context,
)

user_query = query

# Call LLM
tokenizer: Tokenizer = global_config["tokenizer"]
len_of_prompts = len(tokenizer.encode(query + sys_prompt))

# ... 这里省略一个 cache 
response = await use_model_func(
    user_query,
    system_prompt=sys_prompt,
    history_messages=query_param.conversation_history,
    enable_cot=True,
    stream=query_param.stream,
)
```

这个 `response` 会做简单的后处理之后返回给调用方



## Summary



LigthtRAG 使用了两层的检索，对于三个维度的数据源(mix mode) 进行提取

- 低层检索(Low-Level Retrieval): 此层级主要侧重于检索特定实体及其相关属性或关系。此层级的查询注重细节，旨在提取图中特定节点或边的精确信息。 

- 高层检索(Abstract Queries): 此层级关注更广泛的主题和总体思路。此层级的查询会聚合多个相关实体和关系的信息，提供对更高层次概念和概括的洞察，而非具体细节。





## Reference

- https://github.com/HKUDS/LightRAG
- https://arxiv.org/abs/2410.05779
