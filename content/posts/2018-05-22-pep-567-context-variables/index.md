
+++
title = "PEP 567 Context Variables"
summary = ''
description = ""
categories = []
tags = []
date = 2018-05-22T00:13:39+08:00
draft = false
+++

`contextvars` 于 Python 3.7 纳入标准库，之前的版本可以使用 [backport](https://github.com/MagicStack/contextvars)，但是不支持 `asyncio`。为什么这么说呢，看下文你就明白了

应用场景如下

- Context managers like `decimal` contexts and `numpy.errstate`.
- Request-related data, such as security tokens and request data in web applications, language context for `gettext`, etc.
- Profiling, tracing, and logging in large code bases.

`contextvars` 可以起到和 `threading.local` 相同的功能。不过个人认为 `contextvars` 的出现是为了给出一个统一的 context 概念及其解决方法。原来我们在线程并发模型中我们有 TLS 可以使用，但是协程并发模型并没有一个类似的东西。`asycnio` 在 Python 3.7 中对 `contextvars` 进行了支持。这样的好处就是我们可以实现一个 `flask.request` 的全局变量了

关于将 `threading.local` 的代码转换成 `contextvars` 可以参考 [converting-code-that-uses-threading-local](https://www.python.org/dev/peps/pep-0567/#converting-code-that-uses-threading-local) 。好吧，这移植并不是很轻松的，因为他们的 API 不一样。[PEP 550](https://www.python.org/dev/peps/pep-0550/#replication-of-threading-local-interface) 中解释了为什么不去搞一个和 `threading.local` 相同的 API


`contextvars` 的基本用法可以从 [文档](https://docs.python.org/3.7/library/contextvars.html) 上得知。一个小例子，可以感受一下 context 的概念

```Python
from contextvars import ContextVar, copy_context

var = ContextVar('var')
var.set('init')

def hello():
    assert var.get() == 'init'
    var.set('hello')
    assert var.get() == 'hello'
    bye()
    assert var.get() == 'bye'

def bye():
    assert var.get() == 'hello'
    var.set('bye')
    assert var.get() == 'bye'

ctx = copy_context()
ctx.run(hello)    # run Python code in specified context
assert ctx.get(var) == 'bye'
assert var.get() == 'init'
```

OK，看完上面的代码我们大致也可以猜到 `asyncio` 是怎么做的了

```Python
def call_soon(self, callback, *args, context=None):
    if context is None:
        context = contextvars.copy_context()

    # ... some time later
    context.run(callback, *args)
```

```Python
class Task:
    def __init__(self, coro):
        ...
        # Get the current context snapshot.
        self._context = contextvars.copy_context()
        self._loop.call_soon(self._step, context=self._context)
```

实现原理直接参考 [PR](https://github.com/python/cpython/pull/5027/files)

首先是线程状态对象中添加了 `context` 这个 field

```
// Include/pystate.h
typedef struct _ts {
    // ...
    PyObject *context;
    uint64_t context_ver;
    uint64_t id;
} PyThreadState;
```

```
// Include/internal/context.h
struct _pycontextobject {  // 即 PyContext
    PyObject_HEAD
    PyContext *ctx_prev;
    PyHamtObject *ctx_vars;
    PyObject *ctx_weakreflist;
    int ctx_entered;
};

struct _pycontextvarobject {  // 即 PyContextVar
    PyObject_HEAD
    PyObject *var_name;
    PyObject *var_default;
    PyObject *var_cached;
    uint64_t var_cached_tsid;  // 根据 tstate->id 和 tstate->context_ver 来进行缓存查找内容
    uint64_t var_cached_tsver;
    Py_hash_t var_hash;  // Hash 缓存
};
```

`contextvars.copy_context` 的实现

```
// Python/context.c
static inline PyContext *
context_get(void)
{
    PyThreadState *ts = PyThreadState_Get();
    PyContext *current_ctx = (PyContext *)ts->context;
    if (current_ctx == NULL) {
        current_ctx = context_new_empty();
        if (current_ctx == NULL) {
            return NULL;
        }
        ts->context = (PyObject *)current_ctx;
    }
    return current_ctx;
}

static PyContext *
context_new_from_vars(PyHamtObject *vars)
{
    PyContext *ctx = _context_alloc();
    if (ctx == NULL) {
        return NULL;
    }

    Py_INCREF(vars);
    ctx->ctx_vars = vars;

    _PyObject_GC_TRACK(ctx);
    return ctx;
}

PyContext *
PyContext_CopyCurrent(void)
{
    PyContext *ctx = context_get();
    if (ctx == NULL) {
        return NULL;
    }

    return context_new_from_vars(ctx->ctx_vars);
}
```

从当前的线程状态对象获取 `context` 然后创建一个新的对象并将 `ctx_vars` 指向被复制的 `ctx_vars`。我在这里提醒一下目前复制出来的 `context` 和原 `context` 的 `ctx_vars` 指向的是同一个结构体

`contextvars.Context.run` 的实现

```
// Python/context.c
static PyObject *
context_run(PyContext *self, PyObject *const *args,
            Py_ssize_t nargs, PyObject *kwnames)
{
    // ...
    if (PyContext_Enter(self)) {
        return NULL;
    }

    PyObject *call_result = _PyObject_FastCallKeywords(
        args[0], args + 1, nargs - 1, kwnames);  // 调用传入的函数

    if (PyContext_Exit(self)) {
        return NULL;
    }

    return call_result;
}
```

这个函数和 Python 的 Context Manager 完全是一个套路

```
// Python/context.c
int
PyContext_Enter(PyContext *ctx)
{
    if (ctx->ctx_entered) {
        PyErr_Format(PyExc_RuntimeError,
                     "cannot enter context: %R is already entered", ctx);
        return -1;
    }

    PyThreadState *ts = PyThreadState_Get();

    ctx->ctx_prev = (PyContext *)ts->context;  /* borrow */
    ctx->ctx_entered = 1;

    Py_INCREF(ctx);
    ts->context = (PyObject *)ctx;
    ts->context_ver++;

    return 0;
}

int
PyContext_Exit(PyContext *ctx)
{
    if (!ctx->ctx_entered) {
        PyErr_Format(PyExc_RuntimeError,
                     "cannot exit context: %R has not been entered", ctx);
        return -1;
    }

    PyThreadState *ts = PyThreadState_Get();

    if (ts->context != (PyObject *)ctx) {
        /* Can only happen if someone misuses the C API */
        PyErr_SetString(PyExc_RuntimeError,
                        "cannot exit context: thread state references "
                        "a different context object");
        return -1;
    }

    Py_SETREF(ts->context, (PyObject *)ctx->ctx_prev);
    ts->context_ver++;

    ctx->ctx_prev = NULL;
    ctx->ctx_entered = 0;

    return 0;
}
```

我们可以清楚的看到它是在置换掉当前线程状态对象的 `context`

`ContextVar.get` 的实现

```
// Python/context.c
int
PyContextVar_Get(PyContextVar *var, PyObject *def, PyObject **val)
{
    assert(PyContextVar_CheckExact(var));

    PyThreadState *ts = PyThreadState_Get();
    if (ts->context == NULL) {
        goto not_found;
    }

    if (var->var_cached != NULL &&
            var->var_cached_tsid == ts->id &&
            var->var_cached_tsver == ts->context_ver)  // 缓存命中
    {
        *val = var->var_cached;
        goto found;
    }

    assert(PyContext_CheckExact(ts->context));
    PyHamtObject *vars = ((PyContext *)ts->context)->ctx_vars;

    PyObject *found = NULL;
    int res = _PyHamt_Find(vars, (PyObject*)var, &found);
    if (res < 0) {
        goto error;
    }
    if (res == 1) {
        assert(found != NULL);
        var->var_cached = found;  /* borrow */  // 刷新缓存
        var->var_cached_tsid = ts->id;
        var->var_cached_tsver = ts->context_ver;

        *val = found;
        goto found;
    }

not_found:
    if (def == NULL) {
        if (var->var_default != NULL) {
            *val = var->var_default;
            goto found;
        }

        *val = NULL;
        goto found;
    }
    else {
        *val = def;
        goto found;
   }

found:
    Py_XINCREF(*val);
    return 0;

error:
    *val = NULL;
    return -1;
}
```

`ContextVar.set` 的实现

```
// Python/context.c
PyContextToken *
PyContextVar_Set(PyContextVar *var, PyObject *val)
{
    if (!PyContextVar_CheckExact(var)) {
        PyErr_SetString(
            PyExc_TypeError, "an instance of ContextVar was expected");
        return NULL;
    }

    PyContext *ctx = context_get();  // 获取当前的 Context
    if (ctx == NULL) {
        return NULL;
    }

    PyObject *old_val = NULL;
    int found = _PyHamt_Find(ctx->ctx_vars, (PyObject *)var, &old_val);  // 查询之前存储的值
    if (found < 0) {
        return NULL;
    }

    Py_XINCREF(old_val);
    PyContextToken *tok = token_new(ctx, var, old_val);  // 生成 Token 对象，持有旧值
    Py_XDECREF(old_val);

    if (contextvar_set(var, val)) {
        Py_DECREF(tok);
        return NULL;
   }

    return tok;
}
```

`contextvar_set` 是核心实现

```
// Python/context.c
static int
contextvar_set(PyContextVar *var, PyObject *val)
{
    var->var_cached = NULL;
    PyThreadState *ts = PyThreadState_Get();

    PyContext *ctx = context_get();
    if (ctx == NULL) {
        return -1;
    }

    PyHamtObject *new_vars = _PyHamt_Assoc(
        ctx->ctx_vars, (PyObject *)var, val);
    if (new_vars == NULL) {
        return -1;
    }

    Py_SETREF(ctx->ctx_vars, new_vars);

    var->var_cached = val;  /* borrow */
    var->var_cached_tsid = ts->id;
    var->var_cached_tsver = ts->context_ver;
    return 0;
}
```

OK，这里解决了我们刚才的疑惑，`copy_context` 后会导致 `ctx_vars` 被新旧两个 `context` 同时引用。但是 `contextvar_set` 并不是直接对 `ctx_vars` 进行修改，因为这样显然会影响到被复制的那个 `context`。它的做法是生成了一个新的 `ctx_vars`。关于 HAMT 本人后续会进行跟进

### Summary

- `Context` 对象和 `PyThreadState` 进行绑定
- `Context` 中的 `ctx_vars` 域为 `ContextVar` 和对应 value 的关系数组(可以理解为字典)
- `Context.run` 的原理是替换掉当前线程对象的 `context` 然后执行目标函数，最后再还原 `context`
- `ContextVar.set` 和 `ContextVar.get` 均会从当前线程对象的 `context->ctx_vars` 中取出关联数组，然后将自身作为 key 去查找 value
- `ContextVar` 的 `var_cached` 域会缓存对应的 value，前提条件是 `var_cached_tsid == tstate->id && var_cached_tsver == tstate->context_ver`

### Reference

[std-doc: contextvars](https://docs.python.org/3.7/library/contextvars.html)  
[PEP 550 -- Execution Context](https://www.python.org/dev/peps/pep-0550/)  
[PEP 567 -- Context Variables](https://www.python.org/dev/peps/pep-0567/)  

    