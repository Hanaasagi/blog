
+++
title = "Kurumi Atelier Day13"
summary = ''
description = ""
categories = []
tags = []
date = 2018-02-01T13:43:28+08:00
draft = false
+++

*本文参考了[Allocating Frames | Writing an OS in Rust](https://os.phil-opp.com/allocating-frames/)*  
断更了两天，因为不知道接下来该怎么做，所以翻看了两个开源项目(C 实现)的源代码

[OS67](https://github.com/SilverRainZ/OS67)  
[fleurix](https://github.com/Fleurer/fleurix)

接下来打算搞搞内存分配。当使用 paging 时，物理内存被划分为等大小的块(通常是 4096 字节)。这些块便就是之前提到的帧(frame)，通过页表和虚拟分页进行映射。所以这里实现一个简单的 Frame Allocator。它仅保持一个简单的计数器，从 0 开始线性增长。如果当前的 Frame 可以分配，并且没有被内核使用，或者 multiboot 使用，则分配并返回。不过这里的问题是无法去释放

新建一个工程 `memory`，结构如下

```
memory
├── Cargo.toml
└── src
    ├── bump.rs
    ├── frame.rs
    └── lib.rs
```

首先编写一个结构体 `Frame`

```Rust
// memory/src/frame.rs
#[derive(Debug, PartialEq, Eq, PartialOrd, Ord)]
pub struct Frame {
    pub number: usize,
}
```

我们将 Frame 的大小设置为 4KB，并添加一个方法返回物理地址对应的 Frame

```Rust
// memory/src/frame.rs
const PAGE_SIZE: usize = 4096;

impl Frame {
    pub fn containing_address(address: usize) -> Frame {
        Frame{ number: address / PAGE_SIZE }
    }
}
```

定义一个 trait(这不是必须的)

```Rust
// memory/src/frame.rs
pub trait FrameAllocator {
    fn alloc(&mut self) -> Option<Frame>;
    fn free(&mut self, frame: Frame);
}
```

编写 allocator 之前，我们需要先来获取 kernel 和 multiboot 已占用内存区域的起始地址。当使用 multiboot 去加载 kernel 时，其会在 `ebx` 寄存器上存储 boot information structure 的结构体地址。所以我们可以将这个地址作为参数传入 `kmain` 中，然后去读取此次启动的信息。寄存器 `RDI, RSI, RDX, RCX, R8, R9` 便是 `kmain` 可以接收的参数。

但是 multiboot 加载后，我们是处于 32 位模式，所以只能使用 `edi` 来存储

```nasm
; boot.asm
start:
    mov esp, stack_top
    mov edi, ebx
```

借助 multiboot2 这个 crate 可以帮助我们读取信息

```Rust
// src/kernel.rs
extern crate muyltiboot2;

pub extern fn kmain(multiboot_info_addr: usize) -> ! {
    let boot_info = unsafe{ multiboot2::load(multiboot_info_addr) };
    let elf_sections_tag = boot_info.elf_sections_tag()
    .expect("Elf-sections tag required");

    let kernel_start = elf_sections_tag.sections().map(|s| s.addr)
        .min().unwrap();
    let kernel_end = elf_sections_tag.sections().map(|s| s.addr + s.size)
        .max().unwrap();

    let multiboot_start = multiboot_info_addr;
    let multiboot_end = multiboot_start + (boot_info.total_size as usize);

    kprintln!("kernel starts at 0x{:08x}, ends at 0x{:08x}",
              kernel_start, kernel_end);
    kprintln!("multiboot starts at 0x{:08x}, ends at 0x{:08x}",
              multiboot_start, multiboot_end);
}
```

编写 `BumpAllocator`

```Rust
// memory/src/bump.rs
use multiboot2::{MemoryAreaIter, MemoryArea};

pub struct BumpAllocator {
    next_free_frame: Frame,
    current_area: Option<&'static MemoryArea>,
    areas: MemoryAreaIter,
    kernel_start: Frame,
    kernel_end: Frame,
    multiboot_start: Frame,
    multiboot_end: Frame,
}

impl BumpAllocator {
    pub fn new(kernel_start: usize, kernel_end: usize,
               multiboot_start: usize, multiboot_end: usize,
               memory_areas: MemoryAreaIter) -> BumpAllocator {
        let mut allocator = BumpAllocator {
            next_free_frame: Frame::containing_address(0),
            current_area:    None,
            areas:           memory_areas,
            kernel_start:    Frame::containing_address(kernel_start),
            kernel_end:      Frame::containing_address(kernel_end),
            multiboot_start: Frame::containing_address(multiboot_start),
            multiboot_end:   Frame::containing_address(multiboot_end),
        };
        allocator.choose_next_area();
        allocator
    }
}
```
`next_free_frame` 就是计数器，所有低于 `next_free_frame.number` 的 frame 都被认为已使用。这里再次接住了 `multiboot2` crate 的 `MemoryAreaIter` 和 `MemoryArea`。`MemoryAreaIter` 它根据 boot information structure 读取划分的内存区块，然后以迭代器的形式返回一个个 `MemoryArea`。`current_area` 表示包含 `next_free_frame` 的那块内存，`areas` 表示迭代器

下面实现 `alloc` 和 `free`

```Rust
impl FrameAllocator for BumpAllocator {

    fn alloc(&mut self) -> Option<Frame> {
        if let Some(area) = self.current_area {
            let frame = Frame {
                number: self.next_free_frame.number
            };

            // 根据 current_area 的尾地址计算所处于的 frame
            let current_area_last_frame = {
                let address = area.base_addr + area.length - 1;
                Frame::containing_address(address as usize)
            };

            if frame > current_area_last_frame {
                // 当 current_area 的内存被分配完毕后，更新 current_area
                self.choose_next_area();
            } else if frame >= self.kernel_start && frame <= self.kernel_end {
                // frame 被 kernel 占用
                self.next_free_frame = Frame {
                    number: self.kernel_end.number + 1
                };
            } else if frame >= self.multiboot_start && frame <= self.multiboot_end {
                // frame 被 multiboot information structure 占用
                self.next_free_frame = Frame {
                    number: self.multiboot_end.number + 1
                };
            } else {
                self.next_free_frame.number += 1;
                return Some(frame);
            }
            // 再次尝试分配
            self.alloc()
        } else {
            None // 内存用尽
        }
    }

    fn free(&mut self, _frame: Frame) {
        unimplemented!()
    }
}
```

`choose_next_area` 的实现

```Rust
impl BumpAllocator {
    fn choose_next_area(&mut self) {
        self.current_area = self.areas.clone().filter(|area| {
            let address = area.base_addr + area.length - 1;
            Frame::containing_address(address as usize) >= self.next_free_frame
        }).min_by_key(|area| area.base_addr);

        if let Some(area) = self.current_area {
            let start_frame = Frame::containing_address(area.base_addr as usize);
            if self.next_free_frame < start_frame {
                self.next_free_frame = start_frame;
            }
        }
    }
}
```
### Reference
[Allocating Frames | Writing an OS in Rust](https://os.phil-opp.com/allocating-frames/)

    