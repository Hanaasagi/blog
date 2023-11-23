
+++
title = "Task — asyncio 源码剖析(4)"
summary = ''
description = ""
categories = []
tags = []
date = 2017-12-19T02:14:49+08:00
draft = false
+++

*源码基于 Python 3.6.3 的 asyncio commit sha 为 `2c5fed86e0cbba5a4e34792b0083128ce659909d`*

`Task` 正如其 docstring 所言：“A coroutine wrapped in a Future”。其本身是 `Future` 的子类，但是相较于 `Future`，它包含了一个 `coroutine`。`Future` 仅仅作为一个未执行完的结果的占位表示，它依靠外部调用 `set_result()` 或者 `set_exception()` 将状态置为 `FINISHED`。而 `Task` 能够驱动内置的 `coroutine` 然后为自身 `set_result()`

```Python
# tasks.py
class Task(futures.Future):
    def __init__(self, coro, *, loop=None):
        super().__init__(loop=loop)
        if self._source_traceback:
            del self._source_traceback[-1]
        self._coro = coro
        self._fut_waiter = None
        self._must_cancel = False
        self._loop.call_soon(self._step)
        self.__class__._all_tasks.add(self)
```

`Task` 提供了两个类方法 `all_tasks()` 和 `current_task()` 以获取 `loop` 所对应的所有 tasks/运行的 tasks。`Task` 在初始化时会调用 `loop` 的 `call_soon()` 方法，使得自身的 `_step` 方法会得到调度

```Python
# tasks.py
class Task(futures.Future):
    def _step(self, exc=None):
        assert not self.done(), '_step(): already done: {!r}, {!r}'.format(self, exc)
        if self._must_cancel:
            if not isinstance(exc, futures.CancelledError):
                exc = futures.CancelledError()
            self._must_cancel = False
        coro = self._coro
        self._fut_waiter = None
        self.__class__._current_tasks[self._loop] = self
        # Call either coro.throw(exc) or coro.send(None).
        try:
            if exc is None:
                # We use the `send` method directly, because coroutines
                # don't have `__iter__` and `__next__` methods.
                result = coro.send(None)
            else:
                result = coro.throw(exc)
        except StopIteration as exc:
            if self._must_cancel:
                # Task is cancelled right before coro stops.
                self._must_cancel = False
                self.set_exception(futures.CancelledError())
            else:
                self.set_result(exc.value)
        except futures.CancelledError:
            super().cancel()  # I.e., Future.cancel(self).
        except Exception as exc:
            self.set_exception(exc)
        except BaseException as exc:
            self.set_exception(exc)
            raise
        else:
            blocking = getattr(result, '_asyncio_future_blocking', None)
            if blocking is not None:
                # Yielded Future must come from Future.__iter__().
                if result._loop is not self._loop:
                    self._loop.call_soon(
                        self._step,
                        RuntimeError(
                            'Task {!r} got Future {!r} attached to a '
                            'different loop'.format(self, result)))
                elif blocking:
                    if result is self:
                        self._loop.call_soon(
                            self._step,
                            RuntimeError(
                                'Task cannot await on itself: {!r}'.format(
                                    self)))
                    else:
                        result._asyncio_future_blocking = False
                        # future 得到结果后，会调用 `_wakeup()`
                        result.add_done_callback(self._wakeup)
                        self._fut_waiter = result
                        if self._must_cancel:
                            if self._fut_waiter.cancel():
                                self._must_cancel = False
                else:
                    self._loop.call_soon(
                        self._step,
                        RuntimeError(
                            'yield was used instead of yield from '
                            'in task {!r} with {!r}'.format(self, result)))
            elif result is None:
                # Bare yield relinquishes control for one event loop iteration.
                self._loop.call_soon(self._step)
            elif inspect.isgenerator(result):
                # Yielding a generator is just wrong.
                self._loop.call_soon(
                    self._step,
                    RuntimeError(
                        'yield was used instead of yield from for '
                        'generator in task {!r} with {}'.format(
                            self, result)))
            else:
                # Yielding something else is an error.
                self._loop.call_soon(
                    self._step,
                    RuntimeError(
                        'Task got bad yield: {!r}'.format(result)))
        finally:
            self.__class__._current_tasks.pop(self._loop)
            self = None # Needed to break cycles when an exception occurs.
```

