
+++
title = "异步的魔法 — asyncio 源码剖析(1)"
summary = ''
description = ""
categories = []
tags = []
date = 2017-12-12T13:53:00+08:00
draft = false
+++

*源码基于 Python 3.6.3 的 asyncio commit sha 为 `2c5fed86e0cbba5a4e34792b0083128ce659909d`*

`asyncio` 主要做了一件事：协程的调度。协程是由 Python 提供的支持。调度则是通过 `select`/`epoll` 等系统调用结合自身的队列进行实现

本系列文章到此结束(笑)

`asyncio` 的代码实现有些复杂，很多实例都将自己委身于函数或者方法中，比如 `loop`。每个 `Task` 或 `Future` 的实例都携带了 `loop`，然后又将自己想要执行的任务放到 `loop` 的队列中。还有 `Future` 实例注册 callback，然后将自己作为 callback 参数传入的情况。这种实现比较乱，而且有一个严重的后果，那就是循环引用。`asyncio` 代码中有些地方使用了 `self = None` 来对此进行规避。鉴于本文不是用来吐槽的，所以回归正题。先来形象地理解一下 `asyncio`

请想象面前有一条流水线，流水线旁有一个名为 `select` 的搬运工，负责向流水线上扔 coroutine。coroutine 是一个神奇的东西，你只需要对其进行充电，它便能进行某些内部操作，适当的情况下还会向外部发出信号(有几个指示灯)。更高级地是，它断电不会丢失工作状态，充电后便能恢复作业。由于目前的贫穷现状，只有一个电源放在了流水线的尽头。首先你将一个 coroutine (称为 tosaka 吧)放到上面，然后启动流水线，这个 coroutine 便移动到了流水线的尽头，流水线上的其他 coroutine 不被受理。突然 coroutine 的指示灯亮了，显示其需要等待外部的某个资源，请你先对其他的 coroutine 进行充电吧。coroutine 同时也通知了搬运工 `select`，如果有 xxx 出现，就将我再次放到流水线上去。整个流水线原则上是不允许插队的，但是因为某些 coroutine 的需求很紧急，那么它们可以插队。接着流水线便一个接一个的充电。没有 coroutine 怎么办，搬运工 `select` 会主动帮你停下流水线，为公司着想的青年。平稳的日常在搬运工 `select` 发现了 xxx 事件后被打破，它将 tosaka 重新放在流水线上，然后排队进行充电。tosaka 恢复执行，并且结束时要求我还需要执行一些东西，它叫 callback。真是一个任性的 coroutine

