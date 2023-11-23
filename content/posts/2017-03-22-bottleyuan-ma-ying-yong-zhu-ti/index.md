
+++
title = "Bottle源码-应用主体"
summary = ''
description = ""
categories = []
tags = []
date = 2017-03-22T05:26:51+08:00
draft = false
+++

本文基于 [bottle-0.12.13](https://pypi.python.org/pypi/bottle/0.12.13)  
承接上篇，当有请求来临时， WSGIServer 内部做了如下的事情

```
# wsgiref/handlers.py
self.result = application(self.environ, self.start_response)
```

此处的 application 即为我们调用 `server.run(app)` 时传入的 Bottle类 的实例

而这个 environ 则是从 os.environ update 过来的，添加了这次请求的信息
添加的请求信息如下

```
{'CONTENT_LENGTH': '0',
 'CONTENT_TYPE': 'text/plain',
 'GATEWAY_INTERFACE': 'CGI/1.1',
 'HTTP_ACCEPT': 'text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8',
 'HTTP_ACCEPT_ENCODING': 'gzip, deflate',
 'HTTP_ACCEPT_LANGUAGE': 'en-US,en;q=0.5',
 'HTTP_CONNECTION': 'keep-alive',
 'HTTP_HOST': 'localhost:8080',
 'HTTP_USER_AGENT': 'Mozilla/5.0 (X11; Linux x86_64; rv:45.0) Gecko/20100101 '
                    'Firefox/45.0',
 'PATH_INFO': '/hello/name',
 'QUERY_STRING': 'arg=1',
 'REMOTE_ADDR': '127.0.0.1',
 'REMOTE_HOST': '',
 'REQUEST_METHOD': 'GET',
 'SCRIPT_NAME': '',
 'SERVER_NAME': 'localhost',
 'SERVER_PORT': '8080',
 'SERVER_PROTOCOL': 'HTTP/1.1',
 'SERVER_SOFTWARE': 'WSGIServer/0.2',
 'wsgi.errors': <_io.TextIOWrapper name='<stderr>' mode='w' encoding='UTF-8'>,
 'wsgi.file_wrapper': <class 'wsgiref.util.FileWrapper'>,
 'wsgi.input': <_io.BufferedReader name=5>,
 'wsgi.multiprocess': False,
 'wsgi.multithread': True,
 'wsgi.run_once': False,
 'wsgi.url_scheme': 'http',
 'wsgi.version': (1, 0)}
```

Bottle 类的 `__call__` 方法如下

```python
def __call__(self, environ, start_response):
   ''' Each instance of :class:'Bottle' is a WSGI application. '''
   return self.wsgi(environ, start_response)
```

再来看看 Bottle 类的 wsgi 接口

```python
def wsgi(self, environ, start_response):
    """ The bottle WSGI-interface. """
    try:
        # out 是一个 list 类型
        out = self._cast(self._handle(environ)) # 01
        # rfc2616 section 4.3
        if response._status_code in (100, 101, 204, 304)\
        or environ['REQUEST_METHOD'] == 'HEAD':
            if hasattr(out, 'close'): out.close()
            out = []
        # 传入构建好的 response header
        start_response(response._status_line, response.headerlist)
        return out
    except (KeyboardInterrupt, SystemExit, MemoryError):
        raise
    except Exception:
        if not self.catchall: raise
        err = '<h1>Critical error while processing request: %s</h1>' \
              % html_escape(environ.get('PATH_INFO', '/'))
        if DEBUG:
            err += '<h2>Error:</h2>\n&lt;pre&gt;\n%s\n&lt;/pre&gt;\n' \
                   '<h2>Traceback:</h2>\n&lt;pre&gt;\n%s\n&lt;/pre&gt;\n' \
                   % (html_escape(repr(_e())), html_escape(format_exc()))
        environ['wsgi.errors'].write(err)
        headers = [('Content-Type', 'text/html; charset=UTF-8')]
        start_response('500 INTERNAL SERVER ERROR', headers, sys.exc_info())
        return [tob(err)]
```

01) 处调用了 Bottle 类的 _handle 方法

