
+++
title = "关于 Python bytecode 的一些废话"
summary = ''
description = ""
categories = []
tags = []
date = 2017-05-28T11:42:00+08:00
draft = false
+++

*本文基于 Python 3.5*  
最近稍稍看了 Python bytecode 方面的资料

### 预备知识
**1) bytecode(字节码)**  
[Python Glossary](https://docs.python.org/3/glossary.html) 中是这样解释 bytecode 的

>Python source code is compiled into bytecode, the internal representation of a Python program in the CPython interpreter. The bytecode is also cached in .pyc files so that executing the same file is faster the second time (recompilation from source to bytecode can be avoided). This “intermediate language” is said to run on a virtual machine that executes the machine code corresponding to each bytecode. Do note that bytecodes are not expected to work between different Python virtual machines, nor to be stable between Python releases.

**2) Python 执行过程**  
平常所说的 Python 是解释型语言，是对 Python 的主流实现的一种描述。语言是抽象的语法集，并不强制规定其实现方式。C 语言也有着自己的解释器，比如这个[picoc](https://github.com/zsaleeba/picoc)。解释型语言依赖解释器，解释器通常以"编译器+虚拟机(VM)"的方式来实现的，先通过编译器将源代码转换成字节码，然后交由虚拟机去执行。所以解释型语言也需要编译。VM 也分解释型和编译型。如果采用编译方式，VM 会把指令先转换为 native code，然后再执行；而解释方式，则是将指令逐条直接执行。对于 CPython 来说，采用的是解释型

**3) Python dis 模块**  
`dis` 模块的 Docstring 是这样描述的：Disassembler of Python byte code into mnemonics. ~~这里用了反汇编 disassembler 而不是反编译 decompile~~。`dis()` 函数能将 `class`, `method`, `function`, `generator`, `code object` 转换成可读性更高的助记符

其输出格式如下

```
源代码中的行号  字节码偏移量       字节码指令       指令参数         对于参数的相关说明
  4             3            LOAD_CONST        1               ('add')
```

### 正文
先从一个简单的例子开始

```Python
# test.py
def add(a, b):
    return a + b

if __name__ == '__main__':
    add(2, 3)
```

执行 `python -m py_compile test.py` 会生成 `__pycache__/test.cpython-35.pyc` 这个文件，当然如果你是用的 Python 2.x，则会在当前目录生成 `test.pyc`。或者直接 `import` 也行。一个 pyc 文件包含了四部分信息：magic number(编译器特征码)，文件的创建时间， 大小，以及序列化后的 code 对象(Python 2.x 只有三部分，没有 大小)。这个 `code` 对象是源代码经过编译后生成的，包含了原始字节码和变量表等信息，后面会对它进行分析。pyc 文件的作用是和源文件的创建时间进行对比，来决定是直接加载 pyc 还是重新编译

```Python
In [59]: with open('./test.cpython-35.pyc', 'rb') as f:
    ...:     f.seek(12)
    ...:     code = marshal.load(f)      # 反序列化
    ...:     dis.dis(code)               # 反编译
    ...:     
  4           0 LOAD_CONST               0 (<code object add at 0x7fc6cbc5d270, file "test.py", line 4>)
              3 LOAD_CONST               1 ('add')
              6 MAKE_FUNCTION            0
              9 STORE_NAME               0 (add)

  7          12 LOAD_NAME                1 (__name__)
             15 LOAD_CONST               2 ('__main__')
             18 COMPARE_OP               2 (==)
             21 POP_JUMP_IF_FALSE       37

  8          24 LOAD_NAME                0 (add)
             27 LOAD_CONST               3 (2)
             30 LOAD_CONST               4 (3)
             33 CALL_FUNCTION            2 (2 positional, 0 keyword pair)
             36 POP_TOP
        >>   37 LOAD_CONST               5 (None)
             40 RETURN_VALUE
```

