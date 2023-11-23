
+++
title = "Python 中的 multiprocess.Queue"
summary = ''
description = ""
categories = []
tags = []
date = 2019-04-21T12:07:37+08:00
draft = false
+++

本文基于 Linux, Python 3.7.2(Commit Sha `9a3ffc0492d1310ead9ce8f5ee678c26b20a338d`)

### Queue 源码

`multiprocessing.Queue` 内部基于 `multiprocessing.Pipe`，并且使用 feeder 线程将数据从 Buffer 刷到 PIPE 中

首先来看 `Queue` 的初始化

```Python
# Lib/multiprocessing/queues.py
class Queue(object):

    def __init__(self, maxsize=0, *, ctx):
        if maxsize <= 0:
            # Can raise ImportError (see issues #3770 and #23400)
            from .synchronize import SEM_VALUE_MAX as maxsize
        self._maxsize = maxsize
        # 用于进程间通信的 PIPE/socketpair
        # duplex 决定是使用单工的 PIEP，还是双工的 socket
        self._reader, self._writer = connection.Pipe(duplex=False)
        self._rlock = ctx.Lock()  # 用于读取 buffer 时的同步
        self._opid = os.getpid()
        self._wlock = ctx.Lock()  # 用于写入 buffer 时的同步
        self._sem = ctx.BoundedSemaphore(maxsize)  # 内部计数器，用于 queue size 的计算
        # For use by concurrent.futures
        self._ignore_epipe = False

        self._after_fork()

        register_after_fork(self, Queue._after_fork)

    def _after_fork(self):
        debug('Queue._after_fork()')
        self._notempty = threading.Condition(threading.Lock())  # 用于 buffer 和 feeder 线程间的同步
        self._buffer = collections.deque()  # 数据先写入 buffer，然后由 feeder 线程写入 PIPE
        self._thread = None  # feeder 线程
        self._jointhread = None
        self._joincancelled = False
        self._closed = False
        self._close = None
        self._send_bytes = self._writer.send_bytes
        self._recv_bytes = self._reader.recv_bytes
        self._poll = self._reader.poll
```

Queue 内部的 Buffer 采用的是双端队列 `collections.deque`。


再来看 `put` 方法

```Python
class Queue(object):

    def put(self, obj, block=True, timeout=None):
        assert not self._closed, "Queue {0!r} has been closed".format(self)
        if not self._sem.acquire(block, timeout):  # 等待空间, acquire 后计数器减 1
            raise Full

        with self._notempty:
            if self._thread is None:
                self._start_thread()
            self._buffer.append(obj)
            self._notempty.notify()  # buffer 中有数据了，唤醒等待的 feeder 线程
```

这里在第一次 `put` 调用的时候，会去创建 feeder 线程


```Python
class Queue(object):
    def _start_thread(self):
        debug('Queue._start_thread()')

        # Start thread which transfers data from buffer to pipe
        self._buffer.clear()
        self._thread = threading.Thread(
            target=Queue._feed,
            args=(self._buffer, self._notempty, self._send_bytes,
                  self._wlock, self._writer.close, self._ignore_epipe,
                  self._on_queue_feeder_error, self._sem),
            name='QueueFeederThread'
        )

        # 当没有其它的 daemon thread 时会自动退出，在当前情况下就是跟着主线程退出
        self._thread.daemon = True

        debug('doing self._thread.start()')
        self._thread.start()
        debug('... done self._thread.start()')

        if not self._joincancelled:
            self._jointhread = Finalize(
                self._thread, Queue._finalize_join,
                [weakref.ref(self._thread)],
                exitpriority=-5
                )

        # Send sentinel to the thread queue object when garbage collected
        self._close = Finalize(
            self, Queue._finalize_close,
            [self._buffer, self._notempty],
            exitpriority=10
            )
```

再来看 feeder 线程做了什么

