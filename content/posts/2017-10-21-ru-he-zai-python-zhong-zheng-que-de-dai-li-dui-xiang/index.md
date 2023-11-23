
+++
title = "如何在 Python 中正确的代理对象"
summary = ''
description = ""
categories = []
tags = []
date = 2017-10-21T02:19:09+08:00
draft = false
+++

*本文基于 Python 3.6*

### 题外话
先来看一个问题：
已知对于一个对象来说，运算符 `>` 会去调用对象的 `__gt__` 方法：

```Python
In [1]: class T:
   ...:     def __gt__(self, value):
   ...:         print('__gt__ call')
   ...:         return True
   ...:

In [2]: t = T()

In [3]: t > float('inf')
__gt__ call
Out[3]: True

In [4]: t < float('inf')
---------------------------------------------------------------------------
TypeError: '<' not supported between instances of 'T' and 'float'
```

已知对于对象来说，`__getattribute__` 在寻找实例属性时被无条件调用：

```Python
In [5]: class P:
   ...:     def __getattribute__(self, attr):
   ...:         print('__getattribute__ call')
   ...:         return attr
   ...:

In [6]: p = P()

In [7]: p.name
__getattribute__ call
Out[7]: 'name'

In [8]: p.__gt__
__getattribute__ call
Out[8]: '__gt__'
```

那么根据亚里士多德的三段论来说，我们可以使用一个 override `__getattribute__` 方法的对象去代理对象 `t` 的所有方法，实现一个简单的对象代理：

```Python
In [9]: class Proxy:
   ...:     def __init__(self, t):
   ...:         self.__t = t
   ...:     def __getattribute__(self, attr):
   ...:         if attr == '_Proxy__t':
   ...:             return object.__getattribute__(self, attr)
   ...:         return object.__getattribute__(self.__t, attr)
   ...:

In [10]: proxy_t = Proxy(t)

In [11]: proxy_t.__class__
Out[11]: __main__.T

In [12]: proxy_t.__gt__(float('inf'))
__gt__ call
Out[12]: True
```

好像的确可行，但是

```Python
In [9]: proxy_t > float('inf')
---------------------------------------------------------------------------
TypeError: '>' not supported between instances of 'Proxy' and 'float'
```
重新整理一下思路，`>` 对去调用对象的 `__gt__` 方法，而 `__getattribute__` 会去截获属性寻找的过程，返回 `t` 对象的 `__gt__` 方法，所以这种问题应该是前提出现了偏差


根据错误信息可以知道 `__getattribute__` 没起作用，翻阅文档可知

