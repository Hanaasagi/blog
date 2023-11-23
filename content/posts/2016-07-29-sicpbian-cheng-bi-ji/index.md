
+++
title = "HAVE YOU READ YOUR SICP TODAY?"
summary = ''
description = ""
categories = []
tags = []
date = 2016-07-29T12:06:22+08:00
draft = false
+++

![](../../images/2016/08/86dac4f2abd6dca4c2443d198ba8a620_r.jpg)

SICP围绕*抽象*这个问题展开了深刻的讨论。
**为了更加通俗易懂，我这里使用python来表达(原书使用Scheme)**
首先看一个例子，我们如何来表达分数这种数据结构

#### 类

    class Fraction(object):
		def __init__(self, x, y):
			self.numer = x
			self.denom = y

		def __add__(self, other):
			return self.__class__(other.denom*self.numer+self.denom*other.numer, other.denom*self.denom)

		def __str__(self):
			return '{0}/{1}'.format(self.numer, self.denom)

#### 过程

    def fraction(numer, denom):
		def dispatch(m):
			if m==0:
				return numer
			elif m==1:
				return denom
			else:
				raise IndexError
		return dispatch


	def add(x, y):
		return fraction(x(0)*y(1)+y(0)*x(1), x(1)*y(1))

	def display(f):
		return '{0}/{1}'.format(f(0),f(1))

方法2体现了**数据也可以用过程**来表现，这种程序设计风格也称为**消息传递**。如果你觉得这种思维还不足以令人称奇，那么请考虑，我们是否需可以完全没有数。下面代码实现了0和加1的操作

    (define zero (lambda (f) (lambda (x) x)))
    (define (add-1 n) lambda (f) (lambda (x) (f ((n f ) x)))))

这一表示形式称为Chruch Numerals。