```Python
class Queue(object):
    @staticmethod
    def _feed(buffer, notempty, send_bytes, writelock, close, ignore_epipe,
              onerror, queue_sem):
        debug('starting thread to feed data to pipe')
        nacquire = notempty.acquire
        nrelease = notempty.release
        nwait = notempty.wait
        bpopleft = buffer.popleft
        sentinel = _sentinel
        wacquire = writelock.acquire
        wrelease = writelock.release

        while 1:
            try:
                nacquire()
                try:
                    if not buffer:
                        nwait()
                finally:
                    nrelease()
                try:
                    while 1:
                        obj = bpopleft()
                        if obj is sentinel:
                            debug('feeder thread got sentinel -- exiting')
                            close()
                            return

                        # serialize the data before acquiring the lock
                        obj = _ForkingPickler.dumps(obj)
                        if wacquire is None:
                            send_bytes(obj)
                        else:
                            wacquire()
                            try:
                                send_bytes(obj)
                            finally:
                                wrelease()
                except IndexError:
                    pass
            except Exception as e:
                if ignore_epipe and getattr(e, 'errno', 0) == errno.EPIPE:
                    return
                # Since this runs in a daemon thread the resources it uses
                # may be become unusable while the process is cleaning up.
                # We ignore errors which happen after the process has
                # started to cleanup.
                if is_exiting():
                    info('error in queue thread: %s', e)
                    return
                else:
                    # Since the object has not been sent in the queue, we need
                    # to decrease the size of the queue. The error acts as
                    # if the object had been silently removed from the queue
                    # and this step is necessary to have a properly working
                    # queue.
                    queue_sem.release()
                    onerror(e, obj)
```

feeder 线程首先通过条件锁 `notempty` 进行同步，确保满足 buffer 非空。然后不断从 buffer 取出对象，如果是 `sentinel` 对象则退出，否则通过 `pickle` 序列化对象然后尝试获取 PIPE 的写入锁发送到 PIPE 中


