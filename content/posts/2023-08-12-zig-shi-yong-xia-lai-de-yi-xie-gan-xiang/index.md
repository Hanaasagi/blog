
+++
title = "Zig 使用下来的一些感想"
summary = ''
description = ""
categories = []
tags = []
date = 2023-08-12T11:31:21+08:00
draft = false
+++

2023-08-04，Zig 终于 release 了 0.11 的正式版本，这个版本距离 0.10 已经过了半年。但是鉴于目前用 Zig 的大多数项目都是直接使用 Nightly 的，也就没感觉到什么大的差别。本文写于 2023-07，大部分代码都是在 Zig 0.11 下的，所以看的时候注意一下时效性



## 为什么有了 Rust，还需要 Zig

在当下 Rust 已经如此强势的时代，这个问题一定避不开的。关于这个问题 Zig 的 Website 上也有专门的文章 [Why Zig When There is Already C++, D, and Rust?](https://ziglang.org/learn/why_zig_rust_d_cpp/) 来说明，但是这篇文章中给出的几个理由个人感觉没啥说服力。Zig 相较于其他语言，最大的卖点还是在于 comptime 上，不过目前 zig 的 comptime 并不完善，写代码过程中编译器在有些条件下会 crash，参考 [issue #12373](https://github.com/ziglang/zig/issues/12373)。另外在实际的使用过程中，我发现好像也不需要十分复杂的 comptime 过程 😰



想了半天，我没法给出很好的理由来回答。不过 Rust 的复杂程度是众所周知的，而且 Rust 在 FFI 这种情况下，编译器无法发挥优势，还需要用户来编写额外的代码，这也算使用 Zig 的几个理由吧。当然 Rust 有缺点也不意味着要使用 Zig，选择一个自己用着顺手的就可以了



## Zig 的优点

这个 Google 一下应该有一堆文章来赞美 Zig，所以这里就不提了。如果想了解一下 Zig 的设计哲学，可以看一下 `zig zen`

```
 * Communicate intent precisely.
 * Edge cases matter.
 * Favor reading code over writing code.
 * Only one obvious way to do things.
 * Runtime crashes are better than bugs.
 * Compile errors are better than runtime crashes.
 * Incremental improvements.
 * Avoid local maximums.
 * Reduce the amount one must remember.
 * Focus on code rather than style.
 * Resource allocation may fail; resource deallocation must succeed.
 * Memory is a resource.
 * Together we serve the users.
```



## Zig 的缺点

说一下自己在实际使用 Zig 中遇到的一些问题:



#### Lazy compilation

这个新手十有八九会踩坑，在 Zig 中编译是惰性的。比如下面这样的代码

```
const aaa = @import("ssss");
```

这个 `import` 了一个不存在的模块，但是只要 `aaa` 变量没有被用到，那么编译就不会报错。除了语法错误外，从 `main` 函数出发，任何没有被引用到的代码，都是不会被参与编译的，所以你可能写了一堆 bug 在 codebase 里面，但是因为没有编译进来，所以没有察觉到，例如

```c
const T = struct {
    fn f1() void {
        std.debug.print("f1\n", .{});
    }
    fn f2() void {
        std.debug.print2222("f2\n", .{});
    }
};
```



#### Name shadowing

Zig 由于 comptime 的存在，导致 name scope 和其他语言不一样。比如一下代码会报错

```c
const T = struct {
    fn name() void {}

    fn setName(name: []const u8) void {}
};


// src/main.zig:7:16: error: function parameter shadows declaration of 'name'
//    fn setName(name: []const u8) void {}

```

`setName` 的参数 `name` 和函数 `name` 冲突了



#### Interface

Zig 没有一个显式的 interface 语法，很多都是通过 `anytype` 然后在 comptime 进行静态分发，比如 stdlib 中的 reader 和 writer。我个人非常讨厌这种，因为对于一个类型写着 `anytype` 的参数，我没法一下子明白到底可以传入什么类型，实在是过于隐式了。而且由于 comptime 的存在，这就使一切 `antype` 然后通过 `@hasField` 这种动态检查成为可能，你可以像写 Python 一下写 Zig。有一个 Lib [interface.zig](https://github.com/alexnask/interface.zig)  帮助编写更加清晰的 interface 结构，但是作者已经去世了



#### Copy

这个也是大坑，在 `var a = xxx` 的时候会发生 Copy，你如果想要修改原来的值，那么需要 `var a = &xxx` 才行。参考 [Beware the copy!](https://zig.news/gowind/beware-the-copy-32l4) 和 [Pass-by-value-Parameters](https://ziglang.org/documentation/master/#Pass-by-value-Parameters)。Copy 也不只是发生在这里，比如下面这样

```c
const std = @import("std");

const T = struct {
    a: u32,
};

fn testFn() !T {
    var t = try std.heap.page_allocator.create(T);
    t.a = 10;
    return t.*;
}

test "simple test" {
    var t = try testFn();
    try std.testing.expect(t.a == 10);
    std.heap.page_allocator.destroy(&t);
}

```

返回 `T` 的时候发生了一次 Copy，所以这个测试会在执行 `destory` 的时候 panic



再举一个例子

```c
//         children: std.ArrayList(Tuple(&[_]type{ isize, Node(T) })),
for (n.children.items) |item| {
    const d = item[0];
    if (try std.math.absInt((d - distance)) <= self.max_dist) {
        try self.candidates.pushBack(&item[1]);
    }
}

```

这里也会出现问题，我们需要使用 `*item` 而不是 `item`



#### 类型系统

Zig 并没有实现像 Haskell 或者 Rust 那样严谨的代数类型系统，比如

```c
fn name() u32 {
    @panic("");
}

```

一般来说这个返回类型应该是 never type，但是 Zig 允许此函数通过编译



类型推断目前也不完善

```c
fn sum() usize {
    var s = 0;
    for (0..10) |i| {
        s += i;
    }
    return s;
}

// src/main.zig:6:13: error: variable of type 'comptime_int' must be const or comptime
//         var s = 0;
//             ^
// src/main.zig:6:13: note: to modify this variable at runtime, it must be given an explicit fixed-size number type
```

这里无法根据返回值的类型推断出 `s` 的类型应该为 `usize`，比需要手动标注出来 `        var s: usize = 0;`



分支如果判断 `null` 后，依然需要通过 `s.?`

```c
fn sum() usize {
    var s: ?usize = 0;
    if (s != null) {
        return s;
    }
    return 0;
}
// src/main.zig:4:16: error: expected type 'usize', found '?usize'
//        return s;

```

这里经过 `if` 分支判断后，分支里面 `s` 的类型应该是 `usizie` 了。不过这个可以通过

```c
if (value) |v| {
    return v;
}
```

这种编程范式来解决，但是由于 name scope 的原因，我们又需要多想一个变量的名字了。而且关于这个语法

```c
self.deserializeInto(&value) catch |_| {
    return null;
};
```

像这样的是不能写 `_` 来忽略 capture 的，比如写成 `self.deserializeInto(&value) catch return null;` 的形式，这个就很奇怪



#### Allocator

Zig 的设计就是显式的内存分配。基本上你作为一个 Lib，那么你需要提供参数让调用者来决定使用什么 allocator，这里很容易出现 allocator 在代码中间传来传去的情况，写起来很麻烦。你可以通过 comptime 或者像 Rust 那样 once-cell 的思路来一个全局变量解决问题。但是有时候我们修改代码，一个原来不需要 alloc 的函数，现在需要一个 ArrayList 结构来辅助，这就导致函数的签名也要改变，因为 alloc 的话就需要处理 error，通常是向调用方抛出



有多少情况你需要捕获 out of memory 呢，或者 out of memory 之后你又能做什么呢？这些只会让代码写起来很难受。看一下 Bun 的代码，涉及 alloc 的函数都是用的 global 的 minialloc，然后直接 `catch unreachable` 来处理错误。这样真的有意义么？



另外因为不存在自动的作用域结束 deallocate 机制，我们需要手动 `defer`，这个导致我们没法写一些链式调用的代码



#### Unittest Framework

Zig 的内置 `test` 功能在收集 `std.debug` 的输出时候有问题

1. 只有在 test failed 的时候才会输出 stdout
2. stdout 和 test 本身输出的信息混在一起，难以辨识
3. 无法自动收集 codebase 中的所有 test case，需要手动这样来引用一下

```c
test {
    std.testing.refAllDecls(@This());
}
```



#### ZLS

这个用过你就知道了我想说什么了



#### Package Manager

没有一个统一的包管理器，这个是硬伤。[zigmod](https://github.com/nektro/zigmod) 和 [gyro](https://github.com/mattnite/gyro) 两个包管理器使用了不同的配置文件。而且 Zig 在 0.11 搞了一个内置的，但是还很初级，你可以通过 `build.zig.zon` 来指定依赖，比如

```
.{
    .name = "slugify",
    .version = "0.1.0",
    .dependencies = .{
       .deunicode = .{
           .url = "https://github.com/dying-will-bullet/deunicode/archive/refs/tags/v0.1.1.tar.gz",
           .hash = "1220fef06e2fab740b409eaec28fee459526c86f297e6c43fdaee471084cc569f397",
       },
    },
}

```

但这个现在是不支持 Git 协议的，你需要指定一个 `tar.gz` 的 URL。而且 GitHub 在 Web 端进行 Release 的时候是不会将 submodule 加入到 `tar.gz` 的，所以导致这个功能是个残疾

#### Documentation

相关资料比较少，比如 `std.meta` 的文档基本出于缺失状态，需要通过看代码和单元测试来理解。Zig 的 Reddit 之前也因为抗议的原因把板块 private 了，导致将近半个月你去搜索 Zig 相关问题的时候，下面的 Reddit 回答都是需要通过 Google 快照来访问的。不过 Reddit 现在已经可以正常浏览


大致就是以上这样，Zig 依然属于一个开发中的语言，根据作者当初承诺的伟大目标，我觉得离 1.0 可能还要至少 2 年。而且社区生态也很重要，虽然 Zig 可以直接缝合已经有的 C 库，比如像 [zap](https://github.com/zigzap/zap) 做的那样，但是对于 C 和 C++ 的高手来说，迁移到  Zig 又能带来多少回报呢？从现在 AI 的发展速度来看，我觉得以后也是令人担忧的。因为编程就是人类逻辑到机器指令的转换，像 Rust 这样需要复杂的信息标注其实就是编译器不能完全理解我们的意图，说不定以后自然就理解了🥲
    