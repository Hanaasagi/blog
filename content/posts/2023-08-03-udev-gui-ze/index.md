
+++
title = "udev 规则"
summary = ''
description = ""
categories = []
tags = []
date = 2023-08-03T01:45:00+08:00
draft = false
+++

今天滚了一下系统版本后发现自己的键盘控制器挂了，本来以为是 X 更新了，结果是 systemd 的原因。如果 systemd 从 253.7-1 升级到 254-1 后，那么需要检查一下自己的 udev 规则

配置文件路径 `/etc/udev/rules.d/input.rules`

```
# KERNEL=="event*", NAME="input/%k", MODE="660", GROUP="input"
KERNEL=="uinput", NAME="input/%k", MODE="660", GROUP="input", TAG+="uaccess"
```

这里需要做两个变化  
- `KERNEL=="uinput"`  
- `TAG+="uaccess"`

记录一下，可能对其他今天滚系统失败的人有帮助

参考 [udev - ArchLinux wiki](https://wiki.archlinux.org/title/udev)
    