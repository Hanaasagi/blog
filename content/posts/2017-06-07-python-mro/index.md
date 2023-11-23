
+++
title = "Python MRO"
summary = ''
description = ""
categories = []
tags = []
date = 2017-06-07T07:11:05+08:00
draft = false
+++

采用多继承的语言都会面临钻石继承的问题，即可能存在多条通过子类访问父类的路径

```Python
class A(object):

    def __init__(self):
        self.bar = 'A'

    def method(self):
        return '{{{}}}'.format(self.bar)

class B(A):

    def __init__(self):
        self.bar = 'B'

    def method(self):
        return '[{}]'.format(super().method())

class C(A):

    def __init__(self):
        self.bar = 'C'

    def method(self):
        return '({})'.format(super().method())

class D(B, C):

    def __init__(self):
        self.bar = 'D'

    def method(self):
        return super().method()


print(D().method())
# Output
# [({D})]
```

上例是一个典型的钻石继承问题

```
        A
      /   \
     /     \
    B       C
     \     /
      \   /
        D
```

`super()` 用来调用父类的方法，在 Python 3 后可以直接使用 `super().method()`，而 Python 2 需要显示指定 `super(BaseClass, self).method()`。其实不仅这样，`super()` 实际上是实例化了一个 `super` 类的实例，其 docstring 如下

```
Init signature: super(self, /, *args, **kwargs)
Docstring:     
super() -> same as super(__class__, <first argument>)
super(type) -> unbound super object
super(type, obj) -> bound super object; requires isinstance(obj, type)
super(type, type2) -> bound super object; requires issubclass(type2, type)
Typical use to call a cooperative superclass method:
class C(B):
    def meth(self, arg):
        super().meth(arg)
This works for class methods too:
class C(B):
    @classmethod
    def cmeth(cls, arg):
        super().cmeth(arg)
Type:           type
```

`super` 是通过 MRO(Method Resolution Order) 来寻找父类的， MRO 是类继承关系的线性化表示。Python 的 MRO 可以通过属性 `__mro__` 直接查看

```Python
In [1]: D.__mro__
Out[1]: (__main__.D, __main__.B, __main__.C, __main__.A, object)
```

`super` 从第二个参数提取其 `MRO` 信息，然后根据第一个参数得到当前类，这样便可以计算出下一个应当访问的类

```
super(C, D) => A
super(B, D) => C
super(D, D) => B
```

那么 MRO 是如何计算的呢？

Python 2.3 前后分别使用了不同的 `mro` 生成方法，本文仅介绍目前使用的 [C3 方法](https://en.wikipedia.org/wiki/C3_linearization) ~~我不信还有人用 2.3 以前的版本~~

C3 线性化对类进行编号以满足以下两个约束条件：  

* 父类不比子类先被检查  
* 如果是从多个类中继承下来则优先检查先书写的类

步骤如下：

1) 每一个类的 MRO 都是由自身和其所有父类及父类的 MRO 经过 MERGE 得到的
2) MERGE 的步骤是从第一个 MRO 开始，比对第一个元素是否出现在其余 MRO 链的 tail 中(tail 是除第一个元素以外的所有元素)
  若出现则对下一个 MRO 的第一个元素进行 MERGE；若未出现则对当前 MRO 的下一个元素进行 MERGE，并删除其他 MRO 中的这个元素



Example

```
L(A) := [A]
L(B) := [B] + MERGE(L(A), [A])
      = [B] + MERGE([A], [A])
      = [B, A]
# 同理可得
L(C) := [C, A]
L(D) := [D] + MERGE(L(B) + L(C) + [B, C])
      = [D] + MERGE([B, A], [C, A], [B, C])
      = [D, B] + MERGE([A], [C, A], [C])
      # A 出现在了 [C, A] 的 tail 中
      = [D, B, C] + MERGE([A], [A])
      = [D, B, C, A]
```

计算代码

```Python
def merge(seqs):
    print('NOW MERGE [{}] = {}'.format(seqs[0][0],seqs))
    round = 0
    res = []
    while True:
      nonemptyseqs = [seq for seq in seqs if seq]
      if not nonemptyseqs:
          return res
      round += 1
      print('{:<2}round: candidates...'.format(round))
      for seq in nonemptyseqs: # find merge candidates among seq heads
          cand = seq[0];
          print('{:2}{}'.format(' ', cand))
          nothead = [s for s in nonemptyseqs if cand in s[1:]]
          if nothead:
              cand = None #reject candidate
          else:
              break
      if not cand:
          raise "Inconsistent hierarchy"
      res.append(cand)
      for seq in nonemptyseqs: # remove cand
          if seq[0] == cand:
              del seq[0]

def mro(C):
    mro_ = merge([[C]] + list(map(mro, C.__bases__)) + [list(C.__bases__)])
    assert list(C.__mro__) == mro_
    return mro_
```


### Reference
[C3 linearization](https://en.wikipedia.org/wiki/C3_linearization)  
[The Python 2.3 Method Resolution Order](https://www.python.org/download/releases/2.3/mro/)

    