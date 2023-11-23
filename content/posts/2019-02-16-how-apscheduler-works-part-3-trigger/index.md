
+++
title = "How APScheduler works - part 3 (trigger)"
summary = ''
description = ""
categories = []
tags = []
date = 2019-02-16T15:16:14+08:00
draft = false
+++

trigger 用于处理 job 的触发逻辑，对比如 cronjob, interval 等执行周期进行抽象。APScheduler 默认提供了三种内置的 trigger，他们分别是

- date: use when you want to run the job just once at a certain point of time
- interval: use when you want to run the job at fixed intervals of time
- cron: use when you want to run the job periodically at certain time(s) of day

另外 APScheduler 提供了 trigger combine 的功能。可以将 trigger 组合起来使用

```Python
from apscheduler.triggers.combining import AndTrigger
from apscheduler.triggers.interval import IntervalTrigger
from apscheduler.triggers.cron import CronTrigger


# 这个例子来自于文档，暗藏玄机
trigger = AndTrigger([IntervalTrigger(hours=2),
                      CronTrigger(day_of_week='sat,sun')])
scheduler.add_job(job_function, trigger)
```

这个任务将在周六和周日每两小时运行一次

下面以最基础的 `IntervalTrigger` 为例，首先来看基类 `BaseTrigger`

```Python
# triggers/base.py
class BaseTrigger(six.with_metaclass(ABCMeta)):
    """Abstract base class that defines the interface that every trigger must implement."""

    __slots__ = ()

    @abstractmethod
    def get_next_fire_time(self, previous_fire_time, now):
        """
        Returns the next datetime to fire on, If no such datetime can be calculated, returns
        ``None``.
        :param datetime.datetime previous_fire_time: the previous time the trigger was fired
        :param datetime.datetime now: current datetime
        """

    def _apply_jitter(self, next_fire_time, jitter, now):
        """
        Randomize ``next_fire_time`` by adding or subtracting a random value (the jitter). If the
        resulting datetime is in the past, returns the initial ``next_fire_time`` without jitter.
        ``next_fire_time - jitter <= result <= next_fire_time + jitter``
        :param datetime.datetime|None next_fire_time: next fire time without jitter applied. If
            ``None``, returns ``None``.
        :param int|None jitter: maximum number of seconds to add or subtract to
            ``next_fire_time``. If ``None`` or ``0``, returns ``next_fire_time``
        :param datetime.datetime now: current datetime
        :return datetime.datetime|None: next fire time with a jitter.
        """
        if next_fire_time is None or not jitter:
            return next_fire_time

        next_fire_time_with_jitter = next_fire_time + timedelta(
                seconds=random.uniform(-jitter, jitter))

        if next_fire_time_with_jitter < now:
            # Next fire time with jitter is in the past.
            # Ignore jitter to avoid false misfire.
            return next_fire_time

        return next_fire_time_with_jitter
```

`_apply_jitter` 负责给予抖动，返回的是将满足 `[next_fire_time - jitter, next_fire_time + jitter]`，能够避免多个机器在同一时间运行

