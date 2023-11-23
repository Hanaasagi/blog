
+++
title = "你好，类型"
summary = ''
description = ""
categories = []
tags = []
date = 2018-06-04T05:42:33+08:00
draft = false
+++

简单地列一下最近在个人项目中使用 mypy 的一些体会，import cycles 这种常见的问题就不再多提了。另外本文所使用的 mypy 版本为 0.600。因为版本变动比较大，可能有些问题以后并不会复现

#### 艰难的选择

```Python
def set_non_blocking(fd: int) -> None:
    flags = fcntl.fcntl(fd, fcntl.F_GETFL) | os.O_NONBLOCK
    fcntl.fcntl(fd, fcntl.F_SETFL, flags)
```

这段代码传入 `socket.socket` 也是可以工作的。但正确地讲，应当传入 `sock.fileno()` 的。等等我刚才说的对么？

其实这段代码细究一下还是比较有意思的。根据 CPython 3.6.3 的源代码来说，所有传入 `fcntl.fcntl` 的第一个参数(即`fd`)都会先被 `conv_descriptor` 处理。其内部实现就是把 Python 的外衣脱了然后调用 FCNTL(2)

```
// Modules/fcntlmodule.c
static int
conv_descriptor(PyObject *object, int *target)
{
    int fd = PyObject_AsFileDescriptor(object);

    if (fd < 0)
        return 0;
    *target = fd;
    return 1;
}
```

`PyObject_AsFileDescriptor` 的作用见注释

```
// Objects/fileobject.c
/* Try to get a file-descriptor from a Python object.  If the object
   is an integer, its value is returned.  If not, the
   object's fileno() method is called if it exists; the method must return
   an integer, which is returned as the file descriptor value.
   -1 is returned on failure.
*/

int
PyObject_AsFileDescriptor(PyObject *o)
{
    int fd;
    PyObject *meth;
    _Py_IDENTIFIER(fileno);

    if (PyLong_Check(o)) {
        fd = _PyLong_AsInt(o);
    }
    else if ((meth = _PyObject_GetAttrId(o, &PyId_fileno)) != NULL)
    {
        PyObject *fno = PyEval_CallObject(meth, NULL);
        Py_DECREF(meth);
        if (fno == NULL)
            return -1;

        if (PyLong_Check(fno)) {
            fd = _PyLong_AsInt(fno);
            Py_DECREF(fno);
        }
        else {
            PyErr_SetString(PyExc_TypeError,
                            "fileno() returned a non-integer");
            Py_DECREF(fno);
            return -1;
        }
    }
    else {
        PyErr_SetString(PyExc_TypeError,
                        "argument must be an int, or have a fileno() method.");
        return -1;
    }

    if (fd == -1 && PyErr_Occurred())
        return -1;
    if (fd < 0) {
        PyErr_Format(PyExc_ValueError,
                     "file descriptor cannot be a negative integer (%i)",
                     fd);
        return -1;
    }
    return fd;
}
```

可见 `fcntl.fcntl` 实际上是可以传入两种类型的参数。这就如同我们在某个函数中先判断传入的参数是 `str`，还是 `int`，如果是则转成 `str` 的那些套路一样。实际上这种问题可以归为

- 允许传入多种类型，但在代码中尝试类型转换
- 参数类型单一，在传入前进行转换

因为编程水平有限，我无法衡量出哪种更具优势。但在代码中我将 `set_non_blocking` 的参数统一为 `int` 了

下面的代码也会出现类似的问题

```Python
try:
    1 / 0
except ZeroDivisionError as e:
    logger.error(e)
```

`e` 是一个 `Exception` 子类的实例。`logger.error` 却接收的是一个 `str`。这里我选择 `# type: ignore`

#### 内置的坑

上一段简单的代码

```Python
import socket

def create_nonblock_sock(addr):
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    # ...
    return sock
```

这个函数返回什么，如果你使用 monkeytype 去追踪运行时类型则会得到下面的结果

```Python
import socket

from socket import socket
from typing import Tuple

def create_listening_sock(addr: Tuple[str, int]) -> socket:
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    # ...
    return sock
```

这显然不能 work 了，为什么会产生这种结果呢。因为 `socket.socket` 是一个类而且恰巧它的类名和模块名相同。WTF，如果代码遵循 PEP8 规范则应该不会出现这种问题。我在代码中是使用 `from socket import socket as socket_t` 来解决的

#### 尚有不足

```Python
from collections import UserDict


class Config(UserDict):

    def t(self) -> None:
        self.data['x'] = 'y'  # line 7
```

上面的代码会报 `t.py:7: error: Unsupported target for indexed assignment`

这个是因为 `mypy` 所依赖的 `typeshed` 有个 bug

```Python
# https://github.com/python/typeshed/blob/2d4bb04ab3946e08b6a8802b03c9552905d8b99d/stdlib/3/collections/__init__.pyi#L83
class UserDict(MutableMapping[_KT, _VT]):
    data: Mapping[_KT, _VT]
    def __init__(self, dict: Optional[Mapping[_KT, _VT]] = ..., **kwargs: _VT) -> None: ...
    def __len__(self) -> int: ...
    def __getitem__(self, key: _KT) -> _VT: ...
```

这里 `data` 的类型是 `Mapping[_KT, _VT]`，它是只读的。所以应当改为 `Dict[_KT, _VT]`，目前 master 分支中已经修复

    