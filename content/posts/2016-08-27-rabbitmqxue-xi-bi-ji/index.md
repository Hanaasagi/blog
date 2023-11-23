
+++
title = "RabbitMQ学习笔记"
summary = ''
description = ""
categories = []
tags = []
date = 2016-08-27T14:01:37+08:00
draft = false
+++

### RabbitMQ是什么

我们都知道redis可以为我们提供高级消息队列服务，这在架构中非常有用。RabbitMQ 也是一种不错的技术选择。
以下介绍引用自[RabbitMQ Quick](https://www.gitbook.com/book/geewu/rabbitmq-quick/details)

>RabbitMQ是实现AMQP（高级消息队列协议）的消息中间件的一种，最初起源于金融系统，用于在分布式系统中存储转发消息，在易用性、扩展性、高可用性等方面表现不俗。
>
>RabbitMQ主要是为了实现系统之间的双向解耦而实现的。当生产者大量产生数据时，消费者无法快速消费，那么需要一个中间层。保存这个数据。
>
>例如一个日志系统，很容易使用RabbitMQ简化工作量，一个Consumer可以进行消息的正常处理，另一个Consumer负责对消息进行日志记录，只要在程序中指定两个Consumer所监听的queue以相同的方式绑定到同一exchange即可，剩下的消息分发工作由RabbitMQ完成。

	单向解耦

	“Producer”--
	           |
	           |----->"RabbitMQ Clusters" ---> “Consumer”
	"Producer"--

	双向解耦（如：RPC）

	“Producer1”-->
	           |
	           |<----->"RabbitMQ Clusters" <---> “Consumer2&Producer2”
	"Consumer1"<--


RabbitMQ工作模型

![](https://www.rabbitmq.com/img/tutorials/intro/hello-world-example-routing.png)
下面根据官方给的几个[demo](https://github.com/rabbitmq/rabbitmq-tutorials/tree/master/python)来具体说说它该如何使用。

### 简单的消息发送
send.py

	#!/usr/bin/env python
	import pika

	connection = pika.BlockingConnection(pika.ConnectionParameters(
	        host='localhost'))
	channel = connection.channel()


	channel.queue_declare(queue='hello')

	channel.basic_publish(exchange='',
	                      routing_key='hello',
	                      body='Hello World!')
	print(" [x] Sent 'Hello World!'")
	connection.close()


receive.py

	#!/usr/bin/env python
	import pika

	connection = pika.BlockingConnection(pika.ConnectionParameters(
	        host='localhost'))
	channel = connection.channel()


	channel.queue_declare(queue='hello')

	def callback(ch, method, properties, body):
	    print(" [x] Received %r" % body)

	channel.basic_consume(callback,
	                      queue='hello',
	                      no_ack=True)

	print(' [*] Waiting for messages. To exit press CTRL+C')
	channel.start_consuming()

#### 代码解析
首先是建立连接

	connection = pika.BlockingConnection(pika.ConnectionParameters(
		        host='localhost'))
	channel = connection.channel()

建立名为hello的消息队列，我们可以看到receive.py中也有同样的语句，这时为了防止send.py未运行导致不存在hello队列，是一种保险的做法。`queue_declare`可以运行很多次，但只有一个队列会被创建。

	channel.queue_declare(queue='hello')

发布消息，exchange指定交换机类型，routing_key指定路由键，body为发送内容的消息体

	channel.basic_publish(exchange='',
	                      routing_key='hello',
	                      body='Hello World!')

接受消息，callback指定接收到消息后的回调,no_ack指定不进行消息确认。
	channel.basic_consume(callback,
	                      queue='hello',
	                      no_ack=True)

### 使用工作队列
new_task.py

	#!/usr/bin/env python
	import pika
	import sys

	connection = pika.BlockingConnection(pika.ConnectionParameters(
	        host='localhost'))
	channel = connection.channel()

	channel.queue_declare(queue='task_queue', durable=True)

	message = ' '.join(sys.argv[1:]) or "Hello World!"
	channel.basic_publish(exchange='',
	                      routing_key='task_queue',
	                      body=message,
	                      properties=pika.BasicProperties(
	                         delivery_mode = 2, # make message persistent
	                      ))
	print(" [x] Sent %r" % message)
	connection.close()

worker.py

	#!/usr/bin/env python
	import pika
	import time

	connection = pika.BlockingConnection(pika.ConnectionParameters(
	        host='localhost'))
	channel = connection.channel()

	channel.queue_declare(queue='task_queue', durable=True)
	print(' [*] Waiting for messages. To exit press CTRL+C')

	def callback(ch, method, properties, body):
	    print(" [x] Received %r" % body)
	    time.sleep(body.count(b'.'))
	    print(" [x] Done")
	    ch.basic_ack(delivery_tag = method.delivery_tag)

	channel.basic_qos(prefetch_count=1)
	channel.basic_consume(callback,
	                      queue='task_queue')

	channel.start_consuming()
#### 代码解析
`durable=True`参数指定队列是持久化的，即工作时会将消息存储到硬盘，方便崩溃或退出时的恢复。

	channel.queue_declare(queue='task_queue', durable=True)
另外消息也要设为持久化

	properties=pika.BasicProperties(
		                         delivery_mode = 2, # make message persistent
		                      ))

平衡分发 ：同一时刻，不要发送超过1条消息给一个worker，直到它已经处理了上一条消息并且作出了响应

	channel.basic_qos(prefetch_count=1)
注意我们移除了`no_ack=True`，并在`callback`中加入了`ch.basic_ack(delivery_tag = method.delivery_tag)`。我们开启了消息确认。消费者会通过一个ack（响应），告诉RabbitMQ已经收到并处理了某条消息，然后RabbitMQ就会释放并删除这条消息。如果消费者（consumer）挂掉了，没有发送响应，RabbitMQ就会认为消息没有被完全处理，然后重新发送给其他消费者（consumer）

### 发布/订阅
emit_log.py

	#!/usr/bin/env python
	import pika
	import sys

	connection = pika.BlockingConnection(pika.ConnectionParameters(
	        host='localhost'))
	channel = connection.channel()

	channel.exchange_declare(exchange='logs',
	                         type='fanout')

	message = ' '.join(sys.argv[1:]) or "info: Hello World!"
	channel.basic_publish(exchange='logs',
	                      routing_key='',
	                      body=message)
	print(" [x] Sent %r" % message)
	connection.close()

receive_logs.py

	#!/usr/bin/env python
	import pika

	connection = pika.BlockingConnection(pika.ConnectionParameters(
	        host='localhost'))
	channel = connection.channel()

	channel.exchange_declare(exchange='logs',
	                         type='fanout')

	result = channel.queue_declare(exclusive=True)
	queue_name = result.method.queue

	channel.queue_bind(exchange='logs',
	                   queue=queue_name)

	print(' [*] Waiting for logs. To exit press CTRL+C')

	def callback(ch, method, properties, body):
	    print(" [x] %r" % body)

	channel.basic_consume(callback,
	                      queue=queue_name,
	                      no_ack=True)

	channel.start_consuming()

#### 代码解析

这里我们使用了交换机(exchange)，以下代码创建了一个名为logs的扇形交换机，扇形交换机可以实现分发一个消息给多个消费者。

	channel.exchange_declare(exchange='logs',
	                         type='fanout')

调用queue_declare的时候，不提供queue参数就可以创建一个随机名称的队列。exclusive指定为True时，当与消费者（consumer）断开连接的时候，这个队列应会被立即删除。

	result = channel.queue_declare(exclusive=True)

获取队列的名称

	queue_name = result.method.queue

将队列与交换机进行绑定

	channel.queue_bind(exchange='logs',
	                   queue=queue_name)


### 路由
emit_log_direct.py

	#!/usr/bin/env python
	import pika
	import sys

	connection = pika.BlockingConnection(pika.ConnectionParameters(
	        host='localhost'))
	channel = connection.channel()

	channel.exchange_declare(exchange='direct_logs',
	                         type='direct')

	severity = sys.argv[1] if len(sys.argv) > 1 else 'info'
	message = ' '.join(sys.argv[2:]) or 'Hello World!'
	channel.basic_publish(exchange='direct_logs',
	                      routing_key=severity,
	                      body=message)
	print(" [x] Sent %r:%r" % (severity, message))
	connection.close()

receive_logs_direct.py

	#!/usr/bin/env python
	import pika
	import sys

	connection = pika.BlockingConnection(pika.ConnectionParameters(
	        host='localhost'))
	channel = connection.channel()

	channel.exchange_declare(exchange='direct_logs',
	                         type='direct')

	result = channel.queue_declare(exclusive=True)
	queue_name = result.method.queue

	severities = sys.argv[1:]
	if not severities:
	    sys.stderr.write("Usage: %s [info] [warning] [error]\n" % sys.argv[0])
	    sys.exit(1)

	for severity in severities:
	    channel.queue_bind(exchange='direct_logs',
	                       queue=queue_name,
	                       routing_key=severity)

	print(' [*] Waiting for logs. To exit press CTRL+C')

	def callback(ch, method, properties, body):
	    print(" [x] %r:%r" % (method.routing_key, body))

	channel.basic_consume(callback,
	                      queue=queue_name,
	                      no_ack=True)

	channel.start_consuming()

#### 代码解析
我们创建了一个直连交换机

	channel.exchange_declare(exchange='direct_logs',
	                         type='direct')

向绑定键为severity的队列发送消息

	channel.basic_publish(exchange='direct_logs',
	                      routing_key=severity,
	                      body=message)
一个队列可以使用多个绑定

	for severity in severities:
	    channel.queue_bind(exchange='direct_logs',
	                       queue=queue_name,
	                       routing_key=severity)

### 主题交换机
主题交换机（topic exchange）的路由键必须是一个由`.`分隔开的词语列表。绑定键也必须拥有同样的格式。主题交换机背后的逻辑跟直连交换机很相似 —— 一个携带着特定路由键的消息会被主题交换机投递给绑定键与之想匹配的队列。其中

    * (星号) 用来表示一个单词.
    # (井号) 用来表示任意数量（零个或多个）单词。

emit_log_topic.py

	#!/usr/bin/env python
	import pika
	import sys

	connection = pika.BlockingConnection(pika.ConnectionParameters(
	        host='localhost'))
	channel = connection.channel()

	channel.exchange_declare(exchange='topic_logs',
	                         type='topic')

	routing_key = sys.argv[1] if len(sys.argv) > 1 else 'anonymous.info'
	message = ' '.join(sys.argv[2:]) or 'Hello World!'
	channel.basic_publish(exchange='topic_logs',
	                      routing_key=routing_key,
	                      body=message)
	print(" [x] Sent %r:%r" % (routing_key, message))
	connection.close()

receive_logs_topic.py

	#!/usr/bin/env python
	import pika
	import sys

	connection = pika.BlockingConnection(pika.ConnectionParameters(
	        host='localhost'))
	channel = connection.channel()

	channel.exchange_declare(exchange='topic_logs',
	                         type='topic')

	result = channel.queue_declare(exclusive=True)
	queue_name = result.method.queue

	binding_keys = sys.argv[1:]
	if not binding_keys:
	    sys.stderr.write("Usage: %s [binding_key]...\n" % sys.argv[0])
	    sys.exit(1)

	for binding_key in binding_keys:
	    channel.queue_bind(exchange='topic_logs',
	                       queue=queue_name,
	                       routing_key=binding_key)

	print(' [*] Waiting for logs. To exit press CTRL+C')

	def callback(ch, method, properties, body):
	    print(" [x] %r:%r" % (method.routing_key, body))

	channel.basic_consume(callback,
	                      queue=queue_name,
	                      no_ack=True)

	channel.start_consuming()


1) 绑定键为`*`的队列会取到一个路由键为空的消息吗？
不会,因为需要一个路由键，键值任意。
2) 绑定键为`#.*`的队列会获取到一个名为`..`的路由键的消息吗？它会取到一个路由键为单个单词的消息吗？
会，因为`#.*`表示需要至少一个路由键,键值任意。
3) `a.*.#`和`a.#`的区别在哪儿？

	sapphire@debian:~/Rabbitmq$ python receive_logs_topic.py "a.*.#"
	 [*] Waiting for logs. To exit press CTRL+C
	 [x] 'a.':'Hello World!'
	 [x] 'a..':'Hello World!'

	sapphire@debian:~/Rabbitmq$ python receive_logs_topic.py "a.#"
	 [*] Waiting for logs. To exit press CTRL+C
	 [x] 'a.':'Hello World!'
	 [x] 'a..':'Hello World!'
	 [x] 'a':'Hello World!'

