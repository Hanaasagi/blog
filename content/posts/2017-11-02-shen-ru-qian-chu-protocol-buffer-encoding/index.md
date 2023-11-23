
+++
title = "深入浅出 Protocol Buffer Encoding"
summary = ''
description = ""
categories = []
tags = []
date = 2017-11-02T13:49:24+08:00
draft = false
+++

*阅读本文就是在浪费时间，因为你根本不会用到这些 (ﾟ⊿ﾟ)*

本文基于 `proto3`，所以和官方文档会有一定的出入

### 引例

先来写一个 `proto3` 的文件

```
// test1.proto
syntax = "proto3";
package test1;

message Test1 {
  int32 a = 1;
}
```

我这里直接使用 `grpc` 来生成

```Bash
python -m grpc_tools.protoc -I ./ --python_out=. ./test1.proto
```

目录下会多一个个 `.py` 文件

```Bash
(py36) sagiri ➜  cc tree
.
├── test1_pb2.py
└── test1.proto
```

编写用于输出编码结果的应用程序，这里我们将 `a` 的值设为 `150`

```Python
# test1.py
import test1_pb2


t = test1_pb2.Test1()
t.a = 150
print(t.SerializeToString())
```

执行后输出如下

```
b'\x08\x96\x01'
```

如果你对十六进制敏感的话，`150 的 hex` 正是 `0x96`，而 `a` 的 tag 值为 `0x01`。对就是这样，你应该明白了什么

**NOTE 上面一行纯属胡扯**

### Base 128 Varints

为了理解 protocol buffer 的编码，你必须先了解 varints。Varints 是一种使用一个或多个字节去序列化整数的方法。整数的值越小，编码后占用的字节数也越小。这也就是推荐常用的 field 要使用值较小的 tag 的原因

编码后的数据除了最后一个字节外，每一个字节的最高有效位用于标记是否还有后续的数据。如果设为 `1`，则代表下一个字节仍然是当前整数的组成部分；如果为 `0`，则下一个字节是新的数据。低 `7` 字节用于存储整数的二进制补码，先存储整数的低字节

对于 `1` 来说，它只需要占用一个字节，所以不需要设置最高有效位

```
0000 0001
```

再来看看 `300` (二进制为 `1 0010 1100`)

```
1010 1100 0000 0010
// 去除最高有效位后
 010 1100  000 0010
```

由于是先存储的整数的低字节，所以将反转分组的顺序拼接即可得源数据

```
010 1100 000 0010
000 0010 010 1100
即 100101100
```

现在你一定想了解字符串之类的存储方式，不过不要急，回来在看看开始的例子。既然仅使用低七位存储数据，那么我们认为 `\x96` 就是 `150` 显然是一个错误的想法。实际上 `150` 是 `\x96\x01` 一起表示的(Google 这例子真TM棒)

```
150 的二进制
1001 0110
按 7 bit 分组
1  0010110
补前缀
 000 0001  001 0110
先存储低位
 001 0110  000 0001
设置最高有效位
1001 0110 0000 0001
连接
1001011000000001
```

### Message Structure

protobuf buffer 消息是一系列的键值对(key-value)。消息的二进制表示使用了 field 的 tag 来作为 key。每个 field 的名称和声明类型只能在消息的解码端通过引用消息定义(`.proto` 文件)来获取

消息编码时，key 和 value 会被连接成字节流。消息解码时，解析器会跳过它不认识的 field。这样新的 field 可以被添加至消息中而不会破坏不了解它们的老程序。最后，key 实际上由两个值组成，`.proto` 文件中定义 field 时的 tag 和能够提供信息去获取值长度的 wire type

```
Type    Meaning	           Used For
0	      Varint	           int32, int64, uint32, uint64, sint32, sint64, bool, enum
1	      64-bit	           fixed64, sfixed64, double
2	      Length-delimited   string, bytes, embedded messages, packed repeated fields
3	      Start group	       groups (deprecated)
4	      End group	         groups   (deprecated)
5	      32-bit	           fixed32, sfixed32, float
```

实际上的 key 是这样计算的 `(field_number << 3) | wire_type`。所以最开始的示例中 `0x08` 才是 key 的值！这种计算方式的好处就是可以通过 key 的低三位来直接得出 wire type，`0x08=0000 1000`。将 key 右移 `3` 位可以得出 field tag 的值


### More Value Types

##### Signed Integers

有符号整数类型(`sint32` 和 `sint64`)和“标准”整数类型(`int32` 和 `int64`)编码负数时有着很大的不同。如果负数使用 `int32` 和 `int64`　类型，编码的结果总会长达十个字节。因为负数的补码表示，相当于一个数值很大的无符号整数。如果你使用有符号类型，结果会使用 ZigZag 编码

ZigZag 编码将有符号整数映射到无符号整数，因此绝对值小经过 varint 编码后值就会很小。它以一种通过正整数和负整数来回摆动的方式，使得 `-1` 被编码为 `1` ，`1` 被编码为 `2`，`-2` 被编码为 `3`，依此类推 `2147483647` 被编码为 `4294967294`，`-2147483648` 被编码为 `4294967295`

换句话说，就是下面的公式

```
// int32
(n << 1) ^ (n >> 31)
// int64
(n << 1) ^ (n >> 63)
```

需要注意的是 `>>` 是算术右移

可以通过实例对比一下

当 `a` 为 `sint32` 时 `b'\x08\x01'`  
当 `a` 为 `int32` 时 `b'\x08\xff\xff\xff\xff\xff\xff\xff\xff\xff\x01'`


##### Non-varint Numbers

