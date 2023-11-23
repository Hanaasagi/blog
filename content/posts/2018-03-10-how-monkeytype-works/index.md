
+++
title = "MonkeyType 是如何获得运行时类型的"
summary = ''
description = ""
categories = []
tags = []
date = 2018-03-10T02:42:40+08:00
draft = false
+++

[MonkeyType](https://github.com/Instagram/MonkeyType) 项目能够从运行时收集函数参数和返回值的类型，以类型注解的方式添加到 Python 代码中。是不是感觉好爽，不过需要 Python 3.6+ 才行

### How to use

先来看一下如何使用

```Python
# some/module.py
def add(a, b):
    return a + b

# myscript.py
from some.module import add

print(add(1, 2))
```

通过 `monkeytype run myscript.py` 命令可以直接在当前目录中以 sqlite 数据库文件的形式 dump 运行时信息。接着我们使用 `monkeytype apply some.module` 命令将类型信息以注解的方式添加到代码中

```Python
def add(a: int, b: int) -> int:
    return a + b
```

这个库的局限是只能收集函数参数和返回值及生成器 `yield` 出的值的类型。而且比如下面的代码

```Python
add(1, 2)
add(1.0, 2.0)
```

生成的结果是 `a: Union[int, float], b: Union[int, float]`，但是这里实际上应当实现了 `+` 这种操作的类型，除此之外这里好像应当使用泛型来进行约束

### How it works

直白的讲，MonkeyType 就是通过自定义的 profile 来统计运行时信息的，将 stub 写入代码中是借助了 [retype](https://github.com/ambv/retype) 库
。如果你对详细做法感兴趣，可以接着往下看

根据 `entrypoint` 可以定位到入口

```Python
# cli.py
def entry_point_main() -> 'NoReturn':
    # 将当前路径加入 Python path，使得用户代码可以被正确地 import
    sys.path.insert(0, os.getcwd())
    sys.exit(main(sys.argv[1:], sys.stdout, sys.stderr))
```

蠢作者对于这里的 `'NoReturn'` 比较奇怪，[PEP 484](https://www.python.org/dev/peps/pep-0484/#the-noreturn-type)
 中在 `typing` 中加入了 `NoReturn`，为什么这里却是用的一个字符串呢？先来看字符串的用意

[PEP 484 runtime-or-type-checking](https://www.python.org/dev/peps/pep-0484/#runtime-or-type-checking)

>Sometimes there's code that must be seen by a type checker (or other static analysis tools) but should not be executed. For such situations the `typing` module defines a constant, `TYPE_CHECKING`, that is considered `True` during type checking (or other static analysis) but `False` at runtime. Example:

```Python
import typing

if typing.TYPE_CHECKING:
   import expensive_mod

def a_func(arg: 'expensive_mod.SomeClass') -> None:
   a_var = arg  # type: expensive_mod.SomeClass
   ...
```

>(Note that the type annotation must be enclosed in quotes, making it a "forward reference", to hide the expensive_mod reference from the interpreter runtime. In the # type comment no quotes are needed.)

这样可以处理循环 `import`

至于我们这里是因为 Python 3.6.1 没有 `NoReturn`，详情可以参考此 [PR](https://github.com/python/cpython/pull/1366)

**`NoReturn` 是在 Python 3.6.2 与 Python 3.5.4 加入进来的**

接下来看 `main` 函数，其主要是使用 `argparse` 来处理命令行参数。省略掉这部分代码后如下

```Python
def main(argv: List[str], stdout: IO, stderr: IO) -> int:
    handler = getattr(args, 'handler', None)

    with args.config.cli_context(args.command):
        try:
            handler(args, stdout, stderr)
        except HandlerError as err:
            print(f"ERROR: {err}", file=stderr)
            return 1

    return 0
```

`handler` 有四种 `run_handler`、`apply_stub_handler`、`print_stub_handler`、`list_modules_handler`，分别对应着 `run`、`apply`、`stub`、`list_modules` 参数

我们在这里只分析收集类型信息并 dump 至 sqlite 的 `run_handler`

```Python
def run_handler(args: argparse.Namespace, stdout: IO, stderr: IO) -> None:
    old_argv = sys.argv.copy()
    try:
        with trace(args.config):
            if args.m:
                # 删除 monkeytype 自己的参数部分
                sys.argv = sys.argv[3:]
                # 利用 runpy 来运行用户代码 https://docs.python.org/3/library/runpy.html
                runpy.run_module(args.script_path, run_name='__main__', alter_sys=True)
            else:
                sys.argv = sys.argv[2:]
                runpy.run_path(args.script_path, run_name='__main__')
    finally:
        sys.argv = old_argv
```

貌似玄机就在 `trace` 中了，`trace` 定义在 `__init__.py` 中，它的返回值是一个 Context Manager

```Python
# __init__.py
def trace(config: Optional[Config] = None) -> ContextManager:
    if config is None:
        config = get_default_config()
    return trace_calls(
        logger=config.trace_logger(),
        code_filter=config.code_filter(),
        sample_rate=config.sample_rate(),
    )
```

来看 `trace_calls` 的定义

```Python
# tracing.py
@contextmanager
def trace_calls(
    logger: CallTraceLogger,
    code_filter: Optional[CodeFilter] = None,
    sample_rate: Optional[int] = None,
) -> Iterator[None]:
    """Enable call tracing for a block of code"""
    old_trace = sys.getprofile()
    sys.setprofile(CallTracer(logger, code_filter, sample_rate))
    try:
        yield
    finally:
        sys.setprofile(old_trace)
        logger.flush()
```

通过 `sys.setprofile` 添加了一个 hook，这就是 MonkeyType 的奥秘了

```Python
# tracing.py
class CallTracer:
    def __init__(
        self,
        logger: CallTraceLogger,
        code_filter: Optional[CodeFilter] = None,
        sample_rate: Optional[int] = None
    ) -> None:
        self.logger = logger
        self.traces: Dict[FrameType, CallTrace] = {}
        self.sample_rate = sample_rate
        self.cache: Dict[CodeType, Optional[Callable]] = {}
        self.should_trace = code_filter
```

参考 [文档](https://docs.python.org/3/library/sys.html#sys.setprofile)

>Profile functions should have three arguments: `frame`, `event`, and `arg`. `frame` is the current stack frame. `event` is a string: `'call'`, `'return'`, `'c_call'`, `'c_return'`, or `'c_exception'`. `arg` depends on the event type.

```Python
# tracing.py
EVENT_CALL = 'call'
EVENT_RETURN = 'return'
SUPPORTED_EVENTS = {EVENT_CALL, EVENT_RETURN}

class CallTracer:
    def __call__(self, frame: FrameType, event: str, arg: Any) -> 'CallTracer':
        code = frame.f_code
        if (
            event not in SUPPORTED_EVENTS or
            code.co_name == 'trace_types' or
            self.should_trace and not self.should_trace(code)
        ):
            return self
        try:
            if event == EVENT_CALL:
                self.handle_call(frame)
            elif event == EVENT_RETURN:
                self.handle_return(frame, arg)
            else:
                logger.error("Cannot handle event %s", event)
        except Exception:
            logger.exception("Failed collecting trace")
        return self
```

Python 的栈帧直接对应这底层的 `PyFrameObject`，而且这是构成一个链表

```
// python 3.6.3
typedef struct _frame {
    PyObject_VAR_HEAD
    struct _frame *f_back;      /* previous frame, or NULL */
    PyCodeObject *f_code;       /* code segment */
    PyObject *f_builtins;       /* builtin symbol table (PyDictObject) */
    PyObject *f_globals;        /* global symbol table (PyDictObject) */
    PyObject *f_locals;         /* local symbol table (any mapping) */
    int f_lasti;                /* Last instruction if called */
    // ..
} PyFrameObject;
```

第一步我们获取了此栈帧对应的 code object。为了方便起见，这里统一的将后面所有会用到的 code object 的属性全部列出

- co_name: name with which this code object was defined
- co_code: string of raw compiled bytecode
- co_varnames: tuple of names of arguments and local variables
- co_argcount: number of arguments (not including keyword only arguments, `*` or `**` args)

`__call__` 忽略了不关心的事件，调用对应的事件处理函数 `handle_call`/`handle_return`

```Python
# tracing.py
YIELD_VALUE_OPCODE = opcode.opmap['YIELD_VALUE']
class CallTracer:
    def handle_call(self, frame: FrameType) -> None:
        if self.sample_rate and random.randrange(self.sample_rate) != 0:
            return
        func = self._get_func(frame)
        if func is None:
            return
        code = frame.f_code
        # I can't figure out a way to access the value sent to a generator via
        # send() from a stack frame.
        if code.co_code[frame.f_lasti] == YIELD_VALUE_OPCODE:
            return
        arg_names = code.co_varnames[0:code.co_argcount]
        arg_types = {}
        for name in arg_names:
            if name in frame.f_locals:
                arg_types[name] = get_type(frame.f_locals[name])
        self.traces[frame] = CallTrace(func, arg_types)
```

大致做法是从 code object 中取出参数变量名，然后到栈帧中去取出参数对象，最后获取类型。这里先不展开 `_get_func` 和 `get_type`，直接来看 `handle_return` 做了什么

```Python
# tracing.py
class CallTracer:
  def handle_return(self, frame: FrameType, arg: Any) -> None:
      typ = get_type(arg)
      last_opcode = frame.f_code.co_code[frame.f_lasti]
      trace = self.traces.get(frame)
      if trace is None:
          return
      elif last_opcode == YIELD_VALUE_OPCODE:
          trace.add_yield_type(typ)
      else:
          if last_opcode == RETURN_VALUE_OPCODE:
              trace.return_type = typ
          del self.traces[frame]
          self.logger.log(trace)
```

取出我们之前存的 `CallTrace` 对象，然后补上返回值类型或者 yield 出的类型

`_get_func` 的定义

```Python
# tracing.py
class CallTracer:
    def _get_func(self, frame: FrameType) -> Optional[Callable]:
        code = frame.f_code
        if code not in self.cache:
            self.cache[code] = get_func(frame)
        return self.cache[code]
```

`_get_func` 方法本质上是调用 `get_func`，不过在 `cache` 属性里缓存了函数对象。以 `code` 为 key 一是 immutable，而是因为调用相同函数时的栈帧是不同，但是 code object 是相同的

`get_func` 的实现如下

```Python
# tracing.py
def get_func(frame: FrameType) -> Optional[Callable]:
    code = frame.f_code
    if code.co_name is None:
        return None
    # 首先根据函数名在 global 空间中进行查找
    cand = frame.f_globals.get(code.co_name, None)
    # 确定 candidate 是否就是那个 code 的主人
    func = _has_code(cand, code)
    # 如果找不到则尝试从类方法或者实例方法中进行寻找
    # 取出第一个参数即 self/cls，然后遍历实例/类及父类的空间
    if func is None and code.co_argcount >= 1:
        first_arg = frame.f_locals.get(code.co_varnames[0])
        func = get_func_in_mro(first_arg, code)
    # 尝试从静态方法中查找
    if func is None:
        for v in frame.f_globals.values():
            # 判断是否为类
            if not isinstance(v, type):
                continue
            func = get_func_in_mro(v, code)
            if func is not None:
                break
    return func
```

需要注意的是从类空间中查询时必须通过 `code` 进行比较而不能是 `co_name`。因为类是一个命名空间，不同类中的方法是可以重名的。不同类中的方法即使代码相同，他们的 code object 也是不同的

看一下 `_has_code` 的定义

```Python
# tracing.py
def _has_code(func: Optional[Callable], code: CodeType) -> Optional[Callable]:
    while func is not None:
        func_code = getattr(func, '__code__', None)
        if func_code is code:
            return func
        # Attempt to find the decorated function
        func = getattr(func, '__wrapped__', None)
    return None
```

`__wrapped__` 属性存放被装饰器装饰前的原函数，标准库中的 `functools.lru_cache` 装饰器就会为新生成的函数添加这个属性。注意并不是所有的装饰器都会这么做，比如 `functools.partial`，这个行为是自定义的

这就是寻找参数对象的整个过程，接下来需要根据对象获取其在 `typing` 模块中的类型，这部分实现在 `typing.py` 的 `get_type` 函数中，本文就不再展开了

### Reference
[MonkeyType Source Code v18.2.0](https://github.com/Instagram/MonkeyType/tree/v18.2.0)

    