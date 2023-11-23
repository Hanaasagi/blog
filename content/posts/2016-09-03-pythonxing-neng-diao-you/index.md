
+++
title = "Python性能调优"
summary = ''
description = ""
categories = []
tags = []
date = 2016-09-03T06:51:55+08:00
draft = false
+++

[Python性能分析与优化](http://www.ituring.com.cn/book/1744) 阅读笔记

性能分析软件有两类方法论:基于事件的性能分析(event-based profiling)和统计式性能分析(statistical profiling)。

-  基于事件的性能分析器(event-based profiler,也称为轨迹性能分析器,tracing profiler)是通过收集程序执行过程中的具体事件进行工作的。这些性能分析器会产生大量的数据。基本上,它们需要监听的事件越多,产生的数据量就越大。
-  统计式性能分析器以固定的时间间隔对程序计数器(program counter)进行抽样统计。这样做可以让开发者掌握目标程序在每个函数上消耗的时间。由于它对程序计数器进行抽样,所以数据结果是对真实值的统计近似。

优化通常被认为是一个好习惯。但是,如果违背了软件的设计原则就不好了。在开始开发一个新软件时,经常犯的错误就是过早优化(permature optimization)。如果进行了过早优化,结果可能会和原来的代码截然不同。它可能只是完整解决方案的一部分,还可能包含因优化驱动的设计决策而导致的错误。一条经验法则是,如果还没有对代码做过性能分析,优化往往不是个好主意。首先,应该集中精力完成代码,然后通过性能分析发现真正的瓶颈,最后对代码进行优化。


fib.py是一段未经任何优化的求斐波那契数列的代码(请勿吐槽这段代码，因为它实在是很糟糕)，下文将围绕这段代码进行探讨


	# -*-coding:UTF-8 -*-
	def fib(n):
	    if n <= 1:
	        return n
	    else:
	        return fib(n - 1) + fib(n - 2)


	def fib_seq(n):
	    seq = []
	    if n > 0:
	        seq.extend(fib_seq(n - 1))
	    seq.append(fib(n))
	    return seq

	fib_seq(20)

#### cProfile
cProfile 是一种基于事件的性能分析器，它只测量CPU时间,并不关心内存消耗和其他与内存相关的信息统计。
在终端中我们可以使用

	python -m cProfile fib.py -o fib.prof
来使用 cProfile 进行性能分析,结果如下

	         57355 function calls (65 primitive calls) in 0.012 seconds

	   Ordered by: standard name

	   ncalls  tottime  percall  cumtime  percall filename:lineno(function)
	        1    0.000    0.000    0.012    0.012 fib.py:2(<module>)
	 57291/21    0.012    0.000    0.012    0.001 fib.py:2(fib)
	     21/1    0.000    0.000    0.012    0.012 fib.py:9(fib_seq)
	       21    0.000    0.000    0.000    0.000 {method 'append' of 'list' objects}
	        1    0.000    0.000    0.000    0.000 {method 'disable' of '_lsprof.Profiler' objects}
	       20    0.000    0.000    0.000    0.000 {method 'extend' of 'list' objects}

从这个结果中可以收集到如下信息：
第一行告诉我们一共有57355个函数调用被监控,其中65个是原生(primitive)调用,这些调用不涉及递归。
ncalls 表示函数调用的次数。如果在这一列中有两个数值,就表示有递归调用。第二个数值是原生调用的次数,第一个数值是总调用次数。这个数值可以帮助识别潜在的bug (当性能分析器数值大得异乎寻常时可能就是bug),或者可能需要进行内联函数扩展(inline expansion)的位置。
tottime 是函数内部消耗的总时间(不包括调用其他函数的时间)。这列信息可以帮助找到可以进行优化的、运行时间较长的循环。
第一个percall 是 tottime 除以 ncalls ,表示一个函数每次调用的平均消耗时间。另一个 percall 是用 cumtime 除以原生调用的数量,表示到该函数调用时,每个原生调用的平均消耗时间。
cumtime 是之前所有子函数消耗时间的累计和(也包含递归调用时间)。这个数值可以帮助开发者从整体视角识别性能问题,比如算法选择错误。
filename:lineno(function) 显示了被分析函数所在的文件名、行号、函数名。

我们也可以通过 cProfile 提供的方法进行分析。

	# -*-coding:UTF-8 -*-
	import cProfile


	def fib(n):
	    if n <= 1:
	        return n
	    else:
	        return fib(n - 1) + fib(n - 2)


	def fib_seq(n):
	    seq = []
	    if n > 0:
	        seq.extend(fib_seq(n - 1))
	    seq.append(fib(n))
	    return seq

	prof = cProfile.Profile()
	prof.enable()
	fib_seq(20)
	prof.create_stats()
	prof.dump_stats('fib.prof')
	prof.print_stats()

-  enable() :表示开始收集性能分析数据。
-  disable() :表示停止收集性能分析数据。
-  create_stats() :表示停止收集数据,并为已收集的数据创建 stats 对象。
-  print_stats(sort=-1) :打印分析结果。
-  dump_stats(filename) :把当前性能分析的内容写进一个文件。
-  run(cmd) :用来收集命令执行的性能统计信息
-  runctx(cmd, globals, locals) :收集命令执行的性能统计信息,支持两个字典参数: globals
和 locals
-  runcall(func, *args, **kwargs) :收集被调用函数 func 的性能分析信息。

pstats 模块提供了Stats类,可以读取和操作stats文件(Stats类的构造器可以接收 cProfile.Profile 类型的参数,可以不用文件名称作为数据源)

	import pstats
	p = pstats.Stats('fib.prof')
	p.print_stats()
	p.print_callers()

print_callers会显示程序执行过程中调用的每个函数的调用次数、总时间和累计时间,以及文件名、行号和函数名的组合。

	         57355 function calls (65 primitive calls) in 0.016 seconds

	   Random listing order was used

	   ncalls  tottime  percall  cumtime  percall filename:lineno(function)
	       21    0.000    0.000    0.000    0.000 {method 'append' of 'list' objects}
	     21/1    0.000    0.000    0.016    0.016 fib.py:11(fib_seq)
	       20    0.000    0.000    0.000    0.000 {method 'extend' of 'list' objects}
	        1    0.000    0.000    0.000    0.000 {method 'disable' of '_lsprof.Profiler' objects}
	        1    0.000    0.000    0.000    0.000 /usr/lib/python2.7/cProfile.py:90(create_stats)
	 57291/21    0.016    0.000    0.016    0.001 fib.py:4(fib)


	   Random listing order was used

	Function                                          was called by...
	                                                      ncalls  tottime  cumtime
	{method 'append' of 'list' objects}               <-      21    0.000    0.000  fib.py:11(fib_seq)
	fib.py:11(fib_seq)                                <-    20/1    0.000    0.011  fib.py:11(fib_seq)
	{method 'extend' of 'list' objects}               <-      20    0.000    0.000  fib.py:11(fib_seq)
	{method 'disable' of '_lsprof.Profiler' objects}  <-       1    0.000    0.000  /usr/lib/python2.7/cProfile.py:90(create_stats)
	/usr/lib/python2.7/cProfile.py:90(create_stats)   <-
	fib.py:4(fib)                                     <- 57270/38    0.016    0.016  fib.py:4(fib)
	                                                          21    0.000    0.016  fib.py:11(fib_seq)




这个结果表示左边的函数是被右边的函数调用的
#### line_profiler
这个性能分析器和 cProfile 不同，它可以一行一行地分析函数性能。安装 line_profiler 时也会安装 kernprof 。kernprof 会创建一个性能分析器实例,并把名字动态添加到 `__builtins__` 命名空间的 profile 中，所以只要在需要测试的函数加上`@profile`，不用import任何东西，
这里笔者使用 pip 安装一直出现NameError: name 'profile' is not defined， 通过[github](https://github.com/rkern/line_profiler)安装则未出现这种问题

fib.py

	@profile
	def fib(n):
	    if n <= 1:
	        return n
	    else:
	        return fib(n - 1) + fib(n - 2)


	fib(10)

运行

	kernprof -l -v fib.py

-l表示动态插入profile
-v 当程序运行结束后，将结果输出到标准输出，而不是到文件中
输出

	Wrote profile results to fib.py.lprof
	Timer unit: 1e-06 s

	Total time: 0.000158 s
	File: fib.py
	Function: fib at line 2

	Line #      Hits         Time  Per Hit   % Time  Line Contents
	==============================================================
	     2                                           @profile
	     3                                           def fib(n):
	     4       177           51      0.3     32.3      if n <= 1:
	     5        89           24      0.3     15.2          return n
	     6                                               else:
	     7        88           83      0.9     52.5          return fib(n - 1) + fib(n - 2)


Line # :表示文件中的行号。
Hits :性能分析时一行代码的执行次数。
Time :一行代码执行的总时间,由计时器的单位决定。在分析结果的最开始有一行 Timer
unit ,该数值就是转换成秒的计时单位(要计算总时间,需要用 Time 数值乘以计时单位)。不同系统的计时单位可能不同。
Per hit :执行一行代码的平均消耗时间,依然由系统的计时单位决定。
% Time :执行一行代码的时间消耗占程序总消耗时间的比例。


#### memory_profiler
	pip install memory_profiler
	pip install psutil

运行

	python -m memory_profiler fib.py
输出

	Filename: fib.py

	Line #    Mem usage    Increment   Line Contents
	================================================
	     2   20.738 MiB    0.000 MiB   @profile
	     3                             def fib(n):
	     4   20.738 MiB    0.000 MiB       if n <= 1:
	     5   20.738 MiB    0.000 MiB           return n
	     6                                 else:
	     7   20.738 MiB    0.000 MiB           return fib(n - 1) + fib(n - 2)

####优化
这里以fib.py为例进行优化，性能分析结果如下

	         57355 function calls (65 primitive calls) in 0.012 seconds

	   Ordered by: standard name

	   ncalls  tottime  percall  cumtime  percall filename:lineno(function)
	        1    0.000    0.000    0.012    0.012 fib.py:2(<module>)
	 57291/21    0.012    0.000    0.012    0.001 fib.py:2(fib)
	     21/1    0.000    0.000    0.012    0.012 fib.py:9(fib_seq)
	       21    0.000    0.000    0.000    0.000 {method 'append' of 'list' objects}
	        1    0.000    0.000    0.000    0.000 {method 'disable' of '_lsprof.Profiler' objects}
	       20    0.000    0.000    0.000    0.000 {method 'extend' of 'list' objects}

在0.012秒内, 共有57355个函数调用,其中只有65个原生调用
`fib()`有57291次递归调用,`fib_seq()`有21次递归调用
由图中可见,大部分时间都消耗在`fib()`的递归中了
#####step1
现在, 让我们给`fib()`加一个可以缓存之前计算的值的装饰器,缓存之前计算的值，这样我们可以省去不必要的重复计算。

	from functools import wraps


	def memoize(function):
	    memo = {}

	    @wraps(function)
	    def wrapper(*args):
	        if args in memo:
	            return memo[args]
	        else:
	            rv = function(*args)
	            memo[args] = rv
	            return rv
	    return wrapper


	@memoize
	def fib(n):
	    if n <= 1:
	        return n
	    else:
	        return fib(n - 1) + fib(n - 2)


	def fib_seq(n):
	    seq = []
	    if n > 0:
	        seq.extend(fib_seq(n - 1))
	    seq.append(fib(n))
	    return seq

	fib_seq(20)

性能分析结果

	         156 function calls (98 primitive calls) in 0.000 seconds

	   Ordered by: standard name

	   ncalls  tottime  percall  cumtime  percall filename:lineno(function)
	        1    0.000    0.000    0.000    0.000 fib.py:1(<module>)
	       21    0.000    0.000    0.000    0.000 fib.py:18(fib)
	     21/1    0.000    0.000    0.000    0.000 fib.py:26(fib_seq)
	        1    0.000    0.000    0.000    0.000 fib.py:4(memoize)
	    59/21    0.000    0.000    0.000    0.000 fib.py:7(wrapper)
	        1    0.000    0.000    0.000    0.000 functools.py:17(update_wrapper)
	        1    0.000    0.000    0.000    0.000 functools.py:39(wraps)
	        5    0.000    0.000    0.000    0.000 {getattr}
	       21    0.000    0.000    0.000    0.000 {method 'append' of 'list' objects}
	        1    0.000    0.000    0.000    0.000 {method 'disable' of '_lsprof.Profiler' objects}
	       20    0.000    0.000    0.000    0.000 {method 'extend' of 'list' objects}
	        1    0.000    0.000    0.000    0.000 {method 'update' of 'dict' objects}
	        3    0.000    0.000    0.000    0.000 {setattr}


程序的函数调用次数下降到了156,递归调用明显减少了。运行时间也下降到了0.000秒(应该是低于测量精度了)。但当我们尝试求更多位的斐波那契数列时，会发生栈溢出。

##### step2
由于 cPython 为了防止栈溢出,设置了递归保护措施，笔者执行`fib_seq(989)`时就会发生`RuntimeError: maximum recursion depth exceeded`了
理论上如果我们使用尾递归的话就不会出现这种问题了(尾递归等价迭代)，但是 cPython 并不会对尾递归进行优化
这里有一种Hack的做法 参考自[谁说Python不能尾递归优化](http://www.jianshu.com/p/d36746ad845d)


	from functools import wraps
	import types


	def memoize(function):
	    memo = {}

	    @wraps(function)
	    def wrapper(*args):
	        if args in memo:
	            return memo[args]
	        else:
	            rv = function(*args)
	            memo[args] = rv
	            return rv
	    return wrapper


	def fib(count, cur=0, next_=1):
	    if count < 1:
	        yield cur
	    else:
	        yield fib(count - 1, next_, cur + next_)


	def tramp(gen, *args, **kwargs):
	    g = gen(*args, **kwargs)
	    while isinstance(g, types.GeneratorType):
	        g = g.next()
	    return g


	def fib_seq(n):
	    seq = []
	    for i in range(0, n + 1):
	        seq.append(tramp(fib, i))
	    return seq

	fib_seq(988)

性能分析结果

	         1471636 function calls in 0.478 seconds

	   Ordered by: standard name

	   ncalls  tottime  percall  cumtime  percall filename:lineno(function)
	        1    0.000    0.000    0.478    0.478 fib.py:1(<module>)
	   979110    0.141    0.000    0.141    0.000 fib.py:19(fib)
	      989    0.305    0.000    0.477    0.000 fib.py:26(tramp)
	        1    0.000    0.000    0.478    0.478 fib.py:33(fib_seq)
	   490544    0.030    0.000    0.030    0.000 {isinstance}
	      989    0.000    0.000    0.000    0.000 {method 'append' of 'list' objects}
	        1    0.000    0.000    0.000    0.000 {method 'disable' of '_lsprof.Profiler' objects}
	        1    0.000    0.000    0.000    0.000 {range}


但是我们未进行step2优化的 fib_seq(988) 结果为

	         6932 function calls (3970 primitive calls) in 0.005 seconds

	   Ordered by: standard name

	   ncalls  tottime  percall  cumtime  percall filename:lineno(function)
	        1    0.000    0.000    0.005    0.005 fib.py:1(<module>)
	      989    0.001    0.000    0.001    0.000 fib.py:18(fib)
	    989/1    0.003    0.000    0.005    0.005 fib.py:26(fib_seq)
	        1    0.000    0.000    0.000    0.000 fib.py:4(memoize)
	 2963/989    0.001    0.000    0.001    0.000 fib.py:7(wrapper)
	        1    0.000    0.000    0.000    0.000 functools.py:17(update_wrapper)
	        1    0.000    0.000    0.000    0.000 functools.py:39(wraps)
	        5    0.000    0.000    0.000    0.000 {getattr}
	      989    0.000    0.000    0.000    0.000 {method 'append' of 'list' objects}
	        1    0.000    0.000    0.000    0.000 {method 'disable' of '_lsprof.Profiler' objects}
	      988    0.001    0.000    0.001    0.000 {method 'extend' of 'list' objects}
	        1    0.000    0.000    0.000    0.000 {method 'update' of 'dict' objects}
	        3    0.000    0.000    0.000    0.000 {setattr}

经过对比我们发现虽然我们避免了栈溢出，但是反而变慢了几个数量级

##### step3
既然尾递归等价于迭代，我们直接使用迭代就好了

	from functools import wraps


	def memoize(function):
	    memo = {}

	    @wraps(function)
	    def wrapper(*args):
	        if args in memo:
	            return memo[args]
	        else:
	            rv = function(*args)
	            memo[args] = rv
	            return rv
	    return wrapper


	@memoize
	def fib(n):
	    a, b = 0, 1
	    for i in range(0, n):
	        a, b = b, a + b
	    return a


	def fib_seq(n):
	    seq = []
	    for i in range(0, n + 1):
	        seq.append(fib(i))
	    return seq

	fib_seq(988)


性能分析结果

	         3972 function calls in 0.037 seconds

	   Ordered by: standard name

	   ncalls  tottime  percall  cumtime  percall filename:lineno(function)
	        1    0.000    0.000    0.037    0.037 fib.py:1(<module>)
	      989    0.034    0.000    0.036    0.000 fib.py:18(fib)
	        1    0.000    0.000    0.037    0.037 fib.py:26(fib_seq)
	        1    0.000    0.000    0.000    0.000 fib.py:4(memoize)
	      989    0.001    0.000    0.037    0.000 fib.py:7(wrapper)
	        1    0.000    0.000    0.000    0.000 functools.py:17(update_wrapper)
	        1    0.000    0.000    0.000    0.000 functools.py:39(wraps)
	        5    0.000    0.000    0.000    0.000 {getattr}
	      989    0.000    0.000    0.000    0.000 {method 'append' of 'list' objects}
	        1    0.000    0.000    0.000    0.000 {method 'disable' of '_lsprof.Profiler' objects}
	        1    0.000    0.000    0.000    0.000 {method 'update' of 'dict' objects}
	      990    0.002    0.000    0.002    0.000 {range}
	        3    0.000    0.000    0.000    0.000 {setattr}


现在我们的代码比之前快了不少，是较理想的选择

