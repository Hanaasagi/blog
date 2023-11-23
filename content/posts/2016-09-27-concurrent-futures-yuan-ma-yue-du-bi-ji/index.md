
+++
title = "concurrent.futures 源码阅读笔记"
summary = ''
description = ""
categories = []
tags = []
date = 2016-09-27T08:29:30+08:00
draft = false
+++

#### 前言
concurrent.futures 是一个异步库， 详细的介绍请看[这里](http://pythonhosted.org/futures/)
本篇文章将从源码来分析这个包是如何实现异步的。
concurrent.futures 主要包括三个文件：_base.py、thread.py 和 process.py

#### 源码分析
*源码中的注释挺详细的, 基本上通读一遍就能理解差不多*

##### _base.py

首先定义了这样一些全局变量，先有个印象好了

	FIRST_COMPLETED = 'FIRST_COMPLETED'
	FIRST_EXCEPTION = 'FIRST_EXCEPTION'
	ALL_COMPLETED = 'ALL_COMPLETED'
	_AS_COMPLETED = '_AS_COMPLETED'

	# Possible future states (for internal use by the futures package).
	PENDING = 'PENDING'
	RUNNING = 'RUNNING'
	# The future was cancelled by the user...
	CANCELLED = 'CANCELLED'
	# ...and _Waiter.add_cancelled() was called by a worker.
	CANCELLED_AND_NOTIFIED = 'CANCELLED_AND_NOTIFIED'
	FINISHED = 'FINISHED'

	_FUTURE_STATES = [
	    PENDING,
	    RUNNING,
	    CANCELLED,
	    CANCELLED_AND_NOTIFIED,
	    FINISHED
	]

	_STATE_TO_DESCRIPTION_MAP = {
	    PENDING: "pending",
	    RUNNING: "running",
	    CANCELLED: "cancelled",
	    CANCELLED_AND_NOTIFIED: "cancelled",
	    FINISHED: "finished"
	}


##### class Future

我们先来看看 future 类，我理解的 future 就是用来表示一个异步计算的结果。当调用后 future 会被立即返回，但是不一定就是最终结果。最终的结果此时还在计算中。

先来看看 future 类的__init__()方法

    def __init__(self):
        """Initializes the future. Should not be called by clients."""
        self._condition = threading.Condition()
        self._state = PENDING
        self._result = None
        self._exception = None
        self._traceback = None
        self._waiters = []
        self._done_callbacks = []

`_invoke_callbacks()` 调用回调函数

    def _invoke_callbacks(self):
        for callback in self._done_callbacks:
            try:
                callback(self)
            except Exception:
                LOGGER.exception('exception calling callback for %r', self)

`cancelled()` `running()` `done()` 等方法是用来判断状态的。结构相同，这里就不展开了

    def cancelled(self):
        """Return True if the future has cancelled."""
        with self._condition:
            return self._state in [CANCELLED, CANCELLED_AND_NOTIFIED]


`__get_result()`用来获取结果

    def __get_result(self):
        if self._exception:
            raise type(self._exception), self._exception, self._traceback
        else:
            return self._result

`add_done_callback()` 用来添加回调函数，如果状态为 CANCELLED, CANCELLED_AND_NOTIFIED, FINISHED 三者之一， 则会立即执行回调函数

    def add_done_callback(self, fn):
        """Attaches a callable that will be called when the future finishes.

        Args:
            fn: A callable that will be called with this future as its only
                argument when the future completes or is cancelled. The callable
                will always be called by a thread in the same process in which
                it was added. If the future has already completed or been
                cancelled then the callable will be called immediately. These
                callables are called in the order that they were added.
        """
        with self._condition:
            if self._state not in [CANCELLED, CANCELLED_AND_NOTIFIED, FINISHED]:
                self._done_callbacks.append(fn)
                return
        fn(self)

`result()` 通过调用`__get_result()`获取返回结果，如果不能马上得到结果会等待 timeout 指定的时间再次获取。如果 future 已经被取消则会抛出异常

    def result(self, timeout=None):
        """Return the result of the call that the future represents.

        Args:
            timeout: The number of seconds to wait for the result if the future
                isn't done. If None, then there is no limit on the wait time.

        Returns:
            The result of the call that the future represents.

        Raises:
            CancelledError: If the future was cancelled.
            TimeoutError: If the future didn't finish executing before the given
                timeout.
            Exception: If the call raised then that exception will be raised.
        """
        with self._condition:
            if self._state in [CANCELLED, CANCELLED_AND_NOTIFIED]:
                raise CancelledError()
            elif self._state == FINISHED:
                return self.__get_result()

            self._condition.wait(timeout)

            if self._state in [CANCELLED, CANCELLED_AND_NOTIFIED]:
                raise CancelledError()
            elif self._state == FINISHED:
                return self.__get_result()
            else:
                raise TimeoutError()

`exception_info()` 和 `result()` 结构相似，返回 self._exception, self._traceback

Future 类剩余的几个 set 开头的方法(`set_result()`,`set_exception_info` ... ) 只能在 Executor 的实例中被调用，用来修改 Future 实例变量的值

##### class _Waiter

	class _Waiter(object):
	    """Provides the event that wait() and as_completed() block on."""
	    def __init__(self):
	        self.event = threading.Event()
	        self.finished_futures = []

	    def add_result(self, future):
	        self.finished_futures.append(future)

	    def add_exception(self, future):
	        self.finished_futures.append(future)

	    def add_cancelled(self, future):
	        self.finished_futures.append(future)


_AsCompletedWaiter, _FirstCompletedWaiter, _AllCompletedWaiter三个类是继承自 _Waiter 的， 三者的区别在于何时执行 event.set()

##### class Executor

`Executor` (执行器) 是一个虚基类，而且实现了 `__enter__` 和 `__exit__` 是一个上下文管理器

`map()`返回了一个生成器

    def map(self, fn, *iterables, **kwargs):
        """Returns a iterator equivalent to map(fn, iter).

        Args:
            fn: A callable that will take as many arguments as there are
                passed iterables.
            timeout: The maximum number of seconds to wait. If None, then there
                is no limit on the wait time.

        Returns:
            An iterator equivalent to: map(func, *iterables) but the calls may
            be evaluated out-of-order.

        Raises:
            TimeoutError: If the entire result iterator could not be generated
                before the given timeout.
            Exception: If fn(*args) raises for any values.
        """
        timeout = kwargs.get('timeout')
        if timeout is not None:
            end_time = timeout + time.time()

        fs = [self.submit(fn, *args) for args in itertools.izip(*iterables)]

        # Yield must be hidden in closure so that the futures are submitted
        # before the first iterator value is required.
        def result_iterator():
            try:
                for future in fs:
                    if timeout is None:
                        yield future.result()
                    else:
                        yield future.result(end_time - time.time())
            finally:
                for future in fs:
                    future.cancel()
        return result_iterator()



##### _base.py 中其他的函数

`_create_and_install_waiters` 的参数 fs 是一个 future 组成的列表。调用此函数会根据 return_when 的类型来创建 waiter 并添加到每一个 future 内部

	def _create_and_install_waiters(fs, return_when):
	    if return_when == _AS_COMPLETED:
	        waiter = _AsCompletedWaiter()
	    elif return_when == FIRST_COMPLETED:
	        waiter = _FirstCompletedWaiter()
	    else:
	        pending_count = sum(
	                f._state not in [CANCELLED_AND_NOTIFIED, FINISHED] for f in fs)

	        if return_when == FIRST_EXCEPTION:
	            waiter = _AllCompletedWaiter(pending_count, stop_on_exception=True)
	        elif return_when == ALL_COMPLETED:
	            waiter = _AllCompletedWaiter(pending_count, stop_on_exception=False)
	        else:
	            raise ValueError("Invalid return condition: %r" % return_when)

	    for f in fs:
	        f._waiters.append(waiter)

	    return waiter

`as_completed()` 是一个生成器，会先 yield 已经完成的 future。 如果未完成， 则会通过 while 循环来等待 future 完成再 yield

	def as_completed(fs, timeout=None):
	    """An iterator over the given futures that yields each as it completes.

	    Args:
	        fs: The sequence of Futures (possibly created by different Executors) to
	            iterate over.
	        timeout: The maximum number of seconds to wait. If None, then there
	            is no limit on the wait time.

	    Returns:
	        An iterator that yields the given Futures as they complete (finished or
	        cancelled). If any given Futures are duplicated, they will be returned
	        once.

	    Raises:
	        TimeoutError: If the entire result iterator could not be generated
	            before the given timeout.
	    """
	    if timeout is not None:
	        end_time = timeout + time.time()

	    fs = set(fs)
	    with _AcquireFutures(fs):
	        finished = set(
	                f for f in fs
	                if f._state in [CANCELLED_AND_NOTIFIED, FINISHED])
	        pending = fs - finished
	        waiter = _create_and_install_waiters(fs, _AS_COMPLETED)

	    try:
	        for future in finished:
	            yield future

	        while pending:
	            if timeout is None:
	                wait_timeout = None
	            else:
	                wait_timeout = end_time - time.time()
	                if wait_timeout < 0:
	                    raise TimeoutError(
	                            '%d (of %d) futures unfinished' % (
	                            len(pending), len(fs)))

	            waiter.event.wait(wait_timeout)

	            with waiter.lock:
	                finished = waiter.finished_futures
	                waiter.finished_futures = []
	                waiter.event.clear()

	            for future in finished:
	                yield future
	                pending.remove(future)

	    finally:
	        for f in fs:
	            with f._condition:
	                f._waiters.remove(waiter)

`_AcquireFutures` 类是一个上下文管理器，实现了对每一个 future 中的条件锁进行获取

	class _AcquireFutures(object):
	    """A context manager that does an ordered acquire of Future conditions."""

	    def __init__(self, futures):
	        self.futures = sorted(futures, key=id)

	    def __enter__(self):
	        for future in self.futures:
	            future._condition.acquire()

	    def __exit__(self, *args):
	        for future in self.futures:
	            future._condition.release()

`wait()` 会返回当前完成(finished or canceld)和未完成的 future

	DoneAndNotDoneFutures = collections.namedtuple(
	        'DoneAndNotDoneFutures', 'done not_done')
	def wait(fs, timeout=None, return_when=ALL_COMPLETED):
	    """Wait for the futures in the given sequence to complete.

	    Args:
	        fs: The sequence of Futures (possibly created by different Executors) to
	            wait upon.
	        timeout: The maximum number of seconds to wait. If None, then there
	            is no limit on the wait time.
	        return_when: Indicates when this function should return. The options
	            are:

	            FIRST_COMPLETED - Return when any future finishes or is
	                              cancelled.
	            FIRST_EXCEPTION - Return when any future finishes by raising an
	                              exception. If no future raises an exception
	                              then it is equivalent to ALL_COMPLETED.
	            ALL_COMPLETED -   Return when all futures finish or are cancelled.

	    Returns:
	        A named 2-tuple of sets. The first set, named 'done', contains the
	        futures that completed (is finished or cancelled) before the wait
	        completed. The second set, named 'not_done', contains uncompleted
	        futures.
	    """
	    with _AcquireFutures(fs):
	        done = set(f for f in fs
	                   if f._state in [CANCELLED_AND_NOTIFIED, FINISHED])
	        not_done = set(fs) - done

	        if (return_when == FIRST_COMPLETED) and done:
	            return DoneAndNotDoneFutures(done, not_done)
	        elif (return_when == FIRST_EXCEPTION) and done:
	            if any(f for f in done
	                   if not f.cancelled() and f.exception() is not None):
	                return DoneAndNotDoneFutures(done, not_done)

	        if len(done) == len(fs):
	            return DoneAndNotDoneFutures(done, not_done)

	        waiter = _create_and_install_waiters(fs, return_when)

	    waiter.event.wait(timeout)
	    for f in fs:
	        with f._condition:
	            f._waiters.remove(waiter)

	    done.update(waiter.finished_futures)
	    return DoneAndNotDoneFutures(done, set(fs) - done)

##### thread.py

先来看看导入了什么

	import atexit
	from concurrent.futures import _base
	import Queue as queue
	import threading
	import weakref
	import sys

atexit模块很简单，只定义了一个register函数用于注册程序退出时的回调函数，我们可以在这个回调函数中做一些资源清理的操作。
weakref 产生一个对象的弱引用。相对于通常的引用来说，如果一个对象有一个常规的引用，它是不会被垃圾收集器销毁的，但是如果一个对象只剩下一个弱引用，那么它可能被垃圾收集器收回。

`_python_exit()` q.put(None)的作用类似 POISON_PILL

	def _python_exit():
	    global _shutdown
	    _shutdown = True
	    items = list(_threads_queues.items()) if _threads_queues else ()
	    for t, q in items:
	        q.put(None)
	    for t, q in items:
	        t.join(sys.maxint)

`atexit.register(_python_exit)` 注册程序退出时的回调函数

`_WorkItem` 类负责进行计算并向 future 中添加结果

	class _WorkItem(object):
	    def __init__(self, future, fn, args, kwargs):
	        self.future = future
	        self.fn = fn
	        self.args = args
	        self.kwargs = kwargs

	    def run(self):
	        if not self.future.set_running_or_notify_cancel():
	            return

	        try:
	            result = self.fn(*self.args, **self.kwargs)
	        except BaseException:
	            e, tb = sys.exc_info()[1:]
	            self.future.set_exception_info(e, tb)
	        else:
	            self.future.set_result(result)


`_worker()` 会一直从队列中取出`_WorkItem`对象，直到取出 None

	def _worker(executor_reference, work_queue):
	    try:
	        while True:
	            work_item = work_queue.get(block=True)
	            if work_item is not None:
	                work_item.run()
	                # Delete references to object. See issue16284
	                del work_item
	                continue
	            executor = executor_reference()
	            # Exit if:
	            #   - The interpreter is shutting down OR
	            #   - The executor that owns the worker has been collected OR
	            #   - The executor that owns the worker has been shutdown.
	            if _shutdown or executor is None or executor._shutdown:
	                # Notice other workers
	                work_queue.put(None)
	                return
	            del executor
	    except BaseException:
	        _base.LOGGER.critical('Exception in worker', exc_info=True)

`ThreadPoolExecutor` 类是一个执行器的线程池，继承自 `Excutor` 虚基类


    def __init__(self, max_workers):
        """Initializes a new ThreadPoolExecutor instance.

        Args:
            max_workers: The maximum number of threads that can be used to
                execute the given calls.
        """
        self._max_workers = max_workers
        self._work_queue = queue.Queue()
        self._threads = set()
        self._shutdown = False
        self._shutdown_lock = threading.Lock()

`submit()` 生成 future 对象，进一步生成 _WorkItem 对象，并添加到工作队列中。

    def submit(self, fn, *args, **kwargs):
        with self._shutdown_lock:
            if self._shutdown:
                raise RuntimeError('cannot schedule new futures after shutdown')

            f = _base.Future()
            w = _WorkItem(f, fn, args, kwargs)

            self._work_queue.put(w)
            self._adjust_thread_count()
            return f

`_adjust_thread_count()` 如果没有达到最大线程数目则会添加一个线程(守护线程)，并在 _threads_queues 添加 以线程对象为 key ，self._work_queue 为值的键值对。

    def _adjust_thread_count(self):
        # When the executor gets lost, the weakref callback will wake up
        # the worker threads.
        def weakref_cb(_, q=self._work_queue):
            q.put(None)
        # TODO(bquinlan): Should avoid creating new threads if there are more
        # idle threads than items in the work queue.
        if len(self._threads) < self._max_workers:
            t = threading.Thread(target=_worker,
                                 args=(weakref.ref(self, weakref_cb),
                                       self._work_queue))
            t.daemon = True
            t.start()
            self._threads.add(t)
            _threads_queues[t] = self._work_queue

`shutdown()` 进行结束工作

    def shutdown(self, wait=True):
        with self._shutdown_lock:
            self._shutdown = True
            self._work_queue.put(None)
        if wait:
            for t in self._threads:
                t.join(sys.maxint)


看到这里，你还是一头雾水对不对。

#### 栗子

下面看一个 demo

	import urllib
	from concurrent.futures import ThreadPoolExecutor

	URLS = ['http://www.github.com/',
	        'http://stackoverflow.com/',
	        'http://dreamfever.ml/']


	def load_url(url):
	    print 'load url {}'.format(url)
	    return url, urllib.urlopen(url).code


	def with_thread_pool_executor():
	    with ThreadPoolExecutor(10) as executor:
	        return executor.map(load_url, URLS)

	if __name__ == '__main__':
	    for i in with_thread_pool_executor():
	        print '{:<32} status code:{}'.format(*i)

Output

	load url http://www.github.com/
	load url http://stackoverflow.com/
	load url http://dreamfever.ml/
	http://www.github.com/           status code:200
	http://stackoverflow.com/        status code:200
	http://dreamfever.ml/            status code:200

ThreadPoolExecutor 是一个上下文管理器

`__enter__()` 和 `__exit__()` 是继承自父类的方法，用来实现上下文管理器

*_base.py文件中的Executor类*

	def __enter__(self):
	    return self

我们可以看到 `__enter__` 方法返回了 self
之后我们调用了 `executor.map(load_url, URLS)`

*_base.py文件中的Executor类map方法*

首先我们计算出 end_time

	timeout = kwargs.get('timeout')
	if timeout is not None:
	    end_time = timeout + time.time()

对每一个参数调用一次 self.submit()

	fs = [self.submit(fn, *args) for args in itertools.izip(*iterables)]

`submit()` 方法的具体实现在 thread.py

*thread.py文件中的ThreadPoolExceutor类submit方法*

	def submit(self, fn, *args, **kwargs):
	    with self._shutdown_lock:
	        if self._shutdown:
	            raise RuntimeError('cannot schedule new futures after shutdown')

	        f = _base.Future()
	        w = _WorkItem(f, fn, args, kwargs)

	        self._work_queue.put(w)
	        self._adjust_thread_count()
	        return f

创建出一个 _WorkItem 对象，并添加到工作队列中，调用 `self._adjust_thread_count()` 开启一个线程去执行。执行 `self.submit()` 时并没有阻塞，future 对象是立即返回的。fs是一个包含所有 future 对象的列表。

*_base.py文件中的Executor类map方法*

	def result_iterator():
	    try:
	        for future in fs:
	            if timeout is None:
	                yield future.result()
	            else:
	                yield future.result(end_time - time.time())
	    finally:
	        for future in fs:
	            future.cancel()
	return result_iterator()

`result_iterator()` 是 `map()` 方法中的一个内部函数， 它实际上是一个生成器，yield 出 future.result()

*_base.py文件中的Future类result方法*

	with self._condition:
	    if self._state in [CANCELLED, CANCELLED_AND_NOTIFIED]:
	        raise CancelledError()
	    elif self._state == FINISHED:
	        return self.__get_result()

	    self._condition.wait(timeout)

	    if self._state in [CANCELLED, CANCELLED_AND_NOTIFIED]:
	        raise CancelledError()
	    elif self._state == FINISHED:
	        return self.__get_result()
	    else:
	        raise TimeoutError()

*由于本人技术水平有限，难免有所疏漏，欢迎指正*

