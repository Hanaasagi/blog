
+++
title = "Openresty 随手记"
summary = ''
description = ""
categories = []
tags = []
date = 2017-08-14T14:17:16+08:00
draft = false
+++

Openresty 执行阶段

<img style="max-width:820px;" src="https://cloud.githubusercontent.com/assets/2137369/15272097/77d1c09e-1a37-11e6-97ef-d9767035fc3e.png" />

我们可以在 `init_by_lua` 中写一些启动配置，比如读取配置文件等。之后 nginx 会 fork 出若干 worker 进程。由于 fork 的性质，worker 中拥有父进程(master 进程)的数据拷贝。所以依然能够访问到这些在 `init_by_lua` 中加载的变量。但是此时你修改变量的值并不会影响到其他的 worker

全局变量是十分危险的，尽量不要这么做，替代的方法是将其用 module 作为 namespace 进行包裹

```Lua
-- init_by_lua
local setting = require("setting")
setting:load(path)

-- setting.lua
local lyaml = require("lyaml")


local _M = {}


local function flatten(data)
    -- load the setting automaticlly
    local result = {}
    for k, v in pairs(data) do
        if type(v) == 'table' then
            result[k] = flatten(v)
      else
            result[k] = v
        end
    end
    return result
end


function _M:load(filepath)
    local f = io.open(filepath)
    local content = f:read("*all")
    f:close()
    local config = lyaml.load(content)
    self.config = flatten(config)
end


return setmetatable(_M, {__index = function(self, attr)
    return self.config[attr]
end})
```

这样需要使用 setting 时， `require` 一下即可

另外只有在 `init_by_lua` 和 `init_worker_by_lua` 中定义的全局变量才会在之后的执行阶段中访问到。其他阶段中，Openresty 会隔离每次请求创建的全局变量。也就是说你在 `access_by_lua` 中创建的全局变量，并不是真正的全局变量，在 `content_by_lua` 中是访问不到的

当关闭 `lua_code_cache` 时，每一个 ngx_lua 处理的请求都将运行在一个独立的 Lua VM 中,这意味着 `init_by_lua`  `init_worker_by_lua` 都会重新执行。生产环境下 `lua_code_cache` 应当启用。这时 require 的模块将被缓存，仅加载一次。如果被 require 的模块中创建了全局变量，便会产生问题。因为全局变量仅在当前的上下文(执行阶段)内有效。并且这是默认开启的，如果强行要 off，会有警告
```
nginx: [alert] lua_code_cache is off; this will hurt performance in /opt/openresty/nginx/conf/nginx.conf:10
```

如果想要在不同的执行阶段中共享变量， 可以借助 `ngx.ctx`，它能存放各种类型

如果想要在不同的 worker 间共享变量，可以使用 `ngx.sharedict`，这是一个 shm。不过遗憾的是，只能接受 `number` `string`  `boolean` 三种类型

还有一个问题是 `lua_code_cache on` 下模块被缓存，导致模块中的 `local` 变量在请求间共享，比如说，A 请求调用了模块中的函数 `change()`

```
local _M = {}
local count = 1
function _M.change()
    ngx.say(count)
    count = 2333
    ngx.sleep(10)
end
return _M
```

紧接着 B 请求也调用了此函数，会发现一个 response 为 1，另一个为 2333。这种问题和竞态问题有些相似

    