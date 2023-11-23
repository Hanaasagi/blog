
+++
title = "Kubernetes 源码剖析 — kubelet ImageGC"
summary = ''
description = ""
categories = []
tags = []
date = 2021-07-05T13:36:00+08:00
draft = false
+++

*本文基于 Kubernetes v1.21.2, commit sha 为 `092fbfbf53427de67cac1e9fa54aaa09a28371d7`*



本文针对 kubelet 中的 ImageGC 进行分析，目光回到 `createAndInitKubelet` 上

cmd/kubelet/app/server.go:1211

```golang
func createAndInitKubelet(
    // .. 省略: 参数
) (k kubelet.Bootstrap, err error) {
	k, err = kubelet.NewMainKubelet(
        // .. 省略: 参数
    )
	if err != nil {
		return nil, err
	}
	k.StartGarbageCollection()
	return k, nil
}
```

在 `NewMainKubelet` 中，创建了 `imageManager` 结构体

pkg/kubelet/kubelet.go：341

```golang
// NewMainKubelet instantiates a new Kubelet object along with all the required internal modules.
// No initialization of Kubelet and its modules should happen here.
func NewMainKubelet(
    // .. 省略: 参数
) (*Kubelet, error) {
	klet := &Kubelet{
        // .. 省略: 参数
	}
    // .. 省略: 一堆组件的初始化即赋值到 klet 结构体的操作
	// setup imageManager
	imageManager, err := images.NewImageGCManager(
        klet.containerRuntime, 
        klet.StatsProvider, 
        kubeDeps.Recorder, 
        nodeRef,
        imageGCPolicy, 
        crOptions.PodSandboxImage
    )
	klet.imageManager = imageManager
	return klet, nil
}
```

然后在 `createAndInitKubelet` 函数中会调用 `StartGarbageCollection` 函数创建 `ImageGC` 的 goroutine

pkg/kubelet/kubelet.go:1274

```golang
// StartGarbageCollection starts garbage collection threads.
func (kl *Kubelet) StartGarbageCollection() {
	go wait.Until(func() {
		if err := kl.containerGC.GarbageCollect(); err != nil {
			klog.ErrorS(err, "Container garbage collection failed")
		} else {
			klog.V(vLevel).InfoS("Container garbage collection succeeded")
		}
	}, ContainerGCPeriod, wait.NeverStop)  // ContainerGCPeriod 1 分钟

	// when the high threshold is set to 100, stub the image GC manager
	if kl.kubeletConfiguration.ImageGCHighThresholdPercent == 100 {
		klog.V(2).InfoS("ImageGCHighThresholdPercent is set 100, Disable image GC")
		return
	}

	go wait.Until(func() {
		if err := kl.imageManager.GarbageCollect(); err != nil {
            
		} else {
			klog.V(vLevel).InfoS("Image garbage collection succeeded")
		}
	}, ImageGCPeriod, wait.NeverStop)  // ImageGCPeriod 5 分钟
}
```