在分析 bytecode 之前，先来看看之前提到的 `code` 对象。它有以下属性，可以在 [inspect](https://docs.python.org/3/library/inspect.html?highlight=co_argcount#types-and-members) 模块看到

```Python
In [67]: list((attr for attr in dir(code) if not attr.startswith('__' )))
Out[67]:
['co_argcount',       # 参数的数量，不包含强制关键字参数(keyword only arguments),和 * / ** 参数
 'co_cellvars',       # cell 变量表，cell 变量是被内部作用域所引用的变量
 'co_code',           # 编译后的原始字节码
 'co_consts',         # 常量表
 'co_filename',       # 源代码文件名
 'co_firstlineno',    # 源代码的首行号
 'co_flags',          # CO_* flags 的位图 比如判断一个函数是否为生成器：co_flags & 0x100000==0
 'co_freevars',       # 自由变量表(被函数闭包所引用)
 'co_kwonlyargcount', # 强制关键字参数的数量(不包含 ** 参数)
 'co_lnotab',         # encoded mapping of line numbers to bytecode indices
 'co_name',           # 定义此代码对象的名称
 'co_names',          # 全局变量表
 'co_nlocals',        # 局部变量数量
 'co_stacksize',      # VM 所需要的栈空间，这个是通过静态分析得出来的
 'co_varnames']       # 参数和局部变量的变量表
```

字节码经常会对 `co_varnames`， `co_names`， `co_consts` 进行操作。以 `co_consts` 为例

```Python
In [82]: code.co_consts
Out[82]:
(<code object add at 0x7fc6cbc5d270, file "test.py", line 4>,
 'add',
 '__main__',
 2,
 3,
 None)
```

所有的 `LOAD_CONST` 操作后面跟的参数，都是这个 `co_consts` 常量表的索引。

以下均指 CPython，其他的没研究过，不敢妄言  
执行这些 bytecode 的是 Python Virtual Machine(PVM)。它是一个基于栈的虚拟机，关于栈式虚拟器和寄存器虚拟机方面的资料可以查看这篇文章 [虚拟机随谈（一）：解释器，树遍历解释器，基于栈与基于寄存器，大杂烩](http://rednaxelafx.iteye.com/blog/492667)。
bytecode 的大部分操作都是和这个栈(evaluation stack)相关的，比如 `LOAD_NAME` 就是将变量入栈， `STORE_NAME` 就是将栈顶变量存入 `co_names` 中。除了这个常用的栈外还有一个用来存储 block 的栈(block stack)，它用来存储 loops, try 和 with 块。另外，我们可以发现第一条字节码 `LOAD_CONST` 载入了一个 `code` 对象，这个 `code` 对象便是 `add` 函数。源代码中拥有独立作用域的都是一个 `code` 对象，比如 `method`，`class`，`function`。 每个 `code` 对象有着自己独立的栈。

Example

```Python
In [94]: dis(compile('d = (a + b) * c', '<string>', 'exec'))
  1           0 LOAD_NAME                0 (a)
              3 LOAD_NAME                1 (b)
              6 BINARY_ADD
              7 LOAD_NAME                2 (c)
             10 BINARY_MULTIPLY
             11 STORE_NAME               3 (d)
             14 LOAD_CONST               0 (None)
             17 RETURN_VALUE
```

Stack 变化如下(`co_stacksize` 为 2)

```
  LOAD_NAME 0(a)  LOAD_NAME a(b)   BINARY_ADD     LOAD_NAME 2(c)
  -------------   -------------   -------------   -------------
1 |           |   |     b     |   |           |   |     c     |
  |-----------|   |-----------|   |-----------|   |-----------|
0 |     a     |   |     a     |   |   a + b   |   |   a + b   |
  -------------   -------------   -------------   -------------

 BINARY_MULTIPLY  STORE_NAME 3(d) LOAD_CONST 0(None) RETURN_VALUE
  -------------   -------------   -------------   -------------
1 |           |   |           |   |           |   |           |
  |-----------|   |-----------|   |-----------|   |-----------|
0 |  (a+b)*c  |   |           |   |    None   |   |           |
  -------------   -------------   -------------   -------------
```

每一条指令可能为一字节或者三字节，比如 `LOAD_NAME` 占用三字节，而 `BINARY_ADD` 占用一个。第一个字节均为指令，而第二个第三个字节为指令的参数。由于参数只有两个字节，也就是说最多只能表示 0~65535，那么是否意味着 Python 只能定义 65536 个变量呢？不是这样的，蠢作者试着定义了 65537 个变量

```Python
a0 = 2
a1 = 2
# ...
a65536 = 2
```

运行并没有出现错误。不过观察字节码发现

```Python
65536        393210 LOAD_CONST               0 (2)
             393213 STORE_NAME           65535 (a65535)

65537        393216 LOAD_CONST               0 (2)
             393219 EXTENDED_ARG             1
             393222 STORE_NAME           65536 (a65536)
```

在 `a65535 = 2` 和 `a65536 = 2` 相应的 bytecode 之间，有一个 `EXTENDED_ARG`。它提供了两个额外的字节，与原来的两个字节相组合，扩充了表示范围。如果超过了 2 ** 32，则会再次 `EXTENDED_ARG` 扩充两个字节。当然我想，如果你的程序有 65536 个不同的变量，我想你应该被拖出去……

后半部分是冗长的 bytecode 指令集

### bytecode instructions
bytecode 指令集可以从这里找到 [Python Bytecode Instructions](https://docs.python.org/3/library/dis.html#python-bytecode-instructions)

- `NOP`  
  什么也不做，用作字节码优化器的占位符
- `POP_TOP`  
  移除栈顶元素(`TOS`)
- `POP_BLOCK`  
  从 block stack 移除一个 block
- `ROT_TWO`  
  交换栈顶的两个元素，常用在 Python 交换两个变量时
```
In [28]: def func(a, b):
    ...:     a, b = b, a
    ...:     

In [29]: dis(func)
  2           0 LOAD_FAST                1 (b)
              3 LOAD_FAST                0 (a)
              6 ROT_TWO
              7 STORE_FAST               0 (a)
             10 STORE_FAST               1 (b)
             13 LOAD_CONST               0 (None)
             16 RETURN_VALUE
```

- `ROT_THREE`  
  `TOS` 下移两个位置 `[A, B, C] => [B, C, A]`
- `DUP_TOP`  
  复制 `TOS` `[A, B] => [A, A, B]`
- `DUP_TOP_TWO`  
  复制顶部两个元素 `[A, B, C]` => `[A, B, A, B, C]`
- `RETURN_VALUE`  
  `return`，将 `TOS` 返回给调用者
- `YIELD_VALUE`  
  `yield`，`TOS` 出栈，并将它从生成器返回
- `YIELD_FROM`  
  `yield from`，`TOS` 出栈，并作为子生成器进行委派


#### LOAD/STORE 指令
- `LOAD_CONST(consti)`  
  将 `co_consts[consti]` 入栈
- `LOAD_NAME(namei)`  
  将 `co_names[namei]` 入栈
- `LOAD_FAST(var_num)`  
  将 `co_varnames[var_num]` 入栈
- `LOAD_GLOBAL(namei)`  
  将 `co_names[namei]` 入栈
- `STORE_NAME(namei)`  
  `name = TOS`，编译器会尽量使用 `STORE_FAST` 或 `STORE_GLOBAL`
- `STORE_FAST(var_num)`  
  将 `TOS` 存储到局部变量表 `co_varnames[var_num]`
- `STORE_GLOBAL(namei)`  
  类似 `STORE_NAME`，但存储为全局变量
- `DELETE_NAME(namei)`  
  `del name`，`namei` 是 `name` 在 code 对象的 `co_names` 上的索引
- `DELETE_FAST(var_num)`  
  删除局部变量 `co_varnames[var_num]`.
- `DELETE_GLOBAL(namei)`  
  类似 `DELETE_NAME`，但删除全局变量

```Python
In [121]: def func():
     ...:     a = 1
     ...:     global b
     ...:     b = 2
     ...:     c = 3
     ...:     del c
     ...:     

In [122]: dis(func)
  2           0 LOAD_CONST               1 (1)
              3 STORE_FAST               0 (a)

  4           6 LOAD_CONST               2 (2)
              9 STORE_GLOBAL             0 (b)

  5          12 LOAD_CONST               3 (3)
             15 STORE_FAST               1 (c)

  6          18 DELETE_FAST              1 (c)
             21 LOAD_CONST               0 (None)
             24 RETURN_VALUE
```

与上面相似，存在对属性的操作

- `LOAD_ATTR(namei)`  
  使用 `getattr(TOS, co_names[namei])` 替换 `TOS` (文档上用了 replace，而不是 push)
- `STORE_ATTR(namei)`  
  `TOS.name = TOS1`
- `DELETE_ATTR(namei)`  
  `del TOS.name`

####  一元操作
一元操作取栈顶元素，执行操作后，将结果入栈

- `UNARY_POSITIVE`  
  `TOS = +TOS`
- `UNARY_NEGATIVE`  
  `TOS = -TOS`
- `UNARY_NOT`  
  `TOS = not TOS`
- `UNARY_INVERT`  
  `TOS = ~TOS`

#### 二元操作
二元操作移除栈顶元素(`TOS`)和第二个元素(`TOS1`)，然后执行操作，将结果入栈

- `BINARY_POWER`  
  `TOS = TOS1 ** TOS`
- `BINARY_MULTIPLY`  
  `TOS = TOS1 * TOS`
- `BINARY_MATRIX_MULTIPLY`  
  `TOS = TOS1 @ TOS`
- `BINARY_FLOOR_DIVIDE`  
  `TOS = TOS1 // TOS`
- `BINARY_TRUE_DIVIDE`  
  `TOS = TOS1 / TOS`
- `BINARY_MODULO`  
  `TOS = TOS1 % TOS`
- `BINARY_ADD`  
  `TOS = TOS1 + TOS`
- `BINARY_SUBTRACT`  
  `TOS = TOS1 - TOS`
- `BINARY_SUBSCR`  
  `TOS = TOS1[TOS]`
- `BINARY_LSHIFT`  
  `TOS = TOS1 << TOS`
- `BINARY_RSHIFT`  
  `TOS = TOS1 >> TOS`
- `BINARY_AND`  
  `TOS = TOS1 & TOS`
- `BINARY_XOR`  
  `TOS = TOS1 ^ TOS`
- `BINARY_OR`  
  `TOS = TOS1 | TOS`

#### In-place 操作
In-place 操作与二元操作类似，只不过需要 `TOS1` 支持，入栈的结果和 `TOS1` 应当为同一个对象(不是必须的)。

注意是 `TOS1`，自己体会一下

```Python
In [42]: def func(a, b):
    ...:     a += b
    ...:     

In [43]: dis(func)
  2           0 LOAD_FAST                0 (a)
              3 LOAD_FAST                1 (b)
              6 INPLACE_ADD
              7 STORE_FAST               0 (a)
             10 LOAD_CONST               0 (None)
             13 RETURN_VALUE
```

为什么入栈结果和 `TOS1` 不一定为同一个对象呢？因为 Python 是一门动态语言，虽然通过 `+=` 能推导出应当使用 `INPLACE_ADD` 字节码，但是编译时并不知道 `a` 的类型是否存在 `__iadd__()` 方法。如果不存在此方法，则会去调用对象的 `__add__()` 方法，这时便不是同一个对象了。所以 VM 并不是在傻瓜式地执行指令


大部分指令都是从二元操作照搬过来的，所以这里仅列几个不同的

- `STORE_SUBSCR`  
  `TOS1[TOS] = TOS2` 这个涉及到了栈顶的三个元素 `[TOS, TOS1, TOS2]`

```Python
In [48]: def func(a, b):
    ...:     a[0] = b
    ...:     
    ...:     

In [49]: dis(func)
  2           0 LOAD_FAST                1 (b)
              3 LOAD_FAST                0 (a)
              6 LOAD_CONST               1 (0)
              9 STORE_SUBSCR
             10 LOAD_CONST               0 (None)
             13 RETURN_VALUE

```

- `DELETE_SUBSCR`  
  `del TOS1[TOS]`

#### 类型创建指令
- `BUILD_TUPLE(count)`  
  从栈中选取指定数量的元素然后构建元组，并将结果入栈。
- `BUILD_LIST(count)`  
- `BUILD_SET(count)`  
- `BUILD_MAP(count)`  
- `BUILD_SLICE(argc)`  
  将 `slice` 对象入栈。 `argc` 必须为 2 或 3。如果为 2 则入栈 `slice(TOS1, TOS)`；如果为 3 则入栈 `slice(TOS2, TOS1, TOS)`

```Python
In [146]: dis(compile('[1, 2]', '<string>', 'exec'))
  1           0 LOAD_CONST               0 (1)
              3 LOAD_CONST               1 (2)
              6 BUILD_LIST               2
              9 POP_TOP
             10 LOAD_CONST               2 (None)
             13 RETURN_VALUE
```

- `UNPACK_SEQUENCE(count)`
  对 `TOS` 解包至 count 个变量上，这些变量从左至右入栈。对于含有 `*` 的解包 `a, *b = arg` 则使用 `UNPACK_EX(counts)`


#### 迭代指令
- `GET_ITER`  
  `TOS = iter(TOS)`
- `FOR_ITER(delta)`  
  `TOS` 是一个迭代器。调用其 `__next__()` 方法。如果它 `yield` 一个新的值，则将其入栈。如果迭代器已经耗尽(`StopIteration`)，`TOS` 出栈，并且 bytecode coutner 增加 delta。

```Python
In [153]: def func(l):
     ...:     sum = 0
     ...:     for i in l:
     ...:         sum += i
     ...:          

In [154]: dis(func)
  2           0 LOAD_CONST               1 (0)
              3 STORE_FAST               1 (sum)

  3           6 SETUP_LOOP              24 (to 33)
              9 LOAD_FAST                0 (l)
             12 GET_ITER
        >>   13 FOR_ITER                16 (to 32)
             16 STORE_FAST               2 (i)

  4          19 LOAD_FAST                1 (sum)
             22 LOAD_FAST                2 (i)
             25 INPLACE_ADD
             26 STORE_FAST               1 (sum)
             29 JUMP_ABSOLUTE           13
        >>   32 POP_BLOCK
        >>   33 LOAD_CONST               0 (None)
             36 RETURN_VALUE
```

#### 跳转指令
- `COMPARE_OP(opname)`  
  进行布尔运算，包含这些 (`<`, `<=`, `==`, `!=`, `>`, `>=`, `in`, `not in`, `is`, `is not`, `exception match`, `BAD`)
- `POP_JUMP_IF_TRUE(target)`  
  `if not expression`
  如果 `TOS` 为 true，设置 byteorder counter 进行跳转。`TOS` 出栈
- `POP_JUMP_IF_FALSE(target)`  
  `if expression`
  如果 `TOS` 为 false，设置 byteorder counter 进行跳转。`TOS` 出栈
- `JUMP_IF_TRUE_OR_POP(target)`  
  如果 `TOS` 为 true，设置 bytecode counter 进行跳转(`TOS` 依旧留在栈中)，否则 `TOS` 出栈
- `JUMP_IF_FALSE_OR_POP(target)`  
  如果 `TOS` 为 false，设置 bytecode counter 进行跳转(`TOS` 依旧留在栈中)，否则 `TOS` 出栈
- `JUMP_FORWARD(delta)`  
  将 bytecode counter 增加 delta
- `JUMP_ABSOLUTE(target)`  
  设置 bytecode counter 进行跳转，类似汇编的 jmp 修改 IP 寄存器

```Python
In [80]: def func(a, b):
    ...:     if a > b:
    ...:         return a
    ...:     return b
    ...:

In [81]: dis(func)
  2           0 LOAD_FAST                0 (a)
              3 LOAD_FAST                1 (b)
              6 COMPARE_OP               4 (>)
              9 POP_JUMP_IF_FALSE       16

  3          12 LOAD_FAST                0 (a)
             15 RETURN_VALUE

  4     >>   16 LOAD_FAST                1 (b)
             19 RETURN_VALUE
```

#### 循环指令
- `SETUP_LOOP(delta)`  
  将用于循环的块入 block 栈，该块从当前的指令跨越 delta 字节。
- `BREAK_LOOP`  
  终止循环
- `CONTINUE_LOOP(target)`  
  Continues a loop due to a continue statement. target is the address to jump to (which should be a FOR_ITER instruction).

#### 模块相关
- `IMPORT_NAME(namei)`  
  导入 `co_names[namei]` 模块，`TOS` 和 `TOS1` 出栈，并作为 `__import__()` 的 `fromlist` 和 `level` 参数。模块对象入栈。当前的命名空间不受影响。for a proper import statement, a subsequent `STORE_FAST` instruction modifies the namespace.
- `IMPORT_FROM(namei)`  
  从 `TOS` 的模块载入 `co_names[namei]`，并将结果对象入栈，随后由 `STORE_FAST` 指令存储(蠢作者发现这个是视情况而定的)
- `IMPORT_STAR`  
  将模块 TOS 中所有不以 `_` 开始的符号载入到局部命名空间，然后将模块对象出栈

```Python
In [16]: dis(compile('import collections', '<string>', 'exec'))
  1           0 LOAD_CONST               0 (0)
              3 LOAD_CONST               1 (None)
              6 IMPORT_NAME              0 (collections)
              9 STORE_NAME               0 (collections)
             12 LOAD_CONST               1 (None)
             15 RETURN_VALUE

In [17]: dis(compile('from collections import deque', '<string>', 'exec'))
  1           0 LOAD_CONST               0 (0)
              3 LOAD_CONST               1 (('deque',))
              6 IMPORT_NAME              0 (collections)
              9 IMPORT_FROM              1 (deque)
             12 STORE_NAME               1 (deque)
             15 POP_TOP
             16 LOAD_CONST               2 (None)
             19 RETURN_VALUE

In [18]: dis(compile('from collections import *', '<string>', 'exec'))
  1           0 LOAD_CONST               0 (0)
              3 LOAD_CONST               1 (('*',))
              6 IMPORT_NAME              0 (collections)
              9 IMPORT_STAR
             10 LOAD_CONST               2 (None)
             13 RETURN_VALUE
```

另外还有一些 `try-except`， `with` 语句，`async/await`，`closure` 相关的字节码，这里就不列举了。突然发现已经写了 500 lines+，满篇的废话:(

### Reference
[探索 Python 代码对象](http://pycoders-weekly-chinese.readthedocs.io/en/latest/issue7/exploring-python-code-objects.html)  
[Python Virtual Machine](http://www.ics.uci.edu/~brgallar/week9_3.html)

    