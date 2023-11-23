
+++
title = "NumPy基础扫盲"
summary = ''
description = ""
categories = []
tags = []
date = 2016-05-20T14:12:06+08:00
draft = false
+++

*520这天，作为一个单身程序狗，只能在学校敲敲代码。很早以前就听闻NumPy的强大。今天正好借此机会学习了一波，总结了这份快餐教程。*
## 数组对象 ndarray
### array()创建多维数组

    >>> arr = array([[1,2,3],[4,5,6],[7,8,9]])
    >>> print arr
    [[1 2 3]
     [4 5 6]
     [7 8 9]]

### arange()用于创建等差数组
`arange(start,end,step)`

    >>> arr = arange(0,10,2)
    >>> print arr
    [0 2 4 6 8]

### 数组维度

    # 2×3 矩阵
    >>> arr = array([arange(3),arange(3)])
    >>> arr.shape
    (2, 3)
    # 维的个数
    >>> arr.ndim
    2

### 数据类型
NumPy支持的数据类型有整型，浮点型和复数型，同时支持不同的精度。
#### 查看数据类型
    >>> arr = array([1+2j,3+4j,5+6j]) #arr.real 实部 arr.imag 虚部
    >>> arr.dtype
    dtype('complex128')
    #类型所占字节数
    >>> arr.dtype.itemsize
    16

#### 指定数据类型

    >>> arange(5,dtype=float32)
    array([ 0.,  1.,  2.,  3.,  4.], dtype=float32)

#### 创建自定义数据类型

    >>> t = dtype([('name',str,32),('grade',int)])
    >>> student = array([('weiss',90),('ruby',89)],dtype=t)
    >>> print student
    [('weiss', 90) ('ruby', 89)]
    >>> student.shape
    (2,)

#### 转换元素的类型

    >>> arr = arange(4)
    >>> arr.astype(float64)
    array([ 0.,  1.,  2.,  3.])
### 一次性取出多个元素

    >>> arr = arange(10,20,2)
    >>> take(arr,(1,3))
    array([12, 16])

### 多维数组的切片
*和python列表相似*

    # reshape() 改变数组维度  ravel()将数组展开成一维
    >>> arr = arange(24).reshape(2,3,4)
    >>> print arr
    [[[ 0  1  2  3]
      [ 4  5  6  7]
      [ 8  9 10 11]]

     [[12 13 14 15]
      [16 17 18 19]
      [20 21 22 23]]]
    >>> arr[0,0,0]
    0
    >>> arr[0,:,:]
    array([[ 0,  1,  2,  3],
           [ 4,  5,  6,  7],
           [ 8,  9, 10, 11]])
    >>> arr[0,...]
    array([[ 0,  1,  2,  3],
           [ 4,  5,  6,  7],
           [ 8,  9, 10, 11]])
    >>> arr[::-1,...]
    array([[[12, 13, 14, 15],
            [16, 17, 18, 19],
            [20, 21, 22, 23]],

           [[ 0,  1,  2,  3],
            [ 4,  5,  6,  7],
            [ 8,  9, 10, 11]]])
    >>> arr[0,::-1,-1]
    array([11,  7,  3])

### 矩阵转置

    >>> arr = array([[1,2],[3,4]])
    >>> arr.transpose()
    array([[1, 3],
           [2, 4]])

### 数组合并

    #水平组合
    >>> a = arange(9).reshape(3,3)
    >>> b = a * 2
    >>> hstack((a,b))
    array([[ 0,  1,  2,  0,  2,  4],
           [ 3,  4,  5,  6,  8, 10],
           [ 6,  7,  8, 12, 14, 16]])

    #垂直组合
    >>> vstack((a,b))
    array([[ 0,  1,  2],
           [ 3,  4,  5],
           [ 6,  7,  8],
           [ 0,  2,  4],
           [ 6,  8, 10],
           [12, 14, 16]])

    # 也可以通过concatenate函数来进行组合
    # NumPy中维度(dimensions)叫做轴(axis)，轴的个数叫做秩(rank)
    >>> concatenate((a,b),axis=0)
    array([[ 0,  1,  2],
           [ 3,  4,  5],
           [ 6,  7,  8],
           [ 0,  2,  4],
           [ 6,  8, 10],
           [12, 14, 16]])

    # 深度组合
    >>> dstack((a,b))
    array([[[ 0,  0],
            [ 1,  2],
            [ 2,  4]],

           [[ 3,  6],
            [ 4,  8],
            [ 5, 10]],

           [[ 6, 12],
            [ 7, 14],
            [ 8, 16]]])

