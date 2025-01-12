+++
title = "2003, can't connect to MYSQL server on ... (99)"
summary = ""
description = ""
categories = ["MySQL"]
tags = ["connect",  "MySQL", "syscall", "kernel", "TCP", "port"]
date = 2025-01-12T13:09:11+09:00
draft = false

+++



## 问题



最近遇到一个报错，如下

```
(2003, "Can't connect to MYSQL server on 'mysql-server:3306 (99)")
```



这个报错格式中最后一个括号的内容是系统的 `errno`。我们可以查看 `errno.h` 或者通过 `errno` 命令来得知 99 背后的含义。`errno` 命令可以通过如下方式安装:

```
$ apt install moreutils
$ pacman -S moreutils
```



执行命令得到 99 对应 `EADDRNOTAVAIL`

```
$ errno 99
EADDRNOTAVAIL 99 Cannot assign requested address
```



这个错误码在很多 SYSCALL 都会用到，比如 `bind(2)`。结合错误信息可以推断本次是 `connect(2)` 返回的，执行 `man 2 connect`

>        EADDRNOTAVAIL
>               (Internet  domain sockets) The socket referred to by sockfd had not previously been bound to an address and, upon attempting to bind it to an ephemeral
>               port, it was determined that all port numbers in the ephemeral port range are currently  in  use.   See  the  discussion  of  /proc/sys/net/ipv4/ip_lo‐
>               cal_port_range in ip(7).



通过 `ss` 命令查看连接状态，当时的确出现 2 万多指向同一个 MySQL 的 `TIME_WAIT` 连接，将本地端口耗尽了。根本原因是一个 Django 的服务本身没有配置数据库连接池，而且短时间 RPS 激增，每个请求都在尝试新建 MySQL 连接





## `connect(2)` 源码

Linux kernel 代码为 v6.12.6



调用栈如下

1. `tcp_v4_connect` https://elixir.bootlin.com/linux/v6.12.6/source/net/ipv4/tcp_ipv4.c#L218

2. `inet_hash_connect` https://elixir.bootlin.com/linux/v6.12.6/source/net/ipv4/inet_hashtables.c#L1166
3. `__inet_hash_connect` https://elixir.bootlin.com/linux/v6.12.6/source/net/ipv4/inet_hashtables.c#L994



这里简单说一下这个 `__inet_hash_connect` 的逻辑。整体可以分为三部分，第一部分是初始化各种变量

1. 通过 `inet_sk_get_local_port_range` 用于获取本地端口范围 `(low, high)`
2. 确定 `step` 步长，计算系统配置的可用端口总数 `remaining`
3. 加入一定随机性，计算出本次的`offset`



第二部分是 `other_parity_scan` 中的逻辑

1. 按照指定 `step` 增长 `port`
2. 对于当前的 `port`， 检查 `inet_is_local_reserved_port` ，需要跳过本地保留端口
3. 使用 `inet_bind_bucket_for_each`  检查当前 `port` 是否被使用
   1. 如果未使用则跳转到 `ok`
   2. 如果被使用，需要确保不能开启 `SO_REUSEADDR` 和 `SO_REUSEPORT` 
      1. 若开启则跳转 `next_port`
      2. 若未开启，需要进一步通过 `check_established` 检查四元组是否冲突。如果四元组冲突，则跳转 `next_port`
4.  `next_port` 中检查是否还有剩余可以尝试的端口，如果没有则返回 `EADDRNOTAVAIL`。否则跳转 `other_parity_scan` 继续尝试



特别的 `check_established` 中判断过 `TCP_TIME_WAIT` 的重用逻辑( `net.ipv4.tcp_tw_reuse`)。核心函数是 `tcp_twsk_unique`

- https://elixir.bootlin.com/linux/v6.12.6/source/net/ipv4/inet_hashtables.c#L568

- https://elixir.bootlin.com/linux/v6.12.6/source/net/ipv4/tcp_ipv4.c#L116



第三部分便是 `ok` 对应的逻辑

1. bind socket

2. 修改端口随机性相关的值，优化下一次端口起点



## Linux 端口选择策略



另一个需要注意的是 4.6 之后，端口通过奇数偶数分组。`connect` 优先使用偶数端口，`bind` 使用奇数端口。



关于端口选择策略，可以参考这三个 commit



https://github.com/torvalds/linux/commit/07f4c90062f8fc7c8c26f8f95324cbe8fa3145a5

