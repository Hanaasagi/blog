
+++
title = "How APScheduler works - part 1 (scheduler)"
summary = ''
description = ""
categories = []
tags = []
date = 2019-02-16T08:46:02+08:00
draft = false
+++

### Basic Concepts

APScheduler 是一个任务调度框架。它由以下的组件所组成:

- executor: 任务执行组件，提供不同的运行方式(线程、进程)
- jobstore： 任务持久化组件，提供不同的持久化后端
- scheduler: 任务调度组件，提供不同的调度器(tornado/background)
- trigger: 任务触发器组件，可以使用 cronjob 也可以直接指定 interval

一个简单的使用示例:

```Python
import sys
from datetime import datetime

from apscheduler.schedulers.blocking import BlockingScheduler


def tick():
    print(f'Tick! The time is: {datetime.now()}')


if __name__ == '__main__':
    scheduler = BlockingScheduler()
    scheduler.add_jobstore('redis', jobs_key='example.jobs', run_times_key='example.run_times')
    if len(sys.argv) > 1 and sys.argv[1] == '--clear':
        scheduler.remove_all_jobs()

    scheduler.add_job(tick, 'interval', seconds=10)
    # scheduler.add_job(tick, 'interval', seconds=10, id='example', replace_existing=True)
    try:
        scheduler.start()
    except KeyboardInterrupt:
        print('exit')
```

值得注意的是如果此脚本被反复重启，那么会重复添加任务。APScheduler 提供了 `replace_existing` 参数来避免向 jobstore 中重复添加 job_id 相同的任务

首先我们来看，APScheduler 中的 job

### Job

job 即我们要执行的具体函数(准确来说是 `callable`)，它可以是一个函数对象也可以是字符串形式的 import path。不同的 job 可以存储到不同的 jobstore 或者由不同的 exectuor 去执行

Job 中

- `id` 默认为 `uuid.uuid4().hex`
- `name`
- `callable` 我们要执行的
- `args` 执行时的参数
- `kwargs`  执行时的关键字参数
- `trigger`
- `executor`
- `misfire_grace_time` 此任务最多容忍被延迟多少秒
- `max_instances` 允许的最大并发执行数量
- `next_run_time` 下一次执行时间
- `coalesce` 是否需要合并执行
- `_scheduler`


特别的说一下 `misfire_grace_time`，由于系统负载及调度器实现等因素，job 的执行会被延迟(实际上一定是延迟执行的，只是会延迟多久的问题啦)。假如一个 job 本来 10:00 应当运行一次，但是由于某些原因没有被调度上，现在 10:01 了，这个 10:00 的运行实例被提交时，会检查它预计运行时间和当前时间的差值，如果大于我们设置的 `misfire_grace_time`，那么此次便不会执行

因为考虑到可能会产生大量的 job 实例，所以 job 使用了 `__slots__` 以减少对象的体积。job 中的许多方法都是通过 `_scheduler` 来执行的，如

```Python
# job.py
class Job(object):
    def modify(self, **changes):
        """
        Makes the given changes to this job and saves it in the associated job store.
        Accepted keyword arguments are the same as the variables on this class.
        .. seealso:: :meth:`~apscheduler.schedulers.base.BaseScheduler.modify_job`
        :return Job: this job instance
        """
        self._scheduler.modify_job(self.id, self._jobstore_alias, **changes)
        return self
```

这是因为在 APScheduler 的设计中，`scheduler` 是作为入口存在的，提供 API 去修改所有的组件。比如 `scheduler.modify_job` 会去修改 job 并触发 job_store 的更新

```Python
# schedulers/base.py
class BaseScheduler(six.with_metaclass(ABCMeta)):
    def modify_job(self, job_id, jobstore=None, **changes):
        """
        Modifies the properties of a single job.
        Modifications are passed to this method as extra keyword arguments.
        :param str|unicode job_id: the identifier of the job
        :param str|unicode jobstore: alias of the job store that contains the job
        :return Job: the relevant job instance
        """
        with self._jobstores_lock:
            job, jobstore = self._lookup_job(job_id, jobstore)
            job._modify(**changes)
            if jobstore:
                self._lookup_jobstore(jobstore).update_job(job)

        self._dispatch_event(JobEvent(EVENT_JOB_MODIFIED, job_id, jobstore))

        # Wake up the scheduler since the job's next run time may have been changed
        if self.state == STATE_RUNNING:
            self.wakeup()
        return job
```

