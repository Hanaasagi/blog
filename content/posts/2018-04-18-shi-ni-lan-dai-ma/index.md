
+++
title = "是你！烂代码"
summary = ''
description = ""
categories = []
tags = []
date = 2018-04-18T14:37:08+08:00
draft = false
+++

首先说的是本人水平并不高，此文只是发泄工作一天的不爽而已。代码已经过混淆处理，基于 Python2.7  

1) 某个类中的方法

```Python
@staticmethod
# any a in b
def contained(a, b):
    if not a:
        return True
    if not b:
        return False
    return not (set(a) - set(b))

@staticmethod
# any a not in b
def exclusion(a, b):
    if not a or not b:
       return True
    return not (set(a) & set(b))
```

其一，这个注释能不能不要写在装饰器和被装饰函数中间呢。写在上面或者作为 docsting 都可以啊  
其二，两个相近功能的函数不要函数名一个为动词，一个为名词  
其三，这两个函数完全可以使用 `any` 和 `all` 替换掉  

```Python
def contained(a, b):
    return all(e in b for e in a)

def excluded(a, b):
    return all(e not in b for e in a)
```

2) 没有检查到的代码

```Python
if some_data.get(u'xxx', []).get(u"yyy"):
    ...
```

显然这是错误的。更可气的是这不是说单元测试没有覆盖到，而是这个项目根本没有一行单元测试

3) 这他妈是什么玩意

```Python
cmd_output = os.popen(comand).read()
try:
    xxx_list = json.loads(cmd_output)
except:
    if 'No such file or directory' in cmd_output:
        self.system_log.log_error('Command `%s` not found.' % self.basic_command)
    elif 'requires' in cmd_output:
        self.system_log.log_error('No container.')
    else:
        self.system_log.log_error(u'Unknown error: %s' % cmd_output)
```

且不说 bare except，这个异常里都写了些什么鬼。不考虑机器的语言环境设置，比如中文下应该是 `没有那个文件或目录`。而且 `os.popen` Deprecated since version 2.6

本文与在前公司工作时的吐槽文 [由某次坑爹的测试所引发的感想](https://blog.dreamfever.me/2017/08/21/you-mou-ci-keng-die-de-ce-shi-suo-yin-fa-de-gan-xiang/) 一起食用风味更佳

#### Update 2018/04/21

关于第三条，也许当时没有表述清楚。

其一，`No such file or directory` 和 `Command not found` 貌似没有什么关系  
其二，以 `docker inspect` 命令为例，它支持同时获取多个 container 的信息。假设存在 ID 为 `5d605cde1763` 的 container，不存在一个 ID 为 `2333` 的 container

执行下面的命令

```Bash
$ docker inspect 5d605cde1763 2333
[
    {
        "Id": "5d605cde17632443a3ad2823c424fa169488aab39ae5d41c292cf0de0a9fff0f",
        "Created": "2018-04-08T10:07:16.816638071Z",
        "Path": "/bin/sh",
        .... 省略输出
    }
]
Error: No such object: 2333
$ echo $?
1
```

这其实是 stdout 和 stderr 的混合，而 os.popen 返回的文件对象是绑定的 stdout。换句话说使用 `os.popen` 我们将丢失 stderr 的信息，它直接输出到终端了

当然也不是没有办法，可以去改一下执行的 shell 命令，将 stderr 定向到 stdout。比如 `output = os.popen('cat non-exist-file 2>&1').read()`

其三，个人认为应当在执行 shell 命令时做异常处理，而不是在处理其输出时

另外本人犯了个错误，`os.popen` 在 Python3 中被重写，它是可以被使用的，内部实现借助的是 `subprocess.Popen`

那么应当如何去优雅地执行外部命令呢

```Python
def shell_exec(command, strict_mode=True):
    p = subprocess.Popen(command, shell=True, bufsize=-1,  # use system bufsize
                         stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    out, err = p.communicate()
    if strict_mode:
        if p.returncode == 0:
            return out
        raise CommandExecError(err)
    return out, err

# 对于同时想获得 stdout 中有效数据，又想知道是否存在部分错误时
command = 'docker inspect 5d605cde1763 2333'
out, err = shell_exec(command, False)
if err:
   logger.error('"{}" executed failed: "{}"'.format(command, err))
container_data = json.loads(out)

# 执行一般命令

command = 'lll'
try:
    out = shell_exec(command)
except CommandExecError as e:
    logger.error(e)
```

但是这还是有问题的，`communicate` 是一口气读完数据的，数据量大的时候性能不太好

    