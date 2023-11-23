
+++
title = "Watcher — asyncio 源码剖析(8)"
summary = ''
description = ""
categories = []
tags = []
date = 2017-12-31T12:31:35+08:00
draft = false
+++

*源码基于 Python 3.6.3 的 asyncio commit sha 为 `2c5fed86e0cbba5a4e34792b0083128ce659909d`*

在 Linux 平台上 `asyncio` 提供了一组 `watcher` 来监控子进程。具体作用是接收子进程的信号，然后调度对应的 callback

默认情况下，当我们调用 `asyncio.create_subprocess_exec()`、`asyncio.create_subprocess_shell()` 会自动创建 watcher

```Python
# subprocess.py
@coroutine
def create_subprocess_shell(cmd, stdin=None, stdout=None, stderr=None,
                            loop=None, limit=streams._DEFAULT_LIMIT, **kwds):
    if loop is None:
        loop = events.get_event_loop()
    protocol_factory = lambda: SubprocessStreamProtocol(limit=limit,
                                                        loop=loop)
    transport, protocol = yield from loop.subprocess_shell(
                                            protocol_factory,
                                            cmd, stdin=stdin, stdout=stdout,
                                            stderr=stderr, **kwds)
    return Process(transport, protocol, loop)
```

`loop.subprocess_shell` 位于基类 `BaseEventLoop`

```Python
# base_events.py
class BaseEventLoop(events.AbstractEventLoop):
    @coroutine
    def subprocess_shell(self, protocol_factory, cmd, *, stdin=subprocess.PIPE,
                         stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                         universal_newlines=False, shell=True, bufsize=0,
                         **kwargs):
        # 有省略
        protocol = protocol_factory()
        transport = yield from self._make_subprocess_transport(
            protocol, cmd, True, stdin, stdout, stderr, bufsize, **kwargs)
        return transport, protocol
```

```Python
# unix_events.py
class _UnixSelectorEventLoop(selector_events.BaseSelectorEventLoop):
    @coroutine
    def _make_subprocess_transport(self, protocol, args, shell,
                                   stdin, stdout, stderr, bufsize,
                                   extra=None, **kwargs):
        with events.get_child_watcher() as watcher:
            waiter = self.create_future()
            transp = _UnixSubprocessTransport(self, protocol, args, shell,
                                              stdin, stdout, stderr, bufsize,
                                              waiter=waiter, extra=extra,
                                              **kwargs)

            watcher.add_child_handler(transp.get_pid(),
                                      self._child_watcher_callback, transp)
            try:
                # _UnixSubprocessTransport 中创建了一个 _connect_pipes 的 task
                # 当此 task 执行完后 waiter 会被 set_result()
                yield from waiter
            except Exception as exc:
                # Workaround CPython bug #23353: using yield/yield-from in an
                # except block of a generator doesn't clear properly
                # sys.exc_info()
                err = exc
            else:
                err = None

            if err is not None:
                transp.close()
                yield from transp._wait()
                raise err
        return transp
```

```Python
# events.py
def get_child_watcher():
    return get_event_loop_policy().get_child_watcher()
```

使用了策略模式，调用相应平台的 `get_child_watcher()` 实现。Linux 下为 `_UnixDefaultEventLoopPolicy`

```Python
# unix_events.py
class _UnixDefaultEventLoopPolicy(events.BaseDefaultEventLoopPolicy):
    def _init_watcher(self):
        with events._lock:
            if self._watcher is None:
                self._watcher = SafeChildWatcher()
                if isinstance(threading.current_thread(),
                              threading._MainThread):
                    self._watcher.attach_loop(self._local._loop)

    def get_child_watcher(self):
        # 默认情况下为 None，_init_watcher() 会初始化一个 SafeChildWatcher 实例
        # 可以通过 set_child_watcher 进行 watcher 的设置
        # 一个 Policy 实例只会有一个 watcher 实例
        if self._watcher is None:
            self._init_watcher()
        return self._watcher
```

下面看一下 `watcher` 的具体实现，位于 `unix_events.py`

```
# watcher hierarchy
AbstractChildWatcher
 +-- BaseChildWatcher
     +-- SafeChildWatcher
     +-- FastChildWatcher
```