相似的 API 不再赘述，再来看一个值得注意的 API

```Python
# job.py
class Job(object):
     def _get_run_times(self, now):
         """
         Computes the scheduled run times between ``next_run_time`` and ``now`` (inclusive).
         :type now: datetime.datetime
         :rtype: list[datetime.datetime]
         """
         run_times = []
         next_run_time = self.next_run_time
         while next_run_time and next_run_time <= now:
             run_times.append(next_run_time)
             next_run_time = self.trigger.get_next_fire_time(next_run_time, now)

         return run_times
```

这个 API 与一个 `concrete` 参数有关，它计算 `next_run_time` 到此时此刻，此 job 应当被执行多少次，并返回每次执行的时间。一种情况是如果 APScheduler 被停止，然后经过 N 小时后再次被启动。这时从 Redis 等持久化存储中去恢复 job，此 job 的 `next_run_time` 会是过去的某个时间。如果 `concrete` 为 `True` 那么便只会执行一次，否则将应该执行但未执行的次数都补上

job 持久化是通过 pickle 序列化完成的，其定义了 `__getstate__` 和 `__setstate__`

### Scheduler

scheduler 是核心逻辑，负责任务的调度。同时它也提供了 `add_jobstore`、`add_job` 等方法去控制其他的组件

APScheduler 原生提供了如下的几种 scheduler

- `asyncio`
- `background`
- `blocking`
- `gevent`
- `qt`
- `tornado`
- `twisted`

scheduler 有三种状态:

- `STATE_STOPPED`: indicating a scheduler's stopped stating
- `STATE_RUNNING`: indicating a scheduler's running state (started and processing jobs)
- `STATE_PAUSED`: indicating a scheduler's paused state (started but not processing jobs)


先来看一下 scheduler 的启动与停止

```Python
# scheduler/base.py
class BaseScheduler(six.with_metaclass(ABCMeta)):
    def __init__(self, gconfig={}, **options):
        super(BaseScheduler, self).__init__()
        # Dict[str, Any], key 为 executor alias, value 为 executor 实例
        self._executors = {}
        # threading.RLock
        self._executors_lock = self._create_lock()
        self._jobstores = {}
        self._jobstores_lock = self._create_lock()
        self._listeners = []
        self._listeners_lock = self._create_lock()
        self._pending_jobs = []
        self.state = STATE_STOPPED
        self.configure(gconfig, **options)

    def start(self, paused=False):
        """
        Start the configured executors and job stores and begin processing scheduled jobs.
        :param bool paused: if ``True``, don't start job processing until :meth:`resume` is called
        :raises SchedulerAlreadyRunningError: if the scheduler is already running
        :raises RuntimeError: if running under uWSGI with threads disabled
        """
        if self.state != STATE_STOPPED:
            raise SchedulerAlreadyRunningError

        self._check_uwsgi()  # uwsig 下有可能禁用线程

        # 启动所有的组件
        with self._executors_lock:
            # Create a default executor if nothing else is configured
            if 'default' not in self._executors:
                self.add_executor(self._create_default_executor(), 'default')  # 默认线程池

            # Start all the executors
            for alias, executor in six.iteritems(self._executors):
                executor.start(self, alias)

        with self._jobstores_lock:
            # Create a default job store if nothing else is configured
            if 'default' not in self._jobstores:
                self.add_jobstore(self._create_default_jobstore(), 'default')  # 默认内存

            # Start all the job stores
            for alias, store in six.iteritems(self._jobstores):
                store.start(self, alias)

            # Schedule all pending jobs
            for job, jobstore_alias, replace_existing in self._pending_jobs:
                # 将 job 持久化到 jobstore 中，此方法兼容 ADD/UPDATE
                # pending job 是在 scheduler 未启动时添加的 job
                self._real_add_job(job, jobstore_alias, replace_existing)
            del self._pending_jobs[:]

        self.state = STATE_PAUSED if paused else STATE_RUNNING
        self._logger.info('Scheduler started')
        self._dispatch_event(SchedulerEvent(EVENT_SCHEDULER_START))

        if not paused:
            self.wakeup()

    @abstractmethod
    def shutdown(self, wait=True):
        """
        Shuts down the scheduler, along with its executors and job stores.
        Does not interrupt any currently running jobs.
        :param bool wait: ``True`` to wait until all currently executing jobs have finished
        :raises SchedulerNotRunningError: if the scheduler has not been started yet
        """
        if self.state == STATE_STOPPED:
            raise SchedulerNotRunningError

        self.state = STATE_STOPPED

        # Shut down all executors
        with self._executors_lock:
            for executor in six.itervalues(self._executors):
                executor.shutdown(wait)

        # Shut down all job stores
        with self._jobstores_lock:
            for jobstore in six.itervalues(self._jobstores):
                jobstore.shutdown()

        self._logger.info('Scheduler has been shut down')
        self._dispatch_event(SchedulerEvent(EVENT_SCHEDULER_SHUTDOWN))
```

