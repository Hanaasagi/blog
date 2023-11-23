
+++
title = "类型中的 Any, object 和 Optional[Any]"
summary = ''
description = ""
categories = []
tags = []
date = 2019-02-24T11:08:06+08:00
draft = false
+++

- `Any` 不等价于 `Optional[Any]`
- `Any` 不等价于 `object`

首先来说明第一条(本文使用 mypy 0.660)

`Optional[Any]` 等价于 `Union[Any, None]`，但是 `Union[Any, T]`(`T` 为任意类型)并不能被简化成 `Any`

`Union` 是类型的并集。当我们对此类型的变量进行方法调用时，mypy 会检查对象是否具有此方法。比如下面的例子

```Python
from typing import List, Union


def f1(v: Union[int, List[int]]) -> List[int]:
    return [v]  # t.py:5: error: List item 0 has incompatible type "Union[int, List[int]]"; expected "int"


def f2(v: Union[int, List[int]]) -> List[int]:
    if isinstance(v, int):
        return [v]
    return v


def f3(v: Union[int, List[int]]) -> None:
    v * 2


f1(1)
f2(1)
f3(1)
f3([1])
```

这是因为我们，未对传入的类型进行检查，造成了可以返回 `List[List[int]]` 的情况。而 `f3` 可以通过则是因为 `List[int]` 和 `int` 都可以进行 `*2`

同样的下面的代码也可以通过检查

```Python
from typing import Any, List, Union


def f1(v: Union[Any, List[int]]) -> None:
    v.append(2)


def f2(v: Any) -> None:
    v.append(2)


f1([1])
f2([1])
```

因为 `Any` 和任何类型兼容，相当于不检查

由此可见 `Any` 不等价于 `Optional[Any]`，如果为 `Optional[Any]` 则会触发 None safety checks，使得我们需要显式地进行 `is not None` 的条件判断

`Union` 还有几条性质

- 嵌套的 `Union` 可以被展开，比如 `Union[Union[int, str], float] == Union[int, str, float]`
- 只有一个类型的 `Union` 可以去掉 `Union`，比如 `Union[int] == int`
- `Union` 中相同的参数可以被合并，且参数没有顺序关系，比如 `Union[int, str, int] == Union[int, str]`

再来看第二条 `Any` 不等价于 `object` 之前，先来理解 `Any`

首先 `Any` 是一个特殊的类型，它其实是在类型系统里面开了一个天窗，而且它不等于 `Union[A, B, C, ...]` 所有类型的并集。每一种类型都和 `Any` 兼容，且 `Any` 也与任何类型兼容。这句话换个说法是，当参数是 `Any` 是可以传入任何类型，当参数是某个具体的类型时也可以传入 `Any` 类型的变量

```Python
from typing import Any


def f1(v: Any) -> None:
    pass


def f2(v: str) -> None:
    pass


f1("hello")
f2([1])  # t.py:13: error: Argument 1 to "f2" has incompatible type "List[int]"; expected "str"

v: Any = [1]
f2(v)  # NOTICE
```

需要特别注意将一个 `Any` 类型的值赋值到一个类型更加具体的变量时是被允许的。另外函数的参数和返回值没有显式指定 annotation 时，都认为是 `Any。因为这种行为也造成了下面的代码是被允许的(使用 mypy 的默认参数)

```Python
from typing import List


def f1():
    return 1


def f2(v: str) -> List[str]:
    return v.split()


f2(f1())
```

典型的场景就是，我们使用 mypy，但是没有给所有的函数都写 annotation。建议先开启 `disallow_untyped_calls = True`

在 Python 里，一切皆 `object`，而一些类型也与 `Any` 兼容。但是反过来，`object` 并不与所有类型兼容

```Python
def f1(v: object) -> None:
    pass


def f2(v: str) -> None:
    pass


f1("hello")
v: object = [1]
f2(v)  # t.py:11: error: Argument 1 to "f2" has incompatible type "object"; expected "str"
```

文档中也指明了两者的用途

> Use `object` to indicate that a value could be any type in a typesafe manner. Use `Any` to indicate that a value is dynamically typed.

    