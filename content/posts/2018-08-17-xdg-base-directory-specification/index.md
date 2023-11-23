
+++
title = "XDG 基本目录规范"
summary = ''
description = ""
categories = []
tags = []
date = 2018-08-17T15:25:21+08:00
draft = false
+++

### Abstract

XDG Base Directory Specification(XDG 基本目录规范) 对应用程序的文件存储路径给出了一个明确的定义。目前有许多应用都支持 XDG Base Directory Specification，比如 pip，Laravel。具体可以参考 [XDG Base Directory support](https://wiki.archlinux.org/index.php/XDG_Base_Directory_support)

### Basics

XDG 基本目录规范包含以下概念:

- `XDG_DATA_HOME` 下存放用户数据文件，默认值是 `~/.local/share`
- `XDG_CONFIG_HOME` 下存放用户配置文件，默认值是 `~/.config`
- `XDG_DATA_DIRS` 定义一组以 `:` 分隔的有序目录集，规定了除 `XDG_DATA_HOME` 外的搜索路径，默认值是 `/usr/local/share/:/usr/share/`
- `XDG_CONFIG_DIRS` 定义一组以 `:` 分隔的有序目录集，规定了除 `XDG_CONFIG_HOME` 外的搜索路径，默认值是 `/etc/xdg`
- `XDG_CACHE_HOME` 下存放用户的缓存文件，默认值是 `~/.cache`
- `XDG_RUNTIME_DIR` 下存放运行时的用户文件，比如 sockets、named pipes。此目录必须属于该用户，并且他必须是用户中唯一拥有读/写操作的以为，换句话说便是 `0700` 权限

对于 `XDG_CONFIG_DIRS`(`XDG_DATA_DIRS`) 来说，顺序代表了这些目录的重要性，第一个列出的目录是最重要的

另外如果尝试写入文件时，目标目录不存在，那么应当尝试使用 `0700` 权限创建目标目录；如果目标目录已经存在，则不能对权限进行修改。应用程序应当能够处理无法写入文件的情况，可以直接向用户呈现错误信息。如果读取文件时，因为某种原因目录中的文件不可访问。比如目录不存在，文件不存在或者用户没有权限。那么应当跳过此文件，如果因此无法找到所需的文件，则应当向用户呈现错误信息。如果文件同时位于 `XDG_CONFIG_DIRS`(`XDG_DATA_DIRS`) 中的多个目录，那么应用应当定义处理此种情况的行为，比如仅使用最重要的目录中的文件，或者定义一个规则去合并这些位于不同目录中的文件


### Example

以 `pip` 为例，它是符合 XDG Base Directory Specification 的。如果想要知道它如果获取配置的，我们可以阅读文档或者源代码。这里另外给出一个角度，读取配置文件首先需要通过 `open` 系统调用去打开文件。那么可以便通过 `strace` 去跟踪一下它

```Bash
➜ strace -e trace=open pip 2>&1 | grep "pip.conf"
open("/etc/xdg/pip/pip.conf", O_RDONLY) = -1 ENOENT (No such file or directory)
open("/etc/pip.conf", O_RDONLY)         = -1 ENOENT (No such file or directory)
open("/home/sapphire/.pip/pip.conf", O_RDONLY) = -1 ENOENT (No such file or directory)
open("/home/sapphire/.config/pip/pip.conf", O_RDONLY) = -1 ENOENT (No such file or directory)
```

最后说一点，配置文件通常命名为

```
1) ~/.ansible.cfg /etc/pacman.conf
2) ~/.stack/config.yaml  ~/.rustup/settings.toml
3) wgetrc vimrc bashrc
```

其实 `vimrc` 和 `bashrc` 应当不属于配置文件的范畴，因为他们是一个初始化脚本。`rc` 即是 "run commands" 的意思。但这里比较奇怪的是 `wgetrc` 中的内容却看起来想一个普通的配置文件

### Reference
[XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html)

    