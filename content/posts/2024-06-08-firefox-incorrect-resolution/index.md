+++
title = "解决 Linux 下 Firefox 分辨率异常问题"
summary = ""
description = ""
categories = [""]
tags = []
date = 2024-06-08T18:13:37+09:00
draft = false

+++

本文记录今天遇到的一个鬼扯问题。执行 `xdg-open` 后，出现了雪花屏。无奈之下强行了重启系统，结果发现 Firefox 整体被放大了很多倍

排查步骤:

1. 查看系统分辨率 `xrandr`
2. X11 侧显示 `xdpyinfo | grep dimensions`
3. `cat /.Xresources | grep Xft.dpi `，字体的 DPI 配置
4. `xininfo`，应用窗口信息
5. Firefox 的配置，参考 https://wiki.archlinux.org/title/HiDPI#Firefox

发现是 `layout.css.devPixelsPerPx` 的问题，被莫名地修改成了 2。直接重置默认值就好了

### Reference

- https://bbs.archlinux.org/viewtopic.php?id=270419
