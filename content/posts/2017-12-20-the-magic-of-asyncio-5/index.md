
+++
title = "控制组合 — asyncio 源码剖析(5)"
summary = ''
description = ""
categories = []
tags = []
date = 2017-12-20T14:27:59+08:00
draft = false
+++

*源码基于 Python 3.6.3 的 asyncio commit sha 为 `2c5fed86e0cbba5a4e34792b0083128ce659909d`*

本篇文章分析 `wait()`、`wait_for()`、`as_completed()`、`gather()` 和 `shield()` 的实现。关于这几个函数的使用可以参考 [文档](https://docs.python.org/3/library/asyncio-task.html#task-functions) 以及 [https://pymotw.com/3/asyncio/control.html](https://pymotw.com/3/asyncio/control.html) 中的代码示例

#### 1) `wait()`

`wait()` 函数可以让我们通过 `timeout` 和 `return_when` 两个参数去控制一组 `coroutine` 的执行。此函数的接口和 `concurrent.futures.wait` 完全一致

```Python
# tasks.py
# 移植过来的常量
FIRST_COMPLETED = concurrent.futures.FIRST_COMPLETED
FIRST_EXCEPTION = concurrent.futures.FIRST_EXCEPTION
ALL_COMPLETED = concurrent.futures.ALL_COMPLETED

@coroutine
def wait(fs, *, loop=None, timeout=None, return_when=ALL_COMPLETED):
    # 省略参数检查相关代码
    if loop is None:
        loop = events.get_event_loop()
    fs = {ensure_future(f, loop=loop) for f in set(fs)}
    return (yield from _wait(fs, timeout, return_when, loop))
```

核心部分由 `_wait()` 实现

```Python
# tasks.py
def _release_waiter(waiter, *args):
    if not waiter.done():
        waiter.set_result(None)

@coroutine
def _wait(fs, timeout, return_when, loop):
    waiter = loop.create_future()
    timeout_handle = None
    # 若提供了 timeout 条件，会创建一个定时器，在超时时调用 _release_waiter 将 waiter 标记为完成
    if timeout is not None:
        timeout_handle = loop.call_later(timeout, _release_waiter, waiter)
    counter = len(fs)

    def _on_completion(f):
        nonlocal counter
        counter -= 1
        # counter <= 0　意味着 futures 全部完成
        if (counter <= 0 or
            return_when == FIRST_COMPLETED or
            return_when == FIRST_EXCEPTION and (not f.cancelled() and
                                                f.exception() is not None)):
            # 如果 return_when 的条件已经满足，那么取消定时器
            if timeout_handle is not None:
                timeout_handle.cancel()
            if not waiter.done():
                waiter.set_result(None)

    for f in fs:
        f.add_done_callback(_on_completion)

    try:
        yield from waiter
    finally:
        if timeout_handle is not None:
            timeout_handle.cancel()
    # 根据 future 的状态进行分组
    done, pending = set(), set()
    # 已经执行过 _on_completion 的 future，再次去 remove 不会产生问题
    # 可以参考 remove_done_callback 的实现
    for f in fs:
        f.remove_done_callback(_on_completion)
        if f.done():
            done.add(f)
        else:
            pending.add(f)
    return done, pending
```

#### 2) `wait_for()`

在 `future` 篇中提过相较于 `concurrent.future` 中的 `Future`，`asyncio` 的 `Future` 在获取 `result` 时没有提供超时机制，`wait_for()` 便弥补了这一点。当然你也可以使用 `wait()` 达到同样的功能(仅 `wait()` 一个 `coroutine`)

```Python
# tasks.py
@coroutine
def wait_for(fut, timeout, *, loop=None):
    if loop is None:
        loop = events.get_event_loop()

    if timeout is None:
        return (yield from fut)

    waiter = loop.create_future()
    # 超时的处理策略
    timeout_handle = loop.call_later(timeout, _release_waiter, waiter)
    cb = functools.partial(_release_waiter, waiter)

    fut = ensure_future(fut, loop=loop)
    fut.add_done_callback(cb)

    try:
        # wait until the future completes or the timeout
        try:
            yield from waiter
        except futures.CancelledError:
            fut.remove_done_callback(cb)
            fut.cancel()
            raise

        if fut.done():
            return fut.result()
        else:
            fut.remove_done_callback(cb)
            fut.cancel()
            raise futures.TimeoutError()
    finally:
        timeout_handle.cancel()
```

#### 3) `as_completed()`

请注意 `as_completed()` 是一个 `generator` 而不是 `coroutine`。

```Python
# tasks.py
def as_completed(fs, *, loop=None, timeout=None):
    loop = loop if loop is not None else events.get_event_loop()
    todo = {ensure_future(f, loop=loop) for f in set(fs)}
    from .queues import Queue  # Import here to avoid circular import problem.
    # 完成的 future 会被丢进去
    done = Queue(loop=loop)
    timeout_handle = None

    def _on_timeout():
        for f in todo:
            f.remove_done_callback(_on_completion)
            done.put_nowait(None)  # Queue a dummy value for _wait_for_one().
        todo.clear()  # Can't do todo.remove(f) in the loop.

    def _on_completion(f):
        if not todo:
            return  # _on_timeout() was here first.
        todo.remove(f)
        done.put_nowait(f)
        # 最后一个完成时会取消超时
        if not todo and timeout_handle is not None:
            timeout_handle.cancel()

    @coroutine
    def _wait_for_one():
        # 从队列返回结果，如果出现 None 说明发生了超时
        f = yield from done.get()
        if f is None:
            # Dummy value from _on_timeout().
            raise futures.TimeoutError
        return f.result()  # May raise f.exception().

    for f in todo:
        f.add_done_callback(_on_completion)
    if todo and timeout is not None:
        timeout_handle = loop.call_later(timeout, _on_timeout)
    for _ in range(len(todo)):
        yield _wait_for_one()
```

先完成的会被塞进队列中，所以并不保证顺序和传入 `as_completed()` 时一致

#### 4) `shield()`

`shield()` 可以屏蔽 `cancel()`，保护内部的 task

```Python
# tasks.py
def shield(arg, *, loop=None):
    inner = ensure_future(arg, loop=loop)
    if inner.done():
        # Shortcut.
        return inner
    loop = inner._loop
    outer = loop.create_future()

    def _done_callback(inner):
        if outer.cancelled():
            if not inner.cancelled():
                # Mark inner's result as retrieved.
                inner.exception()
            return
        # 复制 inner 的状态至 outer
        if inner.cancelled():
            outer.cancel()
        else:
            exc = inner.exception()
            if exc is not None:
                outer.set_exception(exc)
            else:
                outer.set_result(inner.result())

    inner.add_done_callback(_done_callback)
    return outer
```

#### 5) `gather()`

`gather()` 可以聚合多个 `future`/`coroutine` 的结果，返回一个 `list`。当 `return_exceptions` 参数为 `True` 时，task 的异常也会被当做结果放到返回值中。其返回结果的顺序和传入参数的顺序一致

```Python
# tasks.py
class _GatheringFuture(futures.Future):
    def __init__(self, children, *, loop=None):
        super().__init__(loop=loop)
        self._children = children

    def cancel(self):
        if self.done():
            return False
        ret = False
        for child in self._children:
            if child.cancel():
                ret = True
        return ret

def gather(*coros_or_futures, loop=None, return_exceptions=False):
    # coros_or_futures 参数为空 tuple 时直接返回 []
    # 可以参考 Future 的 __iter__() 方法实现
    if not coros_or_futures:
        if loop is None:
            loop = events.get_event_loop()
        outer = loop.create_future()
        outer.set_result([])
        return outer

    arg_to_fut = {}
    for arg in set(coros_or_futures):
        if not futures.isfuture(arg):
            fut = ensure_future(arg, loop=loop)
            if loop is None:
                loop = fut._loop
            # The caller cannot control this future, the "destroy pending task"
            # warning should not be emitted.
            fut._log_destroy_pending = False
        else:
            fut = arg
            if loop is None:
                loop = fut._loop
            elif fut._loop is not loop:
                raise ValueError("futures are tied to different event loops")
        arg_to_fut[arg] = fut

    children = [arg_to_fut[arg] for arg in coros_or_futures]
    nchildren = len(children)
    outer = _GatheringFuture(children, loop=loop)
    nfinished = 0
    # 占位
    results = [None] * nchildren

    def _done_callback(i, fut):
        nonlocal nfinished
        if outer.done():
            if not fut.cancelled():
                # Mark exception retrieved.
                fut.exception()
            return

        if fut.cancelled():
            res = futures.CancelledError()
            if not return_exceptions:
                outer.set_exception(res)
                return
        elif fut._exception is not None:
            res = fut.exception()  # Mark exception retrieved.
            if not return_exceptions:
                outer.set_exception(res)
                return
        else:
            res = fut._result
        # 替换占位
        results[i] = res
        nfinished += 1
        if nfinished == nchildren:
            outer.set_result(results)

    for i, fut in enumerate(children):
        fut.add_done_callback(functools.partial(_done_callback, i))
    return outer
```

    