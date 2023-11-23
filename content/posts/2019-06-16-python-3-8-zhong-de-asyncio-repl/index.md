
+++
title = "Python 3.8 中的 asyncio REPL"
summary = ''
description = ""
categories = []
tags = []
date = 2019-06-16T13:14:24+08:00
draft = false
+++

IPython 自 7.0 版本开始支持了在 REPL 中直接 `await`(参考 [Asynchronous in REPL: Autoawait](https://ipython.readthedocs.io/en/stable/interactive/autoawait.html))，并且对 `trio` 和 `curio` 进行支持。可以通过 `%autoawait trio` 指令切换 IOLoop

然后 Python 3.8 起内置支持了 asyncio REPL，可以通过 `python -m asyncio` 进入。具体 PR，参考 [#13472](https://github.com/python/cpython/pull/13472)

我们可以通过手动编译，或者拉取 Python 3.8 的镜像来进行体验

### Implementation

Python 的 `code` 提供了实现 REPL 的一些基本类型，可以参考具体[实现](https://github.com/python/cpython/blob/3.7/Lib/code.py)，一个简单的实现如下


```Python
import sys
from codeop import CommandCompiler
sys.ps1 = '>>> '
sys.ps2 = '... '
compile = CommandCompiler()


def interact():
    buf = []
    local_variable = {}
    more = 0
    while True:
        try:
            prompt = sys.ps2 if more else sys.ps1
            try:
                line = input(prompt)
            except EOFError:
                print('\n')
                break
            else:
                buf.append(line)
                source = '\n'.join(buf)
                try:
                    code= compile(source, '<string>', 'single')
                except (OverflowError, SyntaxError, ValueError):
                    # TODO Show Syntax Error
                    more = False
                if code is None:
                    # input is incomplete
                    more = True
                else:
                    # OK, run the code
                    more = False
                    exec(code, local_variable)
                if not more:
                    buf = []

        except KeyboardInterrupt:
            print('exit')


interact()
```

大致就是读取用户的输入，然后不断利用 `CommandCompiler` 去编译，返回值有三种情况

1. The input is incorrect; `compile_command()` raised an exception (`SyntaxError` or `OverflowError`).  A syntax traceback will be printed by calling the `showsyntaxerror()` method.
1. The input is incomplete, and more input is required; `compile_command()` returned `None`.  Nothing happens.
3. The input is complete; `compile_command()` returned a code object.  The code is executed by calling `self.runcode()` (which also handles run-time exceptions, except for `SystemExit`).

那么如何实现在 REPL 中直接 `await` 呢

首先 Python 3.8 的时候可以通过 `ast.PyCF_ALLOW_TOP_LEVEL_AWAIT` 来开启额外的编译支持，允许在顶层出现 `await`、`async for` 和 `async with`。参考 [What’s New In Python 3.8](https://docs.python.org/3.8/whatsnew/3.8.html#builtins)

最开始的官方实现是这样子的

```Python
import ast
import asyncio
import code
import inspect
import types


class AsyncIOInteractiveConsole(code.InteractiveConsole):

    def __init__(self, locals=None):
        super().__init__(locals)
        self.compile.compiler.flags |= ast.PyCF_ALLOW_TOP_LEVEL_AWAIT

        self.loop = asyncio.new_event_loop()
        asyncio.set_event_loop(self.loop)

    def runcode(self, code):
        try:
            func = types.FunctionType(code, self.locals)
            coro = func()
            if inspect.isawaitable(coro):
                return self.loop.run_until_complete(coro)
        except SystemExit:
            raise
        except BaseException:
            self.showtraceback()


console = AsyncIOInteractiveConsole(locals())
console.interact()
```

但是这种写法会有一个问题

```Python
>>> import asyncio
>>> async def f():
...     await asyncio.sleep(1)
...
>>> asyncio.create_task(f())
/usr/local/lib/python3.8/code.py:140: RuntimeWarning: coroutine 'f' was never awaited
  sys.last_traceback = last_tb
RuntimeWarning: Enable tracemalloc to get the object allocation traceback
Traceback (most recent call last):
  File "<console>", line 1, in <module>
  File "/usr/local/lib/python3.8/asyncio/tasks.py", line 355, in create_task
    loop = events.get_running_loop()
RuntimeError: no running event loop
```

因为我们在 `AsyncIOInteractiveConsole` 中使用的是 `loop.run_until_complete`，在每次调用的时候将 Loop 置为 `Running`，然后在 coro 结束的时候停止

```Python
>>> asyncio.get_event_loop()
<_UnixSelectorEventLoop running=False closed=False debug=False>
>>> id(asyncio.get_event_loop())
140477577720976
>>> async def print_loop():
...     print(id(asyncio.get_event_loop()))
...     print(asyncio.get_event_loop())
...
>>> await print_loop()
140477577720976
<_UnixSelectorEventLoop running=True closed=False debug=False>
```

所以在我们执行 `asyncio.create_task(f())` 的时候，因为这段代码并不是 coro，所以不会通过 `loop.run_until_complete` 去调用，并且此时的 Loop 不是 `Running` 的，所以才会出现上面的异常

后来核心开发们将实现改为在主线程中 `loop.run_forever`，然后在另一个线程中启动 REPL，二者间通过 `loop.call_soon_threadsafe` 将 coro 添加到 ready 队列，然后通过写入 `socketpair` 来唤醒主线程正在因为事件而 block 的 loop 来执行 coro。关于 `asyncio` 的这部分原理可以参考我以前写过了 [Thread — asyncio 源码剖析(3)](https://blog.dreamfever.me/2017/12/16/the-magic-of-asyncio-3/)，里面还涉及到了 `AsyncIOInteractiveConsole` 中为什么使用 `concurrent.futures.Future` 来联动 `asyncio` 的 `Future`。这里就不再赘述了

    