
+++
title = "Python async and await"
summary = ''
description = ""
categories = []
tags = []
date = 2017-05-31T14:59:00+08:00
draft = false
+++

*本文可以看做 [generator and coroutine](../python-generator-and-coroutine/) 的后续，主要关于 Python 3.5 后协程的发展*
### PEP 492

在 Python 3.5 之前协程是通过生成器实现的(PEP 342)，在 PEP 380 中引入 `yield from` 语法得到进一步增强。这种实现方式有一些缺点：

- 协程和常规(regular)生成器有着相同的语法，所以他们容易被混淆，尤其对于新人  
- 函数是否是协程是由函数体中是否存在 `yield` 或 `yield from` 所决定。重构时在函数体中添加或删除这些语句时会产生不明显的错误  
- 对异步调用的支持仅限于在语法上允许使用 `yield` 的表达式，一些有用的语法特性被限制，比如 `with` 和 `for` 语句


为了消除生成器和协程之间的模糊关系，PEP 492 试图引入 `async/await`， 实现原生协程(native coroutine)，将这两个概念分离。

>It is proposed to make coroutines a proper standalone concept in Python, and introduce new supporting syntax. The ultimate goal is to help establish a common, easily approachable, mental model of asynchronous programming in Python and make it as close to synchronous programming as possible.

#### `async def` 和 `await`

下面的语法用来声明一个原生协程

```Python
async def read_data(db):
    pass
```

- `async def` 的函数总是一个协程，即使其中不包含 `await`
- `async` 函数中包含 `yield` 和 `yield from` 会是个语法错误(`SyntaxError`)
- `code` 对象添加了两个新的 flag `CO_COROUTINE` 和 `CO_ITERABLE_COROUTINE`
- 调用一个生成器时返回生成器对象，类似地，协程返回协程对象
- `StopIteration` 异常不会传播到协程外部，取而代之的是 `RuntimeError`
- When a native coroutine is garbage collected, a RuntimeWarning is raised if it was never awaited on

`await` 可用来获取一个协程的执行结果

```Python
async def read_data(db):
    data = await db.fetch('SELECT ...')
    ...
```

`await` 和 `yield from` 相似，挂起 `read_data` 的执行直到 `db.fetch` 完成  
`yield from` 和 `await` 一个区别是前者可以接受 generator，而后者不能。其只能接受 awaitable，它可以为以下某一种

- (原生协程函数所返回的)原生协程对象
- 经过 `tyes.coroutine()`装饰的函数所返回的基于生成器的协程对象
- 实现了 `__await__()` 的对象
- 使用 CPython 的 C API `tp_as_async.am_await` 函数修饰的对象


在 `async def` 函数外使用 `await` 会发生语法错误，就像在函数外使用 `yield` 一样

#### `async with`

使用 `async with` 可以创建异步上线文管理器，能够在 enter 和 exit 时挂起。需要实现新的协议 `__aenter__()` 和 `__aexit__()`，两者都要返回 awaitable

```Python
class AsyncContextManager:
    async def __aenter__(self):
        await log('entering context')

    async def __aexit__(self, exc_type, exc, tb):
        await log('exiting context')
```

像常规的 `with` 语句一样，可以在 `async with` 中使用多个上下文管理器。`async with` 只能在 `async def` 函数内部使用

#### 异步迭代器和 `async for`

异步 iterable 能够在其 iter 的实现中调用异步代码；异步 iterator 能狗在其 next 方法中调用异步代码。异步迭代需要以下支持

1. 一个对象必须实现 `__aiter__()` 方法(或者使用 CPython 的 C API `tp_as_async.am_aiter`)返回一个异步 iterator 对象
2. 一个异步 iterator 对象必须实现 `__anext__()` 方法(或者使用 CPython 的 C API `tp_as_async.am_anext`)返回 awaitable
3. 为了停止迭代 `__anext__()` 方法需要抛出 `StopAsyncIteration` 异常

