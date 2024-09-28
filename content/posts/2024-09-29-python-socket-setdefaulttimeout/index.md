+++
title = "Python socket.setdefaulttimeout"
summary = ""
description = ""
categories = [""]
tags = [""]
date = 2024-09-28T16:01:06+09:00
draft = false

+++

最近在代码中发现了这么一行

```python
import socket

socket.setdefaulttimeout(300)
```

看起来就很迷，问了一下说是用来做全局 `requests` 超时用的。遇到问题，先看文档

- `socket.setdefaulttimeout(*timeout*)`

  > Set the default timeout in seconds (float) for new socket objects. When the socket module is first imported, the default is `None`. See [`settimeout()`](https://docs.python.org/3/library/socket.html#socket.socket.settimeout) for possible values and their respective meanings.
  >
  > https://docs.python.org/3/library/socket.html#socket.setdefaulttimeout

- `socket.settimeout(*value*)`

  > Set a timeout on blocking socket operations. The _value_ argument can be a nonnegative floating-point number expressing seconds, or `None`. If a non-zero value is given, subsequent socket operations will raise a [`timeout`](https://docs.python.org/3/library/socket.html#socket.timeout) exception if the timeout period _value_ has elapsed before the operation has completed. If zero is given, the socket is put in non-blocking mode. If `None` is given, the socket is put in blocking mode.
  >
  > For further information, please consult the [notes on socket timeouts](https://docs.python.org/3/library/socket.html#socket-timeouts).
  >
  > Changed in version 3.7: The method no longer toggles [`SOCK_NONBLOCK`](https://docs.python.org/3/library/socket.html#socket.SOCK_NONBLOCK) flag on [`socket.type`](https://docs.python.org/3/library/socket.html#socket.socket.type).
  >
  > https://docs.python.org/3/library/socket.html#socket.socket.settimeout

我们不妨来做个实验

## 实验

- Python 3.12.6
- requests 2.32.3
- httpx 0.27.0

- server.go

```go
package main

import (
	"fmt"
	"net/http"
	"time"
)

func handler(w http.ResponseWriter, r *http.Request) {
	time.Sleep(10 * time.Second)
	fmt.Fprintf(w, "Hello World")
}

func main() {
	http.HandleFunc("/", handler)

	fmt.Println("Server is listening on port 8080...")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Println("Error starting server:", err)
	}
}
```

- test_requests.py

```python
import time
import socket

socket.setdefaulttimeout(5)

import requests

start_at = time.perf_counter()
try:
    resp = requests.get("http://127.0.0.1:8080/")
except Exception as e:
    print(e)
end_at = time.perf_counter()

print(f"Request took {end_at - start_at} seconds")


### Output:
### Request took 10.008946789999754 seconds
```

- test_httpx.py

```python
import socket

socket.setdefaulttimeout(5)

import time
import httpx

start_at = time.perf_counter()
try:
    with httpx.Client() as client:
        resp = client.get("http://127.0.0.1:8080/")
except Exception as e:
    print(e)
end_at = time.perf_counter()

print(f"Request took {end_at - start_at} seconds")


### Output:
### timed out
### Request took 5.009853745999862 seconds
```

其实这些结果，文档已经说的很清楚了，但还是象征性翻一下源码吧。**本文所引用到的代码片段，License 和对应 codebase 相同**

## 为什么 requests 不会产生超时

首先我们对于诸如 `requests.get` 、`requests.post` 等调用都会走到一个新的 `Session` 中。

https://github.com/psf/requests/blob/1ae6fc3137a11e11565ed22436aa1e77277ac98c/src/requests/sessions.py#L584-L589

```python
class Session(SessionRedirectMixin):

    def request(
        self,
		# ...
		timeout=None,
        # ...
    ):
    	# ...

        # Send the request.
        send_kwargs = {
            "timeout": timeout,
            "allow_redirects": allow_redirects,
        }
        send_kwargs.update(settings)
        resp = self.send(prep, **send_kwargs)

        return resp


     def send(self, request, **kwargs):
        # ...

        # Send the request
        r = adapter.send(request, **kwargs)

        # ...

```

我们需要注意 `timeout` 参数。在 `requests` 中，我们如果没有显式传参，那么默认都是 `None` 。这个参数的含义在文档上是这么表述的

https://requests.readthedocs.io/en/latest/api/#requests.request

> **timeout** ([_float_](https://docs.python.org/3/library/functions.html#float) _or_ [_tuple_](https://docs.python.org/3/library/stdtypes.html#tuple)) – (optional) How many seconds to wait for the server to send data before giving up, as a float, or a [(connect timeout, read timeout)](https://requests.readthedocs.io/en/latest/user/advanced/#timeouts) tuple.

这里隐含了额外的含义就是 `None` 是代表没有任何超时设置的。我们可以顺着代码接着来看，在 `Session.send` 的实现中使用了 `adapter` ，这个地方默认是使用的 `HTTPAdapter` ，他底层是使用的 `urllib3` 做了一个连接池

https://github.com/psf/requests/blob/1ae6fc3137a11e11565ed22436aa1e77277ac98c/src/requests/adapters.py#L677

```python
class HTTPAdapter(BaseAdapter):

	def send(
        self, request, stream=False, timeout=None, verify=True, cert=None, proxies=None
    ):
        # ...
        if isinstance(timeout, tuple):
            try:
                connect, read = timeout
                timeout = TimeoutSauce(connect=connect, read=read)
            except ValueError:
                raise ValueError(
                    f"Invalid timeout {timeout}. Pass a (connect, read) timeout tuple, "
                    f"or a single float to set both timeouts to the same value."
                )
        elif isinstance(timeout, TimeoutSauce):
            pass
        else:
            timeout = TimeoutSauce(connect=timeout, read=timeout)

        try:
            resp = conn.urlopen(
                method=request.method,
                url=url,
				# ...
                timeout=timeout,
                chunked=chunked,
            )

        except (ProtocolError, OSError) as err:
            raise ConnectionError(err, request=request)

        # ...
```

因为需要调用到 `urllib3` ，所以这里对于 `requests` 自身的 `timeout` 参数进行了类型转换。如果走默认的参数值 `None`，那么这里转换后的得到的会是 `Timeout(connect=None, read=None)` 对象，这个的含义是 `connect` 系统调用不设置超时，`read` 系统调用不设置超时。如果我们在 `requests` 使用 `timeout=5` ，那么这里会得到 `Timeout(connect=5, read=5)`

关于 `urllib3` 的源码分析，可以参考我之前的文章

- [urllib3 源码分析 I - PoolManager](https://blog.dreamfever.me/posts/2018-09-22-urllib3-yuan-ma-fen-xi-i-poolmanager/)

- [urllib3 源码分析 II -ConnectionPool](https://blog.dreamfever.me/posts/2018-09-23-urllib3-yuan-ma-fen-xi-ii-connectionpool/)

这里不再赘述，直接看这个 `timeout` 会用到哪里。`_make_request` 是一个核心函数，基本上都会走到这里

https://github.com/urllib3/urllib3/blob/2458bfcd3dacdf6c196e98d077fc6bb02a5fc1df/src/urllib3/connectionpool.py#L379

```python
class HTTPConnectionPool(ConnectionPool, RequestMethods):
	def _make_request(
        self,
        # ...
        timeout: _TYPE_TIMEOUT = _DEFAULT_TIMEOUT,
		# ...
    ) -> BaseHTTPResponse:
        timeout_obj = self._get_timeout(timeout)
        conn.timeout = Timeout.resolve_default_timeout(timeout_obj.connect_timeout)

        try:
            conn.request(
				# ...
            )

        except BrokenPipeError:
            pass
        except OSError as e:
            if e.errno != errno.EPROTOTYPE and e.errno != errno.ECONNRESET:
                raise

        read_timeout = timeout_obj.read_timeout
        if not conn.is_closed:
            # In Python 3 socket.py will catch EAGAIN and return None when you
            # try and read into the file pointer created by http.client, which
            # instead raises a BadStatusLine exception. Instead of catching
            # the exception and assuming all BadStatusLine exceptions are read
            # timeouts, check for a zero timeout before making the request.
            if read_timeout == 0:
                raise ReadTimeoutError(
                    self, url, f"Read timed out. (read timeout={read_timeout})"
                )
            conn.timeout = read_timeout

        # Receive the response from the server
        try:
            response = conn.getresponse()
        except (BaseSSLError, OSError) as e:
            self._raise_timeout(err=e, url=url, timeout_value=read_timeout)
            raise

```

可以看到存在两处位置在重置 `conn` 对象的 `timeout` 属性。这两个位置分别对应了发送请求时候的 `connect` 阶段，和读取响应时候的 `read` 阶段。拿 `getresponse` 来举例

https://github.com/urllib3/urllib3/blob/2458bfcd3dacdf6c196e98d077fc6bb02a5fc1df/src/urllib3/connection.py#L481

```python
class HTTPConnection(_HTTPConnection):

    def getresponse(  # type: ignore[override]
        self,
    ) -> HTTPResponse:
        self.sock.settimeout(self.timeout)

        from .response import HTTPResponse

        httplib_response = super().getresponse()

        response = HTTPResponse(
            body=httplib_response,
			# ...
        )
        return response
```

这里会调用 `sock.settimeout` 来重设超时，所以 `setdefaulttimeout` 的值是完全不会生效的。同样的，可以看一下 `_new_conn` 等函数，也是差不多的写法

## 为什么 httpx 会产生超时

这个问题就很简单了，因为他不像 `requests`，他是一个有默认超时的请求库

https://github.com/encode/httpx/blob/95a9527ed6a8b9a8e53ddee2f61447fb1a37e2a9/httpx/_client.py#L642

```python
DEFAULT_TIMEOUT_CONFIG = Timeout(timeout=5.0)
USE_CLIENT_DEFAULT = UseClientDefault()

class Client(BaseClient):

    def __init__(
        self,
        *,
		# ...
        timeout: TimeoutTypes = DEFAULT_TIMEOUT_CONFIG,
		# ...
    ) -> None:
    	# ...

    def get(
        self,
        url: URL | str,
        *,
		# ...
        timeout: TimeoutTypes | UseClientDefault = USE_CLIENT_DEFAULT,
		# ...
    ) -> Response:

```

当我们初始化一个 `Client` 的时候，默认具有 5s 的超时。在 `client.get` 等方法调用时，如果没有显式传参则值为 `USE_CLIENT_DEFAULT`。这是一个全局对象用于表示使用 `Client` 对象的超时时间

所以其实我们的 `socket.setdefaulttimeout(5)` 也并没有起到作用。真正的超时是因为 httpx 的默认超时时间，你可以修改成 `socket.setdefaulttimeout(1)` 再来实验一下，发现还是会 5s 之后返回

## socket.setdefaulttimeout 对于 HTTP 来说有用么

很遗憾，在大多数情况下可能没用，因为一些第三方库可能会这样做

```python
        default_timeout = socket.getdefaulttimeout()
        socket.setdefaulttimeout(timeout)
        try:
            # ...
        finally:
            socket.setdefaulttimeout(default_timeout)
```

除此之外，对于 HTTP 来说，我们的超时一般是指一个 HTTP请求/响应总体的时长。而 `socket.settimeout` 是控制的 TCP 读取写入的时长。拿 HTTP Chunked transfer encoding 来举个例子

```go
package main

import (
	"fmt"
	"net/http"
	"time"
)

func handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)

	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	message := "Hello World"
	for _, char := range message {
		fmt.Fprint(w, string(char))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		time.Sleep(1 * time.Second)
	}
}

func main() {
	http.HandleFunc("/", handler)

	fmt.Println("Server is listening on port 8080...")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Println("Error starting server:", err)
	}
}
```

假设客户端设置 `timeout=2`，但是因为每 1 秒都会有一个 byte 过来，所以实际上不会触发超时阈值。如果等待完整的 HTTP 响应，那么会等待 11 秒（虽然我们一般会通过流式来消费这个数据，不会产生太大问题）。但是不能排除 API Server 直接返回巨型 JSON 时也产生这种情况的可能

如果我们想要真正控制 HTTP 的超时，那么选择可以有如下几个

- 使用 Non-blocking IO + eventloop，比如 Python 的 asyncio，或者换 Go / Node.js
- 通过中间件/旁路注入，比如 sidecar 来做流量管理，https://istio.io/latest/docs/tasks/traffic-management/request-timeouts/

对于 TCP socket 读写来说我们需要 `setdefaulttimeout` 么？socket 的读写应该视场景而定，对于不同的 socket 我们应该会有不同的设置。全局直接 patch 掉超时，虽然感觉很聪明，但我认为这完全是一个坏行为。 **Explicit is better than implicit.**
