
+++
title = "generator and coroutine"
summary = ''
description = ""
categories = []
tags = []
date = 2017-03-18T08:29:23+08:00
draft = false
+++

因为面试中被追问了好几次，所以傻逼作者就写了这篇文章

先说一说面试时的回答：
生成器是包含 `yield value` 的函数，而协程不仅是 `yield value` 还可以 `rev = yield value`

先来看看其他人对于这个问题的回答

[Coroutine vs Continuation vs Generator](http://stackoverflow.com/questions/715758/coroutine-vs-continuation-vs-generator)

>A generator is essentially a cut down (asymmetric) coroutine. The difference between a coroutine and generator is that a coroutine can accept arguments after it's been initially called, whereas a generator can't.

另一份资料，它这样描述
[intermediatePython](https://github.com/yasoob/intermediatePython/blob/master/coroutines.rst)

>Coroutines are similar to generators with a few differences. The main differences are:
>
>- generators are data producers
>- coroutines are data consumers

好吧，上面的答案真的正确么？ 是的，没问题

---

先来看看 coroutine 的定义(摘自 Wiki)

>Coroutines are computer program components that generalize subroutines for non-preemptive multitasking, by allowing multiple entry points for suspending and resuming execution at certain locations.


我们知道，生成器也是可以挂起和恢复的，那么究竟有着什么区别？傻逼作者试图从官方寻找答案。

[PEP 255](https://www.python.org/dev/peps/pep-0255/) 中第一次提出生成器的概念，并且引入了 `yield` 语句。

>When a producer function has a hard enough job that it requires maintaining state between values produced, most programming languages offer no pleasant and efficient solution beyond adding a callback function to the producer's argument list, to be called with each value produced.

并且在这篇 PEP 中有提到 协程 (coroutine)

>A final option is to use the Stackless[2][3] variant implementation of Python instead, which supports lightweight coroutines.  This has much the same programmatic benefits as the thread option, but is much more efficient.

生成器是一种简化的迭代器创建方式，网络上有人说生成器可以避免数据结构一次性构建产生的内存消耗，迭代器同样也可以做到。但人们注意到了生成器能够简单的仅实现函数的挂起与恢复，这和协程的概念很相像。**生成器这时被当做轻量级的协程使用，但是此时的协程并不完善。我们不能向协程中传递参数，也就是说这个协程只能按照预定流程暂停执行返还调用权。**

[PEP 334](https://www.python.org/dev/peps/pep-0334/) 中提及了如何通过迭代器和自定义异常 `SuspendIteration` 来实现协程

>This PEP proposes a limited approach to coroutines based on an extension to the iterator protocol [5] . Currently, an iterator may raise a StopIteration exception to indicate that it is done producing values. This proposal adds another exception to this protocol, SuspendIteration, which indicates that the given iterator may have more values to produce, but is unable to do so at this time.

    from random import randint
    from time import sleep

    class SuspendIteration(Exception):
          pass

    class NonBlockingResource:

        """Randomly unable to produce the second value"""

        def __iter__(self):
            self._next = self.state_one
            return self

        def next(self):
            return self._next()

        def state_one(self):
            self._next = self.state_suspend
            return "one"

        def state_suspend(self):
            rand = randint(1,10)
            if 2 == rand:
                self._next = self.state_two
                return self.state_two()
            raise SuspendIteration()

        def state_two(self):
            self._next = self.state_stop
            return "two"

        def state_stop(self):
            raise StopIteration

    def sleeplist(iterator, timeout = .1):
        """
        Do other things (e.g. sleep) while resource is
        unable to provide the next value
        """
        it = iter(iterator)
        retval = []
        while True:
            try:
                retval.append(it.next())
            except SuspendIteration:
                sleep(timeout)
                continue
            except StopIteration:
                break
        return retval

    print sleeplist(NonBlockingResource())


In a real-world situation, the NonBlockingResource would be a file iterator, socket handle, or other I/O based producer. The sleeplist would instead be an async reactor, such as those found in asyncore or Twisted.

这种方式我们可以通过改变 iter 内部的状态来更改执行流程。除此之外还有多种协程的实现方式，比如 `gevent` 在底层实现了协程的支持。

至此，傻逼作者明白了：

**协程是一个概念，它有着多种实现方式，生成器只是其中一种。**

然后就是 [PEP 342](https://www.python.org/dev/peps/pep-0342/) 了， 通过增强生成器来支持协程。
做出了如下的修改：

`yield` 语句改为表达式（意味着可以 `a = yield b`），并为生成器增加了一些新的方法：`send()`，`throw()`，`close()`。同时，`yield` 可以在 `try/finally` 使用。

`throw()` 是我没接触过的，他可以向协程内部传递一个异常

    def gen():
        while True:
            print("enter loop")
            try:
                yield "First"
                yield "Second"
            except Exception:
                print("catch an Exception")

    g = gen()
    print(next(g))
    print(g.throw(Exception))

Output:

    enter loop
    First
    catch an Exception
    enter loop
    First

除此外还不够，[PEP 380](https://www.python.org/dev/peps/pep-0380/) 中引入了 `yield from`，来解决如下的问题

>A Python generator is a form of coroutine, but has the limitation that it can only yield to its immediate caller. This means that a piece of code containing a yield cannot be factored out and put into a separate function in the same way as other code. Performing such a factoring causes the called function to itself become a generator, and it is necessary to explicitly iterate over this second generator and re-yield any values that it produces.

简而言之便是简化了外层的 `generator` 向内层 `generator` 的消息传递，生成器可以简单的串联在一起

    def inner():
        val = 0
        while True:
            val = yield val

    def outer():
        _inner = inner()
        input_val = None
        ret = _inner.send(input_val)
        while True:
            input_val = yield ret
            ret = _inner.send(input_val)


`yield from` 改进后

    def outer():
         yield from inner()

在 [PEP 3156](https://www.python.org/dev/peps/pep-3156/) 中提出更深一步的协程支持 `asyncio` module 。`asyncio.coroutine` 装饰器可以用来标记作为协程的函数。这里的协程是和事件循环搭配使用的，主要用来实现异步 IO 操作。

    import asyncio

    @asyncio.coroutine
    def task(name, times):
        for i in range(times):
            print(name, i)
            yield from asyncio.sleep(1)

    loop = asyncio.get_event_loop()
    tasks = [
        asyncio.ensure_future(task("A", 2)),
        asyncio.ensure_future(task("B", 3))]
    loop.run_until_complete(asyncio.wait(tasks))
    loop.close()

[PEP 492](https://www.python.org/dev/peps/pep-0492/) 中引入了 `async & await` 语法的协程

>It is proposed to make coroutines a proper standalone concept in Python, and introduce new supporting syntax. The ultimate goal is to help establish a common, easily approachable, mental model of asynchronous programming in Python and make it as close to synchronous programming as possible.

可见，协程要成为一个更加独立的概念

### Reference
- [PEP 255 -- Simple Generators](https://www.python.org/dev/peps/pep-0255/)
- [PEP 334 -- Simple Coroutines via SuspendIteration](https://www.python.org/dev/peps/pep-0334/)
- [PEP 342 -- Coroutines via Enhanced Generators](https://www.python.org/dev/peps/pep-0342/)
- [PEP 492 -- Coroutines with async and await syntax](https://www.python.org/dev/peps/pep-0492/)
- [PEP 3156 -- Asynchronous IO Support Rebooted: the "asyncio" Module](https://www.python.org/dev/peps/pep-3156/)


