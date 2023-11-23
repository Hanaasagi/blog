
+++
title = "Python利用ctypes提高执行速度"
summary = ''
description = ""
categories = []
tags = []
date = 2016-09-08T08:28:30+08:00
draft = false
+++

ctypes 库可以让开发者借助C语言进行开发。这个引入C语言的接口可以帮助我们做很多事情,比如需要调用C代码的来提高性能的一些小型问题。通过它你可以接入Windows系统上的 kernel32.dll 和 msvcrt.dll 动态链接库,以及Linux系统上的 libc.so.6 库。当然你也可以使用自己的编译好的共享库

我们先来看一个简单的例子
我们使用 Python 求 1000000 以内素数，重复这个过程10次，并计算运行时间。

	import math
	from timeit import timeit


	def check_prime(x):
	    values = xrange(2, int(math.sqrt(x)) + 1)
	    for i in values:
	        if x % i == 0:
	            return False
	    return True


	def get_prime(n):
	    return [x for x in xrange(2, n) if check_prime(x)]

	print timeit(stmt='get_prime(1000000)', setup='from __main__ import get_prime',
	             number=10)


Output

	42.8259568214


下面用C语言写一个的 check_prime 函数,然后把它当作共享库(动态链接库)导入

	#include <stdio.h>
	#include <math.h>
	int check_prime(int a)
	{
		int c;
		for ( c = 2 ; c <= sqrt(a) ; c++ ) {
			if ( a%c == 0 )
				return 0;
		}
		return 1;
	}

使用以下命令生成 .so (shared object)文件

`gcc -shared -o prime.so -fPIC prime.c`

	import ctypes
	import math
	from timeit import timeit
	check_prime_in_c = ctypes.CDLL('./prime.so').check_prime


	def check_prime_in_py(x):
	    values = xrange(2, int(math.sqrt(x)) + 1)
	    for i in values:
	        if x % i == 0:
	            return False
	    return True


	def get_prime_in_c(n):
	    return [x for x in xrange(2, n) if check_prime_in_c(x)]


	def get_prime_in_py(n):
	    return [x for x in xrange(2, n) if check_prime_in_py(x)]


	py_time = timeit(stmt='get_prime_in_py(1000000)', setup='from __main__ import get_prime_in_py',
	                 number=10)
	c_time = timeit(stmt='get_prime_in_c(1000000)', setup='from __main__ import get_prime_in_c',
	                number=10)
	print "Python version: {} seconds".format(py_time)

	print "C version: {} seconds".format(c_time)

Output

	Python version: 43.4539749622 seconds
	C version: 8.56250786781 seconds

我们可以看到很明显的性能差距
*[这里](http://blog.csdn.net/arvonzhang/article/details/8564836)有更多的方法去判断一个数是否是素数*

再来看一个复杂点的例子 快速排序


mylib.c

	#include <stdio.h>

	typedef struct _Range {
	    int start, end;
	} Range;

	Range new_Range(int s, int e) {
	    Range r;
	    r.start = s;
	    r.end = e;
	    return r;
	}

	void swap(int *x, int *y) {
	    int t = *x;
	    *x = *y;
	    *y = t;
	}

	void quick_sort(int arr[], const int len) {
	    if (len <= 0)
	        return;
	    Range r[len];
	    int p = 0;
	    r[p++] = new_Range(0, len - 1);
	    while (p) {
	        Range range = r[--p];
	        if (range.start >= range.end)
	            continue;
	        int mid = arr[range.end];
	        int left = range.start, right = range.end - 1;
	        while (left < right) {
	            while (arr[left] < mid && left < right)
	                left++;
	            while (arr[right] >= mid && left < right)
	                right--;
	            swap(&arr[left], &arr[right]);
	        }
	        if (arr[left] >= arr[range.end])
	            swap(&arr[left], &arr[range.end]);
	        else
	            left++;
	        r[p++] = new_Range(range.start, left - 1);
	        r[p++] = new_Range(left + 1, range.end);
	    }
	}



`gcc -shared -o mylib.so -fPIC mylib.c`

使用ctypes有一个麻烦点的地方是原生的C代码使用的类型可能跟Python不能明确的对应上来。比如这里什么是Python中的数组？列表？还是 array 模块中的一个数组。所以我们需要进行转换

test.py

	import ctypes
	import time
	import random

	quick_sort = ctypes.CDLL('./mylib.so').quick_sort
	nums = []
	for _ in range(100):
	    r = [random.randrange(1, 100000000) for x in xrange(100000)]
	    arr = (ctypes.c_int * len(r))(*r)
	    nums.append((arr, len(r)))

	init = time.clock()
	for i in range(100):
	    quick_sort(nums[i][0], nums[i][1])
	print "%s" % (time.clock() - init)


Output
	1.874907

与Python list 的 sort 方法进行对比

	import ctypes
	import time
	import random

	quick_sort = ctypes.CDLL('./mylib.so').quick_sort
	nums = []
	for _ in range(100):
	    nums.append([random.randrange(1, 100000000) for x in xrange(100000)])

	init = time.clock()
	for i in range(100):
	    nums[i].sort()
	print "%s" % (time.clock() - init)

Output

	2.501257

至于结构体，需要定义一个类，包含相应的字段和类型

	class Point(ctypes.Structure):
	    _fields_ = [('x', ctypes.c_double),
	                ('y', ctypes.c_double)]

除了导入我们自己写的C语言扩展文件，我们还可以直接导入系统提供的库文件，比如linux下c标准库的实现 glibc

	import time
	import random
	from ctypes import cdll
	libc = cdll.LoadLibrary('libc.so.6')  # Linux系统
	# libc = cdll.msvcrt # Windows系统
	init = time.clock()
	randoms = [random.randrange(1, 100) for x in xrange(1000000)]
	print "Python version: %s seconds" % (time.clock() - init)
	init = time.clock()
	randoms = [(libc.rand() % 100) for x in xrange(1000000)]
	print "C version : %s seconds" % (time.clock() - init)

Output

	Python version: 0.850172 seconds
	C version : 0.27645 seconds

