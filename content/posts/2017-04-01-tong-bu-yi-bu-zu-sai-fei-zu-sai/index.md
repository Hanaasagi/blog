
+++
title = "同步 异步 阻塞 非阻塞"
summary = ''
description = ""
categories = []
tags = []
date = 2017-04-01T07:27:33+08:00
draft = false
+++

本篇文章试图理清同步、异步、阻塞，非阻塞之间的关系，并着重探讨异步模型

下面是一个比较正式的解释：

1. 同步/异步

同步是指用户发起 IO 请求后需要等待或者轮询内核IO操作完成后才能继续执行；而异步是指用户线程发起IO请求后仍继续执行，当内核IO操作完成后会通知用户线程，或者调用用户线程注册的回调函数

2. 阻塞/非阻塞

用户线程调用内核IO操作的方式：阻塞是指IO操作需要彻底完成后才返回到用户空间；而非阻塞是指IO操作被调用后立即返回给用户一个状态值，无需等到IO操作彻底完成。

好吧，上面的概念还是十分的模糊，傻逼作者用自己的话来概括一下

同步/异步是消息通信机制层面的概念：同步为调用者主动地获取结果，而异步则是被调用者来通知任务已经完成并告诉调用者结果的存储位置  
阻塞/非阻塞是调用者在获得结果之前的状态：阻塞为调用者在结果返回之前被挂起，而非阻塞则是可以继续执行下面的程序流

这几个概念可以组合成 4 种情况：

同步阻塞：最常见的模式，比如我们 IO 操作，或者调用一个执行 sleep 的函数  
同步非阻塞：Linux 下的 `select`、`poll`、`epoll`  
异步阻塞：这个见的比较少。不过可以举个简单的例子：异步模式下，任务执行有时必须满足先后的顺序，具体而言便是下一步操作需要上一步异步调用的结果，也是会产生阻塞  
异步非阻塞：这个也比较常见比如 NodeJS  

下面傻逼作者以 socket IO 中的 read 来举例。我们都知道 IO 操作是要进行用户态和内核态切换的。此过程涉及到 Kernel 和调用这个 IO 的 Process/Thread。一个 read 操作在 Kernel 中的状态可以分为如下的两个阶段：

1 等待数据准备
2 将数据从 Kernel 拷贝到 Process 中

那么这四种情况下，read 操作有什么不同呢？可以用下面几个图来表示

> 引用自《UNIX网络编程：卷一》第六章——I/O复用

同步阻塞模型

```
============================= Blocking IO ======================================

                    Application                   Kernel
                  --------------              --------------
                  |            |              |            |
              ----| recvfrom() | ===========> | no datagram|----
              |   |            | system call  |   ready    |   |
              |   |            |              |            |   |
              |   --------------              --------------   | wait for data
              |                                     ||         |
              |                                     \/         |
              |                               --------------   |
process blocks|                               |            |----
in call to    |                               |  datagram  |
recvfrom      |                               |  ready and |
              |                               |  copy      |----
              |                               --------------   |
              |                                     ||         | copy data
              |                                     \/         | from kernel
              |   --------------              --------------   | to user
              |   |            |              |            |   |
              |   | process    | <=========== |    copy    |----                   
              ----| datagram   |    return    |  complete  |
                  |            |              |            |
                  --------------              --------------

================================================================================
```

同步非阻塞模型

```
============================= Nonblocking IO ===================================

                    Application                   Kernel
                  --------------              --------------
                  |            | system call  |            |
              ----| recvfrom() | ===========> | no datagram|----
              |   |            | <=========== |   ready    |   |
              |   |            | EWOULDBLOCK  |            |   |
              |   --------------              --------------   |
              |                                                |
              |   --------------              --------------   |
              |   |            | system call  |            |   | wait for data
              |   | recvfrom() | ===========> | no datagram|   |
              |   |            | <=========== |   ready    |   |
              |   |            | EWOULDBLOCK  |            |   |
process       |   --------------              --------------   |  
repeatedly    |                                                |
calls recvfrom|   --------------              --------------   |
waiting for an|   |            | system call  |            |   |
OK return     |   | recvfrom() | ===========> | datagram   |----
              |   |            |              | ready and  |
              |   |            |              | copy       |----                  
              |   --------------              --------------   |
              |                                     ||         | copy data
              |                                     \/         | from kernel
              |   --------------              --------------   | to user
              |   |            |              |            |   |
              |   | process    | <=========== |    copy    |----                   
              ----| datagram   |    return    |  complete  |
                  |            |      OK      |            |
                  --------------              --------------

================================================================================
```

