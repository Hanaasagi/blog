
+++
title = "Differences between read and read_buf in Tokio"
summary = ''
description = ""
categories = []
tags = []
date = 2023-04-30T16:35:00+08:00
draft = false
+++



记录一下本周遇到的一个问题，下面是最小复现代码

```rust
use tokio::{
    io::{AsyncReadExt, AsyncWriteExt},
    net::TcpStream,
};

#[tokio::main]
async fn main() -> std::io::Result<()> {
    let mut stream = TcpStream::connect("93.184.216.34:80").await?;
    stream
        .write_all(
            "GET / HTTP/1.1\\r\\nHost: example.com\\r\\nUser-Agent: curl/8.0.1\\r\\nAccept: */*\\r\\n\\r\\n"
                .as_bytes(),
        )
        .await?;

    let mut buf = vec![0; 2048];

    let n = stream.read_buf(&mut buf).await?;

    let bad = String::from_utf8_lossy(&buf);
    assert!(!bad.starts_with("HTTP/1.1 200 OK\\r\\n"));

    let ok = String::from_utf8_lossy(&buf[buf.len() - n..]);
    assert!(ok.starts_with("HTTP/1.1 200 OK\\r\\n"));

    Ok(())
}
```



为什么这个数据不是从 index = 0 的地方开始填充的，而是从 `buf` 的尾部开始填充。下面来一探究竟