[object.__getattribute__(self, name)](https://docs.python.org/3/reference/datamodel.html#object.__getattribute__)  
>Called unconditionally to implement attribute accesses for instances of the class. If the class also defines `__getattr__()`, the latter will not be called unless `__getattribute__()` either calls it explicitly or raises an AttributeError. This method should return the (computed) attribute value or raise an AttributeError exception. In order to avoid infinite recursion in this method, its implementation should always call the base class method with the same name to access any attributes it needs, for example, `object.__getattribute__(self, name)`.

**This method may still be bypassed when looking up special methods as the result of implicit invocation via language syntax or built-in functions. See Special method lookup.**

通过 Python 语法的隐式调用和内建函数时会绕过这个机制，详细的可以参考[special-method-lookup](https://docs.python.org/3/reference/datamodel.html#special-method-lookup)

>Bypassing the `__getattribute__()` machinery in this fashion provides significant scope for speed optimisations within the interpreter, at the cost of some flexibility in the handling of special methods (the special method **must** be set on the class object itself in order to be consistently invoked by the interpreter).

### 正题

那么如何正确地实现一个对象的代理呢？其实 `__getattribute__` 也可以，不过要在 `Proxy` 类中也显示的定义 `__gt__` 等 special method。但是 `__getattribute__` 在编程时要极为留意，避免 `maximum recursion depth exceeded`，还是 `__getattr__` 更加 friendly

Celery 源代码中有一个现成的实现(她自己声称是 stolen from werkzeug.local.Proxy)

```Python
# https://github.com/celery/celery/blob/master/celery/local.py

def _default_cls_attr(name, type_, cls_value):
    # Proxy uses properties to forward the standard
    # class attributes __module__, __name__ and __doc__ to the real
    # object, but these needs to be a string when accessed from
    # the Proxy class directly.  This is a hack to make that work.
    # -- See Issue #1087.

    def __new__(cls, getter):
        instance = type_.__new__(cls, cls_value)
        instance.__getter = getter
        return instance

    def __get__(self, obj, cls=None):
        return self.__getter(obj) if obj is not None else self

    return type(bytes_if_py2(name), (type_,), {
        '__new__': __new__, '__get__': __get__,
    })


class Proxy(object):
    """Proxy to another object."""

    # Code stolen from werkzeug.local.Proxy.
    __slots__ = ('__local', '__args', '__kwargs', '__dict__')

    def __init__(self, local,
                 args=None, kwargs=None, name=None, __doc__=None):
        object.__setattr__(self, '_Proxy__local', local)
        object.__setattr__(self, '_Proxy__args', args or ())
        object.__setattr__(self, '_Proxy__kwargs', kwargs or {})
        if name is not None:
            object.__setattr__(self, '__custom_name__', name)
        if __doc__ is not None:
            object.__setattr__(self, '__doc__', __doc__)

    @_default_cls_attr('name', str, __name__)
    def __name__(self):
        try:
            return self.__custom_name__
        except AttributeError:
            return self._get_current_object().__name__

    @_default_cls_attr('qualname', str, __name__)
    def __qualname__(self):
        try:
            return self.__custom_name__
        except AttributeError:
            return self._get_current_object().__qualname__

    @_default_cls_attr('module', str, __module__)
    def __module__(self):
        return self._get_current_object().__module__

    @_default_cls_attr('doc', str, __doc__)
    def __doc__(self):
        return self._get_current_object().__doc__

    def _get_class(self):
        return self._get_current_object().__class__

    @property
    def __class__(self):
        return self._get_class()

    def _get_current_object(self):
        """Get current object.
        This is useful if you want the real
        object behind the proxy at a time for performance reasons or because
        you want to pass the object into a different context.
        """
        loc = object.__getattribute__(self, '_Proxy__local')
        if not hasattr(loc, '__release_local__'):
            return loc(*self.__args, **self.__kwargs)
        try:  # pragma: no cover
            # not sure what this is about
            return getattr(loc, self.__name__)
        except AttributeError:  # pragma: no cover
            raise RuntimeError('no object bound to {0.__name__}'.format(self))

    @property
    def __dict__(self):
        try:
            return self._get_current_object().__dict__
        except RuntimeError:  # pragma: no cover
            raise AttributeError('__dict__')

    def __repr__(self):
        try:
            obj = self._get_current_object()
        except RuntimeError:  # pragma: no cover
            return '<{0} unbound>'.format(self.__class__.__name__)
        return repr(obj)

    def __bool__(self):
        try:
            return bool(self._get_current_object())
        except RuntimeError:  # pragma: no cover
            return False
    __nonzero__ = __bool__  # Py2

    def __dir__(self):
        try:
            return dir(self._get_current_object())
        except RuntimeError:  # pragma: no cover
            return []

    def __getattr__(self, name):
        if name == '__members__':
            return dir(self._get_current_object())
        return getattr(self._get_current_object(), name)

    def __setitem__(self, key, value):
        self._get_current_object()[key] = value

    def __delitem__(self, key):
        del self._get_current_object()[key]

    def __setslice__(self, i, j, seq):
        self._get_current_object()[i:j] = seq

    def __delslice__(self, i, j):
        del self._get_current_object()[i:j]

    def __setattr__(self, name, value):
        setattr(self._get_current_object(), name, value)

    def __delattr__(self, name):
        delattr(self._get_current_object(), name)

    def __str__(self):
        return str(self._get_current_object())

    def __lt__(self, other):
        return self._get_current_object() < other

    def __le__(self, other):
        return self._get_current_object() <= other

    # omit some special method
```

另外说一点，使用 `_default_cls_attr` 而不用 `property` 装饰器 可以参考 [issue 1087](https://github.com/celery/celery/issues/1087)

从 class 访问 `property` 会返回 `property` 的实例

```Python
In [12]: class A:
    ...:     @property
    ...:     def attr(self):
    ...:         return 'attr'
    ...:

In [13]: A.attr
Out[13]: <property at 0x7fc0735dda98>
```

原因貌似好像是这个 (依据[等价实现](http://pyzh.readthedocs.io/en/latest/Descriptor-HOW-TO-Guide.html))

```Python
class property:
    def __get__(self, obj, objtype=None):
        # look here
        if obj is None:
            return self
        if self.fget is None:
            raise AttributeError, "unreadable attribute"
        return self.fget(obj)
```

所以 Celery 自己搞了一个 descriptor，通过 override `__new__` 生成指定 `type_` 类型的对象。例如 `@_default_cls_attr('name', str, __name__)`，则是一个 `str` 类型值为 `__name__` 的对象。当然这个对象还具有 `__get__` 方法

### Reference
[Data Model](https://docs.python.org/3/reference/datamodel.html)  
[Python描述器引导](http://pyzh.readthedocs.io/en/latest/Descriptor-HOW-TO-Guide.html)

    