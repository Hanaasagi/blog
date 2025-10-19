+++
title = "Nvim 升级后插件进程树的变化"
summary = ""
description = ""
categories = ["nvim"]
tags = []
date = 2025-10-19T16:07:32+09:00
draft = false

+++



简单记录一下 Nvim 升到 v0.11.4 遇到的一个问题。之前我使用的是自己写的复制插件 https://github.com/Hanaasagi/remote-copy.vim。在升级之后发现报错了，原因是 `fd` 不可写入。我的这个插件是将 OSC52 编码后的序列写入到 Nvim 的进程中的。Nvim 在这次升级之后插件的进程树关系发生了改变



如下图

```
nvim─┬─language_server─┬─language_server───18*[{language_server}]
     │                 └─17*[{language_server}]
     ├─2*[lua-language-se───4*[{lua-language-se}]]
     ├─pynvim-python───python
     └─4*[{nvim}]

```



如果我没有记错，之前这个 Python 的进程应该是直接挂在 nvim 下面的。当时这个插件是直接使用 `getppid` 来找到进程的。现在需要取 `ppid` 的 `ppid` 才行



大概就是这样一个小改动 https://github.com/Hanaasagi/remote-copy.vim/pull/1。当然 Nvim 在 2023 年应该就内置了 OSC52 的 clipboard，后续可以考虑替换掉这个插件
