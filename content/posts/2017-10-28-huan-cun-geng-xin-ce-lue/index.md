
+++
title = "缓存更新策略"
summary = ''
description = ""
categories = []
tags = []
date = 2017-10-28T08:00:47+08:00
draft = false
+++

DB 查询是比较昂贵的，而且在高并发下容易 crash。所以对于查询量大的数据，可以放到缓存中减轻 DB 的压力。没有什么万全的解决办法。比如下面这种常见的套路其实仔细想想还是比较有问题的：

>先从缓存查数据，若有结果则直接返回；没有结果则从 DB 中查数据然后放到缓存中
>数据更新时先把数据存到 DB 中，成功后，再写入缓存

第一点如果同时过来 n 个请求他们都发现 cache 没有数据(或者失效)然后都会去请求数据库，会给 DB 带来瞬间的压力。这被称为缓存失效风暴。第二点当数据发生更新时，在存到 DB 和写入缓存的间隔内，二者的数据是不一致的(存在访问过期数据的情况)。如果采用异步刷新缓存，这种不一致的情况比较明显。所以如果想要追求频繁更新下高性能，不妨调换一下。直接对缓存查询/存储数据，然后异步去写入 DB，保证最终一致(有点类似 Linux 下的 write back 机制)。这里的 DB 只负责持久化，而 cache 负责原来的 DB 的责任。当然这种用法场景有限。第三点问题出在“数据 存储至 DB 成功后，写入缓存”这里。假设如果有两个并发的数据更新操作 A 和 B，写入数据库的顺序可能为 A，B，但接下来刷新缓存的顺序可能为 B，A。这样也会造成二者产生数据不一致。所以这里应当去给缓存一个标记，然后由之后的第一次查询请求来进行缓存的更新(也可以将两个操作放到一个事务中)

另外一点，db 查询未查到数据的状态也应当进行缓存(缓存空数据集)

刚才提到了由 cache 负责写入 DB 的套路。其实有一种叫做 Write Through 的模式([wiki](https://en.wikipedia.org/wiki/Cache_(computing)))，就是由 cache 来代理后一层的 DB。所有的读写都操作 cache。说白了好处就是对应用程序透明，你看到的是一个单一的存储层。而由我们的应用代码主动维护 cache 和 DB 的这种模式被称为 Cache Aside。

缓存失效风暴(AKA: Dog Pile Effect)可以通过锁来解决，这里参考 openresty 的做法。需要留意一下 step 3

```Lua、
-- copy from https://github.com/openresty/lua-resty-lock#for-cache-locks
local resty_lock = require "resty.lock"
local cache = ngx.shared.my_cache

-- step 1:
local val, err = cache:get(key)
if val then
    ngx.say("result: ", val)
    return
end

if err then
    return fail("failed to get key from shm: ", err)
end

-- cache miss!
-- step 2:
local lock, err = resty_lock:new("my_locks")
if not lock then
    return fail("failed to create lock: ", err)
end

local elapsed, err = lock:lock(key)
if not elapsed then
    return fail("failed to acquire the lock: ", err)
end

-- lock successfully acquired!

-- step 3:
-- someone might have already put the value into the cache
-- so we check it here again:
val, err = cache:get(key)
if val then
    local ok, err = lock:unlock()
    if not ok then
        return fail("failed to unlock: ", err)
    end

    ngx.say("result: ", val)
    return
end

--- step 4:
local val = fetch_redis(key)
if not val then
    local ok, err = lock:unlock()
    if not ok then
        return fail("failed to unlock: ", err)
    end

    -- FIXME: we should handle the backend miss more carefully
    -- here, like inserting a stub value into the cache.

    ngx.say("no value found")
    return
end

-- update the shm cache with the newly fetched value
local ok, err = cache:set(key, val, 1)
if not ok then
    local ok, err = lock:unlock()
    if not ok then
        return fail("failed to unlock: ", err)
    end

    return fail("failed to update shm cache: ", err)
end

local ok, err = lock:unlock()
if not ok then
    return fail("failed to unlock: ", err)
end

ngx.say("result: ", val)
```

### Reference
[屌丝程序员如何打造日PV百万的网站架构](https://speakerdeck.com/shiningray/diao-si-cheng-xu-yuan-ru-he-da-zao-ri-pvbai-mo-de-wang-zhan-jia-gou?slide=46)  
[Cache-Aside pattern](https://docs.microsoft.com/en-us/azure/architecture/patterns/cache-aside)  
[缓存更新的套路](https://coolshell.cn/articles/17416.html)

    