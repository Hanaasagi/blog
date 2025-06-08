+++
title = "X11 获取并切换窗口"
summary = ""
description = ""
categories = [""]
tags = []
date = 2025-06-08T12:29:30+09:00
draft = false

+++



承接这篇文章 [Linux 下查找应用的 icon](https://blog.dreamfever.me/posts/2025-05-17-linux-desktop-application-icons/)，记录一下 X11 下编程遇到的问题及如何解决的。本文使用的依赖是 `x11rb 0.13.1` 



## 获取活动窗口列表

对于 Linux 下可以使用 `wmctrl -l` 命令来列出窗口。我们也可以通过 `x11rb` 来创建 Client 可以 X11 Server 进行通信。



```rust
use x11rb::connection::Connection;
use x11rb::protocol::xproto::*;
use x11rb::rust_connection::RustConnection;

fn get_window_title<C: Connection>(
    conn: &C,
    window: u32,
    net_wm_name: u32,
    utf8_string: u32,
    fallback_atom: u32,
) -> Result<String, Box<dyn std::error::Error>> {
    let title_reply = conn
        .get_property(false, window, net_wm_name, utf8_string, 0, u32::MAX)?
        .reply();

    let title = match title_reply {
        Ok(reply) if reply.value_len > 0 => String::from_utf8_lossy(&reply.value).into_owned(),
        _ => {
            // fallback to WM_NAME
            let fallback_reply = conn
                .get_property(false, window, fallback_atom, AtomEnum::STRING, 0, u32::MAX)?
                .reply();
            fallback_reply
                .map(|r| String::from_utf8_lossy(&r.value).into_owned())
                .unwrap_or_default()
        }
    };

    Ok(title)
}

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let (conn, screen_num) = RustConnection::connect(None)?;
    let screen = &conn.setup().roots[screen_num];

    let root = screen.root;

    let net_client_list_atom = conn.intern_atom(false, b"_NET_CLIENT_LIST")?.reply()?.atom;

    let net_wm_name = conn.intern_atom(false, b"_NET_WM_NAME")?.reply()?.atom;
    let utf8_string = conn.intern_atom(false, b"UTF8_STRING")?.reply()?.atom;
    let wm_name = AtomEnum::WM_NAME;
    let net_wm_desktop = conn.intern_atom(false, b"_NET_WM_DESKTOP")?.reply()?.atom;

    let reply = conn
        .get_property(
            false,
            root,
            net_client_list_atom,
            AtomEnum::WINDOW,
            0,
            u32::MAX,
        )?
        .reply()?;

    let window_ids = reply.value32().unwrap();

    for (i, window) in window_ids.into_iter().enumerate() {
        let desktop_reply = conn
            .get_property(false, window, net_wm_desktop, AtomEnum::CARDINAL, 0, 1)?
            .reply()?;
        let desktop = desktop_reply.value32().unwrap().next().unwrap_or(0);

        let title = get_window_title(&conn, window, net_wm_name, utf8_string, wm_name.into())
            .unwrap_or_else(|_| String::from("<Unknown>"));

        println!("[{}] {:#012x}  {} localhost {}", i, window, desktop, title);
    }

    Ok(())
}

```





核心函数是 `get_property` ，不过在这之前我们需要了解一下 X11 ATOM。X11 Server 在启动的时候会将一些属性、类型、方法的字符串映射一个整数 ID，这样在客户端调用的时候无需传递字符串，直接传递整整即可。这种符号映射表机制在比如 ProtoBuf/gRPC 这种 RPC 协议中也有体现，使得 payload 更小。所以在我们建立连接之后，需要通过 `inter_atom` 来将一些方法名进行转换。在 X11 Server 的生命周期里面这个是不会变化的，所以客户端侧可以进行缓存



`get_property` 这个函数签名如下

| 参数名        | 类型   | 含义                                                         |
| ------------- | ------ | ------------------------------------------------------------ |
| `window`      | `u32`  | 要获取属性的目标窗口 ID。                                    |
| `delete`      | `bool` | 是否在获取成功后删除该属性（一般设为 `false`）。             |
| `property`    | `Atom` | 要读取的属性名称，例如 `_NET_WM_NAME`（UTF-8 标题）、`WM_NAME`（传统 ASCII 标题）。 |
| `type`        | `Atom` | 期望该属性的类型，例如 `UTF8_STRING`、`STRING`、`WINDOW`、`CARDINAL` 等。 |
| `long_offset` | `u32`  | 从属性值的第几个 32-bit 单位开始读取（偏移）。               |
| `long_length` | `u32`  | 要读取的最大 32-bit 单位数量（长度）。                       |



X11 Server 是使用的序列化协议，这个函数的参数实际是用来构造请求的 payload。其中后 3 个参数用于告诉 X11 返回的数据应该是什么。同样也影响到我们如何去读取这个数据。`long_offset` 和 `long_length` 类似于一个分页，我们可以请求 X11 返回部分数据给我们。我们拿到这个 `reply` 是一个 `Vec<u8>` 的类型，`x11rb` 中封装了一些方法可以读取这个数据，比如 `value32` 这种



另外一个问题是，通过代码可以看到我这里是拿了 `screen` 之后使用 `root` 窗口的 id，来获取的所有窗口列表。这里并没有先拿 `desktop`，是因为它是一个窗口管理器添加上去的属性，是一种逻辑上的空间。X11 里面显示器对应了 `screen`，默认有一个 `root` 的窗口就是我们的背景，其他窗口都是它的子窗口



## 实现窗口切换



首先是一个错误的写法，通过 `change_property32` 是不能工作的！根据 GPT 的回答来说是因为

> 你 不能直接用 `change_property32` 修改 `_NET_ACTIVE_WINDOW` 属性，因为：
>
> 1. 该属性是 窗口管理器来维护的，它根据当前活跃窗口来更新。
> 2. 你写入它不会被窗口管理器感知，也不会触发激活行为。



```rust
let net_current_desktop_atom = conn
    .intern_atom(false, b"_NET_CURRENT_DESKTOP")?
    .reply()?
    .atom;

let net_active_window_atom = conn
    .intern_atom(false, b"_NET_ACTIVE_WINDOW")?
    .reply()?
    .atom;

let current_desktop = conn
    .get_property(
        false,
        root,
        net_current_desktop_atom,
        AtomEnum::CARDINAL,
        0,
        1,
    )?
    .reply()?
    .value32()
    .unwrap()
    .next()
    .unwrap_or(0);

if target_window_desktop != current_desktop {
    println!("Switching to desktop {}", target_window_desktop);
    conn.change_property32(
        PropMode::REPLACE,
        root,
        net_current_desktop_atom,
        AtomEnum::CARDINAL,
        &[target_window_desktop],
    )?;
    conn.flush()?;
}
conn.flush()?;

conn.change_property32(
    PropMode::REPLACE,
    root,
    net_active_window_atom,
    AtomEnum::CARDINAL,
    &[target_window_id],
)?;
conn.flush()?;


```





看了 xdotool 这个项目的源码 https://github.com/jordansissel/xdotool/blob/33092d8a74d60c9ad3ab39c4f05b90e047ea51d8/xdo.c#L466，他是通过 event 来实现的。等价的代码如下



```rust
let net_current_desktop_atom = conn
    .intern_atom(false, b"_NET_CURRENT_DESKTOP")?
    .reply()?
    .atom;

let net_active_window_atom = conn
    .intern_atom(false, b"_NET_ACTIVE_WINDOW")?
    .reply()?
    .atom;

// 获取当前桌面
let current_desktop = conn
    .get_property(
        false,
        root,
        net_current_desktop_atom,
        AtomEnum::CARDINAL,
        0,
        1,
    )?
    .reply()?
    .value32()
    .unwrap()
    .next()
    .unwrap_or(0);

if target_window_desktop != current_desktop {
    println!("Switching to desktop {}", target_window_desktop);

    let event = ClientMessageEvent {
        response_type: x11rb::protocol::xproto::CLIENT_MESSAGE_EVENT,
        format: 32,
        sequence: 0,
        window: root,
        type_: net_current_desktop_atom,
        data: ClientMessageData::from([
            target_window_desktop, // data.l[0]: target desktop index
            x11rb::CURRENT_TIME,   // data.l[1]: timestamp
            0,
            0,
            0,
        ]),
    };

    conn.send_event(
        false,
        root,
        EventMask::SUBSTRUCTURE_NOTIFY | EventMask::SUBSTRUCTURE_REDIRECT,
        event,
    )?;

    conn.flush()?;
}

println!("Focusing window: {}", window_title);

let event = ClientMessageEvent {
    response_type: x11rb::protocol::xproto::CLIENT_MESSAGE_EVENT,
    format: 32,
    sequence: 0,
    window: target_window_id, // 被激活的窗口 ID
    type_: net_active_window_atom,
    data: ClientMessageData::from([2, x11rb::CURRENT_TIME, 0, 0, 0]),
};

conn.send_event(
    false,
    root,
    EventMask::SUBSTRUCTURE_REDIRECT | EventMask::SUBSTRUCTURE_NOTIFY,
    event,
)?;

conn.flush()?;
conn.map_window(target_window_id)?;
conn.flush()?;
println!("Window focused successfully {}", target_window_id);


```

