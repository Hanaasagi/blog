
+++
title = "Python 闭包杂谈"
summary = ''
description = ""
categories = []
tags = []
date = 2017-05-29T05:53:00+08:00
draft = false
+++

### 引例
学 Python 的人，或许都会遇到这么一个经典的问题。此问题可通过 `nonlocal` 解决

```Python
def outer():
    n = 0
    def inner():
        n = n + 1
        return n
    return inner

outer()()
```

运行后便会发生如下的异常

```
Traceback (most recent call last):
  File "test.py", line 8, in <module>
    outer()()
  File "test.py", line 4, in inner
    n = n + 1
UnboundLocalError: local variable 'n' referenced before assignment
```

这个问题和下面这个是类似的

```Python
n = 1

def func():
    if n == 0:
        n = 2

func()
```

`n = 2` 是永远不会执行的。但还是会出现相同的异常

或者这个问题

```Python
n = 1

def func():
    print(n)
    n = 2

func()
```

等等……

JavaScript 也有这类问题，称为 Hoisting 参考自 [JavaScript 秘密花园](https://bonsaiden.github.io/JavaScript-Garden/zh/#function.scopes)

```javascript
var myvar = 'my value';  

(function() {  
    alert(myvar);  // undefined  
    var myvar = 'local value';  
})();
```

要理解这个问题还是需要一些 PVM 和 字节码的知识的。Python 首先会对源代码进行编译，形成字节码，然后将字节码载入运行。所有的分支都会参与编译的。编译时会生成符号表。详见我的上篇文章[]()  

那么，让我们从字节码的角度来看看

```
In [39]: def outer():
    ...:     m = 0
    ...:     n = 0
    ...:     def inner():
    ...:         n = n + m
    ...:     return inner
    ...:
    ...:

In [40]: dis(outer())
  5           0 LOAD_FAST                0 (n)
              3 LOAD_DEREF               0 (m)
              6 BINARY_ADD
              7 STORE_FAST               0 (n)
             10 LOAD_CONST               0 (None)
             13 RETURN_VALUE

```

对于 `m` 和 `n`， Python 使用了不同的 LOAD 操作  

- `LOAD_FAST` 从局部变量表 `co_varnames` 载入
- `LOAD_DEREF` 从 cell 变量和自由变量的存储区载入。cell 变量指被内部作用域所引用的变量，自由变量是引用的外层作用域中的变量。比如定义在外层函数中的 `m`，对于内层函数说它是自由变量，对于外层函数它是 cell 变量

再来看看符号表

```Python
In [41]: c = outer()

In [42]: c.__code__.co_cellvars
Out[42]: ()

In [43]: c.__code__.co_varnames
Out[43]: ('n',)

In [44]: c.__code__.co_freevars
Out[44]: ('m',)

In [45]: outer.__code__.co_cellvars
Out[45]: ('m',)

In [46]: outer.__code__.co_varnames
Out[46]: ('n', 'inner')

In [47]: outer.__code__.co_freevars
Out[47]: ()
```

对比 `n` 和 `m`。体会一下，我们可以发现如果在函数中出现赋值语句，则编译器会认为其是函数的一个局部变量，不会认为其是一个自由变量， `n` 被放到了局部变量表 `co_varnames` 中。而由于 Python 是先执行的 `n + 1`，先去 LOAD `n`。但 `n` 并没有绑定任何对象，所以会抛出 `UnboundLocalError`

所以在出现 `nonlocal` 关键字之前， Python 的闭包作用域可以认为是只读的，当然你也可以用可变类型 trick 掉这个机制。[PEP 227 -- Statically Nested Scopes](https://www.python.org/dev/peps/pep-0227/) 是这样描述这个现象的

>If a name is bound anywhere within a code block, all uses of the name within the block are treated as references to the current block.

### 闭包为何能引用自由变量

```
In [3]: def outer():
   ...:     n = 0
   ...:     def inner():
   ...:         m = n + 1
   ...:     return inner
   ...:

In [4]: dis(outer)
  2           0 LOAD_CONST               1 (0)
              3 STORE_DEREF              0 (n)

  3           6 LOAD_CLOSURE             0 (n)
              9 BUILD_TUPLE              1
             12 LOAD_CONST               2 (<code object inner at 0x7fb71e64ba50, file "<ipython-input-3-d1d0f7f91b20>", line 3>)
             15 LOAD_CONST               3 ('outer.<locals>.inner')
             18 MAKE_CLOSURE             0
             21 STORE_FAST               0 (inner)

  5          24 LOAD_FAST                0 (inner)
             27 RETURN_VALUE
```

- `STORE_DEREF(i)`  
  Stores TOS into the cell contained in slot `i` of the cell and free variable storage.
- `LOAD_CLOSURE(i)`  
  Pushes a reference to the cell contained in slot `i` of the cell and free variable storage. The name of the variable is `co_cellvars[i]` if `i` is less than the length of `co_cellvars`. Otherwise it is `co_freevars[i - len(co_cellvars)]`.
- `MAKE_CLOSURE(argc)`  
  Creates a new function object, sets its `__closure__` slot, and pushes it on the stack. TOS is the qualified name of the function, TOS1 is the code associated with the function, and TOS2 is the tuple containing cells for the closure’s free variables. argc is interpreted as in `MAKE_FUNCTION`; the annotations and defaults are also in the same order below TOS2.

上述字节码做的就是将闭包引用到的变量组成元组然后赋给闭包的 `__closure__`

```Python
In [5]: outer().__closure__
Out[5]: (<cell at 0x7fb71e6598b8: int object at 0x8c1620>,)

In [6]: outer().__closure__[0].cell_contents
Out[6]: 0
```

`__closure__` 对应着 Python 查找变量的顺序 `local -> enclosing -> global -> builtin` 中的 enclosing，这便是能够引用到自由变量的理由

### Update 2017/06/02
突然又想起一个闭包相关的例子

```Python
In [32]: def outer():
    ...:     funcs = []
    ...:     for i in range(3):
    ...:         def inner():
    ...:             return i
    ...:         funcs.append(inner)
    ...:     return funcs
    ...:

In [33]: for f in outer():
    ...:     print(f())
    ...:     
2
2
2
```

`outer()` 中通过循环定义了三个相同的函数 `inner`，然后组成列表返回。`inner` 中持有对自由变量 `i` 的引用。当调用这三个函数时，会去 `LOAD_DEREF i`，此时的 `i` 因为循环已经执行完成所以为 2

一个解决方法是为参数添加默认值

```Python
In [45]: def outer():
    ...:     funcs = []
    ...:     for i in range(3):
    ...:         def inner(i=i):
    ...:             return i
    ...:         funcs.append(inner)
    ...:     return funcs
    ...:

In [46]: for f in outer():
    ...:     print(f())
    ...:     
0
1
2
```

函数创建时如果发现参数有默认值，则会对右值(如`x=2^10`)进行求值(1024)后存储在 `__defaults__` 中。上例中的 `i` 作为形参出现在函数的定义中，所以不再是自由变量，而是局部变量，相应的也使用 `LOAD_FAST` 指令

```Python
In [51]: outer()[0].__code__.co_varnames
Out[51]: ('i',)

In [52]: dis(outer()[0])
  5           0 LOAD_FAST                0 (i)
              3 RETURN_VALUE

In [53]: outer()[0].__defaults__
Out[53]: (0,)
```

~~(这篇文章真的很杂 (ﾉ∀`*))~~

    