我们知道 `multiprocessing` 模块会自动注册 `atexit` 的 hook，当进程退出的时候会去执行 `_exit_function` 函数，这一点可以参考 [multiprocessing/utils.py#L327](https://github.com/python/cpython/blob/3e986de0d65e78901b55d4e500b1d05c847b6d5e/Lib/multiprocessing/util.py#L327)


在 `Queue` 的 `_start_thread` 方法中除了开启了 feeder 线程外，还做了两件事情

```Python
if not self._joincancelled:
    self._jointhread = Finalize(
        self._thread, Queue._finalize_join,
        [weakref.ref(self._thread)],
        exitpriority=-5
        )

# Send sentinel to the thread queue object when garbage collected
self._close = Finalize(
    self, Queue._finalize_close,
    [self._buffer, self._notempty],
    exitpriority=10
    )
```

`Finalize` 通过 `weakref` 为对象添加 finalization 操作，具体是执行 `callback` 中的参数。对于 `_jointhread` 来说便是 `Queue._finalize_join`

```Python
class Queue:
    @staticmethod
    def _finalize_join(twr):
        debug('joining queue thread')
        thread = twr()  # 传入 thread 对象的 weakref
        if thread is not None:
            thread.join()
            debug('... queue thread joined')
        else:
            debug('... queue thread already dead')
```

达到在进程退出的时候自动去 join feeder 线程的目的。


所有的 `Finalizer` 对象都会被添加到 `_finalizer_registry` 中，当一旦调用其的 `__call__` 后，会自动从 `_finalizer_registry` 中移除。所以这里手动调用 `self._close` 是不会存在多次执行的

```Python
class Queue:
    @staticmethod
    def _finalize_close(buffer, notempty):
        debug('telling queue thread to quit')
        with notempty:
            buffer.append(_sentinel)
            notempty.notify()
```

`_finalize_close` 方法用于向 buffer 中添加 sentinel，通知 feeder 线程自行退出。这里并没有先调用 `_finalize_join`，然后 feeder 线程等待 sentinel 产生死锁的问题。因为这 `Finalizer` 提供了优先级机制(`exitpriority` 参数)，`_finalize_close` 一定会比 `_finalize_join` 先调用

Queue 也提供了在进程退出时不去等待 buffer 中的数据全部被 feeder 线程刷到 buffer 中的 `cancel_join_thread` 方法

```Python
class Queue(object):
    def cancel_join_thread(self):
        debug('Queue.cancel_join_thread()')
        self._joincancelled = True
        try:
            self._jointhread.cancel()
        except AttributeError:
            pass
```

其实这里就是调用了 `Finalizer` 的 `cancel` 方法，从 `_finalizer_registry` 中移除，这样在进程结束的时候就不会执行 `_finalize_join` 了。此处需要关联一下我们在启动 feeder 线程的时候是设置了 daemon 的，这样便会平滑退出了


最后我们在最后来看一下 `get` 的实现

```Python
def get(self, block=True, timeout=None):
    if self._closed:
        raise ValueError(f"Queue {self!r} is closed")
    if block and timeout is None:
        with self._rlock:
            res = self._recv_bytes()
        self._sem.release()
    else:
        if block:
            deadline = time.monotonic() + timeout
        if not self._rlock.acquire(block, timeout):
            raise Empty
        try:
            if block:
                timeout = deadline - time.monotonic()
                if not self._poll(timeout):
                    raise Empty
            elif not self._poll():
                raise Empty
            res = self._recv_bytes()
            self._sem.release()
        finally:
            self._rlock.release()
    # unserialize the data after having released the lock
    return _ForkingPickler.loads(res)
```

第一点使用 `monotonic` 避免系统时间漂移，第二点我们注意到在 `block` 且指定了 `timeout` 的情况下，我们去拿读锁，拿到之后依然去 `_poll` 检查了一下是否有数据存在，直到耗尽 `timeout` 或者读到了数据。`block` 没有 `timeout` 的时候仅 `_recv_bytes` 即可，它会阻塞到直到数据可读

好了还有最后一点需要提及，我们向 PIPE 中发送的是通过 `pickle` 序列化后的数据。那么我们读的时候，读到什么时候才算做一个完整的序列化呢？这里我么需要看一下 `_send_bytes` 和 `_recv_bytes` 的实现

```Python
# # Lib/multiprocessing/connection.py
class Connection(_ConnectionBase):
    def _send_bytes(self, buf):
        n = len(buf)
        if n > 0x7fffffff:
            pre_header = struct.pack("!i", -1)
            header = struct.pack("!Q", n)
            self._send(pre_header)
            self._send(header)
            self._send(buf)
        else:
            # For wire compatibility with 3.7 and lower
            header = struct.pack("!i", n)
            if n > 16384:
                # The payload is large so Nagle's algorithm won't be triggered
                # and we'd better avoid the cost of concatenation.
                self._send(header)
                self._send(buf)
            else:
                # Issue #20540: concatenate before sending, to avoid delays due
                # to Nagle's algorithm on a TCP socket.
                # Also note we want to avoid sending a 0-length buffer separately,
                # to avoid "broken pipe" errors if the other end closed the pipe.
                self._send(header + buf)

    def _recv_bytes(self, maxsize=None):
        buf = self._recv(4)
        size, = struct.unpack("!i", buf.getvalue())
        if size == -1:
            buf = self._recv(8)
            size, = struct.unpack("!Q", buf.getvalue())
        if maxsize is not None and size > maxsize:
            return None
        return self._recv(size)
```

这里相当于自定义了一套协议，header 里面会存放下一个(序列化数据)的大小

下面针对 `Queue` 做了一下小实验

### PIPE Buffer Size

测试 Linux PIPE 大小

```Python
import os


for size in [65535, 65536, 65537]:
    text = b'\x00' * size
    read_fd, write_fd = os.pipe()
    with os.fdopen(write_fd, 'wb') as f:
        f.write(text)
    print(f'size {size} pass.')

# Output:
# size 65535 pass.
# size 65536 pass.
# block
```

修改 PIPE Buffer 大小，参考 HuangHuang 菊苣的[Blog](https://mozillazg.com/2017/11/python-how-to-set-pipe-buffer-size-on-linux.html)

```Python
import os
import fcntl
import platform


def modify_pipe_size(fd, size):
    try:
        if platform.system() == 'Linux':
            # Python 的 fcntl 没有 F_SETPIPE_SZ 属性，所以我们定义了这个属性，它的值来自 fcntl.h
            fcntl.F_SETPIPE_SZ = 1031
            fcntl.fcntl(fd, fcntl.F_SETPIPE_SZ, size)
    except IOError:
        print('can not change PIPE buffer size')


text = b'\x00' * 65537
read_fd, write_fd = os.pipe()
modify_pipe_size(read_fd, 65537)
modify_pipe_size(write_fd, 65537)

with os.fdopen(write_fd, 'wb') as f:
    f.write(text)
```

根据文档，如果我们父进程在去 join，而子进程还在向 PIPE 中刷数据并且阻塞住了的时候便会发生死锁

> As mentioned above, if a child process has put items on a queue (and it has not used `JoinableQueue.cancel_join_thread`), then that process will not terminate until all buffered items have been flushed to the pipe.
> This means that if you try joining that process you may get a deadlock unless you are sure that all items which have been put on the queue have been consumed. Similarly, if the child process is non-daemonic then the parent process may hang on exit when it tries to join all its non-daemonic children.


一个典型的复现示例

```Python
import pickle
from multiprocessing import Process, Queue

text = bytearray(b'\x00' * (65536))


def f(q):
    size = len(pickle.dumps(text))
    print(f'size: {size}')  # only block if size > 65536
    q.put(text)


if __name__ == '__main__':
    q = Queue()
    p = Process(target=f, args=(q,))
    p.start()
    p.join()
    q.get()
```

通过 `strace` 跟踪系统调用，可以看到

```
# 30593 为父进程， 30644 为子进程
[pid 30644] futex(0x7fd238000d50, FUTEX_WAIT_BITSET_PRIVATE|FUTEX_CLOCK_REALTIME, 0, NULL, FUTEX_BITSET_MATCH_ANY <unfinished ...>                                                           
[pid 30593] wait4(30644,
```

在 Glibc 中， `pthread_join` 也是用 `futex` 系统调用实现的。所以我们可以看到子进程在 hang 在 join feeder 线程，而 feeder 线程又在等待 PIPE 可写，父进程则又在等待子进程结束的情况

在来一个问题的变种，通过使用 `cancel_join_thread`，阻止后台线程在进程退出的时候自动将 buffer 中的数据冲到 PIPE 中，不过这会造成数据不完整。

比如下例

```Python
import pickle
from multiprocessing import Process, Queue

text = bytearray(b'\x00' * (65536))


def f(q):
    size = len(pickle.dumps(text))
    print(f'size: {size}')
    q.cancel_join_thread()
    q.put(text)


if __name__ == '__main__':
    q = Queue()
    p = Process(target=f, args=(q,))
    p.start()
    p.join()
    print(f'after join, queue size {q.qsize()}')
    q.get()
    print('exit')
```

我不在意数据是否完整，我希望当读取到一个不完整的数据直接抛出异常，然后我跳过它。但是事实上这里直接 hang 住了。父进程最后的系统调用是这样的

```
write(1, "after join, queue size 1\n", 25after join, queue size 1
) = 25
read(3, 0x7f28f7a8d050, 4)              = ? ERESTARTSYS (To be restarted if SA_RESTART is set)
--- SIGWINCH {si_signo=SIGWINCH, si_code=SI_KERNEL} ---
read(3, ^C0x7f28f7a8d050, 4)              = ? ERESTARTSYS (To be restarted if SA_RESTART is set)
```

这是因为子进程写入了 header，但是根据协议父进程在 `q.get` 的底层调用 `_recv_bytes` 的时候会去读取指定的字节，但是其实子进程并没有写入那么多的数据，所以 hang 住了

### Reference
[Python: linux 环境下如何修改 PIPE buffer size](https://mozillazg.com/2017/11/python-how-to-set-pipe-buffer-size-on-linux.html)  
[Pthread Linux中的线程同步机制](https://www.cnblogs.com/tangr206/articles/3097553.html)

    