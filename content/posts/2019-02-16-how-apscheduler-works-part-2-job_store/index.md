
+++
title = "How APScheduler works - part 2 (job_store)"
summary = ''
description = ""
categories = []
tags = []
date = 2019-02-16T09:23:58+08:00
draft = false
+++

APScheduler 默认提供了如下几种 job_store

- in-memory
- mongodb
- redis
- rethinkdb
- sqlalchemy
- zookeeper

所有的 jobstore 共同继承 `jobstore/base.py` 中的 `BaseJobStore` 基类。
本文这里简要的通过 redis jobstore 来举例。由上一节可知，job_store 需要支持增删改查，并且能够支持取出一定时间范围中的 job(`get_due_job`)，换言之就是实现排序。相信看过 Reids 实战的朋友，现在应该已经能想出 `RedisJobStore` 背后使用的哪两种数据结构了

首先来看一下公共基类 `BaseJobStore`

```Python
class BaseJobStore(six.with_metaclass(ABCMeta)):
    """Abstract base class that defines the interface that every job store must implement."""

    _scheduler = None
    _alias = None
    _logger = logging.getLogger('apscheduler.jobstores')

    def start(self, scheduler, alias):
        """
        Called by the scheduler when the scheduler is being started or when the job store is being added to an already running scheduler.
        :param apscheduler.schedulers.base.BaseScheduler scheduler: the scheduler that is starting this job store
        :param str|unicode alias: alias of this job store as it was assigned to the scheduler
        """

        self._scheduler = scheduler
        self._alias = alias
        self._logger = logging.getLogger('apscheduler.jobstores.%s' % alias)

    def shutdown(self):
        """Frees any resources still bound to this job store."""

    def _fix_paused_jobs_sorting(self, jobs):
        for i, job in enumerate(jobs):
            if job.next_run_time is not None:
                if i > 0:
                    paused_jobs = jobs[:i]
                    del jobs[:i]
                    jobs.extend(paused_jobs)
                break
    # 其余的都是抽象方法
    # def lookup_job(self, job_id): ...
    # def get_due_jobs(self, now): ...
    # def get_next_run_time(self): ...
    # def get_all_jobs(self): ...
    # def add_job(self, job): ...
    # def update_job(self, job): ...
    # def remove_job(self, job_id): ...
    # def remove_all_jobs(self): ...
```

`RedisJobStore` 中实现了这些抽象方法

