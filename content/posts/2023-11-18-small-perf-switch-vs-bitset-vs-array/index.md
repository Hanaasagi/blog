+++
title = '简单测试一下分支判断中的 switch/bitset/const array'
summary = 'This is a test page'
description = ""
categories = []
tags = []
date = 2023-11-18T15:09:03+09:00
draft = false

+++

这是一篇笔记，稍微记录一下自己测试的一些结果

### 环境信息

- zig version 0.12.0-dev.1604+caae40c21，编译 flag `-Doptimize=ReleaseFast`
- Linux 6.6.1-arch1-1 x86_64 unknown
- CPU : AMD Ryzen 7 5800H with Radeon Graphics
- CPU flags: `fpu vme de pse tsc msr pae mce cx8 apic sep mtrr pge mca cmov pat pse36 clflush mmx fxsr sse sse2 ht syscall nx mmxext fxsr_opt pdpe1gb rdtscp lm constant_tsc rep_good nopl nonstop_tsc cpuid extd_apicid aperfmperf rapl pni pclmulqdq monitor ssse3 fma cx16 sse4_1 sse4_2 movbe popcnt aes xsave avx f16c rdrand lahf_lm cmp_legacy svm extapic cr8_legacy abm sse4a misalignsse 3dnowprefetch osvw ibs skinit wdt tce topoext perfctr_core perfctr_nb bpext perfctr_llc mwaitx cpb cat_l3 cdp_l3 hw_pstate ssbd mba ibrs ibpb stibp vmmcall fsgsbase bmi1 avx2 smep bmi2 erms invpcid cqm rdt_a rdseed adx smap clflushopt clwb sha_ni xsaveopt xsavec xgetbv1 xsaves cqm_llc cqm_occup_llc cqm_mbm_total cqm_mbm_local user_shstk clzero irperf xsaveerptr rdpru wbnoinvd cppc arat npt lbrv svm_lock nrip_save tsc_scale vmcb_clean flushbyasid decodeassists pausefilter pfthreshold avic v_vmsave_vmload vgif v_spec_ctrl umip pku ospke vaes vpclmulqdq rdpid overflow_recov succor smca fsrm debug_swap`
- 测试集：10000 条随机 npm package name 数据，包含合法、非法的数据。所有 bench 均使用的同一份数据

### Switch / If branch

```zig
const std = @import("std");

pub fn isNPMPackageName(target: []const u8) bool {
    if (target.len == 0) return false;
    if (target.len > 214) return false;

    const scoped = switch (target[0]) {
        'A'...'Z', 'a'...'z', '0'...'9', '$', '-' => false,
        '@' => true,
        else => return false,
    };

    var slash_index: usize = 0;
    for (target[1..], 0..) |c, i| {
        switch (c) {
            'A'...'Z', 'a'...'z', '0'...'9', '-', '_', '.' => {},
            '/' => {
                if (!scoped) return false;
                if (slash_index > 0) return false;
                slash_index = i + 1;
            },
            '!', '~', '*', '\'', '(', ')' => {
                if (!scoped or slash_index > 0) return false;
            },
            else => return false,
        }
    }

    return !scoped or slash_index > 0 and slash_index + 1 < target.len;
}
```

`-Doptimize=ReleaseFast` 下的结果

```
>> hyperfine -r 10000 --warmup 2000  -N "./zig-out/bin/TTT"
Benchmark 1: ./zig-out/bin/TTT
  Time (mean ± σ):     785.9 µs ± 136.5 µs    [User: 670.8 µs, System: 40.9 µs]
  Range (min … max):   544.6 µs … 1775.2 µs    10000 runs
```

这里摘取部分的汇编代码

```assembly
(lldb) disassemble --name isNPMPackageName
; ...
TTT[0x20bb8f] <+15>:  ret
; ...
TTT[0x20bc57] <+215>: movabs rcx, 0x878200000000
TTT[0x20bc61] <+225>: xor    eax, eax
TTT[0x20bc63] <+227>: movzx  edx, byte ptr [rdi + rax + 0x1]
TTT[0x20bc68] <+232>: cmp    rdx, 0x2f
TTT[0x20bc6c] <+236>: ja     0x20bc78                  ; <+248> at main.zig:15:17
TTT[0x20bc6e] <+238>: bt     rcx, rdx
TTT[0x20bc72] <+242>: jb     0x20bb8f                  ; <+15> at main.zig
TTT[0x20bc78] <+248>: cmp    edx, 0x7e
TTT[0x20bc7b] <+251>: je     0x20bb8f                  ; <+15> at main.zig
TTT[0x20bc81] <+257>: cmp    dl, 0x5f
; ...
```