当我们调用 `stream.read_buf` 的时候，会创建一个 `ReadBuf` 数据结构。`ReadBuf` 是 tokio 定义的一个对于底层 buffer 的一层包装，通过两个 cursor 跟踪数据的边界，详细用法可以参考文档 [Struct tokio::io::ReadBuf](https://docs.rs/tokio/latest/tokio/io/struct.ReadBuf.html) 。我们这边主要来看其提供了 `poll` 函数，此函数用于读取数据到其封装的 buffer 中



```rust
// https://github.com/tokio-rs/tokio/blob/52bc6b6f2d773def6bfaabf6925fef4e789782b7/tokio/src/io/util/read_buf.rs#L35
impl<R, B> Future for ReadBuf<'_, R, B>
where
    R: AsyncRead + Unpin,
    B: BufMut,
{
    type Output = io::Result<usize>;

    fn poll(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<io::Result<usize>> {
        use crate::io::ReadBuf;
        use std::mem::MaybeUninit;

        let me = self.project();

        if !me.buf.has_remaining_mut() {
            return Poll::Ready(Ok(0));
        }

        let n = {
            let dst = me.buf.chunk_mut();
            let dst = unsafe { &mut *(dst as *mut _ as *mut [MaybeUninit<u8>]) };
            let mut buf = ReadBuf::uninit(dst);
            let ptr = buf.filled().as_ptr();
            ready!(Pin::new(me.reader).poll_read(cx, &mut buf)?);

            // Ensure the pointer does not change from under us
            assert_eq!(ptr, buf.filled().as_ptr());
            buf.filled().len()
        };

        // Safety: This is guaranteed to be the number of initialized (and read)
        // bytes due to the invariants provided by `ReadBuf::filled`.
        unsafe {
            me.buf.advance_mut(n);
        }

        Poll::Ready(Ok(n))
    }

}
```



逐步分析一下上面函数的过程。[`chunk_mut` ](https://docs.rs/bytes/latest/bytes/buf/trait.BufMut.html#tymethod.chunk_mut) 在不同类型上有不同的定义，对于 `Vec<u8>` 来说定义如下：

```rust
// https://github.com/tokio-rs/bytes/blob/b29112ce4484424a0137173310ec8b9f84db27ae/src/buf/buf_mut.rs#L1480-L1490
fn chunk_mut(&mut self) -> &mut UninitSlice {
    if self.capacity() == self.len() {
        self.reserve(64); // Grow the vec
    }

    let cap = self.capacity();
    let len = self.len();

    let ptr = self.as_mut_ptr();
    unsafe { &mut UninitSlice::from_raw_parts_mut(ptr, cap)[len..] }
}
```

`chunk_mut` 返回了一个从 `len()` 到 `capacity()` 之间的 slice。如果 `capacity()` 的值和 `len()` 是相等的，那么会调用 [reserve]([]()) 函数来分配额外的至少 64 字节的空间。需要特别注意返回的 slice 的其实地址是从 `len()` 开始的，也就是说当我们传入 `vec![0; 2048]` 的时候，返回的是 `vec[2048..cap]` 这区域的 slice。这也就是当我们调用 `stream.read_buf(&mut buf).await?;` 后，不是从 index 为 0 的地方开始填充的根本原因

然后在 `poll` 函数中将这段 slice 初始化为一个新的 `ReadBuf` ，并调用 `ready!(Pin::new(me.reader).poll_read(cx, &mut buf)?);` 从 stream 中读取数据。`            buf.filled().len()` 返回的是已经填充的数据。通过 `advace_mut` 函数来将原来 `buf` 的长度增加 `n`



总结一下， `read_buf` 函数写入的位置是从当前 `len()` 到 `capacity()`。我们再来对比一下另一个函数 `read`

```rust
use tokio::{
    io::{AsyncReadExt, AsyncWriteExt},
    net::TcpStream,
};

#[tokio::main]
async fn main() -> std::io::Result<()> {
    let mut stream = TcpStream::connect("93.184.216.34:80").await?;
    stream
        .write_all(
            "GET / HTTP/1.1\\r\\nHost: example.com\\r\\nUser-Agent: curl/8.0.1\\r\\nAccept: */*\\r\\n\\r\\n"
                .as_bytes(),
        )
        .await?;

    let mut buf = bytes::BytesMut::with_capacity(4096);

    let n = stream.read(&mut buf).await?;
    assert_eq!(n, 0);

    Ok(())
}
```

上面这个示例为什么 n 永远为 0 呢？当我们执行 `&mut buf` 的时候是执行了下面的代码

```rust
#[inline]
pub fn with_capacity(capacity: usize) -> BytesMut {
    BytesMut::from_vec(Vec::with_capacity(capacity))
}

#[inline]
fn as_slice_mut(&mut self) -> &mut [u8] {
    unsafe { slice::from_raw_parts_mut(self.ptr.as_ptr(), self.len) }
}

impl AsMut<[u8]> for BytesMut {
    #[inline]
    fn as_mut(&mut self) -> &mut [u8] {
        self.as_slice_mut()
    }
}

```

`as_slice_mut` 函数会使用当前的 `len` 来作为长度的值。因为我们没有向 `BytesMut` 中添加任何数据，所以 `len` 的值为 0。这就相当于给了一个零长的数组。所以 `read` 无法向其中填充数据。正确用法是初始化长度，这里可以使用 0 进行填充

```rust
let mut buf = bytes::BytesMut::zeroed(16);
stream.read(&mut buf).await?;
```

或者使用 unsafe 代码来修改长度，但是注意 `len()` 需要小于 `capacity()`

```rust
let mut buf = bytes::BytesMut::with_capacity(4096);
unsafe { buf.set_len(4096) };
stream.read(&mut buf).await?;
```

或者刚才我们提到的使用 `read_buf`

```rust
let mut buf = bytes::BytesMut::with_capacity(4096);
stream.read_buf(&mut buf).await?;
```



至此，我们已经明白的 `read` 和 `read_buf` 在参数要求上的区别。下面有几个代码示例，不妨判断一下对错



##### Example 01

```rust
let mut buf = [0u8; 4096];
stream.read_buf(&mut buf.as_mut()).await?;
assert!(&buf[0..4] == b"HTTP");
```

<details><summary> ▶︎ Answer </summary>
✅
</details>



##### Example 02

```rust
let mut buf = vec![0u8; 4096];
stream.read(&mut buf[..]).await?;
assert!(&buf[0..4] == b"HTTP");
```

<details><summary> ▶︎ Answer </summary>
✅
</details>



##### Example 03

```rust
let mut buf = [0u8; 4096];
stream.read_buf(&mut buf.as_mut_slice()).await?;
assert!(&buf[0..4] == b"HTTP");
```

<details><summary> ▶︎ Answer </summary>
✅
</details>



##### Example 04

```rust
let mut buf = vec![0u8; 2048];

stream.read_buf(&mut buf.as_mut_slice()).await?;
assert!(&buf[0..4] == b"HTTP");
```

<details><summary> ▶︎ Answer </summary>
✅
</details>

这个稍微有点难以理解，因为和之前的一个例子很像。区别在于 `chunk_mut` 在 `&mut [u8]` 类型上的定义为

```rust
fn chunk_mut(&mut self) -> &mut UninitSlice {
        // UninitSlice is repr(transparent), so safe to transmute
        unsafe { &mut *(*self as *mut [u8] as *mut _) }
}
```

这个是会返回的是整个 slice



##### Example 05

```rust
let mut buf = Vec::with_capacity(2048);
stream.read(&mut buf[..]).await?;
assert!(&buf[0..4] == b"HTTP");
```

<details><summary> ▶︎ Answer </summary>
❌
</details>



##### Example 06

```rust
use std::mem::MaybeUninit;
let mut buf: [u8; 2048] = unsafe { MaybeUninit::uninit().assume_init() };
stream.read(&mut buf[..]).await?;
assert!(&buf[0..4] == b"HTTP");
```

<details><summary> ▶︎ Answer </summary>
✅
</details>



##### Example 07

```rust
use std::mem::MaybeUninit;
let mut buf: Box<[u8; 2048]> = unsafe { MaybeUninit::uninit().assume_init() };
stream.read(&mut buf[..]).await?;
assert!(&buf[0..4] == b"HTTP");
```

<details><summary> ▶︎ Answer </summary>
❌ core dump
</details>



##### Example 08

```rust
use std::mem::MaybeUninit;
let mut buf: Vec<u8> = unsafe { MaybeUninit::uninit().assume_init() };
buf.reserve(2048);
stream.read_buf(&mut buf).await?;
assert!(&buf[0..4] == b"HTTP");
```

<details><summary> ▶︎ Answer </summary>
❌ core dump
</details>



##### Example 09

```rust
let mut buf: Box<[u8; 2048]> = Box::new([0; 2048]);
stream.read(&mut buf[..]).await?;
assert!(&buf[0..4] == b"HTTP");
```

<details><summary> ▶︎ Answer </summary>
✅
</details>




    