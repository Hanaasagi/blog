
+++
title = "从 namedtuple 到 Data Class"
summary = ''
description = ""
categories = []
tags = []
date = 2018-03-02T15:46:00+08:00
draft = false
+++

### nameptuple

`namedtuple` 在 Python 中一般用来做 record 使用，看一下 `namedtuple` 的源码实现，发现竟然是渲染的模板 (((ﾟдﾟ)))

```Python
def namedtuple(typename, field_names, *, verbose=False, rename=False, module=None):

    # Validate the field names.  At the user's option, either generate an error
    # message or automatically replace the field name with a valid name.
    # 处理 field_names 为字符串的情况
    if isinstance(field_names, str):
        field_names = field_names.replace(',', ' ').split()
    field_names = list(map(str, field_names))
    typename = str(typename)
    # 将 field_names 中非合法标识符(如`2333`)，
    # 或者是与关键字重名，
    # 或者下划线起始，
    # 或者与字段重复的那些进行重命名
    if rename:
        seen = set()
        for index, name in enumerate(field_names):
            if (not name.isidentifier()
                or _iskeyword(name)
                or name.startswith('_')
                or name in seen):
                field_names[index] = '_%d' % index
            seen.add(name)
    # 合法性检查
    for name in [typename] + field_names:
        if type(name) is not str:
            raise TypeError('Type names and field names must be strings')
        if not name.isidentifier():
            raise ValueError('Type names and field names must be valid '
                             'identifiers: %r' % name)
        if _iskeyword(name):
            raise ValueError('Type names and field names cannot be a '
                             'keyword: %r' % name)
    seen = set()
    for name in field_names:
        if name.startswith('_') and not rename:
            raise ValueError('Field names cannot start with an underscore: '
                             '%r' % name)
        if name in seen:
            raise ValueError('Encountered duplicate field name: %r' % name)
        seen.add(name)

    # 渲染类模板
    class_definition = _class_template.format(
        typename = typename,
        field_names = tuple(field_names),
        num_fields = len(field_names),
        arg_list = repr(tuple(field_names)).replace("'", "")[1:-1],  # 这个slice 是为了去括号
        repr_fmt = ', '.join(_repr_template.format(name=name)
                             for name in field_names),
        field_defs = '\n'.join(_field_template.format(index=index, name=name)
                               for index, name in enumerate(field_names))
    )

    # Execute the template string in a temporary namespace and support
    # tracing utilities by setting a value for frame.f_globals['__name__']
    namespace = dict(__name__='namedtuple_%s' % typename)
    exec(class_definition, namespace)
    result = namespace[typename]
    result._source = class_definition
    if verbose:
        print(result._source)

    # For pickling to work, the __module__ variable needs to be set to the frame
    # where the named tuple is created.  Bypass this step in environments where
    # sys._getframe is not defined (Jython for example) or sys._getframe is not
    # defined for arguments greater than 0 (IronPython), or where the user has
    # specified a particular module.
    if module is None:
        try:
            module = _sys._getframe(1).f_globals.get('__name__', '__main__')
        except (AttributeError, ValueError):
            pass
    if module is not None:
        result.__module__ = module

    return result
```

我们可以看一下根据这模板生成出的代码是什么样子

```Python
In [39]: resource = namedtuple('Resource', ['name', 'path', 'help'], verbose=True)
from builtins import property as _property, tuple as _tuple
from operator import itemgetter as _itemgetter
from collections import OrderedDict

class Resource(tuple):
    'Resource(name, path, help)'

    __slots__ = ()

    _fields = ('name', 'path', 'help')

    def __new__(_cls, name, path, help):
        'Create new instance of Resource(name, path, help)'
        return _tuple.__new__(_cls, (name, path, help))

    @classmethod
    def _make(cls, iterable, new=tuple.__new__, len=len):
        'Make a new Resource object from a sequence or iterable'
        result = new(cls, iterable)
        if len(result) != 3:
            raise TypeError('Expected 3 arguments, got %d' % len(result))
        return result

    def _replace(_self, **kwds):
        'Return a new Resource object replacing specified fields with new values'
        result = _self._make(map(kwds.pop, ('name', 'path', 'help'), _self))
        if kwds:
            raise ValueError('Got unexpected field names: %r' % list(kwds))
        return result

    def __repr__(self):
        'Return a nicely formatted representation string'
        return self.__class__.__name__ + '(name=%r, path=%r, help=%r)' % self

    def _asdict(self):
        'Return a new OrderedDict which maps field names to their values.'
        return OrderedDict(zip(self._fields, self))

    def __getnewargs__(self):
        'Return self as a plain tuple.  Used by copy and pickle.'
        return tuple(self)

    name = _property(_itemgetter(0), doc='Alias for field number 0')

    path = _property(_itemgetter(1), doc='Alias for field number 1')

    help = _property(_itemgetter(2), doc='Alias for field number 2')
```

`namedtuple` 是无法给字段添加默认值的。这分享一个小技巧，如何为 `namedtuple` 添加默认值

第一种做法，使用函数去包裹

```Python
In [5]: from collections import namedtuple

In [6]: def _resource():
   ...:     resource = namedtuple('Resource', ['name', 'path', 'help'])
   ...:     def wrapper(name, path, help=''):
   ...:         return resource(name, path, help)
   ...:     return wrapper
   ...:

In [7]: resource = _resource()

In [8]: resource('x', '/root')
Out[8]: Resource(name='x', path='/root', help='')
```