非 varint 数值类型比较简单。wire 类型为 `1` 的 `double`，`fixed64` 使用固定的 `64 bit` 来表示，同样的 `float` 和 `fixed32`(wire type 为 `5`)使用固定的 `32 bit` 表示。他们都是 little-endian byte order

##### Strings

wire type 为 `2` 的类型会将其值的长度存储下来，有点像 TCP 数据包首部。用代码验证一下

```
// test2.proto
syntax = "proto3";
package test2;

message Test2 {
    string a = 2;
}


# test2.py
import test2_pb2


t = test2_pb2.Test2()
t.a = 'testing'
print(t.SerializeToString())
```

输出如下

```
b'\x12\x07testing'
```

对于 `0x12`(`10010`) 取低三位可得 wire type 为 `2`，将其右移三位可得 tag 为 `2`。第二个字节 `0x07` 表示数据长度为 `7`。所以后面的 `7` 个字节便是此 field 的值

那么字符串的长度如果超高 `0xFF` 会怎么样呢？

```Python
import test2_pb2


t = test2_pb2.Test2()
t.a = 's' * (0xFF + 1)
print(t.SerializeToString())
```

输出如下

```
b'\x12\x80\x02ss此处省略 n 个 s
```

`\x80\x02` 正是 `256` 经过 varint 编码后的结果

### Embedded Messages

```
// test3.proto
syntax = "proto3";
package test3;

message Embed {
    int32 a = 1;
}

message Test3 {
    Embed b = 1;
}

# test3.py
import test3_pb2


t = test3_pb2.Test3()
t.b.a = 150
print(t.SerializeToString())
```


输出如下

```
b'\n\x03\x08\x96\x01'
```

正如你所看到的，最后三个字节(`\x08\x96\x01`)与我们的第一个例子完全相同。而第一个字节 `\n` 实际上是 `0x0a`(`1010`) 即 wire type 为 `2`，tag 值为 `1`。根据 wire type 我们猜也猜得出来 `0x03` 是数据的长度。所以 Embedded Messages 和字符串编码策略相同


### Optional And Repeated Elements

在 `proto2` 中含有 `repeated` 的消息(没有 `packed=true`)，编码后会有 `0` 个或多个相同 tag 的键值对。这些重复的值不必连续出现；他们可能与其他字段交错出现。解析时元素之间的相对顺序会被保留，但是相对于其他 field 的顺序会丢失，而在 `proto3` 中，repeat field 使用 packed encoding

`proto3` 中的任何 non-repeated field，或者 `proto2` 中的 `optional` field，编码后的消息可能没有 tag 和对应的键值对

通常，编码后的消息不会出现多于一个的 non-repeated field 实例。然而，解析器将这种情况也考虑在内了。对于数值符类型和字符串，如果相同的 field 出现了多次，那么解析器会取最后一次的值。对于 embedded message field，解析器会合并相同 field 的多个实例，就像使用 `Message::MergeFrom` 方法一样(all singular scalar fields in the latter instance replace those in the former, singular embedded messages are merged, and repeated fields are concatenated)这些规则会令解析两个连续的编码消息和分别解析单个消息然后合并产生的结果相同

```
MyMessage message;
message.ParseFromString(str1 + str2);

is equivalent to this:

MyMessage message, message2;
message.ParseFromString(str1);
message2.ParseFromString(str2);
message.MergeFrom(message2);
```

如果我们不对字段进行赋值呢？借用一下最开始的示例代码

```
import test1_pb2


t = test1_pb2.Test1()
print(t.SerializeToString())
```

输出

```
b''
```

不进行赋值的 field 将会使用默认值。由于默认值是双方早已知晓的，所以并不会进行编码传输。比如上例中 `t.a = 0`，输出依然为 `b''`

### Packed Repeated Fields

在 `proto2` (version `2.1.0` 以上) 中需要使用 `[packed=true]` 才能使用 packed repeated fields。而在 `proto3` 中这已成为默认的行为。包含零个元素的 packed repeated field 不会出现在编码后的消息中。另外所有的元素会被打包成一个 wire type 为 `2` 的键值对。除了前面没有 tag 外，每个元素的编码方式与正常情况相同

测试代码如下

```
// test4.proto
syntax = "proto3";
package test4;

message Test4 {
  repeated int32 a = 1;
}


# test4.py
import test4_pb2


t = test4_pb2.Test4()
t.a.append(1)
t.a.append(2)
print(t.SerializeToString())
```

输出如下

```
b'\n\x02\x01\x02'
```

`\n` 我们之前已经分析过了，表示 wire type 为 `2`，tag 值为 `1`。根据 wire type 为 `2` 的定义来说，接下来的 `\x02` 便是长度。然后 `\x01\x02` 即是我们的数据

### Field Order

虽然在编写 `.proto` 文件时，可以以任意顺序设置 field 的 tag number，但是在序列化消息时会按照 tag number 的顺序进行编码

```
// test5.proto
syntax = "proto3";
package test5;

message Test5 {
    int32 a = 2;
    int32 b= 1;
    int32 c = 3;
}


# test5.py
import test5_pb2


t = test5_pb2.Test5()
t.c = 1
t.a = 1
t.b = 1
print(t.SerializeToString())
```

输出如下

```
b'\x08\x01\x10\x01\x18\x01'
```

```
In [4]: 0x08 >> 3
Out[4]: 1

In [5]: 0x10 >> 3
Out[5]: 2

In [6]: 0x18 >> 3
Out[6]: 3
```


### Reference
[Protocol-Buffers Encoding](https://developers.google.com/protocol-buffers/docs/encoding)

    