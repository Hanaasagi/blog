+++
title = '新人也能懂的调试方法 01 - 通过 syscall 进行定位'
summary = ""
description = ""
categories = ["debug"]
tags = []
date = 2023-10-24T18:22:38+09:00
draft = false

+++

## 序言

最近手边正好积累了一堆 Bugs，而且代码都是开源的，很适合作为题材用于写 Blog。所以准备写一个系列讲讲如何 debug。其实程序员应该对于 debug 这件事情不陌生，毕竟 bug 这个东西应该每天都会遇到的，没有什么软件可以说自己是 100% 没有 bug 的。写这个系列的初衷是之前看到过很多人对于 bug 摸不着头脑，尤其是对于别人写的代码，或者自己本地跑得好好的，上线后结果出了问题这种。这个也不能责怪谁，程序员本身就是解决问题的。但是每个人应该都希望能够快速的定位，解决问题，尤其是生产发生的问题。这里分享一下经验，不过应该看完了也不会有太大的收获，因为这个是实践型的，需要大量实践积累才能逐渐掌握感觉



### 通过分层进行 debug

软件天生是分层的。通过固定每一层的输入和外界环境，我们可以获得一个最小的测试单元，用以保证这一层的正确性。层与层想拼接，构筑成了一个复杂的软件。这个层可以是普通的函数调用，可以是一个 lib，可以是一个单独的服务，或者计算机网络中的网络层，IP 层。通过分层进行 debug，这思想应该每一个人都有用过，比如在不同的函数中通过 `print` 的方式去 trace 一个结果，或者在一个循环中打印一些变量的值。不过大家都熟悉白盒情况下的，如果是一个完全的黑盒呢，该如何通过分层来定位问题呢？



