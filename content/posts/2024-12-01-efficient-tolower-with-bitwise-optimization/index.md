+++
title = "字符串 toLower 的一种优化"
summary = ""
description = ""
categories = ["code-optimization"]
tags = ["string-processing", "bitwise", "performance"]
date = 2024-12-01T00:37:29+09:00
draft = false

+++

[2022-06-27 – tolower() in bulk at speed](https://dotat.at/@/2022-06-27-tolower-swar.html) 一文的阅读笔记。承接上文 [如何快速寻找第一个非 ASCII 字符位置](https://blog.dreamfever.me/posts/2024-11-24-find-first-non-ascii-fastly/)，如果我们知道一个字符串是纯 ASCII，那么我们可以在字符串处理上做一些更多的优化

这里先放一张 ASCII 表，方便查阅

![USASCII_code_chart](./USASCII_code_chart.svg)



对于大写字符其 ASCII 值小于对应的小写字符，并且差值为 `0x20`



## `toLower  `的实现



核心思路还是批处理和位标记。我们来看一下实现的细节，函数如下

```c
uint64_t tolower8(uint64_t octets) {
    uint64_t all_bytes = 0x0101010101010101;
    uint64_t heptets = octets & (0x7F * all_bytes);

    uint64_t is_gt_Z = heptets + (0x7F - 'Z') * all_bytes;
    uint64_t is_ge_A = heptets + (0x80 - 'A') * all_bytes;
    uint64_t is_ascii = ~octets & (0x80 * all_bytes);

    uint64_t is_upper = is_ascii & (is_ge_A ^ is_gt_Z);
    return (octets | is_upper >> 2);
}
```



`uint64_t all_bytes = 0x0101010101010101;`这个是一个辅助掩码，通过乘法运算可以将单字节扩展到 8 字节。举个例子，比如 `(0x7F - 'Z') * all_bytes`

```
(0x7F - 'Z') = 0x25
0x25 * 0x0101010101010101 = 0x2525252525252525
```

编译阶段涉及到 `all_bytes` 的乘法运算会直接折叠成常量

```c#
// uint64_t heptets = octets & (0x7F * all_bytes);

uint64_t heptets = octets & 0x7F7F7F7F7F7F7F7F
```

`heptets` 对于每个字节和 `0x7F` 进行与运算，将最高有效位置为 `0`。方便在之后的逻辑中最高有效位用来作为标记位使用。



我们来关注一下 `is_gt_Z` 和 `is_ge_A` 两个变量的用途

```c
uint64_t is_gt_Z = heptets + (0x7F - 'Z') * all_bytes;
uint64_t is_ge_A = heptets + (0x80 - 'A') * all_bytes;
```

这里是利用了位标记的技巧。ASCII 的最大范围是 `0x7F`，即 `0111_1111`，最高位有效位是 `0`。如果我们向任一个字节加上 `(0x7F - 'Z')`，使最高有效位变为 `1`，那么说明大于 `Z`。如下

```
A = 0x41
Z = 0x5A
[ = 0x5B

(0x7F - 'Z') = 0x25

0x41 + 0x25 = 0x66 = 0110_0110
0x5A + 0x25 = 0x7F = 0111_1111
0x5B + 0x25 = 0x80 = 1000_0000
```



所以我们现在通过着两个变量可以确定一个范围

- `is_gt_Z`: 最高有效位为 `1` 的字节为大于 `Z` 的字符
- `is_ge_A`: 最高有效位为 `1` 的字节为大于 `A` 的字符



通过位反操作，然后和 `(0x80 * all_bytes)` 进行与运算，这样可以通过最高有效位筛选出原始字符串中的 ASCII 部分。

```c
uint64_t is_ascii = ~octets & (0x80 * all_bytes);
uint64_t is_upper = is_ascii & (is_ge_A ^ is_gt_Z);
```



对于任何满足 `is_gt_Z` 的字符也会满足 `is_ge_A`；对于任何不满足 `is_ge_A` 的字符，也不会满足 `is_gt_Z`。因此这里可以通过异或运算来筛选出处于大写区间的字符，即 `(is_ge_A ^ is_gt_Z)`。在这个区间里面的字符，最高有效位均为 `1`。因此 `is_ascii` 和 `(is_ge_A ^ is_gt_Z)` 进行与运算，结果 `is_upper` 中的所有最高有效位为 `1` 的字节均为大写字符



最后一步，我们将大写字符转换成小写。在 ASCII 中，大写字母和其所对应小写字母的差值为 `0x20` 即 `0010_0000`。为了获得转换值，我们需要将 `0x80` 标志位移动到 `0x20` 位置。最后的或运算用于实现加法

```c
uint64_t to_lower = (is_upper >> 2) & (0x20 * all_bytes);
return (octets | to_lower);
```



以 `aA` 为例，每一步的计算结果如下

|                         |            |                     |
| ----------------------- | ---------- | ------------------- |
|                         | 十六进制值 | 二进制值            |
| **原始值**              | `0x6141`   | `01100001 01000001` |
| **heptets**             | `0x6141`   | `01100001 01000001` |
| **is_gt_Z**             | `0x8666`   | `10000110 01100110` |
| **is_ge_A**             | `0xa080`   | `10100000 10000000` |
| **(is_ge_A ^ is_gt_Z)** | `0x26e6`   | `00100110 11100110` |
| **is_ascii**            | `0x8080`   | `10000000 10000000` |
| **is_upper**            | `0x0080`   | `00000000 10000000` |
| **to_lower**            | `0x0020`   | `00000000 00100000` |



## `toUpper` 的实现

根据上面的思路，可以依葫芦画瓢实现一个 `toUpper`，完整代码如下

```c
#include <ctype.h>
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

#define MAX_STRING_LENGTH 10000
#define TEST_CASES 100000

#define VECTOR_MASK 0x0101010101010101ull
#define ASCII_MASK 0x8080808080808080ull

static inline uint64_t toupper8(uint64_t octets)
{
    uint64_t heptets = octets & (0x7F * VECTOR_MASK);

    uint64_t is_gt_z = heptets + (0x7f - 'z') * VECTOR_MASK;
    uint64_t is_ge_a = heptets + (0x80 - 'a') * VECTOR_MASK;

    uint64_t is_ascii = ~octets & (0x80 * VECTOR_MASK);
    uint64_t is_lower = is_ascii & (is_ge_a ^ is_gt_z);
    return octets & ~(is_lower >> 2);
}

size_t ya_toupper(const char* input, size_t length, char* output)
{
    const char* p = input;
    char* out_p = output;
    const char* end = input + length;

    while (p <= end - 8) {
        uint64_t octets = *(const uint64_t*)p;
        uint64_t result = toupper8(octets);
        *(uint64_t*)out_p = result;
        p += 8;
        out_p += 8;
    }

    uint64_t u = 0;
    size_t remaining = end - p;

    switch (remaining) {
    default:
        u |= (uint64_t)(unsigned char)p[7] << 56ull;
    case 7:
        u |= (uint64_t)(unsigned char)p[6] << 48ull;
    case 6:
        u |= (uint64_t)(unsigned char)p[5] << 40ull;
    case 5:
        u |= (uint64_t)(unsigned char)p[4] << 32ull;
    case 4:
        u |= (uint64_t)(unsigned char)p[3] << 24ull;
    case 3:
        u |= (uint64_t)(unsigned char)p[2] << 16ull;
    case 2:
        u |= (uint64_t)(unsigned char)p[1] << 8ull;
    case 1:
        u |= (uint64_t)(unsigned char)p[0];
        break;
    case 0:
        break;
    }

    if (remaining > 0) {
        uint64_t result = toupper8(u);

        for (size_t i = 0; i < remaining; ++i) {
            out_p[i] = (char)(result & 0xFF);
            result >>= 8;
        }
        out_p += remaining;
    }

    return (size_t)(out_p - output);
}

int main()
{
    srand((unsigned int)time(NULL));

    char input[MAX_STRING_LENGTH + 1];
    char expected[MAX_STRING_LENGTH + 1];
    char actual[MAX_STRING_LENGTH + 1];

    for (int i = 0; i < TEST_CASES; ++i) {
        size_t length = rand() % MAX_STRING_LENGTH + 1;

        for (size_t j = 0; j < length; ++j) {
            char rand_value = (char)(rand() % 256);
            input[j] = (rand_value);
        }
        input[length] = '\0';

        for (size_t j = 0; j < length; ++j) {
            expected[j] = toupper((unsigned char)input[j]);
        }
        expected[length] = '\0';

        size_t actual_length = ya_toupper(input, length, actual);
        actual[actual_length] = '\0';

        if (strcmp(expected, actual) != 0) {
            printf("Test case %d failed\n", i + 1);
            printf("Length  : %zd\n", length);

            printf("Input   : ");
            for (size_t j = 0; j < length; ++j) {
                printf("%02X ", (unsigned char)input[j]);
            }
            printf("\n");

            printf("Expected: ");
            for (size_t j = 0; j < length; ++j) {
                printf("%02X ", (unsigned char)expected[j]);
            }
            printf("\n");

            printf("Actual  : ");
            for (size_t j = 0; j < length; ++j) {
                printf("%02X ", (unsigned char)actual[j]);
            }
            printf("\n");

            return 1;
        }
    }

    printf("All test cases passed!\n");
    return 0;
}
```



## Reference

- [2022-06-27 – tolower() in bulk at speed](https://dotat.at/@/2022-06-27-tolower-swar.html)
