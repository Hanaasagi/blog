
+++
title = "GIL 究竟是什么"
summary = ''
description = ""
categories = []
tags = []
date = 2018-03-15T11:56:23+08:00
draft = false
+++

*本文代码取自 CPython 3.6.3， commit sha 为 2c5fed86e0cbba5a4e34792b0083128ce659909d*

本篇文章分析 Python 的多线程实现，侧重于 GIL 方面。由于本人 C 水平及 Python 水平有限，理解可能会有偏差，如果发现希望能够邮件告知

### Abstract

先来看 GIL 的定义

>In CPython, the global interpreter lock, or GIL, is a mutex that protects access to Python objects, preventing multiple threads from executing Python bytecodes at once. This lock is necessary mainly because CPython's memory management is not thread-safe. (However, since the GIL exists, other features have grown to depend on the guarantees that it enforces.)

摘自：[GlobalInterpreterLock - Python Wiki](https://wiki.python.org/moin/GlobalInterpreterLock)

**为什么需要 GIL**

>考虑这样的情形：假设有两个线程A、B，在两个线程中，都同时保存着对内存中同一对象obj的引用，也就是说，这时obj->ob_refcnt的值为2。如果A销毁对obj的引用，显然，A将通过Py_DECREF调整obj的引用计数值。我们知道，Py_DECREF的整个动作可以分为两个部分：  
□   --obj->ob_refcnt；  
□ 	if(obj->ob_refcnt == 0) destory object and free memory。  
如果A在执行完第一个动作之后，obj->ob_refcnt的值变为1。不幸的是，恰恰在这个时候，线程调度机制将A挂起，而唤醒了B。更为不幸的是，B同样也开始销毁对obj的引用。B完成第一个动作之后，obj->ob_refcnt为0，B是一个幸运儿，它没有被线程调度打断，而是顺利地完成了接下来的第二个动作，将对象销毁，内存释放。好了，现在A又被重新唤醒，可惜现在已经物是人非，obj->ob_refcnt已经被B减少到0，而不是当初的1。按照约定，傻乎乎的A开始再一次地对已经销毁的对象进行对象销毁和内存释放的动作。

摘自：《Python源码剖析》 — 陈儒  
在豆瓣阅读书店查看：https://read.douban.com/ebook/1499455/  
本作品由电子工业出版社授权豆瓣阅读全球范围内电子版制作与发行。  
© 版权所有，侵权必究。

所以我们需要加锁来实现不同线程对共享资源互斥访问。对于非共享资源，我们不需要加锁保护。但若进行这种细粒度的加锁会导致大量的加锁、释放锁操作，这对操作系统来说是一个比较重量级的操作，反而没有解释器级别的锁(GIL)性能高 [It isn't Easy to Remove the GIL](https://www.artima.com/weblogs/viewpost.jsp?thread=214235)

**GIL 的负面效果**

>it prevents multithreaded CPython programs from taking full advantage of multiprocessor systems in certain situations. Note that potentially blocking or long-running operations, such as I/O, image processing, and NumPy number crunching, happen outside the GIL. Therefore it is only in multithreaded programs that spend a lot of time inside the GIL, interpreting CPython bytecode, that the GIL becomes a bottleneck.

摘自：[GlobalInterpreterLock - Python Wiki](https://wiki.python.org/moin/GlobalInterpreterLock)

可以看出当程序是 I/O 密集型时，多线程是可以提高并发的


~~下面是简单地分析了 Python GIL 相关的源码，可以直接跳到本文最后看结论~~

### Thread And GIL Implementation

和线程相关的代码位于

```
Python/thread.c
Python/thread_nt.h
Python/thread_pthread.h
Python/ceval.h
Python/ceval_gil.h
Modules/_threadmodule.c
```

我们所能触碰的接口位于 `_threadmodule.c`

创建一个线程底层是调用的 `thread_PyThread_start_new_thread`

```
// _threadmodule.c
static PyObject *
thread_PyThread_start_new_thread(PyObject *self, PyObject *fargs)
{
    PyObject *func, *args, *keyw = NULL;
    struct bootstate *boot;
    unsigned long ident;

    // 将 fargs unpack 至 func, args, keyw 上
    if (!PyArg_UnpackTuple(fargs, "start_new_thread", 2, 3,
                           &func, &args, &keyw))
        return NULL;

    // 省略参数合法性检查 ...

    boot = PyMem_NEW(struct bootstate, 1);
    if (boot == NULL)
        return PyErr_NoMemory();
    boot->interp = PyThreadState_GET()->interp;
    boot->func = func;
    boot->args = args;
    boot->keyw = keyw;
    boot->tstate = _PyThreadState_Prealloc(boot->interp);
    if (boot->tstate == NULL) {
        PyMem_DEL(boot);
        return PyErr_NoMemory();
    }
    Py_INCREF(func);
    Py_INCREF(args);
    Py_XINCREF(keyw);
    PyEval_InitThreads(); /* Start the interpreter's thread-awareness */
    ident = PyThread_start_new_thread(t_bootstrap, (void*) boot);

    // 省略线程创建失败的检查

    return PyLong_FromUnsignedLong(ident);
}
```

`boot` 是什么我们先不考虑，直接来看 `PyEval_InitThreads` 做了什么

```
// ceval.c
static PyThread_type_lock pending_lock = 0; /* for pending calls */

void
PyEval_InitThreads(void)
{
    if (gil_created())
        return;
    create_gil();
    take_gil(PyThreadState_GET());
    main_thread = PyThread_get_thread_ident();
    if (!pending_lock)
        pending_lock = PyThread_allocate_lock();
}
```

当 Python 启动时，是不支持多线程的。换句话说，Python 中支持多线程的数据结构以及 GIL 都是没有创建的，Python 之所以有这种行为是因为大多数的 Python 程序都不需要多线程的支持，`gil_created()` 能够判断当前是否存在 GIL，如果没有那么通过 `create_gil` 去生成一把 GIL

```
// ceval_gil.h
#define MUTEX_T PyMUTEX_T
#define MUTEX_INIT(mut) \
    if (PyMUTEX_INIT(&(mut))) { \
        Py_FatalError("PyMUTEX_INIT(" #mut ") failed"); };
#define COND_T PyCOND_T
#define COND_INIT(cond) \
    if (PyCOND_INIT(&(cond))) { \
        Py_FatalError("PyCOND_INIT(" #cond ") failed"); };

/* Whether the GIL is already taken (-1 if uninitialized). This is atomic
   because it can be read without any lock taken in ceval.c. */
static _Py_atomic_int gil_locked = {-1};
/* This condition variable allows one or several threads to wait until
   the GIL is released. In addition, the mutex also protects the above
   variables. */
static COND_T gil_cond;
static MUTEX_T gil_mutex;

static int gil_created(void)
{
    // 原子操作，https://gcc.gnu.org/onlinedocs/gcc-7.3.0/gcc/_005f_005fatomic-Builtins.html
    return _Py_atomic_load_explicit(&gil_locked, _Py_memory_order_acquire) >= 0;
}

static void create_gil(void)
{
    MUTEX_INIT(gil_mutex);
    COND_INIT(gil_cond);
    _Py_atomic_store_relaxed(&gil_last_holder, 0);
    _Py_ANNOTATE_RWLOCK_CREATE(&gil_locked);
    _Py_atomic_store_explicit(&gil_locked, 0, _Py_memory_order_release);
}
```

这里拿 POSIX Thread(pthreads) 来举例子

```
// Python/condvar.h
#define PyMUTEX_T pthread_mutex_t
#define PyMUTEX_INIT(mut) pthread_mutex_init((mut), NULL)
#define PyCOND_T pthread_cond_t
#define PyCOND_INIT(cond) pthread_cond_init((cond), NULL)
```

等等，为什么出现了 Mutex、Cond，究竟哪个才算做 GIL 呢？ 参考 `ceval_gil.h` 上方的注释

>The GIL is just a boolean variable (gil_locked) whose access is protected by a mutex (gil_mutex), and whose changes are signalled by a condition variable (gil_cond). gil_mutex is taken for short periods of time, and therefore mostly uncontended.

那么为什么需要 Mutex 和 Cond 结合才行呢，一个 Mutex 不能搞定么？这是因为

>A pthread mutex isn't sufficient to model the Python lock type because, according to Draft 5 of the docs (P1003.4a/D5), both of the following are undefined:  
  -> a thread tries to lock a mutex it already has locked  
  -> a thread tries to unlock a mutex locked by a different thread  
pthread mutexes are designed for serializing threads over short pieces of code anyway, so wouldn't be an appropriate implementation of Python's locks regardless. The pthread_lock struct implements a Python lock as a "locked?" bit and a <condition, mutex> pair.  In general, if the bit can be acquired instantly, it is, else the pair is used to block the thread until the bit is cleared.     9 May 1994 tim@ksr.com

接下来主线程去拿 GIL(`take_gil(PyThreadState_GET())`)

```
// ceval_gil.h
#define MUTEX_LOCK(mut) \
    if (PyMUTEX_LOCK(&(mut))) { \
        Py_FatalError("PyMUTEX_LOCK(" #mut ") failed"); };
#define MUTEX_UNLOCK(mut) \
    if (PyMUTEX_UNLOCK(&(mut))) { \
        Py_FatalError("PyMUTEX_UNLOCK(" #mut ") failed"); };

static void take_gil(PyThreadState *tstate)
{
    // 省略条件编译及异常处理
    MUTEX_LOCK(gil_mutex);

    if (!_Py_atomic_load_relaxed(&gil_locked))
        goto _ready;

    while (_Py_atomic_load_relaxed(&gil_locked)) {
        int timed_out = 0;
        unsigned long saved_switchnum;

        saved_switchnum = gil_switch_number;
        COND_TIMED_WAIT(gil_cond, gil_mutex, INTERVAL, timed_out);
        /* If we timed out and no switch occurred in the meantime, it is time
           to ask the GIL-holding thread to drop it. */
        if (timed_out &&
            _Py_atomic_load_relaxed(&gil_locked) &&
            gil_switch_number == saved_switchnum) {
            SET_GIL_DROP_REQUEST();
        }
    }
_ready:
    /* We now hold the GIL */
    _Py_atomic_store_relaxed(&gil_locked, 1);
    _Py_ANNOTATE_RWLOCK_ACQUIRED(&gil_locked, /*is_write=*/1);

    // 发生不同线程之间切换
    if (tstate != (PyThreadState*)_Py_atomic_load_relaxed(&gil_last_holder)) {
        _Py_atomic_store_relaxed(&gil_last_holder, (uintptr_t)tstate);
        ++gil_switch_number;
    }

    if (_Py_atomic_load_relaxed(&gil_drop_request)) {
        RESET_GIL_DROP_REQUEST();
    }
    if (tstate->async_exc != NULL) {
        _PyEval_SignalAsyncExc();
    }

    MUTEX_UNLOCK(gil_mutex);
}
```

首先尝试获取 Mutex，如果 GIL 被别的线程占用(`gil_locked` 为 1)，那么我们去设置 Cond 进行等待

因为这时只有一个线程，主线程理所当然地拿到了 GIL

至此多线程的初始化已经完毕，开始创建新的线程

```
// _threadmodule.c
static PyObject *
thread_PyThread_start_new_thread(PyObject *self, PyObject *fargs)
{
    // ...
    struct bootstate *boot;
    boot = PyMem_NEW(struct bootstate, 1);
    boot->interp = PyThreadState_GET()->interp;
    boot->func = func;
    boot->args = args;
    boot->keyw = keyw;
    boot->tstate = _PyThreadState_Prealloc(boot->interp);
    // ...
    PyEval_InitThreads(); /* Start the interpreter's thread-awareness */
    ident = PyThread_start_new_thread(t_bootstrap, (void*) boot);
    // ...
    return PyLong_FromUnsignedLong(ident);
}
```

`boot` 结构体中存储了我们的线程信息

```
// thread_pthread.h
long
PyThread_start_new_thread(void (*func)(void *), void *arg)
{
    // 去掉了大量的条件编译
    pthread_t th;  // pthread_t 为 unsigned long 类型
    int status;
    dprintf(("PyThread_start_new_thread called\n"));
    if (!initialized)
        PyThread_init_thread();
    // 创建新的线程
    status = pthread_create(&th, (void* (*)(void *))func, (void *)arg);
    if (status != 0)
        return -1;
    // detached 该线程运行结束后会自动释放所有资源
    pthread_detach(th);
    return (long) th;  // 新线程 id
}
```

基本上就是调用的 pthread 的原生 API，也就是说这是一个操作系统提供的原生线程，其执行我们的 `t_bootstrap`

```
// _threadmodule.c
static void
t_bootstrap(void *boot_raw)
{
    struct bootstate *boot = (struct bootstate *) boot_raw;
    PyThreadState *tstate;
    PyObject *res;

    tstate = boot->tstate;
    tstate->thread_id = PyThread_get_thread_ident();
    _PyThreadState_Init(tstate);
    PyEval_AcquireThread(tstate);
    nb_threads++;
    res = PyEval_CallObjectWithKeywords(boot->func, boot->args, boot->keyw);
    // 省略异常处理
    Py_DECREF(res);
    Py_DECREF(boot->func);
    Py_DECREF(boot->args);
    Py_XDECREF(boot->keyw);
    PyMem_DEL(boot_raw);
    nb_threads--;
    PyThreadState_Clear(tstate);
    PyThreadState_DeleteCurrent();
    PyThread_exit_thread();
}
```

首先初始化线程状态 `_PyThreadState_Init`
获取 GIL `PyEval_AcquireThread`
执行我们的 Python 代码 `PyEval_CallObjectWithKeywords`
线程结束后销毁此线程状态 `PyThreadState_Clear`
释放 GIL `PyThreadState_DeleteCurrent`

从 `t_bootstrap` 的代码看上去，似乎子线程会一直执行，直到子线程的所有计算都完成，才会通过 `PyThreadState_DeleteCurrent` 释放 GIL。如此一来，那主线程岂非一直都会处于等待 GIL 的状态？如果真是这样，那哪来的多线程呀。这个后面会说道，现在先来看 `PyEval_AcquireThread`

```
// ceval.c
void
PyEval_AcquireThread(PyThreadState *tstate)
{
    /* Check someone has called PyEval_InitThreads() to create the lock */
    assert(gil_created());
    take_gil(tstate);
    if (PyThreadState_Swap(tstate) != NULL)
        Py_FatalError("PyEval_AcquireThread: non-NULL old thread state");
}
```

实际上还是调用了 `take_gil`，释放 GIL 最终是调用的 `drop_gil`

```
// pystate.c
void
PyThreadState_DeleteCurrent()
{
    PyThreadState *tstate = GET_TSTATE();
    if (tstate == NULL)
        Py_FatalError(
            "PyThreadState_DeleteCurrent: no current tstate");
    tstate_delete_common(tstate);
    if (autoInterpreterState && PyThread_get_key_value(autoTLSkey) == tstate)
        PyThread_delete_key_value(autoTLSkey);
    SET_TSTATE(NULL);
    PyEval_ReleaseLock();
}

// ceval.c
void
PyEval_ReleaseLock(void)
{
    /* This function must succeed when the current thread state is NULL.
       We therefore avoid PyThreadState_GET() which dumps a fatal error
       in debug mode.
    */
    drop_gil((PyThreadState*)_Py_atomic_load_relaxed(
        &_PyThreadState_Current));
}
```

```
// ceval_gil.h
static void drop_gil(PyThreadState *tstate)
{
    if (!_Py_atomic_load_relaxed(&gil_locked))
        Py_FatalError("drop_gil: GIL is not locked");
    /* tstate is allowed to be NULL (early interpreter init) */
    if (tstate != NULL) {
        /* Sub-interpreter support: threads might have been switched
           under our feet using PyThreadState_Swap(). Fix the GIL last
           holder variable so that our heuristics work. */
        _Py_atomic_store_relaxed(&gil_last_holder, (uintptr_t)tstate);
    }

    MUTEX_LOCK(gil_mutex);
    _Py_ANNOTATE_RWLOCK_RELEASED(&gil_locked, /*is_write=*/1);
    _Py_atomic_store_relaxed(&gil_locked, 0);
    COND_SIGNAL(gil_cond);  // 即 pthread_cond_signal
    MUTEX_UNLOCK(gil_mutex);
}
```

回答刚才的疑问，`t_bootstrap` 是不可能一直执行下去的。那么 Python 的线程何时进行切换呢? `ceval_gil.h` 中的注释对此进行了说明

>In the GIL-holding thread, the main loop (PyEval_EvalFrameEx) must be able to release the GIL on demand by another thread. A volatile boolean variable (gil_drop_request) is used for that purpose, which is checked at every turn of the eval loop. That variable is set after a wait of `interval` microseconds on `gil_cond` has timed out.

>A thread wanting to take the GIL will first let pass a given amount of time (`interval` microseconds) before setting gil_drop_request. This encourages a defined switching period, but doesn't enforce it since opcodes can take an arbitrary time to execute.

所以接下来看 Python 解释器的核心部分 `_PyEval_EvalFrameDefault`

```
// ceval.c
PyObject *
_PyEval_EvalFrameDefault(PyFrameObject *f, int throwflag)
{
    // 省略了大量的代码
    for (;;) {
        if (_Py_atomic_load_relaxed(&gil_drop_request)) {
            /* Give another thread a chance */
            if (PyThreadState_Swap(NULL) != tstate)
                Py_FatalError("ceval: tstate mix-up");
            drop_gil(tstate);

            /* Other threads may run now */

            take_gil(tstate);

            /* Check if we should make a quick exit. */
            if (_Py_Finalizing && _Py_Finalizing != tstate) {
                drop_gil(tstate);
                PyThread_exit_thread();
            }

            if (PyThreadState_Swap(tstate) != NULL)
                Py_FatalError("ceval: orphan tstate");
        }
    }
}
```

也就是说 `_Py_atomic_load_relaxed(&gil_drop_request)` 成立时，Python 会释放掉 GIL，让给其他的线程。但是这个线程会接着向下执行的，因为这是一个 C 线程，所以这里我们让它去拿 GIL 进行等待

在 Python2 中线程调度是通过计算执行的 opcode 数量(`_Py_Ticker`) 完成的，这类似于操作系统的时钟中断。这个值可以通过 `sys.getcheckinterval` 来得到，默认是 100。即执行 100 条 opcode 就会触发线程的再调度。但是由于部分 opcode 会进行跳转，所以实际执行的 opcode 可能多于 100

如果你使用 Python3.6 的话，你会发现此 `sys.getcheckinterval` 会有一个 DeprecationWarning。提醒你使用新的函数 `sys.getswitchinterval`，默认值是 `0.005`。这是一个时间间隔，也就是说目前 Python 是通过时间来衡量是否切换线程的。当然这个时间也是个最小值，实际上会比 `0.005` 高

```
// ceval_gil.h
/* microseconds (the Python API uses seconds, though) */
#define DEFAULT_INTERVAL 5000
static unsigned long gil_interval = DEFAULT_INTERVAL;
#define INTERVAL (gil_interval >= 1 ? gil_interval : 1)
```

`INTERVAL` 在我们尝试获取 GIL 的 `take_gil` 中有用到

```
// ceval_gil.h
static void take_gil(PyThreadState *tstate)
{
    while (_Py_atomic_load_relaxed(&gil_locked)) {
      int timed_out = 0;
      unsigned long saved_switchnum;

      saved_switchnum = gil_switch_number;
      COND_TIMED_WAIT(gil_cond, gil_mutex, INTERVAL, timed_out);
      /* If we timed out and no switch occurred in the meantime, it is time
         to ask the GIL-holding thread to drop it. */
      if (timed_out && _Py_atomic_load_relaxed(&gil_locked) &&
          gil_switch_number == saved_switchnum) {
          SET_GIL_DROP_REQUEST();
      }
    }
}
```

`timed_out` 是个 Flag，`COND_TIMED_WAIT` 宏是对于 pthread 的 `pthread_cond_timedwait` 函数的简单包裹。如果发生超时 `timed_out` 会被置为 `1`。如果这个过程中也没有发生线程切换(`gil_switch_number == saved_switchnum`)的话，会通过 `SET_GIL_DROP_REQUEST` 迫使拿到 GIL 的线程释放 GIL。

```
// ceval.c
#define SET_GIL_DROP_REQUEST() \
    do { \
        _Py_atomic_store_relaxed(&gil_drop_request, 1); \
        _Py_atomic_store_relaxed(&eval_breaker, 1); \
} while (0)
```

### Summary

经过一番研究，可以得出下面的几个结论

1. Python 线程基于操作系统原生线程
2. 获取到 GIL 的线程才能运行用户级别的代码
3. 同一时刻可以有多个 Python 级别线程在执行，比如线程的创建和销毁(如 `_PyThreadState_Init`)。通常说的 Python 多线程单核梗应该是指的是执行用户级别的代码。并且对于阻塞操作，会释放 GIL 然后再调用对应的 C API。比如 `time.sleep()` 的实现

```
// Modules/timemodule.c
Py_BEGIN_ALLOW_THREADS  // 释放 GIL
err = select(0, (fd_set *)0, (fd_set *)0, (fd_set *)0, &timeout);
Py_END_ALLOW_THREADS    // 获取 GIL
```

4. 具体执行哪个线程是由操作系统决定的
5. Python3 中获取 GIL 倾向于抢占，这种新的 GIL 实现曾试图 backport 至 2.7 但是被 reject [newgil backport](https://bugs.python.org/issue7753)
6. 在编写 Python 扩展时可以手动获取/释放 GIL
7. GIL 的获取和释放都是在执行完 Python 的 opcode 后发生的，每一条 opcode 都可以视为原子的，但是这并不代表我们写代码时不需要锁

比如经典的 A 线程执行完 `LOAD_NAME a` 后切换至 B 线程，它完成了 `+1`；再次切回 A 线程

```Python
In [2]: dis.dis(compile('a += 1', '<string>', 'exec'))
  1           0 LOAD_NAME                0 (a)
              3 LOAD_CONST               0 (1)
              6 INPLACE_ADD
              7 STORE_NAME               0 (a)
             10 LOAD_CONST               1 (None)
             13 RETURN_VALUE
```

锁是具有粒度的，不同级别需要不同粒度的锁。单线程的 Tornado 和 asyncio 也是实现了锁的，其用途可以参考 [what's Python asyncio.Lock() for?](https://stackoverflow.com/questions/25799576/whats-python-asyncio-lock-for)

关于 Python3 的 GIL 到底进行了那些修改，可以参考此篇 [[Python-Dev] Reworking the GIL](https://mail.python.org/pipermail/python-dev/2009-October/093321.html)

这里大致摘出几个要点：
新式 GIL(new GIL) 是为了解决如下的问题

1) 根据 opcode 的执行数量进行切换是一种比较粗糙的方式，因为每个 opcode 的执行时间不同。并且如果 opcode 的执行时间很短的话，会带来频繁的 GIL 获取/释放，系统调用是比较笨重的
2) 原有的线程切换并不意味着真的进行了线程切换，存在同一个线程释放 GIL 后再次获取到的情况。引入 forced thread switching 和 priority requests 来解决此问题

另外 Python 3.7 进行了修改，在 Python 初始化(`Py_Initialize`)时便会去创建 GIL，而不是按需创建 [bpo-20891](https://bugs.python.org/issue20891)

### Reference
[Python 源码剖析|陈儒](https://read.douban.com/ebook/1499455/)  
[Understanding GIL](https://www.youtube.com/watch?v=Obt-vMVdM8s)

Python Cookboock 作者对于 Python2 下 GIL 的讲解(有对应视频)  
[Inside the Python GIL](http://www.dabeaz.com/python/GIL.pdf)  
[Understanding the Python GIL](http://www.dabeaz.com/python/UnderstandingGIL.pdf)

    