> **tcp/dccp: 尝试在 connect() 中避免耗尽 `ip_local_port_range`**
>
> 在繁忙服务器上，一个长期存在的问题是 TCP 可用端口范围（`/proc/sys/net/ipv4/ip_local_port_range`）较小，以及 `connect()` 系统调用中默认的顺序分配源端口策略。
>
> 如果一台主机有大量活跃的 TCP 会话，很可能所有端口都被至少一个连接占用，导致后续的 `bind(0)` 操作失败，或者需要扫描很大一部分端口空间才能找到可用的端口。
>
> 在这个补丁中，我更改了 `__inet_hash_connect()` 中的起始点，使其优先考虑偶数 [1] 端口，将奇数端口保留给使用 `bind()` 的用户。
>
> 我们仍然执行顺序搜索，因此无法保证完全避免冲突，但如果 `connect()` 的目标非常不同，最终结果是我们可以为 `bind()` 留下更多的可用端口，并且在整个端口范围内更均匀地分布。这降低了 `connect()` 和 `bind()` 操作寻找空闲端口的时间。
>
> 此策略只有在 `/proc/sys/net/ipv4/ip_local_port_range` 为偶数范围时才有效，即起始值和结束值具有不同的奇偶性。
>
> 因此，默认的 `/proc/sys/net/ipv4/ip_local_port_range` 被修改为 **32768 - 60999**（而不是之前的 32768 - 61000）。
>
> 该更改不会影响安全性，仅可能对一些较差的哈希方案产生影响。
>
> **[1]**：奇偶属性取决于 `ip_local_port_range` 的起始和结束值的奇偶性。



https://github.com/torvalds/linux/commit/1580ab63fc9a03593072cc5656167a75c4f1d173

> 在提交 **07f4c90**（"tcp/dccp: try to not exhaust ip_local_port_range in connect()"）中，我引入了一个非常简单的启发式方法，以便更好地使用偶数端口，并为使用 `bind()` 的用户保留更多可用的端口槽。
>
> 该方法效果不错，但在一台典型的服务器上，当有超过 200,000 个 TCP 会话时，大约 30,000 个临时端口仍然是稀缺资源。
>
> 因此，我选择进一步优化：优先检查所有偶数端口，如果没有可用端口，则回退到奇数端口。
>
> 一个配套的补丁对 `bind()` 做了类似的改动，但方向相反（即优先使用奇数端口）。
>
> 在繁忙的服务器上，我观察到执行时间可达 30 毫秒，因此在遍历整个哈希桶时，我不再全程禁用 BH（Bottom Halves 中断），而是仅在每个哈希桶上禁用。此外，我调用了 `cond_resched()`，以便对其他任务更友好。





https://github.com/torvalds/linux/commit/207184853dbdb62d8b02c7a141d3297e94e33451

> **tcp/dccp: 在 connect() 时更改源端口选择逻辑**
>
> 在提交 **1580ab6**（"tcp/dccp: better use of ephemeral ports in connect()"）中，我们引入了一种启发式方法，将 `connect()` 优先使用偶数端口，而 `bind()` 优先使用奇数端口。
>
> 这种方式很好，因为不需要对应用程序进行任何改动。
>
> 但是，当所有偶数端口都已被占用，并且监听器很少但活跃连接很多时，这种方法增加了成本。
>
> 从那时起，引入了 `IP_LOCAL_PORT_RANGE`，允许应用程序根据需要自由划分临时端口范围。
>
> 此补丁扩展了这一思路：如果在 `accept()` 之前在套接字上设置了 `IP_LOCAL_PORT_RANGE`，那么端口选择将不再偏向偶数端口。
>
> 这意味着 `connect()` 可以更快地找到合适的源端口，并且应用程序可以在 `connect()` 和 `bind()` 用户之间进行不同的端口划分。
>
> 此外，这应该可以为 RSS（接收端队列，Receive Side Scaling）使用的 Toeplitz 哈希函数提供更多的熵：以往优先使用偶数端口浪费了 16 位源端口中的 1 位。



## Reference

- [从零开始实现单机百万tcp连接](https://kernel.guide/article/%E4%BB%8E%E9%9B%B6%E5%BC%80%E5%A7%8B%E5%AE%9E%E7%8E%B0%E5%8D%95%E6%9C%BA%E7%99%BE%E4%B8%87tcp%E8%BF%9E%E6%8E%A5)
- [[内核源码] 网络协议栈 - connect (tcp)](https://wenfh2020.com/2021/08/07/linux-kernel-connect/)
- [[内核源码] 网络协议栈 - bind (tcp)](https://wenfh2020.com/2021/07/17/kernel-bind/)

- https://github.com/torvalds/linux/commit/da5e36308d9f7151845018369148201a5d28b46d