`wakeup` 是抽象方法，由特定的子类进行实现。比如 `BlockingScheduler` 就是唤醒了一下 `threading.Event`

```Python
# scheduler/blocking.py
class BlockingScheduler(BaseScheduler):
    """
    A scheduler that runs in the foreground
    (:meth:`~apscheduler.schedulers.base.BaseScheduler.start` will block).
    """
    _event = None

    def start(self, *args, **kwargs):
        self._event = Event()
        super(BlockingScheduler, self).start(*args, **kwargs)
        self._main_loop()

    def shutdown(self, wait=True):
        super(BlockingScheduler, self).shutdown(wait)
        self._event.set()

    def _main_loop(self):
        wait_seconds = TIMEOUT_MAX
        while self.state != STATE_STOPPED:
            self._event.wait(wait_seconds)
            self._event.clear()
            wait_seconds = self._process_jobs()

    def wakeup(self):
        self._event.set()
```

注意这里是先调用的 `super(BlockingScheduler, self).start(*args, **kwargs)` 这是会借由基类的方法实现去调用 `wakeup` ，之后再调用的 `_main_loop`时不会发生阻塞。而在 tornado scheduler 中则是构建 callback chain

```Python
# scheduler/tornado.py
class TornadoScheduler(BaseScheduler):
    def _start_timer(self, wait_seconds):
        self._stop_timer()
        if wait_seconds is not None:
            self._timeout = self._ioloop.add_timeout(timedelta(seconds=wait_seconds), self.wakeup)

    def _stop_timer(self):
        if self._timeout:
            self._ioloop.remove_timeout(self._timeout)
            del self._timeout

    @run_in_ioloop
    def wakeup(self):
        self._stop_timer()
        wait_seconds = self._process_jobs()
        self._start_timer(wait_seconds)
```

apscheduler 支持事件监听器(listener)。通过掩码可以选择只监听特定类型的事件。比如为 Prometheus 提供数据指标，用以观测 APScheduler 的运行状态。`_dispatch_event` 便是将事件分发至所有的 listener

```Python
# schduler/base.py
class BaseScheduler(six.with_metaclass(ABCMeta)):

    def add_listener(self, callback, mask=EVENT_ALL):
        """
        add_listener(callback, mask=EVENT_ALL)
        Adds a listener for scheduler events.
        When a matching event  occurs, ``callback`` is executed with the event object as its
        sole argument. If the ``mask`` parameter is not provided, the callback will receive events
        of all types.
        :param callback: any callable that takes one argument
        :param int mask: bitmask that indicates which events should be
            listened to
        .. seealso:: :mod:`apscheduler.events`
        .. seealso:: :ref:`scheduler-events`
        """
        with self._listeners_lock:
            self._listeners.append((callback, mask))

    def _dispatch_event(self, event):
        """
        Dispatches the given event to interested listeners.
        :param SchedulerEvent event: the event to send
        """
        with self._listeners_lock:
            listeners = tuple(self._listeners)

        for cb, mask in listeners:
            if event.code & mask:
                try:
                    cb(event)
                except BaseException:
                    self._logger.exception('Error notifying listener')
```

再来看一下 job 的添加/删除

