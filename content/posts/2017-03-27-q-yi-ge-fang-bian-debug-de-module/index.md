
+++
title = "q 一个方便 debug 的 module"
summary = ''
description = ""
categories = []
tags = []
date = 2017-03-27T15:15:21+08:00
draft = false
+++

~~最近感觉好懒，星期日看的源代码，现在才写出来文章。难道这就是春困(懒病)？~~  
咳咳，回到正题 关于 [q](https://github.com/zestyping/q) 的详细介绍可以看看 README 文件或者这个[视频](https://www.youtube.com/watch?v=OL3De8BAhME&t=25m15s)

总之，这个 module 可以方便 debug

输出结果默认在 `/tmp/q` 中， 可以使用 `tail` 或者 `cat` 命令查看(此文件内部包含终端颜色字符直接打开文件会出现乱码)。当然最好是使用 `tail -f` 进行输出的持续捕获

Example:

```python
import q

@q.t
def hello(name):
    print('hello ' + name)

if __name__ == '__main__':
    hello('weiss')
```

/tmp/q 文件

```
0.0s hello('weiss')
0.0s -> None
```

整个 module 一共就 300 lines+，经过傻逼作者的一番研究，大致觉得有以下几点可以来讲讲

首先是这种内部类的写法，Python 这样写我是第一次见(本人见识短浅，勿怪)

```python
class Q(object):
    __doc__ = __doc__  # from the module's __doc__ above

    import ast
    import code
    import inspect

    # ...
    class FileWriter(object):
        # ...

    class Writer(object):
        # ...

    def __init__(self):
        self.writer = self.Writer(self.FileWriter(self.OUTPUT_PATH), self.time)

    def t(self):
        # ...
    # ...

sys.modules['q'] = Q()
```

最后一行代码使得 `import` 进来的 `q` 即为 `Q` 的实例，当然这样也会带来副作用：无法访问除 `Q`类 引用到的全局变量。所以我们可以看到 `import` 语句是放在 `Q` 类的内部。

至于这个 module 的工作原理么，其实很简单，就是利用闭包来截获传入的参数和函数调用后的返回值

```python
def trace(func):
    def wrapper(*args, **kwargs):
      # 获取传入的参数
      # 执行函数
      # 获取返回值
    return wrapper
```

不过还做了异常处理来应对函数抛出异常的情况，下面是一个差不多的例子

```python
import sys
import inspect
import textwrap

def error():
    raise ZeroDivisionError

try:
    error()
except:
    etype, evalue, etb = sys.exc_info()
    # context 参数用来指定上下文的行数
    info = inspect.getframeinfo(etb.tb_next, context=3)
    # 获取异常处的上下文
    lines = info.code_context
    # info.index 为异常抛出处在上下文中的 index
    # info.lineno 为异常抛出处的行号
    firstlineno = info.lineno - info.index
    fmt = '<{}'.format(len(str(firstlineno+ len(lines))))
    msg = '>!  {etype} at {filename}:{lineno}'.format(fmt=fmt,
        etype=etype, filename=info.filename, lineno=info.lineno)
    # q module 这里则是写入文件
    print(msg)
    lines = textwrap.dedent(''.join(lines))
    for i, line in enumerate(lines.split('\n')):
        msg = '{lineno:{fmt}}{seq}\t{line}'.format(
            fmt=fmt, lineno=firstlineno+i,
            seq=('>' if i==info.index else ':'), line=line)
        print(msg)
    raise
```

除此之外 q 还能实现了便捷的打印操作

比如

```python
a(b())
```
如果我们想查看这个 `b()` 的返回值，平常我们会这样

```python
tmp = b()
print(tmp)
a(tmp)
```

有了 q 之后便可以

```python
a(q(b()))
```

那么这是如何做到的呢？ 将参数打印出来不就行了 (●▼●;) 不过呢，为了更加优雅的显示，此 module 做了比较多的工作

```python
import ast
import re
import sys
import inspect

def get_call_exprs(line):
    """Gets the argument expressions from the source of a function call."""
    # 构建 AST 并获取参数表达式
    line = line.lstrip()
    try:
        tree = ast.parse(line)
    except SyntaxError:
        return None
    for node in ast.walk(tree):
        if isinstance(node, ast.Call):
            # offsets 用来保存参数的起始偏移和结束偏移
            offsets = []
            for arg in node.args:
                # In Python 3.4 the col_offset is calculated wrong. See
                # https://bugs.python.org/issue21295
                if isinstance(arg, ast.Attribute) and (
                        (3, 4, 0) <= sys.version_info <= (3, 4, 3)):
                    offsets.append(arg.col_offset - len(arg.value.id) - 1)
                else:
                    offsets.append(arg.col_offset)
            # 去除不相关的表达式 or 语句
            # 比如 func(q(arg1), arg2)
            if node.keywords:
                line = line[:node.keywords[0].value.col_offset]
                line = re.sub(r'\w+\s*=\s*$', '', line)
            else:
                line = re.sub(r'\s*\)\s*$', '', line)
            offsets.append(len(line))
            args = []
            #             offset
            # first  arg: 0    1
            # second arg: 1    2
            for i in range(len(node.args)):
                args.append(line[offsets[i]:offsets[i + 1]].rstrip(', '))
            return args

def q(*args):
    # 为方便起见，未考虑代码换行的情况，设置了 context=1
    # q 源代码中 context=9 并对上下文进行的判断
    info = inspect.getframeinfo(sys._getframe(1), context=1)
    lines = info.code_context
    labels = get_call_exprs(''.join(lines).replace('\n', ''))

    # output
    reprs = map(repr, args)
    sep = ''
    for label, repr_var in zip(labels, reprs):
        print('{} => {}'.format(label, repr_var))
        sep = ', '
    # 优雅的实现透明处理
    return args and args[0]


def a(name,a):
    return 'func_a ' + name

def b(name):
    return 'func_b ' + name

a(q(b('weiss')),2)
```

另外， 这个 module 还重载了 `__div__` 和 `__or__`，提供了更加 magic 的调用方式

    