
+++
title = "write in Python"
summary = ''
description = ""
categories = []
tags = []
date = 2019-05-03T11:24:24+08:00
draft = false
+++

本文研究 Python [PEP-3116 New I/O](https://www.python.org/dev/peps/pep-3116/) 引入后，Python 层面的 `write` 和 syscall `write` 之间的关系。文中所使用的 Python 版本为 3.7.2，commit sha 为 `9a3ffc0492d1310ead9ce8f5ee678c26b20a338d`

### Binary Mode

当我们以 `wb` 模式(比如`open('test.log', 'wb')`)去打开一个文件的时候，返回的是一个 `_io.BufferedWriter` 对象，关于 buffer 的大小可以通过 `buffering` 参数去指定。关于 `buffering` 参数可以参考文档

>buffering is an optional integer used to set the buffering policy.
Pass 0 to switch buffering off (only allowed in binary mode), 1 to select
line buffering (only usable in text mode), and an integer > 1 to indicate
the size of a fixed-size chunk buffer.  When no buffering argument is
given, the default buffering policy works as follows:

>* Binary files are buffered in fixed-size chunks; the size of the buffer
  is chosen using a heuristic trying to determine the underlying device's
  "block size" and falling back on `io.DEFAULT_BUFFER_SIZE`.
  On many systems, the buffer will typically be 4096 or 8192 bytes long.

>* "Interactive" text files (files for which isatty() returns True)
  use line buffering.  Other text files use the policy described above
  for binary files.


因为代码太长了，所以这里仅截取几段逻辑，并且我下面指的 `buffer` 一般是 `self->buffer` 而不是 `buffer` 变量(待写入数据)

第一步，计算当前的可用空间，如果大于要写入的数据量则直接使用 `memcpy` 复制到 `self->buffer`

```C
// https://github.com/python/cpython/blob/9a3ffc0492d1310ead9ce8f5ee678c26b20a338d/Modules/_io/bufferedio.c#L1919
// buffer 变量是要写入的数据
// self->buffer 为内置的 buffer
// self->buffer_size 是初始化时指定的 buffer 大小
// self->pos 记录最后一次写入的位置
avail = Py_SAFE_DOWNCAST(self->buffer_size - self->pos, Py_off_t, Py_ssize_t);
if (buffer->len <= avail) {
    memcpy(self->buffer + self->pos, buffer->buf, buffer->len);
    if (!VALID_WRITE_BUFFER(self) || self->write_pos > self->pos) {
        self->write_pos = self->pos;
    }
    ADJUST_POSITION(self, self->pos + buffer->len);
    if (self->pos > self->write_end)
        self->write_end = self->pos;
    written = buffer->len;
    goto end;
}
```

如果当前可用空间不够，那么先将 buffer 中的数据通过 `write(2)` 写入

```C
res = _bufferedwriter_flush_unlocked(self);
```

可以参考 `_bufferedwriter_flush_unlocked` 的 [实现](https://github.com/python/cpython/blob/9a3ffc0492d1310ead9ce8f5ee678c26b20a338d/Modules/_io/bufferedio.c#L1855)


如果我们在 flush 的时候出错了，那么这里首先会将 `buffer` 中还未被写入的数据移动到 `buffer` 的起始处。然后再次计算可用空间，如果能够容纳代写入的数据量，那么便 OK。否则尽可能的去缓冲数据然后返回错误。

```C
if (res == NULL) {
    Py_ssize_t *w = _buffered_check_blocking_error();
    if (w == NULL)
        goto error;
    if (self->readable)
        _bufferedreader_reset_buf(self);
    /* Make some place by shifting the buffer. */
    assert(VALID_WRITE_BUFFER(self));
    memmove(self->buffer, self->buffer + self->write_pos,
            Py_SAFE_DOWNCAST(self->write_end - self->write_pos,
                             Py_off_t, Py_ssize_t));
    self->write_end -= self->write_pos;
    self->raw_pos -= self->write_pos;
    self->pos -= self->write_pos;
    self->write_pos = 0;
    avail = Py_SAFE_DOWNCAST(self->buffer_size - self->write_end,
                             Py_off_t, Py_ssize_t);
    if (buffer->len <= avail) {
        /* Everything can be buffered */
        PyErr_Clear();
        memcpy(self->buffer + self->write_end, buffer->buf, buffer->len);
        self->write_end += buffer->len;
        self->pos += buffer->len;
        written = buffer->len;
        goto end;
    }
    /* Buffer as much as possible. */
    memcpy(self->buffer + self->write_end, buffer->buf, avail);
    self->write_end += avail;
    self->pos += avail;
    /* XXX Modifying the existing exception e using the pointer w
       will change e.characters_written but not e.args[2].
       Therefore we just replace with a new error. */
    _set_BlockingIOError("write could not complete without blocking",
                         avail);
    goto error;
}
Py_CLEAR(res);
```

如果 `buffer` flush 的过程顺利完成且将要写入的数据比 `buffer` 总空间还要大，那么我们首先尝试能不能将数据尽可能的直接通过 `write(2)` 写入直到剩余的数据小于 `buffer` 大小。如果过程中出错那么尽可能的缓存还未写入的数据然后上传递错误

```C
remaining = buffer->len;
written = 0;
while (remaining > self->buffer_size) {
    Py_ssize_t n = _bufferedwriter_raw_write(
        self, (char *) buffer->buf + written, buffer->len - written);
    if (n == -1) {
        goto error;
    } else if (n == -2) {
        /* Write failed because raw file is non-blocking */
        if (remaining > self->buffer_size) {
            /* Can't buffer everything, still buffer as much as possible */
            memcpy(self->buffer,
                   (char *) buffer->buf + written, self->buffer_size);
            self->raw_pos = 0;
            ADJUST_POSITION(self, self->buffer_size);
            self->write_end = self->buffer_size;
            written += self->buffer_size;
            _set_BlockingIOError("write could not complete without "
                                 "blocking", written);
            goto error;
        }
        PyErr_Clear();
        break;
    }
    written += n;
    remaining -= n;
    /* Partial writes can return successfully when interrupted by a
       signal (see write(2)).  We must run signal handlers before
       blocking another time, possibly indefinitely. */
    if (PyErr_CheckSignals() < 0)
        goto error;
}

if (remaining > 0) {
    memcpy(self->buffer, (char *) buffer->buf + written, remaining);
    written += remaining;
}
```

我们来看几个例子

1) 我们将 `buffer_size` 设置为 16，然后每次写入 15 字节的数据
```Python
with open('test.log', 'wb', buffering=16) as f:
    for _ in range(5):
        f.write(b'a' * 15)
```

通过 `strace` 跟踪系统调用，可以发现执行了 5 次

```
➜ strace -e write python t.py
write(3, "aaaaaaaaaaaaaaa", 15)         = 15
write(3, "aaaaaaaaaaaaaaa", 15)         = 15
write(3, "aaaaaaaaaaaaaaa", 15)         = 15
write(3, "aaaaaaaaaaaaaaa", 15)         = 15
write(3, "aaaaaaaaaaaaaaa", 15)         = 15
+++ exited with 0 +++
```

第一次我们写入时，会直接复制到 `buffer` 中。当第二次写入时，因为 `buffer` 的剩余空间(为 1 byte)小于将要写入的数据(15 bytes)，所以先会尝试清空现在的 `buffer`。将 `buffer` 中的 15 bytes 通过 `write(2)` 写入。如果清空成功，那么会尝试比较 `buffer_size` 和将要写入的数据大小，如果小于那么复制到 buffer 中。如果大于那么会尝试直接通过 `write(2)` 写入，直到全部写完或者剩余的数据量小于 `buffer_size`(将其复制到 buffer 中)

2)

