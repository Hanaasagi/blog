
+++
title = "函数缓存"
summary = ''
description = ""
categories = []
tags = []
date = 2016-08-06T01:39:29+08:00
draft = false
+++

SICP在[练习3.27](http://sicp.readthedocs.io/en/latest/chp3/27.html)中提出记忆法(memoization)这种思想，采用这种思想的过程将前面已经计算的一些值维持在一个局部变量中。当这种过程在求值时，会查询这个局部变量，看看是否曾经计算过。如果找到将直接返回这个值。如斐波那契  

    (define (fib n)
	  (cond ((= n 0) 0)
			((= n 1) 1)
			(else (+ (fib (- n 1))
					 (fib (- n 2))))))


同一过程的记忆性版本  

    (define memo-fib
	  (memoize (lambda (n)
				  (cond ((= n 0) 0)
						((= n 1) 1)
						(else (+ (memo-fib (- n 1))
								 (memo-fib (- n 2))))))))


    (define (memoize f)
	  (let ((table (make-table)))
		(lambda (x)
		  (let ((previously-computed-result (lookup x table)))
			(or previously-computed-result
		    	(let ((result (f x)))
		          (insert! x result table)
				  result))))))


python也有类似的概念，称为函数缓存  

*python2+*  

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
	def fibonacci(n):
		if n < 2: return n
		return fibonacci(n - 1) + fibonacci(n - 2)

	fibonacci(25)  


*python3+*  

    from functools import lru_cache

	@lru_cache(maxsize=32)
	def fib(n):
		if n < 2:
		    return n
		return fib(n-1) + fib(n-2)
    