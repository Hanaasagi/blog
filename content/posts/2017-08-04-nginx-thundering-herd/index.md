
+++
title = "nginx  thundering herd"
summary = ''
description = ""
categories = []
tags = []
date = 2017-08-04T14:32:03+08:00
draft = false
+++

>In computer science, the thundering herd problem occurs when a large number of processes waiting for an event are awoken when that event occurs, but only one process is able to proceed at a time. After the processes wake up, they all demand the resource and a decision must be made as to which process can continue. After the decision is made, the remaining processes are put back to sleep, only to all wake up again to request access to the resource.

>This occurs repeatedly, until there are no more processes to be woken up. Because all the processes use system resources upon waking, it is more efficient if only one process was woken up at a time.

>This may render the computer unusable, but it can also be used as a technique if there is no other way to decide which process should continue (for example when programming with semaphores).

大致就是说，多个进程在等待某个事件。当此事件发生时，所有进程被唤醒，但是最终只有一个会对此事件进行处理。Linux accpet 已经不存在 thundering herd 问题了，但是 epoll 中还是存在

```
import os
import socket
import select


sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
sock.bind(('127.0.0.1', 8833))
sock.listen(32)
sock.setblocking(0)
sock.setsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY, 1)
epoll = select.epoll()
epoll.register(sock.fileno(), select.EPOLLIN)

child = []
for i in range(16):
    pid = os.fork()
    if pid == 0:
        while True:
            events = epoll.poll()
            print('wakeup', os.getpid())
            for fileno, event in events:
                epoll.modify(fileno, 0)
    else:
        child.append(pid)

for pid in child:
    os.waitpid(pid, 0)

epoll.unregister(sock.fileno())
epoll.close()
sock.close()
```

nginx 为了避免这种问题，使用了自旋锁

附一篇详细描述的 [文章](http://pureage.info/2015/12/22/thundering-herd.html)
    