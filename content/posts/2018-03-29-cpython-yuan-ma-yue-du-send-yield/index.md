
+++
title = "CPython 源码阅读 - send/yield"
summary = ''
description = ""
categories = []
tags = []
date = 2018-03-29T11:50:00+08:00
draft = false
+++

*本文代码取自 CPython 3.6.3， commit sha 为 2c5fed86e0cbba5a4e34792b0083128ce659909d*

先放一个 Generator 的示例

```Python
def g():
    yield 2

gen = g()
```

每一 Generator 对象都含有几个特别的属性，可以从万能的 `inspect` 模块的[文档](https://docs.python.org/3/library/inspect.html#types-and-members) 中找到

- `gi_frame`: frame
- `gi_running`: is the generator running?
- `gi_code`: code
- `gi_yieldfrom`: object being iterated by yield from, or None

可以看到每个 Generator 对象都通过 `gi_frame` 属性关联了自己的栈帧。Python 通过 `PyFrameObject` 模拟了操作系统的原始栈帧，他们通过 `f_back` 进行连接。`f_code` 便是待执行的 `PyCodeObject`，而 `f_globals`、`f_locals` 则是执行环境信息

```
// Include/frameobject.h
typedef struct _frame {
    PyObject_VAR_HEAD
    struct _frame *f_back;      /* previous frame, or NULL */
    PyCodeObject *f_code;       /* code segment */
    PyObject *f_builtins;       /* builtin symbol table (PyDictObject) */
    PyObject *f_globals;        /* global symbol table (PyDictObject) */
    PyObject *f_locals;         /* local symbol table (any mapping) */
    PyObject **f_valuestack;    /* points after the last local */
    /* Next free slot in f_valuestack.  Frame creation sets to f_valuestack.
       Frame evaluation usually NULLs it, but a frame that yields sets it
       to the current stack top. */
    PyObject **f_stacktop;
    // ...
    int f_lasti;                /* Last instruction if called */
    // ...
    PyTryBlock f_blockstack[CO_MAXBLOCKS]; /* for try and loop blocks */
    PyObject *f_localsplus[1];  /* locals+stack, dynamically sized */
} PyFrameObject;
```

### Behind the `send` and `next`

Generator 相关 API `send` 和 `next` 的实现如下

```
// Objects/genobject.c
// gen.send(var) 的底层调用
PyObject *
_PyGen_Send(PyGenObject *gen, PyObject *arg)
{
    return gen_send_ex(gen, arg, 0, 0);
}

// next(gen) 的底层调用
static PyObject *
gen_iternext(PyGenObject *gen)
{
    return gen_send_ex(gen, NULL, 0, 0);  // next(gen) 和 gen.send(None) 等价
}
```

核心部分位于 `gen_send_ex`

```
// Objects/genobject.c
static PyObject *
gen_send_ex(PyGenObject *gen, PyObject *arg, int exc, int closing)
{
    PyThreadState *tstate = PyThreadState_GET();
    PyFrameObject *f = gen->gi_frame;
    PyObject *result;

    if (gen->gi_running) {
        char *msg = "generator already executing";
        if (PyCoro_CheckExact(gen)) {
            msg = "coroutine already executing";
        }
        else if (PyAsyncGen_CheckExact(gen)) {
            msg = "async generator already executing";
        }
        PyErr_SetString(PyExc_ValueError, msg);
        return NULL;
    }
    if (f == NULL || f->f_stacktop == NULL) {
        if (PyCoro_CheckExact(gen) && !closing) {
            /* `gen` is an exhausted coroutine: raise an error,
               except when called from gen_close(), which should
               always be a silent method. */
            PyErr_SetString(
                PyExc_RuntimeError,
                "cannot reuse already awaited coroutine");
        }
        else if (arg && !exc) {
            /* `gen` is an exhausted generator:
               only set exception if called from send(). */
            if (PyAsyncGen_CheckExact(gen)) {
                PyErr_SetNone(PyExc_StopAsyncIteration);
            }
            else {
                PyErr_SetNone(PyExc_StopIteration);
            }
        }
        return NULL;
    }
    // f_lasti == -1 意味着初始状态，gen 从未执行过
    if (f->f_lasti == -1) {
        // 第一次不能使用 send 发送非 None 数据
        if (arg && arg != Py_None) {
            char *msg = "can't send non-None value to a "
                        "just-started generator";
            if (PyCoro_CheckExact(gen)) {
                msg = NON_INIT_CORO_MSG;
            }
            else if (PyAsyncGen_CheckExact(gen)) {
                msg = "can't send non-None value to a "
                      "just-started async generator";
            }
            PyErr_SetString(PyExc_TypeError, msg);
            return NULL;
        }
    } else {
        /* Push arg onto the frame's value stack */
        result = arg ? arg : Py_None;
        Py_INCREF(result);
        *(f->f_stacktop++) = result;  // [1] 将通过 `send` 发送的值放入栈顶
    }

    /* Generators always return to their most recent caller, not
     * necessarily their creator. */
    Py_XINCREF(tstate->frame);
    assert(f->f_back == NULL);
    f->f_back = tstate->frame;  // 更改上层栈帧，因为创建 gen 和调用 gen 的可能不是同个栈帧

    gen->gi_running = 1;
    result = PyEval_EvalFrameEx(f, exc);  // [2] 执行 gen 中的 opcode，得到 yield 返回的值
    gen->gi_running = 0;

    /* Don't keep the reference to f_back any longer than necessary.  It
     * may keep a chain of frames alive or it could create a reference
     * cycle. */
    assert(f->f_back == tstate->frame);
    Py_CLEAR(f->f_back);

    /* If the generator just returned (as opposed to yielding), signal
     * that the generator is exhausted. */
    if (result && f->f_stacktop == NULL) {
        // 处理 gen return
        if (result == Py_None) {
            /* Delay exception instantiation if we can */
            if (PyAsyncGen_CheckExact(gen)) {
                PyErr_SetNone(PyExc_StopAsyncIteration);  // 参考 PEP 479
            }
            else {
                PyErr_SetNone(PyExc_StopIteration);
            }
        }
        else {
            /* Async generators cannot return anything but None */
            assert(!PyAsyncGen_CheckExact(gen));
            _PyGen_SetStopIterationValue(result);  // 利用 StopIteration 传递返回值
        }
        Py_CLEAR(result);
    }
    else if (!result && PyErr_ExceptionMatches(PyExc_StopIteration)) {
        // 处理在 gen 中 raise StopIteration 的情况，参考 PEP 479
        /* Check for __future__ generator_stop and conditionally turn
         * a leaking StopIteration into RuntimeError (with its cause
         * set appropriately). */

        const int check_stop_iter_error_flags = CO_FUTURE_GENERATOR_STOP |
                                                CO_COROUTINE |
                                                CO_ITERABLE_COROUTINE |
                                                CO_ASYNC_GENERATOR;
        // 通过 flag 检查是否 from __future__ import generator_stop
        if (gen->gi_code != NULL &&
            ((PyCodeObject *)gen->gi_code)->co_flags &
                check_stop_iter_error_flags)
        {
            /* `gen` is either:
                  * a generator with CO_FUTURE_GENERATOR_STOP flag;
                  * a coroutine;
                  * a generator with CO_ITERABLE_COROUTINE flag
                    (decorated with types.coroutine decorator);
                  * an async generator.
            */
            const char *msg = "generator raised StopIteration";
            if (PyCoro_CheckExact(gen)) {
                msg = "coroutine raised StopIteration";
            }
            else if PyAsyncGen_CheckExact(gen) {
                msg = "async generator raised StopIteration";
            }
            _PyErr_FormatFromCause(PyExc_RuntimeError, "%s", msg);
        }
        else {
            /* `gen` is an ordinary generator without
               CO_FUTURE_GENERATOR_STOP flag.
            */

            PyObject *exc, *val, *tb;

            /* Pop the exception before issuing a warning. */
            PyErr_Fetch(&exc, &val, &tb);
            // Python 3.6 中若在 gen 中 raise StopIteration，则会出现弃用警告
            if (PyErr_WarnFormat(PyExc_DeprecationWarning, 1,
                                 "generator '%.50S' raised StopIteration",
                                 gen->gi_qualname)) {
                /* Warning was converted to an error. */
                Py_XDECREF(exc);
                Py_XDECREF(val);
                Py_XDECREF(tb);
            }
            else {
                PyErr_Restore(exc, val, tb);
            }
        }
    }
    else if (PyAsyncGen_CheckExact(gen) && !result &&
             PyErr_ExceptionMatches(PyExc_StopAsyncIteration))
    {
        /* code in `gen` raised a StopAsyncIteration error:
           raise a RuntimeError.
        */
        const char *msg = "async generator raised StopAsyncIteration";
        _PyErr_FormatFromCause(PyExc_RuntimeError, "%s", msg);
    }
    // 清理工作
    if (!result || f->f_stacktop == NULL) {
        /* generator can't be rerun, so release the frame */
        /* first clean reference cycle through stored exception traceback */
        PyObject *t, *v, *tb;
        t = f->f_exc_type;
        v = f->f_exc_value;
        tb = f->f_exc_traceback;
        f->f_exc_type = NULL;
        f->f_exc_value = NULL;
        f->f_exc_traceback = NULL;
        Py_XDECREF(t);
        Py_XDECREF(v);
        Py_XDECREF(tb);
        gen->gi_frame->f_gen = NULL;
        gen->gi_frame = NULL;
        Py_DECREF(f);
    }

    return result;
}
```

这里简略地说一下 PEP 479 中 Python 做了哪些修改

>This PEP proposes a change to generators: when StopIteration is raised inside a generator, it is replaced it with RuntimeError. (More precisely, this happens when the exception is about to bubble out of the generator's stack frame.) Because the change is backwards incompatible, the feature is initially introduced using a `__future__` statement.

每个版本的处理力度不同

- Python 3.5: Enable new semantics under `__future__` import; silent deprecation warning if StopIteration bubbles out of a generator not under `__future__` import.(`from __future__ import generator_stop`)
- Python 3.6: Non-silent deprecation warning.
- Python 3.7: Enable new semantics everywhere.

### Behind the `yield`

[2] 处执行我们的 gen，通过 `dis` 来查看一下字节码

```Python
In [1]: import dis

In [2]: def g():
   ...:     yield 2
   ...:

In [3]: dis.dis(g)
  2           0 LOAD_CONST               1 (2)
              2 YIELD_VALUE
              4 POP_TOP
              6 LOAD_CONST               0 (None)
              8 RETURN_VALUE
```

参考文档 [Python Bytecode Instructions](https://docs.python.org/3/library/dis.html#python-bytecode-instructions)

- `YIELD_VALUE`:Pops TOS and yields it from a generator.
- `POP_TOP`: Removes the top-of-stack (TOS) item.

`YIELD_VALUE` 和 `POP_TOP` 均将 TOS 出栈，但是我们貌似仅入栈了一个元素。联系 [1] 处我们在执行 gen 前向其栈中放了一个值，所以不会出现从空栈弹出元素的情况

Python 解释器对于字节码 `YIELD_VALUE` 的执行如下，这里仅摘出相关的部分

```
// Python/ceval.c
PyObject *
_PyEval_EvalFrameDefault(PyFrameObject *f, int throwflag)
{
    int opcode;
    PyCodeObject *co;
    co = f->f_code;
    for (;;) {
        switch (opcode) {
            TARGET(YIELD_VALUE) {
                retval = POP();  // TOP 出栈
                // [1]
                if (co->co_flags & CO_ASYNC_GENERATOR) {
                    PyObject *w = _PyAsyncGenValueWrapperNew(retval);
                    Py_DECREF(retval);
                    if (w == NULL) {
                        retval = NULL;
                        goto error;
                    }
                    retval = w;
                }

                f->f_stacktop = stack_pointer;
                why = WHY_YIELD;
                goto fast_yield;
            }
        }
    }
}
```

关于 `co_flags`，可以参考 Python 文档 [Data model](https://docs.python.org/3/reference/datamodel.html) 一节

>co\_flags is an integer encoding a number of flags for the interpreter. The following flag bits are defined for co\_flags: bit 0x04 is set if the function uses the \*arguments syntax to accept an arbitrary number of positional arguments; bit 0x08 is set if the function uses the \*\*keywords syntax to accept arbitrary keyword arguments; bit 0x20 is set if the function is a generator.  
>Future feature declarations (from `__future__` import division) also use bits in co\_flags to indicate whether a code object was compiled with a particular feature enabled: bit 0x2000 is set if the function was compiled with future division enabled; bits 0x10 and 0x1000 were used in earlier versions of Python. Other bits in co\_flags are reserved for internal use.

截止 Python 3.6.3 为止，有以下的 flag，根据名称便能猜出意图

```Python
In [2]: dis.COMPILER_FLAG_NAMES
Out[2]:
{1: 'OPTIMIZED',
 2: 'NEWLOCALS',
 4: 'VARARGS',
 8: 'VARKEYWORDS',
 16: 'NESTED',
 32: 'GENERATOR',
 64: 'NOFREE',
 128: 'COROUTINE',
 256: 'ITERABLE_COROUTINE',
 512: 'ASYNC_GENERATOR'}
```

`ASYNC_GENERATOR` 是 [PEP 525](https://www.python.org/dev/peps/pep-0525/) 引入的异步生成器，本文暂且不考虑这些内容

```Python
In [3]: g.__code__.co_flags & 32
Out[3]: 32

In [4]: g.__code__.co_flags & 512
Out[4]: 0

In [5]: async def g():
   ...:     yield 2
   ...:

In [6]: g.__code__.co_flags & 512
Out[6]: 512
```

Python 提供了更好的 API [dis.show_code](https://docs.python.org/3/library/dis.html#dis.show_code) 去展示这些信息

再来看一下跳转至 `fast_yield` 做了什么

```
// Python/ceval.c
PyObject *
_PyEval_EvalFrameDefault(PyFrameObject *f, int throwflag)
{
fast_yield:
    if (co->co_flags & (CO_GENERATOR | CO_COROUTINE | CO_ASYNC_GENERATOR)) {

        /* The purpose of this block is to put aside the generator's exception
           state and restore that of the calling frame. If the current
           exception state is from the caller, we clear the exception values
           on the generator frame, so they are not swapped back in latter. The
           origin of the current exception state is determined by checking for
           except handler blocks, which we must be in iff a new exception
           state came into existence in this frame. (An uncaught exception
           would have why == WHY_EXCEPTION, and we wouldn't be here). */
        int i;
        // f->f_iblock 为 f->f_blockstack 的最大索引
        // f->f_blockstack 为 PyTryBlock 的数组
        for (i = 0; i < f->f_iblock; i++) {
            // 如果 yield 位于一个 except block 中
            if (f->f_blockstack[i].b_type == EXCEPT_HANDLER) {
                break;
            }
        }
        if (i == f->f_iblock)
            /* We did not create this exception. */
            restore_and_clear_exc_state(tstate, f); [3]
        else
            swap_exc_state(tstate, f);
    }

    // 省略 tacing 部分

exit_eval_frame:
    Py_LeaveRecursiveCall();
    f->f_executing = 0;
    tstate->frame = f->f_back;
    // 将 retval 返回
    return _Py_CheckFunctionResult(NULL, retval, "PyEval_EvalFrameEx");
}
```

[3] 处是为了 [never retain a generator's caller's exception state on the generator after a yield/return
](https://github.com/python/cpython/commit/ac91341333d27bf39dd8b8c1c3164b5bdc19f03b)

参考 Python 自己的测试代码

```Python
def test_generator_doesnt_retain_old_exc(self):
    def g():
        self.assertIsInstance(sys.exc_info()[1], RuntimeError)
        yield
        self.assertEqual(sys.exc_info(), (None, None, None))
    it = g()
    try:
        raise RuntimeError
    except RuntimeError:
        next(it)
    self.assertRaises(StopIteration, next, it)
```

`fast_yield` 中的代码貌似和 gen 的 suspend/resume 没有什么关系。其实奥秘是在 `_PyEval_EvalFrameDefault` 函数开始时根据 `PyFrameObject` 的 `f_lasti` 进行了跳转

```
PyObject *
_PyEval_EvalFrameDefault(PyFrameObject *f, int throwflag)
{
    const _Py_CODEUNIT *next_instr;
    const _Py_CODEUNIT *first_instr;

    first_instr = (_Py_CODEUNIT *) PyBytes_AS_STRING(co->co_code);
    next_instr = first_instr;
    // 这里根据 PyFrameObject 的 f->f_lasti 修改了 next_instr
    if (f->f_lasti >= 0) {
        next_instr += f->f_lasti / sizeof(_Py_CODEUNIT) + 1;
    }

    for (;;) {
    // ...
    fast_next_opcode:
        f->f_lasti = INSTR_OFFSET();
        // 省略 tracing 部分
        /* Extract opcode and argument */
        NEXTOPARG();
        switch (opcode) {
            // ...
        }
    }
}
```

`NEXTOPARG` 宏用于获取下一条指定的字节码

```
#define INSTR_OFFSET() (sizeof(_Py_CODEUNIT) * (int)(next_instr - first_instr))
#define NEXTOPARG()  do { \
        _Py_CODEUNIT word = *next_instr; \
        opcode = _Py_OPCODE(word); \
        oparg = _Py_OPARG(word); \
        next_instr++; \
} while (0)
```

本来想试试在代码中直接通过 `sys._getframe(0)` 然后修改 `f_lasti` 来实现个 `goto` 的，但是 `f_lasti` 是个 read only 的属性_(:з」∠)_

### Summary

1) 每个 Generator 对象都携带了自己的 frame
2) `send`/`next` 都是取 Generator 对象的 frame，然后调用 `PyEval_EvalFrameEx` 执行 Generator 中的代码
3) frame 中的 `f_lasti` 记录了当前的执行位置
4) 当 Generator exhausted 时，frame 才会被释放

    