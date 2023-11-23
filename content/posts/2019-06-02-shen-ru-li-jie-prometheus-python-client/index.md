
+++
title = "深入理解 Prometheus Python Client"
summary = ''
description = ""
categories = []
tags = []
date = 2019-06-02T12:49:36+08:00
draft = false
+++

本文使用代码版本为 0.6.0，commit sha 为 `d85d12060ed1e3d46201dca0f1da6a9345e2a23d`

### Basic

使用 prometheus_client 来监控 Flask 应用的示例

```Python
import time                                                                                                                            
from flask import Flask                                            
from flask import g              
from flask import request        
from flask import Response
from prometheus_client import Counter                              
from prometheus_client import Summary                              
from prometheus_client import generate_latest                      
from prometheus_client import REGISTRY                             

app = Flask(__name__)  

REQ_COUNTER = Counter(           
    "requests_total",            
    "How many HTTP requests processed, partitioned by status code and HTTP method.",                                                   
    ["path", "status_code"],  
)                                                                  
REQ_DURATION = Summary(   
    "request_duration_seconds", "The HTTP request latencies in seconds."                                                               
)                                                                  

@app.route("/")
def hello_world():
    time.sleep(1)
    return "Hello World!"

@app.route("/metrics")
def metrics_api():
    return Response(generate_latest(REGISTRY), mimetype="text/plain")

@app.before_request
def before_request_callback():
    if request.path == "/metrics":
        return

    g._start_time = time.monotonic()

@app.after_request
def after_request_callback(response):
    if request.path == "/metrics":
        return response

    REQ_DURATION.observe(time.monotonic() - g._start_time)
    REQ_COUNTER.labels(
        path=request.path, status_code=str(response.status_code)
    ).inc()

    return response


if __name__ == "__main__":
    app.run()

```

访问 `/metrics` 返回

```
# HELP requests_total How many HTTP requests processed, partitioned by status code and HTTP method.
# TYPE requests_total counter                                      
requests_total{path="/",status_code="200"} 1097.0                  
# TYPE requests_created gauge                                      
requests_created{path="/",status_code="200"} 1.5594727021534052e+09
# HELP request_duration_seconds The HTTP request latencies in seconds.
# TYPE request_duration_seconds summary                            
request_duration_seconds_count 1097.0                              
request_duration_seconds_sum 1101.3953003110364                    
# TYPE request_duration_seconds_created gauge   
```


Prometheus 提供了四种数据类型

- Counter: 只能增加，用于计数，比如 API 访问次数
- Guage: 可增可减，用于反馈当前的状态。比如内存、CPU 使用率
- Summary: 计算在一定时间窗口范围内度量指标对象的总数以及所有对量指标值的总和，同时统计百分位数
- Histogram: 和 Summary 类似，不过会返回在不同区间内样本的个数

