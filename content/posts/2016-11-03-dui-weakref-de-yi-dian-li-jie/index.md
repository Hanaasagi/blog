
+++
title = "对 weakref 的一点理解"
summary = ''
description = ""
categories = []
tags = []
date = 2016-11-03T01:15:11+08:00
draft = false
+++

	class Node(object):
	
	    def __init__(self, data):
	        self.data = data
	        self.parent = None
	        self.children = []
	
	    def add_child(self, child):
	        self.children.append(child)
	        child.parent = self
	
	    def __del__(self):
	        print '__del__'
	
	
	n = Node(0)
	del n
	# __del__
	n1 = Node(1)
	n2 = Node(2)
	n1.add_child(n2)
	del n1 # no output
	n2.parent
	# <__main__.Node at 0x7fd87ad5c250>


双亲节点的指针指向孩子节点，孩子节点又指向双亲节点。这构成了循环引用，所以每个对象的引用计数都不可能变成 0 

我们可以手动使用 gc 模块来进行垃圾回收  

	import gc
	
	gc.collect()

糟糕的是，我们这里循环引用的对象都实现了 `__del__` 方法，gc 模块不会销毁这些对象，因为 gc 模块不知道应该先调用哪个对象的 `__del__` 方法。gc模块会把这样的对象放到 gc.garbage 中，并不会销毁对象。


	n1 = Node(1)
	n2 = Node(2)
	print n1, n2
	# <__main__.Node object at 0x7f94109906d0> <__main__.Node object at 0x7f9410990610>
	n1.add_child(n2)
	del n1
	del n2
	gc.collect()
	# 64
	gc.garbage
	# [<__main__.Node object at 0x7f94109906d0> <__main__.Node object at 0x7f9410990610>]



我们可以通过 weakref 来解决，如果一个对象只剩下一个弱引用，那么它是可以被垃圾回收的

	import weakref
	
	
	class Node(object):
	
	    def __init__(self, data):
	        self.data = data
	        self._parent = None
	        self.children = []
	
	    @property
	    def parent(self):
	        return None if self._parent is None else self._parent()
	
	    @parent.setter
	    def parent(self, node):
	        self._parent = weakref.ref(node, callback)
	
	    def add_child(self, child):
	        self.children.append(child)
	        child.parent = self
	
	
	def callback(ref):
	    print '__del__', ref
	
	
	n1 = Node(0)
	n2 = Node(2)
	print n1,n2
	# <__main__.Node object at 0x7fb0c2750c10> <__main__.Node object at 0x7fb0c2750d10>
	n1.add_child(n2)
	del n1
	# __del__ <weakref at 0x7fb0c26e75d0; dead>


不过，如果我们使用 weakref.ref() 创建弱引用，每次使用时都需要形如这样 xx() 来获取，有一点别扭。  可以通过 weakref.proxy() 这种来避免 () 操作。

	n = Node(10)
	p = weakref.proxy(n)
	p.data
	# 10

    