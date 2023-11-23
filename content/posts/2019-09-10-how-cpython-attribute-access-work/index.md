
+++
title = "CPython 源码阅读 - 属性访问"
summary = ''
description = ""
categories = []
tags = []
date = 2019-09-10T14:32:45+08:00
draft = false
+++

*本文所使用代码为 CPython 3.7.4，commit sha 为 `e09359112e250268eca209355abeb17abf822486`*

本文探讨 Python 中当使用 `obj.attr` 的语法访问对象属性的时候会发生什么，是对本人 16 年所写的 [Python 中的 attribute 和 property](https://blog.dreamfever.me/2016/10/20/python-zhong-de-attribute-property/) 的修正与补充

我们先从 bytecode 的角度来分析，如果对 bytecode 不熟悉可以参考 [文档](https://docs.python.org/3/library/dis.html#python-bytecode-instructions) 或者 [关于 Python bytecode 的一些废话](https://blog.dreamfever.me/2017/05/28/guan-yu-python-bytecode-de-yi-xie-fei-hua/)

```
➜ python -m dis t.py
  1           0 LOAD_NAME                0 (obj)
              2 LOAD_ATTR                1 (attr)
              4 POP_TOP
              6 LOAD_CONST               0 (None)
              8 RETURN_VALUE
➜ cat t.py   
obj.attr
```

当我们得到 bytecode 的时候便可以在 Python/ceval.c 中寻找指令的具体实现

```
// Python/ceval.c#L2570-2579
TARGET(LOAD_ATTR) {
    PyObject *name = GETITEM(names, oparg);
    PyObject *owner = TOP();
    PyObject *res = PyObject_GetAttr(owner, name);
    Py_DECREF(owner);
    SET_TOP(res);
    if (res == NULL)
        goto error;
    DISPATCH();
}
```

从栈顶取出 `obj` 然后调用 `PyObject_GetAttr`，获取 `name` 对应的对象然后压入栈顶

```
// Objects/object.c#L898-921
PyObject *
PyObject_GetAttr(PyObject *v, PyObject *name)
{
    PyTypeObject *tp = Py_TYPE(v);

    if (!PyUnicode_Check(name)) {
        PyErr_Format(PyExc_TypeError,
                     "attribute name must be string, not '%.200s'",
                     name->ob_type->tp_name);
        return NULL;
    }
    if (tp->tp_getattro != NULL)
        return (*tp->tp_getattro)(v, name);
    if (tp->tp_getattr != NULL) {
        const char *name_str = PyUnicode_AsUTF8(name);
        if (name_str == NULL)
            return NULL;
        return (*tp->tp_getattr)(v, (char *)name_str);
    }
    PyErr_Format(PyExc_AttributeError,
                 "'%.50s' object has no attribute '%U'",
                 tp->tp_name, name);
    return NULL;
}
```

可以看到如果存在 `tp_getattro` 那么使用它来查找 `name`；其次如果存在 `tp_getattr` 那么将我们的 `name` 从 Unicode 对象转换为 C 字符串，使用 `tp_getattr` 来查找。如果都没有那么直接 `AttributeError`。关于 `tp_getattro` 和 `tp_getattr`，我们可以从 [C-API 文档](https://docs.python.org/3/c-api/typeobj.html#c.PyTypeObject.tp_getattr) 中找到答案

`tp_getattr` 和 `tp_getattro` 应当指向行为相同的两个函数，不同之处在于 `tp_getattr` 的参数是 C 字符串，`tp_getattro` 接受 `PyUnicodeObject`。而且 `tp_getattr` 已经处于废弃状态

`tp_getattro` 在默认情况下是 `PyObject_GenericGetAttr`，当我们定义一个新的类的时候会经过 `tp_new` 函数(定义在 `Objects/typeobject.c`)。此时会检查我们是否在新的类中重写了 dunder method。对于 `__getattr__` 和 `__getattribute__`，如果我们重写了任何一个，`tp_getattro` 会指向 `slot_tp_getattr_hook`

我们先来看一下 `PyObject_GenericGetAttr` 的行为

```
// Objects/object.c#L1303-1307
PyObject *
PyObject_GenericGetAttr(PyObject *obj, PyObject *name)
{
    return _PyObject_GenericGetAttrWithDict(obj, name, NULL, 0);
}

// Objects/object.c#L1198-1301
PyObject *
_PyObject_GenericGetAttrWithDict(PyObject *obj, PyObject *name,
                                 PyObject *dict, int suppress)
{
    PyTypeObject *tp = Py_TYPE(obj);
    PyObject *descr = NULL;
    PyObject *res = NULL;
    descrgetfunc f;
    Py_ssize_t dictoffset;
    PyObject **dictptr;

    if (!PyUnicode_Check(name)){
        PyErr_Format(PyExc_TypeError,
                     "attribute name must be string, not '%.200s'",
                     name->ob_type->tp_name);
        return NULL;
    }
    Py_INCREF(name);

    if (tp->tp_dict == NULL) {
        if (PyType_Ready(tp) < 0)
            goto done;
    }

    // 沿 MRO 寻找对应名称的属性
    descr = _PyType_Lookup(tp, name);

    f = NULL;
    if (descr != NULL) {
        Py_INCREF(descr);
        f = descr->ob_type->tp_descr_get;
        // 判断是否同时有 __get__ 方法和 __set__ 方法
        // 即 data descriptor
        if (f != NULL && PyDescr_IsData(descr)) {
            res = f(descr, obj, (PyObject *)obj->ob_type);
            // 返回 NULL 是 Python 发生异常的表示
            // 根据调用 suppress 为 0，所以这里不会清除异常信息
            if (res == NULL && suppress &&
                    PyErr_ExceptionMatches(PyExc_AttributeError)) {
                PyErr_Clear();
            }
            goto done;
        }
    }

    // 1) 我们无法在 MRO 链中找到对应的名称
    // 2) 或者之前找到的 descr 不是一个 data descriptor
    // 有经验的 Python 程序员一定会知道
    // Funtion 和 Method 都是具有 __get__ 方法的
    if (dict == NULL) {
        // 这段这么复杂的代码其实是在计算偏移然后找到对象的 __dict__
        /* Inline _PyObject_GetDictPtr */
        dictoffset = tp->tp_dictoffset;
        if (dictoffset != 0) {
            if (dictoffset < 0) {
                Py_ssize_t tsize;
                size_t size;

                tsize = ((PyVarObject *)obj)->ob_size;
                if (tsize < 0)
                    tsize = -tsize;
                size = _PyObject_VAR_SIZE(tp, tsize);
                assert(size <= PY_SSIZE_T_MAX);

                dictoffset += (Py_ssize_t)size;
                assert(dictoffset > 0);
                assert(dictoffset % SIZEOF_VOID_P == 0);
            }
            dictptr = (PyObject **) ((char *)obj + dictoffset);
            dict = *dictptr;
        }
    }
    if (dict != NULL) {
        // 如果对象有 __dict__，比如它没有定义 __slots__
        // 那么尝试从它的 __dict__ 中寻找
        Py_INCREF(dict);
        res = PyDict_GetItem(dict, name);
        if (res != NULL) {
            Py_INCREF(res);
            Py_DECREF(dict);
            goto done;
        }
        Py_DECREF(dict);
    }

    // 我们能够从 MRO 中找到，且其具有 __get__ 而不具有 __set__
    if (f != NULL) {
        res = f(descr, obj, (PyObject *)Py_TYPE(obj));
        if (res == NULL && suppress &&
                PyErr_ExceptionMatches(PyExc_AttributeError)) {
            PyErr_Clear();
        }
        goto done;
    }

    // 它就是一个普通的对象
    if (descr != NULL) {
        res = descr;
        descr = NULL;
        goto done;
    }

    if (!suppress) {
        PyErr_Format(PyExc_AttributeError,
                     "'%.50s' object has no attribute '%U'",
                     tp->tp_name, name);
    }
  done:
    Py_XDECREF(descr);
    Py_DECREF(name);
    return res;
}

```

这段代码的圈复杂度有点高，我下面来用人话翻译一下

1) 遍历 MRO 中的类型对象，依次查找其 `__dict__` 中是否有 `name`。如果有且其同时定义了 `__get__` 和 `__set__`，那么调用 `__get__` 然后将结果返回
2) 如果对象的 `__dict__` 中有 `name` 那么返回，即使我们在遍历 MRO 期间找到了 `name`
3) 如果在遍历 MRO 期间找到了 `name`，且对象的 `__dict__` 中不存在相应的 `name`。那么如果其具有 `__get__`，则调用后返回其结果，否则直接返回

