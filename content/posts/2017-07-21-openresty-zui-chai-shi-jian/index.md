
+++
title = "openresty 最差实践"
summary = ''
description = ""
categories = []
tags = []
date = 2017-07-21T13:59:31+08:00
draft = false
+++

这周主要对 API gateway 进行选型，上周自己搞了搞 Kong。但是感觉和目前的公司业务相差比较多，如果要使用的话需要大量改动。所以重新进行了调查，大致选定了两个方向 Golang 实现和 nginx + Lua。

### 坑爹的测试
测试使用的工具是 wrk，最初时使用的蠢作者的开发机做测试，API gateway 和 backend 都放到了本机上，然后本机进行 wrk 压力测试。一番折腾后感觉这种本机到本机的测试状况不好，于是找同事借了他的开发机来进行模拟。本地 mac 用 wrk，我的开发机做转发，同事的开发机当后端。

**诡异的事情发生了** 下面的数据是今天补上的，当时的没有留档

```
➜  ~ wrk -c600 -t2 -d60s http://10.10.10.24 -H "Token: eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJmb28iOiJiYXIifQ.VAoRL1IU0nOguxURF2ZcKR0SGKE1gCbqwyh8u2MLAyY"
Running 1m test @ http://10.10.10.24
  2 threads and 600 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency   291.87ms  419.92ms   1.97s    80.58%
    Req/Sec   152.08    181.56   737.00     78.61%
  13914 requests in 1.00m, 6.67MB read
  Socket errors: connect 0, read 0, write 0, timeout 4511
  Non-2xx or 3xx responses: 3977
Requests/sec:    231.56
Transfer/sec:    113.59KB
```

QPS 只有 231，简直低到了极点。仔细一看，有 3977 个请求是非正常的

access.log
```
192.168.0.107 - - [21/Jul/2017:09:20:08 +0800] "GET / HTTP/1.1" 499 0 "-" "-"
192.168.0.107 - - [21/Jul/2017:09:20:08 +0800] "GET / HTTP/1.1" 499 0 "-" "-"
```

error.log
```
2017/07/21 09:19:43 [error] 25176#0: *16072 connect() failed (110: Connection timed out) while connecting to upstream, client: 192.168.0.107, server: , request: "GET / HTTP/1.1", upstream: "http://10.10.10.157:8080/", host: "10.10.10.24"
2017/07/21 09:19:43 [error] 25176#0: *16069 connect() failed (110: Connection timed out) while connecting to upstream, client: 192.168.0.107, server: , request: "GET / HTTP/1.1", upstream: "http://10.10.10.157:8080/", host: "10.10.10.24"
```

日志显示转发至 upstream 超时了，当时猜想可能连接数太多了，没有及时响应所导致的。于是上调了 `proxy_connect_timeout` 和 `proxy_read_timeout` 等参数，然而并没有卵用。我试着调小打开的连接数目(c参数)，从 10 开始逐步调到 60，QPS 呈逐步上升，60 时能有个 800 requests/sec 的样子。索性我直接调到 100，想用二分法查找出它能承受的最大数目。艹，只有 10 几了。这根本不科学，我又调到 70，还是 10 几，日志中全都是上面的那种错误。再调到 60，只有 100 多。刚才明明 800+ 的，现在却低至这种地步。我感觉并不是 openresty 的锅。而当时测试的 Golang 实现的反向代理 traefik 有着良好的表现，虽然不及 nginx，却也有着其 70% 左右的性能。当日我们一度放弃了 openresty，决定使用 Golang 进行开发。可我隐隐觉得不对劲， openresty 虽然还比较小众，但是也有公司在实际使用，总不可能性能低至这种地步。于是我向技术总监说了这件奇怪的事，他也一脸懵逼，可毕竟老油条，他说会不会是内网有什么问题，给了我四台 aliyun 的机子，让我再去 benchmark

一番编译配置等繁琐的工序后，openresty 貌似展现出正常的性能了，下面是测试数据


```
        backend1 2core/2G           backend2 2core/2G
                \                        /
                 \                      /
               openresty/nginx/traefik/echo  转发
                            |
                            |
                          client

```

#### openresty 转发

```
➜  ~ wrk -c600 -t2 -d60s http://192.168.8.16 -H "Token: eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJmb28iOiJiYXIifQ.VAoRL1IU0nOguxURF2ZcKR0SGKE1gCbqwyh8u2MLAyY"
Running 1m test @ http://192.168.8.16
  2 threads and 600 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency   104.33ms   10.05ms 367.00ms   85.91%
    Req/Sec     2.88k   312.83     5.70k    85.83%
  343575 requests in 1.00m, 374.19MB read
Requests/sec:   5723.93
Transfer/sec:      6.23MB
```

#### nginx 转发

```
➜  ~ wrk -c600 -t2 -d60s http://192.168.8.16 -H "Token: eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJmb28iOiJiYXIifQ.VAoRL1IU0nOguxURF2ZcKR0SGKE1gCbqwyh8u2MLAyY"
Running 1m test @ http://192.168.8.16
  2 threads and 600 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency   116.33ms  163.45ms   1.44s    93.53%
    Req/Sec     3.61k     0.88k    6.58k    72.83%
  430983 requests in 1.00m, 349.36MB read
  Socket errors: connect 0, read 0, write 0, timeout 106
Requests/sec:   7180.68
Transfer/sec:      5.82MB
```

