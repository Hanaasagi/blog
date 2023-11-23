
+++
title = "探秘 New Relic Python agent"
summary = ''
description = ""
categories = []
tags = []
date = 2019-08-18T09:30:15+08:00
draft = false
+++

*本文为阅读 [NewRelic Python agent](https://pypi.python.org/pypi/newrelic) 源代码时的思考，主要讨论如何在不修改用户代码的前提下在 Python 中实现模块级别的 hook。使用到的代码版本为 4.20.1.121*

`newrelic-admin` 调用的是位于 `newrelic/admin/__init__.py` 中的 `main` 函数，不过在这之前有两个 module 中执行的函数 `load_internal_plugins` 和 `load_external_plugins`

```Python
# newrelic/admin/__init__.py
_builtin_plugins = [
    # ...
    'run_program',
    'run_python',

]

def load_internal_plugins():
    for name in _builtin_plugins:
        module_name = '%s.%s' % (__name__, name)
        __import__(module_name)


def load_external_plugins():
    try:
        import pkg_resources
    except ImportError:
        return

    group = 'newrelic.admin'

    for entrypoint in pkg_resources.iter_entry_points(group=group):
        __import__(entrypoint.module_name)

load_internal_plugins()
load_external_plugins()
```

它们分别用于加载内置和外部的插件，比如 `run_program`。这些 plugin 可以借助 `newrelic.admin.command` 装饰器注册自己的命令行参数

当我们使用 `run-program` 去执行用户程序的时候，agent 会修改掉 `PYTHONPATH`

```Python
# newrelic/admin/run_program.py
@command('run-program', '...', 'Executes the command ...')
def run_program(args):
    # 仅摘取关键代码
    from newrelic import __file__ as root_directory

    root_directory = os.path.dirname(root_directory)
    boot_directory = os.path.join(root_directory, 'bootstrap')
    python_path = boot_directory

    if 'PYTHONPATH' in os.environ:
        path = os.environ['PYTHONPATH'].split(os.path.pathsep)
        if boot_directory not in path:
            python_path = "%s%s%s" % (boot_directory, os.path.pathsep,
                    os.environ['PYTHONPATH'])
    os.environ['PYTHONPATH'] = python_path
```

newrelic 安装路径下的 bootstrap 目录会被添加到 `PYTHONPATH` 的最前面，这样便可以通过最高优先级来导入 `newrelic` 的 module

然后 newrelic 做完这些准备工作后，就去执行用户的命令了(环境变量是会被继承的参考之前写过的文章)

```Python
# newrelic/admin/run_program.py
@command('run-program', '...', 'Executes the command ...')
def run_program(args):
    # 仅摘取关键代码
    program_exe_path = args[0]  # args 为用户命令，比如 python app.py

    # 根据 PATH 寻找 bin 文件的完整路径
    if not os.path.dirname(program_exe_path):
        program_search_path = os.environ.get('PATH', '').split(os.path.pathsep)
        for path in program_search_path:
            path = os.path.join(path, program_exe_path)
            if os.path.exists(path) and os.access(path, os.X_OK):
                program_exe_path = path
                break

    log_message('program_exe_path = %r', program_exe_path)
    log_message('execl_arguments = %r', [program_exe_path] + args)

    # 执行
    os.execl(program_exe_path, *args)
```

我们回头来看 bootstrap 目录下面有什么

```
bootstrap
├── __init__.py
└── sitecustomize.py
```

解释器在初始化的时候会自动导入 `PYTHONPATH` 下存在的 `sitecustomize` 和 `usercustomize`(`sitecustomize` 优先级更高)。这样 newrelic 便可以在应用本身中做手脚了

首先 newrelic 会去尝试导入原本的 `sitecustomize`，因为担心会再次导入自身(因为 bootstrap 路径在最前面)而且 import system 自身可能会存在缓存，所以这里在从 `sys.path` 中删除 bootstrap 路径后使用 `imp` 去导入而不是直接 `import`

```Python
# newrelic/bootstrap/sitecustomize.py
import imp
# We need to import the original sitecustomize.py file if it exists. We
# can't just try and import the existing one as we will pick up
# ourselves again. Even if we remove ourselves from sys.modules and
# remove the bootstrap directory from sys.path, still not sure that the
# import system will not have cached something and return a reference to
# ourselves rather than searching again. What we therefore do is use the
# imp module to find the module, excluding the bootstrap directory from
# the search, and then load what was found.

boot_directory = os.path.dirname(__file__)
root_directory = os.path.dirname(os.path.dirname(boot_directory))

path = list(sys.path)

if boot_directory in path:
    del path[path.index(boot_directory)]

try:
    (file, pathname, description) = imp.find_module('sitecustomize', path)
except ImportError:
    pass
else:
    imp.load_module('sitecustomize', file, pathname, description)
```

然后 newrelic agent 进行初始化

```Python
# newrelic/bootstrap/sitecustomize.py
import newrelic.config
newrelic.config.initialize(config_file, environment)
```

```Python
# newrelic/config.py

def initialize(config_file=None, environment=None, ignore_errors=None,
            log_file=None, log_level=None):
    if config_file is None:
        config_file = os.environ.get('NEW_RELIC_CONFIG_FILE', None)
    if environment is None:
        environment = os.environ.get('NEW_RELIC_ENVIRONMENT', None)
    if ignore_errors is None:
        ignore_errors = newrelic.core.config._environ_as_bool(
                'NEW_RELIC_IGNORE_STARTUP_ERRORS', True)

    _load_configuration(config_file, environment, ignore_errors,
            log_file, log_level)
    if _settings.monitor_mode or _settings.developer_mode:
        _settings.enabled = True
        _setup_instrumentation()
        _setup_data_source()
        _setup_extensions()
        _setup_agent_console()
    else:
        _settings.enabled = False
```

在 `sys.meta_path` 中添加了自定义的 finder，实现了对 import 行为的拦截(`from xx import yy` 也会触发 `xx` 的 finder)，参考文档 [`sys.meta_path`](https://docs.python.org/3/library/sys.html#sys.meta_path) 和 [PEP-302](https://www.python.org/dev/peps/pep-0302/)

```Python
sys.meta_path.insert(0, newrelic.api.import_hook.ImportHookFinder())
```

finder 和 loader 的实现

```Python
def _notify_import_hooks(name, module):
    # Is assumed that this function is called with the global
    # import lock held. This should be the case as should only
    # be called from load_module() of the import hook loader.
    hooks = _import_hooks.get(name, None)
    if hooks is not None:
        _import_hooks[name] = None

        for callable in hooks:
            callable(module)

class _ImportHookChainedLoader:

    def __init__(self, loader):
        self.loader = loader

    def load_module(self, fullname):
        module = self.loader.load_module(fullname)
        # Call the import hooks on the module being handled.
        _notify_import_hooks(fullname, module)
        return module

class ImportHookFinder:

    def __init__(self):
        self._skip = {}

    def find_module(self, fullname, path=None):
        # If not something we are interested in we can return.
        if fullname not in _import_hooks:
            return None
        # Check whether this is being called on the second time
        # through and return.
        if fullname in self._skip:
            return None

        # We are now going to call back into import. We set a
        # flag to see we are handling the module so that check
        # above drops out on subsequent pass and we don't go
        # into an infinite loop.
        self._skip[fullname] = True

        try:
            loader = find_loader(fullname, path)  # importlib.find_loader
            if loader:
                return _ImportHookChainedLoader(loader)
        finally:
            del self._skip[fullname]
```

在 Python 3.4 后我们应当在 finder 对象中实现 `find_spec` 方法，`find_module` 只是作为 `find_spec` 不存在时的 callback。这里同时兼容了 Python 2 和 3

`register_import_hook` 函数负责将 hook 注册到 `_import_hooks` 中，那么什么时候注册进来的呢，让我们回到 `initialize` 中

1) `_load_configuration` 中加载了用户自定义的 trace hook
2) `_setup_instrumentation` 中加载了内置的 trace hook，其中又分为多个维度
3) ...

