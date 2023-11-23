
+++
title = "对 Tornado 路由进行魔改"
summary = ''
description = ""
categories = []
tags = []
date = 2017-04-15T15:05:00+08:00
draft = false
+++

*本文将试图实现一个装饰器形式的 Tornado 路由*

根据蠢作者的浅薄经验，路由配置一般分为两种形式：一种是集中配置，比如在一个文件中写出 Web 应用的所有路由，这样比较直观；另一种则是分散配置，将路由分散到各个模块中去，这样使得路由和对应的处理代码紧凑到一起便于模块化开发

Tornado 便是集中配置的典型，在 Tornado 中我们是通过将路由与对应的 Handler 组成一个个元组作为 application 的初始化参数

```python
import tornado.ioloop
import tornado.web


class MainHandler(tornado.web.RequestHandler):

    def get(self):
        self.write('Hello World')

if __name__ == '__main__':
    app = tornado.web.Application([
        (r'/', MainHandler),
    ])

    app.listen(8888)
    tornado.ioloop.IOLoop.current().start()

```

而轻量级框架 Flask 则是采用分散式配置(利用Blueprint)

```python
# 目录结构
# .
# ├── app
# │   ├── api
# │   │   ├── __init__.py
# │   │   └── user.py
# │   └── __init__.py
# └── main.py

# main.py

from app import app

if __name__ == '__main__':
    app.run()

# app/__init__.py

from flask import Flask
from .api import api as api_blueprint

app = Flask(__name__)
app.register_blueprint(api_blueprint)

# app/api/__init__.py

from flask import Blueprint

api = Blueprint('api', __name__)

from . import user

# app/api/user.py

from . import api

@api.route('/api')
def api_handler():
    return 'api'

```

那下面进入正题，对 Tornado 的集中式路由配置进行魔改，首先增加装饰器形式的路由

```python
import tornado.ioloop
import tornado.web

app = tornado.web.Application()

class route(object):

    def __init__(self, url):
        self.url = url

    def __call__(self, cls):
        app.add_handlers(
            r'.*$', (tornado.web.url(self.url, cls, name=cls.__name__),)
        )
        return cls

@route(r'/')
class MainHandler(tornado.web.RequestHandler):

    def get(self):
        self.write('Hello World')

if __name__ == '__main__':
    app.listen(8888)
    tornado.ioloop.IOLoop.current().start()
```

上面说白了是 `app.add_handlers` 的一层语法糖。这样虽然可以完成，但是由于 route 中使用了 app 导致必须将 app 的定义到第一次使用 route 的 `__call__` 方法之前，令人不爽。那么现在模仿一下 Flask 的 Blueprint 来看看，在装饰器路由和 `app.add_handlers` 间添加一层降低耦合度。

```python
# 目录结构
# .
# ├── app
# │   ├── api
# │   │   ├── handler.py
# │   │   └── __init__.py
# │   ├── auth
# │   │   ├── handler.py
# │   │   └── __init__.py
# │   └── __init__.py
# ├── patch.py
# └── main.py

# main.py

import tornado.ioloop
import tornado.web
from app import route


class RouteMixin(object):

    def register_handler(self, url_pattern, host_pattern=r'.*$'):
        self.add_handlers(host_pattern, url_pattern)


class Application(tornado.web.Application, RouteMixin):

    pass


if __name__ == '__main__':
    app = Application()
    app.register_handler(route.handlers)
    app.listen(8888)
    tornado.ioloop.IOLoop.current().start()

# app/__init__.py

from .auth.handler import route as auth_route
from .api.handler import route as api_route

assert (auth_route is api_route) # same object

route = auth_route

# app/api/handler.py

import tornado.web
from patch import route

@route(r'/api')
class ApiHandler(tornado.web.RequestHandler):

    def get(self):
        self.write('api')

# app/auth目录代码相似

```
以上应该还有改进空间，有时间去看看 Flask 的 Blueprint 的实现

    