详细请参考文档 [Metric types](https://prometheus.io/docs/concepts/metric_types/)

需要注意 **Python Client `Summary` 不会统计百分位数**

另外 Python Client 还提供了

- Info: 返回一组 key-value 信息，比如

```
# HELP python_info Python platform information                     
# TYPE python_info gauge
python_info{implementation="CPython",major="3",minor="7",patchlevel="3",version="3.7.3"} 1.0                                           
```

- Enum: 用于返回当前的状态，比如

```
# HELP task_state current task state
# TYPE task_state gauge
my_task_state{my_task_state="starting"} 0.0
my_task_state{my_task_state="running"} 1.0
my_task_state{my_task_state="stopped"} 0.0
```

可以注意到，他们的 Type 其实是 Guage，所以这只不过是一层糖。本质上还是没有离开四种类型

Python 的 Client 也提供了一些装饰器，可以简化代码，具体还是请看官方的 [README](https://github.com/prometheus/client_python/blob/master/README.md) 吧

### How it works

下面我们来说一下 Client 的工作原理。Client 提供了 Registry 机制，在 module 文件中默认实例化了一个 `REGISTRY`。这是一个利用 module 实现的单例。我们需要向 Registry 去注册我们的 Collector/Metrics。Collector 负责统计、输出 metrics。在 Client 中内置了三个 Collector，分别为 `PlatformCollector`、`ProcessCollector`、`GCCollector`。所有的类型均在 `__init__` 方法中都有一个参数 `registry`，并且给了一个默认值 `REGISTRY`，且它们也都在 module 文件的底部被实例化。所以当我们直接调用 `generate_latest` 的时候便会返回这些 metrics

如果想要自定义 Collector，只需要实现 `collect` 方法即可。比如下面的这个示例

```Python
class ComponentStatus:
    """Collector for Garbage collection statistics."""

    def __init__(self, registry=None):
        if registry is None:
            from prometheus_client.registry import REGISTRY
            registry = REGISTRY
        registry.register(self)

    def collect(self):
        g = GaugeMetricFamily(
            'component_status',
            'component status',
            labels=['component'],
        )

        if component_1.is_running():
            g.add_metric(['component_1'], value=1)
        else:
            g.add_metric(['component_1'], value=0)

        return [g]
```

通过检查 PID 来监控协同组件是否活着


我们再来看一下，最常用的 Metric 是如果工作的。四种 Metric 都继承了 `MetricWrapperBase`。其定义了 `collect` 方法，并且在 `__init__` 方法中向 `REGISTRY` 注册了自身

我们来看一下 `Counter` 的实现

```Python
# https://github.com/prometheus/client_python/blob/d85d12060ed1e3d46201dca0f1da6a9345e2a23d/prometheus_client/metrics.py#L197
class Counter(MetricWrapperBase):
    _type = 'counter'

    def _metric_init(self):
        self._value = values.ValueClass(self._type, self._name, self._name + '_total', self._labelnames, self._labelvalues)
        self._created = time.time()

    def inc(self, amount=1):
        '''Increment counter by the given amount.'''
        if amount < 0:
            raise ValueError('Counters can only be incremented by non-negative amounts.')
        self._value.inc(amount)
```

子类中并没有 overwrite `__init__` 方法，而是定义了 `_metric_init` 方法。这个方法是由 `MetricWrapperBase` 来调用的，是一个二阶段实例化。调用的条件是它非 `observable`，典型的场景就是使用 labels 的时候。当我们使用 labels 时，比如 `c = Counter('my_failures', '', ['reason'])`。此时我们无法直接调用 `c.inc()`，除非你提供了默认的 `labelvalues`。我们只能通过 `c.lables('bug').inc()` 的方式，`labels` 的源码如下

```Python
# https://github.com/prometheus/client_python/blob/d85d12060ed1e3d46201dca0f1da6a9345e2a23d/prometheus_client/metrics.py#L105
def labels(self, *labelvalues, **labelkwargs):
    with self._lock:
        if labelvalues not in self._metrics:
            self._metrics[labelvalues] = self.__class__(
                self._name,
                documentation=self._documentation,
                labelnames=self._labelnames,
                unit=self._unit,
                labelvalues=labelvalues,
                **self._kwargs
            )
        return self._metrics[labelvalues]
```

可以看到其实对于每一种 labels 的组合都会生成一个 `Counter` 来单独统计指标

`_is_observable` 的定义如下

```Python
#https://github.com/prometheus/client_python/blob/d85d12060ed1e3d46201dca0f1da6a9345e2a23d/prometheus_client/metrics.py#L51
def _is_observable(self):
    return not self._labelnames or (self._labelnames and self._labelvalues)
```

`Value` 是四种类型的底层的 *值* 的体现，它在不同场景下有不同的实现。默认情况下是一个线程安全的实现

```Python
# https://github.com/prometheus/client_python/blob/d85d12060ed1e3d46201dca0f1da6a9345e2a23d/prometheus_client/values.py#L9
class MutexValue(object):
    '''A float protected by a mutex.'''

    _multiprocess = False

    def __init__(self, typ, metric_name, name, labelnames, labelvalues, **kwargs):
        self._value = 0.0
        self._lock = Lock()

    def inc(self, amount):
        with self._lock:
            self._value += amount

    def set(self, value):
        with self._lock:
            self._value = value

    def get(self):
        with self._lock:
            return self._value
```

当存在 `prometheus_multiproc_dir` 环境变量的时候会变成 `MultiProcessValue`。它通过写文件的方式将当前的 Value 进行记录。需要向 Registry 注册 `MultiProcessCollector` 来统计所有进程的 metrics。为了提高性能 Client 使用了 mmap。这主要用在 pre-fork 模式上。所以说，内建的三个 Collector 都是统计单进程的。`MultiProcessCollector` 这是一个特殊的 Collector，它的 `collect` 方法中定义的是，读取 `prometheus_multiproc_dir` 里的所有文件，然后生成当前所有的 metrics。也就是说如果你在要多进程模式下运行，你要保证

- 启动的时候删除 `prometheus_multiproc_dir` 中的内容
- 终止的时候删除 `prometheus_multiproc_dir` 中的内容
- 定义一系列被 metrics，注册到默认的 Registry。然后定义一个新的 Registry，仅注册一个 `MultiProcessCollector`。在 `generate_latest` 的时候显式的传入刚才新定义的 Registry

使用两个 Registry 是因为需要防止部分 metrics 重复。比如两个进程，调用到 PID 2 的 metrics，这时 Registry 会调用所有已注册的 collector，而 `MultiProcessCollector` 已经会取所有进程的 metrics 了

下面分享一个多进程下的 GCCollector 的写法

```Python
class MultiProcessGCCollector:

    def __init__(self, registry=None):
        if not hasattr(gc, 'get_stats') or platform.python_implementation() != 'CPython':
            return
        if registry is None:
            from prometheus_client.registry import REGISTRY
            registry = REGISTRY

        self.collected = Gauge(
            'python_gc_objects_collected',
            'Objects collected during gc',
            ['generation'],
            multiprocess_mode='liveall'
        )
        self.uncollectable = Gauge(
            'python_gc_objects_uncollectable',
            'Uncollectable object found during GC',
            ['generation'],
            multiprocess_mode='liveall'
        )

        self.collections = Gauge(
            'python_gc_collections',
            'Number of times this generation was collected',
            ['generation'],
            multiprocess_mode='liveall'
        )
        registry.register(self)

    def collect(self):
        for generation, stat in enumerate(gc.get_stats()):
            generation_str = str(generation)
            self.collected.labels(generation_str).set(stat['collected'])
            self.uncollectable.labels(generation_str).set(stat['uncollectable'])
            self.collections.labels(generation_str).set(stat['collections'])
        return []  # 返回 []

REGISTRY = CollectorRegistry()
GC_COLLECTOR = MultiProcessGCCollector(REGISTRY)
multiprocess.MultiProcessCollector(REGISTRY)
```

输出的 metrics 会包含以 PID 为 label 的每个进程的 GC 统计信息

### Reference

[prometheus/client_python](https://github.com/prometheus/client_python)

    