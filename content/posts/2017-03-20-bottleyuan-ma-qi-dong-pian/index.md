
+++
title = "Bottle源码-启动篇"
summary = ''
description = ""
categories = []
tags = []
date = 2017-03-20T06:35:57+08:00
draft = false
+++

本文基于 [bottle-0.12.13](https://pypi.python.org/pypi/bottle/0.12.13)

从 run 函数（程序入口）开始分析

    def run(app=None, server='wsgiref', host='127.0.0.1', port=8080,
            interval=1, reloader=False, quiet=False, plugins=None,
            debug=None, **kargs):
        """ Start a server instance. This method blocks until the server terminates.

            :param app: WSGI application or target string supported by
                   :func:`load_app`. (default: :func:`default_app`)
            :param server: Server adapter to use. See :data:`server_names` keys
                   for valid names or pass a :class:`ServerAdapter` subclass.
                   (default: `wsgiref`)
            :param host: Server address to bind to. Pass ``0.0.0.0`` to listens on
                   all interfaces including the external one. (default: 127.0.0.1)
            :param port: Server port to bind to. Values below 1024 require root
                   privileges. (default: 8080)
            :param reloader: Start auto-reloading server? (default: False)
            :param interval: Auto-reloader interval in seconds (default: 1)
            :param quiet: Suppress output to stdout and stderr? (default: False)
            :param options: Options passed to the server adapter.
         """
        if NORUN: return
        # 自动重新载入 server
        if reloader and not os.environ.get('BOTTLE_CHILD'):
            try:
                lockfile = None
                # For example: (10, '/tmp/bottle.gHcHRe.lock')
                fd, lockfile = tempfile.mkstemp(prefix='bottle.', suffix='.lock')
                os.close(fd) # We only need this file to exist. We never write to it
                while os.path.exists(lockfile):
                    args = [sys.executable] + sys.argv
                    environ = os.environ.copy()
                    environ['BOTTLE_CHILD'] = 'true'
                    environ['BOTTLE_LOCKFILE'] = lockfile
                    # 开启子进程
                    p = subprocess.Popen(args, env=environ)
                    # poll() 检测子进程是否停止
                    # None 子进程未停止
                    # A negative value -N indicates that
                    # the child was terminated by signal N (POSIX only).
                    while p.poll() is None: # Busy wait...
                        # 更新文件修改时间
                        os.utime(lockfile, None) # I am alive!
                        time.sleep(interval)
                    # 子进程以信号 3 退出，证明文件发生改变，重新开启子进程
                    if p.poll() != 3:
                        if os.path.exists(lockfile): os.unlink(lockfile)
                        sys.exit(p.poll())
            except KeyboardInterrupt:
                pass
            finally:
                if os.path.exists(lockfile):
                    os.unlink(lockfile)
            return

        try:
            if debug is not None: _debug(debug) # 1
            app = app or default_app() # 2
            import code
            if isinstance(app, basestring):
                app = load_app(app) # 3
            if not callable(app):
                raise ValueError("Application is not callable: %r" % app)

            for plugin in plugins or []:
                app.install(plugin)

            # 创建对应的 server 对象
            if server in server_names:
                server = server_names.get(server)
            if isinstance(server, basestring):
                server = load(server)
            if isinstance(server, type):
                server = server(host=host, port=port, **kargs)
            if not isinstance(server, ServerAdapter):
                raise ValueError("Unknown or unsupported server: %r" % server)

            server.quiet = server.quiet or quiet
            if not server.quiet:
                _stderr("Bottle v%s server starting up (using %s)...\n" % (__version__, repr(server)))
                _stderr("Listening on http://%s:%d/\n" % (server.host, server.port))
                _stderr("Hit Ctrl-C to quit.\n\n")

            if reloader:
                # 这里只有子进程才会执行
                lockfile = os.environ.get('BOTTLE_LOCKFILE')
                bgcheck = FileCheckerThread(lockfile, interval) # 4
                with bgcheck:
                    server.run(app)
                if bgcheck.status == 'reload':
                    # 子进程主动退出
                    sys.exit(3)
            else:
                server.run(app)
        except KeyboardInterrupt:
            pass
        except (SystemExit, MemoryError):
            raise
        except:
            if not reloader: raise
            if not getattr(server, 'quiet', quiet):
                print_exc()
            time.sleep(interval)
            sys.exit(3)

01) _debug 定义在 run 函数的前面

    def debug(mode=True):
        """ Change the debug level.
        There is only one debug level supported at the moment."""
        global DEBUG
        if mode: warnings.simplefilter('default')
        DEBUG = bool(mode)

    _debug = debug


02) 如果没有传入一个 app 实例的话，会对 default_app 对象调用 `__call__` 方法得到栈顶的对象

    class AppStack(list):
        """ A stack-like list. Calling it returns the head of the stack. """

        def __call__(self):
            """ Return the current default application. """
            return self[-1]

        def push(self, value=None):
            """ Add a new :class:`Bottle` instance to the stack """
            if not isinstance(value, Bottle):
                value = Bottle()
            self.append(value)
            return value

    app = default_app = AppStack()
    app.push()

这里执行了 push() 入栈了一个 Bottle 类的实例

