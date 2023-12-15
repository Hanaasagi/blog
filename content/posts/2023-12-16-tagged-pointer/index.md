+++
title = "Tagged Pointer"
summary = ""
description = ""
categories = [""]
tags = []
date = 2023-12-16T00:00:00+09:00
draft = false

+++



## 环境信息

- Zig 0.12.0-dev.1819+5c1428ea9 
- Linux 6.6.7-arch1-1 x86_64 unknown



本文介绍  Tagged Pointer，一种常见的优化手段，以前看到很多人这样写过。但我最近第一次用，所以记录一下



##  字节对齐

先来看一下字节对齐，以 `struct` 为例。对齐本身是方便 CPU 进行访问，让 CPU 可以在一次读取内读到所有的数据，不必进行拼接。有些操作本身就要求对齐，比如 `Atomic(usize)` 这样的

### packed struct

```zig
const IV = packed struct {
    foo: u32,
    bar: usize,
    baz: u32,
};


// @sizeOf(IV) == 16
// @alignOf(IV) == 8
// &iv.foo == u32@7fff8fca82e0
// &iv.bar == usize@7fff8fca82e4
// &iv.baz == u32@7fff8fca82ec
```



- `7fff8fca82e0` 地址开始的 4 个字节存放了 `u32`
- `7fff8fca82e4` 地址开始的 8 个字节存放了 `usize`
- `7fff8fca82ec` 地址开始的 4 个字节存放了  `u32`



需要注意中间的这个 `usize` 是横跨了两个 8 字节对齐空间的，他的一部分在 `7fff8fca82e4` 到 `7fff8fca82e8` 另一部分在 `7fff8fca82e8` 到 `7fff8fca82ec` 中，像下图这样

```
低地址							rbp  高地址
+-------------------------------+
|  u32  |     usize     |  u32  |
+-------------------------------+
```



我们需要使用 `qword ptr [rbp - 12]` 这样的来访问，这样是 OK 的。但是如果是

```zig
const IV = packed struct {
    foo: u4,
    bar: usize,
    baz: u32,
};
```



这里的 `u4` 会导致访问 `bar` 的时候需要读取 2 次

1. 在 `foo` 的地址读取一个 `qword`，然后通过位运算截取后 60 bit 的空间
2. 在 `baz` 的地址处读取一个 `byte` 然后截取前 4 个 bit
3. 拼接



这是一个示例，具体看编译器



另外如果我们在代码的其他地方对其进行 Atomic 相关的操作，可以看到 Zig 会直接编译错误，因为 align 不符合要求

```
src/main.zig:12:28: error: expected type '*const usize', found '*align(4) const usize'
    _ = @atomicLoad(usize, &iv.bar, .SeqCst);
                           ^~~~~~~
src/main.zig:12:28: note: pointer alignment '4' cannot cast into pointer alignment '8'
```



### normal struct



```zig
const IV = struct {
    foo: u32,
    bar: usize,
    baz: u32,
};

// @sizeOf(IV) == 16
// @alignOf(IV) == 8
// &iv.foo == u32@7ffd365b5a88
// &iv.bar == usize@7ffd365b5a80
// &iv.baz == u32@7ffd365b5a8c
```



未显式要求内存布局，那么这个行为可能是未定义的，编译器可以对此进行优化。目前版本的 Zig 试了一下 Debug/ReleaseFast/ReleaseSafe 都是这样的布局



- `7ffd365b5a80` 地址开始的 8 个字节存放了 `usize`
- `7ffd365b5a88` 地址开始的 4 个字节存放了 `u32`
- `7ffd365b5a8c` 地址开始的 4 个字节存放了 `u32`



可以看到结构体的字段是被重新排列过了，`usize` 被放到了前面的空间。通过把 `alignOf` 大的成员排在前面，按照 `alignOf` 降序的顺序声明成员，可以将 padding 减少到最小。此外当结构体中有很多字段的时候，需要考虑一下 Cacheline 的，将经常一起访问的数据放到相邻的地方会好一些。通过 `packed struct` 强制顺序，然后布局则手动加上 `padding` 字段，可以防止编译器对这个做优化



### extern struct

和 C ABI 相同的内存布局



```zig
const IV = extern struct {
    foo: u32,
    bar: usize,
    baz: u32,
};

// @sizeOf(IV) == 24
// @alignOf(IV) == 8
// &iv.foo == u32@7fff6b125ad0
// &iv.bar == usize@7fff6b125ad8
// &iv.baz == u32@7fff6b125ae0
```



这里结构体中的每个类型都被强制的进行 8 字节对齐，填充了 padding



### custom align

```zig
const IV = struct {
    foo: u32,
    bar: usize align(4),
    baz: u32,
};

// @sizeOf(IV) == 16
// @alignOf(IV) == 4
// &iv.foo == u32@7ffc8217bd70
// &iv.bar == usize@7ffc8217bd74
// &iv.baz == u32@7ffc8217bd7c
```



这里显式指定了对齐字节数，导致和 `packed struct` 的布局相同



## Tagged Pointer



观察一下上面的地址，我们可以发现通过指定字节对齐可以让地址的数值成为 `N` 的整数倍。比如 8 字节对齐，那么十六进制地址值的最低位只有 `0` 和 `8` 两种可能性。那么意味着有 3 个 bit 的空间是可以让我们自由发挥

```
0 => 0000
1 => 0001
2 => 0010
...
8 => 1000
...
F => 1111
```



同理，如果是按照 4 字节对齐的，则有 2 个 bit 的空间可以使用。



那么可以通过这个来做什么呢，常见的用法比如：

