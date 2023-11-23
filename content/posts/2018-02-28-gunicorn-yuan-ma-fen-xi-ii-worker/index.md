
+++
title = "Gunicorn 源码分析 II - Worker"
summary = ''
description = ""
categories = []
tags = []
date = 2018-02-28T10:38:09+08:00
draft = false
+++

*Gunicorn 19.7.1 版本源码，commit sha 为 328e509260ae70de6c04c5ba885ee17960b3ced5*

先来看 `worker` 是如何产生的

```Python
# gunicorn/arbiter.py
class Arbiter(object):
    def spawn_worker(self):
        self.worker_age += 1
        worker = self.worker_class(self.worker_age, self.pid, self.LISTENERS,
                                   self.app, self.timeout / 2.0,
                                   self.cfg, self.log)
        self.cfg.pre_fork(self, worker)  # call hook
        pid = os.fork()
        if pid != 0:
            worker.pid = pid
            self.WORKERS[pid] = worker
            return pid

        # Process Child
        worker.pid = os.getpid()
        try:
            util._setproctitle("worker [%s]" % self.proc_name)
            self.log.info("Booting worker with pid: %s", worker.pid)
            self.cfg.post_fork(self, worker)
            worker.init_process()
            sys.exit(0)  # never execute here
        except SystemExit:
            raise
        except AppImportError as e:
            self.log.debug("Exception while loading the application",
                           exc_info=True)
            print("%s" % e, file=sys.stderr)
            sys.stderr.flush()
            sys.exit(self.APP_LOAD_ERROR)
        except:
            self.log.exception("Exception in worker process"),
            if not worker.booted:
                sys.exit(self.WORKER_BOOT_ERROR)
            sys.exit(-1)
        finally:
            self.log.info("Worker exiting (pid: %s)", worker.pid)
            try:
                worker.tmp.close()
                self.cfg.worker_exit(self, worker)
            except:
                self.log.warning("Exception during worker exit:\n%s",
                                  traceback.format_exc())
```

Gunicorn 支持多种类型的 `worker`，默认为 `gunicorn.workers.sync.SyncWorker`，这里便以此为例

```
Worker
 +-- AiohttpWorker
 +-- TornadoWorker
 +-- ThreadWorker
 +-- SyncWorker
 +-- AsyncWorker
      +-- EventletWorker
      +-- GeventWorker
```

`Worker` 的定义如下

```Python
# gunicorn/workers/base.py
class Worker(object):

    SIGNALS = [getattr(signal, "SIG%s" % x)
            for x in "ABRT HUP QUIT INT TERM USR1 USR2 WINCH CHLD".split()]

    PIPE = []

    def __init__(self, age, ppid, sockets, app, timeout, cfg, log):
        """\
        This is called pre-fork so it shouldn't do anything to the
        current process. If there's a need to make process wide
        changes you'll want to do that in ``self.init_process()``.
        """
        self.age = age
        self.pid = "[booting]"
        self.ppid = ppid
        self.sockets = sockets
        self.app = app
        self.timeout = timeout
        self.cfg = cfg
        self.booted = False
        self.aborted = False
        self.reloader = None

        self.nr = 0
        # avoid all workers restarting at the same time
        jitter = randint(0, cfg.max_requests_jitter)
        self.max_requests = (cfg.max_requests + jitter) or MAXSIZE
        self.alive = True
        self.log = log
        self.tmp = WorkerTmp(cfg)
```

`nr` 属性记录当前处理的 req 数目，如果超过了 `max_requests` 则会将 `alive` 置为 `False`，`worker` 便会主动退出。这部分逻辑在 `handle_request()` 方法中

```Python
class Worker(object):
    def init_process(self):
        # set environment' variables
        if self.cfg.env:
            for k, v in self.cfg.env.items():
                os.environ[k] = v

        # 更改 uid gid
        util.set_owner_process(self.cfg.uid, self.cfg.gid,
                               initgroups=self.cfg.initgroups)

        # Reseed the random number generator
        util.seed()

        # For waking ourselves up
        self.PIPE = os.pipe()
        for p in self.PIPE:
            util.set_non_blocking(p)
            util.close_on_exec(p)

        # Prevent fd inheritance
        [util.close_on_exec(s) for s in self.sockets]
        util.close_on_exec(self.tmp.fileno())
        # include pipe
        self.wait_fds = self.sockets + [self.PIPE[0]]

        self.log.close_on_exec()

        self.init_signals()

        # start the reloader
        if self.cfg.reload:
            def changed(fname):
                self.log.info("Worker reloading: %s modified", fname)
                self.alive = False
                self.cfg.worker_int(self)
                time.sleep(0.1)
                sys.exit(0)

            reloader_cls = reloader_engines[self.cfg.reload_engine]
            self.reloader = reloader_cls(callback=changed)
            self.reloader.start()

        self.load_wsgi()
        self.cfg.post_worker_init(self)

        # Enter main run loop
        self.booted = True
        self.run()
```

