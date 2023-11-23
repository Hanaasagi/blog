
+++
title = "Python threading.Barrier 的实现"
summary = ''
description = ""
categories = []
tags = []
date = 2018-03-07T08:03:47+08:00
draft = false
+++

`threading.Barrier` 是在 Python 3.2 中引入的。它作为一种线程同步机制，用于需要多个线程等待直到它们所有都到达 sync point 的场景。很惭愧，我从来没用过这玩意，所以趁此机会膜一下

示例代码

```Python
import threading
import time

NUM_THREADS = 3

def worker(barrier):
    print("{} waiting for barrier with {} others".format(
        threading.current_thread().name, barrier.n_waiting
    ))
    worker_id = barrier.wait() # [1]
    print("{} after barrier {}".format(
        threading.current_thread().name, worker_id
    ))

if __name__ == '__main__':
    barrier = threading.Barrier(NUM_THREADS)

    threads = [
        threading.Thread(
            name='worker-%s' % i,
            target=worker,
            args=(barrier,),
        )
        for i in range(NUM_THREADS)
    ]

    for t in threads:
        print(t.name, 'starting')
        t.start()
        time.sleep(0.5)

    for t in threads:
        t.join()
```

运行后可以观察到，直到线程 2 到达 [1] 处时，才会唤醒先到达的线程 0 和线程 1，否则它们只能等待

```
# Output
worker-0 starting
worker-0 waiting for barrier with 0 others
worker-1 starting
worker-1 waiting for barrier with 1 others
worker-2 starting
worker-2 waiting for barrier with 2 others
worker-2 after barrier 2
worker-0 after barrier 0
worker-1 after barrier 1
```

感觉像是使用了 `Condition` 判断了等待线程的数量。翻了一下源代码，果然这个 `Barrier` 内部使用了 `Condition`

*代码取自于 Python 3.6.3 commit sha 2c5fed86e0cbba5a4e34792b0083128ce659909d*

```Python
def __init__(self, parties, action=None, timeout=None):
    self._cond = Condition(Lock())
    self._action = action
    self._timeout = timeout
    self._parties = parties
    self._state = 0 #0 filling, 1, draining, -1 resetting, -2 broken
    self._count = 0
```

- `parties` 为参与的线程数量
- `action` 是一个 `callable`，在所有线程都到达 barrier 时被调用，这个仅被调用一次
- `timeout` 用于指定线程等待时间，若超时则会抛出 `BrokenBarrierError` 异常，其为 `RuntimeError` 的子类
- `_state` 表示状态，见注释(这不是废话么)
- `_count` 用于记录已经到达 barrier 的线程数目

```Python
def wait(self, timeout=None):
    if timeout is None:
        timeout = self._timeout
    with self._cond:
        self._enter() # Block while the barrier drains.
        index = self._count
        self._count += 1
        try:
            if index + 1 == self._parties:
                # We release the barrier
                self._release()
            else:
                # We wait until someone releases us
                self._wait(timeout)
            return index
        finally:
            self._count -= 1
            # Wake up any threads waiting for barrier to drain.
            self._exit()
```

这里需要解释一下 `Barrier` 的几种状态

1) `filling` 为初始状态，代表当前还有线程没有到达 barrier
2) `draining` 代表所有线程已经到达了 barrier，开始离开此 barrier

这样可以复用一个 barrier 做多次线程同步

>The barrier can be reused any number of times for the same number of threads.

原因就在 `_enter` 中

```Python
# Block until the barrier is ready for us, or raise an exception
# if it is broken.
def _enter(self):
    while self._state in (-1, 1):
        # It is draining or resetting, wait until done
        self._cond.wait()
    #see if the barrier is in a broken state
    if self._state < 0:
        raise BrokenBarrierError
    assert self._state == 0
```

假设三个线程都到达了第一个 barrier，此时状态处于 `draining`。先执行到第二个 barrier 前的线程会先等待，直至所有的线程都已经确确实实地越过了上一个 barrier。此方法需要结合稍后提到的 `_exit` 一起来看

继续来看 `wait` 方法的实现，如果 `self._count == self._parties`  则调用 `_release` 方法进行释放。`_release` 方法首先调用 `action`，将 `_state` 置为 `draining`，然后唤醒所有在等待 `_cond` 线程

```Python
# Optionally run the 'action' and release the threads waiting
# in the barrier.
def _release(self):
    try:
        if self._action:
            self._action()
        # enter draining state
        self._state = 1
        self._cond.notify_all()
    except:
        #an exception during the _action handler.  Break and reraise
        self._break()
        raise
```

`_wait` 方法等待直到 `self._state` 不为 `0`，

根据 `wait_for` 的 [文档](https://docs.python.org/3/library/threading.html#threading.Condition.wait_for)

>The return value is the last return value of the predicate and will evaluate to False if the method timed out.

可以得知此方法返回 `False` 则说明超时

```Python
# Wait in the barrier until we are released.  Raise an exception
# if the barrier is reset or broken.
def _wait(self, timeout):
    if not self._cond.wait_for(lambda : self._state != 0, timeout):
        #timed out.  Break the barrier
        self._break()
        raise BrokenBarrierError
    if self._state < 0:
        raise BrokenBarrierError
    assert self._state == 1
```

在调用 `action` 发生异常，或者线程等待超时时都会被视为 `_break`

```Python
def _break(self):
    # An internal error was detected.  The barrier is set to
    # a broken state all parties awakened.
    self._state = -2
    self._cond.notify_all()
```

`_exit` 方法为当最后一个线程离开 barrier 时进行收尾的工作

```Python
# If we are the last thread to exit the barrier, signal any threads
# waiting for the barrier to drain.
def _exit(self):
    if self._count == 0:
        if self._state in (-1, 1):
            #resetting or draining
            self._state = 0
            self._cond.notify_all()
```

唤醒在 `_enter` 中等待的线程，开始下一轮

另外还提供了用于重置的 `reset` 方法和取消的 `abort` 方法，作用在注释中写的很明白了

```Python
def reset(self):
    """Reset the barrier to the initial state.
    Any threads currently waiting will get the BrokenBarrier exception
    raised.
    """
    with self._cond:
        if self._count > 0:
            if self._state == 0:
                #reset the barrier, waking up threads
                self._state = -1
            elif self._state == -2:
                #was broken, set it to reset state
                #which clears when the last thread exits
                self._state = -1
        else:
            self._state = 0
        self._cond.notify_all()

def abort(self):
    """Place the barrier into a 'broken' state.
    Useful in case of error.  Any currently waiting threads and threads
    attempting to 'wait()' will have BrokenBarrierError raised.
    """
    with self._cond:
        self._break()
```

### Reference
[`threading.Barrier` Source Code(Python 3.6.3)](https://github.com/python/cpython/blob/v3.6.3/Lib/threading.py#L566)

    