```python
def _handle(self, environ):
    # 'PATH_INFO': '/hello/name' 请求路径
    path = environ['bottle.raw_path'] = environ['PATH_INFO']
    if py3k:
        try:
            # encode('latin1') 对于 \uFF 以上的字符会抛出 UnicodeEncodeError
            # decode('utf8') 会对于不符合 utf-8 的字节抛出 UnicodeDecodeError
            environ['PATH_INFO'] = path.encode('latin1').decode('utf8')
        except UnicodeError:
            return HTTPError(400, 'Invalid path string. Expected UTF-8')

    try:
        environ['bottle.app'] = self
        request.bind(environ) # 001
        response.bind() # 002
        try:
            # 触发 before_request 对应的 hooks
            # hooks 保存在 self._hooks 字典中
            self.trigger_hook('before_request')
            # self.router 为 Router 类的实例，在 __init__ 中声明
            # 这里做了匹配路由并返回了含有 handler 的 Route 类实例 和 对应的参数
            # Example:
            # @route('/hello/<name>')
            # def index(name):
            #     pass
            # :route <class 'bottle.Route'>
            # :arg   {'name': 'weiss'}
            route, args = self.router.match(environ)
            environ['route.handle'] = route
            environ['bottle.route'] = route
            environ['route.url_args'] = args
            # 详见 Route 类源码
            # 返回了渲染好的HTML内容
            return route.call(**args)
        finally:
            # 触发 after_request 中的 hooks
            self.trigger_hook('after_request')

    except HTTPResponse:
        return _e()
    except RouteReset:
        route.reset()
        return self._handle(environ)
    except (KeyboardInterrupt, SystemExit, MemoryError):
        raise
    except Exception:
        if not self.catchall: raise
        stacktrace = format_exc()
        environ['wsgi.errors'].write(stacktrace)
        return HTTPError(500, "Internal Server Error", _e(), stacktrace) # 002
```

001) request 和 response 的定义

```python
#: A thread-safe instance of :class:`LocalRequest`. If accessed from within a
#: request callback, this instance always refers to the *current* request
#: (even on a multithreaded server).
request = LocalRequest()

#: A thread-safe instance of :class:`LocalResponse`. It is used to change the
#: HTTP response for the *current* request.
response = LocalResponse()
```

LocalRequest 和 LocalResponse 分别是 BaseRequest 和 BaseResponse 的子类

```python
class LocalRequest(BaseRequest):
    ''' A thread-local subclass of :class:`BaseRequest` with a different
        set of attributes for each thread. There is usually only one global
        instance of this class (:data:`request`). If accessed during a
        request/response cycle, this instance always refers to the *current*
        request (even on a multithreaded server). '''
    bind = BaseRequest.__init__
    environ = local_property()
```

`BaseRequest.__init__` 定义如下

```python
def __init__(self, environ=None):
    """ Wrap a WSGI environ dictionary. """
    #: The wrapped WSGI environ dictionary. This is the only real attribute.
    #: All other attributes actually are read-only properties.
    self.environ = {} if environ is None else environ
    self.environ['bottle.request'] = self


class LocalResponse(BaseResponse):
    ''' A thread-local subclass of :class:`BaseResponse` with a different
        set of attributes for each thread. There is usually only one global
        instance of this class (:data:`response`). Its attributes are used
        to build the HTTP response at the end of the request/response cycle.
    '''
    bind = BaseResponse.__init__
    _status_line = local_property()
    _status_code = local_property()
    _cookies     = local_property()
    _headers     = local_property()
    body         = local_property()
```

`BaseResponse.__init__` 的定义

```python
def __init__(self, body='', status=None, headers=None, **more_headers):
    self._cookies = None
    self._headers = {}
    self.body = body
    self.status = status or self.default_status
    if headers:
        if isinstance(headers, dict):
            headers = headers.items()
        for name, value in headers:
            self.add_header(name, value)
    if more_headers:
        for name, value in more_headers.items():
            self.add_header(name, value)
```

而这个 local_property 则是这样实现的，使用了 `threading.local()`

```python
def local_property(name=None):
    if name: depr('local_property() is deprecated and will be removed.') #0.12
    ls = threading.local()
    def fget(self):
        try: return ls.var
        except AttributeError:
            raise RuntimeError("Request context not initialized.")
    def fset(self, value): ls.var = value
    def fdel(self): del ls.var
    return property(fget, fset, fdel, 'Thread-local property')
```

