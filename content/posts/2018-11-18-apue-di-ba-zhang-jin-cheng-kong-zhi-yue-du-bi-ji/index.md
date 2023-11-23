
+++
title = "APUE 第八章 [进程控制] 阅读笔记"
summary = ''
description = ""
categories = []
tags = []
date = 2018-11-18T14:32:39+08:00
draft = false
+++

陆续会将前几章节的笔记更新出来...

### `fork(2)`

```
#include <stdio.h>
#include <stdlib.h>
#include <sys/types.h>
#include <unistd.h>
#include <sys/wait.h>


int
main(void)
{
    pid_t pid;

    printf("before fork\n");
    if ((pid = fork()) < 0) {
        // handle error
    } else if (pid == 0) {
        // do nothing
    } else {
        waitpid(pid, NULL , 0);
    }
    exit(0);  // _exit(0);
}
```

运行结果

```Bash
$ ./a.out
before fork
$ ./a.out > temp && cat temp
before fork
before fork
```

默认情况下如果标准输出连到终端设备，那么标准 I/O 是缓冲的；否则它是全缓冲的。`fork(2)` 调用后产生子进程，其运行在独立的内存空间，但拥有和父进程相同的内存数据的副本，这包含了标准 I/O 所使用的缓冲区。另外 `exit(3)` 调用会 flush 并且关闭所有的流。所以当以交互方式运行此程序时，`before fork` 仅会输出一次(父进程输出)，但是重定向到文件后，父子进程都会输出

如果我们这里将 `exit(3)` 替换为 `_exit(2)` 那么运行结果会变为

```Bash
$ ./a.out
before fork
$ ./a.out > temp && cat temp
```

其原因是 `_exit(2)` 并不会 flush 标准 I/O 流，导致这部分的数据被丢弃

> The function `_exit(2)` is like `exit(3)`, but does not call any functions registered with `atexit(3)` or `on_exit(3)`. Open `stdio(3) `streams are not flushed.  On the other hand, `_exit()` does  close  open file descriptors, and this may cause an unknown delay, waiting for pending output to finish.  If the delay is undesired, it may be useful to call functions like `tcflush(3)` before calling `_exit(2)`.  Whether any pending I/O is canceled, and which pending I/O may be canceled upon `_exit(2)`, is implementation-dependent.


### `vfork(2)`

区别于 `fork(2)`，在子进程调用 `exec` 或者 `exit` 之前父进程会被阻塞调度。另外它在父进程的空间中运行，所以如果子进程修改数据会影响到父进程，比如 关闭某个流 `flcose(2);`

### `execve(2)` 与解释器文件

`exec(3)` 函数族用于替换当前进程的正文段、数据段、堆栈，来执行新的程序，它们最终都要调用 `execve(2)`

```
// test_exec.c
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>

int
main(int argc, char *argv[])
{
    char *newargv[] = { NULL, "hello", "world", NULL };
    char *newenviron[] = { NULL };

    newargv[0] = argv[1];  // NOTE

    execve(argv[1], newargv, newenviron);
}
```

```
// show_args.c
#include <stdio.h>
#include <stdlib.h>

int
main(int argc, char *argv[])
{
   for (int i = 0; i < argc; i++) {
       printf("argv[%d]: %s\n", i, argv[i]);
   }
}
```

```
$ gcc test_exec.c -o test_exec && gcc show_args.c -o show_args && ./test_exec show_args
argv[0]: show_args
argv[1]: hello
argv[2]: world
```

可以看到 `show_args` 接收的参数便是 `test_exec` 通过 `execve` 参数传入的那个数组。通常而言第一个参数是新的可执行程序的文件名，但这只是惯例，可以传入任何的参数，上例中便使用的是文件路径

---

UNIX 系统支持解释器文件，此时执行的是该解释器文件中第一行中所指定的文件


> An interpreter script is a text file that has execute permission enabled and whose first line is of the form:

    #! interpreter [optional-arg]

> The interpreter must be a valid pathname for an executable file.  If the filename argument of `execve()` specifies an interpreter script, then interpreter will be invoked with the following arguments:

    interpreter [optional-arg] filename arg...

