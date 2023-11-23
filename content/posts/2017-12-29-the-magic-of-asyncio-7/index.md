
+++
title = "Queue — asyncio 源码剖析(7)"
summary = ''
description = ""
categories = []
tags = []
date = 2017-12-29T14:23:49+08:00
draft = false
+++

*源码基于 Python 3.6.3 的 asyncio commit sha 为 `2c5fed86e0cbba5a4e34792b0083128ce659909d`*

`Queue` 也可以用来进行同步，协调生产者和消费者。具体使用可以参考 [https://pymotw.com/3/asyncio/synchronization.html#queues](https://pymotw.com/3/asyncio/synchronization.html#queues)

```Python
# queues.py
class Queue:
    def __init__(self, maxsize=0, *, loop=None):
        if loop is None:
            self._loop = events.get_event_loop()
        else:
            self._loop = loop
        # maxsize 为 0 则无限长
        self._maxsize = maxsize

        # 存储 get 时被阻塞的 Futures.
        self._getters = collections.deque()
        # 存储 put 时被阻塞的 Futures.
        self._putters = collections.deque()
        self._unfinished_tasks = 0
        self._finished = locks.Event(loop=self._loop)
        self._finished.set()
        self._init(maxsize)

    # These three are overridable in subclasses.
    def _init(self, maxsize):
        self._queue = collections.deque()

    def _get(self):
        return self._queue.popleft()

    def _put(self, item):
        self._queue.append(item)
    # End of the overridable methods.

    def _wakeup_next(self, waiters):
        # Wake up the next waiter (if any) that isn't cancelled.
        while waiters:
            waiter = waiters.popleft()
            if not waiter.done():
                waiter.set_result(None)
                break

    def qsize(self):
        return len(self._queue)

    @property
    def maxsize(self):
        return self._maxsize

    def empty(self):
        return not self._queue

    def full(self):
        if self._maxsize <= 0:
            return False
        else:
            return self.qsize() >= self._maxsize

    @coroutine
    def put(self, item):
        while self.full():
            putter = self._loop.create_future()
            self._putters.append(putter)
            try:
                yield from putter
            except:
                putter.cancel()  # Just in case putter is not done yet.
                if not self.full() and not putter.cancelled():
                    # We were woken up by get_nowait(), but can't take
                    # the call.  Wake up the next in line.
                    self._wakeup_next(self._putters)
                raise
        return self.put_nowait(item)

    def put_nowait(self, item):
        if self.full():
            raise QueueFull
        self._put(item)
        self._unfinished_tasks += 1
        self._finished.clear()
        # 唤醒被阻塞的 get 操作
        self._wakeup_next(self._getters)

    @coroutine
    def get(self):
        while self.empty():
            getter = self._loop.create_future()
            self._getters.append(getter)
            try:
                yield from getter
            except:
                getter.cancel()  # Just in case getter is not done yet.
                if not self.empty() and not getter.cancelled():
                    # We were woken up by put_nowait(), but can't take
                    # the call.  Wake up the next in line.
                    self._wakeup_next(self._getters)
                raise
        return self.get_nowait()

    def get_nowait(self):
        if self.empty():
            raise QueueEmpty
        item = self._get()
        # 唤醒被阻塞的 put 操作
        self._wakeup_next(self._putters)
        return item

    def task_done(self):
        if self._unfinished_tasks <= 0:
            raise ValueError('task_done() called too many times')
        self._unfinished_tasks -= 1
        # 通过 Event 唤醒在 join() 的 coroutine
        if self._unfinished_tasks == 0:
            self._finished.set()

    @coroutine
    def join(self):
        if self._unfinished_tasks > 0:
            # 通过 Event.wait() 进行等待
            yield from self._finished.wait()
```

优先级队列，借助 `heapq` 实现

```Python
class PriorityQueue(Queue):
    """A subclass of Queue; retrieves entries in priority order (lowest first).
    Entries are typically tuples of the form: (priority number, data).
    """

    def _init(self, maxsize):
        self._queue = []

    def _put(self, item, heappush=heapq.heappush):
        heappush(self._queue, item)

    def _get(self, heappop=heapq.heappop):
        return heappop(self._queue)
```

LIFO 队列，其实就是栈

```Python
class LifoQueue(Queue):
    """A subclass of Queue that retrieves most recently added entries first."""

    def _init(self, maxsize):
        self._queue = []

    def _put(self, item):
        self._queue.append(item)

    def _get(self):
        return self._queue.pop()
```

*PS 感觉纯粹水了一篇*

    