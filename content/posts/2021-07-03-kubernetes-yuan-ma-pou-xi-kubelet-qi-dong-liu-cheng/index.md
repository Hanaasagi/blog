
+++
title = "Kubernetes 源码剖析 — kubelet 启动流程"
summary = ''
description = ""
categories = []
tags = []
date = 2021-07-03T13:15:00+08:00
draft = false
+++

*本文代码基于Kubernetes v1.21.2, commit sha 为 `092fbfbf53427de67cac1e9fa54aaa09a28371d7`*



Kubernetes 的命令行代码入口在 cmd/kubelet/组件名称.go 的文件中。命令行所使用的框架为 [spf13/cobra](https://github.com/spf13/cobra)。如没有特殊情况都是按照

1. 设置随机数种子值
2. 创建 `cobra.Command`
3. 调用 `command.Execute` 函数

这种套路来的，最后经过一些命令行参数解析，配置解析之后，会调用到相关组件的 `Run` 函数。以 kubelet 为例:



*P.S. 因为代码太长了，本文进行过省略和折行处理，所有经过省略的地方均以文字进行标注，相关源码文件及行号也已注明*



cmd/kubelet/app/server.go:434

```golang
func Run(
  ctx context.Context,
  s *options.KubeletServer,
  kubeDeps *kubelet.Dependencies,
  featureGate featuregate.FeatureGate
) error {
    // 1. .. 省略: 根据 options.KubeletServer 中的参数配置 klog
    // 2. .. 省略: 执行不同 OS 的初始化操作，目前仅有 Window 下有初始化操作，其他系统直接空函数
    // 3. 调用私有方法 `run`
    if err := run(ctx, s, kubeDeps, featureGate); err != nil {
        return fmt.Errorf("failed to run Kubelet: %v", err)
    }
    return nil
}
```

说明一下此函数接收的参数

1. `options.KubeletServer`，包含 `kubeletFlags` 和 `kubeletConfig` ，即从命令行的参数和配置文件读取的参数
2. `kubelet.Dependencies` ，包含了 kubelet 运行时需要注入的依赖，比如 cloudprovider
3. `featuregate.FeatureGate`，包含了启用的 Feature 用于使用试验性功能，参考 [feature-gates](https://kubernetes.io/docs/reference/command-line-tools-reference/feature-gates/)



这几个参数随着函数调用，传的蛮深的



cmd/kubelet/app/server.go:492

```golang
func run(
  ctx context.Context, 
  s *options.KubeletServer,
  kubeDeps *kubelet.Dependencies,
  featureGate featuregate.FeatureGate
) (err error) {
    // 1. .. 省略: 设置 FeatureGates, 然后检查 options 合法性
    // 2. .. 省略:  如果启动参数中包含 --lock-file 则会尝试获取文件锁，失败则退出
    // 如果文件锁获取成功，且参数中指定了 --exit-on-lock-contention
    // 则会创建一个 goroutine 借助 inotify 去 watch 文件，若产生 `inotify.InOpen|inotify.InDeleteSelf` 事件
    // 则 kubelet 退出
    // 3. .. 省略: 将 s.KubeletConfiguration 注册到 configz
    // 4. .. 省略: 根据 --kubeconfig 判断是按照 API server mode 还是 standalone mode 启动
    // 5. .. 省略: 初始化 CloudProvider, 1.23 之后就没了，可以不管
    // 6. .. 省略: 获取 hostName 和 nodeName
    // hostName 如若不指定，则获取的是 trim 后转小写的 `kern.hostname` 
    // nodeName 由 cloud-provider 提供，没有则使用 hostName
    // 7. .. 省略: 创建 KubeClient / EventClient / HeartbeatClient，
    // 如果是 standalone mode 则不需要。KubeClient 会向 API Server 发送节点的状态信息
    // 8. .. 省略: 将以下的 cgroups 加入 cadvisor 的 metrics 中
    // - root cgroup 可选，由 --cgroup-root 指定，比如 Systemd 和 cgroupfs
    // - kubelet cgroup 可选，由 --kubelet-cgroups 指定，默认是 kubelet 进程所在的，参考 GetKubeletContainer 函数
    // - runtime cgroup 可选，由 --runtime-cgroups 指定，一般是使用 docker 进程所在的
    // - system cgroup 可选，由 --system-cgroups 指定 
    // 9. 创建 ContainerManager
	if kubeDeps.ContainerManager == nil {
        // .. 省略计算 reserved-cpus 的部分
		kubeDeps.ContainerManager, err = cm.NewContainerManager(
			kubeDeps.Mounter,
			kubeDeps.CAdvisorInterface,
			cm.NodeConfig{
				RuntimeCgroupsName:    s.RuntimeCgroups,
				SystemCgroupsName:     s.SystemCgroups,
				KubeletCgroupsName:    s.KubeletCgroups,
				ContainerRuntime:      s.ContainerRuntime,
				CgroupsPerQOS:         s.CgroupsPerQOS,
				CgroupRoot:            s.CgroupRoot,
				CgroupDriver:          s.CgroupDriver,
				KubeletRootDir:        s.RootDirectory,
				ProtectKernelDefaults: s.ProtectKernelDefaults,
				NodeAllocatableConfig: cm.NodeAllocatableConfig{
					KubeReservedCgroupName:   s.KubeReservedCgroup,
					SystemReservedCgroupName: s.SystemReservedCgroup,
					EnforceNodeAllocatable:   sets.NewString(s.EnforceNodeAllocatable...),
					KubeReserved:             kubeReserved,
					SystemReserved:           systemReserved,
					ReservedSystemCPUs:       reservedSystemCPUs,
					HardEvictionThresholds:   hardEvictionThresholds,
				},
				QOSReserved:                             *experimentalQOSReserved,
				ExperimentalCPUManagerPolicy:            s.CPUManagerPolicy,
				ExperimentalCPUManagerReconcilePeriod:   s.CPUManagerReconcilePeriod.Duration,
				ExperimentalMemoryManagerPolicy:         s.MemoryManagerPolicy,
				ExperimentalMemoryManagerReservedMemory: s.ReservedMemory,
				ExperimentalPodPidsLimit:                s.PodPidsLimit,
				EnforceCPULimits:                        s.CPUCFSQuota,
				CPUCFSQuotaPeriod:                       s.CPUCFSQuotaPeriod.Duration,
				ExperimentalTopologyManagerPolicy:       s.TopologyManagerPolicy,
				ExperimentalTopologyManagerScope:        s.TopologyManagerScope,
			},
			s.FailSwapOn,
			devicePluginEnabled,
			kubeDeps.Recorder)
        )
    }
    // 10. 调用 RunKubelet，内部会新建 goroutine
	err = kubelet.PreInitRuntimeService(&s.KubeletConfiguration,
		kubeDeps, &s.ContainerRuntimeOptions,
		s.ContainerRuntime,
		s.RuntimeCgroups,
		s.RemoteRuntimeEndpoint,
		s.RemoteImageEndpoint,
		s.NonMasqueradeCIDR)
	if err != nil {
		return err
	}

	if err := RunKubelet(s, kubeDeps, s.RunOnce); err != nil {
		return err
	}
    // 11. .. 省略: 如果开启健康检查，那么会临时启一个暴露 /healthz 的 HTTPServer 的 goroutine
    // 12. .. 省略: 此 goroutine 等待 channel 中的 message 来结束
}

```

重点放在 `NewContainerManager` 和 `RunKubelet` 上面，首先看一下 `ContainerManager` 是什么结构

pkg/kubelet/cm/container_manager_linux.go:113

```golang
type containerManagerImpl struct {
	sync.RWMutex
	cadvisorInterface cadvisor.Interface
	mountUtil         mount.Interface
	NodeConfig
	status Status
	// External containers being managed.
	systemContainers []*systemContainer
	// Tasks that are run periodically
	periodicTasks []func()
	// Holds all the mounted cgroup subsystems
	subsystems *CgroupSubsystems
	nodeInfo   *v1.Node
	// Interface for cgroup management
	cgroupManager CgroupManager
	// Capacity of this node.
	capacity v1.ResourceList
	// Capacity of this node, including internal resources.
	internalCapacity v1.ResourceList
	// Absolute cgroupfs path to a cgroup that Kubelet needs to place all pods under.
	// This path include a top level container for enforcing Node Allocatable.
	cgroupRoot CgroupName
	// Event recorder interface.
	recorder record.EventRecorder
	// Interface for QoS cgroup management
	qosContainerManager QOSContainerManager
	// Interface for exporting and allocating devices reported by device plugins.
	deviceManager devicemanager.Manager
	// Interface for CPU affinity management.
	cpuManager cpumanager.Manager
	// Interface for memory affinity management.
	memoryManager memorymanager.Manager
	// Interface for Topology resource co-ordination
	topologyManager topologymanager.Manager
}

```

根据结构体中的 Field，可以了解到 `ContainerManager` 接管了大部分功能，比如 QOS, CPU/内存亲和性等。这里每个子组件都比较重要，功能都定义在 pkg/kubelet/cm 这个目录下面，我们以后对其进行展开



再来看 `RunKubelet` 函数

cmd/kubelet/app/server.go:1091

```golang
func RunKubelet(
    kubeServer *options.KubeletServer,
    kubeDeps *kubelet.Dependencies,
    runOnce bool
) error {
    // 1. .. 省略: 获取当前 node 的 ip，可以由参数 --node-ip 指定，默认值是本机 IPv4 地址。并进行 dual-stack 的一些校验
    // 2. 创建并初始化 kubelet 结构体
	k, err := createAndInitKubelet(
    	// ... 省略参数
    )
	if err != nil {
		return fmt.Errorf("failed to create kubelet: %v", err)
	}

	// NewMainKubelet should have set up a pod source config if one didn't exist
	// when the builder was run. This is just a precaution.
	if kubeDeps.PodConfig == nil {
		return fmt.Errorf("failed to create kubelet, pod source config was nil")
	}
	podCfg := kubeDeps.PodConfig

	if err := rlimit.SetNumFiles(uint64(kubeServer.MaxOpenFiles)); err != nil {
		klog.ErrorS(err, "Failed to set rlimit on max file handles")
	}

    // 3. 启动 startKubelet
	if runOnce {
		if _, err := k.RunOnce(podCfg.Updates()); err != nil {
			return fmt.Errorf("runonce failed: %v", err)
		}
		klog.InfoS("Started kubelet as runonce")
	} else {
		startKubelet(k, podCfg, &kubeServer.KubeletConfiguration, kubeDeps, kubeServer.EnableServer)
		klog.InfoS("Started kubelet")
	}
	return nil
}
```

此函数做了一个重要的事情就是创建了 `kubelet` 这个结构体，此后代码的位置就从 cmd 包转移到 pkg 包中了。`createAndInitKubelet` 里面还做了一些事情，比如调用了 `StartGarbageCollection` 这个函数，创建了节点上的镜像 GC 的 goroutine，之后的文章会对此展开



`startKubelet` 是对于以何种方式启动 kubelet 的简单封装

cmd/kubelet/app/server.go:1195

```Golang
func startKubelet(k kubelet.Bootstrap, podCfg *config.PodConfig, kubeCfg *kubeletconfiginternal.KubeletConfiguration, kubeDeps *kubelet.Dependencies, enableServer bool) {
	// start the kubelet
	go k.Run(podCfg.Updates())

	// start the kubelet server
	if enableServer {
		go k.ListenAndServe(kubeCfg, kubeDeps.TLSOptions, kubeDeps.Auth)
	}
	if kubeCfg.ReadOnlyPort > 0 {
		go k.ListenAndServeReadOnly(net.ParseIP(kubeCfg.Address), uint(kubeCfg.ReadOnlyPort))
	}
	if utilfeature.DefaultFeatureGate.Enabled(features.KubeletPodResources) {
		go k.ListenAndServePodResources()
	}
}
```



关键方法在 `Run` 这里 

pkg/kubelet/kubelet.go:1409

```golang
// Run starts the kubelet reacting to config updates
func (kl *Kubelet) Run(updates <-chan kubetypes.PodUpdate) {
    // 1. 初始化 logServer，这个是一个 net.http.FileServer，将 /var/log 挂在到 HTTP的 /logs/ 路径下面
	if kl.logServer == nil {
		kl.logServer = http.StripPrefix("/logs/", http.FileServer(http.Dir("/var/log/")))
	}
    // 2. Start the cloud provider sync manager
	if kl.cloudResourceSyncManager != nil {
		go kl.cloudResourceSyncManager.Run(wait.NeverStop)
	}

    // 3. 内部初始化，具体是以下逻辑:
    //    - 注册 Prometheus metrics
    //    - 创建 kubelet 所使用的目录，比如 /var/lib/kubelet，权限均是 0750
    //    - 确保 /var/log/containers 存在，否则创建，权限是 0755
    //    - 启动 imageManager，这个会负责镜像的 GC 逻辑
    //    - Server 的权限认证管理
    //    - 启动 OOMWatcher 检测 System 的事件 实现参考 pkg/kubelet/oom/oom_watcher_linux.go
    //    - 启动 ResourceAnalyzer，
	if err := kl.initializeModules(); err != nil {
		kl.recorder.Eventf(kl.nodeRef, v1.EventTypeWarning, events.KubeletSetupFailed, err.Error())
		klog.ErrorS(err, "failed to intialize internal modules")
		os.Exit(1)
	}

	// 4. Start volume manager, 循环判断哪些 POD 需要 mounted/unmount volume
	go kl.volumeManager.Run(kl.sourcesReady, wait.NeverStop)

    // 5. 上报节点信息
	if kl.kubeClient != nil {
		// Start syncing node status immediately, this may set up things the runtime needs to run.
		go wait.Until(kl.syncNodeStatus, kl.nodeStatusUpdateFrequency, wait.NeverStop)
		go kl.fastStatusUpdateOnce()

		// start syncing lease
		go kl.nodeLeaseController.Run(wait.NeverStop)
	}
    // 6. 检查 container runtime status，会产生 grpc 调用到 docker
	go wait.Until(kl.updateRuntimeUp, 5*time.Second, wait.NeverStop)

	// 7. Set up iptables util rules
	if kl.makeIPTablesUtilChains {
		kl.initNetworkUtil()
	}

	// 8. Start a goroutine responsible for killing pods (that are not properly
	// handled by pod workers).
	go wait.Until(kl.podKiller.PerformPodKillingWork, 1*time.Second, wait.NeverStop)

	// 9. StatusManager is the Source of truth for kubelet pod status, and should be kept up-to-date with
    // the latest v1.PodStatus. It also syncs updates back to the API server.
	kl.statusManager.Start()

	// 10. Start syncing RuntimeClasses if enabled.
	if kl.runtimeClassManager != nil {
		kl.runtimeClassManager.Start(wait.NeverStop)
	}

	// 11. Start the pod lifecycle event generator.
	kl.pleg.Start()
    // 12. 主循环
	kl.syncLoop(updates, kl)
}
```

`syncLoop` 是一个死循环，它监视多个 Channel 中的事件然后进行处理，核心逻辑在 `syncLoopIteration` 中
    