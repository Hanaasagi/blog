
+++
title = "Python 3.6 下无法通过 spawn context 在子进程运行包含 logger 的 callable"
summary = ''
description = ""
categories = []
tags = []
date = 2019-03-23T08:28:05+08:00
draft = false
+++

本文如无特殊说明，所使用的 Python 版本指 3.6.8，如果你已经了解 https://bugs.python.org/issue30520 那么便不需要继续阅读了

这个事情要从 uvicorn 的一个 issue 说起 [#328](https://github.com/encode/uvicorn/issues/328)

```Python
# run.py
import uvicorn
import logging

logging.basicConfig(level=logging.DEBUG)


if __name__ == '__main__':
    uvicorn.run(
        'app:app',
        logger=logging.getLogger("uvicorn"),
        reload=True
    )
    
# app.py
async def app(scope, receive, send):
    await send({
        'type': 'http.response.start',
        'status': 200,
        'headers': [
            [b'content-type', b'text/plain'],
        ],
    })
    await send({
        'type': 'http.response.body',
        'body': b'Hello, world!',
    })
```

这个启动的时候会 crash

```
Traceback (most recent call last):
  File "start.py", line 11, in <module>
    reload=True
  File "/usr/local/lib/python3.6/site-packages/uvicorn/main.py", line 270, in run
    supervisor.run(server.run, sockets=[socket])
  File "/usr/local/lib/python3.6/site-packages/uvicorn/supervisors/statreload.py", line 34, in run
    process.start()
  File "/usr/local/lib/python3.6/multiprocessing/process.py", line 105, in start
    self._popen = self._Popen(self)
  File "/usr/local/lib/python3.6/multiprocessing/context.py", line 284, in _Popen
    return Popen(process_obj)
  File "/usr/local/lib/python3.6/multiprocessing/popen_spawn_posix.py", line 32, in __init__
    super().__init__(process_obj)
  File "/usr/local/lib/python3.6/multiprocessing/popen_fork.py", line 19, in __init__
    self._launch(process_obj)
  File "/usr/local/lib/python3.6/multiprocessing/popen_spawn_posix.py", line 47, in _launch
    reduction.dump(process_obj, fp)
  File "/usr/local/lib/python3.6/multiprocessing/reduction.py", line 60, in dump
    ForkingPickler(file, protocol).dump(obj)
TypeError: can't pickle _thread.RLock objects
```

因为在 reload 模式下, uvicorn 会创建一个新的进程去运行 Server，而父进程则是去不断检查文件的状态然后决定是否去停止当前的进程然后再创建一个进程执行 [`Server.run` ](https://github.com/encode/uvicorn/blob/e7d2a1b6fc7781b9236b7aa2716876d20c13ebe6/uvicorn/main.py#L300)，这里则会去重新 [`import` app](https://github.com/encode/uvicorn/blob/e7d2a1b6fc7781b9236b7aa2716876d20c13ebe6/uvicorn/main.py#L309)。这是常见的 reload 模式实现。

```Python
# uvicorn/supervisors/statreload.py
class StatReload:
    def run(self, target, *args, **kwargs):
        pid = os.getpid()
        logger = self.config.logger_instance

        logger.info("Started reloader process [{}]".format(pid))

        for sig in HANDLED_SIGNALS:
            signal.signal(sig, self.handle_exit)

        spawn = multiprocessing.get_context("spawn")
        process = spawn.Process(target=target, args=args, kwargs=kwargs)
        process.start()

        while process.is_alive() and not self.should_exit:
            time.sleep(0.1)
            if self.should_restart():  # check st_mtime
                self.clear()
                os.kill(process.pid, signal.SIGTERM)
                process.join()
                process = spawn.Process(target=target, args=args, kwargs=kwargs)
                process.start()
                self.reload_count += 1

        logger.info("Stopping reloader process [{}]".format(pid))
```

但是注意这里 `run` 方法的 `target` 参数传入的时 `Server.run` ，`Server` 对象包含一个成员 `Config`，`Config`则又包含了 `logger`。现在让我们来简化一下代码来更加清楚的呈现问题的原因

```Python
import logging
import multiprocessing

logging.basicConfig(level=logging.DEBUG)


class T:
    def __init__(self, logger):
        self.logger = logger

    def run(self):
        print(self.logger)


if __name__ == '__main__':
    spawn = multiprocessing.get_context("spawn")
    process = spawn.Process(target=T(logging.getLogger("test")).run)
    process.start()
    process.join()
```

首先要声明两点

- 上面的这段代码在 Python 3.7  中是没有问题的，但是在之前的版本应该都会有这个问题。至于 3.7 中为什么没问题，后面会提到
- 如果不使用 `spawn`也是没有**这个**问题的(但是可能存在其他的问题)

```Python
if __name__ == '__main__':
    # spawn = multiprocessing.get_context("spawn")
    # 默认 fork
    process = multiprocessing.Process(target=T(logging.getLogger("test")).run)
    process.start()
    process.join()
```

关于 `spawn` 和 `fork` 参考[文档](https://docs.python.org/3/library/multiprocessing.html#contexts-and-start-methods)

- *spawn*

The parent process starts a fresh python interpreter process.  The child process will only inherit those resources necessary to run the process objects [`run()`](https://docs.python.org/3/library/multiprocessing.html#multiprocessing.Process.run) method.  In particular, unnecessary file descriptors and handles from the parent process will not be inherited.  Starting a process using this method is rather slow compared to using *fork* or *forkserver*.

Available on Unix and Windows.  The default on Windows.

- *fork*

The parent process uses [`os.fork()`](https://docs.python.org/3/library/os.html#os.fork) to fork the Python interpreter.  The child process, when it begins, is effectively identical to the parent process.  All resources of the parent are inherited by the child process.  Note that safely forking a multithreaded process is problematic.

Available on Unix only.  The default on Unix.

可以知道子进程仅会从父进程那里继承一些必要的资源，比如我们的 `Server`。这是通过 `pickle` 传递的

```Python
# https://github.com/python/cpython/blob/3c6b436a57893dd1fae4e072768f41a199076252/Lib/multiprocessing/popen_spawn_posix.py#L47
class Popen(popen_fork.Popen):
    def _launch(self, process_obj):
        from . import semaphore_tracker
        tracker_fd = semaphore_tracker.getfd()
        self._fds.append(tracker_fd)
        prep_data = spawn.get_preparation_data(process_obj._name)
        fp = io.BytesIO()
        set_spawning_popen(self)
        try:
            reduction.dump(prep_data, fp)
            reduction.dump(process_obj, fp)
        finally:
            set_spawning_popen(None)

        parent_r = child_w = child_r = parent_w = None
        try:
            parent_r, child_w = os.pipe()
            child_r, parent_w = os.pipe()
            cmd = spawn.get_command_line(tracker_fd=tracker_fd,
                                         pipe_handle=child_r)
            self._fds.extend([child_r, child_w])
            self.pid = util.spawnv_passfds(spawn.get_executable(),
                                           cmd, self._fds)
            self.sentinel = parent_r
            with open(parent_w, 'wb', closefd=False) as f:
                f.write(fp.getbuffer())
        finally:
            if parent_r is not None:
                util.Finalize(self, os.close, (parent_r,))
            for fd in (child_r, child_w, parent_w):
                if fd is not None:
                    os.close(fd)
```



我们通过 `multiprocess` 的接口创建了 `Process` 对象(这是在父进程中的操作，然后调用 `run` 方法，创建进程然后执行我们传入的 callable。在 `spawn` 下是通过 fork/exec 创建了一个新的 Python 进程(执行`sys.executable`)。`callable` 等必要的数据则是通过 `pipe` 进程传输的，这也是常见的进程通信的方法。详细的便是父进程先将数据通过 `pickle` 进行序列化，写入 pipe 中，子进程则从 fd 中读出然后反序列化。`fork` 则是不一样的子进程会自动拥有父进程所有数据副本，但是这也同样造成了更加令人困扰的问题。多线程环境下的 fork，由于父进程的锁状态也会被 copy，可能会形成死锁

```Python
# https://github.com/python/cpython/blob/3c6b436a57893dd1fae4e072768f41a199076252/Lib/multiprocessing/spawn.py#L109
def _main(fd):
    with os.fdopen(fd, 'rb', closefd=True) as from_parent:
        process.current_process()._inheriting = True
        try:
            preparation_data = reduction.pickle.load(from_parent)
            prepare(preparation_data)
            self = reduction.pickle.load(from_parent)
        finally:
            del process.current_process()._inheriting
    return self._bootstrap()

```

`logging` 模块为了实现线程安全内部使用了 `threading.RLock`，而 `RLock` 则是不能通过 `pickle` 序列化的

```Python
import pickle
from threading import RLock

lock = RLock()


pickle.loads(pickle.dumps(lock))  # TypeError: can't pickle _thread.RLock objects
```

这点 Python 3.6 和 Python 3.7 均是如此。在 Python 的文档中也提到过关于[多进程 pickle 的问题](https://docs.python.org/3/library/multiprocessing.html#programming-guidelines)

> When using the *spawn* or *forkserver* start methods many types from [`multiprocessing`](https://docs.python.org/3/library/multiprocessing.html#module-multiprocessing) need to be picklable so that child processes can use them.  However, one should generally avoid sending shared objects to other processes using pipes or queues. Instead you should arrange the program so that a process which needs access to a shared resource created elsewhere can inherit it from an ancestor process.



那么为什么在 Python 3.7 上面是可以运行的呢，因为 Python 3.7 的 `logging.logger` 自定义了 `__reduce__` (https://github.com/python/cpython/pull/1956)  不去直接序列化 logger，而是通过 `getLogger` 绕一层
    