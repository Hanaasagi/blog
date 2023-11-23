
+++
title = "future的告白 — asyncio 源码剖析(2)"
summary = ''
description = ""
categories = []
tags = []
date = 2017-12-15T12:20:00+08:00
draft = false
+++

*源码基于 Python 3.6.3 的 asyncio commit sha 为 `2c5fed86e0cbba5a4e34792b0083128ce659909d`*

future 这个概念最早在 [PEP 3148](https://www.python.org/dev/peps/pep-3148/) 中提出，于 Python 3.2 加入了 `concurrent` 库提供了支持。future 可以理解为一个占位符，它在未来的某个时刻才会被设置结果(调用 `set_result()`)。可以通过接口 `future.result()` 来获取其结果

刚才说过了 Python 的 `concurrent` 库实现了 future，那么 `asyncio` 的 future 又是什么呢？`asyncio` 的 future 兼容 `concurrent` 的 future。不过做了一些小改动，比如 `result()` 和 `exception()` 不再接收一个 `timeout` 参数，当 future 没有完成时直接抛出异常

future 的源代码位于 `asyncio/futures.py`，task 的源代码位于 `asyncio/tasks.py`。另外还有一个 `base_futures.py` 定义了一些处理函数

先来看一下 future 所具有的属性

```Python
class Future:

    _state = _PENDING
    _result = None
    _exception = None
    _loop = None
    _source_traceback = None

    # This field is used for a dual purpose:
    # - Its presence is a marker to declare that a class implements
    #   the Future protocol (i.e. is intended to be duck-type compatible).
    #   The value must also be not-None, to enable a subclass to declare
    #   that it is not compatible by setting this to None.
    # - It is set by __iter__() below so that Task._step() can tell
    #   the difference between `yield from Future()` (correct) vs.
    #   `yield Future()` (incorrect).
    _asyncio_future_blocking = False

    _log_traceback = False   # Used for Python 3.4 and later
    _tb_logger = None        # Used for Python 3.3 only
```

`_state` 用于表明 `future` 当前所处的状态，共有三种状态 `PENDING`、`CANCELLED`、`FINISHED`。默认是 `PENDING`，提供了两个方法 `done()`、`cancelled()` 去检查当前的状态是否为 `FINISHED`、`CANCELLED`

```Python
# 定义于 base_futures.py
_PENDING = 'PENDING'
_CANCELLED = 'CANCELLED'
_FINISHED = 'FINISHED'
```

在 `future` 没有结果前都是 `PENDING` 状态。当一个 `future` 完成时，它的 `_result` 或者 `_exception` 属性会被赋值。更贴切地说是，当它有了结果(`set_result()`)或者发生了异常(`set_exception()`)后，它的状态才会被转移为 `FINISHED`。

`_loop` 属性存储了事件循环对象，因为 `future` 也会向事件循环去注册一些事件

`future` 对象在初始化时可以手工传入 `loop` 对象，如果不传则会通过 `events.get_event_loop()` 来获取当前的 `loop` 对象。`events.get_event_loop()` 的工作机制上一篇中已经分析过了

```Python
class Future:
    def __init__(self, *, loop=None):
        if loop is None:
            self._loop = events.get_event_loop()
        else:
            self._loop = loop
        self._callbacks = []
        if self._loop.get_debug():
            self._source_traceback = traceback.extract_stack(sys._getframe(1))
```

`future` 有自己的 `callback`，存放在 `_callbacks` 属性中。所有的 `callback` 会在 `future` 完成时得到调度

```Python
class Future:
    def _schedule_callbacks(self):
        callbacks = self._callbacks[:]
        if not callbacks:
            return
        self._callbacks[:] = []
        for callback in callbacks:
            self._loop.call_soon(callback, self)
```

可以看到，`callback` 是可以得到此 `future` 对象然后获取其结果的

向 `future` 添加/删除 `callback` 可以通过如下的方法

```Python
class Future:
    def add_done_callback(self, fn):
        # 如果状态不是 PENDING 则直接进行调度
        if self._state != _PENDING:
            self._loop.call_soon(fn, self)
        else:
            self._callbacks.append(fn)

    def remove_done_callback(self, fn):
        filtered_callbacks = [f for f in self._callbacks if f != fn]
        removed_count = len(self._callbacks) - len(filtered_callbacks)
        if removed_count:
            self._callbacks[:] = filtered_callbacks
        return removed_count
```

再来看一下 `future` 是如何转移到 `FINISHED` 状态的

```Python
class Future:
    def set_result(self, result):
        if self._state != _PENDING:
            raise InvalidStateError('{}: {!r}'.format(self._state, self))
        self._result = result
        self._state = _FINISHED
        self._schedule_callbacks()

    def set_exception(self, exception):
        if self._state != _PENDING:
            raise InvalidStateError('{}: {!r}'.format(self._state, self))
        if isinstance(exception, type):
            exception = exception()
        if type(exception) is StopIteration:
            raise TypeError("StopIteration interacts badly with generators "
                            "and cannot be raised into a Future")
        self._exception = exception
        self._state = _FINISHED
        self._schedule_callbacks()
        if compat.PY34:
            self._log_traceback = True
        else:
            self._tb_logger = _TracebackLogger(self, exception)
            # Arrange for the logger to be activated after all callbacks
            # have had a chance to call result() or exception().
            self._loop.call_soon(self._tb_logger.activate)
```

`future` 的结果可以通过 `result()` 方法获得

```Python
class Future:
    def result(self):
        # future 可能被取消
        if self._state == _CANCELLED:
            raise CancelledError
        # future 未完成则抛出 InvalidStateError
        if self._state != _FINISHED:
            raise InvalidStateError('Result is not ready.')
        self._log_traceback = False
        if self._tb_logger is not None:
            self._tb_logger.clear()
            self._tb_logger = None
        # 如果执行间发生了异常则抛出，否则返回结果
        if self._exception is not None:
            raise self._exception
        return self._result

    def exception(self):
        if self._state == _CANCELLED:
            raise CancelledError
        if self._state != _FINISHED:
            raise InvalidStateError('Exception is not set.')
        self._log_traceback = False
        if self._tb_logger is not None:
            self._tb_logger.clear()
            self._tb_logger = None
        return self._exception
```

也可以取消一个 `PENDING` 状态的 `future`

```Python
class Future:
    def cancel(self):
        self._log_traceback = False
        # 如果为 FINISHED 则并不会改变
        if self._state != _PENDING:
            return False
        self._state = _CANCELLED
        self._schedule_callbacks()
        return True
```

`Future` 的核心，实现了 `__iter__` 和 `__await__`，使其能够支持 `yield from` 和 `await` 语法

```Python
class Future:
    def __iter__(self):
        if not self.done():
            self._asyncio_future_blocking = True
            yield self  # This tells Task to wait for completion.
        assert self.done(), "yield from wasn't used with future"
        return self.result()  # May raise too.

    if compat.PY35:
        __await__ = __iter__ # make compatible with 'await' expression
```

除此之外需要再详细解释一下 `future` 的异常处理，如果你用过 `asyncio` 的话，说不定会遇到形如 `future: <Future finished exception=Exception()>` 的控制台输出。比如下面的错误代码

```Python
import asyncio

async def fn():
    print('fn run')
    raise Exception

asyncio.ensure_future(fn())
loop = asyncio.get_event_loop()
loop.run_forever()
# 这里的正确做法是使用 loop.run_until_complete()
# 它会调用 future.result() 这样便会直接抛出异常了
# 本身不调用 future 的 result 和 exception 就是一种奇怪的做法
# future 不是用来做 fire and forget 式任务的
```

大概运行之后会出来这样的输出

```
fn run
Task exception was never retrieved
future: <Task finished coro=<fn() done, defined at dd.py:4> exception=Exception()>
Traceback (most recent call last):
  File "/root/asyncio/tasks.py", line 180, in _step
    result = coro.send(None)
  File "dd.py", line 6, in fn
    raise Exception
Exception
```

这个异常的 `traceback` 并不是像普通程序那样直接出来的，而且不同版本的 Python 有不同的做法。Python 3.4 之后是通过直接重写 `Future` 类的 `__del__` 方法实现的，所以要 GC 回收之后才会出现

```Python
class Future:
    def __del__(self):
        if not self._log_traceback:
            # set_exception() was not called, or result() or exception()
            # has consumed the exception
            return
        exc = self._exception
        context = {
            'message': ('%s exception was never retrieved'
                        % self.__class__.__name__),
            'exception': exc,
            'future': self,
        }
        if self._source_traceback:
            context['source_traceback'] = self._source_traceback
        # 输出异常
        self._loop.call_exception_handler(context)
```

而在 3.3 中则是直接在 `set_exception` 则是借助了 `_TracebackLogger` 对象的 `__del__` 方法。也就是说同样需要在 GC 回收后才出现。`future` 的属性 `_tb_logger` 就是一个 `_TracebackLogger` 的实例。当出现异常时，可以看到 `set_exception()` 方法会通过 `loop.call_soon()` 方法进行激活

```Python
class _TracebackLogger:
    __slots__ = ('loop', 'source_traceback', 'exc', 'tb')

    def __init__(self, future, exc):
        self.loop = future._loop
        self.source_traceback = future._source_traceback
        self.exc = exc
        self.tb = None

    def activate(self):
        exc = self.exc
        if exc is not None:
            self.exc = None
            self.tb = traceback.format_exception(exc.__class__, exc,
                                                 exc.__traceback__)
    def clear(self):
        self.exc = None
        self.tb = None

    def __del__(self):
        if self.tb:
            msg = 'Future/Task exception was never retrieved\n'
            if self.source_traceback:
                src = ''.join(traceback.format_list(self.source_traceback))
                msg += 'Future/Task created at (most recent call last):\n'
                msg += '%s\n' % src.rstrip()
            msg += ''.join(self.tb).rstrip()
            self.loop.call_exception_handler({'message': msg})
```

至于为什么这样做，`_TracebackLogger` 的 docstring 说明了缘由：

>This solves a nasty problem with Futures and Tasks that have an exception set: if nobody asks for the exception, the exception is never logged.  This violates the Zen of Python: 'Errors should never pass silently.  Unless explicitly silenced.'
However, we don't want to log the exception as soon as `set_exception()` is called: if the calling code is written properly, it will get the exception and handle it properly.  But we *do* want to log it if `result()` or `exception()` was never called -- otherwise developers waste a lot of time wondering why their buggy code fails silently.
An earlier attempt added a `__del__()` method to the Future class itself, but this backfired because the presence of `__del__()` prevents garbage collection from breaking cycles.  A way out of this catch-22 is to avoid having a `__del__()` method on the Future class itself, but instead to have a reference to a helper object with a `__del__()` method that logs the traceback, where we ensure that the helper object doesn't participate in cycles, and only the Future has a reference to it.
The helper object is added when `set_exception()` is called.  When the Future is collected, and the helper is present, the helper object is also collected, and its `__del__()` method will log the traceback.  When the Future's `result()` or `exception()` method is called (and a helper object is present), it removes the helper object, after calling its `clear()` method to prevent it from logging.
One downside is that we do a fair amount of work to extract the traceback from the exception, even when it is never logged.  It would seem cheaper to just store the exception object, but that references the traceback, which references stack frames, which may reference the Future, which references the `_TracebackLogger`, and then the `_TracebackLogger` would be included in a cycle, which is what we're trying to avoid!  As an optimization, we don't immediately format the exception; we only do the work when `activate()` is called, which call is delayed until after all the Future's callbacks have run.  Since usually a Future has at least one callback (typically set by 'yield from') and usually that callback extracts the callback, thereby removing the need to format the exception.

    