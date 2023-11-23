
+++
title = "Implement LRU/LFU Cache"
summary = ''
description = ""
categories = []
tags = []
date = 2017-06-23T11:27:45+08:00
draft = false
+++

常见的缓存算法有 FIFO LFU LRU

- FIFO: 淘汰最先进入缓存
- LRU:  淘汰最久未被访问的
- LFU:  淘汰访问次数最少的

实现一个 LRU Cache 可以通过 HashMap 和 链表来实现。HashMap 负责保证 O(1) 的读取，链表负责按访问时间排序

```Python
class LRUCache:
    """ LRUCache implemented with HashMap and LinkList
    >>> cache = LRUCache(3)
    >>> cache.set(1, 1)
    >>> cache.set(2, 2)
    >>> cache.set(3, 3)
    >>> cache
    capacity: 3 [(1, 1), (2, 2), (3, 3)]
    >>> cache.get(1)
    1
    >>> cache
    capacity: 3 [(2, 2), (3, 3), (1, 1)]
    >>> cache.set(4, 4)
    >>> cache
    capacity: 3 [(3, 3), (1, 1), (4, 4)]
    """

    class Node:

        def __init__(self, key, value):
            self.key = key
            self.value = value
            self.pre = None
            self.next = None

    def __init__(self, capacity):
        self.capacity = capacity
        self._head = self.Node(None, None)
        self._tail = self.Node(None, None)
        self._head.next = self._tail
        self._tail.pre = self._head
        self._map = {}

    def get(self, key):
        n = self._map.get(key, None)
        if n is not None:
            n.pre.next = n.next
            n.next.pre = n.pre
            self._append_tail(n)
            return n.value
        raise KeyError(key)

    def set(self, key, value):
        n = self._map.get(key, None)
        # n already in the map
        if n is not None:
            n.value = value
            self._map[key] = n
            n.pre.next = n.next
            n.next.pre = n.pre
            self._append_tail(n)
            return

        # n not in the map
        # check the capacity
        if len(self._map) == self.capacity:
            tmp = self._head.next
            self._head.next = self._head.next.next
            self._head.next.pre = self._head
            self._map.pop(tmp.key)
        n = self.Node(key, value)
        self._append_tail(n)
        self._map[key] = n

    def _append_tail(self, n):
        n.next = self._tail
        n.pre = self._tail.pre
        self._tail.pre.next = n
        self._tail.pre = n

    def __repr__(self):
        tmp = self._head.next
        result = []
        while tmp is not self._tail:
            result.append((tmp.key, tmp.value))
            tmp = tmp.next
        return 'capacity: {} {}'.format(self.capacity, result.__repr__())


if __name__ == '__main__':
    import doctest
    doctest.testmod(verbose=False)
```

写完后才发现好像不必这么麻烦，直接用 `OrderedDict` 更简单。其本身有序，而且还提供了 `move_to_end` 方法

```Python
from collections import OrderedDict
class LRUCache:

    def __init__(self, capacity):
        self.capacity = capacity
        self._cache = OrderedDict()

    def get(self, key):
        value = self._cache.get(key, None)
        if value is not None:
            self._cache.move_to_end(key)
            return value
        raise KeyError(key)

    def set(self, key, value):
        if key in self._cache:
            self._cache.pop(key)
        elif len(self._cache) == self.capacity:
            # pop the beginning item
            self._cache.popitem(last=False)
        self._cache[key] = value
```

LFU Cache 涉及到按访问次数排序，这里借助了最小堆

```Python
import heapq


class LFUCache:
    """
    >>> cache = LFUCache(3)
    >>> cache.set(1,1)
    >>> cache.set(2,2)
    >>> cache.set(3,3)
    >>> cache
    capacity: 3 [(0, 0, {1: 1}), (0, 1, {2: 2}), (0, 2, {3: 3})]
    >>> cache.get(2)
    2
    >>> cache.get(1)
    1
    >>> cache
    capacity: 3 [(0, 2, {3: 3}), (1, 1, {2: 2}), (1, 0, {1: 1})]
    >>> cache.set(4, 4)
    >>> cache
    capacity: 3 [(0, 3, {4: 4}), (1, 1, {2: 2}), (1, 0, {1: 1})]
    >>> cache.set(5, 5)
    >>> cache
    capacity: 3 [(0, 4, {5: 5}), (1, 1, {2: 2}), (1, 0, {1: 1})]
    >>> cache.set(5, 55)
    >>> cache.set(6, 6)
    >>> cache
    capacity: 3 [(0, 5, {6: 6}), (1, 4, {5: 55}), (1, 1, {2: 2})]
    """
    class Bucket:

        def __init__(self, key, value, index):
            self.key = key
            self.value = value
            self.count = 0  # access count
            self.index = index  # when there are same count, sort by index

    def __init__(self, capacity):
        self.capacity = capacity
        self._map = {}
        self._heap = []
        self._index = 0  # unique

    def get(self, key):
        n = self._map.get(key, None)
        if n is not None:
            self._heap.remove((n.count, n.index, n))
            heapq.heapify(self._heap)
            n.count += 1
            heapq.heappush(self._heap, (n.count , n.index, n))
            return n.value
        raise KeyError(key)

    def set(self, key, value):
        n = self._map.get(key, None)
        # already in
        if n is not None:
            self._heap.remove((n.count, n.index, n))
            heapq.heapify(self._heap)
            n.count += 1
            n.value = value
            self._map[key] = n
            heapq.heappush(self._heap, (n.count, n.index, n))
            return

        if len(self._map) == self.capacity:
            *_, n = heapq.heappop(self._heap)
            self._map.pop(n.key)
        # new value
        n = self.Bucket(key, value, self._index)
        self._map[key] = n
        heapq.heappush(self._heap, (n.count , n.index, n))
        self._index += 1

    def __repr__(self):
        result = [(count, index, {item.key: item.value})
                        for count, index, item in self._heap]
        return 'capacity: {} {}'.format(self.capacity, result.__repr__())


if __name__ == '__main__':
    import doctest
    doctest.testmod(verbose=False)
```

在寻求解决方法时，Google 到了 LeetCode 中也有类似的题目，于是翻看 在线疯狂 大佬的 LeetCode 解题报告。它对于 LFU 有着不同的解决方法，附上链接  
[LRU Cache](http://bookshadow.com/weblog/2015/01/08/leetcode-lru-cache/)  
[LFU Cache](http://bookshadow.com/weblog/2016/11/22/leetcode-lfu-cache/)

    