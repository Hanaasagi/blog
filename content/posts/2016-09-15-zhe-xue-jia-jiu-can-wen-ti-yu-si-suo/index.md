
+++
title = "哲学家就餐问题与死锁"
summary = ''
description = ""
categories = []
tags = []
date = 2016-09-15T06:22:54+08:00
draft = false
+++

#### 问题描述
哲学家就餐问题（Dining philosophers problem）是在计算机科学中的一个经典问题，用来演示在并发计算中多线程同步（Synchronization）时产生的问题。
哲学家就餐问题可以这样表述：五位哲学家围绕一个圆桌就坐,桌上摆着五**支**筷子。哲学家的状态可能是“思考”或者“饥饿”。如果饥饿,哲学家将拿起他两边的筷子并进餐一段时间。进餐结束,哲学家就会放回筷子。哲学家从来不交谈，这就很危险，会产生每个哲学家都拿着左边的筷子，一直在等右边的筷子的情况。

可以用如下的代码表示这个问题:


	import threading
	import time
	import random
	import os
	import sys
	import signal


	class Watcher():

	    def __init__(self):
	        self.child = os.fork()
	        if self.child == 0:
	            return
	        else:
	            self.watch()

	    def watch(self):
	        try:
	            os.wait()
	        except KeyboardInterrupt:
	            self.kill()
	        sys.exit()

	    def kill(self):
	        try:
	            os.kill(self.child, signal.SIGKILL)
	        except OSError:
	            pass


	class Philosopher(threading.Thread):

	    def __init__(self, left, right):
	        threading.Thread.__init__(self)
	        self.left = left
	        self.right = right

	    def run(self):
	        while True:
	            # now thinking random time
	            time.sleep(random.random())
	            # now picking up chopsticks
	            with self.left:
	                with self.right:
	                    # now eating
	                    print '%s is eating %s' % (threading.currentThread().getName(), time.ctime())
	                    time.sleep(random.random())



	if __name__ == '__main__':
	    num = 5
	    chopsticks = [threading.Lock() for _ in range(num)]
	    philosophers = [Philosopher(chopsticks[(_ - 1) % num],
	                                chopsticks[_ % num])for _ in range(num)]
	    Watcher()
	    for p in philosophers:
	        p.start()


上面这段代码产生死锁的概率比较小，运行了两个小时还没出现(可见死锁问题调试时难以被发现)。为了让死锁更容易出现我们稍加修改,假设哲学家们拿起左面的筷子后突然忘记了自己要吃饭，沉思一会后，再拿起右面的筷子去吃饭。

	with self.left:
		time.sleep(random.random())
		with self.right:

Output

	Thread-4 is eating Tue Sep 13 13:15:20 2016
	Thread-3 is eating Tue Sep 13 13:15:21 2016
	...
	Thread-3 is eating Tue Sep 13 13:16:15 2016
	Thread-2 is eating Tue Sep 13 13:16:15 2016
	# block here


当一个线程想要使用多把锁时，就需要考虑死锁。
有一种方案是在获取第二把锁的时候设置超时时间，这种方案虽然避免了无尽的死锁,但这并不是一个足够好的方案。首先,这个方案并不能避免死锁——它只是提供了一种从死锁中恢复的手段。其次,这个方案会受到活锁现象的影响——如果所有死锁线程同时超时,它们极有可能再次陷入死锁。虽然死锁没有永远持续下去,但对资源的争夺状况却没有得到任何改善。
下面给出两种避免死锁的方法

#### 固定顺序获取
有一个简单的方法可以避开死锁——总是按照一个全局的固定的顺序获取多把锁。

	class Philosopher(threading.Thread):

	    def __init__(self, left, right):
	        threading.Thread.__init__(self)
	        if id(left) < id(right):
	            self.first = left
	            self.second = right
	        else:
	            self.first = right
	            self.second = left

	    def run(self):
	        while True:
	            # now thinking random time
	            time.sleep(random.random())
	            # now picking up chopsticks
	            with self.first:
	                time.sleep(random.random())
	                with self.second:
	                    # now eating
	                    print '%s is eating %s' % (threading.currentThread().getName(), time.ctime())
	                    time.sleep(random.random())


#### 条件变量


	class Philosopher(threading.Thread):

	    def __init__(self, con):
	        threading.Thread.__init__(self)
	        self.is_eating = False
	        self.con = con

	    def set_around(self, left, right):
	        self.left = left
	        self.right = right

	    def thinking(self):
	        with self.con:
	            self.is_eating = False
	            self.left.con.notify()
	            self.right.con.notify()
	            time.sleep(random.random())

	    def eating(self):
	        with self.con:
	            while self.left.is_eating or self.right.is_eating:
	                self.con.wait()
	            self.is_eating = True
	            print '%s is eating %s' % (threading.currentThread().getName(), time.ctime())
	            time.sleep(random.random())

	    def run(self):
	        while True:
	            self.thinking()
	            self.eating()


	if __name__ == '__main__':
	    num = 5
	    con = threading.Condition()
	    philosophers = [Philosopher(con) for _ in range(num)]
	    for i in range(num):
	        philosophers[i].set_around(
	            philosophers[(i - 1) % num], philosophers[i % num])
	    Watcher()
	    for p in philosophers:
	        p.start()

与之前不同,我们将竞争从对筷子的竞争转换成了对状态的判断:仅当哲学家的左右都没有进餐时,他才可以进餐。

