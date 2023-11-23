
+++
title = "环境变量那些事"
summary = ''
description = ""
categories = []
tags = []
date = 2019-08-05T09:52:16+08:00
draft = false
+++

本文所使用代码为 CPython 3.7.4，commit sha 为 `e09359112e250268eca209355abeb17abf822486`

### About Environ Variable

每个程序都接收到一张 *环境表*。其是一个字符指针数组，其中每个指针包含一个以 `NULL` 结束的 C 字符串的地址。全局变量 `environ` 则包含了该这指针数组的地址。按照惯例环境变量由 `name=value` 格式的字符串组成

```
 environ 指针          环境表
+----------+       +----------+
|          |  -->  |          | -> HOME=/root
+----------+       |----------|
                   |          | -> PATH=/root/.cargo/bin
                   |----------|
                   |   NULL   |
                   +----------+
```

```
#include <stdio.h>

extern char ** environ;

int main()
{
    char ** envir = environ;

    while(*envir) {
        printf("%s\n",*envir);
        envir++;
    }
    return 0;
}
```

环境表通常存储在进程地址空间的顶部，位于栈的上面。所以如果我们需要添加新的环境变量则需要通过 `malloc` 为新的环境表分配空间，然后将原来的环境表复制过来。并将指向新的 `name=value` 字符串的指针存放在新环境表的尾部，然后在此项的后面添加 `NULL` 表示结束。最后需要使 `environ` 指向新的环境表。如果之前已经进行过此操作，那么只需要 `realloc` 即可

Kernel 会将进程启动时的环境变量通过伪文件系统 `/proc/{pid}/environ` 暴露出来。但是如果进程在运行时修改了环境变量(`execve(2)` 之后)，比如使用 `putenv` 或者直接修改 `environ`。那么此文件不会反映出这些变化

