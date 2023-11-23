
+++
title = "Enum 学习笔记"
summary = ''
description = ""
categories = []
tags = []
date = 2018-04-14T12:49:52+08:00
draft = false
+++

*阅读文档时的笔记*  
`enum`在 Python 3.4 引入，用于表示一组绑定到唯一常量值的符号名称(成员)

### Create

```Python
In [1]: import enum

In [2]: class Status(enum.Enum):
   ...:     FINISHED = 0
   ...:     PENDING = 1
   ...:     RUNNING = 2
   ...:     CANCELED = 3
   ...:

In [3]: Status.FINISHED.name, Status.FINISHED.value  # 支持属性访问
Out[3]: ('FINISHED', 0)

In [4]: Status(0).name, Status(1).name  # 可以通过 value 获取 name
Out[4]: ('FINISHED', 'PENDING')

In [5]: Status['FINISHED'].value, Status['PENDING'].value  # 支持 item access
Out[5]: (0, 1)
```

另一种创建方式

```Python
In [27]: Status = enum.Enum(value='Status', names=(
    ...:     'FINISHED PENDING RUNNING CANCELED'), start=0)  # start 参数可以修改起始值
    ...:
In [28]: for status in Status:
    ...:     print(status.name, status.value)
    ...:
FINISHED 0
PENDING 1
RUNNING 2
CANCELED 3
```

成员按照传入的 `names` 参数升序，起始值为 1。理由是 0 会被认为是 `False`  但枚举成员应当均被认为 `True`

`names` 参数也可以为一个 tuple 的 list

```Python
In [31]: Status = enum.Enum(value='Status', names=[
    ...:     ('FINISHED', 0),
    ...:     ('PENDING', 1)
    ...: ])  # names 也可以为一个 dict

In [32]: for status in Status:
    ...:     print(status.name, status.value)
    ...:
FINISHED 0
PENDING 1
```

也有这种神奇的操作存在

```Python
In [4]: Status.FINISHED.CANCELED.name, Status.FINISHED.CANCELED.value
Out[4]: ('CANCELED', 3)
```

>Enum members are instances of their Enum class, and are normally accessed as EnumClass.member. Under certain circumstances they can also be accessed as EnumClass.member.member, but you should never do this as that lookup may fail or, worse, return something besides the Enum member you are looking for

另外需要注意的是，迭代的顺序是按照定义的顺序来的

```Python
In [6]: for status in Status:
    ...:     print(status.name, status.value)
    ...:
FINISHED 0
PENDING 1
RUNNING 2
CANCELED 3
```

### Compare

继承自 `enum.Enum` 的枚举可以进行比较 identity 和 equality，但是不能比较大小。如果想要实现大小比较需要继承 `enum.IntEnum`

### Unique

具有相同值的枚举成员将作为对同一成员对象的别名，不会出现在迭代中

>two enum members are allowed to have the same value. Given two members A and B with the same value (and A defined first), B is an alias to A. By-value lookup of the value of A and B will return A. By-name lookup of B will also return A

```Python
In [22]: class Status(enum.Enum):
    ...:     FINISHED = 0
    ...:     PENDING = 1
    ...:     RUNNING = 2
    ...:     CANCELED = 3
    ...:     WAITING = 1
    ...:

In [23]: for status in Status:
    ...:     print(status.name, status.value)
    ...:
FINISHED 0
PENDING 1
RUNNING 2
CANCELED 3
In [24]: Status.PENDING is Status.WAITING
Out[24]: True

In [25]: Status.PENDING == Status.WAITING
Out[25]: True

In [26]: Status.WAITING.name
Out[68]: 'PENDING'  # PENDING 而不是 WAITING
```

如果需要强制所有成员具有不同的值，可以使用 `enum.unique` 装饰器去装饰枚举类。此时若出现相同值的成员会抛出异常

既然迭代时不会产生那些别名成员，那么如果有一个场景需要用到这些别名成员该怎么做呢？

`__members__` 属性中记录了一切

```Python
In [26]: Status.__members__
Out[26]:
mappingproxy({'CANCELED': <Status.CANCELED: 3>,
              'FINISHED': <Status.FINISHED: 0>,
              'PENDING': <Status.PENDING: 1>,
              'RUNNING': <Status.RUNNING: 2>,
              'WAITING': <Status.PENDING: 1>})
```

如果你并不在意成员的值可以使用 `auto`

```Python
In [65]: class Status(enum.Enum):
    ...:     FINISHED = enum.auto()
    ...:     PENDING = enum.auto()
    ...:     RUNNING = enum.auto()
    ...:     CANCELED = enum.auto()
    ...:

In [66]: list(Status)
Out[66]:
[<Status.FINISHED: 1>,
 <Status.PENDING: 2>,
 <Status.RUNNING: 3>,
 <Status.CANCELED: 4>]
```

`auto` 是通过 `_generate_next_value_` 来获下一个成员的值，所以可以重写实现自定义行为

### Non-integer Member Values

枚举类成员的值可以为非整型

```Python
In [56]: class State(enum.Enum):
    ...:     SOLID = (0, ('liquid', 'gas'))
    ...:     LIQUID = (1, ('solid', 'gas'))
    ...:     GAS = (2, ('solid', 'gas'))
    ...:
    ...:     def __init__(self, value, transitions):
    ...:         self._value_ = value
    ...:         self.transitions = transitions
    ...:
    ...:     def can_transition(self, new_state):
    ...:         return new_state.name.lower() in self.transitions
    ...:

In [57]: State.SOLID.value
Out[57]: 0

In [58]: State.SOLID.can_transition(State.GAS)
Out[58]: True
```

### Restricted subclassing of enumerations

不能继承一个已经定义了成员的枚举类

### IntFlag

Python 3.6 中新增了 IntFlag，能够很好的支持位运算

```Python
In [73]: class Perm(enum.IntFlag):
    ...:     R = 4
    ...:     W = 2
    ...:     X = 1
    ...:

In [74]: RW = Perm.R | Perm.W

In [75]: Perm.R in RW
Out[75]: True
```

### Reference
[enum – Enumeration Type &mdash; PyMOTW 3](https://pymotw.com/3/enum/index.html)  
[8.13. enum — Support for enumerations](https://docs.python.org/3/library/enum.html)  
[PEP 435 -- Adding an Enum type to the Python standard library](https://www.python.org/dev/peps/pep-0435/)  

    