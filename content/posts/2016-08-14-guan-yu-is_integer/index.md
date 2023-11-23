
+++
title = "关于is_integer()"
summary = ''
description = ""
categories = []
tags = []
date = 2016-08-14T07:40:35+08:00
draft = false
+++

python float对象中的is_integer()可以判断一个浮点数是否是一个整数  

    In [4]: (3.0).is_integer()
    Out[4]: True  

但是，今天写代码时遇到了这样一个bug

    n = 243
    temp1 = math.log10(n) / math.log10(3)
    print temp1, temp1.is_integer()
    temp2 = math.log(n, 3)
    print temp2, temp2.is_integer()

    # output  
    # 5.0 True
    # 5.0 False

WTF，都是5.0但是结果不同QAQ。  
经过查询，发现 

    print temp1.__repr__()
    print temp2.__repr__()
    # output
    # 5.0
    # 4.999999999999999

    