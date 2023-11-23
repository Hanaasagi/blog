
+++
title = "Bottle源码-路由"
summary = ''
description = ""
categories = []
tags = []
date = 2017-03-25T12:12:18+08:00
draft = false
+++

本文基于 [bottle-0.12.13](https://pypi.python.org/pypi/bottle/0.12.13)  

先从一个简单的例子说起， 代码 copy 自 Bottle 官网 demo

```python
from bottle import route, run, template

@route('/hello/<name>')
def index(name):
    return template('<b>Hello {{name}}</b>!', name=name)

run(host='localhost', port=8080)
```

追踪一下 route 是如何定义的

```python
route     = make_default_app_wrapper('route')
```

`make_default_app_wrapper` 是一个闭包， 可以根据 name 从 Bottle 实例中获取相应的方法，并传参调用

``` python
# Shortcuts for common Bottle methods.
# They all refer to the current default application.

def make_default_app_wrapper(name):
    ''' Return a callable that relays calls to the current default app. '''
    @functools.wraps(getattr(Bottle, name))
    def wrapper(*a, **ka):
        return getattr(app(), name)(*a, **ka)
    return wrapper
```

Bottle 类 route 方法

```python
def route(self, path=None, method='GET', callback=None, name=None,
          apply=None, skip=None, **config):
    """ A decorator to bind a function to a request URL. Example::

            @app.route('/hello/:name')
            def hello(name):
                return 'Hello %s' % name

        The ``:name`` part is a wildcard. See :class:`Router` for syntax
        details.

        :param path: Request path or a list of paths to listen to. If no
          path is specified, it is automatically generated from the
          signature of the function.
        :param method: HTTP method (`GET`, `POST`, `PUT`, ...) or a list of
          methods to listen to. (default: `GET`)
        :param callback: An optional shortcut to avoid the decorator
          syntax. ``route(..., callback=func)`` equals ``route(...)(func)``
        :param name: The name for this route. (default: None)
        :param apply: A decorator or plugin or a list of plugins. These are
          applied to the route callback in addition to installed plugins.
        :param skip: A list of plugins, plugin classes or names. Matching
          plugins are not installed to this route. ``True`` skips all.

        Any additional keyword arguments are stored as route-specific
        configuration and passed to plugins (see :meth:`Plugin.apply`).
    """
    if callable(path): path, callback = None, path
    plugins = makelist(apply) # 01
    skiplist = makelist(skip)
    # 此处的 callback 即为我们的 handler
    def decorator(callback):
        # TODO: Documentation and tests
        if isinstance(callback, basestring):
            # 如果为字符串则 load
            callback = load(callback)
        # 如果 path 为空时，会根据 handler 的名字和参数生成路由
        for rule in makelist(path) or yieldroutes(callback): # 02
            for verb in makelist(method):
                verb = verb.upper()
                # 创建 Route 实例，并添加至 router 中
                route = Route(self, rule, verb, callback, name=name,
                              plugins=plugins, skiplist=skiplist, **config)
                self.add_route(route)
        return callback
    # 如果我们没有传入 callback 时会返回 decorator 然后装饰我们的 handler
    # 所以有两种 view 写法
    # 1) 装饰器 @route('/hello/<name>')
    # 2) route('/hello/<name>', callback=func)
    return decorator(callback) if callback else decorator
```

01 处 makelist

```python
def makelist(data): # This is just to handy
    if isinstance(data, (tuple, list, set, dict)): return list(data)
    elif data: return [data]
    else: return []
```

02 处的 `yieldroutes` 设计的十分巧妙，使用了 `inspect` module

```python
def yieldroutes(func):
    """ Return a generator for routes that match the signature (name, args)
    of the func parameter. This may yield more than one route if the function
    takes optional keyword arguments. The output is best described by example::

        a()         -> '/a'
        b(x, y)     -> '/b/<x>/<y>'
        c(x, y=5)   -> '/c/<x>' and '/c/<x>/<y>'
        d(x=5, y=6) -> '/d' and '/d/<x>' and '/d/<x>/<y>'
    """
    path = '/' + func.__name__.replace('__','/').lstrip('/')
    spec = getargspec(func)
    # spec[0] 为所有函数参数的 list
    # spec[3] 为默认参数的 tuple
    # Example:
    # def func(a, b=1, c=2, *args, **kwargs):
    #     pass
    # In [15]: inspect.getargspec(func)
    # Out[15]: ArgSpec(args=['a', 'b', 'c'], varargs='args', keywords='kwargs', defaults=(1, 2))
    argc = len(spec[0]) - len(spec[3] or [])
    path += ('/<%s>' * argc) % tuple(spec[0][:argc])
    yield path
    for arg in spec[0][argc:]:
        path += '/<%s>' % arg
        yield path
```

而 `add_route` 方法做了两件事，将 `route` 对象添加至 `self.routes` 中，并且注册至 `self.router` 中

```python
# Bottle 类 add_route 方法

def add_route(self, route):
    ''' Add a route object, but do not change the :data:`Route.app`
        attribute.'''
    # self.routes = [] # List of installed :class:`Route` instances.
    # self.router = Router() # Maps requests to :class:`Route` instances.
    self.routes.append(route)
    self.router.add(route.rule, route.method, route, name=route.name)
    if DEBUG: route.prepare()
```

下面我们具体看看 Route 类

```python
class Route(object):
    ''' This class wraps a route callback along with route specific metadata and
        configuration and applies Plugins on demand. It is also responsible for
        turing an URL path rule into a regular expression usable by the Router.
    '''

    def __init__(self, app, rule, method, callback, name=None,
                 plugins=None, skiplist=None, **config):
        #: The application this route is installed to.
        self.app = app
        #: The path-rule string (e.g. ``/wiki/:page``).
        self.rule = rule
        #: The HTTP method as a string (e.g. ``GET``).
        self.method = method
        #: The original callback with no plugins applied. Useful for introspection.
        self.callback = callback
        #: The name of the route (if specified) or ``None``.
        self.name = name or None
        #: A list of route-specific plugins (see :meth:`Bottle.route`).
        self.plugins = plugins or []
        #: A list of plugins to not apply to this route (see :meth:`Bottle.route`).
        self.skiplist = skiplist or []
        #: Additional keyword arguments passed to the :meth:`Bottle.route`
        #: decorator are stored in this dictionary. Used for route-specific
        #: plugin configuration and meta-data.
        self.config = ConfigDict().load_dict(config, make_namespaces=True)

    def __call__(self, *a, **ka):
        depr("Some APIs changed to return Route() instances instead of"\
             " callables. Make sure to use the Route.call method and not to"\
             " call Route instances directly.") #0.12
        return self.call(*a, **ka)

    # Bottle 类中的 _handle 方法调用过
    # cached_property 修饰，实际上为 handler
    @cached_property
    def call(self):
        ''' The route callback with all plugins applied. This property is
            created on demand and then cached to speed up subsequent requests.'''
        return self._make_callback()

    def reset(self):
        ''' Forget any cached values. The next time :attr:`call` is accessed,
            all plugins are re-applied. '''
        self.__dict__.pop('call', None)

    def prepare(self):
        ''' Do all on-demand work immediately (useful for debugging).'''
        self.call

    @property
    def _context(self):
        depr('Switch to Plugin API v2 and access the Route object directly.')  #0.12
        return dict(rule=self.rule, method=self.method, callback=self.callback,
                    name=self.name, app=self.app, config=self.config,
                    apply=self.plugins, skip=self.skiplist)

    def all_plugins(self):
        ''' Yield all Plugins affecting this route. '''
        unique = set()
        for p in reversed(self.app.plugins + self.plugins):
            if True in self.skiplist: break
            name = getattr(p, 'name', False)
            if name and (name in self.skiplist or name in unique): continue
            if p in self.skiplist or type(p) in self.skiplist: continue
            if name: unique.add(name)
            yield p

    def _make_callback(self):
        callback = self.callback
        # 关于 plugin 参考　https://bottlepy.org/docs/0.12/tutorial.html#plugins
        for plugin in self.all_plugins():
            try:
                if hasattr(plugin, 'apply'):
                    api = getattr(plugin, 'api', 1)
                    context = self if api > 1 else self._context
                    callback = plugin.apply(callback, context)
                else:
                    callback = plugin(callback)
            except RouteReset: # Try again with changed configuration.
                return self._make_callback()
            if not callback is self.callback:
                update_wrapper(callback, self.callback)
        return callback

    # 这个函数可以获取被装饰器修饰的函数
    # In [14]: def outer(func):
    #     ...:     def inner(*args, **kwargs):
    #     ...:         func(*args, **kwargs)
    #     ...:     return inner
    #     ...:
    #
    # In [15]: @outer
    #     ...: def multi(a, b):
    #     ...:     return a * b
    #     ...:
    #
    # In [16]: multi.__closure__[0].cell_contents
    # Out[16]: <function __main__.multi>
    def get_undecorated_callback(self):
        ''' Return the callback. If the callback is a decorated function, try to
            recover the original function. '''
        func = self.callback
        func = getattr(func, '__func__' if py3k else 'im_func', func)
        closure_attr = '__closure__' if py3k else 'func_closure'
        while hasattr(func, closure_attr) and getattr(func, closure_attr):
            func = getattr(func, closure_attr)[0].cell_contents
        return func

    def get_callback_args(self):
        ''' Return a list of argument names the callback (most likely) accepts
            as keyword arguments. If the callback is a decorated function, try
            to recover the original function before inspection. '''
        return getargspec(self.get_undecorated_callback())[0]

    def get_config(self, key, default=None):
        ''' Lookup a config field and return its value, first checking the
            route.config, then route.app.config.'''
        for conf in (self.config, self.app.conifg):
            if key in conf: return conf[key]
        return default

    def __repr__(self):
        cb = self.get_undecorated_callback()
        return '<%s %r %r>' % (self.method, self.rule, cb)
```

Router 类比较头疼，负责进行路由匹配，所以包含了许多正则表达式

```python
class Router(object):
    ''' A Router is an ordered collection of route->target pairs. It is used to
        efficiently match WSGI requests against a number of routes and return
        the first target that satisfies the request. The target may be anything,
        usually a string, ID or callable object. A route consists of a path-rule
        and a HTTP method.

        The path-rule is either a static path (e.g. `/contact`) or a dynamic
        path that contains wildcards (e.g. `/wiki/<page>`). The wildcard syntax
        and details on the matching order are described in docs:`routing`.
    '''

    default_pattern = '[^/]+'
    default_filter  = 're'

    #: The current CPython regexp implementation does not allow more
    #: than 99 matching groups per regular expression.
    _MAX_GROUPS_PER_PATTERN = 99

    def __init__(self, strict=False):
        self.rules    = [] # All rules in order
        self._groups  = {} # index of regexes to find them in dyna_routes
        self.builder  = {} # Data structure for the url builder
        self.static   = {} # Search structure for static routes
        self.dyna_routes   = {}
        self.dyna_regexes  = {} # Search structure for dynamic routes
        #: If true, static routes are no longer checked first.
        self.strict_order = strict
        self.filters = {
            're':    lambda conf:
                (_re_flatten(conf or self.default_pattern), None, None),
            'int':   lambda conf: (r'-?\d+', int, lambda x: str(int(x))),
            'float': lambda conf: (r'-?[\d.]+', float, lambda x: str(float(x))),
            'path':  lambda conf: (r'.+?', None, None)}

    def add_filter(self, name, func):
        ''' Add a filter. The provided function is called with the configuration
        string as parameter and must return a (regexp, to_python, to_url) tuple.
        The first element is a string, the last two are callables or None. '''
        self.filters[name] = func

    # 可以试试 re.DEBUG 帮助理解
    rule_syntax = re.compile('(\\\\*)'\
        '(?:(?::([a-zA-Z_][a-zA-Z_0-9]*)?()(?:#(.*?)#)?)'\
          '|(?:<([a-zA-Z_][a-zA-Z_0-9]*)?(?::([a-zA-Z_]*)'\
            '(?::((?:\\\\.|[^\\\\>]+)+)?)?)?>))')

    # /article/<num:int> 为例
    def _itertokens(self, rule):
        offset, prefix = 0, ''
        # 正则匹配 <name> 部分
        for match in self.rule_syntax.finditer(rule):
            # prefix 为 rule 中的静态部分: /article/
            prefix += rule[offset:match.start()]
            g = match.groups() # ('', None, None, None, 'num', 'int', None)
            if len(g[0])%2: # Escaped wildcard
                prefix += match.group(0)[len(g[0]):]
                offset = match.end()
                continue
            if prefix:
                yield prefix, None, None
            name, filtr, conf = g[4:7] if g[2] is None else g[1:4]
            # 如果没有 filter 则为 default !!!
            yield name, filtr or 'default', conf or None
            offset, prefix = match.end(), ''
        # 剩下的静态部分
        if offset <= len(rule) or prefix:
            yield prefix+rule[offset:], None, None

    # /article/<num:int> 为例
    def add(self, rule, method, target, name=None):
        ''' Add a new rule or replace the target for an existing rule. '''
        anons     = 0    # Number of anonymous wildcards found
        keys      = []   # Names of keys
        pattern   = ''   # Regular expression pattern with named groups
        filters   = []   # Lists of wildcard input filters
        builder   = []   # Data structure for the URL builder
        is_static = True

        # 构成正则表达式，用来匹配路由
        for key, mode, conf in self._itertokens(rule):
            # mode 为 filter 的 name
            # 如果存在动态路由部分(<name>, <name:filter>)均会进入此分支
            if mode:
                is_static = False
                if mode == 'default': mode = self.default_filter
                # {'int':   lambda conf: (r'-?\d+', int, lambda x: str(int(x)))}
                mask, in_filter, out_filter = self.filters[mode](conf)
                if not key:
                    pattern += '(?:%s)' % mask
                    key = 'anon%d' % anons
                    anons += 1
                else:
                    # 命名捕获组
                    pattern += '(?P<%s>%s)' % (key, mask)
                    keys.append(key)
                if in_filter: filters.append((key, in_filter))
                builder.append((key, out_filter or str))
            # 静态路由进入此分支
            elif key:
                pattern += re.escape(key)
                builder.append((None, key))

        # pattern:  \/article\/(?P<num>-?\d+)
        # builder:  [(None, '/article/'), ('num', <function <lambda> at 0x7fa3f625c320>)]
        self.builder[rule] = builder
        if name: self.builder[name] = builder

        # 如果路由全部为静态
        if is_static and not self.strict_order:
            self.static.setdefault(method, {})
            self.static[method][self.build(rule)] = (target, None)
            return

        # 当存在动态路由部分才会执行下面语句
        try:
            # 构成捕获组
            re_pattern = re.compile('^(%s)$' % pattern)
            re_match = re_pattern.match
        except re.error:
            raise RouteSyntaxError("Could not add Route: %s (%s)" % (rule, _e()))

        if filters:
            # getargs 是用来提取 path 中的参数的，根据路由定义有三种不同的形式
            def getargs(path):
                url_args = re_match(path).groupdict()
                for name, wildcard_filter in filters:
                    try:
                        url_args[name] = wildcard_filter(url_args[name])
                    except ValueError:
                        raise HTTPError(400, 'Path has wrong format.')
                return url_args
        elif re_pattern.groupindex:
            def getargs(path):
                return re_match(path).groupdict()
        else:
            getargs = None

        # 将带有捕获组的正则表达式转换为非捕获组正则表达式
        # pattern: \/article\/(?P<num>-?\d+)
        # flatpat: \/article\/(?:-?\d+)
        # 这里为什么要转换正则表达式呢，个人感到很奇怪
        flatpat = _re_flatten(pattern)
        whole_rule = (rule, flatpat, target, getargs)

        if (flatpat, method) in self._groups:
            if DEBUG:
                msg = 'Route <%s %s> overwrites a previously defined route'
                warnings.warn(msg % (method, rule), RuntimeWarning)
            self.dyna_routes[method][self._groups[flatpat, method]] = whole_rule
        else:
            self.dyna_routes.setdefault(method, []).append(whole_rule)
            self._groups[flatpat, method] = len(self.dyna_routes[method]) - 1

        self._compile(method)

    def _compile(self, method):
        all_rules = self.dyna_routes[method]
        # list 可变类型
        comborules = self.dyna_regexes[method] = []
        maxgroups = self._MAX_GROUPS_PER_PATTERN
        for x in range(0, len(all_rules), maxgroups):
            some = all_rules[x:x+maxgroups]
            combined = (flatpat for (_, flatpat, _, _) in some)
            combined = '|'.join('(^%s$)' % flatpat for flatpat in combined)
            combined = re.compile(combined).match
            rules = [(target, getargs) for (_, _, target, getargs) in some]
            comborules.append((combined, rules))


    def build(self, _name, *anons, **query):
        ''' Build an URL by filling the wildcards in a rule. '''
        builder = self.builder.get(_name)
        if not builder: raise RouteBuildError("No route with that name.", _name)
        try:
            for i, value in enumerate(anons): query['anon%d'%i] = value
            url = ''.join([f(query.pop(n)) if n else f for (n,f) in builder])
            return url if not query else url+'?'+urlencode(query)
        except KeyError:
            raise RouteBuildError('Missing URL argument: %r' % _e().args[0])

    # 进行路由匹配
    def match(self, environ):
        ''' Return a (target, url_agrs) tuple or raise HTTPError(400/404/405). '''
        verb = environ['REQUEST_METHOD'].upper()
        path = environ['PATH_INFO'] or '/'
        target = None
        if verb == 'HEAD':
            methods = ['PROXY', verb, 'GET', 'ANY']
        else:
            methods = ['PROXY', verb, 'ANY']

        for method in methods:
            # 静态路由分支
            if method in self.static and path in self.static[method]:
                target, getargs = self.static[method][path]
                return target, getargs(path) if getargs else {}
            # 动态路由分支
            elif method in self.dyna_regexes:
                for combined, rules in self.dyna_regexes[method]:
                    match = combined(path)
                    if match:
                        target, getargs = rules[match.lastindex - 1]
                        return target, getargs(path) if getargs else {}

        # No matching route found. Collect alternative methods for 405 response
        allowed = set([])
        nocheck = set(methods)
        for method in set(self.static) - nocheck:
            if path in self.static[method]:
                allowed.add(verb)
        for method in set(self.dyna_regexes) - allowed - nocheck:
            for combined, rules in self.dyna_regexes[method]:
                match = combined(path)
                if match:
                    allowed.add(method)
        if allowed:
            allow_header = ",".join(sorted(allowed))
            raise HTTPError(405, "Method not allowed.", Allow=allow_header)

        # No matching route and no alternative method found. We give up
        raise HTTPError(404, "Not found: " + repr(path))
```

    