`init_signals()` 方法重置了信号处理事件，并为 `SIGQUIT`、`SIGTERM`、`SIGINT`、`SIGWINCH`、`SIGUSR1`、`SIGABRT` 这些信号重新注册了 handler

`load_wsgi()` 方法调用了 `app.wsgi()` 加载了我们的 `wsgi app`

```Python
# gunicorn/app/base.py
class BaseApplication(object):
    def wsgi(self):
        if self.callable is None:
            self.callable = self.load()
        return self.callable

# gunicorn/app/wsgiapp.py
class WSGIApplication(Application):
    def load_wsgiapp(self):
        self.chdir()
        # load the app
        return util.import_app(self.app_uri)

    def load(self):
        if self.cfg.paste is not None:
            return self.load_pasteapp()
        else:
            return self.load_wsgiapp()

# gunicorn/util.py
def import_app(module):
    parts = module.split(":", 1)
    if len(parts) == 1:
        module, obj = module, "application"
    else:
        module, obj = parts[0], parts[1]

    try:
        __import__(module)
    except ImportError:
        if module.endswith(".py") and os.path.exists(module):
            msg = "Failed to find application, did you mean '%s:%s'?"
            raise ImportError(msg % (module.rsplit(".", 1)[0], obj))
        else:
            raise

    mod = sys.modules[module]

    is_debug = logging.root.level == logging.DEBUG
    try:
        # specify context
        app = eval(obj, vars(mod))
    except NameError:
        if is_debug:
            traceback.print_exception(*sys.exc_info())
        raise AppImportError("Failed to find application: %r" % module)

    if app is None:
        raise AppImportError("Failed to find application object: %r" % obj)

    if not callable(app):
        raise AppImportError("Application object must be callable.")
    return app
```

接下来看 `run()` 方法的实现，这是由对应的子类实现的

```Python
# gunicorn/app/sync.py
class SyncWorker(base.Worker):
    def run(self):
        # if no timeout is given the worker will never wait and will
        # use the CPU for nothing. This minimal timeout prevent it.
        timeout = self.timeout or 0.5

        # self.socket appears to lose its blocking status after
        # we fork in the arbiter. Reset it here.
        for s in self.sockets:
            s.setblocking(0)

        if len(self.sockets) > 1:
            self.run_for_multiple(timeout)
        else:
            self.run_for_one(timeout)
```

根据 `socket` 的数量使用了不同策略，以仅有一个 `socket` 的情况为例

```Python
# gunicorn/app/sync.py
class SyncWorker(base.Worker):
    def accept(self, listener):
        client, addr = listener.accept()
        client.setblocking(1)
        util.close_on_exec(client)
        # parse http req, call wsgi app, make resp
        self.handle(listener, client, addr)

    def run_for_one(self, timeout):
        listener = self.sockets[0]
        while self.alive:
            self.notify()
            # Accept a connection. If we get an error telling us
            # that no connection is waiting we fall down to the
            # select which is where we'll wait for a bit for new
            # workers to come give us some love.
            try:
                self.accept(listener)
                # Keep processing clients until no one is waiting. This
                # prevents the need to select() for every client that we
                # process.
                continue
            except EnvironmentError as e:
                if e.errno not in (errno.EAGAIN, errno.ECONNABORTED, errno.EWOULDBLOCK):
                    raise

            if not self.is_parent_alive():
                return

            try:
                self.wait(timeout)
            except StopWaiting:
                return
```

