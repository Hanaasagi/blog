
+++
title = "从 timeshift restore 导致 home 挂载失败说起"
summary = ''
description = ""
categories = []
tags = []
date = 2021-12-08T15:28:56+08:00
draft = false
+++

新安装的 ArchLinux 使用的 btrfs 作为文件系统，使用了开源的工具 [timeshift](https://github.com/teejee2008/timeshift) 进行周期备份。某天突发奇想，我备份了这么多次，那么是否能够成功恢复呢？所以作死的使用了 `sudo timeshift --restore`来检验一下备份的效果，果不其然重启后挂了，报错如下

```
Starting version 249.7-2-arch
[FAILED] Failed to mount /home.
[DEPEND] Dependency failed for Local File Systems.
```



如果你遇到了相同的问题并在急求恢复方法的话，可以直接跳到 Solution。如果对 Linux 如何将 home 挂载的感兴趣，可以按照顺序阅读

### What happens before home is mounted

保持平和的心态，比起 Google 然后 Copy & Paste 并且用 sudo 去执行一些自己也不知道啥作用的命令更重要。首先来复习一下 Linux 是如何启动的。这里以 GRUB 接管之后开始说起，对于 BIOS到 MBR 再到 GRUB 的可以参考之前的 [文章 - Kurumi Atelier Day1](https://blog.dreamfever.me/kurumi-atelier-day1/) 。我们来看 GRUB 的配置文件

```
$ cat /boot/grub/grub.cfg
menuentry 'Arch Linux' --class arch --class gnu-linux --class gnu --class os $menuentry_id_option 'gnulinux-simple-a4d7523c-f215-4661-96c5-30ac6518a101' {
	load_video
	set gfxpayload=keep
	insmod gzio
	insmod part_gpt
	insmod fat
	search --no-floppy --fs-uuid --set=root A0FC-3CBD
	echo	'Loading Linux linux ...'
	linux	/vmlinuz-linux root=UUID=a4d7523c-f215-4661-96c5-30ac6518a101 rw rootflags=subvol=@  loglevel=3 quiet
	echo	'Loading initial ramdisk ...'
	initrd	/amd-ucode.img /initramfs-linux.img
}
```

`linux` 命令可以从文件加载 Linux 内核。第一个参数是文件名称，后面则是内核的命令行参数。比如我们在开机的 GRUB 界面选择了 "Arch Linux" 这一项，那么会以

```
root=UUID=a4d7523c-f215-4661-96c5-30ac6518a101 rw rootflags=subvol=@  loglevel=3
```

的参数去解压并装载内核 `/boot/vmlinuz-linux`。此文件可以通过 [extract-vmlinux](https://github.com/torvalds/linux/blob/master/scripts/extract-vmlinux) 脚本来提取，获得的是 ELF 可执行文件。`root` 选项指定的是根文件系统的设备名称或者 UUID。这个可以通过 `blkid` 命令查看，我这里的 `a4d7523c-f215-4661-96c5-30ac6518a101` 即是我硬盘的第二个分区的标识符

```
$ sudo blkid
/dev/nvme0n1p1: UUID="A0FC-3CBD" BLOCK_SIZE="512" TYPE="vfat" PARTUUID="bcda56df-ca67-a248-a4ed-5be1477df90d"
/dev/nvme0n1p2: UUID="a4d7523c-f215-4661-96c5-30ac6518a101" UUID_SUB="b5c16234-c5b1-4e9c-9236-5ef7ae83c5ba" BLOCK_SIZE="4096" TYPE="btrfs" PARTUUID="68a85e66-889c-ec42-a565-e3cd57675692"
```

`rw` 参数则是告诉允许读写，`rootflags` 里面是 mount 需要的一些参数，我这里则是 `btrfs` 的子卷信息 `subvol=@` 。所有 Kernel 接收的参数可以通过 [kernel-parameters](https://www.kernel.org/doc/html/v4.14/admin-guide/kernel-parameters.html) 查找



`initrd` 命令后面跟多个文件名称，作用是按顺序加载 Linux 内核的所有初始 RAM DISK，并在内存中的 Linux 设置区域中设置适当的参数。 比如 `/boot/initramfs-linux.img` 这个是一个包含了 busybox 的临时文件系统可以作为 `/` 被挂载，可以解压看一下里面的内容(不要在 /boot 目录里面直接搞

```
# 有可能是 gzip 有可能是 zstd，需要 file 一下看看压缩类型
$ file initramfs-linux.img
initramfs-linux.img: Zstandard compressed data (v0.8+), Dictionary ID: None
$ zstdcat initramfs-linux.img | cpio -idmv
```

我们会发现两个 shell 脚本 `init` 和 `init_functions` 这个后面会用到的

当初始化并且加载完成后，控制权便移交到 Linux Kernel 了。关键逻辑位于 `start_kernel` 函数中，包含了所有的启动逻辑，基本上就是在调函数。其中各个函数的作用可以通过 [The Linux 2.4 Kernel's Startup Procedure](http://coffeenix.net/doc/kernel/startup.html/t1.html) 来查看，虽然是 2.4 kernel 的，但是很多还是一样的。我们下面来看硬盘的文件系统是如何被挂载的，代码位于 [linux/init/main.c](https://github.com/torvalds/linux/blob/2a987e65025e2b79c6d453b78cb5985ac6e5eb26/init/main.c?_pjax=%23js-repo-pjax-container%2C%20div%5Bitemtype%3D%22http%3A%2F%2Fschema.org%2FSoftwareSourceCode%22%5D%20main%2C%20%5Bdata-pjax-container%5D#L924)

```c
static char *ramdisk_execute_command = "/init";

asmlinkage __visible void __init __no_sanitize_address start_kernel(void)
{
	arch_call_rest_init();
}

void __init __weak arch_call_rest_init(void)
{
	rest_init();
}

noinline void __ref rest_init(void)
{
	struct task_struct *tsk;
	int pid;

	rcu_scheduler_starting();
	/*
	 * We need to spawn init first so that it obtains pid 1, however
	 * the init task will end up wanting to create kthreads, which, if
	 * we schedule it before we create kthreadd, will OOPS.
	 */
	pid = kernel_thread(kernel_init, NULL, CLONE_FS);
    // ...
}


static int __ref kernel_init(void *unused)
{
	int ret;

	if (ramdisk_execute_command) {
		ret = run_init_process(ramdisk_execute_command);
		if (!ret)
			return 0;
		pr_err("Failed to execute %s (error %d)\n",
		       ramdisk_execute_command, ret);
	}

	/*
	 * We try each of these until one succeeds.
	 *
	 * The Bourne shell can be used instead of init if we are
	 * trying to recover a really broken machine.
	 */
	if (execute_command) {
		ret = run_init_process(execute_command);
		if (!ret)
			return 0;
		panic("Requested init %s failed (error %d).",
		      execute_command, ret);
	}

	if (CONFIG_DEFAULT_INIT[0] != '\0') {
		ret = run_init_process(CONFIG_DEFAULT_INIT);
		if (ret)
			pr_err("Default init %s failed (error %d)\n",
			       CONFIG_DEFAULT_INIT, ret);
		else
			return 0;
	}

	if (!try_to_run_init_process("/sbin/init") ||
	    !try_to_run_init_process("/etc/init") ||
	    !try_to_run_init_process("/bin/init") ||
	    !try_to_run_init_process("/bin/sh"))
		return 0;

	panic("No working init found.  Try passing init= option to kernel. "
	      "See Linux Documentation/admin-guide/init.rst for guidance.");
}
```

核心逻辑位于 `kernel_init` 中，执行这个函数的线程会成为之后 PID 为 1 的进程，也就是通常的 init 进程。`run_init_process` 在调用的时候，用的是`kernel_execve`，会替换掉当前的上下文。`kernel_init` 分为 3 层的 fallback 逻辑。`ramdisk_execute_command` 这个变量的默认值是 `/init` ，但是也可以被 `grub.cfg` 中指定的启动参数覆盖掉，对应的为 `init=` 参数。而 `/init` 正式我们之前解压 `/boot/initramfs-linux.img` 后的到的脚本。脚本我在这里放到 [Gist](https://gist.github.com/Hanaasagi/2665e1e28ffcf91d5d62f72fa52fb732) 一份。因为比较长，核心逻辑概述为下:

1. `mount_setup` 挂载特殊文件系统，如 `proc` 到指定的挂载点
2. 从 `/proc/cmdline` 获得内核的参数。可以在机器上 `cat /proc/cmdline` 这个和 `grub.cfg` 中的应当是一致的
3. 根据上一步的参数执行 mount 将真正的 `/` 挂载到 RAM DISK 的 `/new_root ` 上

最后执行

```bash
exec env -i \
    "TERM=$TERM" \
    /usr/bin/switch_root /new_root $init "$@"
```

`switch_root` 可以将别的文件系统作为新的 `/`，并且还会将 `/proc`, `/dev`, `/sys` 等自动挂载到新的文件系统对应的位置。详细文档可以参考 `man 8 switch_root`。`$init` 变量的值为 `/sbin/init` ，注意这里是我们真正意义上 `/` 下的文件，即 `/lib/systemd/systemd` 的一个软链接。此时我们 systemd 成为 init 进程，接管了之后的 mount 操作

对于 systemd 而言，这边是一个个 target。我们要找的进行 mount 操作的位于 `systemd.mount` 下面，可以通过 `man 5 systemd.mount` 查看详细的说明。简单来讲，systemd 会读取 `/etc/fstab` 文件，然后动态的生成 `xxx.mount` 这种 target，比如 home 就是 `home.target` 。`/etc/fstab` 文件如下

```
$ cat /etc/fstab
# /dev/nvme0n1p2
UUID=a4d7523c-f215-4661-96c5-30ac6518a101	/home     	btrfs      rw,noatime,compress=lzo,ssd,space_cache,subvolid=358,subvol=/@home	0 0
```

此文件声明挂载点和被挂载的文件系统的关系，还有执行 mount 时用到的参数。至此用户的文件系统被挂载完成



### Solution

既然由 systemd 来负责，那么我们来研究一下运行日志。为了拿到这些素材，我只能再表演一下了


<img src="../../images/2021/12/1638859901006.png" style="max-width:400px;" align="center">
```
$ systemctl status home.mount
× home.mount - /home
     Loaded: loaded (/etc/fstab; generated)
     Active: failed (Result: exit-code) since Tue 2021-12-07 13:59:26 CST; 34s ago
      Where: /home
       What: /dev/disk/by-uuid/a4d7523c-f215-4661-96c5-30ac6518a101
       Docs: man:fstab(5)
             man:systemd-fstab-generator(8)
        CPU: 3ms

Dec 07 13:59:26 misaka systemd[1]: Mounting /home...
Dec 07 13:59:26 misaka mount[458]: mount: /home: wrong fs type, bad option, bad superblock on /dev/nvme0n1p2, missing codepage or helper program, or other error.
Dec 07 13:59:26 misaka systemd[1]: home.mount: Mount process exited, code=exited, status=32/n/a
Dec 07 13:59:26 misaka systemd[1]: home.mount: Failed with result 'exit-code'.
Dec 07 13:59:26 misaka systemd[1]: Failed to mount /home.
```

初步看起来像是 subvolume 损坏，无法被正常识别。所以需要看一下 `@home` 子卷的情况

```
$ btrfs subvolume --list /
ID 256 gen 15559 top level 5 path timeshift-btrfs/snapshots/2021-12-07_12-49-52/@
ID 258 gen 15729 top level 5 path @tmp
ID 260 gen 15259 top level 5 path @snapshots
ID 261 gen 15730 top level 5 path @var
ID 264 gen 32 top level 261 path @var/lib/portables
ID 265 gen 33 top level 261 path @var/lib/machines
ID 331 gen 15706 top level 5 path timeshift-btrfs/snapshots/2021-12-07_10-43-23/@
ID 332 gen 15267 top level 5 path timeshift-btrfs/snapshots/2021-12-07_10-43-23/@home
ID 342 gen 15601 top level 5 path timeshift-btrfs/snapshots/2021-12-07_12-44-32/@
ID 343 gen 15600 top level 5 path timeshift-btrfs/snapshots/2021-12-07_12-44-32/@home
ID 351 gen 15726 top level 5 path timeshift-btrfs/snapshots/2021-12-07_13-59-01/@home
ID 353 gen 15727 top level 5 path timeshift-btrfs/snapshots/2021-12-07_13-59-01/@
ID 354 gen 15703 top level 5 path timeshift-btrfs/snapshots/2021-12-07_13-46-12/@
ID 355 gen 15704 top level 5 path timeshift-btrfs/snapshots/2021-12-07_13-46-12/@home
ID 356 gen 15725 top level 5 path timeshift-btrfs/snapshots/2021-12-07_13-57-58/@
ID 357 gen 15724 top level 5 path timeshift-btrfs/snapshots/2021-12-07_13-57-58/@home
ID 358 gen 15724 top level 5 path @home
ID 359 gen 15730 top level 5 path @
```

子卷能够被正常列出，那么手动尝试挂载子卷

```
 $ sudo mount /dev/nvme0n1p2 -o subvolid=358 /mnt  # subvolid 为上面命令的 ID，找到需要挂载的 home 子卷，然后挂载到 /mnt
 $ cd /mnt && ls
```

看了一下自己的 `home` 目录可以被正常挂载到，且目录的内容是正常的，基本排除掉是子卷本身的问题，应该是 systemd 在挂载的时候出现了问题。根据前文所述，我们来查看 `/etc/fstab` 文件

```
$ cat /etc/fstab
# Static information about the filesystems.
# See fstab(5) for details.

# <file system> <dir> <type> <options> <dump> <pass>
# /dev/nvme0n1p2
UUID=a4d7523c-f215-4661-96c5-30ac6518a101	/         	btrfs     	rw,noatime,compress=lzo,ssd,space_cache,subvolid=256,subvol=/@	0 0

# /dev/nvme0n1p1
UUID=A0FC-3CBD      	/boot     	vfat      	rw,relatime,fmask=0022,dmask=0022,codepage=437,iocharset=ascii,shortname=mixed,utf8,errors=remount-ro	0 2

# /dev/nvme0n1p2
UUID=a4d7523c-f215-4661-96c5-30ac6518a101	/home     	btrfs     	rw,noatime,compress=lzo,ssd,space_cache,subvolid=351,subvol=/@home	0 0

# /dev/nvme0n1p2
UUID=a4d7523c-f215-4661-96c5-30ac6518a101	/tmp      	btrfs     	rw,noatime,compress=lzo,ssd,space_cache,subvolid=258,subvol=/@tmp	0 0

# /dev/nvme0n1p2
UUID=a4d7523c-f215-4661-96c5-30ac6518a101	/snapshots	btrfs     	rw,noatime,compress=lzo,ssd,space_cache,subvolid=260,subvol=/@snapshots	0 0

# /dev/nvme0n1p2
UUID=a4d7523c-f215-4661-96c5-30ac6518a101	/var      	btrfs     	rw,relatime,compress=lzo,ssd,space_cache,subvolid=261,subvol=/@var	0 0
```

找到 `/home` 这一行，发现这个 `subvolid=351` 的子卷的名称不是 `/@home`而是 `timeshift-btrfs/snapshots/2021-12-07_13-59-01/@home` 。这里两个配置是冲突的，为了寻找哪个是需要被正确挂载的 `home` ，需要手动挂载一下这里面的备份，找到正确的之后，改 `subvolid` 就好了。对于我来说，将 `subvolid` 改成当前 `@home` 的 358 就 OK 了。另外  `subvol` 和 `subvolid` 应该只写一项其实就可以了

### Reference

- [bootup - System bootup process](https://man7.org/linux/man-pages/man7/bootup.7.html)  
- [The Linux 2.4 Kernel's Startup Procedure](http://coffeenix.net/doc/kernel/startup.html/t1.html)  
- [An introduction to the Linux /etc/fstab file](https://www.redhat.com/sysadmin/etc-fstab)  
- [细说linux挂载——mount，及其他……](https://forum.ubuntu.org.cn/viewtopic.php?t=257333)  
- [Using the initial RAM disk (initrd)](https://github.com/torvalds/linux/blob/2a987e6502/Documentation/admin-guide/initrd.rst)


    