可以看出来阻塞和非阻塞的不同之处是 Process 在等待调用结果（消息，返回值）时的状态。

而异步非阻塞模型是下面这样的

```
============================= Asynchronous IO ==================================

                    Application                   Kernel
                  --------------              --------------
                  |            | system call  |            |
              ----| aio_read() | ===========> | no datagram|----
              |   |            | <=========== |   ready    |   |
              |   |            |    return    |            |   |
              |   --------------              --------------   | wait for data
              |                                     ||         |
              |                                     \/         |
              |                               --------------   |
  process     |                               |            |----
  continues   |                               |  datagram  |
  executing   |                               |  ready and |
              |                               |  copy      |----
              |                               --------------   |
              |                                     ||         | copy data
              |                                     \/         | from kernel
              |   --------------              --------------   | to user
              |   | signal     |deliver signal|            |   |
              |   | handler    | <=========== |    copy    |----                   
              ----| process    | specified in |  complete  |
                  | datagram   |   aio_read   |            |
                  --------------              --------------

================================================================================
```

可以看出并不是 Process 主动的去获取 `recvfrom` 的结果，而是继续执行下面的代码。`recvfrom` 执行完成后系统会通过信号来通知 Process
异步编程遇到耗时的操作(IO)，不会去等待，注册一个 callback 便可以去干其他的任务了，提高了 CPU 的利用率。但同时也给程序员带来了麻烦。因为人的思维习惯同步，异步编程正如同其名字一样，没有固定的执行顺序。回调链诚然是有执行顺序的，但是我们很难保证在回调链中的某一节会依赖其他链中结果。而且回调代码在每次执行时顺序都不相同。

当然，上面列举的几个缺点，是针对于注册回调函数形式的异步模型。

异步模型还有如下几种：

1. 返回一个 promise 或者 future 的
比如 Python 的 concurrent， 底部使用的还是进程/线程。将操作交给线程/进程处理，先临时返回一个 future 对象。然后可以通过 future 对象来获取结果

2. 协程

这里具体说一说 回调形式的异步和协程式的异步的区别，以及为什么协程是比较优势的

先来看看回调函数的这种，首先我们准备一个回显我们发送信息的 Socket Server，简称为 echoServer 好了

```Python
# echo_server.py
import socket
import sys

host = '127.0.0.1'
data_payload = 2048
backlog = 5


def echo_server(port):
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    # s.setblocking(0)
    server_address = (host, port)
    print 'starting'
    s.bind(server_address)
    s.listen(backlog)
    while True:
        print 'waiting to recvive'
        try:
            # no blocking will raise a error
            client, address = s.accept()
            data = client.recv(data_payload)
            if data:
                print data
                client.send(data)
            client.close()
        except KeyboardInterrupt:
            break


if __name__ == '__main__':
    echo_server(int(sys.argv[1]))
```

然后我们去实现一个 client

```Python
# async_client.py
import select


class IOLoop(object):

    def __init__(self):
        self.task_list = []
        self.fd_map = {}

    def run(self):
        for func, args in self.task_list:
            func(*args)
        while self.fd_map != {}:
            try:
                readable, writeable, exceptional = select.select(self.fd_map.keys(), [], []， 0.1)
                for sock in readable:
                    data = sock.recv(1024)
                    self.fd_map[sock](sock, data)
                    del self.fd_map[sock]
            except KeyboardInterrupt:
                break

    def add_task(self, fd, func, args, callback):
        self.task_list.append((func, args))
        self.fd_map[fd] = callback


if __name__ == '__main__':
    import socket
    loop = IOLoop()
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    server_address = ('127.0.0.1', 8097)
    s.connect(server_address)

    def callback(sock, data):
        print('echo ' + data)
        sock.close()

    loop.add_task(s, s.send, ('Hello World',), callback)
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    server_address = ('127.0.0.1', 8098)
    s.connect(server_address)
    loop.add_task(s, s.send, ('And Goodbye',), callback)
    loop.run()

```

