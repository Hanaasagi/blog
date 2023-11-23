
+++
title = "Kurumi Atelier Day7"
summary = ''
description = ""
categories = []
tags = []
date = 2018-01-26T13:32:22+08:00
draft = false
+++

上一次封装了 vga 的部分操作，但是调用起来有点麻烦。Rust 提供了 macro 机制。定义 macro 需要使用 `macro_rules!`，貌似这也是个 macro，不过我没查到定义

macro 定义了一系列的规则，如果匹配到相应的 `matcher` 则会执行对应的处理逻辑

```Rust
macro_rules! test {
    (x => $e:expr) => (println!("mode X: {}", $e));
    (y => $e:expr) => (println!("mode Y: {}", $e));
}

fn main() {
    test!(y => 3);
}

// output:
// mode Y: 3
```

还可以利用 `*`，`+` 等进行重复匹配

```Rust
macro_rules! test {
    (x => ($( $e:expr ),*)) => ($( println!("mode X: {}", $e);)*);
    (y => ($( $e:expr ),*)) => ($( println!("mode Y: {}", $e);)*);
}

fn main() {
    test!(x => (3, 4, 5));
}

// output:
// mode X: 3
// mode X: 4
// mode X: 5
```

其中 matcher 中可以使用

- `ident`: an identifier. Examples: `x`; `foo`.
- `path`: a qualified name. Example: `T::SpecialA`.
- `expr`: an expression. Examples: `2 + 2`; `if true { 1 } else { 2 }; f(42`).
- `ty`: a type. Examples: `i32`; `Vec<(char, String)>`; `&T`.
- `pat`: a pattern. Examples: `Some(t)`; `(17, 'a')`; `_`.
- `stmt`: a single statement. Example: `let x = 3`.
- `block`: a brace-delimited sequence of statements and optionally an expression. Example: `{ log(error, "hi"); return 12; }`.
- `item`: an item. Examples: `fn foo() { }`; `struct Bar`;.
- `meta`: a "meta item", as found in attributes. Example: `cfg(target_os = "windows")`.
- `tt`: a single token tree.

token tree 可以为

- a sequence of token trees surrounded by matching `()`, `[]`, or `{}`, or
- any other single token.

在 `vga/src/lib.rs` 中添加如下 macro

```Rust
pub fn kprint(args: fmt::Arguments) {
    WRITER.lock().write_fmt(args).unwrap();
}

#[macro_export]
macro_rules! kprint {
    ($($arg:tt)*) => ({
        $crate::kprint(format_args!($($arg)*));
    });
}

#[macro_export]
macro_rules! kprintln {
    ($fmt:expr) => (kprint!(concat!($fmt, "\n")));
    ($fmt:expr, $($arg:tt)*) => (kprint!(concat!($fmt, "\n"), $($arg)*));
}
```

`src/lib.rs` 中添加如下代码

```Rust
#[macro_use]
extern crate vga;
```

我们便可以在 `kmain` 中使用 `kprintln!` 了

```Rust
#[no_mangle]
pub extern fn kmain() -> ! {
  kprintln!("Welcome to Japari Park")j;
  loop {}
}
```

### Reference
[Macros - The Rust Programming Language](https://doc.rust-lang.org/book/first-edition/macros.html)

    