下面看一个示例 [oven-sh/bun#5315](https://github.com/oven-sh/bun/issues/5315)



### 分析问题

类似于古典推理小说中的，whodunit，whydunit 和 howdunit。对于一个陌生的问题，一般需要掌握尽可能详细的信息，包括但不限于

- 什么时候出现的，比如是否是最近一次发版后有了这个问题
- 是否可以 100% 复现，可以通过本地，或者测试环境来验证
- 最小的 repro code
- ……



根据 issue 中的描述，可以得知这是在 container 中发生的。那么我们这里可以反着想一下，非 contianer 环境中是否可以复现这个问题呢？这里是用的**对比**的思想。简单的找一台 Linux Host，然后运行上面的代码，发现是无法复现这个问题的。但是我们不能直接得出问题在 container，我们还需要尝试一下是否是 `debian:sid-slim` 本身有问题。这个可能性是不大的，因为这么知名的东西，一般不会出问题，优先级可以放后。不过我们可以尝试一下 `oven/bun` 这个镜像是否有问题，试了一下是同样存在问题的

接下来我们是可以通过**二分法** filter branch 的方法来确定一个导致这个问题的 commit。但是这思路的限制很大，一是我们无法确定这个一开始是正确的，有可能从头开始这功能就是有 bug 的；二是编译型语言，尤其大型项目，这个操作很耗时间的。



好了，问题的环境信息我们已经理清。接下来需要从代码入手，首先简单的看一下 [detect-port-alt](https://www.npmjs.com/package/detect-port-alt) 这个 npm 的 package 是在干什么。我们确定了 `bunx detect-port-alt 3000` 这条命令是在检查可用的端口。这里你可以通过阅读 pacakge 源代码或者通过系统编程经验来直接明白



**问题可能出在 bind**



如果没有经验，那么需要看一下 detect-port-alt 这个 pacakge 的代码，一共 114 行，可以非常迅速地归纳出如下的最小复现代码

```javaScript
const net = require("net");

const server = new net.Server();

server.on("error", err => {
  console.log(err);
  server.close();
});

server.listen(3000, "localhost", () => {
  port = server.address();
  server.close();
  console.log(port);
});

```

这里可以对比一下 bun 和 nodejs 的执行结果

```
$ bun run server.js
error: Failed to listen at localhost
 code: "EADDRNOTAVAIL"

$ node server.js
{ address: '127.0.0.1', family: 'IPv4', port: 3000 }
```



可以观察到一个非常显著的信息 `EADDRNOTAVAIL`，这个是一个非常显著的 syscall 中的错误码。直接在 System Calls Manual 中进行 grep

>        EADDRNOTAVAIL
>               A nonexistent interface was requested or the requested address was not local.
>
>        EADDRNOTAVAIL
>               (Internet domain sockets) The socket referred to by sockfd had not previously been bound to an address and, upon attempting to bind it to an ephemeral port, it was determined that all port numbers in the ephemeral port range are  currently  in  use.

此错误码可以由 `bind` 和 `connect` 两个 syscall 产生。联系 JavaScript 的代码是一个 `net.Server`，那么一定是 `bind` 失败了。其实通过 `Failed to listen at localhost` 这行也可以进行切入，不过考虑到软件的模块划分，可能直接 grep 到的不一定是最近的地方，还需要多跳转几次。这个可以作为备用的手段。另外考虑到有些人写代码喜欢直接 `try` 非常大的函数，如果里面错误了，都显示一个错误信息，所以个人从 errcode 入手可能更快定位



### 定位问题

既然是 syscall，那么可以轻松通过 `strace` 等工具进行追踪。可以直接看图

![](./2023-09-14_19-50.png)

根据 syscall 结合 `EADDRNOTAVAIL` 来看，一共发生了两次，第一次在 `connect` 第二次在 `bind` 的时候。这里绑定的是 `::1` 是 IPV6 的 loopback 而不是 `127.0.0.1`。nodejs 在这里选择的是 `127.0.0.1` ，那么为什么 bun 选择 `::1` 呢，这是第一个问题



至此我们需要探入黑盒，来翻阅一下 bun 的代码。通过 strace 还可以获得另一个很重要的信息，那就是 syscall 的参数，尤其是各种 Flag。这里很明显有 `SO_REUSEADDR` `SOCK_NONBLOCK` `IPPROTO_TCP` 等。如果是 C 源代码，那么这些名称是可以直接在代码中搜索到的，它们是同名的。如果是其他语言，可能对于这些有封装，不过名字是相似的。那么直接在代码中去搜索对应语言中的 Flag 或者函数名称。我们完成了下层 syscall 往上层应用进行的反向定位

比如 `IPPROTO_TCP` 这个 Flag 在 Bun 中直接被引用到的地方是

```
src/deps/c-ares/src/lib/ares_process.c
1114:     setsockopt(s, IPPROTO_TCP, TCP_NODELAY,

src/deps/c-ares/test/ares-test.cc
184:  setsockopt(tcpfd_, IPPROTO_TCP, TCP_NODELAY,

src/deps/tinycc/win32/include/winapi/winsock2.h
137:#define IPPROTO_TCP 6

src/deps/boringssl/ssl/test/bssl_shim.cc
106:    if (setsockopt(sock, IPPROTO_TCP, TCP_NODELAY,

packages/bun-usockets/src/bsd.c
276:    setsockopt(fd, IPPROTO_TCP, TCP_NODELAY, (void *) &enabled, sizeof(enabled));
283:    setsockopt(fd, IPPROTO_TCP, TCP_CORK, (void *) &enabled, sizeof(int));
```

如果是 Zig 的，那么先找 `std.os.socket` 再找 TCP 的地方，或者找对于 TCPServer 的高级封装

在上面的例子中，可以轻松找到图中 syscall 的位置 https://github.com/oven-sh/bun/blob/92e95c86dd100f167fb4cf8da1db202b5211d2c1/packages/bun-usockets/src/bsd.c#L443-L461

代码如下

```c
// return LIBUS_SOCKET_ERROR or the fd that represents listen socket
// listen both on ipv6 and ipv4
LIBUS_SOCKET_DESCRIPTOR bsd_create_listen_socket(const char *host, int port, int options) {
    struct addrinfo hints, *result;
    memset(&hints, 0, sizeof(struct addrinfo));

    hints.ai_flags = AI_PASSIVE;
    hints.ai_family = AF_UNSPEC;
    hints.ai_socktype = SOCK_STREAM;

    char port_string[16];
    snprintf(port_string, 16, "%d", port);

    if (getaddrinfo(host, port_string, &hints, &result)) {
        return LIBUS_SOCKET_ERROR;
    }

    LIBUS_SOCKET_DESCRIPTOR listenFd = LIBUS_SOCKET_ERROR;
    struct addrinfo *listenAddr;
    for (struct addrinfo *a = result; a && listenFd == LIBUS_SOCKET_ERROR; a = a->ai_next) {
        if (a->ai_family == AF_INET6) {
            listenFd = bsd_create_socket(a->ai_family, a->ai_socktype, a->ai_protocol);
            listenAddr = a;
        }
    }

    for (struct addrinfo *a = result; a && listenFd == LIBUS_SOCKET_ERROR; a = a->ai_next) {
        if (a->ai_family == AF_INET) {
            listenFd = bsd_create_socket(a->ai_family, a->ai_socktype, a->ai_protocol);
            listenAddr = a;
        }
    }

    if (listenFd == LIBUS_SOCKET_ERROR) {
        freeaddrinfo(result);
        return LIBUS_SOCKET_ERROR;
    }

    if (port != 0) {
        /* Otherwise, always enable SO_REUSEPORT and SO_REUSEADDR _unless_ options specify otherwise */
#ifdef _WIN32
        if (options & LIBUS_LISTEN_EXCLUSIVE_PORT) {
            int optval2 = 1;
            setsockopt(listenFd, SOL_SOCKET, SO_EXCLUSIVEADDRUSE, (void *) &optval2, sizeof(optval2));
        } else {
            int optval3 = 1;
            setsockopt(listenFd, SOL_SOCKET, SO_REUSEADDR, (void *) &optval3, sizeof(optval3));
        }
#else
    #if /*defined(__linux) &&*/ defined(SO_REUSEPORT)
        if (!(options & LIBUS_LISTEN_EXCLUSIVE_PORT)) {
            int optval = 1;
            setsockopt(listenFd, SOL_SOCKET, SO_REUSEPORT, (void *) &optval, sizeof(optval));
        }
    #endif
        int enabled = 1;
        setsockopt(listenFd, SOL_SOCKET, SO_REUSEADDR, (void *) &enabled, sizeof(enabled));
#endif

    }

#ifdef IPV6_V6ONLY
    int disabled = 0;
    setsockopt(listenFd, IPPROTO_IPV6, IPV6_V6ONLY, (void *) &disabled, sizeof(disabled));
#endif

    if (bind(listenFd, listenAddr->ai_addr, (socklen_t) listenAddr->ai_addrlen) || listen(listenFd, 512)) {
        bsd_close_socket(listenFd);
        freeaddrinfo(result);
        return LIBUS_SOCKET_ERROR;
    }

    freeaddrinfo(result);
    return listenFd;
}

```

代码已经定位到了，根据代码可以反推为什么这里会去绑定 `::1` 而不是 `127.0.0.1` 。显然这里的两个 `for` 循环的逻辑是有 IPv6 那么就优先使用 IPv6 的，没有 v6 才会使用 IPv4 的地址。地址的来源则是 `getaddrinfo` 函数的返回值。这个函数是 C 库中的，不是 syscall，所以需要找编译的时候是 link 的哪个 C 库，比如 glibc, musl 等。不过我们先来看一下容器内的配置，执行 `ip addr`，这个是没有配置过 IPv6 的

```
1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN group default qlen 1000
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
    inet 127.0.0.1/8 scope host lo
       valid_lft forever preferred_lft forever
```

如果配置过 v6 是下面这个样子

```
1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN group default qlen 1000
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
    inet 127.0.0.1/8 scope host lo
       valid_lft forever preferred_lft forever
    inet6 ::1/128 scope host proto kernel_lo
       valid_lft forever preferred_lft forever

```



那么为什么 `getaddrinfo` 返回了 `::1` 这个地址呢？这里可以继续看 strace 的结果，因为往前几行就是 `getaddrinfo`  内执行过的 syscall

```
openat(AT_FDCWD, "/etc/hosts", O_RDONLY|O_CLOEXEC) = 3
newfstatat(3, "", {st_mode=S_IFREG|0644, st_size=2637, ...}, AT_EMPTY_PATH) = 0
lseek(3, 0, SEEK_SET)                   = 0
```

或者读一遍 `man 3 getaddrinfo`，不懂的时候看文档。或者直接猜有地方写死了，这个地方应该就是 `/etc/hosts`

```
root@f2271f4affd3:/tmp/T# cat /etc/hosts
127.0.0.1       localhost
::1     localhost ip6-localhost ip6-loopback
fe00::0 ip6-localnet
ff00::0 ip6-mcastprefix
ff02::1 ip6-allnodes
ff02::2 ip6-allrouters
172.17.0.2      f2271f4affd3
```

`getaddrinfo` 的源码在这里，有兴趣可以看一下

- https://github.com/lattera/glibc/blob/895ef79e04a953cac1493863bcae29ad85657ee1/sysdeps/posix/getaddrinfo.c#L2292



关于为什么存在 `::1     localhost ip6-localhost ip6-loopback` 这一行，参考 [moby#35954](https://github.com/moby/moby/issues/35954)，是一个系统设计上的兼容性问题，这里不进行展开



这个问题的修复方式取决于你期望的行为是什么，比如 IPv6 失败之后自动尝试一次 IPv4。另外知道了 Bug 的成因，我们也可以直接在 Linux Host 上对这个进行复现

```
# 禁用 lo 的 ipv
$ sysctl net.ipv6.conf.lo.disable_ipv6=1
$ echo "::1 localhost" >> /etc/hosts
```



### 结束

其实这个问题分析起来没有这么复杂的，如果有经验的话，可以直接上手 strace 然后定位。可以这样思考，端口是谁分配给你的，是 kernel，无论你指定端口好还是不指定的随机分配，都需要 kernel 的同意。而 Userland 和 Kernel 的桥梁就是 syscall，虽然 root cause 在 IP 上，但是最后也会查出来相同的结论





