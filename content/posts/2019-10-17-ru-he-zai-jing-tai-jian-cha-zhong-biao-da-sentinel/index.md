
+++
title = "如何在静态检查中表达 sentinel"
summary = ''
description = ""
categories = []
tags = []
date = 2019-10-17T07:45:19+08:00
draft = false
+++

场景，假设我现在有一个 `req` 函数，它的 `timeout` 参数可能会有两种情况

- `None` 不做超时处理
- `float` 类型，自定义超时时间

现在做一层封装，添加了 `Session` 这一概念，它的 `reqeust` 方法也有一个  `timeout`，可能会出现以下的情况

- `None` 不做超时处理
- `UNSET` 使用默认配置
- `float` 类型，自定义超时时间

代码如下

```Python
from typing import Optional

UNSET = object()

def req(timeout: Optional[float] = None) -> None:
    pass

class Session:
    def __init__(self) -> None:
        self._timeout = 5.0

    def request(self, timeout=UNSET) -> None:
        if timeout is UNSET:
            req(self._timeout)
        elif timeout is None:
            req()
        else:  # 分支可以和上面的合并
            req(timeout)
```

`UNSET` 对象它的类型是 `object` ，而所有的对象都是 `object`。为了解决这个问题，我们可以给 `UNSET` 一个特殊的类型

```Python
from typing import Optional
from typing import Union
from typing import Type

class UnsetType:
    pass

UNSET = UnsetType()

class Session:

    def request(self, timeout: Union[UnsetType, float, None] = UNSET) -> None:
        if timeout is UNSET:
            req(self._timeout)
        elif timeout is None:
            req()
        else:
            req(timeout)  # this line
```

但我们又引入了新的问题

```
Argument 1 to "req" has incompatible type "Union[UnsetType, float]"; expected "Optional[float]"
```

这个报错指出了 `timeout` 变量可以是 `UnsetType` 或者 `float` 类型的实例，但是我们的 `req` 函数仅接受 `None` 对象或者 `float` 类型的实例。因为我们在第一个分支中的判断使用的是 `timeout is UNSET`，所以我们并没有穷尽 `UnsetType` 类型的可能性，导致了在最后的 `else` 中会出现 `timeout` 为 `UnsetType` 的实例的情况，尽管我们知道在整个项目中只有 `UNSET` 是 `UnsetType` 类型的实例。有兴趣可以了解一下代数类型系统



一种解决方法是将 `timeout is UNSET` 替换成 `isinstance(timeout, UnsetType)` ，但是并不美观。我们在这里期待的是对值的判断，而不是类型上的判断。我们想表达 `reqeust` 方法仅能接受 `UNSET` 对象，`None` 对象和浮点数。`None` 其实便是一个特殊的对象，不过它在 Mypy 内部被特别的处理了



PEP 484 中给出了这样的 workround：使用 `enum.Enum` 来处理

> Since the subclasses of `Enum` cannot be further subclassed, the type of variable `x` can be statically inferred in all branches of the above example. The same approach is applicable if more than one singleton object is needed: one can use an enumeration that has more than one value

```Python
class UnsetType(Enum):
    unset = 0

UNSET = UnsetType.unset
```

但是这也不美观，如果不用 Mypy 谁会去这么表达一个 sentinel 呢？不过在类型系统中 `Enum` 的确是值的枚举，而 `Union` 是类型的枚举



那么可以用 Python 3.8 里的 `typing.Literal` 么？它也可以表达对于值的推断，比如 `open` 的 `mode` 参数。但是它的参数只能是字面量



### Reference

[Types for singleton objects](https://github.com/python/typing/issues/236)  
[PEP 484 -- Type Hints](https://www.python.org/dev/peps/pep-0484/#support-for-singleton-types-in-unions)  
[PEP 586 -- Literal Types](https://www.python.org/dev/peps/pep-0586/#legal-and-illegal-parameterizations)
    