### 数组分割

    >>> a = arange(9).reshape(3,3)
    >>> print a
    [[0 1 2]
     [3 4 5]
     [6 7 8]]

    # 水平分割
    >>> hsplit(a,3)
    [array([[0],
           [3],
           [6]]), array([[1],
           [4],
           [7]]), array([[2],
           [5],
           [8]])]

    # 垂直分割
    >>> vsplit(a,3)
    [array([[0, 1, 2]]), array([[3, 4, 5]]), array([[6, 7, 8]])]

    # 通过split 指定轴进行分割
    >>> split(a,3,axis=1)
    [array([[0],
           [3],
           [6]]), array([[1],
           [4],
           [7]]), array([[2],
           [5],
           [8]])]

    # 深度分割
    # 必须三个维度以上的数组，
    >>> a = arange(24).reshape(2,3,4)
    >>> dsplit(a,2)
    [array([[[ 0,  1],
            [ 4,  5],
            [ 8,  9]],

           [[12, 13],
            [16, 17],
            [20, 21]]]), array([[[ 2,  3],
            [ 6,  7],
            [10, 11]],

           [[14, 15],
            [18, 19],
            [22, 23]]])]

### flat属性
flat属性将返回一个flatiter对象，可以让我们像遍历一维数组一样遍历多维数组

    >>> a = arange(24).reshape(2,3,4)
    >>> print a.flat[10]
    10
    # 获取多个元素
    >>> a.flat[[1,2,3,4]]
    array([1, 2, 3, 4])

    # 对flat属性赋值将导致整个数组的元素都被覆盖
    >>> a.flat = 6
    >>> print a
    [[[6 6 6 6]
      [6 6 6 6]
      [6 6 6 6]]

     [[6 6 6 6]
      [6 6 6 6]
      [6 6 6 6]]]

    # 指定元素进行覆盖
    >>> a.flat[[2,4,6,8,10]] = 1
    >>> a
    array([[[6, 6, 1, 6],
            [1, 6, 1, 6],
            [1, 6, 1, 6]],

           [[6, 6, 6, 6],
            [6, 6, 6, 6],
            [6, 6, 6, 6]]])

### 数组转换列表

    >>> arr = arange(4).reshape(2,2)
    >>> arr.tolist()
    [[0, 1], [2, 3]]

### 创建单位矩阵

    >>> a = eye(2)
    >>> a
    array([[ 1.,  0.],
           [ 0.,  1.]])
    # dtype float64
    >>> a.dtype
    dtype('float64')

### 创建全零数组

    >>> arr = zeros(4).reshape(2,2)
    >>> arr
    array([[ 0.,  0.],
           [ 0.,  0.]])

### 保存数组

    >>> arr = arange(27).reshape(3,3,3)
    # 生成 .npy 文件
    >>> save('matrix',arr)
    >>> brr = load('matrix')
    >>> brr = load('matrix.npy')

## 文件读写

### 写入
    >>> a = eye(2)
    >>> savetxt('eye.txt',a)

### 读取csv文件

    #data.csv
    id,file,description,date,author,platform,type,port
    1,platforms/windows/remote/1.c,"Microsoft Windows WebDAV - (ntdll.dll) Remote Exploit",2003-03-23,kralor,windows,remote,80
    2,platforms/windows/remote/2.c,"Microsoft Windows WebDAV - Remote PoC Exploit",2003-03-24,RoMaNSoFt,windows,remote,80

    >>> f = file('data.csv')
    # 第一行不是数据，所以需要先打开数据文件，读取完第一行之后，再把文件对象传递给loadtxt()
    >>> f.readline()
    'id,file,description,date,author,platform,type,port\n'
    # delimiter指定文件分隔符，usecols指定返回的数据列，unpack指定是否将不同的列分开返回，dtype数据结构
    >>> id,author,platform = loadtxt(f,delimiter=',',usecols=(0,4,5),unpack=True,dtype=dtype([('id',int),('author',str,32),('platform',str,32)]))
    >>> print author
    ['kralor' 'RoMaNSoFt']

## 常用函数
如果使用`ipython`，可以使用 函数名? 的方式查看详细用法 如 `average?`

*  average()    # 加权平均数，也可求算术平均数
*  mean()   # 算术平均数
*  max()    # 最大值
*  min()
*  sum()
*  ptp()    # 极差
*  median() # 中位数
*  var()    # 方差
*  std()    # 标准差
*  diff()   # 返回相邻元素差值构成的数组
*  where()  # 返回满足条件的数组元素索引  where(arr>0)