```Python
# schduler/base.py
class BaseScheduler(six.with_metaclass(ABCMeta)):
    def add_job(self, func, trigger=None, args=None, kwargs=None, id=None, name=None,
                misfire_grace_time=undefined, coalesce=undefined, max_instances=undefined,
                next_run_time=undefined, jobstore='default', executor='default',
                replace_existing=False, **trigger_args):
        job_kwargs = {
            'trigger': self._create_trigger(trigger, trigger_args),  # 通过格式创建对应的 trigger
            'executor': executor,  # executor alias
            'func': func,  # 要执行的 callable
            'args': tuple(args) if args is not None else (),  # callable 的位置参数
            'kwargs': dict(kwargs) if kwargs is not None else {},  # callable 的关键字参数
            'id': id,  # job id
            'name': name,  # job name
            'misfire_grace_time': misfire_grace_time,  # 最大容忍延迟
            'coalesce': coalesce,  # 是否需要合并运行
            'max_instances': max_instances,  # 任务的最大并发执行实例数量
            'next_run_time': next_run_time  # 下一次运行时间(在这里便是第一次运行时间)
        }
        job_kwargs = dict((key, value) for key, value in six.iteritems(job_kwargs) if
                          value is not undefined)
        job = Job(self, **job_kwargs)  # 创建 job

        # Don't really add jobs to job stores before the scheduler is up and running
        with self._jobstores_lock:
            if self.state == STATE_STOPPED:
                # 如果 scheduler 未启动则添加到 pending 中
                # scheduler 启动时才会真正地被添加
                self._pending_jobs.append((job, jobstore, replace_existing))
                self._logger.info('Adding job tentatively -- it will be properly scheduled when '
                                  'the scheduler starts')
            else:
                self._real_add_job(job, jobstore, replace_existing)

        return job

    def remove_job(self, job_id, jobstore=None):
        jobstore_alias = None
        with self._jobstores_lock:
            if self.state == STATE_STOPPED:
                # Check if the job is among the pending jobs
                for i, (job, alias, replace_existing) in enumerate(self._pending_jobs):
                    if job.id == job_id and jobstore in (None, alias):
                        del self._pending_jobs[i]
                        jobstore_alias = alias
                        break
            else:
                # Otherwise, try to remove it from each store until it succeeds or we run out of
                # stores to check
                for alias, store in six.iteritems(self._jobstores):
                    if jobstore in (None, alias):
                        try:
                            store.remove_job(job_id)
                            jobstore_alias = alias
                            break
                        except JobLookupError:
                            continue

        if jobstore_alias is None:
            raise JobLookupError(job_id)

        # Notify listeners that a job has been removed
        event = JobEvent(EVENT_JOB_REMOVED, job_id, jobstore_alias)
        self._dispatch_event(event)

        self._logger.info('Removed job %s', job_id)
```

如果 scheduler 未启动则，添加的 job 会存储在一个 list 中，如果此时进程退出那么这些 job 则会丢失。所以要想 job 被持久化，需要保证

- 不适用 in-memory 的 job_store
- 添加 job 后启动 schduler

`_real_add_job` 是添加 job 的主要实现

```Python
# schduler/base.py
class BaseScheduler(six.with_metaclass(ABCMeta)):
    def _real_add_job(self, job, jobstore_alias, replace_existing):
        """
        :param Job job: the job to add
        :param bool replace_existing: ``True`` to use update_job() in case the job already exists
            in the store
        """
        # Fill in undefined values with defaults
        replacements = {}
        for key, value in six.iteritems(self._job_defaults):
            if not hasattr(job, key):
                replacements[key] = value

        # Calculate the next run time if there is none defined
        if not hasattr(job, 'next_run_time'):
            now = datetime.now(self.timezone)
            replacements['next_run_time'] = job.trigger.get_next_fire_time(None, now)

        # Apply any replacements
        job._modify(**replacements)

        # Add the job to the given job store
        store = self._lookup_jobstore(jobstore_alias)
        try:
            store.add_job(job)
        except ConflictingIdError:
            if replace_existing:
                store.update_job(job)
            else:
                raise

        # Mark the job as no longer pending
        job._jobstore_alias = jobstore_alias

        # Notify listeners that a new job has been added
        event = JobEvent(EVENT_JOB_ADDED, job.id, jobstore_alias)
        self._dispatch_event(event)

        self._logger.info('Added job "%s" to jobzhong store "%s"', job.name, jobstore_alias)

        # Notify the scheduler about the new job
        if self.state == STATE_RUNNING:
           self.wakeup()
```

当 job 被成功添加时，会计算下一次执行的时间，保存在 `next_run_time` 属性中

schduler 调度 job 的逻辑位于 `_process_jobs` 中

