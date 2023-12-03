+++
title = '新人也能懂的调试方法 05 - 实战 SIGSEGV'
summary = ""
description = ""
categories = ["debug"]
tags = []
date = 2023-12-03T16:00:00+09:00
draft = false

+++

## 序言

在上一个章节中介绍了 LLDB 的基本使用，因为最近正好 debug 了一个 `SIGSEGV` 的问题，所以作为一个章节来讲一下如何调试一个 `SIGSEGV` 的问题，问题参考 [bun#7209](https://github.com/oven-sh/bun/issues/7209)，复现环境可以 clone 这个 https://github.com/Hanaasagi/bun-crash-repro

## 正文

这个问题在 debug build 下是无法复现的，但是在 release build 却可以复现。这个就比较麻烦了，我们需要编译一份带有 symbol 的二进制，否则只能进行汇编层级的调试。对于这个项目可以通过 `bun build:release` 然后找到生成的 `bun-profile` 二进制来进行调试

在漫长的编译期间，翻看了一下源代码。这个是和 worker 相关的，它会涉及到多线程。在 worker 线程中请求了一个 URL 地址，然后拉回来一堆 IP 数据，通过 JS API 做了一个通信，将数据传回到 main 里了。最后打印这份数据，停止 worker

在没有看到具体的堆栈时，我们不妨先入为主的猜一下。`SIGSEGV` 是由于访问了非法的内存导致的，所以很容易联想到所有权问题。比如 worker 线程中的数据和 main 是 shared 的，但是被 worker GC 后，main 依然在引用，导致访问非法的内存。

但是经过多次运行，发现 main 线程打印数据的时候都是完整的。如果访问了非法的内存，那么这个可能是不完整的。不过在注释掉 `index.ts` 的 `console.log({ t });` 后，多次运行无法重现 `SIGSEGV`。我在这时还是推测和数据有关系，所以将数据成了空数组，问题又无法复现了。尝试逐步增大数据的量，在我本地发现在 500 多左右，开始不稳定的复现，600 之后变得越来越稳定。数据的阈值让人联想到 GC 算法的 threshold

不过这里我们还没有排除另外一个因素， `console.log` 它是一个 IO 操作，不光访问了这一堆数据。我尝试改成单纯的写文件，发现又无法复现这个问题了。那么再试一下重定向 stdout 看看， `bun run index.ts > /dev/null` 也无法这个问题

这真令人头大，只能等 LLDB 来调试了。使用 `bun-profile` 来执行代码后，让其 coredump，然后 `coredumpctl debug --debugger=lldb` 挂载

```
(lldb) bt
* thread #1, name = 'bun-profile', stop reason = signal SIGSEGV
  * frame #0: 0x0000563c395ff547 bun-profile`JSC__VM__notifyNeedTermination + 7
    frame #1: 0x0000563c384a16f0 bun-profile`WebWorker__requestTerminate at shimmer.zig:186:41
    frame #2: 0x0000563c384a16eb bun-profile`WebWorker__requestTerminate [inlined] src.bun.js.bindings.bindings.VM.notifyNeedTermination(vm=<unavailable>) at bindings.zig:5131:14
    frame #3: 0x0000563c384a16eb bun-profile`WebWorker__requestTerminate(this=<unavailable>) at web_worker.zig:317:41
    frame #4: 0x0000563c396c800b bun-profile`WebCore::jsWorkerPrototypeFunction_terminate(JSC::JSGlobalObject*, JSC::CallFrame*) + 235
    frame #5: 0x00007f46ace301b8
    frame #6: 0x0000563c393f8a71 bun-profile`js_trampoline_op_call_ignore_result + 23
    frame #7: 0x0000563c393f7c97 bun-profile`llint_op_call + 176
    frame #8: 0x0000563c393f7c97 bun-profile`llint_op_call + 176
    frame #9: 0x0000563c393f8a31 bun-profile`llint_op_call_ignore_result + 176
    frame #10: 0x0000563c393da5f0 bun-profile`vmEntryToJavaScript + 213
```

触发 `SIGSEGV` 的地方在 `JSC__VM__notifyNeedTermination` ，这个是被优化过的，我们无法直接看到代码文件行号了。不过在 codebase 中全局搜索一下，位置应该在 `src/bun.js/bindings/bindings.cpp:4052`

```cpp
void JSC__VM__notifyNeedTermination(JSC__VM* arg0)
{
    JSC::VM& vm = *arg0;
    bool didEnter = vm.currentThreadIsHoldingAPILock();
    if (didEnter)
        vm.apiLock().unlock();
    vm.notifyNeedTermination();
    if (didEnter)
        vm.apiLock().lock();
}
```

再结合一下汇编代码，发生 `SIGSEGV` 的指令是 `mov    rax, qword ptr [rdi + 0x10]`。这里应该是读取 `rdi +0x10` 时导致的崩溃。

```
(lldb) f 0
frame #0: 0x0000563c395ff547 bun-profile`JSC__VM__notifyNeedTermination + 7
bun-profile`JSC__VM__notifyNeedTermination:
->  0x563c395ff547 <+7>:  mov    rax, qword ptr [rdi + 0x10]
    0x563c395ff54b <+11>: mov    rbx, rdi
    0x563c395ff54e <+14>: cmp    byte ptr [rax + 0x6], 0x0
    0x563c395ff552 <+18>: je     0x3e6c575                 ; <+53>
(lldb)
```

读取 `rdi` ，然后计算一下具体的地址

```
(lldb) register read rdi
     rdi = 0x0000000000000000
```

这里 `rdi` 的值显然不对。回顾上一章节函数调用约定的部分， `rdi` 经常存放函数调用时的第一个参数。结合后面的几条汇编指令， `cmp` 应该是对应了两个 `if` 中的某一个，所以不难判断这个 `rdi` 应该是 `vm` 参数。`0x00` 这个值可以作为 `null` 的另一种表示，但是也有可能是内存被脏写后传参，目前无法判断

试着往前回溯栈帧，看一下 `vm` 的从哪来的

`src/bun.js/web_worker.zig:317`

```zig
    pub fn requestTerminate(this: *WebWorker) callconv(.C) void {
        if (this.requested_terminate) {
            return;
        }
        log("[{d}] requestTerminate", .{this.execution_context_id});
        this.setRef(false);
        this.requested_terminate = true;
        if (this.vm) |vm| {
            vm.jsc.notifyNeedTermination();
            vm.eventLoop().wakeup();
        }
    }

```

`vm.jsc.notifyNeedTermination();` 这里是调用方。判断过 `vm` 是否为 `null`，如果不是那么会访问 `vm.jsc`，这个是函数的参数，相当于 `JSC__VM__notifyNeedTermination(vm.jsc)` 这样的调用。看了一下周边的代码，没有数组之类的，可以大概率排除脏写。应该就是这里的 `vm.jsc` 是 `null` 了

文件内搜索一下关于 `vm` 的处理，可以发现这个函数

```zig
    pub fn exitAndDeinit(this: *WebWorker) noreturn {
        var vm_to_deinit: ?*JSC.VirtualMachine = null;jjj
        if (this.vm) |vm| {
            this.vm = null;
            vm.onExit();
            vm_to_deinit = vm;
        }
        if (vm_to_deinit) |vm| {
            vm.deinit();
        }
    }
```

此函数是 `worker` 主动退出后执行的代码。至此一切都联系起来了。两个线程在这里没有做同步，只是简单的依靠 `this.vm` 是否为 `null` 来判断的，这个会有很多因素干扰。比如编译生成的顺序，下面这两行是没有依赖关系的，有可能顺序发生改变

```cpp
vm.onExit();
this.vm = null
```

还有可见性问题，`this.vm = null` 不一定另一个线程会看到等等

那么为什么 worker 和 main 中传递的数据量会影响到 bug 的复现呢。这个其实不是数据量的问题，而是一种时延。数据量越大， main 中 `console.log` 的时间就越长，导致了复现机率升高。同样的，写文件或者重定向 stdout 会影响 buffer 的大小，变向反映到了时延上面
