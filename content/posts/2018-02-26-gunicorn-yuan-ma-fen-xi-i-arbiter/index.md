
+++
title = "Gunicorn 源码分析 I - Arbiter"
summary = ''
description = ""
categories = []
tags = []
date = 2018-02-26T13:53:35+08:00
draft = false
+++

*Gunicorn 19.7.1 版本源码，commit sha 为 328e509260ae70de6c04c5ba885ee17960b3ced5*

首先明确 Gunicorn 是从 Ruby 的 Unicorn 移植过来的，是一个 WSGI HTTP Server。关于什么是 WSGI 可以自行查阅 PEP 3333。相较于其他 WSGI Server，其特点是采用了 pre-worker 模型

### Basic

仅仅需要一个 WSGI callable 即可

```Python
def app(environ, start_response):
    """Simplest possible application object"""
    data = b'Hello, World!\n'
    status = '200 OK'
    response_headers = [
        ('Content-type','text/plain'),
        ('Content-Length', str(len(data)))
    ]
    start_response(status, response_headers)
    return iter([data])
```

运行时指定此路径即可

```
gunicorn --workers=2 t:app
```

其中有几个值得注意的参数

- `max_requests`:worker 在处理指定数量的请求后会自动重启
- `max_requests_jitter`:worker 在处理完 `randint(0, max_requests_jitter)` 数量的请求后会自动重启，避免同时重启引发的抖动
- `timeout`:worker 在限定时间内若无法处理完当前的请求则会被强制重启
- `graceful_timeout`:在接受到 restart signal 后，超过此时间则会被 kill
- `keepalive`: 限定 http keepalive 的时间

基于安全性的几个参数

- `limit_request_line`
- `limit_request_fields`
- `limit_request_field_size`

Gunicorn 还提供了多种 Server Hooks

### Philosophy

根据 `setup.py` 中的 `entrypoint` 可以定位到入口

```Python
# gunicorn/app/wsgiapp.py
def run():
    from gunicorn.app.wsgiapp import WSGIApplication
    WSGIApplication("%(prog)s [OPTIONS] [APP_MODULE]").run()
```

可见实例化了一个 `WSGIApplication`。另外 `WSGIApplication` 的定义是在同一个文件中的，所以我不清楚这个 `import` 到底有何用意

```
BaseApplication
 +-- Application
      +-- PasterBaseApplication
      |    +-- PasterApplication
      |    +-- PasterServerApplication
      +-- WSGIApplication
```

这里省略掉读取配置、参数处理等逻辑。需要注意的是配置的优先级为 `command line > env > file`。我们重点来看 `run` 部分

```Python
# gunicorn/app/base.py
class BaseApplication(object):
    def run(self):
        try:
            Arbiter(self).run()
        except RuntimeError as e:
            print("\nError: %s\n" % e, file=sys.stderr)
            sys.stderr.flush()
        sys.exit(1)
```

实例化 `Arbiter` 并将自身传入，`Arbiter` 正如字面意思是管理 `worker` 的

>Arbiter maintain the workers processes alive. It launches or
kills them if needed. It also manages application reloading
via SIGHUP/USR2.

```Python
class Arbiter(object):
    def __init__(self, app):
        os.environ["SERVER_SOFTWARE"] = SERVER_SOFTWARE

        self._num_workers = None
        self._last_logged_active_worker_count = None
        self.log = None

        self.setup(app)

        self.pidfile = None
        # ... omit some code  
```

`Arbiter` 的 `__init__()` 方法初始化了一些变量，并且调用了 `setup()` 方法，这里是重点

```Python
# gunicorn/arbiter.py
class Arbiter(object):
    def setup(self, app):
        self.app = app
        self.cfg = app.cfg

        if self.log is None:
            self.log = self.cfg.logger_class(app.cfg)

        # reopen files
        if 'GUNICORN_FD' in os.environ:
            self.log.reopen_files()

        self.worker_class = self.cfg.worker_class
        self.address = self.cfg.address
        self.num_workers = self.cfg.workers
        self.timeout = self.cfg.timeout
        self.proc_name = self.cfg.proc_name

        # ... omit debug print

        # set enviroment' variables
        if self.cfg.env:
            for k, v in self.cfg.env.items():
                os.environ[k] = v

        if self.cfg.preload_app
            self.app.wsgi()
```

