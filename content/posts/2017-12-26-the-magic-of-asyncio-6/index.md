
+++
title = "同步机制 — asyncio 源码剖析(6)"
summary = ''
description = ""
categories = []
tags = []
date = 2017-12-26T14:51:31+08:00
draft = false
+++

*源码基于 Python 3.6.3 的 asyncio commit sha 为 `2c5fed86e0cbba5a4e34792b0083128ce659909d`*

`asyncio` 提供了基本的同步原语，具体使用可以参考 [Synchronization Primitives](https://pymotw.com/3/asyncio/synchronization.html)

获取锁可以换出，释放锁由于锁已经在手中了所以不需要换出

`Lock` 通过 `_ContextManagerMixin` 支持了 `with` 语法

```Python
# locks.py
class _ContextManager:
    def __init__(self, lock):
        self._lock = lock

    def __enter__(self):
        return None  #1

    def __exit__(self, *args):
        try:
            self._lock.release()
        finally:
            self._lock = None # Crudely prevent reuse.

class _ContextManagerMixin:
    def __enter__(self):
        # 提示你应当使用 with await 和 with yield from 的语法
        raise RuntimeError('"yield from" should be used as context manager expression')

    def __exit__(self, *args):
        # This must exist because __enter__ exists, even though that
        # always raises; that's how the with-statement works.
        pass

    @coroutine
    def __iter__(self):
        yield from self.acquire()  #2
        return _ContextManager(self)

    if compat.PY35:
        def __await__(self):
            # To make "with await lock" work.
            yield from self.acquire()
            return _ContextManager(self)

        @coroutine
        def __aenter__(self):
            yield from self.acquire()
            return None

        @coroutine
        def __aexit__(self, exc_type, exc, tb):
            self.release()
```

结合 `#1` 和 `#2` 处可以得知获取锁的操作实际上是在 `#2` 处，而返回的 `_ContextManager` 的 `__enter__` 什么也没做。它只负责在 `with` block 结束后释放锁。需要注意的是获取锁的操作是异步的而释放锁的操作是同步的。另外一点从上面代码可以看出来 `async with` 语法糖的出现简直是福音，可以少写很多代码

下面来看一下 `Lock` 的实现

```Python
# locks.py
class Lock(_ContextManagerMixin):
    def __init__(self, *, loop=None):
        # _waiters 用于存储等待获取此锁的 coroutine
        self._waiters = collections.deque()
        # 锁的状态，True 表示被某 coroutine 占用中
        self._locked = False
        if loop is not None:
            self._loop = loop
        else:
            self._loop = events.get_event_loop()

    def locked(self):
        return self._locked

    @coroutine
    def acquire(self):
        # 锁处于空闲状态，且之前等待锁的 coroutine 都 cancel 则可以直接获取到
        if not self._locked and all(w.cancelled() for w in self._waiters):
            self._locked = True
            return True

        fut = self._loop.create_future()
        self._waiters.append(fut)
        try:
            # 等待此 future done
            yield from fut
            self._locked = True
            return True
        except futures.CancelledError:
            if not self._locked:
                self._wake_up_first()
            raise
        finally:
            self._waiters.remove(fut)

    def release(self):
        if self._locked:
            self._locked = False
            # 唤醒队列中的第一个 coroutine
            self._wake_up_first()
        else:
            raise RuntimeError('Lock is not acquired.')

    def _wake_up_first(self):
        for fut in self._waiters:
            if not fut.done():
                fut.set_result(True)
                break
```

根据 `_wake_up_first()` 的方法实现，当锁被释放时一定是最先想要获取锁的那个 `coroutine` 获得调度。这和操作系统级别的锁有很大的区别。这里需要注意一个细节 `_wake_up_first()` 方法不直接 `popleft` 然后 `set_result()`？这是因为第一个 `future` 有可能已经随着 task 的 cancel 而 cancel。还有一个问题就是为什么 `_wake_up_first()` 方法不直接从 `_waiters` 队列中将那个 `future` 删除，反而将此操作放到 `acquire()` 中呢？原因还是与 future 会被 cancel 有关。要知道在获取一个锁的时候，会转让执行权，回到最外层。有两种方式会再度回到这里 1)此 task 被 cancel 2)别的 task 释放了锁。这两种情况在根本上是一样的，那就是 future 完成(`FINISHED`/`CANCELLED`)。显然 task cancel 的这种情况不会经过 `_wake_up_first()`。为此 `_waiters.remove(fut)` 放到了 `acuqire()` 中


