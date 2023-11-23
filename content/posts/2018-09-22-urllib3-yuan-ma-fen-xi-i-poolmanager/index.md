
+++
title = "urllib3 源码分析 I - PoolManager"
summary = ''
description = ""
categories = []
tags = []
date = 2018-09-22T10:02:00+08:00
draft = false
+++

本文通过 [`urllib3`](https://github.com/urllib3/urllib3) 来分析 PoolManager 的设计。所使用代码版本为 `1.23`，commit sha `7c216f433e39e184b84cbfa49e41135a89e4baa0`

`urllib3` 自带连接池，通过 `PoolManager` 进行管理。默认创建 10 个连接池

```Python
# urllib3/poolmanager.py
class PoolManager(RequestMethods):
    proxy = None

    def __init__(self, num_pools=10, headers=None, **connection_pool_kw):
        RequestMethods.__init__(self, headers)
        self.connection_pool_kw = connection_pool_kw
        self.pools = RecentlyUsedContainer(num_pools, dispose_func=lambda p: p.close())
        # Locally set the pool classes and keys so other PoolManagers can
        # override them.
        self.pool_classes_by_scheme = pool_classes_by_scheme
        self.key_fn_by_scheme = key_fn_by_scheme.copy()
```

`PoolManger` 继承自 `RequestMethods` 这个 Mixin，其为 `PoolManager` 提供了 `request` 方法

`pools` 是 `RecentlyUsedContainer` 实例，其具备线程安全的 Dict 访问，并且能够通过 LRU 算法在数据长度达到 `maxsize` 时将最久未被使用的元素销毁。其中的每个元素都是一个连接池实例

我们主要来看一下 `__getitem__` 和 `__setitem__` 的实现

```Python
# urllib3/_collections.py
class RecentlyUsedContainer(MutableMapping):

    ContainerCls = OrderedDict

    def __init__(self, maxsize=10, dispose_func=None):
        self._maxsize = maxsize
        self.dispose_func = dispose_func

        self._container = self.ContainerCls()
        self.lock = RLock()

    def __getitem__(self, key):
        # Re-insert the item, moving it to the end of the eviction line.
        with self.lock:
            item = self._container.pop(key)
            self._container[key] = item
            return item

    def __setitem__(self, key, value):
        evicted_value = _Null
        with self.lock:
            # Possibly evict the existing value of 'key'
            evicted_value = self._container.get(key, _Null)  # 需要销毁先前存在的元素
            self._container[key] = value

            # If we didn't evict an existing value, we might have to evict the
            # least recently used item from the beginning of the container.
            if len(self._container) > self._maxsize:
                _key, evicted_value = self._container.popitem(last=False)

        if self.dispose_func and evicted_value is not _Null:
            self.dispose_func(evicted_value)
```

大致就是内部维护了一个 `OrderedDict`，每次访问元素时借助 `self._container[key] = value` 将此元素放到最后，那么最前面的元素就是最久未被使用的。如果长度超过 `maxsize` 后，就把这个元素移除并通过 `dispose_func` 进行销毁

这个容器里的元素便是 Connection Pool。在我们调用 `PoolManager.reqeust` 方法时，若当前没有 Connnection Pool，则会创建。这是一种惰性初始化的方法，我们也可以直接通过 `PoolManager.connection_from_host` 来直接创建 Connnection Pool

`PoolManager.request` 方法是继承 Mixin `RequestMethods` 所获得的。其做了一些预处理，比如 urlencode、生成 HTTP Body 等

```Python
# urllib3/request.py
class RequestMethods(object):

    _encode_url_methods = set(['DELETE', 'GET', 'HEAD', 'OPTIONS'])

    def __init__(self, headers=None):
        self.headers = headers or {}

    def request(self, method, url, fields=None, headers=None, **urlopen_kw):
        method = method.upper()
        urlopen_kw['request_url'] = url
        if method in self._encode_url_methods:
            return self.request_encode_url(method, url, fields=fields,
                                           headers=headers,
                                           **urlopen_kw)
        # ...

    def request_encode_url(self, method, url, fields=None, headers=None, **urlopen_kw):
        if headers is None:
            headers = self.headers

        extra_kw = {'headers': headers}
        extra_kw.update(urlopen_kw)

        if fields:
            url += '?' + urlencode(fields)

        return self.urlopen(method, url, **extra_kw)
```

`PoolManager.urllopen` 实际上是对 `HTTPConnectionPool.urlopen` 的一层包裹

```Python
# urllib3/poolmanager.py
class PoolManager(RequestMethods):

    def urlopen(self, method, url, redirect=True, **kw):
        u = parse_url(url)
        conn = self.connection_from_host(u.host, port=u.port, scheme=u.scheme)

        kw['assert_same_host'] = False
        kw['redirect'] = False

        if 'headers' not in kw:
            kw['headers'] = self.headers.copy()

        if self.proxy is not None and u.scheme == "http":
            response = conn.urlopen(method, url, **kw)
        else:
            response = conn.urlopen(method, u.request_uri, **kw)  # 调用连接池的 urlopen 方法

        redirect_location = redirect and response.get_redirect_location()
        if not redirect_location:
            return response
        # ... 递归调用 urlopen 处理 Redirect
```

`connection_from_host` 获取了对应的连接池

```Python
# urllib3/poolmanager.py
class PoolManager(RequestMethods):

    def connection_from_host(self, host, port=None, scheme='http', pool_kwargs=None):
        if not host:
            raise LocationValueError("No host specified.")

        # 将 pool_kwargs 和初始化 PoolManager 时传入的参数进行 merge
        request_context = self._merge_pool_kwargs(pool_kwargs)
        request_context['scheme'] = scheme or 'http'
        if not port:
            # port_by_scheme = {'http': 80, 'https': 443}
            port = port_by_scheme.get(request_context['scheme'].lower(), 80)
        request_context['port'] = port
        request_context['host'] = host

        return self.connection_from_context(request_context)

    def connection_from_context(self, request_context):
        scheme = request_context['scheme'].lower()
        pool_key_constructor = self.key_fn_by_scheme[scheme]
        pool_key = pool_key_constructor(request_context)
        return self.connection_from_pool_key(pool_key, request_context=request_context)

    def connection_from_pool_key(self, pool_key, request_context=None):
        with self.pools.lock:
            # If the scheme, host, or port doesn't match existing open
            # connections, open a new ConnectionPool.
            pool = self.pools.get(pool_key)
            if pool:
                return pool

            # Make a fresh ConnectionPool of the desired type
            scheme = request_context['scheme']
            host = request_context['host']
            port = request_context['port']
            pool = self._new_pool(scheme, host, port, request_context=request_context)
            self.pools[pool_key] = pool

        return pool
```

这部分的逻辑是根据请求信息(Request Context)生成一个 key，然后从 `pools` 中取对应的连接池实例。如果没有对应的连接池，那么便创建一个。这个 key 是一个名为 `PoolKey` 的 `namedtuple` 实例，包含了以下的字段。如果请求中没有字段所需要的信息，那么便为 `None`，可以参考 `_default_key_normalizer` 的处理

```Python
_key_fields = (
    'key_scheme',  # str
    'key_host',  # str
    'key_port',  # int
    'key_timeout',  # int or float or Timeout
    'key_retries',  # int or Retry
    'key_strict',  # bool
    'key_block',  # bool
    'key_source_address',  # str
    'key_key_file',  # str
    'key_cert_file',  # str
    'key_cert_reqs',  # str
    'key_ca_certs',  # str
    'key_ssl_version',  # str
    'key_ca_cert_dir',  # str
    'key_ssl_context',  # instance of ssl.SSLContext or urllib3.util.ssl_.SSLContext
    'key_maxsize',  # int
    'key_headers',  # dict
    'key__proxy',  # parsed proxy url
    'key__proxy_headers',  # dict
    'key_socket_options',  # list of (level (int), optname (int), value (int or str)) tuples
    'key__socks_options',  # dict
    'key_assert_hostname',  # bool or string
    'key_assert_fingerprint',  # str
)

PoolKey = collections.namedtuple('PoolKey', _key_fields)
```

但是我们需要注意在 `PoolManager.urlopen` 中，我们仅向 `connection_from_host` 传入了 `scheme`、`host`、`port` 三个参数

```Python
conn = self.connection_from_host(u.host, port=u.port, scheme=u.scheme)
```

所以相当于我们以 `scheme`、`host`、`port` 作为 key

```Python
# urllib3/poolmanager.py

pool_classes_by_scheme = {
    'http': HTTPConnectionPool,
    'https': HTTPSConnectionPool,
}

class PoolManager(RequestMethods):

    def _new_pool(self, scheme, host, port, request_context=None):
        pool_cls = self.pool_classes_by_scheme[scheme]
        if request_context is None:
            request_context = self.connection_pool_kw.copy()

        # Although the context has everything necessary to create the pool,
        # this function has historically only used the scheme, host, and port
        # in the positional args. When an API change is acceptable these can
        # be removed.
        for key in ('scheme', 'host', 'port'):
            request_context.pop(key, None)

        if scheme == 'http':
            for kw in SSL_KEYWORDS:
                request_context.pop(kw, None)

        return pool_cls(host, port, **request_context)
```

`_new_pool` 就是在创建连接池(`ConnectionPool`)实例，这个放到下一篇文章中进行分析


PoolManager 和 ConnectionPool 的关系可以表示如下

```
---------------------------------------------------
|                   PoolManger                    |
| ------------------  ------------------          |
| | ConnectionPool |  | ConnectionPool |          |
| |                |  |                |          |
| | --------       |  | --------       |  ...     |
| | | Conn |  ...  |  | | Conn |  ...  |          |
| | --------       |  | --------       |          |
| ------------------  ------------------          |
---------------------------------------------------
```

    