上面的做法要敲好多，其实可以这样做

```Python
In [2]: resource = namedtuple('Resource', ['name', 'path', 'help'])

In [3]: resource.__new__.__defaults__ = ('', )

In [4]: resource('x', '/root')
Out[4]: Resource(name='x', path='/root', help='')
```

`resource.__new__.__defaults__ = ('', )` 这种做法没什么特别之处，就是为 `__new__` 添加了默认值。就像我们可以为函数添加默认值一样

```Python
In [23]: def func(a, b):
    ...:     return a + b
    ...:

In [24]: func.__defaults__ = (1, )

In [25]: func(2)
Out[25]: 3
```

### dataclasses

`dataclass` 是 Python 3.7 引入的新特性(PEP 557)，最初起源于一个第三方 Repo [dataclasses](https://github.com/ericvsmith/dataclasses)。Python 3.6 可以通过 backport 的方式来使用(`pip install dataclasses`)。蠢作者这里直接 pull 了一个 image (`python:3.7.0b1-alpine3.7`)

那么为什么 Python 3.5 就用不了呢？因为它使用了于 [PEP 526](https://www.python.org/dev/peps/pep-0526/)引入的变量类型注解(type annotation)。Python 3.5 仅实现了实现函数注解的 [PEP 484](https://www.python.org/dev/peps/pep-0484/)

简单来说就是“变漂亮了”(误)

```Python
# Python 3.5
primes = []  # type: List[int]

# Python 3.6
primes: List[int] = []
```

`dataclass` 可以认为是 "mutable namedtuples with defaults"，尽管他们的实现不同

> In this document, such variables are called fields. Using these fields, the decorator adds generated method definitions to the class to support instance initialization, a repr, comparison methods, and optionally other methods as described in the Specification section. Such a class is called a Data Class, but there's really nothing special about the class: the decorator adds generated methods to the class and returns the same class it was given.

来自于 [PEP 557](https://www.python.org/dev/peps/pep-0557/) 的小例子

```Python
from dataclasses import dataclass

@dataclass
class InventoryItem:
    '''Class for keeping track of an item in inventory.'''
    name: str
    unit_price: float
    quantity_on_hand: int = 0

    def total_cost(self) -> float:
        return self.unit_price * self.quantity_on_hand

item = InventoryItem('Ruby', 2.0, 3)
```

这风格有点像 Django 的 ORM。`dataclass` 会自动会我们生成一些常用的方法，比如 `__init__`、`__eq__` 等。另外你可以随意添加自己想要的方法，用起来和普通的 `class` 一样。也就是说他不会覆盖你自行定义的 `__init__` 等 "dunder method"。相较于 `namedtuple` 更加灵活，还以通过 `frozen` 参数使其 immutable

在 `dataclass` 出现之前有许多类似的第三方解决方法，也提供了额外的特性。`dataclass` 的目的并不是要取代它们

>Data Classes are not, and are not intended to be, a replacement mechanism for all of the above libraries. But being in the standard library will allow many of the simpler use cases to instead leverage Data Classes. Many of the libraries listed have different feature sets, and will of course continue to exist and prosper.

`dataclass` 不适用的地方

- API compatibility with tuples or dicts is required.
- Type validation beyond that provided by PEPs 484 and 526 is required, or value validation or conversion is required.

`dataclass` 的原理是从 `class` 的 `__annotations__` 属性中提取参数信息，比如上例中为 `{'name': <class 'str'>, 'unit_price': <class 'float'>, 'quantity_on_hand': <class 'int'>}`。然后以此顺序生成 `__init__` 方法，所以必须遵循 Python 函数参数的规则，有默认值的 field 需要放在无默认值的 field 后面。

从 `__annotations__` 提取出的信息会生成对应的 `Field` 对象，相关源码如下

```Python
def _process_class(cls, repr, eq, order, unsafe_hash, init, frozen):
    # omit some code
    for f in _find_fields(cls):
        fields[f.name] = f

def _find_fields(cls):
    annotations = getattr(cls, '__annotations__', {})
    return [_get_field(cls, a_name, a_type)
            for a_name, a_type in annotations.items()]

def _get_field(cls, a_name, a_type):
    default = getattr(cls, a_name, MISSING)
    if isinstance(default, Field):
        f = default
    else:
        f = field(default=default)
    # omit some code
    return f
```

你也可以通过手动赋值的方式去控制这一过程，比如将其从 `__init__` 中排除

```Python
hidden: str = field(init=False, repr=False)
```

`dataclass` 也提供了一个 hook，它在 `__init__` 之后调用

```Python
@dataclass
class C:
    a: float
    b: float
    c: float = field(init=False)

    def __post_init__(self):
        self.c = self.a + self.b
```

被 `dataclass` 装饰过的类都会有一个 `_MARKER`，证明它是一个 Data Class。同时这个属性中存放着所有生成的`Field` 对象

`dataclass` 支持继承，它会搜索 MRO 来提取 `Field` 对象

```Python
@dataclass
class Base:
    x: Any = 15.0
    y: int = 0

@dataclass
class C(Base):
    z: int = 10
    x: int = 15
```

`C` 的 `__init__` 方法的签名如下

```Python
def __init__(self, x: int = 15, y: int = 0, z: int = 10):
```

`dataclasses` 模块附赠了一个 helper function。比如将 Data Class 转换成 `dict` 的 `asdict`。感觉还是蛮不错的

### Reference
[PEP 557 -- Data Classes](https://www.python.org/dev/peps/pep-0557/)

    