上面我模糊了一些概念，如 `Future`、`Task`、`Handle` 等。而且在下文中也不会具体的涉及具体实现，毕竟这是第一篇。接下来的内容便十分的枯燥了，下面是摘自 [文档](https://docs.python.org/3/library/asyncio-task.html#example-chain-coroutines) 的一个例子

```Python
import asyncio

async def compute(x, y):
    print("Compute %s + %s ..." % (x, y))
    await asyncio.sleep(1.0)
    return x + y

async def print_sum(x, y):
    result = await compute(x, y)
    print("%s + %s = %s" % (x, y, result))

loop = asyncio.get_event_loop()
loop.run_until_complete(print_sum(1, 2))
loop.close()
```

如果这是一个同步的代码，那么只要这样即可(迷之突起)

```
print_sum
  |
   \   compute
     \
      |
     /
   /
  |
```

但是在 `asyncio` 的异步下，执行流程如下图(两个迷之突起)

<img style="max-width:820px;" src="../../images/2017/12/tulip_coro.png" />

首先来看 `asyncio.get_event_loop()` 发生了什么

```Python
# events.py

# A TLS for the running event loop, used by _get_running_loop.
class _RunningLoop(threading.local):
    loop_pid = (None, None)

_running_loop = _RunningLoop()

def _get_running_loop():
    running_loop, pid = _running_loop.loop_pid
    if running_loop is not None and pid == os.getpid():
        return running_loop

def get_event_loop_policy():
    # 模块级变量 _event_loop_policy 的默认值为 None
    # 可通过 set_event_loop_policy() 指定其值，比如你可以使用 uvloop
    if _event_loop_policy is None:
        # 初始化 _event_loop_policy，根据不同的 platform 有不同的值
        # 比如 Linux 下则是位于 unix_events.py 的 _UnixDefaultEventLoopPolicy 实例
        _init_event_loop_policy()
    return _event_loop_policy

def get_event_loop():
    current_loop = _get_running_loop()
    if current_loop is not None:
        return current_loop
    return get_event_loop_policy().get_event_loop()  # 01
```

这部分代码主要是用于获取当前进程/线程中的 `event_loop`。当第一次调用时，会调用对应 policy 的 `get_event_loop()` 方法(即 01 处)。反正就是为了平台兼容做了一个策略模式，饶了一个弯

```
BaseDefaultEventLoopPolicy 位于 events.py
|--------- _UnixDefaultEventLoopPolicy 位于 unix_events.py
|--------- _WindowsDefaultEventLoopPolicy 位于 windows_events.py
```

`policy` 实例的 `get_event_loop()` 方法实现位于基类 `BaseDefaultEventLoopPolicy` 中

```Python
# events.py
class BaseDefaultEventLoopPolicy(AbstractEventLoopPolicy):
    """Default policy implementation for accessing the event loop.
    In this policy, each thread has its own event loop. However, we
    only automatically create an event loop by default for the main
    thread; other threads by default have no event loop.
    Other policies may have different rules (e.g. a single global
    event loop, or automatically creating an event loop per thread, or
    using some other notion of context to which an event loop is
    associated).
    """
    _loop_factory = None

    class _Local(threading.local):
        _loop = None
        _set_called = False

    def __init__(self):
        self._local = self._Local()

    def get_event_loop(self):
        if (self._local._loop is None and
            not self._local._set_called and
            isinstance(threading.current_thread(), threading._MainThread)):  # 01
            self.set_event_loop(self.new_event_loop())
        if self._local._loop is None:
            raise RuntimeError('There is no current event loop in thread %r.'
                               % threading.current_thread().name)
        return self._local._loop

    def set_event_loop(self, loop):
        self._local._set_called = True
        assert loop is None or isinstance(loop, AbstractEventLoop)
        self._local._loop = loop

    def new_event_loop(self):
        return self._loop_factory()  # 02
```

01 处约束了当前线程为主线程的情况下才会设置 `event_loop`。注意这里的 `set_event_loop()` 等方法子类进行了 override，添加了 watcher。`_loop_factory` 则由子类进行实现

```Python
# unix_events.py
class _UnixDefaultEventLoopPolicy(events.BaseDefaultEventLoopPolicy):
    _loop_factory = _UnixSelectorEventLoop
    # 省略
```

所以说 `_UnixSelectorEventLoop` 是 Linux 下 `event_loop` 的真正实现。因为考虑文章篇幅，所以这里不在展开

既然得到了 `loop` 对象，那么根据示例代码，接下来是调用了 `loop` 对象的 `run_until_complete()` 方法。这里会涉及 `asyncio` 的核心部分——事件循环。`run_until_complete` 的实现位于其基类 `BaseSelectorEventLoop` 的基类 `BaseEventLoop` 中

```Python
# base_events.py
class BaseEventLoop(events.AbstractEventLoop):

    def run_until_complete(self, future):
        # 如果 event_loop 已经 close 则会抛出 RuntimeError
        self._check_closed()

        # isfuture 的实现位于 base_futures.py 中
        # 是否为 future 取决于对象具有 _asyncio_future_blocking 且值非 None
        # 而不是简单的使用 isinstance，主要为了 duck typing
        new_task = not futures.isfuture(future)
        future = tasks.ensure_future(future, loop=self)  # 01
        if new_task:
            # An exception is raised if the future didn't complete, so there
            # is no need to log the "destroy pending task" message
            future._log_destroy_pending = False

        future.add_done_callback(_run_until_complete_cb)  # 02
        # 这里忽略了异常处理
        self.run_forever()
        if not future.done():
            raise RuntimeError('Event loop stopped before Future completed.')
        return future.result()
```

`01` 处的代码实现

```Python
# tasks.py
def ensure_future(coro_or_future, *, loop=None):
    # 已经是 future 的情况下不能再绑定不同的 loop
    if futures.isfuture(coro_or_future):
        if loop is not None and loop is not coro_or_future._loop:
            raise ValueError('loop argument must agree with Future')
        return coro_or_future
    elif coroutines.iscoroutine(coro_or_future):
        if loop is None:
            loop = events.get_event_loop()
        task = loop.create_task(coro_or_future)
        if task._source_traceback:
            del task._source_traceback[-1]
        return task
    elif compat.PY35 and inspect.isawaitable(coro_or_future):
        return ensure_future(_wrap_awaitable(coro_or_future), loop=loop)
    else:
        raise TypeError('An asyncio.Future, a coroutine or an awaitable is required')

@coroutine
def _wrap_awaitable(awaitable):
    return (yield from awaitable.__await__())
```

我们可以发现使用 `async def` 定义的函数最终也要被 `coroutine` 装饰器装饰。另外，`ensure_future()` 函数对于参数为 `future` 和 `coroutine` 表现的行为不同，换句话说这个函数有可能返回 `Future` 对象也有可能返回 `Task` 对象，不过这没有关系因为 `Task` 是 `Future` 的子类。下面是 `loop.create_task` 的实现，调用的是基类的方法

```Python
# base_events.py
class BaseEventLoop(events.AbstractEventLoop):
    def create_task(self, coro):
        self._check_closed()
        if self._task_factory is None:
            task = tasks.Task(coro, loop=self)
            if task._source_traceback:
                del task._source_traceback[-1]
        else:
            task = self._task_factory(self, coro)
        return task
```

可以看到一个 `Task` 包含了 `loop` 自身。这里就不展开 `Future`/`Task` 的实现了。不过你需要知道可以通过 `task_obj._loop` 来获取其所绑定的 `loop`

`Task` 的 `__init__()` 方法中，调用了 `loop` 对象的 `call_soon()` 方法，传入了自身了 `_step()` 方法。`loop.call_soon()` 则会将其包裹成了 `handle` 对象，然后加入了 `loop._ready` 中。`loop._ready` 实际上是一个双端队列 `deque`。这里不展开了

`02` 处的 `add_done_callback()` 所注册的回调函数 `_run_until_complete_cb` 依赖了 `_loop`，完成了 `loop` 的 stop。根据函数签名我们也可以看出回调可以以 `future` 作为参数

```Python
# base_events.py
def _run_until_complete_cb(fut):
    # 省略
    fut._loop.stop()  # loop_obj._stopping = True
```

有了这一点我们也能够明白为什么 `run_until_complete` 实际上调用的是 `run_forever`

```Python
# base_events.py
class BaseEventLoop(events.AbstractEventLoop):
    def run_forever(self):
        # 省略大量代码
        events._set_running_loop(self)
        while True:
            self._run_once()
            if self._stopping:
                break
        events._set_running_loop(None)
```

`run_forever` 方法设置了 `_running_loop` 并且一次一次的调用 `_run_once()` 方法。`_run_once()` 完成了核心的任务调度，即通过不断的循环调用 `select`/`epoll` 等系统调用，得出被唤醒的 `fd` 然后将对应的 `handler` 放入队列中。那么你肯定会问，我们的示例代码中并没有 `fd` 啊。没有 `fd` 便会返回一个空的 `event_list`。也可以通过其他方式进行入队，比如之前的 `call_soon`。下面是 `_run_once()` 方法的实现：

```Python
# base_events.py
class BaseEventLoop(events.AbstractEventLoop):
    def _run_once(self):
        """Run one full iteration of the event loop.
        This calls all currently ready callbacks, polls for I/O,
        schedules the resulting callbacks, and finally schedules
        'call_later' callbacks.
        """
        # 代码有删改
        sched_count = len(self._scheduled)
        if (sched_count > _MIN_SCHEDULED_TIMER_HANDLES and
            self._timer_cancelled_count / sched_count >
                _MIN_CANCELLED_TIMER_HANDLES_FRACTION):
            # Remove delayed calls that were cancelled if their number is too high
            new_scheduled = []
            for handle in self._scheduled:
                if handle._cancelled:
                    handle._scheduled = False
                else:
                    new_scheduled.append(handle)

            heapq.heapify(new_scheduled)
            self._scheduled = new_scheduled
            self._timer_cancelled_count = 0
        else:
            # Remove delayed calls that were cancelled from head of queue.
            while self._scheduled and self._scheduled[0]._cancelled:
                self._timer_cancelled_count -= 1
                handle = heapq.heappop(self._scheduled)
                handle._scheduled = False

        timeout = None
        if self._ready or self._stopping:
            timeout = 0
        elif self._scheduled:
            # Compute the desired timeout.
            when = self._scheduled[0]._when
            timeout = max(0, when - self.time())

        event_list = self._selector.select(timeout)
        # 会将任务添加到 _ready 队列中
        self._process_events(event_list)

        # Handle 'later' callbacks that are ready.
        end_time = self.time() + self._clock_resolution
        while self._scheduled:
            handle = self._scheduled[0]
            if handle._when >= end_time:
                break
            handle = heapq.heappop(self._scheduled)
            handle._scheduled = False
            self._ready.append(handle)

        # This is the only place where callbacks are actually *called*.
        # All other places just add them to ready.
        ntodo = len(self._ready)
        for i in range(ntodo):
            handle = self._ready.popleft()
            if handle._cancelled:
                continue
            handle._run()
        handle = None  # Needed to break cycles when an exception occurs.
```

`_ready` 是一个很重要的队列，所有的得到调度(将要执行)的 `Task` 都会被放到这里，然后依次执行。对于我们的示例代码说，第一次 `_ready` 队列中只有一个任务，那便是包裹了 `Task._step()` 方法的 `Handle` 对象实例。`handle._run()` 会去调用 `Task._step()` 方法。其主要是调用了 `result = coro.send(None)`，`coro` 对象即为我们的被装饰为 `coroutine` 的 `print_sum`

```Python
# tasks.py
class Task(futures.Future):
    def _step(self, exc=None):
        coro = self._coro
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
        # 省略大量代码
```

那么 `result` 是什么呢？需要查看一下 `sleep()` 的源代码

```Python
# tasks.py
@coroutine
def sleep(delay, result=None, *, loop=None):
    """Coroutine that completes after a given time (in seconds)."""
    if delay == 0:
        yield
        return result

    if loop is None:
        loop = events.get_event_loop()
    future = loop.create_future()
    h = future._loop.call_later(delay, futures._set_result_unless_cancelled,
                                future, result)
    try:
        return (yield from future)
    finally:
        h.cancel()
```

它生成了一个 `Future` 对象，并调用了 `loop` 的 `call_later` 方法，在 1 秒之后调用 `futures._set_result_unless_cancelled`。它会为 `future` 赋值 `result`。`call_later` 内部调用了 `loop` 的 `call_at()` 方法，会在 `loop` 的 `_scheduled` 队列中放置一个包裹了回调函数 `futures._set_result_unless_cancelled` 的特殊 `Handle`——`TimerHandle` 对象。注意这里的 `yield from future` 会调用 `future` 对象的 `__iter__` 方法

```Python
# futures.py
class Future:
    def __iter__(self):
        if not self.done():
            self._asyncio_future_blocking = True
            yield self  # This tells Task to wait for completion.
        return self.result() # May raise too.
```

可以看到如果状态非 `DONE` 则会 `yield` 自身。`_step()` 方法会对得到的这个 `future` 添加 `self._wakeup` 这个回调函数(上面代码略去了这一部分)

接着便再次回到事件循环 `_run_once` 当中，此时将要执行的便是 `TimerHandle` 的 `_run()` 方法，触发回调 `futures._set_result_unless_cancelled` 为 `future` 设置结果并将其状态置为 `_FINISHED`。这里会对此 `future` 的所有 callback 进行调度

```Python
# futures.py
def _set_result_unless_cancelled(fut, result):
    """Helper setting the result only if the future was not cancelled."""
    if fut.cancelled():
        return
    fut.set_result(result)

class Future:
    def _schedule_callbacks(self):
        callbacks = self._callbacks[:]
        if not callbacks:
            return
        self._callbacks[:] = []
        for callback in callbacks:
            self._loop.call_soon(callback, self)  # 将 future 自身传入
```

具体而言是将我们刚才在 `_step()` 中注册的 `_wakeup()` 方法通过 `call_soon()` 进行调度。所以当我们再次回到事件循环时我们会执行 `Task._wakup()`，可以简单地理解这个方法再度调用了 `_step()`。此时再次激活了我们的 `coroutine` `print_sum`。对于示例代码 coroutine 而言，现在已经 exhausted，会抛出 `StopIteration` 异常。`_step()` 方法捕捉到此一样后会执行 `self.set_result(exc.value)`，进而执行 `_schedule_callbacks()` 方法将自身的 callback `_run_until_complete_cb()` 进行调度。正如我们之前所分析的，`loop` 对象的 `close()` 被调用，`run_forever` 被结束，接着程序结束

    