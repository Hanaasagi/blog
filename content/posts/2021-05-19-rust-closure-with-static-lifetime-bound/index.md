
+++
title = "Rust: closure with static lifetime bound"
summary = ''
description = ""
categories = []
tags = []
date = 2021-05-19T14:09:55+08:00
draft = false
+++

近日使用 `winit` 这个 crate 的时候，遇到了这么一个问题：`EventLoop` 的 `run` 方法的参数是一个 closure，并且它具有 `static` 的 lifetime bound。方法签名如下

```
// https://docs.rs/winit/0.25.0/src/winit/event_loop.rs.html#150-155

pub fn run<F>(self, event_handler: F) -> !
where
    F: 'static + FnMut(Event<'_, T>, &EventLoopWindowTarget<T>, &mut ControlFlow)
        
```

当它使用上下文的自由变量的时候，编译报错  `xxx` has an anonymous lifetime `'_` but it needs to satisfy a `'static` lifetime requirement



为了减少理解成本，精简代码如下：

```Rust
use std::path::Path;

enum Event {
    // UserEvent(T),
    Suspended,
    Resumed,
    MainEventsCleared,
    RedrawEventsCleared,
    LoopDestroyed,
    // ...
}

struct EventLoop {
    // phantom : PhantomData<T>
}

impl EventLoop {
    fn new() -> Self {
        Self {
            // phantom: PhantomData
        }
    }

    pub fn run<F>(self, event_handler: F) -> !
    where
        F: 'static + FnMut(Event),
    {
        loop {}
    }
}

struct Config {}

impl Config {
    fn load_from(file_path: impl AsRef<Path>) -> Self {
        Self {}
    }
}
struct Application {
    config: Config,
}

impl Application {
    fn new(config: Config) -> Self {
        Self { config: config }
    }
    fn suspended(&mut self) {}

    fn run_forever(&mut self) -> ! {
        let event_loop = EventLoop::new();

        event_loop.run(move |event| match event {
            Event::Suspended => {
                self.suspended(); // 报错位置
            }
            _ => unimplemented!(),
        })
    }
}

fn main() {
    let config = Config::load_from(Path::new("./testing.cfg"));
    let mut app = Application::new(config);
    app.run_forever();
}

```

详细报错如下

```
error[E0759]: `self` has an anonymous lifetime `'_` but it needs to satisfy a `'static` lifetime requirement
  --> src/main.rs:52:24
   |
49 |       fn run_forever(&mut self) -> ! {
   |                      --------- this data with an anonymous lifetime `'_`...
...
52 |           event_loop.run(move |event| match event {
   |  _ _ _ _ _ _ _ _ _ _ _ __^
53 | |             Event::Suspended => {
54 | |                 self.suspended();
55 | |             }
56 | |             _ => unimplemented!(),
57 | |         })
   | |_ _ _ _ _^ ...is captured here...
   |
note: ...and is required to live as long as `'static` here
```



首先我们需要搞清楚  `F: 'static` 的含义，这个是一个比较容易搞混的概念，在 [common rust lifetime misconceptions](https://github.com/pretzelhammer/rust-blog/blob/master/posts/common-rust-lifetime-misconceptions.md#2-if-t-static-then-t-must-be-valid-for-the-entire-program) 中也有提到：

- ✅ `T: 'static` should be read as *"`T` is bounded by a `'static` lifetime"* 

- ❌ `T: 'static` should be read as *"`T` has a `'static` lifetime"*

  

**`T: 'static` can be dynamically allocated at run-time, can be safely and freely  mutated, can be dropped, and can live for arbitrary durations.**



这里 `run` 方法需要我们提供的是一个这样的函数，它需要保证内部使用到的任何数据都不会在此函数 drop 之前被 drop。所以我们上面的报错是因为在 Closure 中使用到的 `self` 变量不满足这个条件



下面有这么几种思路去解决这个问题：



### 思路一

