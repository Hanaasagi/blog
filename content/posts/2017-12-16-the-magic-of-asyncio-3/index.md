
+++
title = "Thread — asyncio 源码剖析(3)"
summary = ''
description = ""
categories = []
tags = []
date = 2017-12-16T14:15:39+08:00
draft = false
+++

*源码基于 Python 3.6.3 的 asyncio commit sha 为 `2c5fed86e0cbba5a4e34792b0083128ce659909d`*

`asyncio` 提供了线程支持，可以用来做一个简易的异步任务队列。真实情境下你可以用发送邮件等代码去替换 `compute`

```Python
# forgive the ugly function name
import asyncio
import threading
from flask import Flask

app = Flask(__name__)

async def compute(x):
    await asyncio.sleep(5)
    return 2 ** x

def print_sum(future):
    print(future.result())

def do_job(pk):
    print(threading.current_thread().getName(), pk)
    task = asyncio.tasks.ensure_future(compute(pk))
    task.add_done_callback(print_sum)

def init_worker(loop):
    asyncio.set_event_loop(loop)
    loop.run_forever()

@app.route("/jobs/<int:pk>/")
def do_jobs(pk):
    workers[pk % len(workers)].call_soon_threadsafe(do_job, pk)
    return 'exec in background'

if __name__ == '__main__':
    workers = []
    for _ in range(2):
        loop = asyncio.new_event_loop()
        workers.append(loop)
        worker = threading.Thread(target=init_worker, args=(loop,))
        worker.setDaemon(True)
        worker.start()

    app.run()
```

根据输出，可以发现我们在主线程中调用了 `loop.call_soon_threadsafe()`，在子线程中执行 `do_job()`。如果没有 GIL，那么这是完美的。好吧，我想你一定在问为什么不是直接使用 `call_soon()` 呢？如果你尝试这样去做，那么你会发现 `do_job()` 并不会执行

```Python
# base_events.py
class BaseEventLoop(events.AbstractEventLoop):
    def call_soon_threadsafe(self, callback, *args):
        """Like call_soon(), but thread-safe."""
        self._check_closed()
        if self._debug:
            self._check_callback(callback, 'call_soon_threadsafe')
        handle = self._call_soon(callback, args)
        if handle._source_traceback:
            del handle._source_traceback[-1]
        self._write_to_self()  # 1
        return handle
    # 给出 call_soon 方法来对比
    def call_soon(self, callback, *args):
        self._check_closed()
        if self._debug:
            # _check_thread() 方法会去检测当前线程是否为运行 run_forever 的线程
            # 因为不同线程可能去共享同一个 loop
            self._check_thread()
            self._check_callback(callback, 'call_soon')
        handle = self._call_soon(callback, args)
        if handle._source_traceback:
            del handle._source_traceback[-1]
        return handle
```

貌似 `call_soon_threadsafe()` 方法只比 `call_soon()` 方法多了 `_write_to_self()` 的调用。这方法是由子类 `BaseSelectorEventLoop` 进行实现的

```Python
# selector_events.py
class BaseSelectorEventLoop(base_events.BaseEventLoop):
    def _write_to_self(self):
        # This may be called from a different thread, possibly after
        # _close_self_pipe() has been called or even while it is
        # running.  Guard for self._csock being None or closed.  When
        # a socket is closed, send() raises OSError (with errno set to
        # EBADF, but let's not rely on the exact error code).
        csock = self._csock
        if csock is not None:
            try:
                csock.send(b'\0')
            except OSError:
                if self._debug:
                    logger.debug("Fail to write a null byte into the "
                                 "self-pipe socket", exc_info=True)
```

`loop` 对象在初始化时会调用 `_make_self_pipe()` 来创建一个 `socketpair`。简单来说 `socketpair` 是一个双向管道，可以用于进程间通信也可以用于线程间通信

```Python
# selector_events.py
class BaseSelectorEventLoop(base_events.BaseEventLoop):
    def __init__(self, selector=None):
        super().__init__()
        if selector is None:
            # 标准库 selectors 的 DefaultSelector() 视平台返回不同
            selector = selectors.DefaultSelector()
        self._selector = selector
        self._make_self_pipe()

    def _make_self_pipe(self):
        self._ssock, self._csock = self._socketpair()
        # signal.set_wakeup_fd 要求 fd 必须为 non-blocking mode
        self._ssock.setblocking(False)
        self._csock.setblocking(False)
        self._internal_fds += 1
        self._add_reader(self._ssock.fileno(), self._read_from_self)

    def _add_reader(self, fd, callback, *args):
        self._check_closed()
        handle = events.Handle(callback, args, self)
        try:
            # 已经注册的 fd 不能再次注册
            # 若已经注册过，则会取注册时的监听事件和 handle
            key = self._selector.get_key(fd)
        except KeyError:
            # 未注册则为此 fd 注册读监听事件
            # register 的参数分别为 (文件描述符 监听事件类型，(读触发事件，写触发时间))
            self._selector.register(fd, selectors.EVENT_READ, (handle, None))
        else:
            mask, (reader, writer) = key.events, key.data
            # 通过或运算添加读监听事件，并且替换触发的 handle
            self._selector.modify(fd, mask | selectors.EVENT_READ, (handle, writer))
            # 此 fd 若已经注册了读监听时间的 handler 则 cancel
            if reader is not None:
                reader.cancel()

# _socketpair() 方法实现在其子类，Linux 平台代码位于 unix_events.py
class _UnixSelectorEventLoop(selector_events.BaseSelectorEventLoop):
    def _socketpair(self):
        return socket.socketpair()
```

