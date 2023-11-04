+++
title = '新人也能懂的调试方法 02 - ptrace 应用'
summary = ""
description = ""
categories = ["debug", "ptrace"]
tags = []
date = 2023-11-03T18:43:09+09:00
draft = false

+++

## 序言

在上一篇章中介绍了通过 `strace` 来追踪应用程序的 syscall。这一篇章打算讲一下其背后的 `ptrace` ，及一些特殊的玩法

本文编写及调试所用的环境是 

- Linux 6.5.9-arch2-1 x86_64 unknown
- zig 0.12.0-dev.1297+a9e66ed73



## Syscall

下面是一个简单的 syscall。`syscall0` 意味着它是一个没有参数的 syscall；`syscall1` 则是一个含有一个参数的 `syscall`

```zig
// https://github.com/ziglang/zig/blob/94cee4fb27a433824c2540dc37375dc14befdf47/lib/std/os/linux/x86_64.zig#L18
pub fn syscall0(number: SYS) usize {
    return asm volatile ("syscall"
        : [ret] "={rax}" (-> usize),
        : [number] "{rax}" (@intFromEnum(number)),
        : "rcx", "r11", "memory"
    );
}

pub fn syscall1(number: SYS, arg1: usize) usize {
    return asm volatile ("syscall"
        : [ret] "={rax}" (-> usize),
        : [number] "{rax}" (@intFromEnum(number)),
          [arg1] "{rdi}" (arg1),
        : "rcx", "r11", "memory"
    );
}
```

- `rdi` 用作第一个参数
- `rax` 用作返回值和 syscall 的编号，后面还有一个 `orig_rax` 里面一定是 syscall nr 的值



这个其实和普通的函数调用/FFI 之类的类似

1. 参数/返回值约定
2. 函数地址
3. `call`



比如一个很简单的 0 参数 syscall  `getpid` 

```zig
const std = @import("std");

pub fn main() !void {
    // getpid syscall number
    const number = 0x27;

    const ret = asm volatile ("syscall"
        : [ret] "={rax}" (-> usize),
        : [number] "{rax}" (number),
        : "rcx", "r11", "memory"
    );

    std.log.info("syscall => {d}", .{ret});
    std.log.info("std.os.linux.getpid => {d}", .{std.os.linux.getpid()});
}
```

*P.S. 这里 `ret` 和 `number` 可以合并成一个变量，少 8 字节的空间占用*



在 Kernel 中找对应的 syscall 可以通过

1. `sys_xxx` ，比如 `sys_getpid` 这个是一种统一的命名规范
2. `SYSCALL_DEFINE0` 这个相当于我们在用户态中定一个 `syscall0` ，这样可以 grep 出来再进一步进行筛选
3. 在相关的模块下查找，比如 `getpid` 应该在 sys.c 文件中，而 `kill` 应该在 singal.c 中。这个可以根据你 C 代码中的头文件来猜测的

```c
// https://elixir.bootlin.com/linux/v6.6/source/kernel/sys.c#L958

/**
 * sys_getpid - return the thread group id of the current process
 *
 * Note, despite the name, this returns the tgid not the pid.  The tgid and
 * the pid are identical unless CLONE_THREAD was specified on clone() in
 * which case the tgid is the same in all threads of the same group.
 *
 * This is SMP safe as current->tgid does not change.
 */
SYSCALL_DEFINE0(getpid)
{
	return task_tgid_vnr(current);
}

```



## Ptrace 基础

`ptrace` 系统调用提供了一种方式，其中一个进程（“跟踪器”）可以观察和控制另一个进程（“被跟踪者”）的执行，并检查和更改被跟踪者的内存和寄存器。它主要用于实现断点调试和系统调用跟踪。文档在 `man 2 ptrace` 中，这里只选取重要的部分来讲。`ptrace` 的函数签名如下

```c
SYNOPSIS
       #include <sys/ptrace.h>

       long ptrace(enum __ptrace_request request, pid_t pid,
                   void *addr, void *data);
```

`request` 参数的值可以为以下的:

- `PTRACE_TRACEME`: 表明此进程将由其父进程跟踪
- `PTRACE_PEEKTEXT`, `PTRACE_PEEKDATA`: 从被跟踪者内存中的地址 addr 读取一个字，并将其作为调用的结果返回
- `PTRACE_PEEKUSER` 从被跟踪者的 USER 区域中的偏移地址 addr 处读取一个字
- `PTRACE_POKETEXT`, `PTRACE_POKEDATA`, `PTRACE_POKEUSER`: 与 `PTRACE_PEEKTEXT` 等成对的写入 API
- `PTRACE_GETREGS`, `PTRACE_GETFPREGS`: 将被跟踪者的通用寄存器或浮点寄存器复制到跟踪器中的指定地址
- `PTRACE_GETSIGINFO`: 获取导致进程停止的信号的信息
- `PTRACE_CONT`: 重新启动已停止的被跟踪进程。如果data不为零，则解释为要传递给被跟踪者的信号编号。否则，不传递信号
- `PTRACE_SYSCALL`, `PTRACE_SINGLESTEP`: 重新启动已停止的被跟踪进程，与 `PTRACE_CONT` 类似，但安排被跟踪者在进入或退出系统调用时，或在执行单个指令后停止
- `PTRACE_ATTACH`: 附加到pid中指定的进程，使其成为调用进程的被跟踪者



Q1: 如何判断自己是否被别人 trace 了

```
>> cat /proc/<PID>/status
Name:   python
Umask:  0022
State:  S (sleeping)
Tgid:   323328
Ngid:   0
Pid:    323328
PPid:   3503
TracerPid:  327488
```

这里有一个 `TracerPid` 的字段，如果是 0 那么没有被 trace



Q2: 可以有两个 tracer 么

不可以，第二个进行 ptrace 的进程会得到 `PermissionDenied` 错误



Q3: 调试进程会变成被追踪进程的父进程么

这个是不会的，但是需要解释一下这个谣传是怎么来的。Linux 的 process 对应的 `task_struct` 是包含了两个 `parent` 字段的

```c
// https://elixir.bootlin.com/linux/v6.6/source/include/linux/sched.h#L975

	/* Real parent process: */
	struct task_struct __rcu	*real_parent;

	/* Recipient of SIGCHLD, wait4() reports: */
	struct task_struct __rcu	*parent;

```

在 `ptrace` 的时候的确改了目标进程的 `parent`

```c
// https://elixir.bootlin.com/linux/v6.6/source/kernel/ptrace.c#L69
void __ptrace_link(struct task_struct *child, struct task_struct *new_parent,
		   const struct cred *ptracer_cred)
{
	BUG_ON(!list_empty(&child->ptrace_entry));
	list_add(&child->ptrace_entry, &new_parent->ptraced);
	child->parent = new_parent;
	child->ptracer_cred = get_cred(ptracer_cred);
}
```

但是如果我们是获取进程关系，比如 `getppid(2)` 这种，那么是通过的 `current->real_parent`

```c
// https://elixir.bootlin.com/linux/v6.6/source/kernel/sys.c#L975
SYSCALL_DEFINE0(getppid)
{
	int pid;

	rcu_read_lock();
	pid = task_tgid_vnr(rcu_dereference(current->real_parent));
	rcu_read_unlock();

	return pid;
}
```



关于 `ptrace` 的实现可以参考

- https://elixir.bootlin.com/linux/v6.6/source/kernel/ptrace.c#L1278

- https://elixir.bootlin.com/linux/v6.6/source/arch/x86/kernel/ptrace.c#L730



## Ptrace 实践

### 通过 ptrace 获取 syscall NR

思路:

1. `PTRACE_ATTACH` 对进程进行跟踪
2. `PTRACE_PEEKUSER` 读取指定寄存器中的值，比如 `RAX` 中存放的 syscall nr
3. `PTRACE_SYSCALL` 恢复被跟踪的进程



代码如下