从汇编代码可以看到，这个函数已经被修改的面目全飞了。原来函数中的 `switch` 因为 `scoped` 会产生部分的合并，然后有一部分的ASCII 范围上的比较会被转换成 bitset ，比如 `0x878200000000` 这个值

```
In [89]: for i in range(0, 255):
    ...:     if biset & (1 << i) != 0:
    ...:         print(i, hex(i), chr(i))
    ...:
33 0x21 !
39 0x27 '
40 0x28 (
41 0x29 )
42 0x2a *
47 0x2f /

```

函数中的第二个 `for` 循环，对应 `movzx  edx, byte ptr [rdi + rax + 0x1]` 指令。这里将 `char` 赋值到了 `edx` 寄存器中。下一步则是比较了是否大于 `0x2f` => `/`，如果大于，如果大于那么跳转到 `0x20bc78`。这里会继续比较 `0x7e` => `~`，如果相等那么直接函数返回了；如果小于 `0x2f` 那么会通过 `bt     rcx, rdx` 来检测 `rdx` 所在的位是否为 1，我们可以通过上面的 Python 结果来看到，这里其实抠出来了几个在函数中用到的 ASCII 字符，如果不满足，那么直接返回。否则又重复一次 `0x7e` 的比较，但是这个应该永假的

`ReleaseFast` 编译后的这个逻辑流哈，感觉不知道在干些什么。。。🤯

### std.bit_set.IntegerBitSet

使用 std 中的 bitset，来看一下是否能够在不动用逻辑的前提下对函数进行加速

```zig
fn construct_bitset(chars: []const u8) std.bit_set.IntegerBitSet(255) {
    var bitset = std.bit_set.IntegerBitSet(std.math.maxInt(u8)).initEmpty();
    for (chars) |char| {
        bitset.set(@as(u8, char));
    }

    return bitset;
}
pub fn isNPMPackageName(target: []const u8) bool {
    if (target.len == 0) return false;
    if (target.len > 214) return false;

    const bset1 = comptime construct_bitset("ABCDEFGHIJKLMNOPQRSTUVWXYZ" ++
        "abcdefghijklmnopqrstuvwxyz" ++
        "0123456789" ++
        "$-");

    const bset2 = comptime construct_bitset("ABCDEFGHIJKLMNOPQRSTUVWXYZ" ++
        "abcdefghijklmnopqrstuvwxyz" ++
        "0123456789" ++
        "-_.");

    const bset3 = comptime construct_bitset("!~*'()");

    const scoped = blk: {
        if (bset1.isSet(target[0])) {
            break :blk false;
        } else if (target[0] == '@') {
            break :blk true;
        } else {
            return false;
        }
    };

    var slash_index: usize = 0;
    for (target[1..], 0..) |c, i| {
        if (bset2.isSet(c)) {
            continue;
        } else if (c == '/') {
            if (!scoped) return false;
            if (slash_index > 0) return false;
            slash_index = i + 1;
        } else if (bset3.isSet(c)) {
            if (!scoped or slash_index > 0) return false;
        } else {
            return false;
        }
    }

    return !scoped or slash_index > 0 and slash_index + 1 < target.len;
}
```

`-Doptimize=ReleaseFast` 下的结果

```
>> hyperfine -r 10000 --warmup 2000  -N "./zig-out/bin/TTT"
Benchmark 1: ./zig-out/bin/TTT
  Time (mean ± σ):     786.9 µs ± 134.8 µs    [User: 668.9 µs, System: 44.1 µs]
  Range (min … max):   559.0 µs … 1819.6 µs    10000 runs


```

zig 的 std 中的 `IntegerBitSet` 就是掩码加位移，没有针对于数据量的不同进行特别的优化

```zig
        /// Returns true if the bit at the specified index
        /// is present in the set, false otherwise.
        pub fn isSet(self: Self, index: usize) bool {
            // assert(index < bit_length);
            return (self.mask & maskBit(index)) != 0;
        }


```

使用 bitset 厚的执行流没有之前修改的那么夸张，能够清晰的看到我们之前出现 magic number，并且有位移相关的指令

```assembly
TTT[0x20bc8f] <+175>: movabs rcx, 0x7fffffe87fffffe
TTT[0x20bc99] <+185>: movabs rdx, 0x3ff600000000000

```

### 常量数组 [256]bool

最后看一下常量的 256 元素数组来表示 Latin1 范围，128 其实也可以表示。`bool` 可以认为是 `u1` 的但是实际上 `[256]u8` 并不是 32 字节的，后面会说明