```Python
with open('test.log', 'wb', buffering=16) as f:
    f.write(b'a' * 15)
    f.write(b'a' * 1)
    f.write(b'a' * 3)
    f.write(b'a' * 3)
```

`strace` 跟踪结果

```
➜ strace -e write python t.py
write(3, "aaaaaaaaaaaaaaaa", 16)        = 16
write(3, "aaaaaa", 6)                   = 6
+++ exited with 0 +++
```

第一次写入先缓冲 15 bytes，第二次写入再缓冲 1 byte。第三次写入时满了，进行 flush，一次性写入 16 bytes。然后待写入的 3 bytes 小于 `buffer_size`，再次缓冲。第四次写入再缓冲 3 bytes。最后在 `with` 的作用域结束会自动写入这 6 bytes


### Text Mode

接下来是以 `t` 模式打开文件，Python 返回的是 `_io.TextIOWrapper`，这个对象中包含了一个 `_io.BufferedWriter`，可以通过 `f.buffer` 进行访问。除了这个底层缓冲区，`_io.TextIOWrapper` 中还包含了一个 `pending_bytes` 的缓冲区，它默认大小是 8192 bytes(初始化时[硬编码的](https://github.com/python/cpython/blob/9a3ffc0492d1310ead9ce8f5ee678c26b20a338d/Modules/_io/textio.c#L1156))，也可以通过 `_CHUNK_SIZE` 进行修改

```C
// https://github.com/python/cpython/blob/9a3ffc0492d1310ead9ce8f5ee678c26b20a338d/Modules/_io/textio.c#L638
typedef struct
{
    // ...
    Py_ssize_t chunk_size;

    /* Reads and writes are internally buffered in order to speed things up.
       However, any read will first flush the write buffer if itsn't empty.
       Please also note that text to be written is first encoded before being
       buffered. This is necessary so that encoding errors are immediately
       reported to the caller, but it unfortunately means that the
       IncrementalEncoder (whose encode() method is always written in Python)
       becomes a bottleneck for small writes.
    */
    PyObject *decoded_chars;       /* buffer for text returned from decoder */
    Py_ssize_t decoded_chars_used; /* offset into _decoded_chars for read() */
    PyObject *pending_bytes;       /* list of bytes objects waiting to be
                                      written, or NULL */
    Py_ssize_t pending_bytes_count;
    // ...
} textio;
```

这里的 `pending_bytes` 是一个 bytes 对象的列表，`pending_bytes_count` 保存了列表中所有对象的字节数目统计

当我们写入时首先会检查 `\n`，因为它会影响到缓冲策略。然后将其编码为 `bytes`，追加到 `pending_bytes` 中。如果 `pending_bytes_count > chunk_size` 那么会写入底层的 `buffer` 中。这个就是我们刚才所谈论的 `_io.BufferedWriter` 了

```C
// https://github.com/python/cpython/blob/9a3ffc0492d1310ead9ce8f5ee678c26b20a338d/Modules/_io/textio.c#L1524

static PyObject *
_io_TextIOWrapper_write_impl(textio *self, PyObject *text)
/*[clinic end generated code: output=d2deb0d50771fcec input=fdf19153584a0e44]*/
{
    PyObject *ret;
    PyObject *b;
    Py_ssize_t textlen;
    int haslf = 0;
    int needflush = 0, text_needflush = 0;

    if (PyUnicode_READY(text) == -1)
        return NULL;

    CHECK_ATTACHED(self);
    CHECK_CLOSED(self);

    if (self->encoder == NULL)
        return _unsupported("not writable");

    Py_INCREF(text);

    textlen = PyUnicode_GET_LENGTH(text);

    if ((self->writetranslate && self->writenl != NULL) || self->line_buffering)
        if (PyUnicode_FindChar(text, '\n', 0, PyUnicode_GET_LENGTH(text), 1) != -1)
            haslf = 1;

    if (haslf && self->writetranslate && self->writenl != NULL) {
        PyObject *newtext = _PyObject_CallMethodId(
            text, &PyId_replace, "ss", "\n", self->writenl);
        Py_DECREF(text);
        if (newtext == NULL)
            return NULL;
        text = newtext;
    }

    if (self->write_through)  // 可以通过 reconfigure 设置
        text_needflush = 1;
    if (self->line_buffering &&
        (haslf ||
         PyUnicode_FindChar(text, '\r', 0, PyUnicode_GET_LENGTH(text), 1) != -1))
        needflush = 1;

    /* XXX What if we were just reading? */
    if (self->encodefunc != NULL) {
        b = (*self->encodefunc)((PyObject *) self, text);
        self->encoding_start_of_stream = 0;
    }
    else
        b = PyObject_CallMethodObjArgs(self->encoder,
                                       _PyIO_str_encode, text, NULL);
    Py_DECREF(text);
    if (b == NULL)
        return NULL;
    if (!PyBytes_Check(b)) {
        PyErr_Format(PyExc_TypeError,
                     "encoder should return a bytes object, not '%.200s'",
                     Py_TYPE(b)->tp_name);
        Py_DECREF(b);
        return NULL;
    }

    if (self->pending_bytes == NULL) {
        self->pending_bytes = PyList_New(0);
        if (self->pending_bytes == NULL) {
            Py_DECREF(b);
            return NULL;
        }
        self->pending_bytes_count = 0;
    }
    if (PyList_Append(self->pending_bytes, b) < 0) {
        Py_DECREF(b);
        return NULL;
    }
    self->pending_bytes_count += PyBytes_GET_SIZE(b);
    Py_DECREF(b);
    if (self->pending_bytes_count > self->chunk_size || needflush ||
        text_needflush) {
        if (_textiowrapper_writeflush(self) < 0)
            return NULL;
    }

    if (needflush) {
        ret = PyObject_CallMethodObjArgs(self->buffer, _PyIO_str_flush, NULL);
        if (ret == NULL)
            return NULL;
        Py_DECREF(ret);
    }

    textiowrapper_set_decoded_chars(self, NULL);
    Py_CLEAR(self->snapshot);

    if (self->decoder) {
        ret = _PyObject_CallMethodId(self->decoder, &PyId_reset, NULL);
        if (ret == NULL)
            return NULL;
        Py_DECREF(ret);
    }

    return PyLong_FromSsize_t(textlen);
}
```

需要注意 `_textiowrapper_writeflush` 实际上只会将 `pending_bytes` 中的内容写入到 `buffer` 中，不会 flush `buffer`

```C
// https://github.com/python/cpython/blob/9a3ffc0492d1310ead9ce8f5ee678c26b20a338d/Modules/_io/textio.c#L1489
static int
_textiowrapper_writeflush(textio *self)
{
    PyObject *pending, *b, *ret;

    if (self->pending_bytes == NULL)
        return 0;

    pending = self->pending_bytes;
    Py_INCREF(pending);
    self->pending_bytes_count = 0;
    Py_CLEAR(self->pending_bytes);

    b = _PyBytes_Join(_PyIO_empty_bytes, pending);
    Py_DECREF(pending);
    if (b == NULL)
        return -1;
    ret = NULL;
    do {
        ret = PyObject_CallMethodObjArgs(self->buffer,
                                         _PyIO_str_write, b, NULL);
    } while (ret == NULL && _PyIO_trap_eintr());
    Py_DECREF(b);
    if (ret == NULL)
        return -1;
    Py_DECREF(ret);
    return 0;
}
```

那么比如我们指定行缓冲时，底层的 `buffer` 的大小是多大呢

首先来看一下 `open` 的实现

```C
// https://github.com/python/cpython/blob/9a3ffc0492d1310ead9ce8f5ee678c26b20a338d/Modules/_io/_iomodule.c#L401
if (buffering == 1 || (buffering < 0 && isatty)) {
    buffering = -1;
    line_buffering = 1;
}
else
    line_buffering = 0;

if (buffering < 0) {
    PyObject *blksize_obj;
    blksize_obj = _PyObject_GetAttrId(raw, &PyId__blksize);  // raw 的类型是 PyFileIO_Type
    if (blksize_obj == NULL)
        goto error;
    buffering = PyLong_AsLong(blksize_obj);
    Py_DECREF(blksize_obj);
    if (buffering == -1 && PyErr_Occurred())
        goto error;
}
```

可见指定了行缓冲(`buffering = 1`)后，`buffering` 的值会变成 `blksize` 的值，根据 `_io_FileIO___init___impl` 的[实现](https://github.com/python/cpython/blob/9a3ffc0492d1310ead9ce8f5ee678c26b20a338d/Modules/_io/fileio.c#L431) 取的是 `DEFAULT_BUFFER_SIZE`。这个值即是 `io.DEFAULT_BUFFER_SIZE`，我这里是 8192 bytes


我们来看几个例子

1)

```Python
with open('test.log', 'w', buffering=16) as f:
    for _ in range(5):
        f.write('a' * 8192)
```

`strace` 输出

```
➜ strace -e write python t.py
write(3, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"..., 16384) = 16384
write(3, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"..., 16384) = 16384
write(3, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"..., 8192) = 8192
+++ exited with 0 ++
```

这里我们限定了 buffer 的大小为 16 bytes。第一次写入，数据会被追加到 `pending_bytes` 列表中，然后 `pending_bytes_count` 为 8192 bytes，它并没有超过 `_CHUNK_SIZE`(8192 bytes)。在第二次写入时，数据同样追加到 `pending_bytes`，但是此时 `pending_bytes_count`(16384 bytes) 超过了 `_CHUNK_SIZE`，导致数据被刷到 `buffer` 中。不过 `buffer` 的长度是 16 bytes，所以数据被尝试直接使用 `write(2)` 写入。所以有了我们看到了 `write(3, "aaa", 16384)` 的 syscall。最后的 8192 bytes 的写入是因为 `with` 作用域导致的 `close` 所写入的

2)

让我们将 buffer 调到 30000 bytes

```Python
with open('test.log', 'w', buffering=30000) as f:
    for _ in range(7):
        f.write('a' * 8192)

```

`strace` 输出

```
➜ strace -e write python t.py
write(3, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"..., 16384) = 16384
write(3, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"..., 16384) = 16384
write(3, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"..., 24576) = 24576
+++ exited with 0 +++
```

第一次写入与第二次写入和上例一样，不过 `pending_bytes` 被刷到 `buffer` 中后并没有超过 `buffer_size`，它被放到了缓冲区中。第三次写入，数据被缓冲在 `pending_bytes` 中，第四次写入造成 `pending_bytes` 缓冲的数据总量超过 `_CHUNK_SIZE`，使得 16384 bytes 被刷到 `buffer` 中，此时 `buffer` 中存放了之前缓冲的 16384 bytes，而 `buffer` 的大小是 30000。可用空间不足，buffer 先被 flush，如果没有意外 16384 bytes 会被通过 `write(2)` 写入。这样 `buffer` 又空了，新来的 16384 bytes 会被缓冲住。第五次写入和第六次写入所发生的事情和第三次、第四次相同。第七次写入时，数据会放在 `pending_bytes` 中。当 `with` 结束，这 8192 bytes 会被先刷到 `buffer` 中，然后 `buffer` 再清空，所以写入了 16384 + 8192 bytes

3)

我们在上面还看到了一个 `write_through` 变量，如果为 `True` 那么写入是不会经过 `pending_bytes` 的

```Python
with open('test.log', 'w', buffering=30000) as f:
    f.reconfigure(write_through=True)
    for _ in range(7):
        f.write('a' * 8192)
```

`strace` 结果如下

```
➜ strace -e write python t.py
write(3, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"..., 24576) = 24576
write(3, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"..., 24576) = 24576
write(3, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"..., 8192) = 8192
+++ exited with 0 +++
```

因为不经过 `pending_bytes`，所以结果和

```Python
with open('test.log', 'wb', buffering=30000) as f:
    for _ in range(7):
        f.write(b'a' * 8192)
```

完全一致

4)

使用行缓冲的情况

```Python
with open('test.log', 'w', buffering=1) as f:
    for _ in range(5):
        f.write('a\n')
```

```
➜ strace -e write python t.py
write(3, "a\n", 2)                      = 2
write(3, "a\n", 2)                      = 2
write(3, "a\n", 2)                      = 2
write(3, "a\n", 2)                      = 2
write(3, "a\n", 2)                      = 2
+++ exited with 0 +++
```

    