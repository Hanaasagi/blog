
+++
title = "Python 中的 attribute 和 property"
summary = ''
description = ""
categories = []
tags = []
date = 2016-10-20T05:28:20+08:00
draft = false
+++

#### attribute 和 property 的区别
attribute , property 这两个词翻译成中文的话，一个特性一个属性，感觉十分相近，读起来也感觉拗口。不如不翻译  (￣▽￣")
关于 attribute 我们来看这么一段文字

>When we reference an attribute of an object with something like `someObject.someAttr`, Python uses several special methods to get the someAttr attribute of the object.
>
>In the simplest case, attributes are simply instance variables. But that’s not the whole story. To see how we can control the meaning of attributes, we have to emphasize a distinction:
>
>    An attribute is a name that appears after an object name. This is the syntactic construct. For example, someObj.name.
>    An instance variable is an item in the internal `__dict__` of an object.
>
>The default semantics of an attribute reference is to provide access to the instance variable. When we say `someObj.name`, the default behavior is effectively `someObj.__dict__['name']`.
>
>This is not the only meaning an attribute name can have.
>
>In Semantics of Attributes we’ll look at the various ways we can control what an attribute name means.
>
>The easiest technique for controlling the meaning of an attribute name is to define a property. We’ll look at this in Properties.
>
>The most sophisticated technique is to define a descriptor. We’ll look at this in Descriptors.
>
>The most flexible technique is to override the special method names and take direct control over attribute processing. We’ll look at this in Attribute Access Exercises.

像 someObject.someAttr 这种形式，我们是引用的是对象的 attribute ， Python 使用几种特殊方法来获取对象的 attribute

简单来说，attribute 是实例变量。但这并不是所有情况，为了展现我们如何控制 attributes 的含义，我们必须先强调一个区别：


- attribute 是出现在对象名称后面的名称。这是语法结构。举个例子 someObj.name
- 实例变量是对象 `__dict__` 中的一项

attribute 的默认语义(semantics)是提供访问实例变量的途径。当我们表达 someObj.name 时，默认的行为是 `someObj.__dict__['name']`

~~我这里觉得原文表述不太对 ，因为当我们表达 someObj.name 时，这个 name 有可能在 `type(someObj).__dict__` 中，这也是默认的~~。

这并不是 attribute 拥有的唯一含义

我们有许多方法去控制 attribute 所代表的含义，比如定义一个 property;复杂点的方法是定义一个描述符(descriptor);灵活一点的话，可以重写特殊方法，直接控制其行为。

这里不再展开了，更详细的请戳[这里](http://itmaybeahack.com/book/python-2.6/html/p03/p03c05_properties.html)

所以，我们一般在中文技术书籍中看到的 属性 一词，应该是指 attribute (翻译成属性更贴近中文习惯)

我们再来谈谈 property ，这个我们经常使用，再来回顾一下

使用 property 一般有以下两种方式

通过装饰器

	class Demo(object):

	    def __init__(self, val):
	        self._x = val

	    # 经过装饰器后，x 为 property对象
	    @property
	    def x(self):
	        return self._x

	    # x 的setter方法是一个装饰器 返回新的 property 对象
	    @x.setter
	    def x(self, val):
	        self._x = val

	    @x.deleter
	    def x(self):
	        print 'del x'

通过创建 property 的实例

	class Demo2(object):

	    def __init__(self, val):
	        self._x = val

	    def getx(self):
	        return self._x

	    def setx(self, val):
	        self._x = val

	    def delx(self):
	        print 'del x'

	    x = property(getx, setx, delx)

我们可以使用 `__get__` 和 `__set__` 实现一个等价的 property
参考自[http://pyzh.readthedocs.io/en/latest/Descriptor-HOW-TO-Guide.html](http://pyzh.readthedocs.io/en/latest/Descriptor-HOW-TO-Guide.html)

	class Property(object):
	    "Emulate PyProperty_Type() in Objects/descrobject.c"

	    def __init__(self, fget=None, fset=None, fdel=None, doc=None):
	        self.fget = fget
	        self.fset = fset
	        self.fdel = fdel
	        self.__doc__ = doc

	    def __get__(self, obj, objtype=None):
	        if obj is None:
	            return self
	        if self.fget is None:
	            raise AttributeError, "unreadable attribute"
	        return self.fget(obj)

	    def __set__(self, obj, value):
	        if self.fset is None:
	            raise AttributeError, "can't set attribute"
	        self.fset(obj, value)

	    def __delete__(self, obj):
	        if self.fdel is None:
	            raise AttributeError, "can't delete attribute"
	        self.fdel(obj)

	    def setter(self, fset):
	        return type(self)(self.fget, fset, self.fdel, self.__doc__)

	    def deleter(self, fdel):
	        return type(self)(self.fget, self.fset, fdel, self.__doc__)

假如创建了 Demo 类的实例 demo
demo.x 首先会调用 `__getattribute__` 方法，查找实例变量和类变量中是否存在 x ，若存在会接着查找 x 是否有 `__get__` 如果有则会调用。此时的参数 obj 为 demo。通过调用注册好的 fget() 来返回 _x


一个奇怪的例子，~~大概没人会这么写~~

	class Demo3(object):

	    def __init__(self, val):
	        self._x = val
	        self.x = property(self.getx, self.setx, self.delx)

	    def getx(self):
	        return self._x

	    def setx(self, val):
	        self._x = val

	    def delx(self):
	        print 'del x'

	demo = Demo3(0)
	demo.x
	# <property at 0x7f26e24ae890>

我们尝试一下

	demo.x.__get__(demo, type(demo))

	TypeError: getx() takes exactly 1 argument (2 given)

这是因为我们已经和实例绑定，所以这里多传了一个参数

所以如果非要这么做，可以使用自己实现的 Property，并做如下修改

	class Property(object):
	    "Emulate PyProperty_Type() in Objects/descrobject.c"

	    def __init__(self, fget=None, fset=None, fdel=None, doc=None):
	        self.fget = fget
	        self.fset = fset
	        self.fdel = fdel
	        self.__doc__ = doc

	    def __get__(self, obj, objtype=None):
	        if obj is None:
	            return self
	        if self.fget is None:
	            raise AttributeError, "unreadable attribute"
	        # 不需要传 obj
	        return self.fget()


#### 关于描述符(descriptor)的一点拓展

我想我们都知道 property 是一个描述符(实现了`__get__` `__set__` `__delete__` 方法)

先来点概念性的东西

    资料描述符：同时定义了 __get__ 和 __set__ , 对于同名属性优先使用资料描述符
    非资料描述符:只定义了 __get__ ，对于同名属性优先使用字典中的属性

举个例子

	class NonNegative(object):

	    def __init__(self, default):
	        self.value = default

	    def __set__(self, instance, value):
	        if value > 0:
	            self.value = value
	        else:
	            raise ValueError()

	    def __get__(self, instance, cls):
	        return self.value


	class Demo(object):

	    x = NonNegative(0)

	    def __init__(self):
	        self.x = 1
	        self.y = 2

	print d1.__dict__
	# {'y': 2}
	print type(d1).__dict__
	# <dictproxy {'__dict__': <attribute '__dict__' of 'Demo' objects>,
	#  '__doc__': None,
	#  '__init__': <function __main__.__init__>,
	#  '__module__': '__main__',
	#  '__weakref__': <attribute '__weakref__' of 'Demo' objects>,
	#  'x': <__main__.NonNegative at 0x7f5e94189890>}>


上面定义了资料描述符，在实例中，字典属性 x 和描述符属性同名，则优先使用了资料描述符(作为描述符的 x 是一个类变量)，所以实例字典 `__dict__` 就没有x属性这一项， 如果去掉 `__set__` 则实例的 `__dict__` 会是 `{'x': 1, 'y': 2}`(非资料描述符)


另外上面代码实现的是对属性值的范围的检查，如果一个类中很多个属性都有相同的检查规则，使用 property 来修饰的话未免太过臃肿。所以我们可以向上面这样实现自己的描述符

但是上面这种写法存在很大的问题， x 和 y 都是类属性，他们在实例中共享。修改时会牵一发而动全身。


	d1 = Demo()
	d1.x = 100

	d2 = Demo()

	d2.x
	# 1

	d1.x
	# 1

我们需要将实例与它们在描述符中对应的值进行绑定，最简单的是使用字典。

	class NonNegative(object):

	    def __init__(self, default):
	        self.default = default
	        self.data = WeakKeyDictionary()     # 使用WeakKeyDictionary来取代普通的字典以防止内存泄露

	    def __get__(self, instance, owner):
	        """calls x.d, 且d不为负数，x为instance, type(x)为owner"""
	        return self.data.get(instance, self.default)

	    def __set__(self, instance, value):
	        if value < 0:
	            raise ValueError("Negative value not allowed: %s" % value)
	        # 使用字典弱引用确保实例的数据只属于实例本身，否则多个实例都共享相同的值
	        self.data[instance] = value


利用 描述符 这种黑魔法我们还可以其他有趣的事情，比如对类属性的缓存

	class PropertyCache(object):
	    """ a decorator to cache property
	    """

	    def __init__(self, func):
	        self.func = func

	    def __get__(self, obj, cls):
	        if not obj:
	            return self
	        value = self.func(obj)
	        setattr(obj, self.func.__name__, value)
	        return value


	class Demo(object):

	    def __init__(self):
	        self._property_to_be_cached = 'result'

	    @PropertyCache
	    def property_to_be_cached(self):
	        print('compute')
	        return self._property_to_be_cached


	demo = Demo()
	print demo.property_to_be_cached
	# compute
	# result
	print demo.property_to_be_cached
	# result

另外函数也有 `__get__` ，当函数被调用时，它就会把 `obj.f(*args)` 的调用转换成 `f(obj, *args)`；`klass.f(*args)` 变成调用 `f(*args)`