```zig
const std = @import("std");

// In Zig, currently don't have this variable from sys/reg.h, so I'm hard-coding it.
// https://elixir.bootlin.com/linux/v6.6/source/arch/x86/include/asm/user_64.h#L69
const ORIG_RAX = 15;

pub fn main() !void {
    var iter = std.process.args();
    _ = iter.skip();
    var pid = try std.fmt.parseInt(
        std.os.pid_t,
        iter.next() orelse {
            std.log.err("No pid given", .{});
            std.os.exit(1);
        },
        10,
    );

    // Attach the target process
    try std.os.ptrace(std.os.linux.PTRACE.ATTACH, pid, 0, 0);
    std.log.info("Tracing process {d}...\n", .{pid});

    var ret: usize = undefined;
    while (true) {
        var status: u32 = undefined;
        ret = std.os.linux.wait4(pid, &status, 0, null);
        if (std.os.errno(ret) != .SUCCESS) {
            std.log.err("Error: wait4 => {d}", .{ret});
        }

        // Get the syscall number
        var syscall_nr: i32 = undefined;
        // 64-bit registers, offset 15 =>  (64 / 8 * 15)
        ret = std.os.linux.ptrace(
            std.os.linux.PTRACE.PEEKUSER,
            pid,
            8 * ORIG_RAX,
            @intFromPtr(&syscall_nr),
            0,
        );
        if (std.os.errno(ret) != .SUCCESS) {
            std.log.err("Error: PTRACE_PEEKUSER => {d}", .{ret});
        }

        // convert the syscall number to name string
        const syscall_name = @tagName(@as(std.os.linux.SYS, @enumFromInt(syscall_nr)));
        std.log.info("Syscall: {s}({d})", .{ syscall_name, syscall_nr });

        // restart the stopped tracee
        ret = std.os.linux.ptrace(std.os.linux.PTRACE.SYSCALL, pid, 0, 0, 0);
        if (std.os.errno(ret) != .SUCCESS) {
            std.log.err("Error: PTRACE_SYSCALL => {d}", .{ret});
        }
    }
}

```



另外对于 Linux 5.3 之后是直接可以通过 `PTRACE_GET_SYSCALL_INFO` 这个获取 syscall 信息的



### 对于 write(2) 进行截获并修改

首先看一下 write(2) 这个 syscall 的参数

