+++
title = "tgrep 源码阅读：trigram 索引"
summary = "以 tgrep 源码为例，拆解 trigram 索引的完整实现：三字节滑动窗口的零碰撞 u32 编码、原始与 ASCII 小写双套提取、next_mask Bloom 过滤、无压缩磁盘格式与 mmap 读取、批处理与外部排序构建，以及从正则 pattern 到查询计划树、smallest-first 交集的候选粗筛执行。"
description = "tgrep 源码阅读笔记：trigram 索引如何从文件提取三字节窗口、以原始小端二进制格式落盘，并在查询时把正则分解为 trigram 计划做筛选。"
categories = ["Rust"]
tags = ["Rust", "trigram", "search", "indexing", "tgrep"]
date = 2026-10-06T08:23:33+09:00
draft = false

+++



本文基于 [tgrep](https://github.com/microsoft/tgrep) commit [7b70671](https://github.com/microsoft/tgrep/tree/7b70671) 的代码，分析一下 trigram 索引的实现。tgrep 复用了 ripgrep 生态的一些 crate：`ignore` 负责目录遍历和 gitignore 规则，`regex`/`regex-syntax` 负责正则，`globset`/`memchr` 负责 glob 匹配和字节级搜索加速，然后在此之上构建了 trigram 索引层。对于大型代码仓库，直接 grep 需要逐文件扫描，而 trigram 索引可以更快地定位到包含特定模式的候选文件，再只对这些文件做正则匹配。我们来看一下它是怎么做的



## 什么是 Trigram

Trigram 就是字节序列上每个长度为 3 的重叠滑动窗口。给定字符串 `the cat`，它的 trigram 集合为：

```text
the cat
^^^      -> "the"
 ^^^     -> "he "
  ^^^    -> "e c"
   ^^^   -> " ca"
    ^^^  -> "cat"
```



为什么选 3 个字符？这是实践里的平衡点：2-gram 一共只有 65536 种，种类太少，区分度不够；4-gram 有 2^32 约 43 亿种，索引会膨胀得太厉害；3 个字符是 2^24 约 1670 万种，刚好。



在 tgrep 中，三个字节被打包进一个 `u32` 的低 24 位，直接作为唯一哈希使用：

```rust
// tgrep-core/src/trigram.rs
pub type TrigramHash = u32;

#[inline]
pub fn hash(a: u8, b: u8, c: u8) -> TrigramHash {
    (a as u32) << 16 | (b as u32) << 8 | c as u32
}
```



比如 `hash(b't', b'h', b'e')` 得到 `0x746865`，就是 `t`、`h`、`e` 三个 ASCII 码拼在一起。不同的三字节窗口绝对不会产生相同的哈希值，这意味着只要一个 trigram 匹配，对应的文件一定包含该三字节序列。严格来说，文件包含该序列本身，或某个 ASCII 小写化后恰好等于它的窗口，因为索引同时收录原始和小写两套 trigram。两套的原因在 Case-Insensitive 一节会说明



## Trigram 提取

### 基础提取

最简单的提取就是对字节序列做 `windows(3)` 滑动，去重后收集：

```rust
// tgrep-core/src/trigram.rs
pub fn extract(data: &[u8]) -> Vec<TrigramHash> {
    if data.len() < 3 {
        return Vec::new();
    }
    let mut seen = HashSet::new();
    let mut result = Vec::new();
    for window in data.windows(3) {
        let h = hash(window[0], window[1], window[2]);
        if seen.insert(h) {
            result.push(h);
        }
    }
    result
}
```



注意这是**字节级**的滑动窗口，不是 Unicode 字符级。UTF-8 多字节字符会被拆进多个 trigram，例如 `ü`（`\xc3\xbc`）的两个字节会和前后字节一起组成 trigram。这并不影响搜索正确性，查询侧也对 pattern 做同样的字节级处理。



### 带 Mask 的提取（索引构建实际使用的版本）

不过磁盘索引的构建用的并不是这个基础版本，而是 `extract_merged_masks`。这个函数同时提取原始和 ASCII 小写两套 trigram，并且为每个 trigram 附加两个辅助 mask：

```rust
// tgrep-core/src/trigram.rs
pub struct TrigramMasks {
    /// 位 i 置位表示该 trigram 出现在 offset % 8 == i 的位置
    pub loc_mask: u8,
    /// 紧随该 trigram 之后的字符的 8 位 Bloom 过滤器
    pub next_mask: u8,
}
```



这两个 mask 都只占 1 字节：

- **`loc_mask`**：记录 trigram 出现的位置信息，对应代码里的 `loc_bit(i)`，就是 `1 << (i % 8)`：trigram 出现在偏移 `i` 处时，把第 `i % 8` 个比特置位。这样设计是因为相邻的 trigram 起点正好差 1，位置也就差 1，`check_adjacency` 就是把前一个的 loc_mask 循环左移一位，再和后一个做 AND 来判断相邻。只记模 8 的余数当然会撞，比如 offset 1 和 offset 9 就是同一位，所以只能算粗判。当前的查询路径 `execute_plan_with_masks` 并未调用它，这个 mask 已落盘，是为未来优化预留的能力
- **`next_mask`**：是紧跟在 trigram 后面那个字节的 8 位 Bloom 过滤器。Bloom 过滤器用少量 bit 记录"哪些值可能出现过"，查询时如果已知下一个字节是什么，就可以拿这个 mask 做快速排除，被排除的文件一定是安全的，留下来的交给正则验证即可。它具体怎么工作，我们接着往下看。这是当前查询阶段唯一使用的 mask 优化



`next_mask` 的 Bloom 位是用乘法哈希算出来的：

```rust
// tgrep-core/src/trigram.rs
#[inline]
fn next_bit(byte: u8) -> u8 {
    1u8 << (byte.wrapping_mul(0x9E) >> 5 & 0x07)
}
```



这个函数要做的是把 256 种字节映射到 8 个 bit 位置，多对一是必然的。分两步走：先用乘法混合选位置，再 `1u8 <<` 置位。

**第一步：乘法混合。** 先约定位置的编号：一个 u8 有 8 个 bit，从右往左依次是第 0 位到第 7 位，`1u8 << n` 就是把第 `n` 位置 1。

最直接的映射是 `byte % 8`，也就是取输入最低的 3 位。问题是它只看这 3 位，高 5 位完全不影响结果，于是只有高位不同的字节会全部撞在一起。26 对大小写字母就是这种情况：

```text
'A' = 0x41 = 0b0100_0001
'a' = 0x61 = 0b0110_0001
                 ^
                 只有第 5 位（0x20）不同，低 3 位都是 001
```

所以对 `% 8` 来说，`A` 和 `a` 是同一个位置，26 对字母一对都分不开。

乘法混合利用的是二进制乘法的一个性质：乘积的第 `i` 位受输入第 0 到 `i` 位共同影响，越高的位混入的输入信息越多。那就先乘一下，让低位的信息扩散到高位，再取乘积的高 3 位（`>> 5`）当位置。取高 3 位还有个附带的好处：大小写差的那一位是第 5 位，它只能影响乘积的第 5 位及更高位，`>> 5` 留下来的恰好就是这几位。`0x9E`（158）这个乘数的效果实际跑一下就能看到：26 对大小写字母**全部**被分到不同位置，95 个可打印 ASCII 落在 8 个位置上的数量在 11–13 之间，接近理想均匀。

以空格为例走一遍完整计算：

```text
0x20 × 0x9E = 0x13C0
0xC0 = 0b1100_0000     <- u8 截断，保留低 8 位
0b110 = 6              <- >> 5，取最高 3 位（& 0x07 对 u8 是冗余保险）
0b0100_0000 = 0x40     <- 1u8 << 6，把第 6 位置 1
```

空格映射到第 6 位，也就是 `next_mask` 里 `0b0100_0000` 那一位。

这里还有一个和字符编码直接相关的性质：`0x9E` 是偶数，所以 `128 × 0x9E = 20224 = 79 × 256`。换句话说，给输入字节加上 128（翻转最高的第 7 位）只会让乘积多出 256 的整数倍，而 `u8` 截断恰好就是模 256，多出的部分直接消失，映射到哪个位置只由**低 7 位**决定。对 ASCII 文本这没有任何损失（ASCII 的有效信息恰好就是 7 位）；UTF-8 多字节字符的每个字节只按低 7 位参与映射，可能和某个 ASCII 字符撞位置，但撞位置只是多一次假阳性，不影响正确性。

**第二步：one-hot 置位。** `1u8 << n` 产出的值恰好只有一个 bit 置位，一个 mask 就是记录哪些位置出现过的 bit 向量。计算 `next_mask` 就是把该 trigram 后面跟过的每个字节映射到的位置逐个 OR 进来。例如文件中 `abc` 后面跟过一次 `d` 和一次 `X`：

```text
'd' -> 位置 5:   0010 0000  (0x20)
'X' -> 位置 2:   0000 0100  (0x04)

next_mask    =   0010 0100  (0x24)    <- OR：两个位置同时置位
```

查询时拿 pattern 预期的下一字节算出同样的位置，和 mask 做 AND：

```text
预期 'f'（位置 7）:  0010 0100 & 1000 0000 = 0000 0000   -> 为 0，文件安全排除
预期 'e'（位置 2）:  0010 0100 & 0000 0100 = 0000 0100   -> 非 0，文件成为候选
```

第二种情况里 `e` 其实从未出现过，它只是和 `X` 撞了位置，算一次假阳性，留给后面的正则验证兜底。可能误报、绝不漏报，这就是 Bloom 过滤器的语义



### Case-Insensitive 的处理

`extract_merged_masks` 的一个设计是同时覆盖原始大小写和 ASCII 小写两套 trigram。这样无论查询是 case-sensitive 还是 case-insensitive，都能命中索引。



```rust
// tgrep-core/src/trigram.rs
pub fn extract_merged_masks(content: &[u8]) -> TrigramMaskMap {
    let mut per_tri = TrigramMaskMap::default();
    if content.len() < 3 {
        return per_tri;
    }

    let has_upper = content.iter().any(|byte| byte.is_ascii_uppercase());
    let len = content.len();

    for (i, window) in content.windows(3).enumerate() {
        let trigram = hash(window[0], window[1], window[2]);
        let loc = loc_bit(i);
        let next = if i + 3 < len { next_bit(content[i + 3]) } else { 0 };

        if !has_upper {
            // 全小写文件：跳过第二套逻辑
            let entry = per_tri.entry(trigram).or_default();
            entry.loc_mask |= loc;
            entry.next_mask |= next;
            continue;
        }

        let lowered = hash(
            window[0].to_ascii_lowercase(),
            window[1].to_ascii_lowercase(),
            window[2].to_ascii_lowercase(),
        );

        let next_lowered = if i + 3 < len {
            next_bit(content[i + 3].to_ascii_lowercase())
        } else { 0 };

        let entry = per_tri.entry(trigram).or_default();
        entry.loc_mask |= loc;
        if lowered == trigram {
            // 窗口本身已经是小写，合并两套 next mask
            entry.next_mask |= next | next_lowered;
            continue;
        }
        entry.next_mask |= next;

        // 同时写入小写版本的 trigram
        let lowered_entry = per_tri.entry(lowered).or_default();
        lowered_entry.loc_mask |= loc;
        lowered_entry.next_mask |= next_lowered;
    }

    per_tri
}
```



这段代码做的事情等价于：对原文件提取一遍 trigram，再对全文件的小写副本提取一遍，把两份结果按 key 合并。但它并不真的分配小写副本，而是在滑动窗口里**就地**完成。之所以可行，是因为 ASCII 小写化是逐字节独立映射，比如 `A` -> `a`，小写副本里 offset `i` 的窗口就是原文 offset `i` 窗口的小写版，`loc_bit(i)` 不变，而 mask 合并只用 `|=`，所以逐窗口算两套、就地合并，和做两遍完整提取再合并的结果完全相同。



关键是 `lowered == trigram` 这个分支。`trigram` 是原始窗口的 hash，`lowered` 是同一窗口小写化后的 hash。以 `Foo` 为例：`hash("Foo") = 0x466f6f` 和 `hash("foo") = 0x666f6f` 是两个不同的 key，需要向 map 写入两个条目，case-sensitive 查询 `Foo` 查前者，case-insensitive 查询（pattern 先转小写）查后者，都能命中。但如果窗口本身不含大写字母（比如 `the`），小写化什么都改变不了，两个 hash 指向**同一个** key：本该是第二次插入，现在 entry 已经在手，只需把两套 next mask 用一次 `|=` 并进去，第二套提取的成本从一次 hash + 一次哈希表探测降为一次 `|=`。注意即使 key 相同，两套 next mask 也不能省成一套：窗口小写不代表下一字节也小写，文件里是 `abcD` 时，原样那套记 `next_bit('D')`，小写那套记 `next_bit('d')`，两者是不同的 bit。如果只存前者，case-insensitive 查询 `abcd` 会拿着 `d` 算出来的位置来检查，导致把这个真正匹配的文件排除（假阴性）；两个 bit 都收进同一个 entry 才能同时命中两类查询，所以代码里是 `next | next_lowered`。

真实代码文件里绝大多数窗口都是纯小写，`lowered == trigram` 直接命中。如果整个文件都没有大写字母（`has_upper == false`），连 `lowered` 都不用算，直接走单套逻辑。



注意这里只做 ASCII 大小写折叠（`to_ascii_lowercase`），不是 Unicode case folding



顺带对比一下别的做法。Google Code Search 当年是在查询侧展开的：把每个 trigram 的所有大小写组合都列出来做 OR，`hello world` 的查询展开成了 9 组 AND，每组是最多 8 种大小写组合的 OR。tgrep 反过来了，在索引侧双写一套小写 trigram，查询时只要把 pattern 转成小写。同一个问题，一个把复杂度放在查询时，一个放在构建时用空间换时间



### 自定义 Hasher

由于 trigram 就是自身的哈希值（24 位无碰撞），tgrep 用一个自定义 `TrigramHasher` 替代标准库的 SipHash。

```rust
// tgrep-core/src/trigram.rs
pub struct TrigramHasher(u64);

impl std::hash::Hasher for TrigramHasher {
    #[inline]
    fn write_u32(&mut self, value: u32) {
        let mixed = u64::from(value).wrapping_mul(0x9E37_79B9_7F4A_7C15);
        self.0 = mixed ^ (mixed >> 32);
    }
}
```

换掉 SipHash 的原因是提取的时候每个输入字节都要哈希一次，正好卡在每次构建索引的关键路径上，而 trigram 本身就是无碰撞的 24 位值。乘法之后为什么还要再做一次异或移位？因为 hashbrown 取 bucket 索引用的是哈希值的低位，而 `value * K` 的低 k 位只取决于 value 的低 k 位，对 trigram 来说这部分就只有最后一个字节。`mixed ^ (mixed >> 32)` 把高 32 位折回低位，让全部 24 位都能影响 bucket 索引以及 hashbrown 高 7 位的控制字节





## 磁盘索引格式

tgrep 的内容索引由三个二进制文件组成：提取出的 trigram、mask 和文件路径，最终就落盘为这三个文件，存放在 `.tgrep/` 目录下。

索引的主体是一张 trigram -> 文件列表的映射。对一个 trigram 来说，包含它的文件 ID 列表称为它的 **posting list**（倒排表项），列表中的每个元素（一个文件 ID 外加两个 mask）称为一条 **posting**。三个文件各管一件事：`lookup.bin` 回答某个 trigram 的 posting list 在哪里，`index.bin` 是所有 posting list 的连续存放，`files.bin` 回答文件 ID 对应哪条路径。

我们先造一个真实的最小索引，再对着字节来看。

```bash
mkdir /tmp/tgrep-demo
echo -n 'the cat' > /tmp/tgrep-demo/a.txt
echo -n 'cat dog' > /tmp/tgrep-demo/b.txt
tgrep index /tmp/tgrep-demo
```

```text
Found 2 text files (0 binary skipped, 0 too large, 0 errors)
Extracting trigrams...
Writing index (9 trigrams, 2 files, 0 spill segment(s))...
```

两个文件各贡献 5 个 trigram，其中 `cat` 共享，共 9 个唯一 trigram。下面三个小节的格式说明都会用到这份真实数据



### `lookup.bin` Trigram 查找表

固定 16 字节/条目，按 trigram hash 排序（支持二分查找）：

```text
+----------------+----------------+----------------+
| trigram_hash   | offset         | length         |
| u32 (4B LE)    | u64 (8B LE)    | u32 (4B LE)    |
+----------------+----------------+----------------+
```

```rust
// tgrep-core/src/ondisk.rs
pub(crate) const LOOKUP_ENTRY_SIZE: usize = 16; // 4 + 8 + 4

pub(crate) struct LookupEntry {
    pub trigram: u32,
    pub offset: u64,
    pub length: u32,
}
```

`offset` 是 posting list 在 `index.bin` 中的字节偏移，`length` 是 posting 的**条数**（每条 6 字节）。



上面这份真实索引的 `lookup.bin` 共 144 字节 = 9 条 × 16B，解码后如下（按 trigram 升序，正是二分查找的前提）：

| trigram    | ASCII   | offset | length |
| ---------- | ------- | ------ | ------ |
| `0x206361` | `" ca"` | 0      | 1      |
| `0x20646f` | `" do"` | 6      | 1      |
| `0x617420` | `"at "` | 12     | 1      |
| `0x636174` | `"cat"` | 18     | **2**  |
| `0x646f67` | `"dog"` | 30     | 1      |
| `0x652063` | `"e c"` | 36     | 1      |
| `0x686520` | `"he "` | 42     | 1      |
| `0x742064` | `"t d"` | 48     | 1      |
| `0x746865` | `"the"` | 54     | 1      |

`cat` 的 length 是 2，因为两个文件都包含它，占 2 条 posting 共 12 字节，所以下一条从 offset 30 开始。



### `index.bin` Posting Lists

每条 posting 6 字节：

```text
+----------------+----------+----------+
| file_id        | loc_mask | next_mask|
| u32 (4B LE)    | u8 (1B)  | u8 (1B)  |
+----------------+----------+----------+
```

```rust
// tgrep-core/src/ondisk.rs
pub(crate) const POSTING_ENTRY_SIZE: usize = 6; // 4 + 1 + 1

pub struct PostingEntry {
    pub file_id: u32,
    pub loc_mask: u8,
    pub next_mask: u8,
}
```

同一个 trigram 的所有 posting 连续存放，由 `lookup.bin` 中的 `offset`/`length` 定位。这种倒排索引的结构和搜索引擎里的 inverted index 是同一个思路：key 是 trigram，value 是包含该 trigram 的文件 ID 列表。

这份索引的 `index.bin` 共 60 字节 = 10 条 × 6B。以 `the` 的 posting（`lookup.bin` 中 offset 54、length 1）为例手工验算 mask：`the` 在 a.txt 中位于 offset 0，`loc_mask = 1 << 0 = 0x01`；下一字节是空格（`0x20`），`0x20 × 0x9E = 0x13C0`，u8 截断得 `0xC0`，`>> 5` 得 6，`next_mask = 1 << 6 = 0x40`。磁盘上的实际字节是：

```text
00 00 00 00 01 40    file_id=0, loc_mask=0x01, next_mask=0x40
```



其中 `file_id = 0` 是路径表里的编号。`files.bin`（下一小节）记录着 `0 -> a.txt`、`1 -> b.txt`，而 `the` 出现在 a.txt 中。posting 存 4 字节编号而不是路径本身：一条路径会在成百上千条 posting list 中重复出现，路径只在 `files.bin` 集中存一份，`index.bin` 才能保持 6 字节/条的定长紧凑格式。编号则在构建时从 0 开始分配



### `files.bin` 文件路径表

变长记录：`file_id(u32 LE) + path_len(u16 LE) + path_bytes`（`path_len` 为路径的字节数），带有版本化的 magic header。这份索引的 `files.bin` 全文 38 字节，逐字节拆开：

```text
ff ff ff ff 00 00 ff ff ff ff 00 00   FILE_TABLE_MAGIC（12B）
03 00 00 00                           INDEX_FORMAT_VERSION = 3（u32 LE）
00 00 00 00  05 00  "a.txt"           file_id=0, path_len=5
01 00 00 00  05 00  "b.txt"           file_id=1, path_len=5
```

整个格式**不使用压缩**，全部是原始小端二进制。这是一个有意的取舍：posting list 中每条只有 6 字节，压缩收益有限，而不压缩可以做 zero-copy 的 mmap 读取。mmap 就是把文件直接映射到内存地址空间，读文件就像读内存一样，不用先拷贝出来。

除了这三个文件，`.tgrep/` 下还有两个不参与 trigram 查询的成员：`meta.json` 记录索引元数据，这份索引里就是 `version: 3`、`num_files: 2`、`num_trigrams: 9`、`complete: true`；`files-extra.bin` 是一个辅助文件，保存那些应该被 `--files` 列出但不产生内容 posting 的路径（比如二进制文件），它有自己的容器格式（`TGRPXP02` magic + JSON payload），本例中 paths 为空，因为两个文件都进了内容索引。下文中提到的索引都是指内容索引三件套





## 索引构建

### 整体流程

`builder.rs` 中的 `build_index_with_options_and_ignorecase` 是索引构建的主入口，简化后的流程如下：

1. 确定索引目录（默认 `.tgrep/`），写入未完成状态的 `meta.json`
2. 调用 `walker::walk_dir_with_ignorecase` 遍历目录，收集待索引的文件路径（遵循 `.gitignore`、hidden file 规则、`max_file_size` 限制等）
3. **分批并行**读取文件、提取 trigram
4. 将 posting 数据写入磁盘
5. 更新 `meta.json` 标记索引完成

`meta.json` 一头一尾的两次写入是有意的设计：开始时先写 `complete: false`，构建中途崩溃留下的是一个明确的"未完成"索引，而不是看似可用的半成品；全部数据落盘后才把它标记为完成，作为索引最终的发布标记。

单个文件的处理流程是 `extract_indexed_file`：

```rust
// tgrep-core/src/builder.rs
fn extract_indexed_file(path: String, data: &IndexRead) -> ExtractedFile {
    let text = crate::encoding::decode_for_index(data);
    let (trigrams, content_id) = if trigram::is_binary(&text) {
        (None, None)
    } else {
        (
            Some(trigram::extract_merged_masks(&text)),
            Some(meta::ContentId::from_indexed_bytes(&text)),
        )
    };
    // ...
}
```



先做编码检测和解码（`decode_for_index`），然后检查是否为二进制文件（前 8KB 是否包含 NUL 字节），非二进制文件才提取 trigram。代码里的 `ContentId` 是解码后字节的一份哈希，跟着索引存下来，用来标识这次索引的到底是哪份字节。它要到增量构建一节才派上用场，这里先记住这个名字。

另外这段代码还有一个版本校验的细节。文件读取有两种方式：一种是把内容读进内存，拿到一份私有拷贝（owned）；另一种是 mmap 映射，字节还在文件页上。文件打开时会记录版本信息（mtime、size 等），提取完成后再校验一次，校验失败说明读取过程中文件被别人改过。owned 的拷贝在提取和算哈希之间不可能变化，`ContentId` 依然准确；mmap 的映射则可能边读边被改，这时算出的 `ContentId` 对应不上任何一份真实内容，留下来只会误导后面的去重，所以只有 mmap 这种情况要把它丢掉



### 内存管理：批处理与预算控制

对一个大型代码仓库，不可能一次把所有文件读进内存再构建索引。tgrep 用了一套分批处理的策略：

- **`INDEX_BUILD_BATCH_BYTES = 64 MiB`**：每批同时持有的堆上读缓冲总量上限
- **`INDEX_BUILD_BATCH_SIZE = 1024`**：每批最多处理的文件数
- **`OwnedReadBudget` + `OwnedReadPermit`**：小文件的读取共享 64 MiB 配额，通过条件变量协调，drop 时自动归还

大文件（≥ 1 MiB）优先使用**只读 mmap**，字节不进堆，只有提取出的 trigram HashMap 占堆内存。由于一个文件的 unique trigram 数量有理论上限（约 1670 万，实际远小于此），这保证了峰值内存可控。映射文件也不是完全"免费"的：每个映射文件按实际大小和 2 MiB 中较小的那个计入上面的 64 MiB 批次预算（用来折算提取结果的堆占用），同时每批常驻的映射总量另有 256 MiB（`MAPPED_BATCH_BYTES`）的单独上限。

批与批之间串行处理，每批内部用 **rayon** 并行提取：

```text
batch 1: [file_0 .. file_1023] -> par_iter -> extract -> push postings
batch 2: [file_1024 .. file_2047] -> par_iter -> extract -> push postings
...
```



### 两种写入策略

tgrep 提供了两种将 posting 数据落盘的策略：

- **InMemory**：所有 posting 收集到 `Vec<TrigramPosting>` 里，排好序一次性写入 `lookup.bin` 和 `index.bin`。实现简单，但这个 Vec 没有上限：一个 50 万文件的仓库，每个文件平均几千个 trigram，光堆内存就要 10 GB 以上，还没开始写盘就撑不住了。
- **External**（默认）：用外部排序（`ExternalSorter`）把峰值内存限定在一块固定大小的缓冲区上，装不下就往磁盘溢出，最后再归并。它的过程值得展开看一下。

ExternalSorter 的缓冲区（arena）是 64 MiB（和上面批次的 64 MiB 是两回事），一条 posting 在里面占 12 字节，装满大约 560 万条。整个过程分三步：

1. **累积与 spill**：posting 不断塞进 arena，塞满了就把里面的数据按（trigram, file_id）排序，编码后写成磁盘上的临时段文件（`seg-00000.bin`、`seg-00001.bin`...），然后 arena 清空接着塞。
2. **段内编码**：段文件里存的不是原始 posting。排好序之后，相邻的 trigram 只存与前一个的差值，file_id 同理，差值再用变长整数编码（这就是 delta + varint）：排序保证了差值非负，而小差值占的字节少，所以段文件比 6 字节一条的原始格式省大约一半。
3. **多路归并**：所有文件处理完之后，手里是一批各自有序的段文件。归并时用一个小顶堆，每个段先读出自己的第一个 trigram 进堆；每次弹出堆里最小的 trigram，把它的 posting 写进最终索引，再从同一个段补读下一个进堆，直到所有段读空。

第三步就是 k-way merge。因为每个段各自有序，全局弹出的顺序天然就是 trigram 升序，所以可以边合并边流式写入 `index.bin`；每写出一个 trigram 的完整 posting list，也就知道了它在 `index.bin` 里的 offset 和 length，顺手写一条 `lookup.bin` 的记录。

拿前面的迷你索引走一遍。假装 arena 只能装 4 条 posting，10 条 posting 会 spill 出三个段：

```text
arena 批次            排序后写入段文件
[a.txt 的前 4 条]     seg-00000.bin:  " ca"(f0) "e c"(f0) "he "(f0) "the"(f0)
[接下来的 4 条]       seg-00001.bin:  "at "(f1) "cat"(f0) "cat"(f1) "t d"(f1)
[最后 2 条]           seg-00002.bin:  " do"(f1) "dog"(f1)
```

归并时堆里放的是每个段的当前首元素，每次弹出最小的 trigram：

```text
堆: " ca"(段0)  "at "(段1)  " do"(段2)
弹出 " ca" -> 写入，段0 补进 "e c"        堆: " do" / "at " / "e c"
弹出 " do" -> 写入，段2 补进 "dog"        堆: "at " / "e c" / "dog"
弹出 "at " -> 写入，段1 补进 "cat"(f0)    堆: "cat" / "e c" / "dog"
弹出 "cat" -> 写入 f0 这条，段1 补进 "cat"(f1)，堆顶还是 "cat"
弹出 "cat" -> 写入 f1 这条，段1 补进 "t d"  堆: "dog" / "e c" / "t d"
... 直到三个段都读空
```

还有两个细节值得提一下。堆里遇到相同 trigram 时按段号决定先后顺序，而段是按 file_id 的顺序 spill 的（file_id 按目录遍历顺序分配），所以同一 trigram 在各段里的 posting 拼接起来天然是 file_id 升序，写入前不用再排一次序；代码里还留了一个防御性的 fallback，发现顺序被破坏时会补一次排序。另外归并阶段 arena 已经释放，原来的预算转给所有段共享的读缓冲，所以峰值内存始终是那块预算，和仓库规模无关。

如果仓库小到 arena 从来没塞满过，一次 spill 都不会发生，直接排序后写索引，行为和 InMemory 完全一致，小仓库不为这个默认值付出任何代价。



### 增量构建

tgrep 不只有全量构建，还提供了增量能力：

- `build_index_for_files`：给定文件列表，不做目录遍历，建 delta 索引
- `merge_index_with_delta`：将 delta 与旧索引二路合并，支持文件替换和删除
- `append_overlay_to_index`：仅追加新文件，旧索引的 posting 字节从 mmap 中原样拷贝，不做解码

前面 `extract_indexed_file` 中计算的 `ContentId` 在这里发挥作用：它是解码后字节的 16 字节截断 BLAKE3 哈希，带域分隔前缀（domain separation prefix），随索引持久化。server 发现文件 mtime 变化并重读文件后，会重新提取并计算 `ContentId`；如果和持久化的值相同，说明解码内容其实没有变，那就跳过 overlay 提交，让磁盘索引中已有的 posting 继续生效。

这些增量能力在 server 模式下被 file watcher 触发，用来保持索引的近实时更新。查询侧对应地分成了两层：下层是 mmap 的磁盘索引，上层是一个内存中的 `LiveIndex` overlay。watcher 报告的变更先写入 overlay，`HybridIndex` 在查询时把两层的 posting 合并，并过滤掉已被覆盖或删除的旧条目。overlay 中的变更随后在后台通过上面提到的 merge/append 路径批量落盘、替换 reader。所以在 server 模式下，单次查询看到的是一个近实时的一致视图。



## 查询：从 Pattern 到候选文件

查询的核心在 `query.rs`，它负责把用户输入的 pattern（正则表达式或字面量字符串）转化为 trigram 查询计划，然后在索引上执行。

### QueryPlan 树

```rust
// tgrep-core/src/query.rs
pub enum QueryPlan {
    /// 所有 trigram 都必须匹配（posting list 交集）
    And(Vec<TrigramQuery>),
    /// 任一分支匹配即可（结果并集）
    Or(Vec<Self>),
    /// 无法提取 trigram，必须全文件扫描
    MatchAll,
}
```

每个 `TrigramQuery` 除了 trigram hash，还带了一个 `expected_next`：它是解析出的字面量中紧跟在该 trigram 之后的字节，用来在查询时做 `next_mask` Bloom 过滤。对正则来说，它取自语法树里提取出的字面量，可能和 pattern 原文不同。



### 字面量查询

字面量查询最直接：对 pattern 的字节序列做滑动窗口，每个窗口产生一个 `TrigramQuery`，所有 trigram 用 AND 组合：

```rust
// tgrep-core/src/query.rs
fn literals_to_query_plan(bytes: &[u8]) -> QueryPlan {
    if bytes.len() < 3 {
        return QueryPlan::MatchAll;
    }
    let queries: Vec<TrigramQuery> = (0..bytes.len() - 2)
        .map(|i| {
            let hash = trigram::hash(bytes[i], bytes[i + 1], bytes[i + 2]);
            let expected_next = if i + 3 < bytes.len() {
                Some(bytes[i + 3])
            } else {
                None
            };
            TrigramQuery { hash, expected_next }
        })
        .collect();
    QueryPlan::And(queries)
}
```

也就是说，搜索 `fn parse_config` 会产生一系列 trigram（`"fn "`, `"n p"`, `" pa"`, `"par"`, ...），所有这些 trigram 的 posting list 做交集就是候选文件集合。pattern 越长，产生的 trigram 越多，交集越小，候选越精确。

如果 pattern 少于 3 字节（比如搜索 `ab`），一个 trigram 都提取不出来，返回 `MatchAll`，调用方需要对所有文件做全文搜索



### 正则表达式查询

字面量查询之所以简单，是因为 pattern 本身就是一段确定的字节。正则表达式麻烦的地方在于它既有确定的部分（字面量），也有不确定的部分（`.`、`\d`、`x*`）。但大多数正则里仍然藏着"任何匹配都必须包含的字节片段"，查询计划的构造就是把这些片段找出来。

以 `foo.bar|qux` 为例。匹配左分支的文本一定包含 `foo` 和 `bar`（`.` 是什么不确定，但它两侧是确定的）；匹配右分支的文本一定包含 `qux`。所以候选文件是"同时含 `foo`、`bar` 的文件"与"含 `qux` 的文件"的并集，写成计划树就是 `Or(And[foo 的 trigram, bar 的 trigram], And[qux 的 trigram])`。

实现上，`regex-syntax` 先把 pattern 解析成语法树（HIR），`decompose_hir` 递归遍历这棵树，让每个节点回答同一个问题：匹配这段的文本，一定包含哪些连续字节？答案只有三种：一组 trigram（AND）、若干种可能（OR）、答不上来（MatchAll）：



| 节点类型           | 例子      | 回答                                              |
| ------------------ | --------- | ------------------------------------------------- |
| 字面量 `Literal`   | `foo`     | 它的全部 trigram（AND）                           |
| 连接 `Concat`      | `foo.bar` | 各段字面量的 trigram 合并（AND）                  |
| 分支 `Alternation` | `foo\|bar` | 各分支的答案取 OR；任一分支答不上来，整体答不上来 |
| 重复 `Repetition`  | `colou?r` | `min ≥ 1` 就问子节点；`min = 0` 则当它不存在      |
| 捕获组 `Capture`   | `(foo)`   | 直接问子节点                                      |
| 字符类、断言等     | `\d`、`^` | 答不上来（MatchAll）                              |

答不上来不等于放弃整个查询。在 Concat 里，MatchAll 的节点只是不贡献 trigram，两边的字面量段照常工作。`colou?r` 就是这样：可选的 `u` 没有贡献，但前面的 `colo` 是任何匹配都必含的，用它的 trigram（`col`、`olo`）就能粗筛。只有当整个 pattern 找不出一段必含的、长度 ≥3 的字节时（比如 `\d+`），计划才整体退化为 `MatchAll`，等于不筛。



有两个细节值得展开。

第一个和忽略大小写有关。忽略大小写搜索 `alert` 的时候，`Alert`、`ALERT` 这些写法都应该命中。`regex-syntax` 的做法是把每个有大小写变体的字母展开成一个字符类：字符类就是 `[...]`，表示这个位置可以是括号里的任意一个字符。所以 `(?i)Alert` 的语法树大致是 `[Aa][Ll][Ee][Rr][Tt]`：第一个位置是 `A` 或 `a`，第二个位置是 `L` 或 `l`，五个位置各有两种选择，一共 32 种写法。


问题在于，字符类在上表里是"答不上来"的那一档：它只说这个位置可以二选一，给不出任何必含的字节。五个字符类连在一起，整段字面量一个 trigram 都提取不出来，好好的 `Alert` 就退化成全量扫描了。


`Concat` 分支因此多做一步：遇到字符类时先尝试用 `ascii_class_literal` 把它折叠回一个字节，条件是这个类的所有成员都是 ASCII，并且小写化之后是同一个字节。`[Aa]` 满足条件，折成 `a`；五个类都折完就拿到字面量 `alert`，再提取 trigram，正好对上索引里存的小写 trigram。

但不是所有类都折得了。`[KkK]` 就不行：这里的第三个 `K` 其实不是 ASCII 的 K，而是 U+212A Kelvin 符号，只是长得一模一样，光是"必须是 ASCII"这条就把它卡掉了。如果硬把它折成 `k`，等于断言匹配里一定含有 `k` 这个字节，但 Kelvin 符号在文件里的实际编码是 UTF-8 的 `0xE2 0x84 0xAA` 三个字节，里面一个 `k` 都没有，包含它的文件会被漏掉，所以只能断开字面量段。



第二个在 `simplify` 阶段。这个阶段会对计划做去重，把相同的 trigram 合并成一个。但同一个 trigram 可能在 pattern 里出现多次，后随的字节却不同。`mutex.*mutex_lock` 里的 `tex` 就是这样：第一次出现在字面量段末尾，后面是什么不确定（没有 `expected_next`）；第二次后面跟着 `_`。去重时发现同一个 hash 带了两个不同的 `expected_next`，就把保留项的 `expected_next` 清空：这个 trigram 出现在多个上下文里，没法确定该拿哪个字节去查 `next_mask`，不过滤总比误过滤安全。



另外，传入多个 pattern（`-e`/`-f`）时，`build_multi_pattern_plan` 把每个 pattern 的计划用 OR 联合：文件匹配任一 pattern 即命中。和 Alternation 一样，任一 pattern 不可索引（`MatchAll`）则整体退化。



### PCRE 模式：放宽约束以保住索引

`-P` 的 pattern 可能用到 lookaround、反向引用这类结构，`regex-syntax` 解析不了，上面的语法树方法直接不可用。朴素的做法是放弃索引、全量扫描。tgrep 多做了一步：先用一个逐字符的扫描器把 pattern 改写成 `regex-syntax` 能解析的形式（`relax_for_indexing`），再从改写结果里提取 trigram。

改写能成立，靠的是只动那些不影响"必含字节"的部分。比如 `(?<!//)ExchangePrincipal`，它匹配 `ExchangePrincipal`，但要求前面不是 `//`（排除注释）。断言 `(?<!//)` 只是加了一个限制条件；删掉它，所有真实命中照样包含 `ExchangePrincipal` 这个字符串，只是会多匹配一些注释里的出现。多匹配没关系，候选集变大而已，最后由支持 PCRE 语法的回溯引擎（`fancy-regex`）逐文件验证时自然会排除。于是这个 pattern 依然能用索引粗筛，而不是扫描整个仓库。

改写规则就两类：

- **删**：四种零宽断言（lookahead/lookbehind）：`(?=...)`、`(?!...)`、`(?<=...)`、`(?<!...)`，删掉一个断言只是去掉一个约束；
- **换**：原子组 `(?>...)` 改成普通非捕获组 `(?:...)`，原子组只是禁止回溯，换掉它匹配范围只大不小。

剩下的结构不改写，直接放弃索引：反向引用（`\1`–`\9`、`\k`、`\g`、`(?P=name)`，引用的内容取决于前文实际匹配了什么，静态无法确定）、`\K`、`\G`、条件组、注释组 `(?#...)`、占有量词，以及改变词法规则的 `x` 模式。

底线是改写只能让匹配变多，不能变少。一旦变少，某个真实命中就可能不再含有我们提取的 trigram，文件会被索引误删。占有量词(Possessive Quantifiers) 放弃优化的原因就在这里：要删掉 `a{2,3}+` 末尾那个表示"占有"的 `+`，得先确认 `{2,3}` 真是量词；但 `a{x}+b` 里 `{x}` 是字面量花括号，那个 `+` 的意思是"一个或多个 `}`"，删掉它匹配就变少了。扫描器分不清这两种 `}`，索性遇到疑似占有量词都放弃。

放宽失败也不是错误，只是退化为 `MatchAll`，回到全量扫描



### 执行查询

有了 `QueryPlan`，执行就是对每个 trigram 查索引拿到 posting list，然后做集合运算：

```rust
// tgrep-core/src/query.rs
pub fn execute_plan<F>(plan: &QueryPlan, lookup: &F) -> Vec<u32>
where
    F: Fn(TrigramHash) -> Vec<u32>,
{
    match plan {
        QueryPlan::And(queries) => {
            let mut lists: Vec<Vec<u32>> = queries.iter()
                .map(|q| lookup(q.hash)).collect();
            // 按 posting list 长度升序排序，从最小的开始交集
            lists.sort_by_key(|l| l.len());

            let mut result: Vec<u32> = lists.remove(0);
            result.sort_unstable();
            result.dedup();

            for mut list in lists {
                list.sort_unstable();
                list.dedup();
                result = intersect_sorted(&result, &list);
                if result.is_empty() {
                    break;  // 提前退出
                }
            }
            result
        }
        QueryPlan::Or(plans) => {
            let lists = plans.iter()
                .map(|sub| execute_plan(sub, lookup)).collect();
            union_many_sorted(lists)
        }
        QueryPlan::MatchAll => Vec::new(), // 调用方需处理：扫描所有文件
    }
}
```

这里有两个关键的优化：

1. **Smallest-first 交集**：将 posting list 按长度升序排列，从最短的开始做交集。第一轮交集的结果就是一个很小的集合，后续每轮交集的输入规模都很小。一旦某轮交集结果为空，直接 `break`。
2. **有序双指针交集/并集**：`intersect_sorted` 和 `union_sorted` 都是 O(n+m) 的双指针算法。



顺带说明 `MatchAll` 在调用方的含义：候选集合直接取索引内的全部 file_id（`reader.all_file_ids()`），再照常进入正则验证。有些查询模式天然和 trigram 粗筛不兼容，会被强制走这条路：`-v` 反转匹配（它反转的是行，含有 pattern 的文件只要存在不匹配的行就仍要输出，所以"包含 pattern 的文件"这个候选集恰好是错误的过滤器）、`--passthru`（需要访问所有文件），以及非默认 `--encoding`（重新解码后的文本不是索引时看到的文本）



### 带 Mask 的查询执行

`execute_plan_with_masks` 是更精细的版本，它利用 `next_mask` 在交集过程中做额外过滤：

```rust
// tgrep-core/src/query.rs（简化）
// 对每个候选 file_id，检查 posting 中的 next_mask 是否包含预期的下一个字节
if let Some(next_byte) = query.expected_next {
    let bit = trigram::bloom_hash(next_byte);
    candidates.retain(|&(_, _, nm)| nm & bit != 0);
}
```

因为 `next_mask` 是一个 8 位 Bloom 过滤器，单次检查的假阳性率取决于该 trigram 在文件中后续字节的种类数，种类越少，对应的 bit 越少，过滤越精确。对于一个由 N 个 trigram 组成的 AND 查询，每个 trigram 都独立做一次 Bloom 检查，多轮过滤叠加可以显著压缩候选集。



## 索引读取

索引的磁盘读取在 `reader.rs` 里。

### Mmap Zero-Copy

`lookup.bin` 和 `index.bin` 使用 **mmap**（`memmap2`）映射到内存，实现 zero-copy 访问：

```rust
// tgrep-core/src/reader.rs
pub struct IndexReader {
    lookup: Option<Mmap>,
    postings: Option<Mmap>,
    file_paths: Vec<String>,
    // ...
    num_entries: usize,
}
```

`files.bin` 则用标准 `std::fs::read` 整文件读入，因为需要解码成 `Vec<String>` 供路径查找使用



### 二分查找 Posting List

查询一个 trigram 的 posting list，就是在按 trigram hash 排序的 `lookup.bin` 上做二分查找：

```rust
// tgrep-core/src/reader.rs
fn binary_search(&self, trigram: u32) -> Option<usize> {
    let mut lo = 0usize;
    let mut hi = self.num_entries;
    while lo < hi {
        let mid = lo + (hi - lo) / 2;
        let entry = self.read_lookup_entry(mid);
        match entry.trigram.cmp(&trigram) {
            std::cmp::Ordering::Equal => return Some(mid),
            std::cmp::Ordering::Less => lo = mid + 1,
            std::cmp::Ordering::Greater => hi = mid,
        }
    }
    None
}
```

找到位置后，读取 `LookupEntry` 中的 `offset` 和 `length`，直接从 mmap 中的 `index.bin` 区域切片并解码 posting entries。整个过程没有堆分配（除了最终的 `Vec<PostingEntry>` 结果），查找复杂度是 O(log N)，其中 N 是索引中 unique trigram 的数量。

对于 builder 的 merge 场景，`nth_trigram_raw` 甚至可以返回 `&[u8]` 直接指向 mmap，连 posting 的解码都省了，原始字节直接拷贝到新索引文件中。





## 搜索全流程

最后我们把上面的模块串起来，看一次 trigram-indexed 搜索的完整流程：

```text
用户输入 pattern
  -> query: build_multi_pattern_plan / build_relaxed_multi_pattern_plan
       解析 pattern（PCRE 先放宽），提取 trigram，构建 QueryPlan
       如果计划为 MatchAll：候选 = 索引内全部文件，直接进入验证阶段
  -> query: execute_plan_with_masks(plan, |hash| reader.lookup_trigram_with_masks(hash))
       对每个 trigram:
         reader: binary_search(lookup.bin) -> offset/length
         reader: read_posting_entries(index.bin[offset..offset+length])
         （server 模式下由 HybridIndex 合并磁盘索引与内存 overlay 两层结果）
       对所有 posting list:
         smallest-first 交集 + next_mask Bloom 过滤
  -> 候选 file_id 列表（可能有少量假阳性）
  -> 映射回路径，先限定在搜索根目录下，再应用 glob、--type、--max-depth 这些过滤
  -> 读取文件内容，用正则引擎做最终匹配验证
  -> 输出匹配结果
```

Trigram 索引的角色是**粗筛**：用极低的代价（一次二分查找 + 几次有序数组交集）把搜索范围从全部文件缩小到可能包含 pattern 的少量文件，然后只对这些候选文件跑正则匹配。对于一个包含几十万文件的仓库，这通常意味着只需要对几十个甚至几个文件做实际的正则扫描。

最后我用前面那份迷你索引跑了两条查询，直接观察两阶段搜索的行为：

```bash
./target/debug/tgrep -F -- "cat" /tmp/tgrep-demo
# a.txt:the cat
# b.txt:cat dog

./target/debug/tgrep -F -- "the c" /tmp/tgrep-demo
# a.txt:the cat
```

第一条只查了 `cat` 一个 trigram，它的 posting list 长度是 2，两个文件全部命中；第二条会产生 `the`、`he `、`e c` 三个 trigram，b.txt 一个都不含，交集直接收敛到 a.txt，整个过程中 b.txt 从未被打开过。



## 小结

- Trigram 是字节级的 3-byte 滑动窗口，打包到 `u32`，零碰撞
- 索引构建时同时提取原始和 ASCII 小写两套 trigram，通过就地折叠避免分配文件的小写副本
- 每个 posting 附带 `loc_mask`（位置信息，已落盘但查询未启用）和 `next_mask`（8 位 Bloom 过滤器，查询时用于减少假阳性）
- 磁盘格式由三个无压缩的小端二进制文件组成（`lookup.bin` 排序查找表、`index.bin` posting lists、`files.bin` 路径表），支持 mmap zero-copy 读取
- 索引构建使用 64 MiB 批预算 + rayon 并行 + 外部排序，峰值内存与仓库规模解耦；`meta.json` 先写"未完成"后写"完成"，兼作崩溃标记与发布标记
- 查询时用 `regex-syntax` 解析正则提取字面量段的 trigram，构建 AND/OR 查询计划树；PCRE 模式先放宽为匹配语言的超集再提取，保证不丢命中
- 执行采用 smallest-first 交集策略 + next_mask Bloom 过滤，双指针 O(n+m) 合并；无法提取 trigram 的计划（`MatchAll`）退化为全量候选
- server 模式下查询由 `HybridIndex` 合并磁盘索引与内存 overlay 两层结果，保持近实时
- 整体搜索是两阶段的：trigram 索引粗筛候选文件 -> 正则引擎精确匹配



## Reference

- [microsoft/tgrep](https://github.com/microsoft/tgrep)
- [Regular Expression Matching with a Trigram Index  or  How Google Code Search Worked](https://swtch.com/~rsc/regexp/regexp4.html)
- [Trigram indexing - Wikipedia](https://en.wikipedia.org/wiki/Trigram_search)
- [ripgrep](https://github.com/BurntSushi/ripgrep)
