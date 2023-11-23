
+++
title = "Hanaasagi's blog 容器化之旅"
summary = ''
description = ""
categories = []
tags = []
date = 2017-11-24T15:03:57+08:00
draft = false
+++

服务器运行 246 天，内存已经在 700 M 上下了。考虑到以后方便还是决定进行容器化。首先看一下 `node` 的版本，4.4.3。与最新的 9.2.0 已经相差太多了。而且 ghost 的版本也太低了，我也不想去升级，因为新版的 ghost 体验并不好。所以直接复制原环境，这样省事不少。当然现在的做法是给以后留坑

#### 迁移 ghost
基于 4.4.3 的 `node` 镜像进行构建。建一个空的目录，复制 `package.json` 添加如下的 `Dockerfile`

```Dockerfile
FROM node:4.4.3

WORKDIR /var/www/ghost/

COPY package.json /var/www/ghost/package.json

RUN npm install --production && \
    npm install forever -g && \
    npm cache clean && \
    rm -rf package.json
```
执行

```
docker build -t blog .
```

将启动命令做成脚本，方便以后使用 `blog.sh`

```Bash
#!/bin/bash

docker run -d \
  --name blog \
  -v /var/www/ghost:/var/www/ghost \
  -e NODE_ENV=production \
  --net=host \
  --log-opt max-size=10m \
  --log-opt max-file=9 \
  blog forever index.js
```

#### 迁移 nginx

这次做的一个重大的改变是，对未翻墙的人一个友好的提示，所以将 nginx 直接换为可编程扩展的 openresty

直接用 openresty 提供的 alpine 镜像，在 `nginx.conf` 中的 `location` section 中添加如下的代码 

```Lua
access_by_lua_block {
  if ngx.req.get_headers()['cf-ipcountry'] == 'CN' then
    ngx.say('the fox jumps over the lazy dog')
    ngx.exit(ngx.OK)
  end
}
```

上述代码针对 cloudfare 代理的情况。也可以自己来获取 ip 对应的区域

不过需要安装 `resty-http` 这个包，所以基于 `alpine-fat` 镜像再次构建，这是一个带有 `luarocks` 的镜像

```Dockerfile
FROM openresty/openresty:alpine-fat

RUN luarocks install lua-resty-http
```

`http` section 中添加  `lua_shared_dict ip_cache 10m;` 用于缓存, `server` section 中添加 `resolver 8.8.8.8;` 否则无法解析域名。

```Lua
access_by_lua_block {
  local cjson = require('cjson')
  
  local ip_cache = ngx.shared['ip_cache']
  local remote_addr = ngx.var.remote_addr
  local is_chinese = ip_cache:get(remote_addr)

  if is_chinese == nil then
    local http = require('resty.http')
    local httpc = http.new()
    local uri = 'http://ip-api.com/json/'..ngx.var.remote_addr

    local res, err = httpc:request_uri(uri)
    if res.status ~= 200 then
      ngx.exit(res.status)
    end

    local data = cjson.decode(res.body)
    if data['countryCode'] == 'CN' then
      ip_cache:set(remote_addr, ngx.time() + 60 * 60 * 24)
    end
  end

  is_chinese = ip_cache:get(remote_addr)
  if is_chinese then
    ngx.say('the fox jumps over the lazy dog')
    ngx.exit(ngx.OK)
  end
}
```

启动脚本

```Bash
docker run -d --name openresty \
  -v /var/www/conf:/usr/local/openresty/nginx/conf \
  -v /var/www/mainpage:/var/www/mainpage \
  --net=host \
  --restart=always \
  --log-opt max-size=10m \
  --log-opt max-file=9 \
  openresty/openresty:alpine
```
    