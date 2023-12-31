
+++
title = "Python readline module"
summary = ''
description = ""
categories = []
tags = []
date = 2017-03-01T03:50:00+08:00
draft = false
+++

`GNU readline` 库提供了一些函数供程序调用，来允许用户修改他们键入的命令。通过这个库可以达到以下目的

1) `<TAB>` 补全
2) `↑` 或 `↓` 浏览历史命令
`...`

Python 的 [`readline` 模块](https://docs.python.org/3.5/library/readline.html)对这个库进行了一层封装

通过文档可以得知其基本用法，下面举几个小例子

### 补全


    import time
    import readline


    class Command(object):
        command_list = ['time']

        @staticmethod
        def exec_command(command):
            if command in Command.command_list:
                return getattr(Command, command)()
            return 'Command Not Found'

        @staticmethod
        def time():
            return time.ctime()

    class SimpleCompleter(object):

        def __init__(self, complete_list):
            self.complete_list = complete_list

        def complete(self, text, state):
            response = None
            text = text.lower()
            if state == 0:
                if text:
                    self.matches = [ s for s in self.complete_list
                                    if s.startswith(text)]
                else:
                    self.matches = self.complete_list

            try:
                response = self.matches[state]
            except IndexError:
                pass
            return response

    readline.set_completer(SimpleCompleter(Command.command_list).complete)
    readline.parse_and_bind('tab: complete')

    if __name__ == '__main__':
        line = ''
        while line != 'stop':
            line = input('>>')
            result = Command.exec_command(line)
            print(result)

一个更好的示例便是[ Python 的 rlcompleter 模块源代码](https://hg.python.org/cpython/file/3.5/Lib/rlcompleter.py)

### 历史命令

    import atexit
    import readline

    histfile = '/tmp/.test_history'
    try:
        readline.read_history_file(histfile)
        # default history len is -1 (infinite), which may grow unruly
        readline.set_history_length(1000)
    except FileNotFoundError:
        pass

    atexit.register(readline.write_history_file, histfile)

    if __name__ == '__main__':
        line = ''
        while line != 'exit':
            line = input('>>')

### Reference

- [readline — GNU readline interface](https://docs.python.org/3.5/library/readline.html)
- [GNU Readline Library](https://cnswww.cns.cwru.edu/php/chet/readline/rltop.html)
- [Python readline.set_completer Examples](http://www.programcreek.com/python/example/873/readline.set_completer)
