
+++
title = "Why Python has abstract base class"
summary = ''
description = ""
categories = []
tags = []
date = 2017-06-10T13:50:54+08:00
draft = false
+++

抽象基类(Abstract base classes)并不是 Python 本身的语言特性，而是在标准库中提供了一种支持。关于抽象基类， Python Glossary 中定义如下

>Abstract base classes complement duck-typing by providing a way to define interfaces when other techniques like hasattr() would be clumsy or subtly wrong (for example with magic methods). ABCs introduce virtual subclasses, which are classes that don’t inherit from a class but are still recognized by isinstance() and issubclass(); see the abc module documentation. Python comes with many built-in ABCs for data structures (in the collections.abc module), numbers (in the numbers module), streams (in the io module), import finders and loaders (in the importlib.abc module). You can create your own ABCs with the abc module.


一般定义抽象基类可以使用 `abc.ABCMeta` 这个元类，通过 `register()` 方法能将不相关的具体类(包含 built-in class)及其他抽象类注册为虚子类(virtual subclasses)。~~[virtual subclass 这个概念好像只有在  Python 中有提到]~~ `issubclass()` 会认为他们之间具有子类关系，但抽象基类并不会出现在被注册类的 MRO 中，也不会调用到抽象基类中定义的方法。当然也可以不用费劲的通过 `register()` 一个个进行注册，能够通过 `__subclasshook__()` 来控制 `issubclass()` 的行为。普通类定义 `__subclasshook__()` 并不会被触发

>This method should return True, False or NotImplemented. If it returns True, the subclass is considered a subclass of this ABC. If it returns False, the subclass is not considered a subclass of this ABC, even if it would normally be one. If it returns NotImplemented, the subclass check is continued with the usual mechanism.

演示代码

```Python
In [17]: int.__mro__
Out[17]: (int, object)

In [18]: bool.__mro__
Out[18]: (bool, int, object)

In [19]: class A(metaclass=abc.ABCMeta):
    ...:     pass
    ...:

In [20]: A.register(int)
Out[20]: int

In [21]: int.__mro__
Out[21]: (int, object)

In [22]: bool.__mro__
Out[22]: (bool, int, object)

In [23]: issubclass(int, A)
Out[23]: True

In [24]: issubclass(bool, A)
Out[24]: True
```

通常而言，抽象基类就是定义了一系列子类必须要实现的东西，相当于一种契约，如果没有实现这种契约是不能被实例化的。Python 也是如此，不过这种概念相对而言意义不大，因为 Python 可以通过自省(instrospect)和 EAFP 灵活地判断对象是否满足契约。不过在 Python 中抽象基类还有一个作用，那便是用于 duck-typing 中(Glossary 中也提到抽象基类为 duck-typing 提供了扩充)。在这种编程风格中，并不关注对象的类型，而是关注对象是否具有某些行为。而抽象基类可以看做提供了一系列行为的集合。如果你实现了这些行为，便可以看做是这个抽象基类的  virtual subclass

```Python
In [35]: class P:
    ...:     def __iter__(self):
    ...:         pass
    ...:     def __contains__(self, item):
    ...:         pass
    ...:     
    ...:     

# 触发 Container 的 __subclasshook__
In [36]: issubclass(P, Container)
Out[36]: True

In [37]: issubclass(P, Iterable)
Out[37]: True      
```

我们可以通过抽象基类来检测对象是否满足条件，正如 duck-typing 所形容的，实现了 `__contains__()` 的类型都是 Container; 实现了 `__iter__()` 的类型都是 Iterable。因此可以进行更友好的行为检测，而不是像下面这样松散的代码

```Python
if hasattr(P, '__iter__') and hasattr(P, '__contains__'):
    pass
# or
if all(map(lambda attr: hasattr(P, attr), ['__iter__', '__contains__'])):
    pass
```

#### Reference
[Why use Abstract Base Classes in Python?](https://stackoverflow.com/questions/3570796/why-use-abstract-base-classes-in-python)

    