```Python
class IntervalTrigger(BaseTrigger):
    """
    Triggers on specified intervals, starting on ``start_date`` if specified, ``datetime.now()`` +
    interval otherwise.
    :param int weeks: number of weeks to wait
    :param int days: number of days to wait
    :param int hours: number of hours to wait
    :param int minutes: number of minutes to wait
    :param int seconds: number of seconds to wait
    :param datetime|str start_date: starting point for the interval calculation
    :param datetime|str end_date: latest possible date/time to trigger on
    :param datetime.tzinfo|str timezone: time zone to use for the date/time calculations
    :param int|None jitter: advance or delay the job execution by ``jitter`` seconds at most.
    """

    __slots__ = 'timezone', 'start_date', 'end_date', 'interval', 'interval_length', 'jitter'

    def __init__(self, weeks=0, days=0, hours=0, minutes=0, seconds=0, start_date=None,
                 end_date=None, timezone=None, jitter=None):
        self.interval = timedelta(weeks=weeks, days=days, hours=hours, minutes=minutes,
                                  seconds=seconds)
        # 以秒为单位
        self.interval_length = timedelta_seconds(self.interval)
        if self.interval_length == 0:
            self.interval = timedelta(seconds=1)
            self.interval_length = 1

        if timezone:
            self.timezone = astimezone(timezone)
        elif isinstance(start_date, datetime) and start_date.tzinfo:
            self.timezone = start_date.tzinfo
        elif isinstance(end_date, datetime) and end_date.tzinfo:
            self.timezone = end_date.tzinfo
        else:
            self.timezone = get_localzone()

        start_date = start_date or (datetime.now(self.timezone) + self.interval)
        # 转换成带 时区 信息的 datetime，最后一个参数会被 format 到异常信息中
        self.start_date = convert_to_datetime(start_date, self.timezone, 'start_date')
        # convert_to_datetime 的第一个参数如果为 None 那么返回 None
        self.end_date = convert_to_datetime(end_date, self.timezone, 'end_date')

        self.jitter = jitter

    def get_next_fire_time(self, previous_fire_time, now):
        # 计算下一次的执行时间
        if previous_fire_time:
            # 根据上一次的运行时间计算出下次的运行时间
            next_fire_time = previous_fire_time + self.interval
        elif self.start_date > now:
            # 之前没运行过，如果当前时间未到指定的最早时间，那么直接返回这个最早时间
            next_fire_time = self.start_date
        else:
            # 之前没运行过，且当前时间大于 start_date 那么根据二者的时间差来计算下一次的执行时间
            timediff_seconds = timedelta_seconds(now - self.start_date)
            next_interval_num = int(ceil(timediff_seconds / self.interval_length))
            next_fire_time = self.start_date + self.interval * next_interval_num

        if self.jitter is not None:
            # 给予抖动
            next_fire_time = self._apply_jitter(next_fire_time, self.jitter, now)
        # 检验返回的时间是否在结束时间内，如果在之外那么便不需要再运行了
        if not self.end_date or next_fire_time <= self.end_date:
            return self.timezone.normalize(next_fire_time)

    def __getstate__(self):
        # 用于序列化
        return {
            'version': 2,
            'timezone': self.timezone,
            'start_date': self.start_date,
            'end_date': self.end_date,
            'interval': self.interval,
            'jitter': self.jitter,
        }

    def __setstate__(self, state):
        # 用于反序列化
        # This is for compatibility with APScheduler 3.0.x
        if isinstance(state, tuple):
            state = state[1]

        if state.get('version', 1) > 2:
            raise ValueError(
                'Got serialized data for version %s of %s, but only versions up to 2 can be '
                'handled' % (state['version'], self.__class__.__name__))

        self.timezone = state['timezone']
        self.start_date = state['start_date']
        self.end_date = state['end_date']
        self.interval = state['interval']
        self.interval_length = timedelta_seconds(self.interval)
        self.jitter = state.get('jitter')
```

组合的逻辑比较简单，`AndTrigger` 就是取包含的所有 trigger 的 `next_fire_time` 的交集

```Python
# triggers/combining.py
class BaseCombiningTrigger(BaseTrigger):
    __slots__ = ('triggers', 'jitter')

    def __init__(self, triggers, jitter=None):
        self.triggers = triggers
        self.jitter = jitter


class AndTrigger(BaseCombiningTrigger):
    """
    Always returns the earliest next fire time that all the given triggers can agree on.
    The trigger is considered to be finished when any of the given triggers has finished its
    schedule.
    Trigger alias: ``and``
    :param list triggers: triggers to combine
    :param int|None jitter: advance or delay the job execution by ``jitter`` seconds at most.
    """

    __slots__ = ()

    def get_next_fire_time(self, previous_fire_time, now):
        while True:
            fire_times = [trigger.get_next_fire_time(previous_fire_time, now)
                          for trigger in self.triggers]
            if None in fire_times:
                return None
            elif min(fire_times) == max(fire_times):
                return self._apply_jitter(fire_times[0], self.jitter, now)
            else:
                now = max(fire_times)

```

首先来解释本文最开始的代码有什么问题。我们已经熟悉 scheduler 的代码了，在 `_real_add_job` 中第一次添加 job 时会计算下一次的运行时间

```Python
# scheduler/base.py
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
```

第一个问题 `IntervalTrigger` 如果不指定 `start_date` 那么会使用当前时间，也就是方法调用时的时间。这个时间有很大的概率不是一个整点，而 `CronTrigger` 中指定了周六日运行。那么下一次的运行时间应当是下一个周六/日的零点。这两个 trigger 是不可能产生重合时间的。其实细想便知，不是所有的 trigger 都会产生相同的 `next_fire_time`。但是目前的 APScheduler 会陷入死循环而没有任何的提示，这点需要多加考虑才行

第二个问题即使产生了重合(未来也将确定会多次产生重合)，我们来看 job 的 `_get_run_times`(回忆第一章节中 scheduler 的 `_process_jobs` 方法执行逻辑)

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

这里向 `get_next_fire_time` 传入了一个 `datetime` 对象且它是小于 `now` 的。假设它是一个 `AndTrigger`，结合源代码可以知道它在循环中并没有更新 `previous_fire_time`，所以如果它包含一个 `IntervalTrigger` 的话，它永远会返回一个固定的时间(只要没指定 `end_date`)，且如果此时间小于另一个 trigger，那么也会陷入死循环

所以说这个地方是有很多坑的，另外需要注意如果给被组合的 trigger 设定了 `jitter`，那么条件可能永远不会成立

### Reference
[Advanced Python Scheduler Documentation](https://apscheduler.readthedocs.io/en/latest)  
[The Architecture of APScheduler](https://enqueuezero.com/concrete-architecture/apscheduler.html#overview)

    