| NR   | syscall name | references                                                   | %rax | arg0 (%rdi)     | arg1 (%rsi)     | arg2 (%rdx)  | arg3 (%r10) | arg4 (%r8) | arg5 (%r9) |
| ---- | ------------ | ------------------------------------------------------------ | ---- | --------------- | --------------- | ------------ | ----------- | ---------- | ---------- |
| 1    | write        | [man/](https://man7.org/linux/man-pages/man2/write.2.html) [cs/](https://source.chromium.org/search?ss=chromiumos&q=SYSCALL_DEFINE.*write) | 0x01 | unsigned int fd | const char *buf | size_t count | -           | -          | -          |

- `rdi`: fd 的值
- `rsi`: 写入数据的指针
- `rdx`: 写入的长度



对于寄存器的读取依旧可以使用 `PTRACE_PEEKUSER` ，但是这里涉及到了多个寄存器的值，会产生多次调用。我们不如使用更方便的 `PTRACE_GETREGS` 来一次性读取



思路:

1. `PTRACE_GETREGS` 获取寄存器的值
2. 根据 `rsi` 寄存器中的地址寻址，注意这里要通过 `PTRACE_PEEKDATA` 来通过 kernel 在被追踪的进程的空间里面寻址；而不是在你的进程里面寻址
3. `rdx` 里面是数据的长度，可以结合 `rsi` 然后每次 8 字节，读取所有的数据
4. `PTRACE_POKEDATA` 可以让我们修改被追踪的进程空间里面的值，比如将 `buf` 中的字符串



完整代码如下

```zig
const std = @import("std");

// https://elixir.bootlin.com/linux/v6.6/source/arch/x86/include/asm/user_64.h#L69
const user_regs_struct = struct {
    r15: c_ulong,
    r14: c_ulong,
    r13: c_ulong,
    r12: c_ulong,
    rbp: c_ulong,
    rbx: c_ulong,
    r11: c_ulong,
    r10: c_ulong,
    r9: c_ulong,
    r8: c_ulong,
    rax: c_ulong,
    rcx: c_ulong,
    rdx: c_ulong,
    rsi: c_ulong,
    rdi: c_ulong,
    orig_rax: c_ulong,
    rip: c_ulong,
    cs: c_ulong,
    eflagsr: c_ulong,
    rsp: c_ulong,
    ss: c_ulong,
    fs_base: c_ulong,
    gs_base: c_ulong,
    ds: c_ulong,
    es: c_ulong,
    fs: c_ulong,
    gs: c_ulong,
};

pub fn main() !void {
    var iter = std.process.args();
    _ = iter.skip();
    var pid = try std.fmt.parseInt(
        std.os.pid_t,
        iter.next() orelse {
            std.log.err("No pid given", .{});
            std.os.exit(1);
        },
        10,
    );

    // Attach the target process
    try std.os.ptrace(std.os.linux.PTRACE.ATTACH, pid, 0, 0);
    std.log.info("Tracing process {d}...\n", .{pid});

    var ret: usize = undefined;
    while (true) {
        var status: u32 = undefined;
        ret = std.os.linux.wait4(pid, &status, 0, null);
        if (std.os.errno(ret) != .SUCCESS) {
            std.log.err("Error: wait4 => {d}", .{ret});
        }

        var regs: user_regs_struct = undefined;
        // Read the registers
        ret = std.os.linux.ptrace(std.os.linux.PTRACE.GETREGS, pid, 0, @intFromPtr(&regs), 0);
        if (std.os.errno(ret) != .SUCCESS) {
            std.log.err("Error: PTRACE_GETREGS => {d}", .{ret});
        }

        // orig_rax stores the syscall nr
        const syscall = @as(std.os.linux.SYS, @enumFromInt(regs.orig_rax));
        if (syscall == std.os.linux.SYS.write) {
            std.log.info("Register: %rdi={d}, %rsi={d}, %rdx={d}", .{ regs.rdi, regs.rsi, regs.rdx });

            // - `rdi`: f
            // - `rsi`: *buffer
            // - `rdx`: length
            const addr: usize = @bitCast(regs.rsi);
            var data: usize = undefined;
            ret = std.os.linux.ptrace(std.os.linux.PTRACE.PEEKDATA, pid, addr, @intFromPtr(&data), 0);
            if (std.os.errno(ret) != .SUCCESS) {
                std.log.err("Error: PTRACE_PEEKDATA => {d}", .{ret});
            }

            std.log.info("First 8 bytes in buf: {s}", .{@as([*:8]u8, @ptrCast(&data))[0..8]});

            // let's chagne this
            const new_data = "cyber wo";
            ret = std.os.linux.ptrace(
                std.os.linux.PTRACE.POKEDATA,
                pid,
                addr,
                std.mem.readIntSlice(usize, new_data, .Little),
                0,
            );
            if (std.os.errno(ret) != .SUCCESS) {
                std.log.err("Error: PTRACE_POKEDATA=> {d}", .{ret});
            }
        }

        // restart the stopped tracee
        ret = std.os.linux.ptrace(std.os.linux.PTRACE.SYSCALL, pid, 0, 0, 0);
        if (std.os.errno(ret) != .SUCCESS) {
            std.log.err("Error: PTRACE_SYSCALL => {d}", .{ret});
        }
    }
}

```



观察这个程序的输出，我们可以发现只有第一次读取的 `buf` 中的值为 `hello wo` ，之后都是 `cyber wo` 了。这是因为哦我们直接修改了对方进程中的值。只要对方进程不对这个 `buf` 再次修改，那么依然还是 `cyber wo`

```
info: Tracing process 410363...

info: Register: %rdi=3, %rsi=139623659102112, %rdx=11
info: First 8 bytes in buf: hello wo
info: Register: %rdi=3, %rsi=139623659102112, %rdx=11
info: First 8 bytes in buf: cyber wo
info: Register: %rdi=3, %rsi=139623659102112, %rdx=11
info: First 8 bytes in buf: cyber wo
```



### Shellcode 注入

在上面的例子中，我们实践了对于进程 syscall 的截获和内存空间的修改。那么我们按照这个思路可以做一些更有意思的事情

#### 编写 shellcode

以一个 `/bin/ls` 来举例子。既然需要执行一个 command，那么我们可以通过 `execve` 这个 syscall 来做。

| NR   | syscall name | references                                                   | %rax | arg0 (%rdi)          | arg1 (%rsi)             | arg2 (%rdx)             | arg3 (%r10) | arg4 (%r8) | arg5 (%r9) |
| ---- | ------------ | ------------------------------------------------------------ | ---- | -------------------- | ----------------------- | ----------------------- | ----------- | ---------- | ---------- |
| 59   | execve       | [man/](https://man7.org/linux/man-pages/man2/execve.2.html) [cs/](https://source.chromium.org/search?ss=chromiumos&q=SYSCALL_DEFINE.*execve) | 0x3b | const char *filename | const char *const *argv | const char *const *envp | -           | -          | -          |

- `rax` 寄存器需要是 filename，比如  `/bin/ls` 这种 command 所在的路径。`/bin//ls` 这里正好是一个  8 长度的字符串，处理起来方便。而且 Linux 下路径中重复的 `//` 也是可以正常识别的
- `rsi` 寄存器是 command  的参数，这里用 `NULL` 就好了
- `rdx` 寄存器是环境变量参数，这里也用 `NULL` 就好了

注意以上的寄存器存储的是指针，而不是一个值自身。所以我们需要借助一段内存空间来存储这些值，然后把地址塞到寄存器中。最简单的就是利用栈空间了



下一步就是手写汇编文件了

```assembly
SECTION .data
SECTION .text
    global main

main:
    ; we need a NULL
    xor rdx, rdx  ; reset rdx to 0
    push rdx; ; c string, null terminated

    ; handle argv
    mov rsi, rsp  ;  get the address of NULL and store it in the rsi.

    ; handle envp
    mov rdx, rsp  ;  get the address of NULL and store it in the rdx.

    ; handle filename
    mov rax, 0x736c2f2f6e69622f  ; rax = "/bin//ls"
    push rax  ; push rax to stack
    mov rdi, rsp  ; get the address of "/bin//ls" and store it in the rdi.

    ; prepare syscall number
    xor rax, rax  ; reset rax to 0
    mov al, 0x3b  ; execve syscall number, store it in the rax
    syscall

```

可以编译后测试一下，如果执行起来没有 segmentfault 等问题，那么是 OK 的

```
>> nasm -f elf64 -g ls.asm
>> gcc -g ls.o -o a.out  # test shell code
```

通过 `objdump` 命令，我们可以获取这些汇编对应的 Hex

```
>> objdump -d ls.o

ls.o:     file format elf64-x86-64


Disassembly of section .text:

0000000000000000 <main>:
   0:   48 31 d2                xor    %rdx,%rdx
   3:   52                      push   %rdx
   4:   48 89 e6                mov    %rsp,%rsi
   7:   48 89 e2                mov    %rsp,%rdx
   a:   48 b8 2f 62 69 6e 2f    movabs $0x736c2f2f6e69622f,%rax
  11:   2f 6c 73
  14:   50                      push   %rax
  15:   48 89 e7                mov    %rsp,%rdi
  18:   48 31 c0                xor    %rax,%rax
  1b:   b0 3b                   mov    $0x3b,%al
  1d:   0f 05                   syscall

```

把这些 Hex 抠出来，就是我们之后需要用到的 shellcode

```
"\x48\x31\xd2\x52\x48\x89\xe6\x48\x89\xe2\x48\xb8\x2f\x62\x69\x6e\x2f\x2f\x6c\x73\x50\x48\x89\xe7\x48\x31\xc0\xb0\x3b\x0f\x05"
```

P.S. 这个应该是有点长了，我记得祖传的 shellcode 应该是 27 字节的，汇编部分的代码还可以优化一下减少 shellcode 的长度。



#### 通过 ptrace 注入

思路:

1. 将 shellcode 写入进程空间
2. `PTRACE_SETREGS` 来修改 `rip` 的值，执行 shellcode



首先因为 Linux 的 ASLR，我们必须动态的去寻找一段空间的地址，这个可以读取 `/proc/<PID>/maps` 获得进程的详细信息

```
>> cat /proc/410363/maps
55d8281bb000-55d8281bc000 r--p 00000000 00:19 8603878                    /usr/bin/python3.11
55d8281bc000-55d8281bd000 r-xp 00001000 00:19 8603878                    /usr/bin/python3.11
55d8281bd000-55d8281be000 r--p 00002000 00:19 8603878                    /usr/bin/python3.11
55d8281be000-55d8281bf000 r--p 00002000 00:19 8603878                    /usr/bin/python3.11
55d8281bf000-55d8281c0000 rw-p 00003000 00:19 8603878                    /usr/bin/python3.11
55d828c05000-55d828d26000 rw-p 00000000 00:00 0                          [heap]
7efcaa900000-7efcaaa00000 rw-p 00000000 00:00 0
```

第二个点就是这个空间需要有 `x` 的权限，否则写入后也无法执行。对于 ptrace 来说，这里是可以直接忽略掉对方地址空间的 `w` 的权限，所以我们需要寻找一段权限为 `r-xp` 的空间



完整代码如下

```zig
const std = @import("std");

// https://elixir.bootlin.com/linux/v6.6/source/arch/x86/include/asm/user_64.h#L69
const user_regs_struct = struct {
    r15: c_ulong,
    r14: c_ulong,
    r13: c_ulong,
    r12: c_ulong,
    rbp: c_ulong,
    rbx: c_ulong,
    r11: c_ulong,
    r10: c_ulong,
    r9: c_ulong,
    r8: c_ulong,
    rax: c_ulong,
    rcx: c_ulong,
    rdx: c_ulong,
    rsi: c_ulong,
    rdi: c_ulong,
    orig_rax: c_ulong,
    rip: c_ulong,
    cs: c_ulong,
    eflagsr: c_ulong,
    rsp: c_ulong,
    ss: c_ulong,
    fs_base: c_ulong,
    gs_base: c_ulong,
    ds: c_ulong,
    es: c_ulong,
    fs: c_ulong,
    gs: c_ulong,
};

pub fn get_address(pid: std.os.linux.pid_t) !usize {
    var path_buf: [64]u8 = undefined;
    const path = try std.fmt.bufPrint(&path_buf, "/proc/{d}/maps", .{pid});
    const file = try std.fs.cwd().openFile(path, .{});
    defer file.close();

    var buf_reader = std.io.bufferedReader(file.reader());
    var in_stream = buf_reader.reader();
    var buf: [1024]u8 = undefined;

    while (try in_stream.readUntilDelimiterOrEof(&buf, '\n')) |line| {
        //0021d000-0029d000 r-xp 0001c000 00:19 23142607                           /home/kasumi/zig/ztrace/zig-out/bin/ztrace
        var iter = std.mem.split(u8, line, " ");
        const mem_range = iter.next().?;
        const permission = iter.next().?;
        if (std.mem.eql(u8, permission, "r-xp")) {
            iter = std.mem.split(u8, mem_range, "-");
            return std.fmt.parseInt(usize, iter.next().?, 16);
        }
    }

    @panic("Address not found");
}

pub fn main() !void {
    var iter = std.process.args();
    _ = iter.skip();
    var pid = try std.fmt.parseInt(
        std.os.pid_t,
        iter.next() orelse {
            std.log.err("No pid given", .{});
            std.os.exit(1);
        },
        10,
    );

    try std.os.ptrace(std.os.linux.PTRACE.ATTACH, pid, 0, 0);
    std.log.info("Tracing process {d}...\n", .{pid});

    var ret: usize = undefined;
    var status: u32 = undefined;
    ret = std.os.linux.wait4(pid, &status, 0, null);

    ret = std.os.linux.ptrace(std.os.linux.PTRACE.SYSCALL, pid, 0, 0, 0);
    ret = std.os.linux.wait4(pid, &status, 0, null);

    var regs: user_regs_struct = undefined;
    _ = std.os.linux.ptrace(std.os.linux.PTRACE.GETREGS, pid, 0, @intFromPtr(&regs), 0);

    const addr: usize = try get_address(pid);
    std.log.info("Found memory address {x}", .{addr});
    const shell_code = "\x48\x31\xd2\x52\x48\x89\xe6\x48\x89\xe2\x48\xb8\x2f\x62\x69\x6e\x2f\x2f\x6c\x73\x50\x48\x89\xe7\x48\x31\xc0\xb0\x3b\x0f\x05";

    var i: usize = 0;
    var j: usize = shell_code.len / 8;
    while (i < j) : (i += 1) {
        ret = std.os.linux.ptrace(
            std.os.linux.PTRACE.POKEDATA,
            pid,
            addr + i * 8,
            std.mem.readIntSlice(usize, shell_code[(i * 8) .. (i + 1) * 8], .Little),
            0,
        );
        if (std.os.errno(ret) != .SUCCESS) {
            std.log.err("Error: PTRACE_POKEDATA => {d}", .{ret});
        }
    }
    j = shell_code.len % 8;
    if (j != 0) {
        var tmp: [8]u8 = .{0} ** 8;
        @memcpy(tmp[0..j], shell_code[i * 8 ..]);
        ret = std.os.linux.ptrace(
            std.os.linux.PTRACE.POKETEXT,
            pid,
            addr + i * 8,
            std.mem.readIntSlice(usize, &tmp, .Little),
            0,
        );
        if (std.os.errno(ret) != .SUCCESS) {
            std.log.err("Error: PTRACE_POKEDATA => {d}", .{ret});
        }
    }

    regs.rip = addr;
    _ = std.os.linux.ptrace(std.os.linux.PTRACE.SETREGS, pid, 0, @intFromPtr(&regs), 0);
    std.log.info("Jump to shell code", .{});

    ret = std.os.linux.ptrace(std.os.linux.PTRACE.CONT, pid, 0, 0, 0);
    if (std.os.errno(ret) != .SUCCESS) {
        std.log.err("Error: PTRACE_CONT => {d}", .{ret});
    }
    std.log.info("Continue", .{});
}

```



基本上还是使用的之前提到过的知识。不过这里需要提一下为什么我在 `PTRACE_ATTACH` 之后调用了 `PTRACE_SYSCALL`。比如被跟踪的目标进程是下面的代码

```python
import os
import time

print(os.getpid())
print("sleeping")
time.sleep(10)

print("Now I'm busy")
while True:
    continue
```

这段代码分为两个部分，一个是 `sleep` 的 syscall 这个会切换进程的状态，从 CPU 拿下来。另一个是一个 busy loop



在注释掉上面代码中的下面几行后

```
    // ret = std.os.linux.ptrace(std.os.linux.PTRACE.SYSCALL, pid, 0, 0, 0);
    // ret = std.os.linux.wait4(pid, &status, 0, null);
```

在目前进程 sleep 的时候，如果进行 ptrace 注入，那么会触发 segmentation fault；但是在 busy loop 的时候可以大概率成功注入。这个是因为在上下文切换的时候，环境会被保存。然后我们在这个期间进行注入，进程在一轮上下文切换回来之后，一些变量会被重新覆盖的

加了上面两个行会等待当前的 syscall 完成，就执行结果来看，`ls` 的输出是在 `sleep` 结束之后



## 结束

本章主要还是 `ptrace` 的一些使用上的例子，有了这些我想不难明白 `lldb` 或者 `gdb` 这些单步调试工具是如何运作的。想要进一步学习，可以参考一些调试器的实现 

- [实现一个简单的调试器](https://paper.seebug.org/2051/)

