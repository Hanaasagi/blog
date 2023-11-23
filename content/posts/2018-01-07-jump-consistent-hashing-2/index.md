
+++
title = "Jump Consistent Hashing"
summary = ''
description = ""
categories = []
tags = []
date = 2018-01-07T05:38:37+08:00
draft = false
+++

*本文是 [A Fast, Minimal Memory, Consistent Hash Algorithm](https://arxiv.org/ftp/arxiv/papers/1406/1406.2294.pdf) 的阅读笔记*

### 概述

Jump Consistent Hash 是一致性哈希的一种实现，仅用 `5` 行代码便可实现

```C++
#include <iostream>
#include <cstdint>
#include <boost/range/irange.hpp>
#include <boost/format.hpp>

int32_t JumpConsistentHash(uint64_t key, int32_t num_buckets)
{
    int64_t b = -1, j = 0;
    while (j < num_buckets) {
        b = j;1
        key = key * 2862933555777941757ULL + 1;
        j = (b + 1) * (double(1LL << 31) / double((key >> 33) + 1));
    }
    return b;
}

int main(void)
{
    int32_t total = 4;
    for (auto i: boost::irange(0, 32)) {
        int32_t before = JumpConsistentHash(i, total);
        int32_t after = JumpConsistentHash(i, total+1);
        std::cout<<boost::format("%d  %d") % before % after;
        if(before != after) {
            std::cout<<" changed";
        }
        std::cout<<std::endl;
    }
    return 0;
}
```

### 使用限制

>Both of these algorithms(指 Karger 算法和 rendezvous 算法) allow buckets to have arbitrary ids, and handle not only new buckets
being added, but also arbitrary buckets being removed. This ability to add or remove buckets in any order can be valuable for cache servers where the servers are purely a performance improvement. But for data storage applications, where each bucket represents a different shard of the data, it is not acceptable for shards to simply disappear, because that shard is only place where the corresponding data is stored. Typically this is handled by either making the shards redundant (with several replicas), or being able to quickly recover a replacement, or accepting lower availability for some data. Server death thus does not cause reallocation of data; only capacity changes do. This means that shards can be assigned numerical ids in increasing order as they are added, so that the active bucket ids always fill the range [0, num_buckets).

>Only two of the four properties of consistent hashing described in the Karger et al. paper are important for data storage applications. These are **balance**, which essentially states that objects are evenly distributed among buckets, and **monotonicity**(单调性), which says that when the number of buckets is increased, objects move only from old buckets to new buckets, thus doing no unnecessary rearrangement. Their other two properties, spread and load, both measure the behavior of the hash function under the assumption that each client may see a different arbitrary subset of the buckets. Under our data storage model this cannot happen, because all clients see the same set of buckets [0, num_buckets).  This restriction enables jump consistent hash.

根据叙述可以得知此算法并不适用于存在节点宕机/删除的情况，需要保证区间 `[0, num_buckets]` 中皆为可用节点，且必须顺序编号。

### 算法分析

假设算法为 `ch(key, num_buckets)`，返回应当存储的 bucket 编号。则应当对于任意的 `key` 都有 `ch(key, 1) == 0` 成立，因为只有一个 bucket。当增加至两个 bucket 时，理想情况下应当有一半的数据进行迁移以保证 balance。以此类推 `ch(key, n)` 到 `ch(key, n+1)` 时应当有 `1/(n+1)` 的数据迁移到编号为 `n` 的 bucket，其余的 `n/(n+1)` 保持不动

一种将 `key` 作为随机数 seed 的实现：

```C++
int ch(int key, int num_buckets) {
    random.seed(key) ;
    int b = 0; // This will track ch(key, j +1) .
    for (int j = 1; j < num_buckets; j ++) {
        if (random.next() < 1.0/(j+1) ) b = j ;
    }
    return b;
}
```

由于计算机所实现的随机数是伪随机数，所以有 `ch(key, n) == ch(key, n)` 恒成立。另外此伪随机数满足在 `[0, 1)` 区间上均匀分布的条件。所以可以认为 `ch(key, n+1)` 的情况下，若随机数的值落在 `[0, 1/(n+1))` 区间，则应当进行迁移

上面的算法是 `O(n)` 的，下面给出一种 `log(n)` 的实现：

```C++
int ch(int key, int num_buckets) {
    random. seed(key) ;
    int b = -1; //  bucket number before the previous jump
    int j = 0; // bucket number before the current jump
    while(j<num_buckets){
        b=j;
        double r=random.next(); //  0<r<1.0
        j = floor( (b+1) /r);
    }
    return b;
}
```

根据 `O(n)` 算法可以看出 `ch(key, n+1)` 的结果在 `n` 增加时有相当大的概率不会改变(`n` 为最大的 bucket 编号，`n+1` 为 bucket 的数量)。假设上一次得出的 bucket 编号为 `b`，即有 `ch(k, b) ≠ ch(k, b+1), ch(k, b+1) = b`，随着 `n` 的不断增长存在一个最小的 `n` 使得其结果发生改变，不妨这个 `n` 记为 `j`。存在如下的式子成立 `ch(k, j+1) ≠ ch(k, b+1)`，`ch(k, j) = ch(k, b+1)`

为了获得 `j` 上的概率约束，对于任意的 bucket 编号 `i` 来说，我们有 `j ≥ i` 当且仅当 `ch` 的结果未发生改变。即当且仅当 `ch(k, i) = ch(k, b+1)`。所以满足如下的式子

```
P(j ≥ i) = P( ch(k, i) = ch(k, b+1) )
```

理想情况下，`P( ch(k, 10) = ch(k, 11) )` 的概率应当为 `10/11`，`P( ch(k, 11) = ch(k, 12) )` 的概率应当为 `11/12`, 那么可以得出 `P( ch(k, 10) = ch(k, 12) )` 的概率应当为 `10/11 * 11/12 = 10/12`，即连续多次不迁移的概率的乘积。由此可以得出下面的式子

```
P(j ≥ i) = P( ch(k, i) = ch(k, b+1) ) = (b+1) / i
```

现在我们生成一个依赖于 `k` 和 `j` 的伪随机变量 `r`，其在 `[0, 1]` 区间均匀分布。规定当且仅当 `r ≤ (b+1) / i` 时有 `j ≥ i`。解此不等式可得 `i ≤ (b+1) / r`，这样就得到了 `i` 的上界是 `(b+1) / r`，由于对任意的 `i` 都要有 `j>=i` 成立，因此 `j = floor((b+1) / r)`

本文最开始的代码是使用 `64` 位线性同余随机数生成器进行优化过的，所以看起来有点奇怪

### 性能

见 [A Fast, Minimal Memory, Consistent Hash Algorithm](https://arxiv.org/ftp/arxiv/papers/1406/1406.2294.pdf) 原文中的图表

### Reference
[A Fast, Minimal Memory, Consistent Hash Algorithm](https://arxiv.org/ftp/arxiv/papers/1406/1406.2294.pdf)

    