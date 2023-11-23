
+++
title = "Young tableau"
summary = ''
description = ""
categories = []
tags = []
date = 2016-06-06T12:38:28+08:00
draft = false
+++

### Definition
已知一个2维矩阵，其中的元素每一行从左至右依次增加，每一列从上到下依次增加。即对于矩阵Table有Table[i][j] ≤Table[i][j + 1], Table[i][j] ≤ Table[i + 1][j]，我们也称这样的矩阵为杨氏矩阵。

### Insert
因为每个元素的下面一个元素和右面一个元素都会比当前元素大，所以右下角的元素是最大的一个元素。所以我们将元素放到矩阵的右下角，然后再来调整元素的位置。我们将这个元素与它上面的元素和左面的元素进行比较，将最大的那个元素与这个元素进行交换，如果这个元素就是最大的话，则已经插入正确的位置。若上面和左面的两个元素相等且大于这个元素，则我们可以交换哪一个都可以。

### Delete
我们要把矩阵中指定的一个元素去掉，那么我们需要调整它右面和下面的元素来符合杨氏矩阵的特性。所以我们不妨将要删除的元素置为NAN。我们将这个元素与他右面和下面的元素中最小的那个进行交换(类似insert操作)

### Find
我们从右上角开始来查找，目的元素比当前元素大则向下查，比当前元素小则向左查。这样们可以在2*n的次数内找到想要的元素。

### Modify
我们将这个重新赋值的元素和它四周的元素进行比较，通过交换调整位置来满足杨氏矩阵的特性。

    #-*-coding:utf-8-*-
    from numpy import *

    def insert(m, value, i, j):
    	m[i][j] = value
    	largesti = i
    	largestj = j

    	if i-1>=0 and (isnan(m[i-1][j]) or m[i-1][j] > m[i][j]):
    		largesti = i-1
    		largestj = j
    	if j-1>=0 and (isnan(m[i][j-1]) or m[i][j-1] > m[largesti][largestj]):
    		largesti = i
    		largestj = j - 1
    	if i!=largesti or j!=largestj:
    		temp = m[i][j]
    		m[i][j] = m[largesti][largestj]
    		m[largesti][largestj] = m[i][j]
    		insert(m,value,largesti,largestj)

    def delete(m, i, j):
    	m[i][j] = NAN
    	mini = i
    	minj = j
    	if i+1<m.shape[0]:
    		mini = i+1
    		minj = j
    	if j+1<m.shape[1] and (isnan(m[mini][minj]) or m[i][j+1] < m[mini][minj]):
    		mini = i
    		minj = j+1

    	if mini!=i or minj!=j:
    		temp = m[i][j]
    		m[i][j] = m[mini][minj]
    		m[mini][minj] = temp
    		delete(m, mini, minj)

    def find(m,value):
    	i = 0
    	j = m.shape[1] - 1
    	while i<m.shape[0] and j >=0:
    		if isnan(m[i][j]) or m[i][j] > value:
    			j = j -1
    		elif isnan(value) or m[i][j] < value:
    			i = i+ 1
    		else:
    			return True
    	return False

    def modify(m, i, j, value):
    	m[i][j] = value
    	nexti = i
    	nextj = j
    	if i-1>=0 and m[i-1][j] > m[i][j]:
    		nexti = i-1
    		nextj = j
    	if j-1>=0 and m[i][j-1] > m[nexti][nextj]:
    		nexti = i
    		nextj = j-1

    	if i+1<m.shape[0] and m[i][j] > m[i+1][j]:
    		nexti = i+1
    		nextj = j
    	if j+1<m.shape[1] and( isnan(m[nexti][nextj]) or m[i][j+1] < m[nexti][nextj]):
    		nexti = i
    		nextj = j+1

    	if nexti!=i or nextj!=j:
    		temp = m[i][j]
    		m[i][j] = m[nexti][nextj]
    		m[nexti][nextj] = temp
    		modify(m, nexti, nextj, value)

    if __name__ == '__main__':
    	m = array([[2,4,6,NAN],[3,7,10,NAN],[5,12,NAN,NAN],[8,NAN,NAN,NAN]])
    	h,v = m.shape
    	print 'matrix'
    	print m
    	print '-'*24
    	print 'after insert 7'
    	insert(m,7,h-1,v-1)
    	print m
    	print '-'*24
    	print 'after delete m[0][0]'
    	delete(m,0,0)
    	print m
    	print '-'*24
    	print 'if 12 in the matrix'
    	print find(m,12)
    	print '-'*24
    	print 'after update m[1][1] to 1'
    	modify(m, 1, 1,1)
    	print m

脚本输出

    matrix
    [[  2.   4.   6.  nan]
     [  3.   7.  10.  nan]
     [  5.  12.  nan  nan]
     [  8.  nan  nan  nan]]
    ------------------------
    after insert 7
    [[  2.   4.   6.  nan]
     [  3.   7.  10.  nan]
     [  5.   7.  nan  nan]
     [  8.  12.  nan  nan]]
    ------------------------
    after delete m[0][0]
    [[  3.   4.   6.  nan]
     [  5.   7.  10.  nan]
     [  7.  12.  nan  nan]
     [  8.  nan  nan  nan]]
    ------------------------
    if 12 in the matrix
    True
    ------------------------
    after update m[1][1] to 1
    [[  1.   4.   6.  nan]
     [  3.   5.  10.  nan]
     [  7.  12.  nan  nan]
     [  8.  nan  nan  nan]]