```Python
class AsyncIterable:
    def __aiter__(self):
        return self

    async def __anext__(self):
        data = await self.fetch_data()
        if data:
            return data
        else:
            raise StopAsyncIteration

    async def fetch_data(self):
        ...
```

`async for` 必须在 `async def` 函数内部使用，如果向 `async for` 传入一个不含有 `__aiter__()` 的常规 iterable 会抛出 `TypeError`。`async for` 向通常的 `for` 一样可以有 `else`

下面的工具类实现了常规的 iterable 向异步 iterable 的转换。虽然这不是一件非常有用的事情，但说明了二者之间的关系

```Python
class AsyncIteratorWrapper:
    def __init__(self, obj):
        self._it = iter(obj)

    def __aiter__(self):
        return self

    async def __anext__(self):
        try:
            value = next(self._it)
        except StopIteration:
            raise StopAsyncIteration
        return value

async for letter in AsyncIteratorWrapper("abc"):
    print(letter)
```


另外 PEP 492 中还解答了下面这些问题  

[为什么 StopIteration 变为 RuntimeError, 并使用  StopAsyncIteration](https://www.python.org/dev/peps/pep-0492/#why-stopasynciteration)  
[为什么 await 必须要在 async def 内](https://www.python.org/dev/peps/pep-0492/#importance-of-async-keyword)  
[为什么不重用已有的方法](https://www.python.org/dev/peps/pep-0492/#why-not-reuse-existing-magic-names)  
[为什么不使用 for 和 with](https://www.python.org/dev/peps/pep-0492/#why-not-reuse-existing-for-and-with-statements)

### PEP 525
**(Python 3.6 中实现)**  
PEP 525 中描述了异步生成器(asynchronous generators)，注意这里说的不是用生成器实现异步，而是异步的生成器。异步生成器是为了进一步扩展 Python 的异步能力。常规的生成器在 PEP 255 中引入，目的是为了提供一种和迭代器行为相似，更简洁优雅的的生成复杂数据的途径。然而目前并没有等价的概念用于异步迭代器协议(asynchronous iteration protocol) `async for`。要想使用这种语法必须要定义一个类实现 `__aiter__` 和 `__anext__`，这使得写异步数据生成器存在着不必要的麻烦。

另外，据 PEP 525 中所言

>Performance is an additional point for this proposal: in our testing of the reference implementation, asynchronous generators are 2x faster than an equivalent implemented as an asynchronous iterator.

异步生成器的执行速度是对应异步迭代器的两倍

```Python
class Ticker:
    """Yield numbers from 0 to `to` every `delay` seconds."""

    def __init__(self, delay, to):
        self.delay = delay
        self.i = 0
        self.to = to

    def __aiter__(self):
        return self

    async def __anext__(self):
        i = self.i
        if i >= self.to:
            raise StopAsyncIteration
        self.i += 1
        if i:
            await asyncio.sleep(self.delay)
        return i
```

使用异步生成器的等价实现

```Python
async def ticker(delay, to):
    """Yield numbers from 0 to `to` every `delay` seconds."""
    for i in range(to):
        yield i
        await asyncio.sleep(delay)
```

调用异步生成器函数会返回一个实现了异步迭代协议(PEP 492)的异步生成器对象。异步生成器中不能包含非空 `return` 语句

完整的例子

```Python
import asyncio
import time


async def ticker(delay, to):
    for i in range(to):
        yield i
        await asyncio.sleep(delay)


async def run():
    async for i in ticker(1, 10):
        print(i)


import asyncio
loop = asyncio.get_event_loop()
try:
    loop.run_until_complete(run())
finally:
    loop.close()
```

### Reference

[PEP 342 -- Coroutines via Enhanced Generators](https://www.python.org/dev/peps/pep-0342/)  
[PEP 492 -- Coroutines with async and await syntax](https://www.python.org/dev/peps/pep-0492/)  
[PEP 525 -- Asynchronous Generators](https://www.python.org/dev/peps/pep-0525/)

    