> where arg... is the series of words pointed to by the argv argument of `execve()`, starting at `argv[1]`.

需要注意 optional-arg 在 Linux 上值得是 interpreter 后的整个字符串，可以包含空格

> On Linux, the entire string following the interpreter name is passed as a single argument to the interpreter, and this  string can include white space.

```Bash
$ cat s
#! ./show_args test
```

此时输出变为

```Bash
$ ./test_exec ./s
argv[0]: ./show_args
argv[1]: test
argv[2]: ./s
argv[3]: hello
argv[4]: world
```

跟踪系统调用

```
execve("./test_exec", ["./test_exec", "./s"], 0x7fff8c8c9db0 /* 25 vars */) = 0
execve("./s", ["./s", "hello", "world"], 0x7ffe0a6d1108 /* 0 vars */) = 0
```

当执行 `./s` 时，其实是执行的 `./show_args test ./s hello world`。执行 `s` 脚本中的 `./show_args`，并不会产生 `fork(2)`


如果执行未添加 shebang 的可执行脚本，会多调用一次 `execve(2)`

比如 `a.sh` 中的内容如下

```
sleep 2
```

打开一个终端，获取 shell 进程的 PID

```
$ echo $$
8606
```

在另一个终端通过 `strace` 来跟踪 是 shell 进程的系统调用，因为在 shell 中执行命令都会 `fork(2)` 出一个新的进程，所以需要添加 `-f` 参数

```
$ strace -f -p 8606
Process 8606 attached
read(0, "./a.sh\n", 8192)               = 7
clone(Process 8733 attached
child_stack=0, flags=CLONE_CHILD_CLEARTID|CLONE_CHILD_SETTID|SIGCHLD, child_tidptr=0x7f68f14339d0) = 8733
[pid  8606] wait4(-1,  <unfinished ...>
[pid  8733] execve("./a.sh", ["./a.sh"], [/* 55 vars */]) = -1 ENOEXEC (Exec format error)
[pid  8733] execve("/bin/sh", ["/bin/sh", "./a.sh"], [/* 55 vars */]) = 0
[pid  8733] clone(child_stack=0, flags=CLONE_CHILD_CLEARTID|CLONE_CHILD_SETTID|SIGCHLD, child_tidptr=0x7f2bdfca99d0) = 8734
Process 8734 attached
[pid  8733] wait4(-1,  <unfinished ...>
[pid  8734] execve("/bin/sleep", ["sleep", "2"], [/* 55 vars */]) = 0
[pid  8734] nanosleep({2, 0}, NULL)     = 0
[pid  8734] exit_group(0)               = ?
[pid  8734] +++ exited with 0 +++
[pid  8733] <... wait4 resumed> [{WIFEXITED(s) && WEXITSTATUS(s) == 0}], 0, NULL) = 8734
[pid  8733] --- SIGCHLD {si_signo=SIGCHLD, si_code=CLD_EXITED, si_pid=8734, si_uid=1000, si_status=0, si_utime=0, si_stime=0} ---
[pid  8733] read(10, "", 8192)          = 0
[pid  8733] exit_group(0)               = ?
[pid  8733] +++ exited with 0 +++
<... wait4 resumed> [{WIFEXITED(s) && WEXITSTATUS(s) == 0}], WSTOPPED, NULL) = 8733
--- SIGCHLD {si_signo=SIGCHLD, si_code=CLD_EXITED, si_pid=8733, si_uid=1000, si_status=0, si_utime=0, si_stime=0} ---
rt_sigreturn()                          = 8733
wait4(-1, 0x7ffdda2b36dc, WNOHANG|WSTOPPED, NULL) = -1 ECHILD (No child processes)
write(2, "$ ", 2)                       = 2
```

我们可以观察到，shell进程(`/bin/sh`)进行首先 `fork(2)` 了一个新进程，然后通过 `execve(2)` 去执行 `./a.sh`。但是这并不是一个合法的可执行文件(`ENOEXEC`)。于是它又尝试将其作为 `/bin/sh` 的参数来执行。此时 `a.sh` 中的每一条命令都会 `fork(2)` 新的进程执行

### Reference

[UNIX环境高级编程(3rd)](https://book.douban.com/subject/25900403/)

    