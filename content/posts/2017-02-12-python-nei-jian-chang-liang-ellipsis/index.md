
+++
title = "Python 内建常量 Ellipsis"
summary = ''
description = ""
categories = []
tags = []
date = 2017-02-12T15:33:14+08:00
draft = false
+++

*version Python3.5.3*

查阅 Python 文档时发现了这么一个常量 `Ellipsis`

文档中表述如下   

**Ellipsis**  
The same as `...`. Special value used mostly in conjunction with extended slicing syntax for user-defined container data types.



    In [25]: ... is Ellipsis
    Out[25]: True


有了这个常量，我们便可以定义一些有趣的东西，比如一个和 Haskell 中类似的 `list`  


    λ [1, 2]
    [1,2]
    :: Num t => [t]
    λ [1..10]
    [1,2,3,4,5,6,7,8,9,10]
    :: (Enum t, Num t) => [t]
    λ [1, 4 .. 10]
    [1,4,7,10]
    :: (Enum t, Num t) => [t]

为此我们先来看看 Python 中的序列协议 `__getitem__`  

    In [1]: class Seq(object):
       ...:     def __getitem__(self, key):
       ...:         return key
       ...:     

    In [2]: seq = Seq()

    In [3]: seq[1, 2, 3, 4, 5]
    Out[3]: (1, 2, 3, 4, 5)

    In [4]: seq[1:2]
    Out[4]: slice(1, 2, None)

    In [5]: seq[1:10:2]
    Out[5]: slice(1, 10, 2)

    In [6]: seq[3, 1:10:2]
    Out[6]: (3, slice(1, 10, 2))


我们可以看出，传入的是一个 `tuple` 类型，如果有切片操作的话，则对应一个 `slice` 对象  

知道这点便好办了，至于无穷 `list` 我们可以使用 `generator` 来解决    


    class InfiniteSeq(object):
        def __getitem__(self, items):
            if isinstance(items, tuple):
                index = 0
                while index < len(items):
                    if items[index] is Ellipsis:
                        end = items[index+1] if index+1 < len(items) else float('INF')
                        if index > 1:
                            step = items[index-1] - items[index-2]
                        else:
                            step = 1 if end > items[index-1] else -1
                        value = items[index-1] + step
                        while (step > 0 and value <= end) or (step < 0 and value >= end):
                            yield value
                            value = value + step
                        index = index + 1
                    else:
                        yield items[index]
                    index = index + 1
            else:
                raise SyntaxError

测试结果  

    In [109]: seq = InfiniteSeq()

    In [110]: list(seq[1, 2, 3, 4, 5])
    Out[110]: [1, 2, 3, 4, 5]

    In [111]: list(seq[1, ..., 5])
    Out[111]: [1, 2, 3, 4, 5]

    In [112]: list(seq[1, 3, ..., 7])
    Out[112]: [1, 3, 5, 7]

    In [113]: list(seq[1, 3, ..., 8])
    Out[113]: [1, 3, 5, 7]

    In [114]: list(seq[1, 3, ..., 9])
    Out[114]: [1, 3, 5, 7, 9]

    In [115]: list(seq[10, 1, 3, ..., 9, 12])
    Out[115]: [10, 1, 3, 5, 7, 9, 12]

    In [116]: list(seq[10, ..., 1])
    Out[116]: [10, 9, 8, 7, 6, 5, 4, 3, 2, 1]

    In [117]: list(seq[10, 8, ..., 0])
    Out[117]: [10, 8, 6, 4, 2, 0]

    In [118]: for i in seq[1, ...]:
         ...:     print(i)
         ...:     if i > 5:
         ...:         break
         ...:     
    1
    2
    3
    4
    5
    6


Update 2017.05.06:
注 `Numpy` 在多维数组切片时有用到这个常量。比如 `x` 是一个四维数组，则 `x[i, ...]` 等价于 `x[i, :, :, :]` 
    