`Event` 和 `threading.Event` 的效果差不多，其实现如下

```Python
class Event:
    def __init__(self, *, loop=None):
        self._waiters = collections.deque()
        self._value = False
        if loop is not None:
            self._loop = loop
        else:
            self._loop = events.get_event_loop()

    def is_set(self):
        return self._value

    @coroutine
    def wait(self):
        if self._value:
            return True

        fut = self._loop.create_future()
        self._waiters.append(fut)
        try:
            yield from fut
            return True
        finally:
            self._waiters.remove(fut)

    def set(self):
        if not self._value:
            self._value = True
            for fut in self._waiters:
                if not fut.done():
                    fut.set_result(True)

    def clear(self):
        self._value = False
```

套路和 lock 基本一致，不再多说了。`Condition` 可以唤醒指定数量的 `coroutine`

```Python
class Condition(_ContextManagerMixin):
    def __init__(self, lock=None, *, loop=None):
        if loop is not None:
            self._loop = loop
        else:
            self._loop = events.get_event_loop()

        if lock is None:
            lock = Lock(loop=self._loop)
        elif lock._loop is not self._loop:
            raise ValueError("loop argument must agree with lock")

        self._lock = lock
        self.locked = lock.locked
        self.acquire = lock.acquire
        self.release = lock.release

        self._waiters = collections.deque()

    @coroutine
    def wait(self):
        if not self.locked():
            raise RuntimeError('cannot wait on un-acquired lock')

        self.release()
        try:
            fut = self._loop.create_future()
            self._waiters.append(fut)
            try:
                yield from fut
                return True
            finally:
                self._waiters.remove(fut)

        finally:
            # Must reacquire lock even if wait is cancelled
            while True:
                try:
                    yield from self.acquire()
                    break
                except futures.CancelledError:
                    pass

    @coroutine
    def wait_for(self, predicate):
        result = predicate()
        while not result:
            yield from self.wait()
            result = predicate()
        return result

    def notify(self, n=1):
        if not self.locked():
            raise RuntimeError('cannot notify on un-acquired lock')

        idx = 0
        for fut in self._waiters:
            if idx >= n:
                break

            if not fut.done():
                idx += 1
                fut.set_result(False)

    def notify_all(self):
        self.notify(len(self._waiters))
```

信号量的实现

```Python
# locks.py
class Semaphore(_ContextManagerMixin):
    def __init__(self, value=1, *, loop=None):
        if value < 0:
            raise ValueError("Semaphore initial value must be >= 0")
        self._value = value
        self._waiters = collections.deque()
        if loop is not None:
            self._loop = loop
        else:
            self._loop = events.get_event_loop()

    def _wake_up_next(self):
        while self._waiters:
            waiter = self._waiters.popleft()
            if not waiter.done():
                waiter.set_result(None)
                return

    def locked(self):
        return self._value == 0

    @coroutine
    def acquire(self):
        while self._value <= 0:
            fut = self._loop.create_future()
            self._waiters.append(fut)
            try:
                yield from fut
            except:
                # See the similar code in Queue.get.
                fut.cancel()
                if self._value > 0 and not fut.cancelled():
                    self._wake_up_next()
                raise
        self._value -= 1
        return True

    def release(self):
        self._value += 1
        self._wake_up_next()
```

`Semaphore` 维持了一个内部计数器。当 `acquire()` 调用时，计数器加 1；当 `release()` 调用时，计数器减 1。计数器的值减小至 0 时会阻塞，直至有 `coroutine` 调用 `release()` 使计数器的值增加

`BoundedSemaphore` 是一个计数器大小为 1 的特殊 `Semaphore`

```Python
# locks.py
class BoundedSemaphore(Semaphore):
    def __init__(self, value=1, *, loop=None):
        self._bound_value = value
        super().__init__(value, loop=loop)

    def release(self):
        if self._value >= self._bound_value:
            raise ValueError('BoundedSemaphore released too many times')
        super().release()
```

    