`Arbiter` 通过 `app` 获得了 `worker_class`、`num_workers` 等参数的值，并且如果是 `preload_app` 则会载入 `wsgiapp`。至于如何载入先暂且不表。

`preload_app` 使得在 `Arbiter` 进程中加载了 `wsgi app`，这样有利于节省内存，因为 Linux 具有 Copy-on-Write 的特性。当然这也有坏处，如果在 `worker` 进程中去加载 `wsgi app` 可以轻松地进行 reload。根据文档描述，当使用 `--preload` 参数启动 Gunicorn 后是无法 reload 的

接下来看最为重要的 `run()` 方法

```Python
# gunicorn/arbiter.py
class Arbiter(object):
    def run(self):
        "Main master loop."
        self.start()
        util._setproctitle("master [%s]" % self.proc_name)

        try:
            self.manage_workers()

            while True:
                self.maybe_promote_master()

                sig = self.SIG_QUEUE.pop(0) if len(self.SIG_QUEUE) else None
                if sig is None:
                    self.sleep()
                    self.murder_workers()
                    self.manage_workers()
                    continue

                if sig not in self.SIG_NAMES:
                    self.log.info("Ignoring unknown signal: %s", sig)
                    continue

                signame = self.SIG_NAMES.get(sig)
                handler = getattr(self, "handle_%s" % signame, None)
                if not handler:
                    self.log.error("Unhandled signal: %s", signame)
                    continue
                self.log.info("Handling signal: %s", signame)
                handler()
                self.wakeup()  # is it important to wakeup immediately
        except StopIteration:
            self.halt()
        except KeyboardInterrupt:
            self.halt()
        except HaltServer as inst:
            self.halt(reason=inst.reason, exit_status=inst.exit_status)
        except SystemExit:
            raise
        except Exception:
            self.log.info("Unhandled exception in main loop",
                          exc_info=True)
            self.stop(False)
            if self.pidfile is not None:
                self.pidfile.unlink()
            sys.exit(-1)
```

分解来看，首先是 `start()` 方法

```Python
# gunicorn/arbiter.py
class Arbiter(object):
    def start(self):
        """\
        Initialize the arbiter. Start listening and set pidfile if needed.
        """
        self.log.info("Starting gunicorn %s", __version__)

        if 'GUNICORN_PID' in os.environ:
            self.master_pid = int(os.environ.get('GUNICORN_PID'))
            self.proc_name = self.proc_name + ".2"
            self.master_name = "Master.2"

        self.pid = os.getpid()
        if self.cfg.pidfile is not None:
            pidname = self.cfg.pidfile
            if self.master_pid != 0:
                pidname += ".2"
            self.pidfile = Pidfile(pidname)
            self.pidfile.create(self.pid)
        self.cfg.on_starting(self)

        self.init_signals()

        if not self.LISTENERS:
            fds = None
            listen_fds = systemd.listen_fds()
            if listen_fds:
                self.systemd = True
                fds = range(systemd.SD_LISTEN_FDS_START,
                            systemd.SD_LISTEN_FDS_START + listen_fds)

            elif self.master_pid:
                fds = []
                for fd in os.environ.pop('GUNICORN_FD').split(','):
                    fds.append(int(fd))

            self.LISTENERS = sock.create_sockets(self.cfg, self.log, fds)

        listeners_str = ",".join([str(l) for l in self.LISTENERS])
        self.log.debug("Arbiter booted")
        self.log.info("Listening at: %s (%s)", listeners_str, self.pid)
        self.log.info("Using worker: %s", self.cfg.worker_class_str)

        # check worker class requirements
        if hasattr(self.worker_class, "check_config"):
            self.worker_class.check_config(self.cfg, self.log)

        self.cfg.when_ready(self)
```