这里选择其中一部分来举例子


```Python
def _setup_instrumentation():
    # ...
    _process_module_builtin_defaults()

def _process_module_builtin_defaults():
    # ...
    _process_module_definition('flask.app',
            'newrelic.hooks.framework_flask',
            'instrument_flask_app')
    _process_module_definition('flask.templating',
            'newrelic.hooks.framework_flask',
            'instrument_flask_templating')
    _process_module_definition('flask.blueprints',
            'newrelic.hooks.framework_flask',
            'instrument_flask_blueprints')
    _process_module_definition('flask.views',
            'newrelic.hooks.framework_flask',
            'instrument_flask_views')
```

所有的 hook 都放在 newrelic/hooks 目录下面

```Python
# newrelic/hooks/framework_flask.py
def instrument_flask_app(module):
    # ...
    wrap_function_wrapper(module, 'Flask.handle_http_exception',
            _nr_wrapper_Flask_handle_http_exception_)
```

使用 `_nr_wrapper_Flask_handle_http_exception_` 去装饰原生的函数，然后记录数据上传到 Server 端

### Summary

总结一下:

- 修改 `PYTHONPATH`，利用 `sitecustomize` 导入自己的 hook
- `sys.meta_path` 添加自己的 finder
- 装饰原生函数
- 可能还有其他的魔法？

根据 newrelic agent 的现有代码量来说，如果要造一个推送应用 metrics 数据到 push gateway，然后由 Prometheus 做 APM 的轮子好像不如直接改代码来的方便一些

    