```Python
class RedisJobStore(BaseJobStore):
    """
    Stores jobs in a Redis database. Any leftover keyword arguments are directly passed to redis's
    :class:`~redis.StrictRedis`.
    Plugin alias: ``redis``
    :param int db: the database number to store jobs in
    :param str jobs_key: key to store jobs in
    :param str run_times_key: key to store the jobs' run times in
    :param int pickle_protocol: pickle protocol level to use (for serialization), defaults to the
        highest available
    """

    def __init__(self, db=0, jobs_key='apscheduler.jobs', run_times_key='apscheduler.run_times',
                 pickle_protocol=pickle.HIGHEST_PROTOCOL, **connect_args):
        super(RedisJobStore, self).__init__()

        if db is None:
            raise ValueError('The "db" parameter must not be empty')
        if not jobs_key:
            raise ValueError('The "jobs_key" parameter must not be empty')
        if not run_times_key:
            raise ValueError('The "run_times_key" parameter must not be empty')

        self.pickle_protocol = pickle_protocol
        self.jobs_key = jobs_key
        self.run_times_key = run_times_key
        self.redis = StrictRedis(db=int(db), **connect_args)

    def lookup_job(self, job_id):
        """
        Returns a specific job, or ``None`` if it isn't found..
        The job store is responsible for setting the ``scheduler`` and ``jobstore`` attributes of
        the returned job to point to the scheduler and itself, respectively.
        :param str|unicode job_id: identifier of the job
        :rtype: Job
        """
        job_state = self.redis.hget(self.jobs_key, job_id)
        return self._reconstitute_job(job_state) if job_state else None

    def get_due_jobs(self, now):
        """
        Returns the list of jobs that have ``next_run_time`` earlier or equal to ``now``.
        The returned jobs must be sorted by next run time (ascending).
        :param datetime.datetime now: the current (timezone aware) datetime
        :rtype: list[Job]
        """
        timestamp = datetime_to_utc_timestamp(now)
        job_ids = self.redis.zrangebyscore(self.run_times_key, 0, timestamp)
        if job_ids:
            job_states = self.redis.hmget(self.jobs_key, *job_ids)
            return self._reconstitute_jobs(six.moves.zip(job_ids, job_states))
        return []

    def get_next_run_time(self):
        """
        Returns the earliest run time of all the jobs stored in this job store, or ``None`` if
        there are no active jobs.
        :rtype: datetime.datetime
        """
        next_run_time = self.redis.zrange(self.run_times_key, 0, 0, withscores=True)
        if next_run_time:
            return utc_timestamp_to_datetime(next_run_time[0][1])

    def get_all_jobs(self):
        """
        Returns a list of all jobs in this job store.
        The returned jobs should be sorted by next run time (ascending).
        Paused jobs (next_run_time == None) should be sorted last.
        The job store is responsible for setting the ``scheduler`` and ``jobstore`` attributes of
        the returned jobs to point to the scheduler and itself, respectively.
        :rtype: list[Job]
        """
        job_states = self.redis.hgetall(self.jobs_key)
        jobs = self._reconstitute_jobs(six.iteritems(job_states))
        paused_sort_key = datetime(9999, 12, 31, tzinfo=utc)  # 保证 paused job 放到最后
        return sorted(jobs, key=lambda job: job.next_run_time or paused_sort_key)

    def add_job(self, job):
        """
        Adds the given job to this store.
        :param Job job: the job to add
        :raises ConflictingIdError: if there is another job in this store with the same ID
        """
        if self.redis.hexists(self.jobs_key, job.id):
            raise ConflictingIdError(job.id)

        with self.redis.pipeline() as pipe:
            pipe.multi()
            pipe.hset(self.jobs_key, job.id, pickle.dumps(job.__getstate__(),
                                                          self.pickle_protocol))
            if job.next_run_time:
                # 3.5.3 没支持 redis 3.0+
                pipe.zadd(self.run_times_key, datetime_to_utc_timestamp(job.next_run_time), job.id)
            pipe.execute()

    def update_job(self, job):
        """
        Replaces the job in the store with the given newer version.
        :param Job job: the job to update
        :raises JobLookupError: if the job does not exist
        """
        if not self.redis.hexists(self.jobs_key, job.id):
            raise JobLookupError(job.id)

        with self.redis.pipeline() as pipe:
            pipe.hset(self.jobs_key, job.id, pickle.dumps(job.__getstate__(),
                                                          self.pickle_protocol))
            if job.next_run_time:
                pipe.zadd(self.run_times_key, datetime_to_utc_timestamp(job.next_run_time), job.id)
            else:
                pipe.zrem(self.run_times_key, job.id)
            pipe.execute()

    def remove_job(self, job_id):
        """
        Removes the given job from this store.
        :param str|unicode job_id: identifier of the job
        :raises JobLookupError: if the job does not exist
        """
        if not self.redis.hexists(self.jobs_key, job_id):
            raise JobLookupError(job_id)

        with self.redis.pipeline() as pipe:
            pipe.hdel(self.jobs_key, job_id)
            pipe.zrem(self.run_times_key, job_id)
            pipe.execute()

    def remove_all_jobs(self):
        """Removes all jobs from this store."""
        with self.redis.pipeline() as pipe:
            pipe.delete(self.jobs_key)
            pipe.delete(self.run_times_key)
            pipe.execute()

    def shutdown(self):
        self.redis.connection_pool.disconnect()

    def _reconstitute_job(self, job_state):
        # 从 Redis 中恢复 Job
        # 这里重新创建了新的 Job 对象
        job_state = pickle.loads(job_state)
        job = Job.__new__(Job)
        job.__setstate__(job_state)
        job._scheduler = self._scheduler
        job._jobstore_alias = self._alias
        return job

    def _reconstitute_jobs(self, job_states):
        jobs = []
        failed_job_ids = []
        for job_id, job_state in job_states:
            try:
                jobs.append(self._reconstitute_job(job_state))
            except BaseException:
                self._logger.exception('Unable to restore job "%s" -- removing it', job_id)
                failed_job_ids.append(job_id)

        # Remove all the jobs we failed to restore
        if failed_job_ids:
            with self.redis.pipeline() as pipe:
                pipe.hdel(self.jobs_key, *failed_job_ids)
                pipe.zrem(self.run_times_key, *failed_job_ids)
                pipe.execute()

        return jobs
```


阅读以上代码，便可得知 RedisJobStore 的存储结构:

Job 经过 pickle 序列化后，以 `job_id` 作为 key 存储在 Redis 的 Hash 数据结构(默认为 `apscheduler.jobs`)中。`lookup_job` 为 `O(1)` 操作。并且将 `job_id` 作为 member， `next_run_time` (处理成 UTC 的时间戳)作为 score，存储到 SortedSet 中。APScheduer 的应用层使用的是 `datetiem`，然后存时间的时候一律用 UTC 时区的时间戳，方便不同时区的机器使用同一个 Redis 作为 job_store，这也是数据库存储时间字段的常见做法

比如上节(schduler)中开始的例子，在 Redis 中会以如下的形式存储

```
127.0.0.1:6379> KEYS *
1) "example.jobs"
2) "example.run_times"
127.0.0.1:6379> HGETALL example.jobs
1) "a8a8485641b04ed4b2d2d381dde9dd13"
2) "序列化后的内容"
127.0.0.1:6379> ZSCORE example.run_times a8a8485641b04ed4b2d2d381dde9dd13
"1549978065.3989"
```

### Reference
[Advanced Python Scheduler Documentation](https://apscheduler.readthedocs.io/en/latest)  
[The Architecture of APScheduler](https://enqueuezero.com/concrete-architecture/apscheduler.html#overview)

    