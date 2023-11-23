
+++
title = "何时进行 Urldecode"
summary = ''
description = ""
categories = []
tags = []
date = 2018-09-17T15:53:37+08:00
draft = false
+++

本文关注一个非常简单的问题: 何时进行 urldecode(unquote)。所涉及的代码分别为:

- Flask 0.12.2, commit sha `571334df8e26333f34873a3dcb84441946e6c64c`
- Werkzeug 0.14, commit sha `5b53d1539147c5db3210e0769d85397ab91f902d`
- Gunicorn 19.7.1, commit sha `328e509260ae70de6c04c5ba885ee17960b3ced5`

以 Flask + Gunicorn 为例(本人 Gunicorn 比 Werkzeug 熟一些)

```Python
app = Flask(__name__)


@app.route('/<name>/')
def demo(name):
    from_ = request.args.get('from', '')
    return f"I'm {name} from {from_}."


if __name__ == '__main__':
    app.run()
```

```Bash
$ curl "127.0.0.1:8000/%e8%b7%af%e6%98%8e%e9%9d%9e/?from=%e5%8d%a1%e5%a1%9e%e5%b0%94%e5%ad%a6%e9%99%a2"
I'm 路明非 from 卡塞尔学院.
```

我们可以看到在 Flask 中，得到的已经是解码之后的字符串了，那么我们便来分析一下解码发生的位置。首先需要说明的是 `PATH_INFO` 和 `QUERY_STRING` 是在不同的位置进行的解码

#### `PATH_INFO`

[gunicorn/workers/sync.py#L176](https://github.com/bakalab/gunicorn/blob/ver19.7.1/gunicorn/workers/sync.py#L176)

```Python
# 简化后代码
def handle_request(self, listener, req, client, addr):
    resp, environ = wsgi.create(req, client, addr, listener.getsockname(), self.cfg)
    respiter = self.wsgi(environ, resp.start_response)
    for item in respiter:
        resp.write(item)
    resp.close()
```

`environ` 是通过 `wsgi.create` 所解析出来，[gunicorn/http/wsgi.py#L116](https://github.com/bakalab/gunicorn/blob/ver19.7.1/gunicorn/http/wsgi.py#L202)


```Python
def create(req, sock, client, server, cfg):
    # ...
    environ['PATH_INFO'] = unquote_to_wsgi_str(path_info)
    # ...
```

这里对 `PATH_INFO` 进行了解码，但是在 PEP-333 中并未看到需要对此字段进行解码的要求 [environ Variables](https://www.python.org/dev/peps/pep-0333/#environ-variables)。并且翻了 werkzeug 的代码也是对 `PATH_INFO` 进行了解码，可以参考[这里](https://github.com/bakalab/werkzeug/blob/v0.14/werkzeug/serving.py#L169)

关于需不需要进行解码，`Gunicorn` 上也有相关[讨论](https://github.com/benoitc/gunicorn/pull/1211)

#### `QUERY_STRING`

再来看 `QUERY_STRING` 的解码，Flask 的 `Request` 是继承的 `werkzeug.wrappers.BaseRequest`。稍微来回顾一下这个流程


```
# flask/app.py
from .wrappers import Request

class Flask(_PackageBoundObject):
    request_class = Request

    def request_context(self, environ):
        return RequestContext(self, environ)

    def wsgi_app(self, environ, start_response):
        ctx = self.request_context(environ)
        ctx.push()
        # ...

    def __call__(self, environ, start_response):
        """Shortcut for :attr:`wsgi_app`."""
        return self.wsgi_app(environ, start_response)

# flask/ctx.py
class RequestContext(object):

    def __init__(self, app, environ, request=None):
        self.app = app
        if request is None:
            request = app.request_class(environ)
        self.request = request
```


核心实现位于 [werkzeug/wrappers.py#L453](https://github.com/bakalab/werkzeug/blob/v0.14/werkzeug/wrappers.py#L453)

```Python
# werkzeug/wrappers.py
class BaseRequest(object):
    @cached_property
    def args(self):
        return url_decode(wsgi_get_bytes(self.environ.get('QUERY_STRING', '')),
                          self.url_charset, errors=self.encoding_errors,
                          cls=self.parameter_storage_class)
```


这里对于 `QUERY_STRING` 进行了解码。那么如果是以 POST 提交 `Content-Type: application/x-www-form-urlencoded` 格式的数据，根据协议规定自然会进行解码，这个也是 Web Framework 所需要做的

题外话:PHP是世界上最好的语言

```PHP
<?php
if(eregi("hackerDJ",$_GET[id])) {
  echo("<p>not allowed!</p>");
  exit();
}

$_GET[id] = urldecode($_GET[id]);
if($_GET[id] == "hackerDJ")
{
  echo "<p>Access granted!</p>";
  echo "<p>flag: *****************} </p>";
}
?>
```

`$_GET[id]` 会自动进行 urldecode，所以编码两次就行了 `%2568ackerDJ` => `%68ackerDJ` => `hackerDJ`

### Summary up
- `PATH_INFO` 应当有 WSGI Server 进行解码
- `QUERY_STRING` 应当有框架自身进行解码
    