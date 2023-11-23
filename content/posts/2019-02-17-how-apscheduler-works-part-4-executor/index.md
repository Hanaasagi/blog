
+++
title = "How APScheduler works - part 4 (executor)"
summary = ''
description = ""
categories = []
tags = []
date = 2019-02-17T07:07:43+08:00
draft = false
+++

APScheduler 支持如下几种 executor

- `asyncio`
- `debug`
- `gevent`
- `processpool`
- `threadpool`
- `tornado`
- `twisted`


executor 负责 Job 的执行，本文这里选线程池的实现来进行说明

首先来看基类 `BaseExecutor`

```Python
# executors/base.py
class BaseExecutor(six.with_metaclass(ABCMeta, object)):
    """Abstract base class that defines the interface that every executor must implement."""

    _scheduler = None
    _lock = None
    _logger = logging.getLogger('apscheduler.executors')

    def __init__(self):
        super(BaseExecutor, self).__init__()
        self._instances = defaultdict(lambda: 0)

    def start(self, scheduler, alias):
        """
        Called by the scheduler when the scheduler is being started or when the executor is being
        added to an already running scheduler.
        :param apscheduler.schedulers.base.BaseScheduler scheduler: the scheduler that is starting
            this executor
        :param str|unicode alias: alias of this executor as it was assigned to the scheduler
        """
        self._scheduler = scheduler
        self._lock = scheduler._create_lock()
        self._logger = logging.getLogger('apscheduler.executors.%s' % alias)

    def shutdown(self, wait=True):
        """
        Shuts down this executor.
        :param bool wait: ``True`` to wait until all submitted jobs
            have been executed
        """

    def submit_job(self, job, run_times):
        """
        Submits job for execution.
        :param Job job: job to execute
        :param list[datetime] run_times: list of datetimes specifying
            when the job should have been run
        :raises MaxInstancesReachedError: if the maximum number of
            allowed instances for this job has been reached
        """
        assert self._lock is not None, 'This executor has not been started yet'
        with self._lock:
            if self._instances[job.id] >= job.max_instances:
                raise MaxInstancesReachedError(job)

            self._do_submit_job(job, run_times)
            self._instances[job.id] += 1

    @abstractmethod
    def _do_submit_job(self, job, run_times):
        """Performs the actual task of scheduling `run_job` to be called."""

    def _run_job_success(self, job_id, events):
        """
        Called by the executor with the list of generated events when :func:`run_job` has been
        successfully called.
        """
        with self._lock:
            self._instances[job_id] -= 1
            if self._instances[job_id] == 0:
                del self._instances[job_id]

        for event in events:
            self._scheduler._dispatch_event(event)

    def _run_job_error(self, job_id, exc, traceback=None):
        """Called by the executor with the exception if there is an error calling `run_job`."""
        with self._lock:
            self._instances[job_id] -= 1
            if self._instances[job_id] == 0:
                del self._instances[job_id]

        exc_info = (exc.__class__, exc, traceback)
        self._logger.error('Error running job %s', job_id, exc_info=exc_info)
```

`_instances` 中存储任务的运行实例数目，如果超过任务自身的所允许的 `max_instances` 则会拒绝执行。`_do_submit_job` 是执行逻辑的具体实现。而 `_run_job_success` 和 `_run_job_error` 是作为任务执行后的 callback，比如执行成功便将运行实例数减一，然后分发对应的事件。这里我们看到只有 `_run_job_success` 中有事件通知相关的代码，这是为什么呢。我们接着往下看

```Python
# executors/pool.py
class BasePoolExecutor(BaseExecutor):
    @abstractmethod
    def __init__(self, pool):
        super(BasePoolExecutor, self).__init__()
        self._pool = pool

    def _do_submit_job(self, job, run_times):
        def callback(f):
            exc, tb = (f.exception_info() if hasattr(f, 'exception_info') else
                       (f.exception(), getattr(f.exception(), '__traceback__', None)))
            if exc:
                self._run_job_error(job.id, exc, tb)
            else:
                self._run_job_success(job.id, f.result())

        f = self._pool.submit(run_job, job, job._jobstore_alias, run_times, self._logger.name)
        f.add_done_callback(callback)

    def shutdown(self, wait=True):
        self._pool.shutdown(wait)


class ThreadPoolExecutor(BasePoolExecutor):
    """
    An executor that runs jobs in a concurrent.futures thread pool.
    Plugin alias: ``threadpool``
    :param max_workers: the maximum number of spawned threads.
    """

    def __init__(self, max_workers=10):
        pool = concurrent.futures.ThreadPoolExecutor(int(max_workers))
        super(ThreadPoolExecutor, self).__init__(pool)
```

线程池选用的是 `concurrent.futures.ThreadPoolExecutor`，可以看到对于 future 对象添加了 `callback`，通过是否有异常来判断执行 `_run_job_error` 还是 `_run_job_success`。另外 `_run_job_success` 的第三个参数 `events` 是传入的 `f.result()`，那么它是什么呢。我们需要看一下 `run_job` 的实现，`run_job` 是对任务执行逻辑的底层封装

```Python
# executors/base.py
def run_job(job, jobstore_alias, run_times, logger_name):
    """
    Called by executors to run the job. Returns a list of scheduler events to be dispatched by the
    scheduler.
    """
    events = []
    logger = logging.getLogger(logger_name)
    for run_time in run_times:
        # See if the job missed its run time window, and handle
        # possible misfires accordingly
        if job.misfire_grace_time is not None:
            difference = datetime.now(utc) - run_time
            grace_time = timedelta(seconds=job.misfire_grace_time)
            if difference > grace_time:
                events.append(JobExecutionEvent(EVENT_JOB_MISSED, job.id, jobstore_alias,
                                                run_time))
                logger.warning('Run time of job "%s" was missed by %s', job, difference)
                continue

        logger.info('Running job "%s" (scheduled at %s)', job, run_time)
        try:
            retval = job.func(*job.args, **job.kwargs)
        except BaseException:
            exc, tb = sys.exc_info()[1:]
            formatted_tb = ''.join(format_tb(tb))
            events.append(JobExecutionEvent(EVENT_JOB_ERROR, job.id, jobstore_alias, run_time,
                                            exception=exc, traceback=formatted_tb))
            logger.exception('Job "%s" raised an exception', job)

            # This is to prevent cyclic references that would lead to memory leaks
            if six.PY2:
                sys.exc_clear()
                del tb
            else:
                import traceback
                traceback.clear_frames(tb)
                del tb
        else:
            events.append(JobExecutionEvent(EVENT_JOB_EXECUTED, job.id, jobstore_alias, run_time,
                                            retval=retval))
            logger.info('Job "%s" executed successfully', job)
    return events
```

通过阅读 `run_job` 可以知道返回值是事件集合，而且 `_run_job_error` 是只有在框架层代码出现异常时才会被调用。而应用层代码出现异常则是向返回值中添加 ERROR 事件，最后是调用的 `_run_job_success`。此外我们需要注意比如当前只有一个任务，但是由于一些原因它的 `run_times` 为 20，它也并不会并发执行。因为 `_do_submit_job` 中提交的一个内部会循环二十次的 job，而不是 20 个相同的 job

### Reference
[Advanced Python Scheduler Documentation](https://apscheduler.readthedocs.io/en/latest)  
[The Architecture of APScheduler](https://enqueuezero.com/concrete-architecture/apscheduler.html#overview)

    