可以看出来  `request.bind(environ)` 与 `response.bind()` 实际上调用了 `__init__` 实现了重新的初始化，复用了这两个对象

002) HTTPError 继承自 HTTPREsponse

```python
# Response = BaseResponse
class HTTPResponse(Response, BottleException):
    def __init__(self, body='', status=None, headers=None, **more_headers):
        super(HTTPResponse, self).__init__(body, status, headers, **more_headers)

    def apply(self, response):
        response._status_code = self._status_code
        response._status_line = self._status_line
        response._headers = self._headers
        response._cookies = self._cookies
        response.body = self.body


class HTTPError(HTTPResponse):
    default_status = 500
    def __init__(self, status=None, body=None, exception=None, traceback=None,
                 **options):
        self.exception = exception
        self.traceback = traceback
        super(HTTPError, self).__init__(body, status, **options)
```

继续 01 处 `_cast` 的定义， `_cast` 负责转换成符合 WSGI 规范的响应

```python
def _cast(self, out, peek=None):
    """ Try to convert the parameter into something WSGI compatible and set
    correct HTTP headers when possible.
    Support: False, str, unicode, dict, HTTPResponse, HTTPError, file-like,
    iterable of strings and iterable of unicodes
    """
    # Empty output is done here
    if not out:
        if 'Content-Length' not in response:
            response['Content-Length'] = 0
        return []
    # Join lists of byte or unicode strings. Mixed lists are NOT supported
    if isinstance(out, (tuple, list))\
    and isinstance(out[0], (bytes, unicode)):
        out = out[0][0:0].join(out) # b'abc'[0:0] -> b''
    # Encode unicode strings
    if isinstance(out, unicode):
        # encode 得到 bytes 对象
        out = out.encode(response.charset)
    # Byte Strings are just returned
    if isinstance(out, bytes):
        if 'Content-Length' not in response:
            response['Content-Length'] = len(out)
        return [out]
    # HTTPError or HTTPException (recursive, because they may wrap anything)
    # TODO: Handle these explicitly in handle() or make them iterable.
    if isinstance(out, HTTPError):
        out.apply(response) # 3
        # __init__ 方法中定义的一个 dict
        # 根据状态码调用对应的 error_handler
        out = self.error_handler.get(out.status_code, self.default_error_handler)(out)
        return self._cast(out)
    if isinstance(out, HTTPResponse):
        out.apply(response)
        return self._cast(out.body)

    # File-like objects.
    if hasattr(out, 'read'):
        if 'wsgi.file_wrapper' in request.environ:
            return request.environ['wsgi.file_wrapper'](out)
        elif hasattr(out, 'close') or not hasattr(out, '__iter__'):
            return WSGIFileWrapper(out)

    # Handle Iterables. We peek into them to detect their inner type.
    try:
        iout = iter(out)
        first = next(iout)
        while not first:
            first = next(iout)
    except StopIteration:
        return self._cast('')
    except HTTPResponse:
        first = _e()
    except (KeyboardInterrupt, SystemExit, MemoryError):
        raise
    except Exception:
        if not self.catchall: raise
        first = HTTPError(500, 'Unhandled exception', _e(), format_exc())

    # These are the inner types allowed in iterator or generator objects.
    if isinstance(first, HTTPResponse):
        return self._cast(first)
    elif isinstance(first, bytes):
        new_iter = itertools.chain([first], iout)
    elif isinstance(first, unicode):
        encoder = lambda x: x.encode(response.charset)
        new_iter = imap(encoder, itertools.chain([first], iout))
    else:
        msg = 'Unsupported response type: %s' % type(first)
        return self._cast(HTTPError(500, msg))
    if hasattr(out, 'close'):
        new_iter = _closeiter(new_iter, out.close)
    return new_iter
```

大致流程如下

```
      request
        ||
        \/
-------------------
|                 |
|   WSGI Server   |
|                 |
-------------------
        ||  调用 Bottle 实例的 __call__ 方法
        \/
-------------------
|                 |
| before_request  |   左面的操作会对 response 对象进行修改，封装好响应头
| Router match    |        ---------------
| custom handler  |   ==>  | WSGI Server |  => response
| after_request   |        ---------------
| start_response  |
|                 |
-------------------
```

    