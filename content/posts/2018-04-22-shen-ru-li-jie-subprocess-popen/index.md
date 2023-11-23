
+++
title = "深入理解 subprocess.Popen"
summary = ''
description = ""
categories = []
tags = []
date = 2018-04-22T13:29:00+08:00
draft = false
+++

提起 `subprocess` 执行 shell 命令，最大的坑就是不去 wait；或者 buf 满的时候父进程没有及时读取数据而去 wait，子进程又想继续写入，从而造成死锁，此问题 [文档](https://docs.python.org/3/library/subprocess.html#subprocess.Popen.wait) 中已经写明：建议使用 `communicate`。但是它是一下子读取完的，数据量大的时候效率不高。所以可以用个 `select` 自己撸一遍，做一个流式处理

`communicate` 的内部实现其实也是使用了 IO Multiplexing 去轮询多个管道。嗯，我指的是 \*nix 下；Windows 下是开了线程的

本文仅以 Linux 部分举例，重点放在父子进程间的数据交互，代码基于 CPython 3.6.3， commit sha 为 `2c5fed86e0cbba5a4e34792b0083128ce659909d`

### 子进程创建

`subprocess.Popen` 是一个类，当我们实例化它时会创建一个对应的进程。而这个实例是此进程的对象表示

```Python
class Popen(object):
    _child_created = False  # Set here since __del__ checks it

    def __init__(self, args, bufsize=-1, executable=None,
                 stdin=None, stdout=None, stderr=None,
                 preexec_fn=None, close_fds=_PLATFORM_DEFAULT_CLOSE_FDS,
                 shell=False, cwd=None, env=None, universal_newlines=False,
                 startupinfo=None, creationflags=0,
                 restore_signals=True, start_new_session=False,
                 pass_fds=(), *, encoding=None, errors=None):
        _cleanup()  # 清理遗留问题
        # Held while anything is calling waitpid before returncode has been
        # updated to prevent clobbering returncode if wait() or poll() are
        # called from multiple threads at once.  After acquiring the lock,
        # code must re-check self.returncode to see if another thread just
        # finished a waitpid() call.
        self._waitpid_lock = threading.Lock()

        self._input = None
        self._communication_started = False
        if bufsize is None:
            bufsize = -1  # Restore default
        if not isinstance(bufsize, int):
            raise TypeError("bufsize must be an integer")

        if close_fds is _PLATFORM_DEFAULT_CLOSE_FDS:
            close_fds = True
        if pass_fds and not close_fds:
            warnings.warn("pass_fds overriding close_fds.", RuntimeWarning)
            close_fds = True
        if startupinfo is not None:
            raise ValueError("startupinfo is only supported on Windows "
                             "platforms")
        if creationflags != 0:
            raise ValueError("creationflags is only supported on Windows "
                             "platforms")

        self.args = args
        self.stdin = None
        self.stdout = None
        self.stderr = None
        self.pid = None
        self.returncode = None
        self.universal_newlines = universal_newlines
        self.encoding = encoding
        self.errors = errors

        (p2cread, p2cwrite,
         c2pread, c2pwrite,
         errread, errwrite) = self._get_handles(stdin, stdout, stderr)

        text_mode = encoding or errors or universal_newlines

        self._closed_child_pipe_fds = False

        # 从 fd 转换为文件对象
        try:
            if p2cwrite != -1:
                self.stdin = io.open(p2cwrite, 'wb', bufsize)
                if text_mode:
                    self.stdin = io.TextIOWrapper(self.stdin, write_through=True,
                            line_buffering=(bufsize == 1),
                            encoding=encoding, errors=errors)
            if c2pread != -1:
                self.stdout = io.open(c2pread, 'rb', bufsize)
                if text_mode:
                    self.stdout = io.TextIOWrapper(self.stdout,
                            encoding=encoding, errors=errors)
            if errread != -1:
                self.stderr = io.open(errread, 'rb', bufsize)
                if text_mode:
                    self.stderr = io.TextIOWrapper(self.stderr,
                            encoding=encoding, errors=errors)

            self._execute_child(args, executable, preexec_fn, close_fds,
                                pass_fds, cwd, env,
                                startupinfo, creationflags, shell,
                                p2cread, p2cwrite,
                                c2pread, c2pwrite,
                                errread, errwrite,
                                restore_signals, start_new_session)
        except:
            # Cleanup if the child failed starting.
            for f in filter(None, (self.stdin, self.stdout, self.stderr)):
                try:
                    f.close()
                except OSError:
                    pass  # Ignore EBADF or other errors.

            if not self._closed_child_pipe_fds:
                to_close = []
                # 这里仅关闭我们内部创建的那些管道，不能将用户传入的管道关闭掉
                if stdin == PIPE:
                    to_close.append(p2cread)
                if stdout == PIPE:
                    to_close.append(c2pwrite)
                if stderr == PIPE:
                    to_close.append(errwrite)
                if hasattr(self, '_devnull'):
                    to_close.append(self._devnull)
                for fd in to_close:
                    try:
                        os.close(fd)
                    except OSError:
                        pass

            raise
```

`_cleanup` 感觉像是在结束时调用的，这里却是在最开始的时候调用。那么来看一下它做了哪些清理的工作

```Python
# This lists holds Popen instances for which the underlying process had not
# exited at the time its __del__ method got called: those processes are wait()ed
# for synchronously from _cleanup() when a new Popen object is created, to avoid
# zombie processes.
_active = []

def _cleanup():
    for inst in _active[:]:
        res = inst._internal_poll(_deadstate=sys.maxsize)
        if res is not None:
            try:
                _active.remove(inst)
            except ValueError:
                # This can happen if two threads create a new Popen instance.
                # It's harmless that it was already removed, so ignore.
                pass
```

请注意代码中写的是 `for inst in _active[:]` 而不是 `for inst in _active`。这是因为考虑到了在迭代过程中，其他线程有可能去修改 `_active`，所以这里是获取整个 `slice`

如果你不明白可以体会一下下面的示例

```Python
l = list(range(10))
for i in l:
    print(i)
    l.append(i)
# 对比
l = list(range(10))
for i in l[:]:
    print(i)
    l.append(i)
```

`_active` 中存放的是被销毁时底层对应进程还未结束的 `Popen` 实例。当我们再一次创建 `Popen` 实例时，会尝试去 `wait` 它们以免产生 zombie process

关于 `_internal_poll` 我们稍后再说

`_get_handles` 方法按需初始化了一堆管道，用于和子进程进行通信

```Python
class Popen(object):
    def _get_handles(self, stdin, stdout, stderr):
        p2cread, p2cwrite = -1, -1
        c2pread, c2pwrite = -1, -1
        errread, errwrite = -1, -1

        # 下面的代码分别初始化了 stdin, stdout, stderr 三个管道
        if stdin is None:  # 默认情况，即 subprocess.Popen 时未指定 stdin
            pass
        elif stdin == PIPE:  # 即 subprocess.PIPE，自动创建管道
            p2cread, p2cwrite = os.pipe()
        elif stdin == DEVNULL:
            p2cread = self._get_devnull()  # 子进程从 /dev/null 读取
        elif isinstance(stdin, int):  # 用户传入自定义管道，如 os.pipe()
            p2cread = stdin
        else:
            # Assuming file-like object
            p2cread = stdin.fileno()  # 当自定义管道非 fd 时，如 socket.socketpair()

        if stdout is None:
            pass
        elif stdout == PIPE:
            c2pread, c2pwrite = os.pipe()
        elif stdout == DEVNULL:
            c2pwrite = self._get_devnull()  # 子进程向 /dev/null 写入
        elif isinstance(stdout, int):
            c2pwrite = stdout
        else:
            # Assuming file-like object
            c2pwrite = stdout.fileno()

        if stderr is None:
            pass
        elif stderr == PIPE:
            errread, errwrite = os.pipe()
        elif stderr == STDOUT:  # 将子进程的 stderr 定向到 stdout 中
            if c2pwrite != -1:
                errwrite = c2pwrite
            else: # child's stdout is not set, use parent's stdout
                errwrite = sys.__stdout__.fileno()
        elif stderr == DEVNULL:
            errwrite = self._get_devnull()
        elif isinstance(stderr, int):
            errwrite = stderr
        else:
            # Assuming file-like object
            errwrite = stderr.fileno()

        return (p2cread, p2cwrite,
                c2pread, c2pwrite,
                errread, errwrite)
```

即建立了如下的管道

```
# Parent                   Child
# ------                   -----
# p2cwrite   ---stdin--->  p2cread
# c2pread    <--stdout---  c2pwrite
# errread    <--stderr---  errwrite
```

随后打开这些 fd，封装一层 `io.TextIOWrapper`。接下来便是 fork-exec

```Python
class Popen(object):
    def _execute_child(self, args, executable, preexec_fn, close_fds,
                       pass_fds, cwd, env,
                       startupinfo, creationflags, shell,
                       p2cread, p2cwrite,
                       c2pread, c2pwrite,
                       errread, errwrite,
                       restore_signals, start_new_session):
        if isinstance(args, (str, bytes)):
            args = [args]
        else:
            args = list(args)

        if shell:
            args = ["/bin/sh", "-c"] + args  # execute through the shell
            if executable:
                args[0] = executable

        if executable is None:
            executable = args[0]
        orig_executable = executable

        # For transferring possible exec failure from child to parent.
        # Data format: "exception name:hex errno:description"
        # Pickle is not used; it is complex and involves memory allocation.
        errpipe_read, errpipe_write = os.pipe()
        # errpipe_write must not be in the standard io 0, 1, or 2 fd range.
        low_fds_to_close = []
        while errpipe_write < 3:  # 此处参考 https://bugs.python.org/issue15798
            low_fds_to_close.append(errpipe_write)
            errpipe_write = os.dup(errpipe_write)
        for low_fd in low_fds_to_close:
            os.close(low_fd)
        try:
            try:
                # We must avoid complex work that could involve
                # malloc or free in the child process to avoid
                # potential deadlocks, thus we do all this here.
                # and pass it to fork_exec()

                # 处理环境变量
                if env is not None:
                    env_list = []
                    for k, v in env.items():
                        k = os.fsencode(k)
                        if b'=' in k:
                            raise ValueError("illegal environment variable name")
                        env_list.append(k + b'=' + os.fsencode(v))
                else:
                    env_list = None  # Use execv instead of execve.
                executable = os.fsencode(executable)
                if os.path.dirname(executable):
                    executable_list = (executable,)
                else:
                    # This matches the behavior of os._execvpe().
                    executable_list = tuple(
                        os.path.join(os.fsencode(dir), executable)
                        for dir in os.get_exec_path(env))
                fds_to_keep = set(pass_fds)
                fds_to_keep.add(errpipe_write)

                # 调用 C 实现
                self.pid = _posixsubprocess.fork_exec(
                        args, executable_list,
                        close_fds, tuple(sorted(map(int, fds_to_keep))),
                        cwd, env_list,
                        p2cread, p2cwrite, c2pread, c2pwrite,
                        errread, errwrite,
                        errpipe_read, errpipe_write,
                        restore_signals, start_new_session, preexec_fn)
                self._child_created = True
            finally:
                # be sure the FD is closed no matter what
                os.close(errpipe_write)

            # self._devnull is not always defined.
            devnull_fd = getattr(self, '_devnull', None)
            # 这里关闭了管道中不需要使用的那一端
            if p2cread != -1 and p2cwrite != -1 and p2cread != devnull_fd:
                os.close(p2cread)
            if c2pwrite != -1 and c2pread != -1 and c2pwrite != devnull_fd:
                os.close(c2pwrite)
            if errwrite != -1 and errread != -1 and errwrite != devnull_fd:
                os.close(errwrite)
            if devnull_fd is not None:
                os.close(devnull_fd)
            # Prevent a double close of these fds from __init__ on error.
            self._closed_child_pipe_fds = True

            # Wait for exec to fail or succeed; possibly raising an
            # exception (limited in size)
            errpipe_data = bytearray()
            while True:
                part = os.read(errpipe_read, 50000)
                errpipe_data += part
                if not part or len(errpipe_data) > 50000:
                    break
        finally:
            # be sure the FD is closed no matter what
            os.close(errpipe_read)

        # exec 失败
        if errpipe_data:
            try:
                pid, sts = os.waitpid(self.pid, 0)
                if pid == self.pid:
                    self._handle_exitstatus(sts)
                else:
                    self.returncode = sys.maxsize
            except ChildProcessError:
                pass

            try:
                exception_name, hex_errno, err_msg = (
                        errpipe_data.split(b':', 2))
                # The encoding here should match the encoding
                # written in by the subprocess implementations
                # like _posixsubprocess
                err_msg = err_msg.decode()
            except ValueError:
                exception_name = b'SubprocessError'
                hex_errno = b'0'
                err_msg = 'Bad exception data from child: {!r}'.format(
                              bytes(errpipe_data))
            child_exception_type = getattr(
                    builtins, exception_name.decode('ascii'),
                    SubprocessError)
            if issubclass(child_exception_type, OSError) and hex_errno:
                errno_num = int(hex_errno, 16)
                child_exec_never_called = (err_msg == "noexec")
                if child_exec_never_called:
                    err_msg = ""
                    # The error must be from chdir(cwd).
                    err_filename = cwd
                else:
                    err_filename = orig_executable
                if errno_num != 0:
                    err_msg = os.strerror(errno_num)
                    if errno_num == errno.ENOENT:
                        err_msg += ': ' + repr(err_filename)
                raise child_exception_type(errno_num, err_msg, err_filename)
           raise child_exception_type(err_msg)
```

`errpipe_read` 和 `errpipe_write` 这条管道区别于 `errread` 和 `errwrite`，用于传递 exec 时的错误

`_handle_exitstatus` 针对不同的退出情况做了不同的处理

```Python
class Popen(object):
    def _handle_exitstatus(self, sts, _WIFSIGNALED=os.WIFSIGNALED,
            _WTERMSIG=os.WTERMSIG, _WIFEXITED=os.WIFEXITED,
            _WEXITSTATUS=os.WEXITSTATUS, _WIFSTOPPED=os.WIFSTOPPED,
            _WSTOPSIG=os.WSTOPSIG):
        """All callers to this function MUST hold self._waitpid_lock."""
        # This method is called (indirectly) by __del__, so it cannot
        # refer to anything outside of its local scope.
        if _WIFSIGNALED(sts):  # if the process exited due to a signal
            # return the signal which caused the process to exit
            self.returncode = -_WTERMSIG(sts)
        elif _WIFEXITED(sts):  # if the process exited using the exit(2) system call
            # return the integer parameter to the exit(2) system call
            self.returncode = _WEXITSTATUS(sts)
        elif _WIFSTOPPED(sts):  # if the process has been stopped
            # return the signal which caused the process to stop.
            self.returncode = -_WSTOPSIG(sts)
        else:
            # Should never happen
            raise SubprocessError("Unknown child exit status!")
```

之前从来没想过 `returncode` 的获取是如此繁琐，于是翻了一遍 `wait(2)`，发现自己的理解还是不到位的

首先 `wait`, `waitpid`, `waitid` 这一组系统调用是用于等待进程状态改变(wait for process to change state)，而不是先前理解的结束

>All of these system calls are used to wait for state changes in a child of the calling process, and obtain information about the child whose state has changed.  A state change is considered to be: the child terminated; the child was stopped by a signal; or the child was resumed by a signal.  In the case of a terminated child, performing a wait allows the system to release
the resources associated with the child; if a wait is not performed, then the terminated child remains in  a  "zombie"  state.
>The  `waitpid()`  system  call  suspends execution of the calling process until a child specified by pid argument has changed state.  By default, `waitpid()` waits only for terminated children, but this behavior is modifiable via the options argument, as described below.

比如可以检测子进程的 stop 和 resume

```Python
while True:
    _, sts = os.waitpid(pid, os.WUNTRACED | os.WCONTINUED)
    if os.WIFSTOPPED(sts):
        print('stop')
    elif os.WIFCONTINUED(sts):
        print('continue')
    else:
        break
```


### `communicate`

`communicate` 的实现如下

```Python
class Popen(object):
    def communicate(self, input=None, timeout=None):
        if self._communication_started and input:
            raise ValueError("Cannot send input after starting communication")

        # Optimization: If we are not worried about timeouts, we haven't
        # started communicating, and we have one or zero pipes, using select()
        # or threads is unnecessary.
        if (timeout is None and not self._communication_started and
            [self.stdin, self.stdout, self.stderr].count(None) >= 2):
            stdout = None
            stderr = None
            if self.stdin:
                self._stdin_write(input)
            elif self.stdout:
                stdout = self.stdout.read()
                self.stdout.close()
            elif self.stderr:
                stderr = self.stderr.read()
                self.stderr.close()
            self.wait()
        else:  # 需要同时处理多个管道
            if timeout is not None:
                endtime = _time() + timeout
            else:
                endtime = None

            try:
                stdout, stderr = self._communicate(input, endtime, timeout)
            finally:
                self._communication_started = True

            sts = self.wait(timeout=self._remaining_time(endtime))

        return (stdout, stderr)
```

这里有一个优化：当我们仅需要处理一个管道，或者根本对这些不关心时，直接读取数据然后 wait 即可。但是当我们需要处理多个管道且需要做限制时间时便需要通过 `_communicate` 来完成

```Python
# poll/select have the advantage of not requiring any extra file
# descriptor, contrarily to epoll/kqueue (also, they require a single
# syscall).
if hasattr(selectors, 'PollSelector'):
    _PopenSelector = selectors.PollSelector
else:
    _PopenSelector = selectors.SelectSelector


class Popen(object):
    def _communicate(self, input, endtime, orig_timeout):
        if self.stdin and not self._communication_started:
            # Flush stdio buffer.  This might block, if the user has
            # been writing to .stdin in an uncontrolled fashion.
            try:
                self.stdin.flush()
            except BrokenPipeError:
                pass  # communicate() must ignore BrokenPipeError.
            if not input:
                try:
                    self.stdin.close()
                except BrokenPipeError:
                    pass  # communicate() must ignore BrokenPipeError.

        stdout = None
        stderr = None

        # Only create this mapping if we haven't already.
        if not self._communication_started:
            self._fileobj2output = {}
            if self.stdout:
                self._fileobj2output[self.stdout] = []
            if self.stderr:
                self._fileobj2output[self.stderr] = []

        if self.stdout:
            stdout = self._fileobj2output[self.stdout]
        if self.stderr:
            stderr = self._fileobj2output[self.stderr]

        self._save_input(input)

        if self._input:
            input_view = memoryview(self._input)

        with _PopenSelector() as selector:
            # 注册
            if self.stdin and input:
                selector.register(self.stdin, selectors.EVENT_WRITE)
            if self.stdout:
                selector.register(self.stdout, selectors.EVENT_READ)
            if self.stderr:
                selector.register(self.stderr, selectors.EVENT_READ)

            while selector.get_map():  # 当前还存在需要监听的时间
                timeout = self._remaining_time(endtime)  # endtime - time.monotonic()
                if timeout is not None and timeout < 0:
                    raise TimeoutExpired(self.args, orig_timeout)

                ready = selector.select(timeout)
                self._check_timeout(endtime, orig_timeout)  # 二次检测，是否超时

                for key, events in ready:
                    if key.fileobj is self.stdin:
                        chunk = input_view[self._input_offset :
                                           self._input_offset + _PIPE_BUF]
                        try:
                            self._input_offset += os.write(key.fd, chunk)
                        except BrokenPipeError:
                            selector.unregister(key.fileobj)
                            key.fileobj.close()
                        else:
                            if self._input_offset >= len(self._input):
                                selector.unregister(key.fileobj)
                                key.fileobj.close()
                    elif key.fileobj in (self.stdout, self.stderr):
                        # 32KB 优化，参考 https://bugs.python.org/issue19929
                        data = os.read(key.fd, 32768)
                        if not data:
                            selector.unregister(key.fileobj)
                            key.fileobj.close()
                        self._fileobj2output[key.fileobj].append(data)

        self.wait(timeout=self._remaining_time(endtime))

        # All data exchanged.  Translate lists into strings.
        if stdout is not None:
            stdout = b''.join(stdout)
        if stderr is not None:
            stderr = b''.join(stderr)

        # Translate newlines, if requested.
        # This also turns bytes into strings.
        if self.encoding or self.errors or self.universal_newlines:
            if stdout is not None:
                stdout = self._translate_newlines(stdout,
                                                  self.stdout.encoding,
                                                  self.stdout.errors)
            if stderr is not None:
                stderr = self._translate_newlines(stderr,
                                                  self.stderr.encoding,
                                                  self.stderr.errors)
        return (stdout, stderr)
```

`_communicate` 使用了 IO Multiplexing 来处理 stdin, stdout, stderr。因为至多会处理两个，所以用不着 `epoll`(需要三个系统调用)

### `wait`

`wait` 的实现

```Python
class Popen(object):
    def wait(self, timeout=None, endtime=None):
        if self.returncode is not None:
            return self.returncode

        if endtime is not None:
            warnings.warn(
                "'endtime' argument is deprecated; use 'timeout'.",
                DeprecationWarning,
                stacklevel=2)
        if endtime is not None or timeout is not None:
            if endtime is None:
                endtime = _time() + timeout
            elif timeout is None:
                timeout = self._remaining_time(endtime)

        if endtime is not None:
            # Enter a busy loop if we have a timeout.  This busy loop was
            # cribbed from Lib/threading.py in Thread.wait() at r71065.
            delay = 0.0005 # 500 us -> initial delay of 1 ms
            while True:
                if self._waitpid_lock.acquire(False):  # 非阻塞
                    try:
                        if self.returncode is not None:
                            break  # Another thread waited.
                        (pid, sts) = self._try_wait(os.WNOHANG)
                        assert pid == self.pid or pid == 0
                        if pid == self.pid:
                            self._handle_exitstatus(sts)
                            break
                    finally:
                        self._waitpid_lock.release()
                remaining = self._remaining_time(endtime)
                if remaining <= 0:
                    raise TimeoutExpired(self.args, timeout)
                delay = min(delay * 2, remaining, .05)
                time.sleep(delay)
        else:
            while self.returncode is None:
                with self._waitpid_lock:
                    if self.returncode is not None:
                        break  # Another thread waited.
                    (pid, sts) = self._try_wait(0)
                    # Check the pid and loop as waitpid has been known to
                    # return 0 even without WNOHANG in odd situations.
                    # http://bugs.python.org/issue14396.
                    if pid == self.pid:
                        self._handle_exitstatus(sts)
        return self.returncode
```

`wait` 通过 `_waitpid_lock` 保证了同一时刻只有一个线程在去 `wait` 子进程。`wait` 的核心部分在 `_try_wait` 中，实际上就是对 `os.waitpid` 的封装。对于设置超时的情况，我们采取非阻塞的方式去获取锁，然后不断校对是否超时。`os.WNOHANG` 即 wait no hange，如果子进程没有结束则返回 `(0, 0)`

`_try_wait` 的实现如下

```Python
class Popen(object):
    def _try_wait(self, wait_flags):
        try:
            (pid, sts) = os.waitpid(self.pid, wait_flags)
        except ChildProcessError:
            # This happens if SIGCLD is set to be ignored or waiting
            # for child processes has otherwise been disabled for our
            # process.  This child is dead, we can't get the status.
            pid = self.pid
            sts = 0
        return (pid, sts)
```

`poll` 可以去检查子进程是否结束，它就是调用的 `_internal_poll`。`_internal_poll` 的实现如下

```Python
class Popen(object):
    def _internal_poll(self, _deadstate=None, _waitpid=os.waitpid,
            _WNOHANG=os.WNOHANG, _ECHILD=errno.ECHILD):
        """Check if child process has terminated.  Returns returncode
        attribute.
        This method is called by __del__, so it cannot reference anything
        outside of the local scope (nor can any methods it calls).
        """
        if self.returncode is None:
            if not self._waitpid_lock.acquire(False):
                # Something else is busy calling waitpid.  Don't allow two
                # at once.  We know nothing yet.
                return None
            try:
                if self.returncode is not None:
                    return self.returncode  # Another thread waited.
                pid, sts = _waitpid(self.pid, _WNOHANG)
                if pid == self.pid:
                    self._handle_exitstatus(sts)
            except OSError as e:
                if _deadstate is not None:
                    self.returncode = _deadstate
                elif e.errno == _ECHILD:
                    # This happens if SIGCLD is set to be ignored or
                    # waiting for child processes has otherwise been
                    # disabled for our process.  This child is dead, we
                    # can't get the status.
                    # http://bugs.python.org/issue15756
                    self.returncode = 0
            finally:
                self._waitpid_lock.release()
        return self.returncode
```

当然如果一个 `Popen` 实例的引用计数归零，也会尝试去 `wait`。若失败则添加到 `_active` 中

```Python
class Popen(object):
    def __del__(self, _maxsize=sys.maxsize, _warn=warnings.warn):
        if not self._child_created:
            # We didn't get to successfully create a child process.
            return
        if self.returncode is None:
            # Not reading subprocess exit status creates a zombie process which
            # is only destroyed at the parent python process exit
            _warn("subprocess %s is still running" % self.pid,
                  ResourceWarning, source=self)
        # In case the child hasn't been waited on, check if it's done.
        self._internal_poll(_deadstate=_maxsize)
        if self.returncode is None and _active is not None:
            # Child is still running, keep us alive until we can wait on it.
            _active.append(self)
```

执行 `_active.append(self)` 后，我们对此实例进行了新的引用，所以并不会被销毁，这种手段被称为 object resurrection。CPython 中当此实例被再次销毁时，并不会再次调用 `__del__`，参考 [Python DataModel](https://docs.python.org/3/reference/datamodel.html#object.__del__)

>It is implementation-dependent whether `__del__()` is called a second time when a resurrected object is about to be destroyed; the current CPython implementation only calls it once.

另外需要注意 `__del__` 并不是一定会被调用的，参考 [Python DataModel](https://docs.python.org/3/reference/datamodel.html#object.__del__)

>It is not guaranteed that `__del__()` methods are called for objects that still exist when the interpreter exits.

此时未结束的子进程会被 `init` 接管

    