可见只有绑定键为`a.#`的才会接收路由键为`a`的消息。因为`a.*.#`表示接收含有`a`和至少一个任意键值的路由键(长度最小为2)。`a.#`表示接收含有`a`的路由键(长度最小为1)。

这里笔者有一点困惑:
terminal1:

	sapphire@debian:~/Rabbitmq$ python emit_log_topic.py ""
	 [x] Sent '':'Hello World!'

terminal2:

	sapphire@debian:~/Rabbitmq$ python receive_logs_topic.py "*"
	 [*] Waiting for logs. To exit press CTRL+C

绑定键为`*`的不会接收路由键为空的消息

terminal1:

	sapphire@debian:~/Rabbitmq$ python emit_log_topic.py "."
	 [x] Sent '.':'Hello World!'

terminal2:

	sapphire@debian:~/Rabbitmq$ python receive_logs_topic.py "*.*"
	 [*] Waiting for logs. To exit press CTRL+C
	 [x] '.':'Hello World!


但是`*.*`却可以接收路由键为`.`的消息。

#### 以下纯属胡扯
`*`是可以和空路由键匹配的，但是在""和"*"时不会匹配，因为认为路由键的长度为0，而绑定键的长度为1。

#### 配置认证

笔者 debian 8.0 通过 apt 安装 rabbitmq-server

配置文件示例在

	/usr/share/doc/rabbitmq-server/abbitmq.config.example.gz


配置文件应存放于(默认不存在，需新建)

	/etc/rabbitmq/rabbitmq.config

配置认证

创建一个用户

	rabbitmqctl add_user Haruna moegirl

添加权限

	sudo rabbitmqctl set_permissions -p "/" Haruna ".*" ".*" ".*"

	rabbitmqctl set_user_tags Haruna administrator


再次更改

	[{rabbit, [{loopback_users, ["Haruna"]}]}].

重启 rabbitmq-server

	sudo service rabbitmq-server restart

