
+++
title = "浅谈 API Gateway"
summary = ''
description = ""
categories = []
tags = []
date = 2017-07-15T03:28:11+08:00
draft = false
+++

写一点最近的工作经验

### 何为 SOA
SOA（Service-Oriented Architecture，面向服务的架构）是一种设计方法，其中包含多个服务，而服务之间通过配合最终会提供一系列功能。一个服务通常以独立的形式存在于操作系统进程中。服务之间通过网络调用，而非采用进程内调用的方式进行通信。

人们逐渐认识到 SOA 可以用来应对臃肿的单块应用程序,从而提高软件的可重用性,比如多个终端用户应用程序可以共享同一个服务。它的目标是在不影响其他任何人的情况下透明地替换一个服务,只要替换之后的服务的外部接口没有太大的变化即可。这种性质能够大大简化软件维护甚至是软件重写的过程

*引用自 <<微服务设计>>*

```
  帖子    <--------    朋友    -------->    图片
<<Ruby>>           <<Python>>           <<Java>>
  ||                   ||                  ||
  \/                   \/                  \/
文档存储              图数据库            blob 存储
```

### 为什么需要 API Gateway
一个客户端的某个功能可能会需要 n 个服务相互配合，所以会同时访问 n 个服务(可能小于 n，因为有些是服务请求服务)。而这些服务中会有一些相似的部分，比如对用户的身份进行验证。这些功能是相似甚至完全相同的。n 个服务，我们也不能让客户端去发送 n 个请求，这样十分消耗流量，是对客户的不负责。另外服务一般是动态的，可能会对服务进行拆分或者合并，这种变动将会影响前端的逻辑。所以不应当由客户端直接去访问服务，而应当存在一个映射关系，实现客户端访问单一资源就能够得到多个服务的响应聚合。在这种背景下，就诞生了 API Gateway 这样的抽象层，负责作为单一入口。当然还有诸多其他用处，可以参考 [Amazon API Gateway](https://aws.amazon.com/cn/api-gateway/) 的宣传资料

### 架构
先来看一下通常的 Web 架构。一般来说现在都是在应用服务器前添加几台 nginx，高级一点的前面还有一层负载均衡

```
                        负载均衡 (LVS 或 SLB)
                                                              转发
                nginx         nginx        nginx
                                                              转发
            service  service  service  service  service
(service 之间也会有一些相互调用)
```
一般而言应用服务器都集成了 Web 服务器，比如 gunicorn。那么为什么还需要 nginx 呢？使用 nginx 明显多添加了一层，需要多进行一次 socket 连接，为什么都说添加 nginx 反向代理后会带来性能上的提升呢？

原因主要有一下几点

1) nginx 性能优越，能够保持住大量的连接。可以在 nginx 中将 https 转为 http，减少 backend 计算压力。(这也有可能由负载均衡来做)
2) 处理静态资源，当然也可以走 CDN
3) 一般来说应用服务器会同时开启多个进程，这样可以使用 nginx 来统一端口。还可以添加一些 Location 或 Rewrite 规则

这种模型就已经有了 Gateway 的概念，比如使用 nginx 来做 rate limit。nginx 就像是进入系统的一个服务提供点，好吧这里负载均衡貌似更像一个系统入口，但是这个有可能是硬件的，也有可能你摸不到，比如 阿里云的SLB。更重要的是 nginx 提供了 Lua 脚本的扩展，可以添加一些定制化的东西。现有一个比较好的 API gateway 就是基于 nginx 的 [Kong](http://getkong.org)

另外就是另造轮子，不直接复用已有的 nginx，而在 nginx 前或 nginx 后添加一层 API Gateway。反正只要添加在应用服务器之前就行了。如果放在 nginx 之前似乎有些不合理，感觉会拖慢 nginx 的速度，毕竟挡在 nginx 前的东西需要具有 nginx 同等级或在其之上的抗并发能力。那么就应当放在 nginx 之后了，这里也面临一个问题。比如 nginx 和 backend 之间的通信协议有可能不是 http，例如 uwsgi。所以 API Gateway 必须也要支持这种协议。Spring Cloud 的 Zuul 便是在 nginx 之后的一层，不过协议还是 http。当然不管在 nginx 前还是后，都需要提供对并发连接的支持，一个简单粗暴的方法是横向扩展实例。

### API Gateway 的要求

1) 性能，上面已经提到过了
2) 能够进行热更新，因为需求变化时，资源映射关系也要随之变化
3) 易于扩展
4) 失败处理/熔断 参考 [熔断器](https://eacdy.gitbooks.io/spring-cloud-book/content/2%20Spring%20Cloud/2.4%20%E7%86%94%E6%96%AD%E5%99%A8.html)


### Reference
[企业级API网关设计](https://mp.weixin.qq.com/s/RuN5RfQfksQZRPACloqHEq)  
[微服务实战（二）：使用API Gateway](http://dockone.io/article/482)

    