需要注意的是我们的 `socket` 在先前被置为 nonblocking，所以这里会异常 `socket.error: [Errno 11] Resource temporarily unavailable`。而这里使用 `EnvironmentError` 则是为了版本兼容，可以参考 [文档](https://docs.python.org/3/library/exceptions.html#EnvironmentError)，在 Python 3.3 后 `EnvironmentError` 和 `IOError` 都是 `OSError` 的 Alias

接下来的 `is_parent_alive()` 方法通过检查目前的 `ppid` 和 `worker` 诞生时的 `ppid` 是否相同来判断父进程是否还活着。因为如果父进程挂掉，子进程会被 init 进程收留，这时 `ppid` 会变成 `1`

`wait()` 中则是通过 `select` (并未没有使用 `epoll`) 来等待 `socket` 或者 `PIPE` 就绪

```Python
# gunicorn/app/sync.py
class SyncWorker(base.Worker):
    def wait(self, timeout):
        try:
            self.notify()
            # wait_fds include PIPE
            ret = select.select(self.wait_fds, [], [], timeout)
            if ret[0]:
                if self.PIPE[0] in ret[0]:
                    os.read(self.PIPE[0], 1)
                return ret[0]
        except select.error as e:
            if e.args[0] == errno.EINTR:
                return self.sockets
            if e.args[0] == errno.EBADF:
                if self.nr < 0:
                    return self.sockets
                else:
                    raise StopWaiting
            raise
```

之前提到过 `worker` 也重新注册了部分 `signal` 的 handler，因为 `Arbiter` 和 `worker` 间也通过信号进行通信

```Python
# gunicorn/arbiter.py
class Arbiter(object):
    def kill_worker(self, pid, sig):
        try:
            os.kill(pid, sig)
        except OSError as e:
            if e.errno == errno.ESRCH:
                try:
                    worker = self.WORKERS.pop(pid)
                    worker.tmp.close()
                    # server hook
                    self.cfg.worker_exit(self, worker)
                    return
                except (KeyError, OSError):
                    return
            raise
```

类似下图表示

```
                        ----------
                        | worker | (wait connection)
           (manage)     ----------
          -----------  /
signal -> | Arbiter |
          -----------  \
                        ----------
                        | worker | (wait connection)
                        ----------
```

这里拿 `reload` 操作为例。通过向 `Arbiter` 进程发送 `HUP` 信号可以 `reload` 我们的 `worker`，而且是平滑的 `reload`。不会导致现有的服务中断(自称)

根据上一篇的分析，`Arbiter` 进程接收到信号后，经过入队、出队后会执行对应的 handler `handle_hup()`。此方法去调用 `reload()` 方法

```Python
# gunicorn/arbiter.py
class Arbiter(object):
    def reload(self):
        old_address = self.cfg.address

        # reset old environment
        for k in self.cfg.env:
            if k in self.cfg.env_orig:
                # reset the key to the value it had before
                # we launched gunicorn
                os.environ[k] = self.cfg.env_orig[k]
            else:
                # delete the value set by gunicorn
                try:
                    del os.environ[k]
                except KeyError:
                    pass

        # reload conf
        self.app.reload()
        self.setup(self.app)

        # reopen log files
        self.log.reopen_files()

        # do we need to change listener ?
        if old_address != self.cfg.address:
            # close all listeners
            [l.close() for l in self.LISTENERS]
            # init new listeners
            self.LISTENERS = sock.create_sockets(self.cfg, self.log)
            listeners_str = ",".join([str(l) for l in self.LISTENERS])
            self.log.info("Listening at: %s", listeners_str)

        # call server hook
        self.cfg.on_reload(self)

        # unlink pidfile
        if self.pidfile is not None:
            self.pidfile.unlink()

        # create new pidfile
        if self.cfg.pidfile is not None:
            self.pidfile = Pidfile(self.cfg.pidfile)
            self.pidfile.create(self.pid)

        # set new proc_name
        util._setproctitle("master [%s]" % self.proc_name)

        # spawn new workers
        for i in range(self.cfg.workers):
            self.spawn_worker()

        # manage workers
        self.manage_workers()
```

可以看到首先是通过 `app.reload()` 实现配置文件的 `reload`，然后调用 `setup()` 方法覆盖了之前的一些属性比如 `worker_class`、`worker_num` 等。如果需要更换 `ADDR` 则关闭先前的 `LISTENERS` 然后重新创建

接着是创建指定数量的 `worker`。此时 Gunicorn 是新旧 `worker` 共同存在的状态，二者同时接入流量。调用 `manage_workers()` 方法，显然当前的 `worker` 数量已经超出，所以 `kill` 掉那些历史残留

```Python
# gunicorn/arbiter.py
class Arbiter(object):
    def manage_workers(self):
        workers = self.WORKERS.items()
        workers = sorted(workers, key=lambda w: w[1].age)
        while len(workers) > self.num_workers:
            (pid, _) = workers.pop(0)
        self.kill_worker(pid, signal.SIGTERM)
```

`worker` 会接收到 `SIGTERM` 信号，调用 `handle_exit()` 方法

```Python
# gunicorn/workers/base.py
class Worker(object):
    def handle_exit(self, sig, frame):
        self.alive = False
```

此方法就是简单的将 `FLAG` 置为 `False`。参考 `run_for_one()`，`worker` 会主动退出。之后 master 进程会收到 `SIGCHLD` 信号去 `wait` 已经退出的子进程，避免出现僵尸进程。如此一般，便实现了 `worker` 的新旧交替

Gunicorn 的基本工作流程就这样了。现在提一个问题，在使用 `SyncWorker`，`worker` 数量为 9 且均空闲的情况下，如果有连接到来会有几个进程被唤醒？答案所有进程。不过如果你添一个 `print` debug 时，可能只会看到 1~9 个输出。全部唤醒是由于 Gunicorn 没有处理 epoll 惊群，但由于 kernel 层面处理了 accept 惊群所以虽然有多个进程被唤醒，不过最终还是只有一个进程 accept 成功。而 debug 时不能每次浮现所有进程被唤醒的情形是因为已经有进程 accpet 成功了，事件被清除，其余进程不会唤醒

    