
+++
title = "Kurumi Atelier Day6"
summary = ''
description = ""
categories = []
tags = []
date = 2018-01-25T14:30:00+08:00
draft = false
+++

将 vga 打印部分进行简单的封装，`cargo new vga` 创建一个新的项目

`Cargo.toml` 中添加

```
[dependencies.vga]
path = "vga"
```

<table><tr>
<th colspan="8">Attribute</th>
<th colspan="8">Character</th>
</tr>
<tr>
<td align="center" width="48">7</td>
<td align="center" width="48">6</td>
<td align="center" width="48">5</td>
<td align="center" width="48">4</td>
<td align="center" width="48">3</td>
<td align="center" width="48">2</td>
<td align="center" width="48">1</td>
<td align="center" width="48">0</td>
<td align="center" width="48">7</td>
<td align="center" width="48">6</td>
<td align="center" width="48">5</td>
<td align="center" width="48">4</td>
<td align="center" width="48">3</td>
<td align="center" width="48">2</td>
<td align="center" width="48">1</td>
<td align="center" width="48">0</td>
</tr>
<tr>
<td align="center">Blink</td>
<td colspan="3" align="center">Background color</td>
<td colspan="4" align="center">Foreground color</td>
<td colspan="8" align="center">Code point</td>
</tr>
</table>


编写 `vga/src/lib.rs`，记得添加 `#![no_std]`。第一步来编写色表

```Rust
#[allow(dead_code)]
#[repr(u8)]
pub enum Color {
    Black      = 0,
    Blue       = 1,
    Green      = 2,
    Cyan       = 3,
    Red        = 4,
    Magenta    = 5,
    Brown      = 6,
    LightGray  = 7,
    DarkGray   = 8,
    LightBlue  = 9,
    LightGreen = 10,
    LightCyan  = 11,
    LightRed   = 12,
    Pink       = 13,
    Yellow     = 14,
    White      = 15,
}
```

`repr(u8)` 指定枚举中每一个元素使用 `u8` 来进行存储，实际上 `u4` 就够了，但是 Rust 中没有此类型

颜色由前景色和背景色组成，使用 `ColorCode` 结构体来表示

```Rust
#![feature(const_fn)]

#[derive(Debug, Clone, Copy)]
struct ColorCode(u8);

impl ColorCode {
    const fn new(fgcolor: Color, bgcolor: Color) -> ColorCode {
        ColorCode((bgcolor as u8) << 4 | (fgcolor as u8))
    }
}
```

接下来用 `ScreenChar` 结构体表示一个字符单元，包含颜色和 ASCII

```Rust
#[repr(C)]
#[derive(Debug, Clone, Copy)]
struct ScreenChar {
    ascii_char: u8,
    color_code: ColorCode,
}
```

一屏容纳 80 × 25 字符

```Rust
const BUFFER_HEIGHT: usize = 25;
const BUFFER_WIDTH: usize = 80;

struct Buffer {
    chars: [[ScreenChar; BUFFER_WIDTH]; BUFFER_HEIGHT]
}
```

编写 `Writer`，记录当前光标位置，并提供方法打印

```Rust
#![feature(unique)]

use core::ptr::Unique;

pub struct Writer {
    column_position: usize,
    color_code: ColorCode,
    buffer: Unique<Buffer>,
}
```

因为之后要创建 `static` 的 `Writer` 所以这里使用 `Unique`

```Rust
use core::fmt;

impl fmt::Write for Writer {
    fn write_str(&mut self, s: &str) -> fmt::Result {
        for byte in s.bytes() {
          self.write_byte(byte)
        }
        Ok(())
    }
}

impl Writer {
    pub fn write_byte(&mut self, byte: u8) {
        match byte {
            b'\n' => self.new_line(),
            byte => {
                if self.column_position >= BUFFER_WIDTH {
                    self.new_line();
                }

                let row = BUFFER_HEIGHT - 1;
                let col = self.column_position;

                let color_code = self.color_code;
                self.buffer().chars[row][col] = ScreenChar {
                    ascii_char: byte,
                    color_code: color_code,
                };
                self.column_position += 1;
            }
        }
    }

    fn buffer(&mut self) -> &mut Buffer {
        unsafe{ self.buffer.as_mut() }
    }

    fn new_line(&mut self) {
        for row in 1..BUFFER_HEIGHT {
            for col in 0..BUFFER_WIDTH {
                let buffer = self.buffer();
                let character = buffer.chars[row][col];
                buffer.chars[row - 1][col] = character;
            }
        }
        self.clear_row(BUFFER_HEIGHT-1);
        self.column_position = 0;
    }

    fn clear_row(&mut self, row: usize) {
        let blank = ScreenChar {
            ascii_char: b' ',
            color_code: self.color_code,
        };
        for col in 0..BUFFER_WIDTH {
            self.buffer().chars[row][col] = blank;
        }
    }

}
```

创建 `static` 的 `Writer`，由于 `write_byte` 需要 `&mut`，而我们的变量是 `static` 的，所以通过 `Mutex` 来避免不安全的读写。由于不使用标准库，所以这里使用了 `spin` crate

```Rust
#![feature(const_unique_new)]

extern crate spin;
use spin::Mutex;

pub static WRITER: Mutex<Writer> = Mutex::new(Writer {
    column_position: 0,
    color_code: ColorCode::new(Color::White, Color::Blue),
    buffer: unsafe { Unique::new_unchecked(0xb8000 as *mut _) },
});
```

现在我们便可以使用经过封装的 `vga` crate 了

```Rust
extern crate vga;

pub extern fn kmain() -> ! {
    vga_buffer::WRITER.lock().write_str("Hello again");
}
```

若发现在编译时出现如下错误

```
target/x86_64-kurumi/release/libkurumi.a(core-be3bd474730d9ea4.core0-ec52c816b790ee3f7b9beb191610e0af.rs.rcgu.o): In function `core::fm
t::num::<impl core::fmt::Debug for i128>::fmt':
core0-ec52c816b790ee3f7b9beb191610e0af.rs:(.text._ZN4core3fmt3num51_$LT$impl$u20$core..fmt..Debug$u20$for$u20$i128$GT$3fmt17he1c0b811bd
433411E+0x71): undefined reference to `__umodti3'
core0-ec52c816b790ee3f7b9beb191610e0af.rs:(.text._ZN4core3fmt3num51_$LT$impl$u20$core..fmt..Debug$u20$for$u20$i128$GT$3fmt17he1c0b811bd
433411E+0x86): undefined reference to `__udivti3'
```

这些函数是 LLVM 的 [compiler-rt builtins](https://compiler-rt.llvm.org/)，通常和标准库连接。但是我们现在不使用标准库，所以会出现上面的错误。由于我们写的 kernel 不会用到上面的函数，所以可以使用 `--gc-sections` 移除不使用的 section 来解决

但是这样改，qemu 运行时会报 `error: no multiboot header found.`。这是因为 `boot` section 也不行被移除了。通过 `KEEP` 命令显式标记一下就行了

### Reference
[Printing to Screen | Writing an OS in Rust](https://os.phil-opp.com/printing-to-screen/)

    