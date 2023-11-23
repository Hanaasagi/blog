
+++
title = "WSGI OVERVIEW"
summary = ''
description = ""
categories = []
tags = []
date = 2017-06-28T04:12:26+08:00
draft = false
+++

PEP 333 是 Python 的 WSGI 规范，另外还有一篇 PEP 3333 是针对 Py3 进行修改的更新版。WSGI 规定了 Web server 和 Python Web 应用或框架之间的标准接口，提高了 Web 应用在各种 Web server 间的可移植性

这里的 server 是指 application server。一般 web 架构大概是下面这样的

```
----------------------   /path   --------------   /path
|                    |   <====   |            |   <=====
| application server |           |            |   =====>       ------------
|                    |   ====>   |   (nginx)  |    html        |          |
----------------------    html   | web server |   /style.css   |  Broswer |
                                 |            |   <=====       |          |
                                 |            |   =====>       ------------
                                 |            |   style.css
                                 --------------
```

Python 目前拥有各种各样的 Web 框架，例如 Zope，Quixote，Webware，SkunkWeb，PSO和 Twisted Web 等(蠢作者前面几种全没听过)。这种广泛的选择对于新的 Python 用户来说可能是一个问题，因为一般来说，他们选择的 Web 框架将限制他们可选择的 Web server，反之亦然。相比之下，尽管 Java 具有许多 Web 框架，但是 Java 的 servlet API 可以使任何使用 Java Web 应用框架写的程序运行在任何支持 servlet API 的 Web server 上

对于 Python 的 Web server 来说，无论是使用 Python 所写的 servers (e.g.Medusa)，还是嵌入了 Python (e.g.mod_python)，或者是通过网关协议(e.g.CGI, FastCGI,etc.)来调用 Python。这种 API 能够将框架的选择和 server 的选择独立，用户可以自由地进行配对，同时框架和 server 开发人员能够专注于他们的专业领域。WSGI 即是这样的一个 API。当然仅仅一个 WSGI 规范并不能解决 Python Web 框架和 server 的现状。server 和框架的作者和维护者必须实际实现 WSGI 才能有效果。所以它必须足够简单来让开发者愿意去实现。

*蠢作者注： mod_python 是一个 Apache HTTP Server 的 module，集成了 Python 语言。旨在为 Apache HTTP Server 提供 Python 语言绑定。 详见 [wiki](https://en.wikipedia.org/wiki/Mod_python)*

除了能易于实现框架和 server 之外，还可以轻松地创建请求预处理程序(request preproessors)，响应后处理程序(response postprocessors)和其他基于 WSGI 的中间件(middleware)，这些组件对于包含着它们的 server 来说看起来像是一个应用；对于它们所包含的应用来说，像是一个 server(中间件即实现了 server 的规范又实现了 application 的规范，能够透明的插入至原来的 server 和 application 之间)

WSGI 接口规定了两方面的内容
1) server or gateway
2) application or framework

application 必须是一个 callable ，它接受两个参数。为了方便之后的说明，分别起名为 `environ` 和 `start_response`。server 或 gateway 每当从 HTTP 客户端接收到一个请求便会调用 application 一次。就像这样 `result = application(environ, start_response)`。