首先来介绍影响镜像 GC 的三个参数，参考 [文档](https://kubernetes.io/docs/reference/config-api/kubelet-config.v1beta1/)

- image-gc-high-threshold：磁盘使用率上限，有效范围 `[0-100]`，默认 85
- image-gc-low-threshold：磁盘使用率下限，有效范围 `[0-100]`，默认 80
- minimum-image-ttl-duration：镜像最短应该生存的年龄，默认 2 分钟

我们可以先来做一个小实验，看看 kubelet 的 ImageGC 是如何工作的。这里笔者所使用的环境通过 [kubernetes-sigs/kind](https://github.com/kubernetes-sigs/kind) 来运行的 kubernetes。第一步更改 `/var/lib/kubelet/config.yaml`, 添加一下配置项

```Text
imageGCHighThresholdPercent: 2
imageGCLowThresholdPercent: 1
```

这样一定会触发 ImageGC 的。然后我们部署一个 nginx 的 Pod

```Text
$ kubectl run nginx --image=nginx
```

之后看节点的镜像列表

```Text
root@local-worker:/# ctr -n k8s.io images list -q
docker.io/kindest/kindnetd:v20210326-1e038dc5
docker.io/library/nginx:latest
docker.io/library/nginx@sha256:47ae43cdfc7064d28800bc42e79a429540c7c80168e8c8952778c0d5af1c09db
k8s.gcr.io/kube-proxy:v1.21.1
k8s.gcr.io/pause:3.5
k8s.gcr.io/pause@sha256:1ff6c18fbef2045af6b9c16bf034cc421a29027b800e4f9b68ae9b1cb3e9ae07
sha256:4f380adfc10f4cd34f775ae57a17d2835385efd5251d6dfe0f246b0018fb0399
sha256:ed210e3e4a5bae1237f1bb44d72a05a2f1e5c6bfe7a7e73da179e2534269c459
```

一共存在 4 个镜像，nginx 的镜像为 `4f380adfc10f` 。然后我们将 nginx 删除，这时其所属的镜像不会被任何容器所引用

```Text
$ kubectl delete pod nginx
```

我们可以观察 kubelet 的日志 

```Text
Jul 04 10:38:14 local-worker kubelet[119]: I0704 10:38:14.578833     119 image_gc_manager.go:304] "Disk usage on image filesystem is over the high threshold, trying to free bytes down to the low threshold" usage=73 highThreshold=2 amountToFree=96299243888 lowThreshold=1
Jul 04 10:38:14 local-worker kubelet[119]: E0704 10:38:14.581006     119 kubelet.go:1302] "Image garbage collection failed multiple times in a row" err="failed to garbage collect required amount of images. Wanted to free 96299243888 bytes, but freed 0 bytes"
Jul 04 10:38:24 local-worker kubelet[119]: I0704 10:38:24.271103     119 scope.go:111] "RemoveContainer" containerID="60da0a55e30ba583d9a9fad77e374554ccb46ca2cdb7a3a2e122d6184bc77b21"
Jul 04 10:38:24 local-worker kubelet[119]: I0704 10:38:24.275618     119 scope.go:111] "RemoveContainer" containerID="60da0a55e30ba583d9a9fad77e374554ccb46ca2cdb7a3a2e122d6184bc77b21"
Jul 04 10:38:24 local-worker kubelet[119]: E0704 10:38:24.279239     119 remote_runtime.go:334] "ContainerStatus from runtime service failed" err="rpc error: code = NotFound desc = an error occurred when try to find container \"60da0a55e30ba583d9a9fad77e374554ccb46ca2cdb7a3a2e122d6184bc77b21\": not found" containerID="60da0a55e30ba583d9a9fad77e374554ccb46ca2cdb7a3a2e122d6184bc77b21"
Jul 04 10:38:24 local-worker kubelet[119]: I0704 10:38:24.279504     119 pod_container_deletor.go:52] "DeleteContainer returned error" containerID={Type:containerd ID:60da0a55e30ba583d9a9fad77e374554ccb46ca2cdb7a3a2e122d6184bc77b21} err="failed to get container status \"60da0a55e30ba583d9a9fad77e374554ccb46ca2cdb7a3a2e122d6184bc77b21\": rpc error: code = NotFound desc = an error occurred when try to find container \"60da0a55e30ba583d9a9fad77e374554ccb46ca2cdb7a3a2e122d6184bc77b21\": not found"
Jul 04 10:38:24 local-worker kubelet[119]: I0704 10:38:24.331240     119 reconciler.go:196] "operationExecutor.UnmountVolume started for volume \"kube-api-access-k2cbj\" (UniqueName: \"kubernetes.io/projected/d589118c-7374-4ea5-aaab-2e4985232433-kube-api-access-k2cbj\") pod \"d589118c-7374-4ea5-aaab-2e4985232433\" (UID: \"d589118c-7374-4ea5-aaab-2e4985232433\") "
Jul 04 10:38:24 local-worker kubelet[119]: I0704 10:38:24.333804     119 operation_generator.go:829] UnmountVolume.TearDown succeeded for volume "kubernetes.io/projected/d589118c-7374-4ea5-aaab-2e4985232433-kube-api-access-k2cbj" (OuterVolumeSpecName: "kube-api-access-k2cbj") pod "d589118c-7374-4ea5-aaab-2e4985232433" (UID: "d589118c-7374-4ea5-aaab-2e4985232433"). InnerVolumeSpecName "kube-api-access-k2cbj". PluginName "kubernetes.io/projected", VolumeGidValue ""
Jul 04 10:38:24 local-worker kubelet[119]: I0704 10:38:24.432331     119 reconciler.go:319] "Volume detached for volume \"kube-api-access-k2cbj\" (UniqueName: \"kubernetes.io/projected/d589118c-7374-4ea5-aaab-2e4985232433-kube-api-access-k2cbj\") on node \"local-worker\" DevicePath \"\""
Jul 04 10:43:14 local-worker kubelet[119]: I0704 10:43:14.588014     119 image_gc_manager.go:304] "Disk usage on image filesystem is over the high threshold, trying to free bytes down to the low threshold" usage=73 highThreshold=2 amountToFree=96299063664 lowThreshold=1
Jul 04 10:43:14 local-worker kubelet[119]: I0704 10:43:14.590414     119 image_gc_manager.go:375] "Removing image to free bytes" imageID="sha256:ed210e3e4a5bae1237f1bb44d72a05a2f1e5c6bfe7a7e73da179e2534269c459" size=301416
Jul 04 10:43:14 local-worker kubelet[119]: I0704 10:43:14.596874     119 image_gc_manager.go:375] "Removing image to free bytes" imageID="sha256:4f380adfc10f4cd34f775ae57a17d2835385efd5251d6dfe0f246b0018fb0399" size=53740695
Jul 04 10:43:14 local-worker kubelet[119]: E0704 10:43:14.706395     119 kubelet.go:1302] "Image garbage collection failed multiple times in a row" err="failed to garbage collect required amount of images. Wanted to free 96299063664 bytes, but freed 54042111 bytes"

```

1. 在 Jul 04 10:38:14 的时候启动了一次 GC 检查，但是无法进行释放。因为 nginx 的 Pod 正在运行
2. 在 Jul 04 10:38:24 的时候我们删除了 Container
3. 在 Jul 04 10:43:14 的时候启动了一次 GC 检查，可以进行释放，删除了镜像 `ed210e3e4a5b` 和 `4f380adfc10f` 但是这次释放后还是无法低于最低阈值，依然在报警

经过 GC 后再次查看镜像列表

```Text
root@local-worker:/# ctr -n k8s.io images list -q
docker.io/kindest/kindnetd:v20210326-1e038dc5
k8s.gcr.io/kube-proxy:v1.21.1
```

剩下的镜像只有两个了。 pause 镜像和 nginx 的镜像被删除了



上面是一个常规的流程，我们来一点不常规的: 在节点上手动运行 Container，看看 kubelet 是否会处理。在 default 的 ns 下拉取 nginx 的镜像，然后手动跑一个容器

```Text
root@local-worker:/# ctr image list -q
docker.io/library/nginx:latest
root@local-worker:/# ctr run  -d docker.io/library/nginx:latest v1
root@local-worker:/# ctr container list
CONTAINER    IMAGE                             RUNTIME
v1           docker.io/library/nginx:latest    io.containerd.runc.v2
```

然后删除容器

```Text
root@local-worker:/# ctr container rm  v1
root@local-worker:/# ctr container list
CONTAINER    IMAGE    RUNTIME
root@local-worker:/# ctr image list -q
docker.io/library/nginx:latest
```

经过一次 GC 后发现 default 的 ns 下的镜像是不会被删除的，即使没有 Container 了

```Text
Jul 04 11:08:15 local-worker kubelet[119]: I0704 11:08:15.573914     119 image_gc_manager.go:304] "Disk usage on image filesystem is over the high threshold, trying to free bytes down to the low threshold" usage=73 highThreshold=2 amountToFree=96353073520 lowThreshold=1
Jul 04 11:08:15 local-worker kubelet[119]: E0704 11:08:15.576573     119 kubelet.go:1302] "Image garbage collection failed multiple times in a row" err="failed to garbage collect required amount of images. Wanted to free 96353073520 bytes, but freed 0 bytes"

# 这里我们删除了容器

Jul 04 11:13:15 local-worker kubelet[119]: I0704 11:13:15.577780     119 image_gc_manager.go:304] "Disk usage on image filesystem is over the high threshold, trying to free bytes down to the low threshold" usage=73 highThreshold=2 amountToFree=96353053040 lowThreshold=1
Jul 04 11:13:15 local-worker kubelet[119]: E0704 11:13:15.579804     119 kubelet.go:1302] "Image garbage collection failed multiple times in a row" err="failed to garbage collect required amount of images. Wanted to free 96353053040 bytes, but freed 0 bytes"

```

那么如果我们在 k8s.io 的 ns 下去做相同的事情呢？

```Text
root@local-worker:/# ctr -n k8s.io image pull docker.io/library/nginx:latest
root@local-worker:/# ctr -n k8s.io run -d docker.io/library/nginx:latest v1
/docker-entrypoint.sh: /docker-entrypoint.d/ is not empty, will attempt to perform configuration
/docker-entrypoint.sh: Looking for shell scripts in /docker-entrypoint.d/
root@local-worker:/# ctr -n k8s.io images list -q
docker.io/kindest/kindnetd:v20210326-1e038dc5
docker.io/library/nginx:latest
k8s.gcr.io/kube-proxy:v1.21.1
sha256:4f380adfc10f4cd34f775ae57a17d2835385efd5251d6dfe0f246b0018fb0399
root@local-worker:/# ctr -n k8s.io container list
CONTAINER                                                           IMAGE                                            RUNTIME
057832de4646850c121a7288c82351d390fe7016c2e7904607deb9b1463fecbe    k8s.gcr.io/pause:3.5                             io.containerd.runc.v2
23f2f96df21b26388c5342733b48892ed9d9190a704b0977093b3cec7c8efbb1    k8s.gcr.io/pause:3.5                             io.containerd.runc.v2
3c1e08af929131bd6f5b88b52f09701c9313dd7d839c8d02d1ed4a66c2d35ff8    docker.io/kindest/kindnetd:v20210326-1e038dc5    io.containerd.runc.v2
9101af29104768353b69553a13ffbb3de2ebb5c539a646591fe7365e4831c1fe    k8s.gcr.io/kube-proxy:v1.21.1                    io.containerd.runc.v2
a10739fca077e7d7acde18ddcbdaf62624a6695f8456c97d3691eee71027b932    docker.io/kindest/kindnetd:v20210326-1e038dc5    io.containerd.runc.v2
b35db41afb19a97706423cdf63fe7c5097d9cbb75298a8b5252b72c538d6962d    k8s.gcr.io/pause:3.5                             io.containerd.runc.v2
c6b5b3a38f9b697b59532f0a9d6a634f56583dfe00a78c3f76da97d0f85db8dc    k8s.gcr.io/pause:3.5                             io.containerd.runc.v2
d50043fb73e93a3d6fe0c7e67aa3f169996ed9cc4264b58c65a4134a249e7106    k8s.gcr.io/kube-proxy:v1.21.1                    io.containerd.runc.v2
v1                                                                  docker.io/library/nginx:latest                   io.containerd.runc.v2

```

我们再观测一次 GC

```Text
Jul 04 11:18:15 local-worker kubelet[119]: I0704 11:18:15.582943     119 image_gc_manager.go:304] "Disk usage on image filesystem is over the high threshold, trying to free bytes down to the low threshold" usage=73 highThreshold=2 amountToFree=96499661168 lowThreshold=1
Jul 04 11:18:15 local-worker kubelet[119]: E0704 11:18:15.585676     119 kubelet.go:1302] "Image garbage collection failed multiple times in a row" err="failed to garbage collect required amount of images. Wanted to free 96499661168 bytes, but freed 0 bytes"

# 这里我们在 k8s.io 的 namespace 创建了容器

Jul 04 11:23:15 local-worker kubelet[119]: I0704 11:23:15.588888     119 image_gc_manager.go:304] "Disk usage on image filesystem is over the high threshold, trying to free bytes down to the low threshold" usage=73 highThreshold=2 amountToFree=96499685744 lowThreshold=1
Jul 04 11:23:15 local-worker kubelet[119]: I0704 11:23:15.594540     119 image_gc_manager.go:375] "Removing image to free bytes" imageID="sha256:4f380adfc10f4cd34f775ae57a17d2835385efd5251d6dfe0f246b0018fb0399" size=53740695
Jul 04 11:23:15 local-worker kubelet[119]: E0704 11:23:15.600000     119 kubelet.go:1302] "Image garbage collection failed multiple times in a row" err="failed to garbage collect required amount of images. Wanted to free 96499685744 bytes, but freed 53740695 bytes"

```

**即使存在 Container，但是 kubelet 依旧删除了我们的镜像**

```Text
root@local-worker:/# ctr -n k8s.io image list -q  # k8s.io 下没有 nginx 镜像
docker.io/kindest/kindnetd:v20210326-1e038dc5
k8s.gcr.io/kube-proxy:v1.21.1

root@local-worker:/# ctr -n k8s.io container list  # k8s.io 下存在 nginx 的 Container
CONTAINER                                                           IMAGE                                            RUNTIME
057832de4646850c121a7288c82351d390fe7016c2e7904607deb9b1463fecbe    k8s.gcr.io/pause:3.5                             io.containerd.runc.v2
23f2f96df21b26388c5342733b48892ed9d9190a704b0977093b3cec7c8efbb1    k8s.gcr.io/pause:3.5                             io.containerd.runc.v2
3c1e08af929131bd6f5b88b52f09701c9313dd7d839c8d02d1ed4a66c2d35ff8    docker.io/kindest/kindnetd:v20210326-1e038dc5    io.containerd.runc.v2
9101af29104768353b69553a13ffbb3de2ebb5c539a646591fe7365e4831c1fe    k8s.gcr.io/kube-proxy:v1.21.1                    io.containerd.runc.v2
a10739fca077e7d7acde18ddcbdaf62624a6695f8456c97d3691eee71027b932    docker.io/kindest/kindnetd:v20210326-1e038dc5    io.containerd.runc.v2
b35db41afb19a97706423cdf63fe7c5097d9cbb75298a8b5252b72c538d6962d    k8s.gcr.io/pause:3.5                             io.containerd.runc.v2
c6b5b3a38f9b697b59532f0a9d6a634f56583dfe00a78c3f76da97d0f85db8dc    k8s.gcr.io/pause:3.5                             io.containerd.runc.v2
d50043fb73e93a3d6fe0c7e67aa3f169996ed9cc4264b58c65a4134a249e7106    k8s.gcr.io/kube-proxy:v1.21.1                    io.containerd.runc.v2
v1                                                                  docker.io/library/nginx:latest                   io.containerd.runc.v2

root@local-worker:/# ctr -n default container list
CONTAINER    IMAGE    RUNTIME

root@local-worker:/# ctr -n default image list -q  # default 下的 nginx 镜像依旧没有被删除
docker.io/library/nginx:latest

```



带着这些实验结果我们回过头来看代码 :

`NewImageGCManager` 负责初始化结构体，位于 pkg/kubelet/images/image_gc_manager.go:152，此函数校验了阈值规则，返回了结构体 `realImageGCManager` 定义于 pkg/kubelet/images/image_gc_manager.go:78，这里不再展开。我们主要来看 `GarbageCollect` 函数

pkg/kubelet/images/image_gc_manager.go:273

```golang
func (im *realImageGCManager) GarbageCollect() error {
    // 1. 获取磁盘空间使用量
	fsStats, err := im.statsProvider.ImageFsStats()
	if err != nil {
		return err
	}

	var capacity, available int64
	if fsStats.CapacityBytes != nil {
		capacity = int64(*fsStats.CapacityBytes)
	}
	if fsStats.AvailableBytes != nil {
		available = int64(*fsStats.AvailableBytes)
	}

	if available > capacity {
		klog.InfoS("Availability is larger than capacity", "available", available, "capacity", capacity)
		available = capacity
	}

	// Check valid capacity.
	if capacity == 0 {
		err := goerrors.New("invalid capacity 0 on image filesystem")
		im.recorder.Eventf(im.nodeRef, v1.EventTypeWarning, events.InvalidDiskCapacity, err.Error())
		return err
	}

    // 2. 计算当前使用率，如果超过最高阈值，那么尝试释放到最低阈值
	usagePercent := 100 - int(available*100/capacity)
	if usagePercent >= im.policy.HighThresholdPercent {
        // 估算期望释放的总量
		amountToFree := capacity*int64(100-im.policy.LowThresholdPercent)/100 - available
		klog.InfoS("Disk usage on image filesystem is over the high threshold, trying to free bytes down to the low threshold", "usage", usagePercent, "highThreshold", im.policy.HighThresholdPercent, "amountToFree", amountToFree, "lowThreshold", im.policy.LowThresholdPercent)
        // 进行释放
		freed, err := im.freeSpace(amountToFree, time.Now())
		if err != nil {
			return err
		}

        // 如果实际释放量低于期望释放量，那么会打日志并纪录事件
		if freed < amountToFree {
			err := fmt.Errorf("failed to garbage collect required amount of images. Wanted to free %d bytes, but freed %d bytes", amountToFree, freed)
			im.recorder.Eventf(im.nodeRef, v1.EventTypeWarning, events.FreeDiskSpaceFailed, err.Error())
			return err
		}
	}

	return nil
}

```

举个例子: 我的磁盘总共 100G，然后配置了

- LowThresholdPercent 设置 20
- HighThresholdPercent 设置 80

当前我镜像占用了 90G 的空间，那么这次的 `usagePercent` 为 90，高于配置的 `HighThresholdPercent`，需要执行回收逻辑。期望释放 70G 的空间，来满足最低阈值 `LowThresholdPercent`。如果实际释放的量不足 70G，会有事件纪录



关键函数有两个:

- `im.statsProvider.ImageFsStats()` 是如何计算使用量的
- `im.freeSpace` 磁盘空间释放逻辑



首先我们来看  `ImageFsStats` 函数。这个的具体实现依 `kubeDeps.useLegacyCadvisorStats` 而定:

- `true` 使用 cadvisor 的 API
- `false` 使用 Container Runtime 的返回的 Image Path 然后自己算的



这里对 CRI 的实现进行展开，`ImageFsStats` 函数的定义如下

pkg/kubelet/stats/cri_stats_provider.go:303

```golang
// ImageFsStats returns the stats of the image filesystem.
func (p *criStatsProvider) ImageFsStats() (*statsapi.FsStats, error) {
	resp, err := p.imageService.ImageFsInfo()
	fs := resp[0]
	s := &statsapi.FsStats{
		Time:      metav1.NewTime(time.Unix(0, fs.Timestamp)),
		UsedBytes: &fs.UsedBytes.Value,
	}
	if fs.InodesUsed != nil {
		s.InodesUsed = &fs.InodesUsed.Value
	}
	imageFsInfo := p.getFsInfo(fs.GetFsId())
	if imageFsInfo != nil {
		s.AvailableBytes = &imageFsInfo.Available
		s.CapacityBytes = &imageFsInfo.Capacity
		s.InodesFree = imageFsInfo.InodesFree
		s.Inodes = imageFsInfo.Inodes
	}
	return s, nil
}
```

磁盘使用量是通过 `ImageFsInfo` 函数获取的，这个会最终请求到 Container Runtime。比如 Docker 的就会调用 `/info` 这个路径然后拿 JSON 中的 `DockerRootDir` 这个字段，然后通过 Golang  std 的 `file path.Walk` 去遍历计算这个目录的大小。关于 Docker API 可以通过 `curl `调用试试，` curl --unix-socket /var/run/docker.sock http://127.0.0.1/info | jq` ，`DockerRootDir` 一般为路径 `/var/lib/docker`。这部分计算的代码在 pkg/kubelet/dockershim/docker_image_linux.go:33，这里不再展开了

磁盘的总量是通过 cadvisor 的 API 来获取的挂载点的文件系统信息然后提取的

pkg/kubelet/stats/cri_stats_provider.go:354

```golang
// getFsInfo returns the information of the filesystem with the specified
// fsID. If any error occurs, this function logs the error and returns
// nil.
func (p *criStatsProvider) getFsInfo(fsID *runtimeapi.FilesystemIdentifier) *cadvisorapiv2.FsInfo {
	if fsID == nil {
		klog.V(2).InfoS("Failed to get filesystem info: fsID is nil")
		return nil
	}
	mountpoint := fsID.GetMountpoint()
    // 这里还是会调用 cadvisor 的 API
	fsInfo, err := p.cadvisor.GetDirFsInfo(mountpoint)
	if err != nil {
		msg := "Failed to get the info of the filesystem with mountpoint"
		if err == cadvisorfs.ErrNoSuchDevice {
			klog.V(2).InfoS(msg, "mountpoint", mountpoint, "err", err)
		} else {
			klog.ErrorS(err, msg, "mountpoint", mountpoint)
		}
		return nil
	}
	return &fsInfo
}
```



最后我们来看一下释放的逻辑

pkg/kubelet/images/image_gc_manager.go:332

```golang
// Tries to free bytesToFree worth of images on the disk.
//
// Returns the number of bytes free and an error if any occurred. The number of
// bytes freed is always returned.
// Note that error may be nil and the number of bytes free may be less
// than bytesToFree.
func (im *realImageGCManager) freeSpace(bytesToFree int64, freeTime time.Time) (int64, error) {
    // 1. 获取应当被保留的镜像列表
	imagesInUse, err := im.detectImages(freeTime)

	im.imageRecordsLock.Lock()
	defer im.imageRecordsLock.Unlock()

    // 2. 不在 imagesInUse 中的镜像放到驱逐列表中
	images := make([]evictionInfo, 0, len(im.imageRecords))
	for image, record := range im.imageRecords {
		if isImageUsed(image, imagesInUse) {
			continue
		}
		images = append(images, evictionInfo{
			id:          image,
			imageRecord: *record,
		})
	}
    // 3. 按照 LRU 进行排序，使用的就是 imageRecord 的 lastUsed 字段
    // 这个字段是在 detectImages 中被更新的
	sort.Sort(byLastUsedAndDetected(images))

	// Delete unused images until we've freed up enough space.
	spaceFreed := int64(0)
	for _, image := range images {
		if image.lastUsed.Equal(freeTime) || image.lastUsed.After(freeTime) {
			continue
		}

        // 4. 如果这个镜像在节点上存在时间不够长，那么忽略，这个阈值是 2 分钟
		if freeTime.Sub(image.firstDetected) < im.policy.MinAge {
			continue
		}
        // 5. 删除 Image
		err := im.runtime.RemoveImage(container.ImageSpec{Image: image.id})
        // 6. 从内部列表中删除
		delete(im.imageRecords, image.id)
		spaceFreed += image.size
        // 7. 如果我们已经释放足够多的容量满足了期望值，那么中断，保证镜像的使用量不会小于 LowThresholdPercent
		if spaceFreed >= bytesToFree {
			break
		}
	}

	return spaceFreed, nil
}
```



`detectImages` 的工作逻辑如下

pkg/kubelet/images/image_gc_manager.go:209

```golang
func (im *realImageGCManager) detectImages(detectTime time.Time) (sets.String, error) {
	imagesInUse := sets.NewString()
    // 1. 将 Pause 容器的 Image 加入 imagesInUse，因为这个镜像不需要删除
	imageRef, err := im.runtime.GetImageRef(container.ImageSpec{Image: im.sandboxImage})
	if err == nil && imageRef != "" {
		imagesInUse.Insert(imageRef)
	}

    // 2. 列出节点上所有的镜像
	images, err := im.runtime.ListImages()
	if err != nil {
		return imagesInUse, err
	}
    
    // 3. 列出节点当前的所有 Pod
	pods, err := im.runtime.GetPods(true)
	if err != nil {
		return imagesInUse, err
	}

    // 3. 计算哪些 Image 需要被保留。
    // 如果镜像被 Pod 中的 Container 引用，那么加入 imagesInUse 中
	for _, pod := range pods {
		for _, container := range pod.Containers {
			imagesInUse.Insert(container.ImageID)
		}
	}

	now := time.Now()
	currentImages := sets.NewString()
	im.imageRecordsLock.Lock()
	defer im.imageRecordsLock.Unlock()
    // 4. 遍历节点上所有的 Image，内部维护了一 Map 来纪录镜像最近的使用时间
	for _, image := range images {
		currentImages.Insert(image.ID)
		if _, ok := im.imageRecords[image.ID]; !ok {
			im.imageRecords[image.ID] = &imageRecord{
				firstDetected: detectTime,
			}
		}
		if isImageUsed(image.ID, imagesInUse) {
			im.imageRecords[image.ID].lastUsed = now
		}
		im.imageRecords[image.ID].size = image.Size
	}

    // 5. 从 imageRecords 中删除已经不存在的镜像
	for image := range im.imageRecords {
		if !currentImages.Has(image) {
			delete(im.imageRecords, image)
		}
	}

    // 6. 返回需要被保留的镜像列表
	return imagesInUse, nil
}
```



这里仔细的读者会发现，这个代码的行为和之前我们实验的结果是有出入的。在实验中我们删除了 nginx 的 Pod，然后节点上的 `k8s.gcr.io/pause:3.5` 镜像是被删除了的。但是根据上面 `detectImage` 的逻辑来说，pause 即 sandboxImage 应当在最初的时候被加入 `imagesInUse` 这个集合里面，它根本不会被删除才对。这里其实有一个 Bug 的，因为笔者这里使用的是 containerd，在 kubelet 初始化的时候会判断 `ContainerRuntime` 是否为 `remote`



cmd/kubelet/app/server.go:196

```golang
if kubeletFlags.ContainerRuntime == "remote" && cleanFlagSet.Changed("pod-infra-container-image") {
	klog.InfoS("Warning: For remote container runtime, --pod-infra-container-image is ignored in kubelet, which should be set in that remote runtime instead")
}
```



但是之后的代码里面没有从 containerd 取获取 `sandboxImage` 的值。。。这就导致了在 `imageManager` 中拿到的 `sandboxImage` 是一个默认值。这个默认值是 `k8s.gcr.io/pause:3.4.1` 但是在 containerd 中使用的实际是 `k8s.gcr.io/pause:3.5` ，所以这个镜像被删除了


    