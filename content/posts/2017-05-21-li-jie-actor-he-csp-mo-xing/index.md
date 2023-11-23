
+++
title = "理解 Actor 和 CSP 模型"
summary = ''
description = ""
categories = []
tags = []
date = 2017-05-21T12:39:00+08:00
draft = false
+++

*本文将讲述并发编程的两种模型 Actor 与 CSP;
因为蠢作者并未有过这方面的实际 project，所以本文大概只是纸上谈兵*

先引用一句话
> Use locks and shared memory to shoot yourself in the foot in parallel. (From https://wiki.python.org/moin/Concurrency/Patterns)

再丢一句
> Do not communicate by sharing memory; instead, share memory by communicating. (From Effective Go)

### 缘由以及很大一段题外话

蠢作者第一次接触到并发模型这种概念是在 [七周七并发模型](http://www.ituring.com.cn/book/1649) 这本书中。不过先扯点题外话吧，来一个再常见不过的问题，并发和并行有什么区别？

蠢作者所在学校的操作系统教材 计算机操作系统(第四版) 中第 13 页对这两个概念进行了区分：

>并行性和并发性是既相似又有区别的两个概念。并行性是指两个或多个事件在同一时刻发生。而并发性是指两个或多个事件在同一时间间隔内发生。

可能这也是大部分人的答案，或者有人还会总结出并行一定并发，并发不一定并行，单核计算机只能并发执行这种经过一番推敲的答案。这个答案无疑是正确的，但却也是一个不够完美的答案。因为它只是从任务层级(task-level)上去描述这两个概念，如果你了解计算机偏向硬件方面的知识的话，
你可能会听过并行加法器这种组件。其实现代计算机在不同层次上都使用了并行技术。

下面引用自 [七周七并发模型](http://www.ituring.com.cn/book/1649)

>位级(bit-level)并行
>
>为什么32位计算机的运行速度比8位计算机更快？因为并行。对于两个32位数的加法，8位计算机必须进行多次8位计算，而32位计算机可以一步完成，即并行地处理32位数的4字节。
>计算机的发展经历了8位、16位、32位，现在正处于64位时代。然而由位升级带来的性能改善是存在瓶颈的，这也正是短期内我们无法步入128位时代的原因。
>
>指令级(instruction-level)并行
>
>现代CPU的并行度很高，其中使用的技术包括流水线、乱序执行和猜测执行等。
>程序员通常可以不关心处理器内部并行的细节，因为尽管处理器内部的并行度很高，但是经过精心设计，从外部看上去所有处理都像是串行的。
>而这种“看上去像串行”的设计逐渐变得不适用。处理器的设计者们为单核提升速度变得越来越困难。进入多核时代，我们必须面对的情况是：无论是表面上还是实质上，指令都不再串行执行了。我们将在2.2节的“内存可见性”部分展开讨论。
>
>数据级(data)并行  
>
>数据级并行(也称为“单指令多数据”，SIMD)架构，可以并行地在大量数据上施加同一操作。这并不适合解决所有问题，但在适合的场景却可以大展身手。
>图像处理就是一种适合进行数据级并行的场景。比如，为了增加图片亮度就需要增加每一个像素的亮度。现代GPU(图形处理器)也因图像处理的特点而演化成了极其强大的数据并行处理器。
>
>任务级(task-level)并行
>
>这里略掉吧

所以即使是单任务的情况下也是会存在并行的，广义上的并行和并发不存在子集关系。并发偏向于逻辑，并行更偏向于物理。

回到正题，传统并发有什么不好？

我想如果你写过多线程，你可能会知道线程安全问题，然后去用锁，然后去查看是否有死锁/活锁；如果你选择了多进程，那么需要考虑进程通信问题。总之当你想要用并发去解决问题，反而会引入另一些问题......

并发模型并没有改变依赖多线程/多进程的实质，只是提供了一种避免麻烦的手段。

有了以上准备，开始正题

### Actor 模型

>Actor 模型是一种适用性非常好的通用并发编程模型。它可以应用于共享内存架构和分布式内存架构，适合解决地理分布型的问题。同时它还能提供很好的容错性。  

我们都知道在(线程)并发中，共享的可变状态会引入诸多麻烦。对此有许多解决方法，比如常见的锁，事务，函数式编程中则直接使用了不可变状态。而 Actor 模型则允许可变状态，只是只通过消息传递的方式来进行状态的改变。Actor 模型由一个个称为 Actor 的执行体和 mailbox 组成。用户将消息发送给 Actor，实际上就是将消息放入一个队列中， 然后将其转交给处理被接受消息的一个内部线程。消息让 Actor 之间解耦。一个 Actor 收到其他 Actor 的消息后，会做出不同的行为，还可能会给其他 Actor 发送更进一步的消息。  

下面代码来自 [Python3 CookBook](http://python3-cookbook.readthedocs.io/zh_CN/latest/c12/p10_defining_an_actor_task.html)  

*actor.py*

```python
from queue import Queue
from threading import Thread, Event


class ActorExit(Exception):
  	pass


class Actor(object):

	  def __init__(self):
	      self._mailbox = Queue()

	  def send(self, msg):
	      self._mailbox.put(msg)

	  def recv(self):
	      msg = self._mailbox.get()
	      if msg is ActorExit:
	          raise ActorExit()
	      return msg

	  def close(self):
	      self.send(ActorExit)

	  def start(self):
	      self._terminated = Event()
	      t = Thread(target=self._bootstrap)
	      t.daemon = True
	      t.start()

	  def _bootstrap(self):
	      try:
	          self.run()
	      except ActorExit:
	          pass
	      finally:
	          self._terminated.set()

	  def join(self):
	      self._terminated.wait()

	  def run(self):
	      '''
	      Run method to be implemented by the user
	      '''
	      while True:
	          msg = self.recv()
```

*demo.py*

```python
from actor import Actor
from threading import Event


class Result(object):

    def __init__(self):
        self._evt = Event()
        self._result = None

    def set_result(self, value):
        self._result = value
        self._evt.set()

    @property
    def result(self):
        self._evt.wait()
        return self._result


class Worker(Actor):

    def submit(self, func, *args, **kwargs):
        r = Result()
        self.send((func, args, kwargs, r))
        return r

    def run(self):
        while True:
            func, args, kwargs, r = self.recv()
            r.set_result(func(*args, **kwargs))


if __name__ == '__main__':
    worker = Worker()
    worker.start()
    r = worker.submit(pow, 2, 4)
    print('it will not block')
    print(r.result)
```

上面只是一个简单的示例
Actor 的 send 方法可以更改为在套接字上传输数据或者通过消息队列作为中间层 比如 RabbitMQ 来发送。

### CSP 模型

CSP 为 [communicating sequential processes] 的缩写，有趣的是它还是代数演算，见[wiki](https://en.wikipedia.org/wiki/Communicating_sequential_processes)。与 Actor 模型类似，CSP 模型也是由独立的、并发执行的实体所组成，实体之间也是通过发送消息进行通信。但两种模型的差别是：CSP 模型不关注发送消息的实体，而是关注发送消息时使用的 channel。这个区别之后会再次提到。  

写出来大概是下面这样，摘自 Taipei Pycon 2016

```python
from threading import Thread
from queue import Queue
from time import sleep


def run_thread(func, *args, **kwargs):
    t = Thread(target=func, args=args, kwargs=kwargs)
    t.start()
    return t

def work_for(channel):
    while True:
        cmd = channel.get()
        if cmd == 'return':
            return

        print('Start working on {} ...'.format(cmd))
        sleep(1)
        print('Done')

if __name__ == '__main__':
    channel = Queue()
    for i in range(3):
        run_thread(work_for, channel)

    for i in range(6):
        channel.put(i)

    for i in range(3):
        channel.put('return')
```
WTF，这个好像和平时利用 `Queue` 的多线程没有什么区别啊？的确，我们无意之间已经在使用 CSP 了。

再来看看 Golang 的，Golang 自身支持了CSP，通过一个叫 goroutine 的作为执行实体。实际上就是协程。

Go 版本代码  

```Go
package main

import "fmt"

var ch = make(chan string)

func message(){
    msg := <- ch
    fmt.Println(msg)    
}

func main(){
    go message()
    ch <- "Hello World."
}
```

所以，你看懂 Actor 和 CSP 两者的区别了么？

个人认为，这两者是十分相近的概念，它们都着眼于消息传递，而且都使用了 channel/mailbox 这样的信道(队列)和 Actor/goroutine 这样的执行实体。只不过 Actor 模型将执行实体和 mailbox 进行了耦合，一个 Actor 用一个 mailbox；而 CSP 则可以绑定多个 channel(类似发布者/订阅者模型)。另外有人说，向 Actor 发送消息时是非阻塞操作，而 CSP 如果执行体正忙则会产生阻塞。这点我更觉得奇怪了，感觉有点牵强，不太清楚这种设计的理由。只是有一点是可以肯定的：CSP 中的 channel 通常是匿名的, 即任务放进 channel 之后你并不需要知道是哪个 channel 在执行任务，而 Actor 是有"身份"的，你可以明确的知道哪个 Actor 在执行任务。

### Reference
[Golang CSP并发模型](http://www.jianshu.com/p/36e246c6153d)  
[七周七并发模型](http://www.ituring.com.cn/book/1649)  
[并发编程：Actors模型和CSP模型](http://www.importnew.com/24226.html)
    