此方法做了三件事情  
1) PID 相关处理
2) 重新注册信号 handler
3) 创建 `LISTENERS`

```Python
# gunicorn/arbiter.py
import signal

class Arbiter(object):
    SIGNALS = [getattr(signal, "SIG%s" % x)
               for x in "HUP QUIT INT TERM TTIN TTOU USR1 USR2 WINCH".split()]
    def init_signals(self):
        """\
        Initialize master signal handling. Most of the signals
        are queued. Child signals only wake up the master.
        """
        # close old PIPE
        if self.PIPE:
            [os.close(p) for p in self.PIPE]

        # initialize the pipe
        self.PIPE = pair = os.pipe()
        for p in pair:
            util.set_non_blocking(p)
            util.close_on_exec(p)

        self.log.close_on_exec()

        # initialize all signals
        [signal.signal(s, self.signal) for s in self.SIGNALS]
        signal.signal(signal.SIGCHLD, self.handle_chld)
```

Linux 子进程是通过 fork-exec 来完成的，通过设置 `FD_CLOEXEC` 标志位可以自动在 exec 时关闭无用的文件描述符。需要注意的是自 Python3.4 后，文件描述符默认是 non-inheritable，所以这里其实是在向下兼容

参考 [Inheritance of File Descriptors](https://docs.python.org/3/library/os.html#fd-inheritance)

```Python
# gunicorn/util.py
import fcntl
def close_on_exec(fd):
    flags = fcntl.fcntl(fd, fcntl.F_GETFD)
    flags |= fcntl.FD_CLOEXEC
    fcntl.fcntl(fd, fcntl.F_SETFD, flags)
```

`SIGHUP`、`SIGQUIT`、`SIGINT`、`SIGTERM`、`SIGTTIN`、`SIGTTOU`、`SIGUSR1`、`SIGUSR2`、`SIGWINCH` 的 handler 均被重新注册为 `signal` 方法

```Python
# gunicorn/arbiter.py
class Arbiter(object):
    SIG_QUEUE = []
    def signal(self, sig, frame):
        if len(self.SIG_QUEUE) < 5:
            self.SIG_QUEUE.append(sig)
            self.wakeup()

    def wakeup(self):
        """\
        Wake up the arbiter by writing to the PIPE
        """
        try:
            os.write(self.PIPE[1], b'.')
        except IOError as e:
            if e.errno not in [errno.EAGAIN, errno.EINTR]:
                raise
```

当信号触发时，将信号入队，并调用 `wakeup()` 方法，向 `self.PIPE` 写入数据用于唤醒。其实感觉这里可以用 `signal.set_wakeup_fd()` 替代的，这点我不是很明白。特别地，对于子进程退出时向父进程发送的 `SIGCHLD`，我们需要通过 `wait()` 回收避免产生僵尸进程。这部分和 worker 相关代码分析放到后面

信号重绑定部分结束，让我们回到 `start()` 来看 `LISTENERS`

```Python
# gunicorn/sock.py
def create_sockets(conf, log, fds=None):
    """
    Create a new socket for the configured addresses or file descriptors.
    If a configured address is a tuple then a TCP socket is created.
    If it is a string, a Unix socket is created. Otherwise, a TypeError is
    raised.
    """
    # gunicorn 支持绑定多个 ADDR
    listeners = []

    # get it only once
    laddr = conf.address

    # check ssl config early to raise the error on startup
    # only the certfile is needed since it can contains the keyfile
    if conf.certfile and not os.path.exists(conf.certfile):
        raise ValueError('certfile "%s" does not exist' % conf.certfile)

    if conf.keyfile and not os.path.exists(conf.keyfile):
        raise ValueError('keyfile "%s" does not exist' % conf.keyfile)

    # sockets are already bound
    if fds is not None:
        for fd in fds:
            sock = socket.fromfd(fd, socket.AF_UNIX, socket.SOCK_STREAM)
            sock_name = sock.getsockname()
            sock_type = _sock_type(sock_name)
            listener = sock_type(sock_name, conf, log, fd=fd)
            listeners.append(listener)

        return listeners

    # no sockets is bound, first initialization of gunicorn in this env.
    for addr in laddr:
        sock_type = _sock_type(addr)
        sock = None
        # 重试 5 次
        for i in range(5):
            try:
                sock = sock_type(addr, conf, log)
            except socket.error as e:
                if e.args[0] == errno.EADDRINUSE:
                    log.error("Connection in use: %s", str(addr))
                if e.args[0] == errno.EADDRNOTAVAIL:
                    log.error("Invalid address: %s", str(addr))
                if i < 5:
                    msg = "connection to {addr} failed: {error}"
                    log.debug(msg.format(addr=str(addr), error=str(e)))
                    log.error("Retrying in 1 second.")
                    time.sleep(1)
            else:
                break

        if sock is None:
            log.error("Can't connect to %s", str(addr))
            sys.exit(1)

        listeners.append(sock)

    return listeners
```

回到 `run()` 方法中，在 `start()` 之后调用的是 `manage_workers()` 方法

```Python
# gunicorn/arbiter.py
class Arbiter(object):
    WORKERS = {}
    def manage_workers(self):
        """\
        Maintain the number of workers by spawning or killing
        as required.
        """
        if len(self.WORKERS.keys()) < self.num_workers:
            self.spawn_workers()

        workers = self.WORKERS.items()
        workers = sorted(workers, key=lambda w: w[1].age)
        while len(workers) > self.num_workers:
            (pid, _) = workers.pop(0)
            self.kill_worker(pid, signal.SIGTERM)

        active_worker_count = len(workers)
        if self._last_logged_active_worker_count != active_worker_count:
            self._last_logged_active_worker_count = active_worker_count
            # ... omit debug print
```

`WORKERS` 是一个 `dict` 记录了当前的 `worker`。如果当前的 `worker` 数量不足则 `spawn`；如果数量超出则 `kill`。也就是说会去维护了 `num_workers` 的 `worker`

回到 `run()` 方法进入死循环，这是 `Arbiter` 的最后职责。检查信号队列 `SIG_QUEUE`，根据信号类型去调用对应的 handler

```Python
# omit some code in while loop
while True:
    sig = self.SIG_QUEUE.pop(0) if len(self.SIG_QUEUE) else None
    if sig is None:
        self.sleep()
        self.murder_workers()
        self.manage_workers()
        continue
    signame = self.SIG_NAMES.get(sig)
    handler = getattr(self, "handle_%s" % signame, None)
    handler()
    self.wakeup()
```

注意一点这个 busy loop 内部使用了 `select`

```Python
# gunicorn/arbiter.py
class Arbiter(object):
    def sleep(self):
        """\
        Sleep until PIPE is readable or we timeout.
        A readable PIPE means a signal occurred.
        """
        try:
            ready = select.select([self.PIPE[0]], [], [], 1.0)
            if not ready[0]:
                return
            while os.read(self.PIPE[0], 1):
                pass
        except select.error as e:
            if e.args[0] not in [errno.EAGAIN, errno.EINTR]:
                raise
        except OSError as e:
            if e.errno not in [errno.EAGAIN, errno.EINTR]:
                raise
        except KeyboardInterrupt:
            sys.exit()
```

不知你注意到没有，没有主动 `raise KeyboardInterrupt` 的地方，而且重新注册了 `SIGINT` 的处理事件。所以这个 `KeyboardInterrupt` 异常的捕获逻辑貌似是多余的

`murder_workers()` 会 kill 超时的 `worker`，参见 [timeout 参数](http://docs.gunicorn.org/en/stable/settings.html#timeout)

`Arbiter` 中以 `handle_` 前缀，信号名称结尾的均为自定义的 handler，比如 `SIGINT` 的 handler

```Python
# gunicorn/arbiter.py
class Arbiter(object):
    def handle_int(self):
        "SIGINT handling"
        self.stop(False)
        raise StopIteration
```

### Reference
[Gunicorn 19.7.1 documentation](http://docs.gunicorn.org/en/19.7.1/)

    