```zig
fn construct_array(chars: []const u8) [256]bool {
    var array: [256]bool = undefined;
    @memset(array[0..], false);
    for (chars) |char| {
        array[(@as(u8, char))] = true;
    }

    return array;
}
pub fn isNPMPackageName(target: []const u8) bool {
    if (target.len == 0) return false;
    if (target.len > 214) return false;

    const array1 = comptime construct_array("ABCDEFGHIJKLMNOPQRSTUVWXYZ" ++
        "abcdefghijklmnopqrstuvwxyz" ++
        "0123456789" ++
        "$-");

    const array2 = comptime construct_array("ABCDEFGHIJKLMNOPQRSTUVWXYZ" ++
        "abcdefghijklmnopqrstuvwxyz" ++
        "0123456789" ++
        "-_.");

    const array3 = comptime construct_array("!~*'()");

    const scoped = blk: {
        if (array1[@as(u8, target[0])]) {
            break :blk false;
        } else if (target[0] == '@') {
            break :blk true;
        } else {
            return false;
        }
    };

    var slash_index: usize = 0;
    for (target[1..], 0..) |c, i| {
        if (array2[@as(u8, c)]) {
            continue;
        } else if (c == '/') {
            if (!scoped) return false;
            if (slash_index > 0) return false;
            slash_index = i + 1;
        } else if (array3[@as(u8, c)]) {
            if (!scoped or slash_index > 0) return false;
        } else {
            return false;
        }
    }

    return !scoped or slash_index > 0 and slash_index + 1 < target.len;
}

```

`-Doptimize=ReleaseFast` 下的结果

```
>> hyperfine -r 10000 --warmup 2000  -N "./zig-out/bin/TTT"
Benchmark 1: ./zig-out/bin/TTT
  Time (mean ± σ):     785.9 µs ± 149.8 µs    [User: 672.0 µs, System: 40.3 µs]
  Range (min … max):   560.7 µs … 2907.7 µs    10000 runs


```

这次生成的函数的汇编的指令数目是最小的，摘录其中一段

```assembly
TTT[0x20bec0] <+64>:  movzx  ecx, byte ptr [rdi + rax + 0x1]
TTT[0x20bec5] <+69>:  cmp    byte ptr [rcx + 0x202db0], 0x1
TTT[0x20becc] <+76>:  jne    0x20beda                  ; <+90> at main.zig
TTT[0x20bece] <+78>:  lea    rcx, [rax + 0x1]
TTT[0x20bed2] <+82>:  cmp    rsi, rax
TTT[0x20bed5] <+85>:  mov    rax, rcx
TTT[0x20bed8] <+88>:  jne    0x20bec0                  ; <+64> at main.zig:38:23
TTT[0x20beda] <+90>:  ret
```

此段汇编来源于第二个循环，这里 `0x202db0` 是 `[256]bool` 数组的首地址，可以看到区别于以前，这种方式使用的不是一个立即数。每次循环来判断的时候都是对内存单元的寻址，但是因为局部性原理，这里的消耗不会太大。我们也可以看到这里 `byte ptr` 然后直接比较的，所以其实每一个 `bool` 元素是按照 8 字节对齐的，`[256]u8` 的大小是 `256 * 8` 字节

### Debug 模式下的结果

#### switch / if branch

```
>> hyperfine -r 10000 --warmup 2000  -N "./zig-out/bin/TTT"
Benchmark 1: ./zig-out/bin/TTT
  Time (mean ± σ):      14.1 ms ±   0.4 ms    [User: 13.7 ms, System: 0.2 ms]
  Range (min … max):    13.2 ms …  17.5 ms    10000 runs


```

#### bitset

```
>> hyperfine -r 10000 --warmup 2000  -N "./zig-out/bin/TTT"
Benchmark 1: ./zig-out/bin/TTT
  Time (mean ± σ):      15.0 ms ±   0.4 ms    [User: 14.7 ms, System: 0.2 ms]
  Range (min … max):    14.1 ms …  20.7 ms    10000 runs

```

#### [256]bool

```
Benchmark 1: ./zig-out/bin/TTT
  Time (mean ± σ):      13.3 ms ±   0.4 ms    [User: 12.9 ms, System: 0.2 ms]
  Range (min … max):    12.5 ms …  20.4 ms    10000 runs

```

在 `Debug` 模式下可以看到常量数组是速度最快的，其次是 `switch` 。`bitset` 的效果不明显。体验下来在分支判断这方面，如果数据少的话还是直接进行 `switch` 摊开，如果数据量大且动态那么还是要使用 `bitset` 来解决的。常量数组在空间使用上有点大

结论只是个大概，具体问题具体分析