03）并且如果 app 是一个字符串的话，会去调用 load_app()，实现从字符串制定的 moduel 加载 app

    def load(target, **namespace):
        """ Import a module or fetch an object from a module.

            * ``package.module`` returns `module` as a module object.
            * ``pack.mod:name`` returns the module variable `name` from `pack.mod`.
            * ``pack.mod:func()`` calls `pack.mod.func()` and returns the result.

            The last form accepts not only function calls, but any type of
            expression. Keyword arguments passed to this function are available as
            local variables. Example: ``import_string('re:compile(x)', x='[a-z]')``
        """
        module, target = target.split(":", 1) if ':' in target else (target, None)
        if module not in sys.modules: __import__(module)
        if not target: return sys.modules[module]
        if target.isalnum(): return getattr(sys.modules[module], target)
        package_name = module.split('.')[0]
        namespace[package_name] = sys.modules[package_name]
        return eval('%s.%s' % (module, target), namespace)


    def load_app(target):
        """ Load a bottle application from a module and make sure that the import
            does not affect the current default application, but returns a separate
            application object. See :func:`load` for the target parameter. """
        global NORUN; NORUN, nr_old = True, NORUN
        try:
            tmp = default_app.push() # Create a new "default application"
            rv = load(target) # Import the target module
            return rv if callable(rv) else tmp
        finally:
            default_app.remove(tmp) # Remove the temporary added default application
            NORUN = nr_old

04）新开一个线程检测文件修改时间

    class FileCheckerThread(threading.Thread):
        ''' Interrupt main-thread as soon as a changed module file is detected,
            the lockfile gets deleted or gets to old. '''

        def __init__(self, lockfile, interval):
            threading.Thread.__init__(self)
            self.lockfile, self.interval = lockfile, interval
            #: Is one of 'reload', 'error' or 'exit'
            self.status = None

        def run(self):
            exists = os.path.exists
            mtime = lambda path: os.stat(path).st_mtime
            files = dict()

            for module in list(sys.modules.values()):
                path = getattr(module, '__file__', '')
                if path[-4:] in ('.pyo', '.pyc'):
                    path = path[:-1]
                if path and exists(path):
                    files[path] = mtime(path)

            while not self.status:
                if not exists(self.lockfile)\
                or mtime(self.lockfile) < time.time() - self.interval - 5:
                    self.status = 'error'
                    thread.interrupt_main()
                # 检测每个文件的修改时间，若修改则向主线程发出 KeyboardInterrupt
                # 结束主线程中的 with 语句
                for path, lmtime in list(files.items()):
                    if not exists(path) or mtime(path) > lmtime:
                        self.status = 'reload'
                        thread.interrupt_main()
                        break
                time.sleep(self.interval)

        def __enter__(self):
            self.start()

        def __exit__(self, exc_type, exc_val, exc_tb):
            if not self.status: self.status = 'exit' # silent exit
            self.join()
            # return True 说明异常已经被处理，不会再向 with 语句外层的 try/except 抛出
            # return False 会触发外层的 except 块
            return exc_type is not None and issubclass(exc_type, KeyboardInterrupt)

傻逼作者画了这么张图

    -----------         -------------
    |         |  poll() |           |
    |  main   |   ==>   |   sub     |
    | process |   <==   |  process  |
    |         |  signal | run server|
    -----------         -------------
                          ||    /\
                          \/    || if file updated, `thread.interrupt_main()`
                        -------------
                        |           |
                        |  thread   |
                        |           |
                        |check file |
                        -------------


之后来探讨一下 server 部分

先来看一下 ServerAdapter 这个适配器类

    class ServerAdapter(object):
        quiet = False
        def __init__(self, host='127.0.0.1', port=8080, **options):
            self.options = options
            self.host = host
            self.port = int(port)

        def run(self, handler): # pragma: no cover
            pass

        def __repr__(self):
            args = ', '.join(['%s=%s'%(k,repr(v)) for k, v in self.options.items()])
            return "%s(%s)" % (self.__class__.__name__, args)

通过继承 ServerAdapter 重写 run 方法，实现了如下 server

    ServerAdapter
     +-- CGIServer
     +-- FlupFCGIServer
     # ...
     +-- BjoernServer
     +-- AutoServer

并将这些累关联到一个 server_names 字典中

    server_names = {
        'cgi': CGIServer,
        'flup': FlupFCGIServer,
        # ...
        'bjoern' : BjoernServer,
        'auto': AutoServer,
    }

默认的使用了 WSGIRefServer

    class WSGIRefServer(ServerAdapter):
        def run(self, app): # pragma: no cover
            from wsgiref.simple_server import WSGIRequestHandler, WSGIServer
            from wsgiref.simple_server import make_server
            import socket

            class FixedHandler(WSGIRequestHandler):
                def address_string(self): # Prevent reverse DNS lookups please.
                    return self.client_address[0]
                def log_request(*args, **kw):
                    if not self.quiet:
                        return WSGIRequestHandler.log_request(*args, **kw)

            handler_cls = self.options.get('handler_class', FixedHandler)
            server_cls  = self.options.get('server_class', WSGIServer)

            if ':' in self.host: # Fix wsgiref for IPv6 addresses.
                if getattr(server_cls, 'address_family') == socket.AF_INET:
                    class server_cls(server_cls):
                        address_family = socket.AF_INET6

            srv = make_server(self.host, self.port, app, server_cls, handler_cls)
            srv.serve_forever()
    