
+++
title = "再谈 Python 单例模式"
summary = ''
description = ""
categories = []
tags = []
date = 2017-05-08T07:42:27+08:00
draft = false
+++

单例模式是个老生长谈的问题，今天我又一次把它揪了出来。原因是[Python 中的单例模式](http://python.jobbole.com/87294/)这一篇文章中使用 `__new__` 的创建单例的方式是有瑕疵的，具体代码如下

```python
class Singleton(object):
    _instance = None
    def __new__(cls, *args, **kw):
        if not cls._instance:
            cls._instance = super(Singleton, cls).__new__(cls, *args, **kw)  
        return cls._instance  

class MyClass(Singleton):  
    a = 1
```

乍看好像没有什么问题，但这种写法就是个坑。我们知道 Python 中创建对象的是 `__new__`，`__init__` 则是用来修饰对象的。实现自定义的 `__new__` 方法使得我们无论如何得到的都是一个对象，这点可以通过 `is` 操作清晰的看到

```
In [14]: MyClass() is MyClass()
Out[14]: True
```

但是我们忽略了 `__init__`

```python
In [23]: class Singleton(object):
    ...:     _instance = None
    ...:     def __new__(cls, *args, **kw):
    ...:         if not cls._instance:
    ...:             cls._instance = super(Singleton, cls).__new__(cls, *args, *
    ...: *kw)  
    ...:         return cls._instance  
    ...:
    ...: class MyClass(Singleton):  
    ...:     def __init__(self):
    ...:         self.l = []
    ...:         

In [24]: a = MyClass()

In [25]: a.l.append(2)

In [26]: a.l
Out[26]: [2]

In [27]: b = MyClass()

In [28]: a.l
Out[28]: []

In [29]: a is b
Out[29]: True
```

显然这不是我们想要的结果。我们应该使这个对象仅初始化一次。好吧，我们大可以不在一棵树上吊死，毕竟有那么多中单例模式的写法，比如 `metaclass`。但是强迫症复发的蠢作者偏偏想用 `__new__` 怎么办，我难道需要设置个条件变量然后在 `__init__` 中再进行判断？经过一番思索，干脆直接避开 `__init__`

```
class Singleton(object):
    _instance = None
    def __new__(cls, *args, **kw):
        print(cls)
        if not cls._instance:
            cls._instance = super(Singleton, cls).__new__(cls)
            cls._instance.initialize(*args, **kw)  
        return cls._instance  

class MyClass(Singleton):  
    a = 1
    def initialize(self):
        self.l = []
```

ugly ugly ugly !!!!

    