随后子线程调用 `run_forever()`，这样初始化部分就结束了。当主线程调用 `call_soon_threadsafe()`，随后执行 `csock.send(b'\0')`。这时 `socketpair` 的另一端会有读事件触发，`select` 察觉后取出 `handler`，添加到 `loop` 的 `_ready` 队列中，然后 `loop` 会取出 `_ready` 里的所有 `handler` 依次执行。我们的 `_read_from_self()` 方法便会被调用

```Python
# selector_events.py
class BaseSelectorEventLoop(base_events.BaseEventLoop):
    def _read_from_self(self):
        while True:
            try:
                data = self._ssock.recv(4096)
                if not data:
                    break
                # 此方法由子类实现
                self._process_self_data(data)
            except InterruptedError:
                continue
            except BlockingIOError:
                break
# unix_events.py
class _UnixSelectorEventLoop(selector_events.BaseSelectorEventLoop):
    def _process_self_data(self, data):
        for signum in data:
            if not signum:
                # ignore null bytes written by _write_to_self()
                continue
            self._handle_signal(signum)
```

`_process_self_data()` 方法主要用于处理信号。`asyncio` 的信号处理也利用了上面的 `socketpair`，结合 `signal.set_wakeup_fd(fd)`，当信号触发时也会向 `_csock` 写入数据，不过值为信号的整数表示。这里不再展开，由之后的文章来详细介绍。回到示例中，由于写入的是 `\0` 所以不做处理。那么说调用了这么多方法结果什么都没做？不，不是这样的。我们需要回顾一下 `loop` 的 `_run_once()` 方法

```Python
class BaseEventLoop(events.AbstractEventLoop):
    def _run_once(self):
        # 有省略
        timeout = None
        if self._ready or self._stopping:
            timeout = 0
        elif self._scheduled:
            # Compute the desired timeout.
            when = self._scheduled[0]._when
            timeout = max(0, when - self.time())

        # 有删改
        event_list = self._selector.select(timeout)
        self._process_events(event_list)
        # 有省略
```

当 `_ready` 为空时，`timeout` 为 `None`。这时会进入阻塞模式，直至某个文件描述符就绪。这便是先前使用 `call_soon()` 却不会执行 `do_job()` 的原因

请仔细回想一下，这一切操作真的是 thread safe 的么？

当你调用 `call_soon_threadsafe()` 正在执行 `#1` 时，子线程存在正在执行 `#2` 处的可能

```Python
class BaseEventLoop(events.AbstractEventLoop):
    def _call_soon(self, callback, args):
         self._ready.append(handle)  # 1
class BaseEventLoop(events.AbstractEventLoop):
    def _run_once(self):
        ntodo = len(self._ready)
        for i in range(ntodo):
            handle = self._ready.popleft()  # 2
```

也就是说存在同时修改 `loop._ready` 的情况。但是 `_ready` 的 `append()` 和 `popleft()` 是线程安全的。参考 [clarify which deque methods are thread-safe ](https://bugs.python.org/issue15329#msg199368)

>The deque's append(), appendleft(), pop(), popleft(), and len(d) operations are thread-safe in CPython.   The append methods have a DECREF at the end (for cases where maxlen has been set), but this happens after all of the structure updates have been made and the invariants have been restored, so it is okay to treat these operations as atomic.

所以可以认为 `call_soon_threadsafe()` 是线程安全的

**请注意 `loop` 对象只能由一个线程运行(`run_forever()`)**

另外有时我们想要获取子线程执行后的结果，那么我们可以借助函数 `run_coroutine_threadsafe()` 来实现

```Python
import asyncio
import threading

async def compute(x):
    await asyncio.sleep(1)
    return 2 ** x

def init_worker(loop):
    asyncio.set_event_loop(loop)
    loop.run_forever()

loop = asyncio.new_event_loop()
worker = threading.Thread(target=init_worker, args=(loop,))
worker.start()
future = asyncio.run_coroutine_threadsafe(compute(2), loop)
print(future.result(2))
```

你可能注意到了，这个返回的 `future` 和之前的不一样啊，没错这是 `concurrent.futures` 模块中的 `future`。`run_coroutine_threadsafe()` 内部将包裹了 coroutine `compute(2)` 的 `future`(严格讲是 `Task` 对象) 和一个新创建的 `concurrent.futures.Future` 的实例进行了联结。实现代码在 `asyncio.futures._chain_future`。简单来讲就是这么一件事：如果真正的 future 执行完毕则会将状态复制到作为返回值的 `future` 中。如果对作为返回值的 `future` 进行了 `cancel`，那么真正的 `future` 也会被 `cancel`。差不多和“偷梁换柱”一个意思吧(“共生”应该更贴切)

为什么这样做呢？因为 `asyncio` 本身提供的 `future` 获取 `result` 时没有 `timeout`。毕竟 `asyncio` 不需要这种功能

    