`environ` 参数是一个字典对象(dictionary object)，包含了 CGI 风格的环境变量。这个对象必须是 built-in 的 dict。不能是其子类，或者 `UserDict` 或者其他的。application 可以以任何想要的方式修改这个字典。`environ` 还应该包含一些特定的 WSGI 所需变量，也可以包含一些 server 特定的扩展变量。详见 [environ Variables](https://www.python.org/dev/peps/pep-3333/#id24)

`start_response` 必须是 callable，它接受两个位置参数(positional argument)和一个可选参数(optional argument)，假设它们分别为 `status`, `response_headers` 和 `exc_info`。而且 application 必须调用 `start_response`。`status` 参数是一个形如 "999 Message here" 的字符串(两头不允许包含空格，末尾也没有换行符)，`response_headers` 是一个 `(header_name, header_value)` 的 list (每一个 `header_name` 必须是合法的 HTTP header 字段名，`header_value` 不能包含任何控制字符，如回车或换行符等，中间嵌入或者在末尾都不行。如果 application 没提供必要的头信息，则 server/gateway 应当补上)。可选的 `exc_info` 参数只有在应用程序产生了错误并希望在浏览器上显示相关信息的时候才会用，其应当为 `sys.exc_info()` 那样的元组。`start_response` 必须返回一个 callable，暂称之为 `write`。它接受一个可选参数：作为 HTTP response body 的字节串

将 `environ` 和 `start_response` 传入是实现了依赖注入。application 可以自行决定如何处理 `environ`，何时调用 `start_response`，这也是控制反转的体现

除此之外，application 必须返回一个能够生成 0 个或多个字节的 iterable，作为 response body。这可以通过多种方法实现，比如返回一个字节字符串的列表，或 application 本身是一个生成字节字符串的生成器函数(返回生成器)，或者 application 是一个类且其实例是可迭代的。不管如何实现，application 对象必须返回一个 iterable。server/gateway 必须将生成的字节流以一种无缓冲的方式传送到客户端。换句话说，application 应当自己实现缓冲。除此之外  server/gateway 应该把产生的字节字符串当作字节流对待。application 负责确保字节字符串符合格式，server/gateway 可以对内容进行 HTTP 传输编码(e.g.gzip)，或者执行其他转换，以实现 HTTP features，如字节范围传输(byte-range transmission)

好了，这里可能会有一些疑问，既然是由 application 调用的 `start_response`，并且其返回 `write`。那么为什么不由 application 去调用 `write` 将 response body 写入呢？

蠢作者的答案是：如果这样实现，那么 application 必须承担起许多本来应有 server/gateway 完成的工作，比如 HTTP 传输编码、客户端突然断开连接的异常处理等。这样的分工有点不合理

官方是这样解释的

>New WSGI applications and frameworks should not use the write() callable if it is possible to avoid doing so. The write() callable is strictly a hack to support imperative streaming APIs. In general, applications should produce their output via their returned iterable, as this makes it possible for web servers to interleave other tasks in the same Python thread, potentially providing better throughput for the server as a whole.

那么为什么 `start_response` 还返回了 `write` 呢？

>Why can you call write() and yield bytestrings/return an iterable? Shouldn't we pick just one way?

>If we supported only the iteration approach, then current frameworks that assume the availability of "push" suffer. But, if we only support pushing via write() , then server performance suffers for transmission of e.g. large files (if a worker thread can't begin work on a new request until all of the output has been sent). Thus, this compromise allows an application framework to support both approaches, as appropriate, but with only a little more burden to the server implementor than a push-only approach would require.


下面来看看 Bottle 框架是怎么做的

```Python
# bottle.py
class Bottle(object):

    def wsgi(self, environ, start_response):
        """ The bottle WSGI-interface. """
        try:
            # out 是一个 list 类型
            out = self._cast(self._handle(environ)) # 1
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
            # ...

    def __call__(self, environ, start_response):
        ''' Each instance of :class:'Bottle' is a WSGI application. '''
        return self.wsgi(environ, start_response)
```


```Python
# /opt/python3.5/lib/python3.5/WSGIref/handler.py
class BaseHandler:
    # ...
    def run(self, application):
        """Invoke the application"""
        # Note to self: don't move the close()!  Asynchronous servers shouldn't
        # call close() from finish_response(), so if you close() anywhere but
        # the double-error branch here, you'll break asynchronous servers by
        # prematurely closing.  Async servers must return from 'run()' without
        # closing if there might still be output to iterate over.
        try:
            self.setup_environ()
            self.result = application(self.environ, self.start_response)
            self.finish_response()
        except:
            try:
                self.handle_error()
            except:
                # If we get an error handling an error, just give up already!
                self.close()
                raise   # ...and let the actual server figure it out.

    # ...

    def finish_response(self):
        """Send any iterable data, then close self and the iterable

        Subclasses intended for use in asynchronous servers will
        want to redefine this method, such that it sets up callbacks
        in the event loop to iterate over the data, and to call
        'self.close()' once the response is finished.
        """
        try:
            if not self.result_is_file() or not self.sendfile():
                for data in self.result:
                    self.write(data)
                self.finish_content()
        finally:
            self.close()

    # ...

    def start_response(self, status, headers,exc_info=None):
        """'start_response()' callable as specified by PEP 3333"""

        if exc_info:
            try:
                if self.headers_sent:
                    # Re-raise original exception if headers sent
                    raise exc_info[0](exc_info[1]).with_traceback(exc_info[2])
            finally:
                exc_info = None        # avoid dangling circular ref
        elif self.headers is not None:
            raise AssertionError("Headers already set!")

        self.status = status
        self.headers = self.headers_class(headers)
        status = self._convert_string_type(status, "Status")
        assert len(status)>=4,"Status must be at least 4 characters"
        assert status[:3].isdigit(), "Status message must begin w/3-digit code"
        assert status[3]==" ", "Status message must have a space after code"

        if __debug__:
            for name, val in headers:
                name = self._convert_string_type(name, "Header name")
                val = self._convert_string_type(val, "Header value")
                assert not is_hop_by_hop(name),"Hop-by-hop headers not allowed"

        return self.write

    # ...

    def write(self, data):
        """'write()' callable as specified by PEP 3333"""

        assert type(data) is bytes, \
            "write() argument must be a bytes instance"

        if not self.status:
            raise AssertionError("write() before start_response()")

        elif not self.headers_sent:
            # Before the first output, send the stored headers
            self.bytes_sent = len(data)    # make sure we know content-length
            self.send_headers()
        else:
            self.bytes_sent += len(data)

        # XXX check Content-Length and truncate if too many bytes written?
        self._write(data)
        self._flush()
```

通常而言，中间件对于 server/gateway 和 application/framework 两方都是透明的，并且不需要特别的支持。希望在 application 上添加中间件的用户仅需把中间件当作 application 提供给 server，并配置中间件调用 application 即可(像一个 server 一般)。当然，中间件也可以串联，构成所谓的 "middleware stack"，增强了扩展性

在大多数情况下，中间件必须同时符合 WSGI 的 server 和 application 的要求。有时候甚至比纯粹的 server 或 application 要求还要严格。

下面是一个简单的中间件的示例

```Python
from piglatin import piglatin

class LatinIter:

    """Transform iterated output to piglatin, if it's okay to do so

    Note that the "okayness" can change until the application yields
    its first non-empty bytestring, so 'transform_ok' has to be a mutable
    truth value.
    """

    def __init__(self, result, transform_ok):
        if hasattr(result, 'close'):
            self.close = result.close
        self._next = iter(result).__next__
        self.transform_ok = transform_ok

    def __iter__(self):
        return self

    def __next__(self):
        if self.transform_ok:
            return piglatin(self._next())   # call must be byte-safe on Py3
        else:
            return self._next()

class Latinator:

    # by default, don't transform output
    transform = False

    def __init__(self, application):
        self.application = application

    def __call__(self, environ, start_response):

        transform_ok = []

        def start_latin(status, response_headers, exc_info=None):

            # Reset ok flag, in case this is a repeat call
            del transform_ok[:]

            for name, value in response_headers:
                if name.lower() == 'content-type' and value == 'text/plain':
                    transform_ok.append(True)
                    # Strip content-length if present, else it'll be wrong
                    response_headers = [(name, value)
                        for name, value in response_headers
                            if name.lower() != 'content-length'
                    ]
                    break

            write = start_response(status, response_headers, exc_info)

            if transform_ok:
                def write_latin(data):
                    write(piglatin(data))   # call must be byte-safe on Py3
                return write_latin
            else:
                return write

        return LatinIter(self.application(environ, start_latin), transform_ok)


# Run foo_app under a Latinator's control, using the example CGI gateway
from foo_app import foo_app
run_with_cgi(Latinator(foo_app))
```
这个如果使用装饰器来做，效果更好

另外此 PEP 还规定了诸多细节，比如缓冲和流，Unicode问题，错误处理等。请参考 [这篇翻译](https://github.com/mainframer/PEP333-zh-CN)，不过是 PEP 333 的

    