既然 `self` 变量的 lifetime 短，那么就延长 lifetime。将 `run_forever` 的签名改为 ` fn run_forever(&'static mut self) -> ! `，这一操作具有传播性，需要接着更改调用方。具体步骤又可以分为几种:

1. 直接 `static` + `unsafe`  这是一种最直接粗暴方法，但是需要保证安全性。而且 `Application` 的参数没法在构造的时候传进去，需要延后

```rust
static mut app: Application = Application {};


fn main() {
    unsafe {
        app.run_forever();
    }
}
```

2. 通过 [`Box::leak`](https://doc.rust-lang.org/std/boxed/struct.Box.html#method.leak) 来故意泄漏出一个 `static` 的 ref

> Consumes and leaks the `Box`, returning a mutable reference, `&'a mut T`. Note that the type `T` must outlive the chosen lifetime `'a`. If the type has only static references, or none at all, then this may be chosen to be `'static`.
>
> This function is mainly useful for data that lives for the remainder of the program’s life. Dropping the returned reference will cause a memory leak. If this is not acceptable, the reference should first be wrapped with the [`Box::from_raw`](https://doc.rust-lang.org/std/boxed/struct.Box.html#method.from_raw) function producing a `Box`. This `Box` can then be dropped which will properly destroy `T` and release the allocated memory.
>
> Note: this is an associated function, which means that you have to call it as `Box::leak(b)` instead of `b.leak()`. This is so that there is no conflict with a method on the inner type.

但是这种方法，需要注意使用的场景，因为这个内存需要管理起来

```Rust
fn main() {
    let app = Box::new(Application::new());
    Box::leak(app).run_forever();
}
```



### 思路二

Rust 中有一些编译问题可以通过适当的改变代码结构来解决的，但是不推荐这种做法，因为破坏了 `Application` 结构的封装

```Rust
fn main() {
    let config: Config = Config::load_from(Path::new("./testing.cfg"));
    let mut app = Application::new(config);

    let event_loop = EventLoop::new();

    event_loop.run(move |event| match event {
        Event::Suspended => {
            app.suspended();
        }
        _ => unimplemented!(),
    })
}
```

相比较最开始的编译错误的代码， `app` 变量现在保证了不会在 closure 之前被回收，所以这样能够通过编译



### 思路三

使用 *reference counting* 来解决

- 单线程场景 `Rc` + `RefCell`
- 并发场景 `Arc` +( `Mutex` 或者 `RwLock`)

```Rust
impl Application {
    fn run_forever(s: Rc<RefCell<Self>>) -> ! {
        let event_loop = EventLoop::new();

        event_loop.run(move |event| match event {
            Event::Suspended => {
                s.borrow_mut().suspended();
            }
            _ => unimplemented!(),
        })
    }
}

fn main() {
    let config = Config::load_from(Path::new("./testing.cfg"));
    let app = Application::new(config);
    Application::run_forever(Rc::new(RefCell::new(app)));
}
```



### 思路四

这种方法只需要改动一个字符.将 `run` 方法的签名替换为从 ``fn run_forever(&mut self) -> !` 替换为 `fn run_forever(mut self) -> !`。不再使用  Mutable References，直接转移 ownership。不过这样我们无法再之后使用 `app` 变量了



### Reference 

- [Common Rust Lifetime Misconceptions](https://github.com/pretzelhammer/rust-blog/blob/master/posts/common-rust-lifetime-misconceptions.md#common-rust-lifetime-misconceptions)

- [Cursive: closures with static lifetime](https://users.rust-lang.org/t/cursive-closures-with-static-lifetime/19586)
- [How do I call a function that requires a 'static lifetime with a variable created in main?](https://stackoverflow.com/questions/50740632/how-do-i-call-a-function-that-requires-a-static-lifetime-with-a-variable-create)

- [What to do if an external crate requires static lifetime explicitly?](https://stackoverflow.com/questions/63106684/what-to-do-if-an-external-crate-requires-static-lifetime-explicitly)


    