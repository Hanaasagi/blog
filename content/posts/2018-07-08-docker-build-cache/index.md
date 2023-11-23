
+++
title = "Docker build cache"
summary = ''
description = ""
categories = []
tags = []
date = 2018-07-08T10:09:47+08:00
draft = false
+++

首先镜像是分层的，通常来说 Dockerfile 中每一条指令都会构建出一个新的镜像层。但是 Docker 存在 cache 机制，当新的镜像层和已有的镜像重复时，它会直接复用已有的那层镜像。以下面前端项目的 Dockerfile 来说

```Dockerfile
FROM node:9.2-stretch

WORKDIR /app

ADD package*.json /tmp/

RUN cd /tmp && \
    npm install && \
    cp -a /tmp/node_modules /app/

ADD . /app

ARG HELP_LINK
ENV HELP_LINK=$HELP_LINK

ARG CREATE_ALERT_LINK
ENV CREATE_ALERT_LINK=$CREATE_ALERT_LINK

RUN node build/build.js
```

它基于 `node:9.2-stretch` 进行构建，会新建 9 层镜像。即每一条命令都睡生成一层。这点我们可以通过执行 `docker build` 时的输出得到验证(`Step n/10`)

当我们在未经改动的情况下再次构建，cache 会全部命中

```Bash
Sending build context to Docker daemon  973.8kB                                              
Step 1/10 : FROM node:9.2-stretch
 ---> 4d4db4a147fc
Step 2/10 : WORKDIR /app
 ---> Using cache
 ---> b41c5bd21a4c
Step 3/10 : ADD package*.json /tmp/
 ---> Using cache
 ---> a8b566f374cd
Step 4/10 : RUN cd /tmp &&     npm install &&     cp -a /tmp/node_modules /app/              
 ---> Using cache
 ---> 48d0f18cc788
Step 5/10 : ADD . /app
 ---> Using cache
 ---> fdf7803e3c89
Step 6/10 : ARG HELP_LINK
 ---> Using cache
 ---> eaa58e19b837
Step 7/10 : ENV HELP_LINK=$HELP_LINK
 ---> Using cache
 ---> 2638069c84d8
Step 8/10 : ARG CREATE_ALERT_LINK
 ---> Using cache
 ---> 70114ae46dcc
Step 9/10 : ENV CREATE_ALERT_LINK=$CREATE_ALERT_LINK                                         
 ---> Using cache
 ---> befe7fe074af
Step 10/10 : RUN node build/build.js
 ---> Using cache
 ---> 32d71daced8a
Successfully built 32d71daced8a
Successfully tagged test:latest   
```

那么合适会被判定为 cache 命中呢？

- json 文件相符
- 父镜像相同
- 拷贝到镜像文件系统中的内容不变，包含文件内容，修改时间等

首先来看一下 json 文件相同的含义。镜像是由文件系统和 json 文件组成的，json 文件包含了元数据。这里拿 Step 2 的 json 文件举例，我省略了一些字段

```Bash
➜  sha256 pwd
/var/lib/docker/image/overlay2/imagedb/content/sha256
➜  sha256 cat b41c5bd21a4c7ae9ae741b4661f9ab09426e9c794131098c2335aadb6be5821e | jq . -C
```

```Json
{
  "architecture": "amd64",
  "config": {
    "Hostname": "",
    "Domainname": "",
    "User": "",
    "AttachStdin": false,
    "AttachStdout": false,
    "AttachStderr": false,
    "Tty": false,
    "OpenStdin": false,
    "StdinOnce": false,
    "Env": [
      "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
      "NODE_VERSION=9.2.1",
      "YARN_VERSION=1.3.2"
    ],
    "Cmd": [
      "node"
    ],
    "ArgsEscaped": true,
    "Image": "sha256:4d4db4a147fcc05f44ec4d79402f65ab4530f6cb37aa098fb8ea829a4cf5348a",
    "Volumes": null,
    "WorkingDir": "/app",
    "Entrypoint": null,
    "OnBuild": [],
    "Labels": null
  },
  "container": "5396b7fb10f11dd4d873001acdb46dd981b873bcb2ddaddae34cb1c6adc94888",
  "container_config": {
    "Hostname": "5396b7fb10f1",
    "Domainname": "",
    "User": "",
    "AttachStdin": false,
    "AttachStdout": false,
    "AttachStderr": false,
    "Tty": false,
    "OpenStdin": false,
    "StdinOnce": false,
    "Env": [
      "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
      "NODE_VERSION=9.2.1",
      "YARN_VERSION=1.3.2"
    ],
    "Cmd": [
      "/bin/sh",
      "-c",
      "#(nop) WORKDIR /app"
    ],
    "ArgsEscaped": true,
    "Image": "sha256:4d4db4a147fcc05f44ec4d79402f65ab4530f6cb37aa098fb8ea829a4cf5348a",
    "Volumes": null,
    "WorkingDir": "/app",
    "Entrypoint": null,
    "OnBuild": [],
    "Labels": {}
  }
}
```

可以看出 `b41c5bd21a4c` 的镜像是通过镜像 `4d4db4a147fc` 的容器 `5396b7fb10f1` 执行 `/bin/sh -c #(nop) WORKDIR /app` 而来的。我们可以逐步地跟踪每一个 Step 的镜像来搞清楚整个构建的逻辑

不过 json 文件中提供了更简单的方式去回溯，那便是 `history` 字段(我刚才省略掉了)，它甚至包含了基础镜像的构建历史。亦可通过 `docker history` 命令

那么 json 文件和 cache 有什么关系呢。只要某个镜像的 json 文件和将要创建的镜像的 json 文件一致，那么便可以复用此镜像

另一个影响 cache 命中的因素是拷贝的内容。`ADD` 和 `COPY` 命令会把 build context 中的内容拷贝至镜像，显然如果仅存在上述条件是无法在文件内容发生改变时重新构建的。所以 Docker 在 `ADD` 和 `COPY` 指令是会关注拷贝内容的 inode 信息，比如文件内容、修改时间等

根据 cache 的机制，可以得出使用 Dockerfile 进行构建时需要注意

- 将稳定不变的部分放在 Dockfile 的顶部，比如一些 `apt-get` 安装的依赖
- 如果在大部分镜像中都是用了相同的指令，比如 `MAINTAINER`，那么最好将他们放在顶部，并且保持在所有的 Dockfile 中有着相同的顺序。这样可以在不同镜像间共享 cache
- 如果一些文件是被频繁更新的，且并不是构建所必须的。那么可以通过 `.dockerignore` 排除，否则将影响 `ADD` 和 `COPY` 的 cache 命中

    