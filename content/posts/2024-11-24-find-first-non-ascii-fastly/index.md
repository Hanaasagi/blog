+++
title = "如何快速寻找第一个非 ASCII 字符位置"
summary = ""
description = ""
categories = ["code-optimization"]
tags = ["string-processing", "bitwise", "performance"]
date = 2024-11-24T15:02:37+09:00
draft = false

+++



最近阅读到一篇技术文章  [小さい文字列からASCII外のバイトを見つけたい](https://methane.hatenablog.jp/entry/2024/10/28/162612)，正好以前也看到过 Bun 代码里面也有类似的实现，所以记录一下这个技巧



原文代码在处理 0 宽字符串的时候会有问题，这里是修改后的版本

```c
#include <stddef.h>


#define ASCII_MASK 0x8080808080808080ull

size_t find_first_non_ascii(const char* start, const char* end)
{
    const char* p = start;

    while (p <= end - 8) {
        size_t x = *(size_t*)p & ASCII_MASK;
        if (x) {
            // p[0] がASCII外の時、 ctzll(0x80) == 7, (7-7)/8 == 0
            // p[1] がASCII外の時、 ctzll(0x8000) == 15, (15-7)/8 == 1
            return p - start + (__builtin_ctzll(x) - 7) / 8;
        }
        p += 8;
    }

    size_t u = 0;
    switch (end - p) {
    default:
        u |= (size_t)(p[7]) << 56ull;
    case 7:
        u |= (size_t)(p[6]) << 48ull;
    case 6:
        u |= (size_t)(p[5]) << 40ull;
    case 5:
        u |= (size_t)(p[4]) << 32ull;
    case 4:
        u |= (size_t)(p[3]) << 24;
    case 3:
        u |= (size_t)(p[2]) << 16;
    case 2:
        u |= (size_t)(p[1]) << 8;
    case 1:
        u |= (size_t)(p[0]);
        break;
    case 0:
        return end - start;
    }

    if (u & ASCII_MASK) {
        return p - start + (__builtin_ctzll(u & ASCII_MASK) - 7) / 8;
    }

    return end - start;
}
```



理解这段代码首先要复习一下 ASCII 和 UTF-8 的编码知识



- ASCII 字符由 7 位二进制表示，范围是 `0x00` 到 `0x7F`。特点是：

  - **最高位（MSB）为 `0`**。这一性质使我们可以直接通过最高位判断是否为 ASCII 字符



- UTF-8 是一种变长编码标准，用于表示 Unicode 字符，参考 [UTF-8](https://en.wikipedia.org/wiki/UTF-8) 具有以下特点：

  - **兼容 ASCII**：UTF-8 对 ASCII 字符（范围 `0x00` 到 `0x7F`）采用单字节表示，最高位始终是 `0`（如 `01111111` 表示 `0x7F`）。

  - **非 ASCII 字符**：超出 ASCII 范围的字符使用多字节表示：

    - **2 字节**：以 `110xxxxx 10xxxxxx` 开头
    - **3 字节**：以 `1110xxxx 10xxxxxx 10xxxxxx` 开头
    - **4 字节**：以 `11110xxx 10xxxxxx 10xxxxxx 10xxxxxx` 开头

    

  

由此我们可以理解为什么 `ASCII_MASK` 使用 `0x8080808080808080` 的原因:

`0x80` 即 `10000000` 该掩码在每个字节的最高位设置为 `1`。当将 64 位的字符串片段与该掩码进行按位与运算时：

- **ASCII 字节**：最高位为 `0`，结果为 `0`。
- **非 ASCII 字节**：最高位为 `1`，结果为非零。



这样我们可以在 64bit 系统下，将字符串按 8 个字节划分，转换成 `u64` 类型。然后通过这个掩码高效地判断出来是否存在非 ASCII 字符。如果存在则我们需要寻找第一个字符的起始位置



- `__builtin_ctzll`：计算 `x` 的二进制表示中从右到左的连续 `0` 的个数，这个值定位了最高位为 `1` 的位置
- 减去 `7` 并除以 `8`，将位偏移转换为字节偏移



关于计算 `__builtin_ctzll` 的实现，最近正好也读到一篇文章  [【Go 语言编程珠玑】deBruijn 序列快速计算 64 位二进制数末尾多少个 0](https://grzhan.tech/2024/11/11/GoTrailingZero/) 介绍了使用 de Bruijn 序列来实现 Golang 的 `TrailingZeros64` 的方式，这里也提一下



如果我们的内存分配是按 8 字节对齐且字符串也是 8 字节对齐的，那么即使越界访问字符串尾部之后的几个字节也是不会出现段错误的。但是在没有对齐的情况下，我们需要小心处理以免越界读取。这里使用了一个 fall through 的 `switch` 语句，通过逐字节移位累积到 `u` 中，然后用相同的掩码逻辑检测非 ASCII 字节



Bun 中有类似的函数 `firstNonASCIIWithType` 完成相同的功能在 https://github.com/oven-sh/bun/blob/ff4eccc3b40cf53166c9d2c4188af0d66b021ae0/src/string_immutable.zig#L4019。相比上面的 C 代码，此函数有一个 SIMD 的优化，核心思路还是批处理



## Reference

- [小さい文字列からASCII外のバイトを見つけたい](https://methane.hatenablog.jp/entry/2024/10/28/162612)

- [bun.js source code](https://github.com/oven-sh/bun/blob/ff4eccc3b40cf53166c9d2c4188af0d66b021ae0/src/string_immutable.zig#L4019)

- [【Go 语言编程珠玑】deBruijn 序列快速计算 64 位二进制数末尾多少个 0](https://grzhan.tech/2024/11/11/GoTrailingZero/) 

- [神奇的德布鲁因序列](https://halfrost.com/go_s2_de_bruijn/)