环境变量的相关操作不是线程安全的，另外在运行时修改环境变量是一种糟糕的做法。比如 PyInstaller 在 3.5 版本引入的 `certifi` runtime hook，会导致强行给用户添加一个 `SSL_CERT_FILE` 的环境变量，虽然 `SSL_CERT_FILE` 是 OpenSSL 所使用的。参考 [代码](https://github.com/pyinstaller/pyinstaller/blob/d37b36a34ad332a4c6c5b04e255660f19f7df1a8/PyInstaller/loader/rthooks/pyi_rth_certifi.py#L15-L16)

### Environ In CPython

在 Python 里环境变量的相关操作大概是下面这么多了

```Python
import os
import uuid
name = uuid.uuid4().hex

assert name not in os.environ
assert os.getenv(name) is None

os.putenv(name, 'test')
assert name not in os.environ  # Attention
assert os.getenv(name) is None

os.environ[name] = 'test'
assert name in os.environ
assert os.getenv(name) == 'test'

os.unsetenv(name)
assert name in os.environ
assert os.getenv(name) == 'test'
```

值得注意的是 `os.putenv` 和 `os.unsetenv` 是不会修改 `os.environ` 的。参考文档

> When `putenv()` is supported, assignments to items in os.environ are automatically translated into corresponding calls to `putenv()`; however, calls to `putenv()` don’t update `os.environ`, so it is actually preferable to assign to items of `os.environ`.

> When `unsetenv()` is supported, deletion of items in os.environ is automatically translated into a corresponding call to `unsetenv()`; however, calls to `unsetenv()` don’t update `os.environ`, so it is actually preferable to delete items of `os.environ`.


`os.environ` 定义在 `os` 模块中，为 `_Environ` 类型。在模块初始化的时候通过 `_createenviron` 生成

```Python
# /Lib/os.py
def _createenviron():
    # Where Env Var Names Can Be Mixed Case
    encoding = sys.getfilesystemencoding()
    def encode(value):
        if not isinstance(value, str):
            raise TypeError("str expected, not %s" % type(value).__name__)
        return value.encode(encoding, 'surrogateescape')
    def decode(value):
        return value.decode(encoding, 'surrogateescape')
    encodekey = encode
    data = environ  # Here
    return _Environ(data,
        encodekey, decode,
        encode, decode,
        _putenv, _unsetenv)

# unicode environ
environ = _createenviron()
del _createenviron
```

`_createenviron` 中所引用的 `environ` 是从 `posix` 模块 `import` 进来的，是在模块出初始化的时候生成的一个 `Dict` 对象

```
// Modules/posixmodule.c
PyMODINIT_FUNC
INITFUNC(void)
{
    PyObject *m, *v;

    m = PyModule_Create(&posixmodule);
    if (m == NULL)
        return NULL;

    /* Initialize environ dictionary */
    v = convertenviron();
    Py_XINCREF(v);
    if (v == NULL || PyModule_AddObject(m, "environ", v) != 0)
        return NULL;
    Py_DECREF(v);

    // ...
}
```

`convertenviron` 定义如下

```
extern char **environ;

static PyObject *
convertenviron(void)
{
    PyObject *d;
    char **e;

    d = PyDict_New();
    if (d == NULL)
        return NULL;

    if (environ == NULL)
        return d;
    /* This part ignores errors */
    for (e = environ; *e != NULL; e++) {
        PyObject *k;
        PyObject *v;
        const char *p = strchr(*e, '=');
        if (p == NULL)
            continue;
        k = PyBytes_FromStringAndSize(*e, (int)(p-*e));
        if (k == NULL) {
            PyErr_Clear();
            continue;
        }
        v = PyBytes_FromStringAndSize(p+1, strlen(p+1));
        if (v == NULL) {
            PyErr_Clear();
            Py_DECREF(k);
            continue;
        }
        if (PyDict_GetItem(d, k) == NULL) {
            if (PyDict_SetItem(d, k, v) != 0)
                PyErr_Clear();
        }
        Py_DECREF(k);
        Py_DECREF(v);
    }
    return d;
}
```

可以看到 `environ` 是 `posix` 模块初始化时 `environ`(C) 的一个快照。让我们回过头来看一下 `_Environ` 类型的定义

```Python
class _Environ(MutableMapping):
    def __init__(self, data, encodekey, decodekey, encodevalue, decodevalue, putenv, unsetenv):
        self.encodekey = encodekey
        self.decodekey = decodekey
        self.encodevalue = encodevalue
        self.decodevalue = decodevalue
        self.putenv = putenv
        self.unsetenv = unsetenv
        self._data = data

    def __getitem__(self, key):
        try:
            value = self._data[self.encodekey(key)]
        except KeyError:
            # raise KeyError with the original key value
            raise KeyError(key) from None
        return self.decodevalue(value)

    def __setitem__(self, key, value):
        key = self.encodekey(key)
        value = self.encodevalue(value)
        self.putenv(key, value)
        self._data[key] = value

    def __delitem__(self, key):
        encodedkey = self.encodekey(key)
        self.unsetenv(encodedkey)
        try:
            del self._data[encodedkey]
        except KeyError:
            # raise KeyError with the original key value
            raise KeyError(key) from None

    # ...
```

继承了 `MutableMapping` 而不是 `dict`，这是 Python 推荐的实现自定义 Dict 类型的方式。这里我们关注 `__setitem__` 和 `__delitem__` 两个方法。可以看到当我们直接修改 `os.environ` 时，会自动调用 `os.putenv` 和 `os.unsetenv`。另外我们也需要注意这里的 `encode` 和 `decode` 指定了 `surrogateescape`，利用 Surrogate 码位保存无法解码的字节

下面让我们看一下 `putenv` 的实现

```
static PyObject *
os_putenv_impl(PyObject *module, PyObject *name, PyObject *value)
{
    PyObject *bytes = NULL;
    char *env;
    const char *name_string = PyBytes_AS_STRING(name);
    const char *value_string = PyBytes_AS_STRING(value);

    if (strchr(name_string, '=') != NULL) {
        PyErr_SetString(PyExc_ValueError, "illegal environment variable name");
        return NULL;
    }
    bytes = PyBytes_FromFormat("%s=%s", name_string, value_string);
    if (bytes == NULL) {
        return NULL;
    }

    env = PyBytes_AS_STRING(bytes);
    if (putenv(env)) {
        Py_DECREF(bytes);
        return posix_error();
    }

    posix_putenv_garbage_setitem(name, bytes);
    Py_RETURN_NONE;
}
```

将 `name` 和 `value` 拼接成 `{name}={value}` 的形式，然后通过 C 的 `putenv` 设置环境变量。`posix_putenv_garbage_setitem` 是为了重复设置相同 `name` 的环境变量造成内存泄露。原来这里是直接 `malloc` 的字符串，后来改成创建 `PyString`/`PyBytes` 对象，然后将此对象转换成 C 字符串。创建出来的对象会放在 Dict 对象 `posix_putenv_garbage` 中。因为持有引用计数所以此对象不会被释放，当我们为相同的 `name` 设置新的 `value` 的时候，也会修改 `posix_putenv_garbage`，使得先前的对象引用计数变为 0，通过 Python 的 GC 机制回收内存。参考 [https://github.com/python/cpython/commit/762e206706b5fbf4d541006b8f2b0ca17cac6240](https://github.com/python/cpython/commit/762e206706b5fbf4d541006b8f2b0ca17cac6240
) 和源代码

```
/* Save putenv() parameters as values here, so we can collect them when they
 * get re-set with another call for the same key. */
static PyObject *posix_putenv_garbage;

static void
posix_putenv_garbage_setitem(PyObject *name, PyObject *value)
{
    /* Install the first arg and newstr in posix_putenv_garbage;
     * this will cause previous value to be collected.  This has to
     * happen after the real putenv() call because the old value
     * was still accessible until then. */
    if (PyDict_SetItem(posix_putenv_garbage, name, value))
        /* really not much we can do; just leak */
        PyErr_Clear();
    else
        Py_DECREF(value);
}
```

`unsetenv` 也是调用的 C 的 `unsetenv` 然后使用 `PyDict_DelItem` 从 `posix_putenv_garbage` 删除对应的项。这里便不再赘述了


### Inheritance

In the unix model, starting another program involves two primivites:

- `fork()` creates an (almost) identical copy of the calling process. The new process is called the child process and the original process is called the parent process. The child process runs the same code as the original, has the same permissions, has the same environment, and receives a copy of the mutable data memory of the parent process. The most visible difference between the two processes is that they have different process IDs and different parent process IDs (the child's PPID is the parent's PID).
- `execve()` replaces the code and data of the current process by code and data loaded from an executable file. This system call takes the new environment of the process as an argument.

Most high-level functions built around fork() and execve() pass the process's current environment to execve(). Thus, unless the process changes its own environment or calls execve() directly, the called program will inherit the calling program's environment.

From [Exception of inheritance of environment variables](https://unix.stackexchange.com/questions/17758/exception-of-inheritance-of-environment-variables)

### Reference
[Python 3 的 surrogateescape](http://timothyqiu.com/archives/surrogateescape-in-python-3/)

    