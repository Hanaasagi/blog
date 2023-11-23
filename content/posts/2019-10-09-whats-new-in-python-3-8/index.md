
+++
title = "What's new in Python 3.8"
summary = ''
description = ""
categories = []
tags = []
date = 2019-10-09T13:56:03+08:00
draft = false
+++

根据 [PEP-569](https://www.python.org/dev/peps/pep-0569/) 的计划，Python 3.8.0 final 将于本月的 14 号发布。本文将概括性地说一下 Python 3.8 有哪些变化。重要的 feature，比如 [PEP-572](https://www.python.org/dev/peps/pep-0572/)，便不再细致地讨论，有兴趣可以看一下相关 PEP 的讨论

### New Feature

#### Assignment expressions

引入 `:=` 海象运算符(walrus operator)。详细参考 [PEP-572](https://www.python.org/dev/peps/pep-0572/)，里面有详细记载为什么是 `:=`，及为什么不引入新的作用域

#### Positional-only parameters

目的有三:

- allows pure Python functions to fully emulate behaviors of existing C coded functions
- preclude keyword arguments when the parameter name is not helpful.
- marking a parameter as positional-only is that it allows the parameter name to be changed in the future without risk of breaking client code.

下面这个例子比较有趣

```Python
>>> def f(a, b, /, **kwargs):
...     print(a, b, kwargs)
...
>>> f(10, 20, a=1, b=2, c=3)         # a and b are used in two ways
10 20 {'a': 1, 'b': 2, 'c': 3}
```

#### Parallel filesystem cache for compiled bytecode files

允许通过 `PYTHONPYCACHEPREFIX` 环境变量或者 `-X` 参数来自定义字节码缓存文件的位置，默认为 `__pycache__`

#### Debug build uses the same ABI as release build

Python now uses the same ABI whether it built in release or debug mode. On Unix, when Python is built in debug mode, it is now possible to load C extensions built in release mode and C extensions built using the stable ABI.

#### f-strings support = for self-documenting expressions and debugging

f-strings 添加了 `=` 修饰符，便于 debug 使用

```Python
>>> print(f'{theta=}  {cos(radians(theta))=:.3f}')
theta=30  cos(radians(theta))=0.866
```

#### PEP 587: Python Initialization Configuration

提供更多的 C API 来配置 Python 的初始化，参考 [PEP 587](https://www.python.org/dev/peps/pep-0587)

#### Vectorcall: a fast calling protocol for CPython

[PEP 590](https://www.python.org/dev/peps/pep-0590/) introduces a new C API to optimize calls of objects. It introduces a new "vectorcall" protocol and calling convention. This is based on the "fastcall" convention, which is already used internally by CPython. The new features can be used by any user-defined extension class.

Most of the new API is private in CPython 3.8. The plan is to finalize semantics and make it public in Python 3.9.

#### Pickle protocol 5 with out-of-band data buffers

`pickle` 提供新的协议用于提速大对象的传输

### Other Language Changes

- `continue` 可以在 `finally` 中使用了。之前 [PEP-601](https://www.python.org/dev/peps/pep-0601/) 中建议禁止在 `finally` 中使用 `break`/`return`/`continue` 但是此提议已经被拒绝了
- `bool`, `int`, `fractions.Fraction` 类型增加 `as_integer_ratio` 方法
- `int`, `float`, `complex` 有了新的 dunder method `__index__`。Called to implement `operator.index()`, and whenever Python needs to losslessly convert the numeric object to an integer object (such as in slicing, or in the built-in `bin()`, `hex()` and `oct()` functions). Presence of this method indicates that the numeric object is an integer type. Must return an integer.
- 正则表达式添加 `\N{name}`，它会被扩展成对应的 Unicode 字符。比如 `\N{EM DASH}` 会匹配 `—`(这个字符不是 `-`)。关于 Em dash 字符可以参考 https://www.thepunctuationguide.com/em-dash.html
- Dict 和 dictviews 可以使用 `reversed()` 来以插入相反的顺序进行迭代
- 严格限制关键字参数的用法，比如 `f((a)=1)` 现在已经是不合法的了

- `yield` 和 `return` 语句中对 iterable 的 unpack 操作不再需要圆括号了

```Python
def parse(family):
    lastname, *members = family.split()
    # return lastname.upper(), (*members)
    # return (lastname.upper(), *members)
    return lastname.upper(), *members
```

- 对于代码中缺少逗号的情况，编译器会友情提醒 `SyntaxWarning`

```python
# 3.7
>>> [(10, 20) (30, 40)]
Traceback (most recent call last):
  File "<stdin>", line 1, in <module>
TypeError: 'tuple' object is not callable

# 3.8
>>> [(10, 20) (30, 40)]
<stdin>:1: SyntaxWarning: 'tuple' object is not callable; perhaps you missed a comma?
Traceback (most recent call last):
  File "<stdin>", line 1, in <module>
TypeError: 'tuple' object is not callable
```

- `datetime.date` 或 `datetime.datetime` 的子类对象与 `datetime.timedelta` 对象进行运算时现在返回的是子类而不是父类对象。这个行为也影响到了一些间接利用 `datetime.timedelta` 进行运算的实现，比如 `datetime.datetime.astimezone()`

```Python
from datetime import datetime, timezone

class DateTimeSubclass(datetime):
   pass

dt = DateTimeSubclass(2012, 1, 1)
dt2 = dt.astimezone(timezone.utc)
assert type(dt) is type(dt2)  # 3.8 下成立
```

- 由于 `SIGINT` 信号导致的退出的退出码改变了，有利于判断进程是否是由于 `Ctrl-C` 造成的退出

```
(py38) sagiri ➜  ~ python t.py
^CTraceback (most recent call last):
  File "t.py", line 4, in <module>
    time.sleep(1)
KeyboardInterrupt
(py38) sagiri ➜  ~ echo $?
130
(py38) sagiri ➜  ~ python3.7 t.py
^CTraceback (most recent call last):
  File "t.py", line 4, in <module>
    time.sleep(1)
KeyboardInterrupt
(py38) sagiri ➜  ~ echo $?
1
```

- `code` 对象现在提供了 `replace` 方法去复制一个新的 `code` 对象后修改一些参数

```python
>>> from statistics import mean
>>> mean(data=[10, 20, 90])
40
>>> mean.__code__ = mean.__code__.replace(co_posonlyargcount=1)  # 通过修改 code 对象，现在我们不能再使用关键字参数
>>> mean(data=[10, 20, 90])
Traceback (most recent call last):
  ...
TypeError: mean() got some positional-only arguments passed as keyword arguments: 'data'
```

- For integers, the three-argument form of the [`pow()`](https://docs.python.org/3.8/library/functions.html#pow) function now permits the exponent to be negative in the case where the base is relatively prime to the modulus. It then computes a modular inverse to the base when the exponent is `-1`, and a suitable power of that inverse for other negative exponents.  For example, to compute the [modular multiplicative inverse](https://en.wikipedia.org/wiki/Modular_multiplicative_inverse) of 38 modulo 137, write:

```
>>> pow(38, -1, 137)
119
>>> 119 * 38 % 137
1
```

  Modular inverses arise in the solution of [linear Diophantine equations](https://en.wikipedia.org/wiki/Diophantine_equation). For example, to find integer solutions for `4258𝑥 + 147𝑦 = 369`, first rewrite as `4258𝑥 ≡ 369 (mod 147)` then solve:

```
>>> x = 369 * pow(4258, -1, 147) % 147
>>> y = (4258 * x - 369) // -147
>>> 4258 * x + 147 * y
369
```

- Dict comprehensions 和 dict literals 的计算方式统一了，先计算 key 后计算 value

```python
# 3.7，先 ask actor 后 ask role
>>> cast = {input('role? '): input('actor? ') for i in range(1)}
actor? Chapman
role? King Arthur

# 3.8
>>> cast = {input('role? '): input('actor? ') for i in range(1)}
role? King Arthur
actor? Chapman
```

这样便保证了在使用 `:=` 的时候可以 `cast = {(n := input('role? ')): n for i in range(1)}`

### New Modules

- 新增 `importlib.metadata` 用于提取第三方库的 metadata

### Improved Modules

详细请参考 https://docs.python.org/3.8/whatsnew/3.8.html#improved-modules

这里仅列出几个本人比较感兴趣的

- `ast.parse()` 增强
  - `type_comments=True` causes it to return the text of [**PEP 484**](https://www.python.org/dev/peps/pep-0484) and [**PEP 526**](https://www.python.org/dev/peps/pep-0526) type comments associated with certain AST nodes;
  - `mode='func_type'` can be used to parse [**PEP 484**](https://www.python.org/dev/peps/pep-0484) “signature type comments” (returned for function definition AST nodes);
  - `feature_version=(3, N)` allows specifying an earlier Python 3 version.  (For example, `feature_version=(3, 4)` will treat `async` and `await` as non-reserved words.)

- `compile()` 现在接受 `ast.PyCF_ALLOW_TOP_LEVEL_AWAIT`，可以允许 top-level 的 `await`/`async for`/`async with`。关联 [Python 3.8 中的 asyncio REPL](https://blog.dreamfever.me/2019/06/16/python-3-8-zhong-de-asyncio-repl/)
- `functools.lru_cache()`现在可以直接装饰 callable 了
- `gc.get_objects()` 提供可选的参数 `generation`，可以仅获取指定 generation 中的对象
- `os.path` 下了一些返回 boolean 结果的函数比如 `exists()`,`lexists()`,`isdir()`,`isfile()`,`islink()`,`ismount()` 对于一些在 OS level 中无法正确表示的路径，现在返回 `False` 而不是抛出 `ValueError`

```
# 3.7
>>> import os
>>> os.path.exists('\x00')
Traceback (most recent call last):
  File "<stdin>", line 1, in <module>
  File "/usr/lib/python3.7/genericpath.py", line 19, in exists
    os.stat(path)
ValueError: embedded null byte

# 3.8

>>> import os
>>> os.path.exists('\x00')
False
```

同样的 `pathlib.Path` 下的相同作用的方法也做了此修改

- `socket.create_server()` 支持 v4/v6 双栈了，通过 `dualstack_ipv6 `参数开启此功能
- 新增 `threading.excepthook()` 用于处理 `threading.Thread.run()` 中的未捕获异常
- 新增`threading.get_native_id()` 用于获取内核分配的线程 ID
- 新增 `typing.Protocol`用于定义 interface，方便表达 duck typing。(文档上是 new in 3.8, 但是我记得在 typeshed 中很早就在用了)
- 新增 `typing.TypedDict` 来表达异构(heterogeneous)的 dict。但是这是否是一种合适的行为呢？不好说，其他语言的 dict 貌似都是同构的(homogeneous)。异构直接用 dataclass 不是更好么

```Python
class Point2D(TypedDict):
    x: int
    y: int
    label: str

a: Point2D = {'x': 1, 'y': 2, 'label': 'good'}  # OK
b: Point2D = {'z': 3, 'label': 'bad'}           # Fails type check
```

- 新增 `typing.Literal` ，可以对字面量参数的值进行检测了
- 新增 `typing.Final` 为了
  - Declaring that a method should not be overridden
  - Declaring that a class should not be subclassed
  - Declaring that a variable or attribute should not be reassigned

- `unittest` 新增 `AsyncMock`


### Optimizations

详情参考 https://docs.python.org/3.8/whatsnew/3.8.html#optimizations

### Build and C API Changes

详情参考 https://docs.python.org/3.8/whatsnew/3.8.html#build-and-c-api-changes

### Deprecated

详情参考 https://docs.python.org/3.8/whatsnew/3.8.html#deprecated

### API and Feature Removal

详情参考 https://docs.python.org/3.8/whatsnew/3.8.html#api-and-feature-removals

### Porting to Python 3.8

详情参考 https://docs.python.org/3.8/whatsnew/3.8.html#porting-to-python-3-8

----

下面进入实战篇，讲一下 aiohttp 兼容 Python 3.8 的一些工作。详见 [PR#4056](https://github.com/aio-libs/aiohttp/pull/4056)，主要是根据单元测试来进行修复，可能还有 bug

#### 弃用警告

- `asyncio.coroutine` 已经被弃用，并将所有的 `yield from` 全部替换成了 `await`
- `asyncio.sleep` 的 `loop` 参数被弃用


#### 上游重构导致的不兼容

asyncio 在 Python 3.8 中将所有的异常定义都移动到了 `asyncio.exceptions`。而 aiohttp 在使用这些异常时是非常具体的，像这样 `asyncio.streams.IncompleteReadError` 而不是 `asyncio.IncompleteReadError`。这便导致了在 asyncio 进行重构的时候，破坏了兼容性。本人在写代码的时候也很喜欢使用 `requests.exceptions.HTTPError` 而不是 `requests.HTTPError` 的写法。那么哪一种写法是比较好的呢？本人又想起了以前的一些思考，比如我在使用某个 third-party 时，我是否应该将创建一个文件将所有用到的东西 `import` 进来然后本地代码再从这个文件中 `import` 呢。因为显然如果上游的 API 发生变动，比如参数类型改变，那么显然后直接影响到所有使用此 API 的地方。但是如果我们创建一个适配器层，将上游 API 进行兼容，我们便仅需要改动一个文件。可实际上我几乎没有看到有这样做的

如何写出优雅的可维护的代码对于我来说还是太难了


#### 新功能导致的不兼容

3.8 新增了 `AsyncMock`，但这也导致的单元测试挂了一片

```
>>> mock.AsyncMock
<class 'unittest.mock.AsyncMock'>
>>> m = mock.AsyncMock()
>>> m.called
False
>>> m()
<coroutine object AsyncMockMixin._mock_call at 0x7fa7135bf5c0>
>>> m.called
<stdin>:1: RuntimeWarning: coroutine 'AsyncMockMixin._mock_call' was never awaited
RuntimeWarning: Enable tracemalloc to get the object allocation traceback
False
>>>
```

这个在调用 `__call__` 的时候会去调用 `_mock_call`，而那个 `AsyncMockMixin` 重写了 `_mock_call` 导致返回了 `AsyncMockMixin._mock_call` 使得 `called` 依然为 `False`

解决方法是使用 `new_callable` 或者重写单元测试


#### 看起来没有关系，但是却不兼容的改动

aiohttp 在 `BaseProtocol` 中使用了 `__slots__`，而 3.8 中 `asyncio.Protocol` 也使用了 `__slots__`，参考 https://bugs.python.org/issue35394

这导致了一些地方单元测试挂掉，因为这么写的 QAQ


```
__________________ ERROR at setup of test_shutdown[pyloop]

make_srv = <function make_srv.<locals>.maker at 0x7f991b19faf0>                                
transport = <Mock id='140295561061664'>                                                        

    @pytest.fixture                                                                            
    def srv(make_srv, transport):                                                              
        srv = make_srv()                                                                       
        srv.connection_made(transport)                                                         
        transport.close.side_effect = partial(srv.connection_lost, None)                       
>       srv._drain_helper = mock.Mock()                                                        
E       AttributeError: 'RequestHandler' object attribute '_drain_helper' is read-only         

tests/test_web_protocol.py:45: AttributeError    
```


因为 `__slots__` 的原因我们不能在对象创建后动态地赋予新的 attribute。但这并不是真正的问题，为什么原来使用了 `__slots__` 没有事，直到 `asyncio.Protocol` 也做了相似的操作才出现问题

根据 Python 的 Data Model 中相应[小节](https://docs.python.org/3/reference/datamodel.html#slots)，我们可以看到

- When inheriting from a class without `__slots__`, the `__dict__` and `__weakref__` attribute of the instances will always be accessible.
- Without a `__dict__` variable, instances cannot be assigned new variables not listed in the `__slots__` definition. Attempts to assign to an unlisted variable name raises AttributeError. If dynamic assignment of new variables is desired, then add `'__dict__'` to the sequence of strings in the `__slots__` declaration.
- The action of a `__slots__` declaration is not limited to the class where it is defined. `__slots__` declared in parents are available in child classes. However, child subclasses will get a `__dict__` and `__weakref__` unless they also define `__slots__` (which should only contain names of any additional slots).

大致就是这样了(逃

    