`AbstractChildWatcher` 中都是抽象方法，所以先来看一下 `BaseChildWatcher` 的实现

```Python
# unix_events.py
class BaseChildWatcher(AbstractChildWatcher):
    def __init__(self):
        self._loop = None
        self._callbacks = {}

    def attach_loop(self, loop):
        if self._loop is not None and loop is None and self._callbacks:
            warnings.warn(
                'A loop is being detached '
                'from a child watcher with pending handlers',
                RuntimeWarning)
        # 如果之前绑定过 loop 则取消已经注册的 handler，然后绑定新的 loop
        if self._loop is not None:
            self._loop.remove_signal_handler(signal.SIGCHLD)
        self._loop = loop
        if loop is not None:
            loop.add_signal_handler(signal.SIGCHLD, self._sig_chld)
            # Prevent a race condition in case a child terminated
            # during the switch.
            self._do_waitpid_all()

    def _sig_chld(self):
        try:
            self._do_waitpid_all()
        except Exception as exc:
            # self._loop should always be available here
            # as '_sig_chld' is added as a signal handler
            # in 'attach_loop'
            self._loop.call_exception_handler({
                'message': 'Unknown exception in SIGCHLD handler',
                'exception': exc,
            })
    # 省略了几个方法
```

这里调用了 `loop.add_signal_handler` 为 `signal.SIGCHLD` 信号注册了对应的 handler `_sig_chld`。其工作原理在 [Thread — asyncio 源码剖析(3)](https://blog.dreamfever.me/the-magic-of-asyncio-3/) 中已经提及。就是通过一对 `socketpair`，一端在 `select` 中进行注册，监听事件；另一端由 `signal.set_wakeup_fd` 绑定。当有信号来临时，会写入对应的 `signum`，然后经由 `select` 调度对应的 `handler`(具体步骤为 `loop._read_from_self()` -> `loop._process_self_data()` -> `loop._handle_signal()`)。`signum` 和 `handler` 以字典的形式存储在 `loop._signal_handlers` 属性中

 `_do_waitpid_all()` 由子类进行实现

```Python
# unix_events.py
class SafeChildWatcher(BaseChildWatcher):
    def close(self):
        self._callbacks.clear()
        super().close()

    def __enter__(self):
        return self

    def __exit__(self, a, b, c):
        pass

    def add_child_handler(self, pid, callback, *args):
        if self._loop is None:
            raise RuntimeError("Cannot add child handler, "
                "the child watcher does not have a loop attached")

        self._callbacks[pid] = (callback, args)

        # Prevent a race condition in case the child is already terminated.
        self._do_waitpid(pid)

    def remove_child_handler(self, pid):
        try:
            del self._callbacks[pid]
            return True
        except KeyError:
            return False

    def _do_waitpid_all(self):
        for pid in list(self._callbacks):
            self._do_waitpid(pid)

    def _do_waitpid(self, expected_pid):
        try:
            # 等待 pid 的进程结束，指定 os.WNOHANG 则不会产生阻塞
            pid, status = os.waitpid(expected_pid, os.WNOHANG)
        except ChildProcessError:
            # The child process is already reaped
            # (may happen if waitpid() is called elsewhere).
            pid = expected_pid
            returncode = 255
            logger.warning(
                "Unknown child process pid %d, will report returncode 255", pid)
        else:
            if pid == 0:
                # The child process is still alive.
                return

            returncode = self._compute_returncode(status)
            if self._loop.get_debug():
                logger.debug('process %s exited with returncode %s',
                             expected_pid, returncode)

        try:
            callback, args = self._callbacks.pop(pid)
        except KeyError:
            # May happen if .remove_child_handler() is called
            # after os.waitpid() returns.
            if self._loop.get_debug():
                logger.warning("Child watcher got an unexpected pid: %r",
                               pid, exc_info=True)
        else:
            callback(pid, returncode, *args)
```

再来看一下先前注册的 callback

```Python
# unix_events.py
class _UnixSelectorEventLoop(selector_events.BaseSelectorEventLoop):
    def _child_watcher_callback(self, pid, returncode, transp):
        self.call_soon_threadsafe(transp._process_exited, returncode)

# base_subprocess.py
class BaseSubprocessTransport(transports.SubprocessTransport):
    def _process_exited(self, returncode):
        # 此处有省略
        self._returncode = returncode
        if self._proc.returncode is None:
            # asyncio uses a child watcher: copy the status into the Popen
            # object. On Python 3.6, it is required to avoid a ResourceWarning.
            self._proc.returncode = returncode
        self._call(self._protocol.process_exited)
        self._try_finish()

        # wake up futures waiting for wait()
        for waiter in self._exit_waiters:
            if not waiter.cancelled():
                waiter.set_result(returncode)
        self._exit_waiters = None
```

之所以这样是因为 `asyncio.Process.wait` 并不是一个真正的阻塞操作。当我们调用 `wait()` 仅是去创建了一个 `future`，如果此 `future` 完成则说明子进程已经结束，才会再度调度这个 `coroutine`。也就说不被调度即视为阻塞。

```Python
# subprocess.py
class Process:
    @coroutine
    def wait(self):
        """Wait until the process exit and return the process return code.
        This method is a coroutine."""
        return (yield from self._transport._wait())
```

```Python
# base_subprocess.py
class BaseSubprocessTransport(transports.SubprocessTransport):
    @coroutine
    def _wait(self):
        """Wait until the process exit and return the process return code.
        This method is a coroutine."""
        if self._returncode is not None:
            return self._returncode

        waiter = self._loop.create_future()
        self._exit_waiters.append(waiter)
        return (yield from waiter)
```

另外还有一个 `FastChildWatcher`

```Python
class FastChildWatcher(BaseChildWatcher):
    def __init__(self):
        super().__init__()
        self._lock = threading.Lock()
        self._zombies = {}
        self._forks = 0

    def close(self):
        self._callbacks.clear()
        self._zombies.clear()
        super().close()

    def __enter__(self):
        with self._lock:
            self._forks += 1

            return self

    def __exit__(self, a, b, c):
        with self._lock:
            self._forks -= 1

            if self._forks or not self._zombies:
                return

            collateral_victims = str(self._zombies)
            self._zombies.clear()

        logger.warning(
            "Caught subprocesses termination from unknown pids: %s",
            collateral_victims)

    def add_child_handler(self, pid, callback, *args):
        assert self._forks, "Must use the context manager"

        if self._loop is None:
            raise RuntimeError(
                "Cannot add child handler, "
                "the child watcher does not have a loop attached")

        with self._lock:
            try:
                returncode = self._zombies.pop(pid)
            except KeyError:
                # The child is running.
                self._callbacks[pid] = callback, args
                return

        # The child is dead already. We can fire the callback.
        callback(pid, returncode, *args)

    def remove_child_handler(self, pid):
        try:
            del self._callbacks[pid]
            return True
        except KeyError:
            return False

    def _do_waitpid_all(self):
        # Because of signal coalescing, we must keep calling waitpid() as
        # long as we're able to reap a child.
        while True:
            try:
                pid, status = os.waitpid(-1, os.WNOHANG)
            except ChildProcessError:
                # No more child processes exist.
                return
            else:
                if pid == 0:
                    # A child process is still alive.
                    return

                returncode = self._compute_returncode(status)

            with self._lock:
                try:
                    callback, args = self._callbacks.pop(pid)
                except KeyError:
                    # unknown child
                    if self._forks:
                        # It may not be registered yet.
                        self._zombies[pid] = returncode
                        if self._loop.get_debug():
                            logger.debug('unknown process %s exited '
                                         'with returncode %s',
                                         pid, returncode)
                        continue
                    callback = None
                else:
                    if self._loop.get_debug():
                        logger.debug('process %s exited with returncode %s',
                                     pid, returncode)

            if callback is None:
                logger.warning(
                    "Caught subprocess termination from unknown pid: "
                    "%d -> %d", pid, returncode)
            else:
                callback(pid, returncode, *args)
```

`FastChildWatcher` 相较于 `SafeChildWatcher` 使用了一个循环去收割所有已经退出的子进程

    