```Python
# schedulers/base.py
class BaseScheduler(six.with_metaclass(ABCMeta)):
    def _process_jobs(self):
        """
        Iterates through jobs in every jobstore, starts jobs that are due and figures out how long
        to wait for the next round.
        If the ``get_due_jobs()`` call raises an exception, a new wakeup is scheduled in at least
        ``jobstore_retry_interval`` seconds.
        """
        if self.state == STATE_PAUSED:
            self._logger.debug('Scheduler is paused -- not processing jobs')
            return None

        self._logger.debug('Looking for jobs to run')
        now = datetime.now(self.timezone)
        next_wakeup_time = None
        events = []

        with self._jobstores_lock:
            for jobstore_alias, jobstore in six.iteritems(self._jobstores):
                try:
                    # 返回一个 job list，满足 next_run_time 小于等于 now
                    # 并且为升序排列
                    due_jobs = jobstore.get_due_jobs(now)
                except Exception as e:
                    # Schedule a wakeup at least in jobstore_retry_interval seconds
                    self._logger.warning('Error getting due jobs from job store %r: %s',
                                         jobstore_alias, e)
                    retry_wakeup_time = now + timedelta(seconds=self.jobstore_retry_interval)
                    if not next_wakeup_time or next_wakeup_time > retry_wakeup_time:
                        next_wakeup_time = retry_wakeup_time

                    continue

                for job in due_jobs:
                    # Look up the job's executor
                    try:
                        executor = self._lookup_executor(job.executor)
                    except BaseException:
                        self._logger.error(
                            'Executor lookup ("%s") failed for job "%s" -- removing it from the '
                            'job store', job.executor, job)
                        self.remove_job(job.id, jobstore_alias)
                        continue

                    run_times = job._get_run_times(now)  # 从上次执行到现在应当执行多少次
                    run_times = run_times[-1:] if run_times and job.coalesce else run_times  # 合并执行
                    if run_times:
                        try:
                            executor.submit_job(job, run_times)  # 提交执行
                        except MaxInstancesReachedError:
                            self._logger.warning(
                                'Execution of job "%s" skipped: maximum number of running '
                                'instances reached (%d)', job, job.max_instances)
                            event = JobSubmissionEvent(EVENT_JOB_MAX_INSTANCES, job.id,
                                                       jobstore_alias, run_times)
                            events.append(event)
                        except BaseException:
                            self._logger.exception('Error submitting job "%s" to executor "%s"',
                                                   job, job.executor)
                        else:
                            event = JobSubmissionEvent(EVENT_JOB_SUBMITTED, job.id, jobstore_alias,
                                                       run_times)
                            events.append(event)

                        # Update the job if it has a next execution time.
                        # Otherwise remove it from the job store.
                        job_next_run = job.trigger.get_next_fire_time(run_times[-1], now)  # 下一次任务的执行时间
                        if job_next_run:
                            job._modify(next_run_time=job_next_run)
                            jobstore.update_job(job)
                        else:
                            self.remove_job(job.id, jobstore_alias)

                # Set a new next wakeup time if there isn't one yet or
                # the jobstore has an even earlier one
                jobstore_next_run_time = jobstore.get_next_run_time()
                if jobstore_next_run_time and (next_wakeup_time is None or
                                               jobstore_next_run_time < next_wakeup_time):
                    next_wakeup_time = jobstore_next_run_time.astimezone(self.timezone)

        # Dispatch collected events
        for event in events:
            self._dispatch_event(event)

        # Determine the delay until this method should be called again
        if self.state == STATE_PAUSED:
            wait_seconds = None
            self._logger.debug('Scheduler is paused; waiting until resume() is called')
        elif next_wakeup_time is None:
            wait_seconds = None
            self._logger.debug('No jobs; waiting until a job is added')
        else:
            wait_seconds = min(max(timedelta_seconds(next_wakeup_time - now), 0), TIMEOUT_MAX)
            self._logger.debug('Next wakeup is due at %s (in %f seconds)', next_wakeup_time,
                               wait_seconds)

        return wait_seconds
```

`next_wakeup_time` 这里有一个定时器中常见的优化。在轮询中，计算当前和下一次任务执行时间的时间差，用以减少无意义的轮询次数

### Reference
[Advanced Python Scheduler Documentation](https://apscheduler.readthedocs.io/en/latest)  
[The Architecture of APScheduler](https://enqueuezero.com/concrete-architecture/apscheduler.html#overview)

    