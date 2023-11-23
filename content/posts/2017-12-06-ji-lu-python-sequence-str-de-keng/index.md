
+++
title = "记录 Python Sequence[str] 的坑"
summary = ''
description = ""
categories = []
tags = []
date = 2017-12-06T14:32:10+08:00
draft = false
+++

Python 一切皆为对象，没有像 Java 那样具备基本类型，支持一套 boxing/unboxing 操作。并且一个很重要的问题是 Python 弱化了 `char` 和 `string` 的概念。这虽然方便了程序员进行编程，当时也埋下了一个炸弹，那便是 `Sequence[str]`

[PEP 484](https://www.python.org/dev/peps/pep-0484/) 为 Python 注入了新的活力，引入了一套类型系统。结合 `mypy` 可以进行静态的语法分析，这对于大型工程来说十分有用，尤其在阅读源代码时。可能你也有所体会，有时我们惯性地想去知道参数是什么类型。我可以使用 n 种类型的参数来满足函数运行的条件，那么我在修改代码时，想去调用参数的另一个方法时，我只能去追溯调用链，查明是何种参数。况且，有可能此函数在代码中被不同的地方调用多次，且传入了不同的参数。这是一个令人抓狂的问题。毕竟工程一大，参与的人一多，时间已久，什么问题都会出来。有不少开源项目都使用 docstring 来标注类型

type hint 和 mypy 的出现，改变了这一窘况

```Python
def func(a: int, b: int) -> int:
    return a + b

func('a', 'b')

# c.py:4: error: Argument 1 to "func" has incompatible type "str"; expected "int"
```

但是 Python 最初并没有想到这一步，就如同 Python2 的编码一样。不区分 `char` 和 `str` 引入了新的问题

```Python
In [1]: from collections import Sequence

In [2]: isinstance('c', Sequence)
Out[2]: True

In [3]: isinstance('ss', Sequence)
Out[3]: True
```

`c` 和 `ss` 都是 `str` 且均被认为是 `Sequence` 类型。这导致了 `Sequence[str]` 和 `str` 等价。比如你想指定参数是一个字符串的序列，`Sequence[str]`

```Python
from typing import Sequence


def func(a: Sequence[str]) -> str:
    return a[0] if len(a) else ''


func('ss')
```

`mypy` 并不能检查出这类问题，因为 `ss` 是 `Sequence` 并且每一个元素也是 `str` 。所以这里只能写 `List[str]`

    