上面通过 select 轮询 fd 来触发回调事件， select 是 IO 多路复用模型。它是同步非阻塞的，但通过这个我们在上层实现了异步。*其实异步/同步/阻塞/非阻塞在不同的层面思考是不同的。比如 select 是一个轮询，在未指定 timeout 时，它会一直查看 fd 是否可读/写。从用户代码的层面来看当前进程已经阻塞了*

开启两个 server 分别监听 8097 8098 端口。多次运行 async_client.py 会发现会有两种输出

```
echo Hello World
echo And Goodbye

echo And Goodbye
echo Hello World
```

另外因为傻逼作者将回调函数放在了事件循环中，所以如果回调函数阻塞，则整个事件循环也被会阻塞。一种更好的想法是 Event Loop 为单独的一个线程，执行回调函数也为一个单独的线程，然后两者通过 queue 连接

```
-----------              --------------
|         |              |            |
|  main   | register fd  | Event Loop |
| thread  |  ========>   |   thread   |
|         | add callback |            |
-----------              --------------
                               ||
                               \/  Queue
                         --------------
                         |            |
                         |  execute   |
                         |            |
                         |            |
                         --------------
```

再来看看协程的形式。  
协程的核心思想是控制流的主动让出和恢复，一般是通过栈来实现。其起源并不是为了解决异步编程中的 callback hell 问题。最早的协程是用来做词法分析，通过一个生产者消费者模型，一个去读，一个去解析。因为使用协程时生产者消费者能够各自保存上下文(状态)，代码写起来会比较方便。然后人们又发现协程的种种好处，比如它始终在用户空间内，不存在线程的系统开销。用户可以决定什么时候换出什么时候换入等等。

那么让我们使用协程模型用同步的写法去写刚才的异步 demo

```Python
import select
import socket


class IOLoop(object):

    def __init__(self):
        self.task_list = []
        self.fd_map = {}

    def add_task(self, func, args):
        self.task_list.append((func, args))

    def run(self):
        todo_count = len(self.task_list)
        for task, args in self.task_list:
            coroutine = task(*args)
            func, msg = next(coroutine)
            self.fd_map[func.__self__] = coroutine
            func(msg)
        while todo_count > 0:
            try:
                readable, writeable, exceptional = select.select(self.fd_map.keys(), [], [], 0.1)
                for sock in readable:
                    data = sock.recv(1024)
                    try:
                        self.fd_map[sock].send(data)
                    except StopIteration:
                        del self.fd_map[sock]
                        todo_count -= 1
                        break
            except KeyboardInterrupt:
                break

def send_msg(addr, port, msg):
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    server_address = (addr, port)
    s.connect(server_address)
    result = yield (s.send, msg)
    print(result)
    s.close()

if __name__ == '__main__':
    loop = IOLoop()
    loop.add_task(send_msg, ('127.0.0.1', 8097, 'Hello world'))
    loop.add_task(send_msg, ('127.0.0.1', 8098, 'And GoodBye'))
    loop.run()
```

如果你用过协程框架的话，你会觉得这个例子比较古怪， `yield (s.send, msg)` 这是什么玩意。协程框架里不都是 `yield s.send(msg)` 就是了么。这是因为没有 hack 掉 socket 类的方法，使用原有的 `send` 会直接执行，然后 `yield` 其返回值。
如果你想更深一步的了解协程，可以看看 Tornado 的源代码。这也是傻逼作者这个月的计划之一。

### Reference
[怎样理解阻塞非阻塞与同步异步的区别？](https://www.zhihu.com/question/19732473)  
[高性能IO模型浅析](http://www.cnblogs.com/fanzhidongyzby/p/4098546.html)  
[大话同步/异步、阻塞/非阻塞](https://ring0.me/2014/11/sync-async-blocked/)  
[同步，异步，阻塞，非阻塞等关系轻松理解](https://github.com/calidion/calidion.github.io/issues/40)

    