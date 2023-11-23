
+++
title = "由 Gevent SSL 引发的 XY Problem"
summary = ''
description = ""
categories = []
tags = []
date = 2019-03-24T06:18:19+08:00
draft = false
+++

本文所使用的环境

- Python==3.7.2
- gevent==1.4.0
- gunicron==19.9.0
- urllib3==1.24.1
- requests=2.21.0

可以说是很早就存在的坑了，但是由于不恰当的解决方式有引发的其他的问题

```Python
# app.py
import requests
from flask import Flask
from flask import jsonify
app = Flask(__name__)


@app.route("/")
def hello():
    # import requests 这是一种解决方法
    resp = requests.get('https://httpbin.org/get')
    return jsonify(resp.json())

# run.py
from gunicorn.app.base import BaseApplication
from app import app


class StandaloneApplication(BaseApplication):

    def __init__(self, app, options=None):
        self.options = options or {}
        self.application = app
        super().__init__()

    def load_config(self):
        config = {key: value for key, value in self.options.items()
                  if key in self.cfg.settings and value is not None}
        for key, value in config.items():
            self.cfg.set(key.lower(), value)

    def load(self):
        return self.application


if __name__ == '__main__':
    options = {
        'bind': '%s:%s' % ('127.0.0.1', '8000'),
        'workers': 2,
        'worker_class': 'gevent'
    }
    StandaloneApplication(app, options).run()
```

运行时会有 Warning(一些老版本没有)

```
/root/py37/lib/python3.7/site-packages/gunicorn/workers/ggevent.py:65: MonkeyPatchWarning: Monkey-patching ssl after ssl has already been imported may lead to errors, including RecursionError on Python 3.6. It may also silently lead to incorrect behaviour on Python 3.7. Please monkey-patch earlier. See https://github.com/gevent/gevent/issues/1016. Modules that had direct imports (NOT patched): ['urllib3.util.ssl_ (/root/py37/lib/python3.7/site-packages/urllib3/util/ssl_.py)', 'urllib3.util (/root/py37/lib/python3.7/site-packages/urllib3/util/__init__.py)'].
  monkey.patch_all(subprocess=True)
```

并且在请求的时候会产生无限递归

```
  File "/root/py37/lib/python3.7/site-packages/urllib3/util/ssl_.py", line 281, in create_urllib3_context
    context.options |= options
  File "/usr/lib/python3.7/ssl.py", line 507, in options
    super(SSLContext, SSLContext).options.__set__(self, value)
  File "/usr/lib/python3.7/ssl.py", line 507, in options
    super(SSLContext, SSLContext).options.__set__(self, value)
  File "/usr/lib/python3.7/ssl.py", line 507, in options
    super(SSLContext, SSLContext).options.__set__(self, value)
  [Previous line repeated 475 more times]
RecursionError: maximum recursion depth exceeded while calling a Python object
```

这个问题可以追溯到两年前的 https://github.com/gevent/gevent/issues/1016

当我们在 gunicorn  中使用 gevent 的 worker 时，会进行 patch

```Python
# https://github.com/benoitc/gunicorn/blob/29f0394cdd381df176a3df3c25bb3fdd2486a173/gunicorn/workers/ggevent.py#L57
    def patch(self):
        from gevent import monkey
        monkey.noisy = False

        # if the new version is used make sure to patch subprocess
        if gevent.version_info[0] == 0:
            monkey.patch_all()
        else:
            monkey.patch_all(subprocess=True)
```