先前我们已经研究过部分实现了，现在解释一下 `_asyncio_future_blocking` 的作用。这个属性定义于其父类 `Future` 中，默认值为 `False`。其作用有两个

- 1) 作为一个 marker，表示此类实现了 Future 协议，可以用在兼容 duck-type 的代码中。当值为 `True` 或 `False` 时，表示可以兼容；值为 `None` 表示不兼容
- 2) `Future` 的 `__iter__()` 方法会将此属性的值设置为 `True`。用于区分 `yield from Future()` 和 `yielf Future()`

关于 `send()` 和 `throw()` 如果不清楚可以去参考 [PEP 342](https://www.python.org/dev/peps/pep-0342/)

再来看一下 `_wakeup()` 方法的实现

```Python
# tasks.py
class Task(futures.Future):
    def _wakeup(self, future):
        try:
            future.result()
        except Exception as exc:
            # This may also be a cancellation.
            self._step(exc)
        else:
            # Don't pass the value of `future.result()` explicitly,
            # as `Future.__iter__` and `Future.__await__` don't need it.
            # If we call `_step(value, None)` instead of `_step()`,
            # Python eval loop would use `.send(value)` method call,
            # instead of `__next__()`, which is slower for futures
            # that return non-generator iterators from their `__iter__`.
            self._step()
        self = None # Needed to break cycles when an exception occurs.
```

`_wakeup` 负责将 `future` 的异常(若存在)通过 `_step()` 再次传入我们的 `coroutine`。所以我们才可以这样去写代码

```Python
try:
    result = await db.query('a SQL statement')
except DatabaseError as e:
   # do something
```

另外需要注意的是，第一 `_wakeup()` 没有取返回值，第二 `_step()` 方法是写死的 `coro.send(None)`。而 `future` 的结果是由自身取出然后返回的

```Python
# futures.py
class Future:
    def __iter__(self):
        if not self.done():
            self._asyncio_future_blocking = True
            yield self  # This tells Task to wait for completion.
        assert self.done(), "yield from wasn't used with future"
        return self.result() # May raise too.
```

其实 `send()` 方法可以将 `future` 的结果直接传回的，但是却没有这么做。这个原因在注释中已经写明了

>If we call `_step(value, None)` instead of `_step()`, Python eval loop would use `.send(value)` method call, instead of `__next__()`, which is slower for futures that return non-generator iterators from their `__iter__`.

再来看一下 `cancel()` 方法的实现

```Python
# tasks.py
class Task(futures.Future):
    def cancel(self):
        self._log_traceback = False
        if self.done():
            return False
        if self._fut_waiter is not None:
            # 此时 _fut_waiter 的状态可能为 FINISHED，则需要强制 cancel
            if self._fut_waiter.cancel():
                # Leave self._fut_waiter; it may be a Task that
                # catches and ignores the cancellation so we may have
                # to cancel it again later.
                return True
        # It must be the case that self._step is already scheduled.
        self._must_cancel = True
        return True

# futures.py
class Future:
    def cancel(self):
        self._log_traceback = False
        if self._state != _PENDING:
            return False
        self._state = _CANCELLED
        self._schedule_callbacks()
        return True
```

根据 `Future` 类的 `cancel()` 实现可以得知，当 `_fut_waiter` 已经完成时，并不会再将其状态置为 `_CANCELLED`。这样的话调用 `_fut_waiter.result()` 也不会抛出 `CancelledError`。所以这里引入了一个用于强制 cancel 的属性 `_must_cancel`。当 `Task` 的 `_step()` 方法再次被调用时会向 `coro` 发送一个 `CancelledError`。但这种做法也引入了新的问题，如果在 `coro` 中使用了异常捕获，而且代码写的是 `Exception` 时就很尴尬了

另外还需要注意 `Task` 重写了父类的 `cancel()` 方法，也没有去调用 `super().cancel()`。当调用 `task.cancel()` 时，立即调用 `task.cancelled()` 方法并不会返回 `True`，必须要到下一次调用 `_step()`

    