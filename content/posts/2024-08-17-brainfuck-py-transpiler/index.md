+++
title = "实现 Brainfuck Transpiler"
summary = ""
description = ""
categories = [""]
tags = []
date = 2024-08-17T18:00:00+09:00
draft = false

+++

承接上文，这次来实现一个 transpiler，将 Brainfuck 转换成 Python 的 bytecode 然后运行在 Python VM 上。

本文代码基于 Python 3.12.4 编写，不同版本会略有差异。Python 字节码相关的文档可以参考文档 [Python Bytecode Instructions](https://docs.python.org/3/library/dis.html#python-bytecode-instructions)

首先来看一个简单示例，熟悉一下 Python 的字节码和 Code 对象中的内容

```
In [3]: import dis

In [4]: def f():
   ...:     a = [0] * 128
   ...:     print(a)
   ...:

In [5]: dis.dis(f)
  1           0 RESUME                   0

  2           2 LOAD_CONST               1 (0)
              4 BUILD_LIST               1
              6 LOAD_CONST               2 (128)
              8 BINARY_OP                5 (*)
             12 STORE_FAST               0 (a)

  3          14 LOAD_GLOBAL              1 (NULL + print)
             24 LOAD_FAST                0 (a)
             26 CALL                     1
             34 POP_TOP
             36 RETURN_CONST             0 (None)

In [6]: code_object = f.__code__

In [7]: for name in dir(code_object):
   ...:     if not name.startswith("co_"):
   ...:         continue
   ...:     print(name, getattr(code_object, name))
   ...:
co_argcount 0
co_cellvars ()
co_code b'\x97\x00d\x01g\x01d\x02z\x05\x00\x00}\x00t\x01\x00\x00\x00\x00\x00\x00\x00\x00|\x00\xab\x01\x00\x00\x00\x00\x00\x00\x01\x00y\x00'
co_consts (None, 0, 128)
co_exceptiontable b''
co_filename <ipython-input-4-1c2143026761>
co_firstlineno 1
co_flags 3
co_freevars ()
co_kwonlyargcount 0
co_lines <built-in method co_lines of code object at 0x757c241b93e0>
co_linetable b'\x80\x00\xd8\t\n\x88\x03\x88c\x89\t\x80A\xdc\x04\t\x88!\x85H'
<ipython-input-7-9bd8d5e7f107>:4: DeprecationWarning: co_lnotab is deprecated, use co_lines instead.
  print(name, getattr(code_object, name))
co_lnotab b'\x02\x01\x0c\x01'
co_name f
co_names ('print',)
co_nlocals 1
co_positions <built-in method co_positions of code object at 0x757c241b93e0>
co_posonlyargcount 0
co_qualname f
co_stacksize 3
co_varnames ('a',)

```

Python 的字节码是，字节码 + 一元操作数的格式。`RESUME` 不用管，3.11 加入的内部调试用的字节码。我们来看 `LOAD_CONST` ，字面意义将一个常量压栈，而操作数中的 `1` 表示载入索引为 `1` 的常量。`(0)` 则是一个辅助提示，生成的字节码序列中并不会包含。对于 `LOAD_CONST` 字节码，它的查找位置是 `co_consts` 。这个列表存储了所有的常量 `(None, 0, 128)`

`BUILD_LIST` 字节码会弹出栈中的 N 个元素组成一个 Python 的 `list` 对象再压栈，这里的 N 为 1，那么组成的便是 `[0]` 这个对象。接着 `LOAD_CONST 2` 将 `128` 压入。此时栈中的数据为

```
128
[0]
```

`BINARY_OP` 是通用的二元运算字节码，之前版本的 Python 会有不同类型的二元操作字节码，比如 `BINARY_MULTIPLY`；现在则是通过操作数 `5` 来表示运算的类型。这里是乘法。执行此字节码会弹出栈顶的两个元素，然后执行 `__mul__` 函数，再将结果压栈

`STORE_FAST` 会将栈顶弹出，然后把值保存在`co_varnames` 中。下标 `0` 对应这变量 `a`。`LOAD_GLOBAL`, `LOAD_FAST`这两个操作分别入栈 `NULL`, 函数 `print`，变量 `a`，然后 `CALL` 会执行函数调用。`CALL` 这里稍稍有点复杂，是因为 Python 的位置参数和命名参数的排列比较多样，文档如下

- > CALL(_argc_)
  >
  > Calls a callable object with the number of arguments specified by `argc`, including the named arguments specified by the preceding [`KW_NAMES`](https://docs.python.org/3/library/dis.html#opcode-KW_NAMES), if any. On the stack are (in ascending order), either: NULL The callable The positional arguments The named arguments or: The callable `self` The remaining positional arguments The named arguments `argc` is the total of the positional and named arguments, excluding `self` when a `NULL` is not present. `CALL` pops all arguments and the callable object off the stack, calls the callable object with those arguments, and pushes the return value returned by the callable object. Added in version 3.11.
  >
  > - NULL
  > - The callable
  > - The positional arguments
  > - The named arguments

我们再来看 `code` 对象，[文档](https://docs.python.org/3/reference/datamodel.html#code-objects)这里解释了 `co_varnames` / `co_cellvars` / `co_consts` 等变量的具体意义。Python 提供了一个 `       mportlib._bootstrap_external.1(code_object)` 函数可以将 `code` 对象转换成 `pyc` 文件的字节内容。如果不通过这个函数，我们需要计算一个 header，它包含了代表 Python 版本的 magic number，时间戳或者文件 hash 等，然后跟上序列话后的 code 对象

```python
# https://github.com/python/cpython/blob/d60b97a833fd3284f2ee249d32c97fc359d83486/Lib/importlib/_bootstrap_external.py#L521
def _code_to_timestamp_pyc(code, mtime=0, source_size=0):
    "Produce the data for a timestamp-based pyc."
    data = bytearray(MAGIC_NUMBER)
    data.extend(_pack_uint32(0))
    data.extend(_pack_uint32(mtime))
    data.extend(_pack_uint32(source_size))
    data.extend(marshal.dumps(code))
    return data
```

有了以上的基础我们便可以写一个 Brainfuck 的 transpiler 了。在上一篇文章中我们抽象出来了如下几种内部表示

```c
typedef enum {
    INCREMENT_PTR,
    DECREMENT_PTR,
    INCREMENT_VAL,
    DECREMENT_VAL,
    OUTPUT_VAL,
    INPUT_VAL,
    LOOP_BEGIN,
    LOOP_END
} OpcodeType;
```

我们需要寻找在 Python 中的等价表示。一种方法是我们通过编写相同语义的 Python 代码，然后通过 `dis` 进行查找。比如我们在上面看到的 `[0] * 128` 这个正好是初始化 Brainfuck 的代码

比如 `ptr += 1`

```Python
def increment_ptr(self) -> List[ByteCode]:
    """
    ptr += 1
    """
    return [
        LOAD_FAST(self.varname_index("ptr")),
        LOAD_CONST(self.const_index(1)),
        BINARY_OP(NB_ADD),
        STORE_FAST(self.varname_index("ptr")),
    ]

```

`varname_index` 这里是我封装的函数，用于计算下标。这段代码生成的字节码的字节序列是 `7c0064017a0d00007d00`

- `LOAD_FAST 00 (ptr)` 对应 `7c 00`

- `LOAD_CONST 01 (1)` 对应 `64 01`
- `BINARY_OP 13 ` 对应 `7a 0d 00`

可以看到这里 `BINARY_OP` 的字节序列是 3 个字节，后面跟了 `00`。这个是因为

> Changed in version 3.11: Some instructions are accompanied by one or more inline cache entries, which take the form of [`CACHE`](https://docs.python.org/3/library/dis.html#opcode-CACHE) instructions. These instructions are hidden by default, but can be shown by passing `show_caches=True` to any [`dis`](https://docs.python.org/3/library/dis.html#module-dis) utility. Furthermore, the interpreter now adapts the bytecode to specialize it for different runtime conditions. The adaptive bytecode can be shown by passing `adaptive=True`.

这里在调试的时候需要额外注意，不能两个一组分割指令

`decrement_ptr` 的实现相似，这里不赘述。我们来看 `increment_value`

```Python
def increment_value(self) -> List[ByteCode]:
    """
    stack[ptr] += 1
    """
    return [
        LOAD_NAME(self.name_index("stack")),
        LOAD_FAST(self.varname_index("ptr")),
        COPY(2),
        COPY(2),
        BINARY_SUBSCR(0),
        LOAD_CONST(self.const_index(1)),
        BINARY_OP(NB_ADD),
        SWAP(3),
        SWAP(2),
        STORE_SUBSCR(0),
    ]

```

> - COPY(_i_)
>
>   Push the i-th item to the top of the stack without removing it from its original location: `assert i > 0 STACK.append(STACK[-i]) ` Added in version 3.11.
>
> - BINARY_SUBSCR
>
>   Implements: `key = STACK.pop() container = STACK.pop() STACK.append(container[key]) `
>
> - SWAP(_i_)
>
>   Swap the top of the stack with the i-th element: `STACK[-i], STACK[-1] = STACK[-1], STACK[-i] ` Added in version 3.11.
>
> - STORE_SUBSCR
>
>   Implements: `key = STACK.pop() container = STACK.pop() value = STACK.pop() container[key] = value`

这里连续的两个 `COPY(2)` ，第一条入栈了 `stack`，第二条入栈了 `ptr` 。相当于重复了栈上的元素。我们可以看一下栈的变化就可以理解为什么插入了 `COPY` 和 `SWAP` 字节码

```
ptr
stack

- After COPY(2)

stack
ptr
stack

- After COPY(2)

ptr
stack
ptr
stack

- After BINARY_SUBSCR

stack[ptr]
ptr
stack

- After LOAD_CONST(self.const_index(1)), BINARY_OP(NB_ADD),

stack[ptr] + 1
ptr
stack

- After SWAP(3)

stack
ptr
stack[ptr] + 1

- After SWAP(2)

ptr ; key
stack ; container
stack[ptr] + 1 ; value

```

最后再来看一下循环的实现

```Python
def loop_begin(self, offset: int) -> List[ByteCode]:
    """
    while stack[ptr]:
    """
    return [
        LOAD_NAME(self.name_index("stack")),
        LOAD_FAST(self.varname_index("ptr")),
        BINARY_SUBSCR(0),
        LOAD_CONST(self.const_index(0)),
        COMPARE_OP(CMP_EQ),
        # for long jump
        EXTENDED_ARG((offset >> 16) & 0xFF),
        EXTENDED_ARG((offset >> 8) & 0xFF),
        POP_JUMP_IF_TRUE(offset & 0xFF),
    ]

def loop_end(self, offset: int) -> List[ByteCode]:
    """
    end
    """
    return [
        # for long jump
        EXTENDED_ARG((offset >> 16) & 0xFF),
        EXTENDED_ARG((offset >> 8) & 0xFF),
        JUMP_BACKWARD(offset & 0xFF),
    ]

```

在平常我们的操作数都是 一个字节的，它最多表达 255 条指令偏移。但是对于跳转来说，有可能是会超过的，所以需要借助 `EXTEND_ARG` 扩展长度，将接下来的几个字节都用于表示操作数

- > EXTENDED*ARG(\_ext*)
  >
  > Prefixes any opcode which has an argument too big to fit into the default one byte. _ext_ holds an additional byte which act as higher bits in the argument. For each opcode, at most three prefixal `EXTENDED_ARG` are allowed, forming an argument from two-byte to four-byte.

`POP_JUMP_IF_TRUE` 是真值判断跳转，在这个上下文中则代表如果满足等于 `0` 的条件那么跳转。它的参数是循环外的第一条指令的位置。也即是说不等于 `0` 则进入循环。而这个循环中的最后一条字节码是 `JUMP_BACKWARD` ，他会直接跳转会循环开始比较 `0` 的位置。这两个字节码的偏移量不要搞错，因为本身它们也带有后缀的 `CACHE` `00` 的

以上，我们便可以翻译所有的 Brainfuck 指令到 Python 字节码。解析 Brainfuck 的逻辑可以沿用上一篇文章的，完整代码在 [GitHub](https://github.com/Hanaasagi/kaleido/blob/de39baeeb57d20374ce19193d9d2291b78af557a/brainfuck/transpiler.py)