更进一步的总结就是

- data descriptor 优先于对象的 `__dict__` 中的同名属性
- 对象的 `__dict__` 中属性优先于 non data descriptor

所以 `obj.attr` 的查找顺序是, `obj.__dict__['attr']` , 然后 `type(obj).__dict__['attr']` , 然后找 `type(obj)` 的父类的 `__dict__['attr']`，然后父类的父类...这是不严谨的说法

另外我在上面的表述中有意不使用"实例"，这是因为实际上 `obj.attr` 中的 `obj` 不一定是实例，它也有可能是类。所以这里统一使用对象进行表述。侧面也说明了即使 `obj` 是一个类，它的查找机制也是类似的

蛤，你可能会问难道我每次访问一次属性都会遍历 MRO？不，实际上它也是有缓存的，具体可以参考 `_PyType_Lookup` 的实现


其次让我们来看一下 `slot_tp_getattr_hook` 的实现

```
static PyObject *
slot_tp_getattr_hook(PyObject *self, PyObject *name)
{
    PyTypeObject *tp = Py_TYPE(self);
    PyObject *getattr, *getattribute, *res;
    _Py_IDENTIFIER(__getattr__);

    /* speed hack: we could use lookup_maybe, but that would resolve the
       method fully for each attribute lookup for classes with
       __getattr__, even when the attribute is present. So we use
       _PyType_Lookup and create the method only when needed, with
       call_attribute. */
    getattr = _PyType_LookupId(tp, &PyId___getattr__);
    if (getattr == NULL) {
        /* No __getattr__ hook: use a simpler dispatcher */
        // 之前提到过如果重写了 __getattr__ 或者 __getattribute__ 之一
        // tp_getattro 才会指向 slot_tp_getattr_hook
        // 现在 __getattr__ 没有被定义，那么一定是重写了 __getattribute__
        // 所以更改指针的指向，指向一个直接调用 __getattribute__ 的函数
        tp->tp_getattro = slot_tp_getattro;
        return slot_tp_getattro(self, name);
    }
    Py_INCREF(getattr);
    /* speed hack: we could use lookup_maybe, but that would resolve the
       method fully for each attribute lookup for classes with
       __getattr__, even when self has the default __getattribute__
       method. So we use _PyType_Lookup and create the method only when
       needed, with call_attribute. */
    getattribute = _PyType_LookupId(tp, &PyId___getattribute__);
    // 在这里我们可能同时重写了 ___getattr__ 和 __getattribute__
    // 也可能仅重写了 __getattr__
    if (getattribute == NULL ||
        (Py_TYPE(getattribute) == &PyWrapperDescr_Type &&
         ((PyWrapperDescrObject *)getattribute)->d_wrapped ==
         (void *)PyObject_GenericGetAttr))
         // 如果我们没有重写 __getattribute__
        res = PyObject_GenericGetAttr(self, name);
    else {
        // 如果我们重写了 __getattribute__
        Py_INCREF(getattribute);
        res = call_attribute(self, getattribute, name);
        Py_DECREF(getattribute);
    }
    // 到这里，我们一定会调用 __getattribute__，不管这是默认的还是自己重写过的
    // 当且仅当我们无法找到 attr 的时候，我们会去调用 __getattr__
    if (res == NULL && PyErr_ExceptionMatches(PyExc_AttributeError)) {
        PyErr_Clear();
        res = call_attribute(self, getattr, name);
    }
    Py_DECREF(getattr);
    return res;
}
```

用人话表述就是:

`__getattribute__` 优先级高于 `__getattr__`，且只有我们无法通过 `__getattribute__` 找到的时候才会调用 `__getattr__`

### Reference

[Python 源码剖析](https://read.douban.com/ebook/1499455/)

    