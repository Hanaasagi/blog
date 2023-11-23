
+++
title = "二叉树常用操作总结"
summary = ''
description = ""
categories = []
tags = []
date = 2016-10-12T01:11:32+08:00
draft = false
+++

我们将二叉树结点定义如下

	class TreeNode(object):
	    def __init__(self, x):
	        self.val = x
	        self.left = None
	        self.right = None

测试用例

	    3
	   / \
	  9  20
	    /  \
	   15   7
	    \  / \
	     8 2  12

为了方便调试，我们先搞一种方法，将列表转换为二叉树

[3, 9, 20, None, None, 15, 7, None, 8, 2, 12]

	def init_Tree(node_list):
	    root = TreeNode(node_list.pop(0))
	    stack = [root]
	    while len(node_list):
	        node = stack.pop(0)
	        val = node_list.pop(0)
	        if val is not None:
	            node.left = TreeNode(val)
	            stack.append(node.left)
	        val = node_list.pop(0)
	        if val is not None:
	            node.right = TreeNode(val)
	            stack.append(node.right)
	    return root

那为了检验我们生成二叉树的方法对不对，我们需要将二叉树打印出来

	def lookup(root):
	    queue = [root]
	    while queue:
	        current = queue.pop(0)
	        print current.val
	        if current.left:
	            queue.append(current.left)
	        if current.right:
	            queue.append(current.right)


#### 打印二叉树路径

	def binaryTree_paths(root):
	    res = []
	    if root is None:
	        return res
	    stack = [root]
	    path_stack = [str(root.val)]

	    while len(stack):
	        node = stack.pop()
	        path = path_stack.pop()
	        if node.left is None and node.right is None:
	            res.append(path)
	            continue
	        if node.left is not None:
	            stack.append(node.left)
	            path_stack.append('{0}->{1}'.format(path, node.left.val))
	        if node.right is not None:
	            stack.append(node.right)
	            path_stack.append('{0}->{1}'.format(path, node.right.val))
	    return res

#### 先序遍历

	def preorder_traversal(root):
	    res = []
	    def traversal(node):
	        if node is None:
	            return
	        res.append(node.val)
	        traversal(node.left)
	        traversal(node.right)
	    traversal(root)
	    return res

#### 中序遍历

	def inorder_traversal(root):
	    res = []
	    def traversal(node):
	        if node is None:
	            return
	        traversal(node.left)
	        res.append(node.val)
	        traversal(node.right)
	    traversal(root)
	    return res

#### 后序遍历

	def postorder_traversal(root):
	    res = []
	    def traversal(node):
	        if node is None:
	            return
	        traversal(node.left)
	        traversal(node.right)
	        res.append(node.val)
	    traversal(root)
	    return res

#### 二叉查找树

	def sortedArrayToBST(nums):
	    n = len(nums)
	    if not nums:
	        return None
	    mid = n / 2
	    root = TreeNode(nums[mid])
	    root.left = sortedArrayToBST(nums[:mid])
	    root.right = sortedArrayToBST(nums[mid + 1:])
	    return root

#### 最大树深

	def max_depth(root):
	    if not root:
	        return 0
	    return max(max_depth(root.left), max_depth(root.right)) + 1

#### 最小树深

	def min_depth(root):
	    if root is None:
	        return 0
	    elif root.left is None:
	        return min_depth(root.right) + 1
	    elif root.right is None:
	        return min_depth(root.left) + 1
	    return min(min_depth(root.left),min_depth(root.right)) + 1


#### 反转二叉树
~~想起了 Google 的梗~~

	def invert_tree(root):
	    if root is not None:
	        root.left, root.right = invert_tree(root.right), invert_tree(root.left)
	    return root

#### 判断树是否相同

	def isSameTree(p, q):
	    if p is None and q is None:
	        return True
	    elif p is None or q is None:
	        return False

	    if p.val == q.val:
	        return isSameTree(p.left, q.left) and isSameTree(p.right, q.right)
	    else:
	        return False