如果我们在 patch 之前`from ssl import SSLContext `便会复现这个问题，因为`requests`所依赖的 [`urllib3`进行了这个操作](https://github.com/urllib3/urllib3/blob/a6ec68a5c5c5743c59fe5c62c635c929586c429b/src/urllib3/util/ssl_.py#L113)，所以产生了问题

下面便来分析一下问题无限递归产生的原因，简化一下代码逻辑，首先来看下面的三个例子

```Python
import ssl
from ssl import SSLContext

from gevent import monkey
monkey.patch_all()

context = SSLContext(ssl.PROTOCOL_SSLv23)
context.options |= 0  # Boom
```

```Python
import ssl

from gevent import monkey
monkey.patch_all()

context = ssl.SSLContext(ssl.PROTOCOL_SSLv23)
context.options |= 0  # Ok
```

```Python
import ssl

from gevent import monkey
monkey.patch_all()

from ssl import SSLContext

context = SSLContext(ssl.PROTOCOL_SSLv23)
context.options |= 0  # Ok
```

这里我们使用的依旧是没有被 patch 的 `SSLContext`(`ssl.SSLContext`)，而不是`gevent._ssl3.SSLContext`。所以在 `context.options |= 0` 时会调用

```Python
# https://github.com/python/cpython/blob/9a3ffc0492d1310ead9ce8f5ee678c26b20a338d/Lib/ssl.py#L505
class SSLContext(_SSLContext):
    @options.setter
    def options(self, value):
        super(SSLContext, SSLContext).options.__set__(self, value)
```

但是执行这一步的时候又在 patch 之后，所以`SSLContext`变成里`gevent._ssl3.SSLContext`，而它又是
`ssl.SSLContext`的子类。所以这里的 super 会找到自己然后就无限递归了



```Python
@options.setter
def options(self, value):
	print(SSLContext.__module__)
	context = super(SSLContext, SSLContext)
	print(context.__module__)
    context.options.__set__(self, value)
```

可以在`ssl`的源代码中打印一下 `__module__`，输出

```
gevent._ssl3
ssl
```

为了解决这个问题，在 `run.py` 的开始处调用了 `monkey.patch_all()`。这个虽然会 patch 两次，但是没有问题。真正的问题在于提前 patch 会造成 APScheduler 的 `BackgroundScheduler` 重复执行任务

我们有一个 `create_app` 的函数，就像这样

```Python
def create_app():
    app = Flask(__name__.split('.')[0], template_folder=Config.TEMPLATE_FOLDER)
    app.config.from_object(Config)
    register_extensions(app)
    register_blueprints(app)
    register_schedulers(app)
    return app
```

它会进行一些操作然后返回 app 实例，最后传入到 gunicorn 中

```Python
StandaloneApplication(create_app(), options).run()
```

APSchduler 中的 `BackgroundScheduler`使用的是线程，提前 patch 会导致它变成 greenlet。然后 gunicorn 在 fork 时，子进程也会得到拷贝。导致执行一次的定时任务被执行 works + 1 次

既然这样便将 `BackgroundScheduler` 替换为 `BlockingScheduler`，然后放到一个进程中

```Python
def register_schedulers(app: Flask):
    """ Register schedulers. """
    cron = BlockingScheduler(config={
        'timezone': app.config['TIMEZONE']
    })
    now = datetime.datetime.now(tz=app.config['TIMEZONE'])
    cron.add_job(
        cron1,
        args=[app],
        trigger=IntervalTrigger(seconds=60 * 60 * 24)
    )
    def start():
        try:
            cron.start()
        except KeyboardInterrupt:
            cron.shutdown()
            sys.exit(0)

    Process(target=start, daemon=True).start()
```

这样便不会存在执行多次的问题了，但是它又引发了另一个问题

在退出的时候 `AssertionError`

```
Error in atexit._run_exitfuncs:
Traceback (most recent call last):
  File "multiprocessing/util.py", line 319, in _exit_function
  File "multiprocessing/process.py", line 122, in join
AssertionError: can only join a child process
```

这个报错出在 `multiprocessing` 使用 `atexit` 注册了退出函数中

```Python
# https://github.com/python/cpython/blob/9a3ffc0492d1310ead9ce8f5ee678c26b20a338d/Lib/multiprocessing/util.py#L327
def _exit_function(info=info, debug=debug, _run_finalizers=_run_finalizers,
                   active_children=process.active_children,
                   current_process=process.current_process):
    # We hold on to references to functions in the arglist due to the
    # situation described below, where this function is called after this
    # module's globals are destroyed.

    global _exiting

    if not _exiting:
        _exiting = True

        info('process shutting down')
        debug('running all "atexit" finalizers with priority >= 0')
        _run_finalizers(0)

        if current_process() is not None:
            # We check if the current process is None here because if
            # it's None, any call to ``active_children()`` will raise
            # an AttributeError (active_children winds up trying to
            # get attributes from util._current_process).  One
            # situation where this can happen is if someone has
            # manipulated sys.modules, causing this module to be
            # garbage collected.  The destructor for the module type
            # then replaces all values in the module dict with None.
            # For instance, after setuptools runs a test it replaces
            # sys.modules with a copy created earlier.  See issues
            # #9775 and #15881.  Also related: #4106, #9205, and
            # #9207.

            for p in active_children():
                if p.daemon:
                    info('calling terminate() for daemon %s', p.name)
                    p._popen.terminate()

            for p in active_children():
                info('calling join() for process %s', p.name)
                p.join()

        debug('running the remaining "atexit" finalizers')
        _run_finalizers()

atexit.register(_exit_function)
```

因为我们在 gunicorn fork worker 之前 `create_app`，便产生了一个新的进程去执行调度任务。fork 后 `Process` 对象也被继承下来，在退出的时候子进程也会执行使用 `atexit`所注册的退出函数。但是它的 `Process`对象是继承下来的

```Python
# https://github.com/python/cpython/blob/9a3ffc0492d1310ead9ce8f5ee678c26b20a338d/Lib/multiprocessing/process.py#L133
class BaseProcess(object):
    def join(self, timeout=None):
        '''
        Wait until child process terminates
        '''
        self._check_closed()
        assert self._parent_pid == os.getpid(), 'can only join a child process'
        assert self._popen is not None, 'can only join a started process'
        res = self._popen.wait(timeout)
        if res is not None:
            _children.discard(self)
```

会导致 `assert self._parent_pid == os.getpid()` 左边是 parent 右边是自己。形象点解释便是

``` 
parent --- cron
      |
       --- worker
```

`Process`对象代表进程 `cron`，它的 `_parent_pid`便是 `parent`的 pid。在 `parent`中调用 `join`的时候便没有问题，但是在 `worker`中便会出问题

一个补救的办法，便是利用 `gunicorn` 的 `post_worker_init`在子进程中清理掉

```Python
def post_worker_init(worker):
    for child in multiprocessing.process.active_children():
        if child.name == 'xxx':
            multiprocessing.process._children.remove(child)
```



但是我们回过神来思考一下，这不是典型的 XY Problem 么。我们最初遇到的是 gevent ssl 的问题，为什么后面变成了修复我们的前一个修复方案所产生的问题呢？

这时我们应该更改 gevent ssl 的解决方案才对，比如 `monkey.patch_ssl()` 而不是 `monkey_patch_all()`
    