- 用这几个 bit 来表达指针所指向的数据的类型。比如`0001` 来表示用户自定义的类型对象，`0010` 表示 list 对象，`0011` 表示字符串类型对象等等，信息量可以达到 14 种
- CPython 中 GC 的一个小优化，利用这几个 bit 来打标记。具体参考这个文章 [Garbage collector design](https://devguide.python.org/internals/garbage-collector/#optimization-reusing-fields-to-save-memory)
- 将数值本身存储在指针中。比如像 Python 这样的整数对象，对于非常大的数字使用堆上的内存空间表示，对于小的数字直接使用这个指针本身，这样小的数字也不需要经过 GC 了。但是据我之前所看到的代码，CPython 应该都是 `PyLongObject`，加了一个 256 的对象池这样的



当然这个也不是没有代价。如果我们使用了这种技术，那么对于这个指针我们在有些情况下是无法进行直接寻址的，需要将低 3 位归零后才能寻址



## Example

最近正好在 K/V database 中用到了，我这边使用最后一个 bit 来作为 tag，如果为 1 那么代表值存放在指针本身中，如果为 0 那么表示这就是一个普通的指针。对于小于等于 7 个字节的字符串直接存储在指针的值中，比如 `ABCDEFG` 这个字符串，然后将长度左移后与 1 进行或运算反倒最后一个字节中



``` 
A              0011
A B C D  E F   1101
A B C D  E F G 1111
```



如果长度大于 7 了，那么将指针指向堆上的字符串。这里使用一个定长的 Header 后面跟上字符串的内容。考虑到我这里的使用场景

- 定长数据
- 减小重复的内存占用



这个地方我是用了一个 RC 的

```zig
const Header = struct {
    rc: std.atomic.Value(usize),
    len: usize,
};
```



全部的代码如下

```zig
const std = @import("std");
const testing = std.testing;

const SIZE: usize = @sizeOf(usize);
const MAX: usize = SIZE - 1;

const Header = struct {
    rc: std.atomic.Value(usize),
    len: usize,
};

pub const IV = struct {
    _data: [SIZE]u8 align(8),

    const Self = @This();

    pub fn deinit(self: *Self, allocator: std.mem.Allocator) void {
        if (!self.is_inline()) {
            const header = self.deref_header();
            const rc = header.rc.fetchSub(1, .Release) - 1;
            if (rc == 0) {
                allocator.free(self.remote_ptr()[0 .. @sizeOf(Header) + header.len]);
            }
        }
    }

    pub fn from_bytes(
        allocator: std.mem.Allocator,
        slice: []const u8,
    ) !IV {
        var data: [SIZE]u8 = std.mem.zeroes([SIZE]u8);

        if (slice.len <= MAX) {
            data[SIZE - 1] = @as(u8, @intCast(slice.len)) << 1 | 1;
            @memcpy(data[0..slice.len], slice);
        } else {
            const header = Header{
                .rc = std.atomic.Value(usize).init(1),
                .len = slice.len,
            };

            const total = slice.len + @sizeOf(Header);
            // TODO: align alloc?
            var buf = try allocator.alloc(u8, total);

            const bytes align(8) = std.mem.asBytes(&header);
            @memcpy(buf[0..bytes.len], bytes);

            @memcpy(buf[bytes.len..total], slice);
            std.mem.writeInt(usize, &data, @intFromPtr(buf.ptr), .little);
        }

        return .{
            ._data = data,
        };
    }

    pub fn to_bytes(self: *Self) []const u8 {
        if (self.is_inline()) {
            return self._data[0..self.inline_len()];
        } else {
            const header = self.deref_header();
            const start = @sizeOf(Header);

            return self.remote_ptr()[start .. start + header.len];
        }
    }

    pub fn eql(self: *Self, other: *Self) bool {
        return std.mem.eql(u8, self.to_bytes(), other.to_bytes());
    }

    pub fn clone(self: *Self) Self {
        if (!self.is_inline()) {
            _ = self.deref_header().rc.fetchAdd(1, .Monotonic);
        }
        return .{
            ._data = self._data,
        };
    }

    pub inline fn is_inline(self: *Self) bool {
        return self.trailer() & 1 == 1;
    }

    pub inline fn trailer(self: *Self) u8 {
        return self._data[SIZE - 1];
    }

    pub inline fn inline_len(self: *Self) usize {
        return @intCast(self.trailer() >> 1);
    }

    pub inline fn remote_ptr(self: *Self) [*]const u8 {
        std.debug.assert(!self.is_inline());
        const ptr = std.mem.readInt(usize, &self._data, .little);
        return @ptrFromInt(ptr);
    }

    pub inline fn deref_header(self: *Self) *Header {
        std.debug.assert(!self.is_inline());
        return @constCast(@ptrCast(@alignCast(self.remote_ptr())));
    }
};

```



这里是直接指向了 `Header + string` 这个结构的 `Header` 位置。如果有别的需求，比如扩展到 C，可以将指针直接指向 String 的位置(String 需要 `\0` 结尾)，然后通过减去偏移量来获取 `Header`。这个就有点类似 Redis 的 SDS 数据结构了，指针直接指向的 C 字符串



## 扩展阅读

- [字节对齐与填充（Data Alignment and Padding In C）](https://zhuanlan.zhihu.com/p/598708950)

- Bun 中的 Tagged Pointer https://github.com/oven-sh/bun/blob/bun-v1.0.18/src/tagged_pointer.zig

- [主流语言内存管理笔记](https://gist.github.com/wangyingsm/0eebdfe41889efb12c346ae7852d8302)

- [CPython Garbage collector design](https://devguide.python.org/internals/garbage-collector/)

  
