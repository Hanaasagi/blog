
+++
title = "Docker 中的信号"
summary = ''
description = ""
categories = []
tags = []
date = 2017-07-25T14:49:00+08:00
draft = false
+++

#### 准备工作

test.py
```
import signal
import os


def custom_handler(signum, frame):
    print("received: {}".format(signum))


signal.signal(signal.SIGTERM, custom_handler)
print("Hello World, pid is {}".format(os.getpid()))
signal.pause()
```

Dockerfile
```
FROM debian:latest
RUN apt-get update && \
    apt-get -y install python3 &&\
    apt-get clean all
COPY test.py ~/
CMD ["/usr/bin/python3", "-u", "~/test.py"]
```


构建镜像 `docker build -t demo:latest .`



#### 信号机制

关于 Linux 的信号就不多说了，支持的信号种类可从 `man 7 signal` 中获取

Docker 支持向容器内发送信号，可以通过 `docker kill <SINGAL> [container]` 发送自定义信号

创建容器

```
sagiri ➜ docker run --name demo demo:latest
Hello World, pid is 1
```

容器中的 Python 脚本在等待信号，执行 `docker kill --signal="SIGTERM" demo` 后，可以看到

```
received: 15
```

并且容器退出

另外，我们需要注意 Python 脚本的 PID 为 1。发送到 Docker 容器的信号，默认会被 PID 为 1 的进程处理，这个进程一般是 Dockerfile 中 `ENTRYPOINT` 或者 `CMD` 所执行的命令

#### 另外一种玩法

我们还可以向 Docker 的 sock 中发送消息，比如这样

```
curl --unix-socket /var/run/docker.sock -X POST  http:/v1.24/containers/demo/kill -d signal="SIGTERM"
```

那么假如我们现在的情景是这样的，一个 Docker 中运行 nginx，另一个 Docker 中运行某个脚本。它接受 bus 中的消息，并决定是否对 nginx 进行 reload。乍一看貌似 Docker 间是隔离的，无法相互影响。但是我们可以在脚本的 Docker 中去挂载宿主机的 `docker.sock` 文件，这样便可以实现这种要求

    