#### traefik 转发

```
➜  ~ wrk -c600 -t2 -d60s http://192.168.8.16:8090 -H "Token: eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJmb28iOiJiYXIifQ.VAoRL1IU0nOguxURF2ZcKR0SGKE1gCbqwyh8u2MLAyY"
Running 1m test @ http://192.168.8.16:8090
  2 threads and 600 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency   145.90ms   59.36ms 684.21ms   75.25%
    Req/Sec     2.08k   492.73     3.53k    71.81%
  248381 requests in 1.00m, 187.13MB read
Requests/sec:   4137.66
Transfer/sec:      3.12MB
```

#### echo 转发

```
➜  ~ wrk -c600 -t2 -d60s http://192.168.8.16:1323 -H "Token: eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJmb28iOiJiYXIifQ.VAoRL1IU0nOguxURF2ZcKR0SGKE1gCbqwyh8u2MLAyY"
Running 1m test @ http://192.168.8.16:1323
  2 threads and 600 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency   172.37ms   53.05ms 623.39ms   76.56%
    Req/Sec     1.75k   490.84     3.02k    66.67%
  208662 requests in 1.00m, 157.21MB read
Requests/sec:   3475.66
Transfer/sec:      2.62MB
```

总体来看, openresty 有着仅次于 nginx 的转发性能。并且易于进行扩展开发，能够满足日益增长的业务需求。但想用 openresy 就要去雪 Lua，可貌似 Lua 对于我们来说只能做 openresty。Golang 则还可以开发一些别的项目。经过一番商量最终还是决定入 openresty 这个坑。


附测试时的配置

#### openresty

```
worker_processes 1;
worker_rlimit_nofile 200000;

events {
    worker_connections 10000;
    use epoll;
    multi_accept on;
}

http {
    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    keepalive_requests 100000;
    types_hash_max_size 2048;
    keepalive_timeout 600;
    proxy_connect_timeout 600;
    proxy_read_timeout 600;
    open_file_cache max=200000 inactive=300s;
    open_file_cache_valid 300s;
    open_file_cache_min_uses 2;
    open_file_cache_errors on;

    upstream backend {
        server 192.168.8.10;
        server 192.168.8.17;
    }
    server {
        listen 80;

        location / {
            access_by_lua '
                local cjson = require "cjson"
                local jwt = require "resty.jwt"

                local jwt_token = ngx.req.get_headers()["Token"]
                local jwt_obj = jwt:verify("lua-resty-jwt", jwt_token)
                -- verified the token
                if jwt_obj.verified == true then
                    ngx.header["User-Info"] = cjson.encode(jwt_obj)
                else
                    ngx.exit(ngx.HTTP_FORBIDDEN)
                end
            ';
            proxy_pass http://backend;
        }
    }
}
```


#### nginx 配置

```
worker_processes 1;
worker_rlimit_nofile 200000;

events {
    worker_connections 10000;
    use epoll;
    multi_accept on;
}

http {
    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    keepalive_requests 100000;
    types_hash_max_size 2048;
    keepalive_timeout 600;
    proxy_connect_timeout 600;
    proxy_read_timeout 600;
    open_file_cache max=200000 inactive=300s;
    open_file_cache_valid 300s;
    open_file_cache_min_uses 2;
    open_file_cache_errors on;

    upstream backend {
        server 192.168.8.10;
        server 192.168.8.17;
    }
    server {
        listen 80;

        location / {
           proxy_pass http://backend;
        }
    }
}
```


#### traefik 配置

```
MaxIdleConnsPerHost = 100000
defaultEntryPoints = ["http"]
# logLevel = "DEBUG"

[entryPoints]
  [entryPoints.http]
  address = ":8090"

[file]
  [backends]
    [backends.httpecho]
      [backends.httpecho.servers.server1]
        url = 'http://192.168.8.10'
        weight = 1
      [backends.httpecho.servers.server2]
        url = "http://192.168.8.17"
        weight = 1
  [frontend]
    [frontends.fe1]
    backend = "httpecho"
```

#### server.go
```
package main

import (
    "net/url"

    "github.com/labstack/echo"
    "github.com/labstack/echo/middleware"
)

func main() {
    e := echo.New()

    // Setup proxy
    url1, err := url.Parse("http://192.168.8.10")
    if err != nil {
        e.Logger.Fatal(err)
    }
    url2, err := url.Parse("http://192.168.8.17")
    if err != nil {
        e.Logger.Fatal(err)
    }
    e.Use(middleware.Proxy(&middleware.RoundRobinBalancer{
        Targets: []*middleware.ProxyTarget{
            &middleware.ProxyTarget{
                URL: url1,
            },
            &middleware.ProxyTarget{
                URL: url2,
            },
        },
    }))

    e.Start(":1323")
}
```

    