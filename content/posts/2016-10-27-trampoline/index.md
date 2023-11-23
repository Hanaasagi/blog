
+++
title = "Trampoline"
summary = ''
description = ""
categories = []
tags = []
date = 2016-10-27T14:32:59+08:00
draft = false
+++

Python 的递归是有最大限制的，超过这个限制便会抛出  

	RuntimeError: maximum recursion depth exceeded

`sys.getrecursionlimit()`可以查看递归的最大层数，默认为 1000

`sys.setrecursionlimit()` 可以改变这个最大层数 

cPython 不支持尾递归优化，但有些 hack 的做法可以绕过，比如 trampoline 这种神奇的技术。当调用递归函数时，先插入一个 trampoline 函数。通过这个函数来调用真正的递归函数，并且对其进行修改，不让它再次进行递归的函数调用，而是直接返回下次递归调用的参数，由 trampoline 来进行下一次“递归“调用。这样一层一层的递归调用就会变成一次一次的迭代调用。


~~个人认为 如果要使用这种 hack 的做法，不如去使用一种可以对尾递归进行优化的语言~~

参考自[http://code.activestate.com/recipes/474088/](http://code.activestate.com/recipes/474088/)，有少许改动

	#!/usr/bin/env python2.7
	# This program shows off a python decorator(
	# which implements tail call optimization. It
	# does this by throwing an exception if it is
	# it's own grandparent, and catching such
	# exceptions to recall the stack.
	
	import sys
	from functools import wraps
	
	
	class TailRecurseException(BaseException):
	
	    def __init__(self, args, kwargs):
	        self.args = args
	        self.kwargs = kwargs
	
	
	def tail_call_optimized(func):
	    """
	    This function decorates a function with tail call
	    optimization. It does this by throwing an exception
	    if it is it's own grandparent, and catching such
	    exceptions to fake the tail call optimization.
	
	    This function fails if the decorated
	    function recurses in a non-tail context.
	    """
	    @wraps(func)
	    def wrapper(*args, **kwargs):
	        f = sys._getframe()
	        if f.f_back and f.f_back.f_back \
	                and f.f_back.f_back.f_code == f.f_code:
	            raise TailRecurseException(args, kwargs)
	        else:
	            while True:
	                try:
	                    return func(*args, **kwargs)
	                except TailRecurseException as e:
	                    args = e.args
	                    kwargs = e.kwargs
	    return wrapper
	
	
	@tail_call_optimized
	def factorial(n, acc=1):
	    "calculate a factorial"
	    if n == 0:
	        return acc
	    return factorial(n - 1, n * acc)
	
	
	print factorial(10000)
	# prints a big, big number,
	# but doesn't hit the recursion limit.
	
	
	@tail_call_optimized
	def fib(i, current=0, next=1):
	    if i == 0:
	        return current
	    else:
	        return fib(i - 1, next, current + next)
	
	
	print fib(10000)
	# also prints a big number,
	# but doesn't hit the recursion limit.


这段代码最精彩地方的就是一发生递归就会抛出一个携带函数参数的自定义异常，然后通过循环的方式来再次调用(传入捕获到的参数)。这样我们虽然写代码时写的是递归，但是实际上发生的迭代。

上面的代码使用了 `sys._getframe()` 获取栈帧，以判断是否发生递归。这个平时写代码时一般很少接触到。

Python 文档中是这样表述的  


>    Return a frame object from the call stack. If optional integer depth is given, return the frame object that many calls below the top of the stack. If that is deeper than the call stack, ValueError is raised. The default for depth is zero, returning the frame at the top of the call stack.
>
>    CPython implementation detail: This function should be used for internal and specialized purposes only. It is not guaranteed to exist in all implementations of Python.


	# 获取当前栈帧对象
	f = sys._getframe()
	# 获取前一个栈帧对象
	f.f_back
	# 获取栈帧的代码对象
	f.f_code
	# 获取栈帧的代码对象名称
	f.f_code.co_name


另外当没有前一个栈帧时，f.f_back 会返回 None

回到之前的代码中，大致分析一下流程  
执行 factorial(1000) ，由于我们使用了tail_call_optimized 装饰器，实际上我们调用了 wrapper   
这时前一个栈帧的代码对象名称为 `<module>` (这里表达的有点不恰当，大概是这个意思)，前前栈帧的代码对象名称为 `execfile`  
进入 while True 死循环，执行 `func(*args, **kwargs)` 创建了新的栈帧。这时发生递归调用又进入新的 wrapper 栈帧中，但是栈帧对应的代码对象(f_code)不变，所以这里我们抛出了自定义异常 TailRecurseException ，并将此次递归的参数写入  
捕获到这个异常后，并用从异常对象中读出的参数再次执行 `func(*args, **kwargs)`  


**注意，我们并没有阻止栈帧的线性增长。**

另外还有一种利用 yield 的实现

[http://www.jianshu.com/p/d36746ad845d](http://www.jianshu.com/p/d36746ad845d)
    