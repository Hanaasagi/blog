
+++
title = "Kurumi Atelier Day9"
summary = ''
description = ""
categories = []
tags = []
date = 2018-01-28T12:45:12+08:00
draft = false
+++

IDT(Interrupt Descriptor Table)，中断描述符表。每一种中断对应一个中断号。CPU 执行中断指令时，会去 IDT 中查找对应的中断服务程序(interrupt service routine ，ISR)。中断处理过程是由 CPU 直接调用的，CPU 有专门的寄存器 IDTR 来保存 IDT 在内存中的位置。IDTR 有 48 位，前 32 位是 IDT 在内存中的地址，后 16 位是 IDT 的大小。可以使用 LIDT 和 SIDT 指令来读写 IDTR。中断过程中可以产生新的中断。中断时有优先级的，高优先级的中断可以“中断”低优先级的中断。有的 ISR 不能被中断，可以使用 STI(set interrupt-enable flag) 和 CLI(clear interrupt-enable flag) 设置 IF 标志来启动和关闭中断

新建工程 `interrupts`，参考 [AMD64 IDT结构 - OSDev Wiki](https://wiki.osdev.org/Interrupt_Descriptor_Table#Structure_AMD64) 将 `IdtEntry` 实现如下，占用 16 字节

```Rust
#[derive(Debug, Clone, Copy)]
#[repr(C, packed)]
pub struct IdtEntry {
    offsetl:  u16,   // offset bits 0..15
    selector: u16,   // a code segment selector in GDT or LDT
    ist:      u8,    // bits 0..2 holds Interrupt Stack Table offset, rest of bits zero.
    flags:    u8,    // type and attributes
    offsetm:  u16,   // offset bits 16..31
    offseth:  u32,   // offset bits 32..63
    zero:     u32,   // reserved
}
```

`offset` 应当是一个 64 bit 的值，表示 ISR 的地址，这里被分成三份(`offsetl`, `offsetm`, `offseth`)存储

`flags` 8 bit 结构即含义如下

```
7                           0
+---+---+---+---+---+---+---+---+
| P |  DPL  | S |    GateType   |
+---+---+---+---+---+---+---+---+
```

- P: One bit, indicating if the ISR is present. Set to 0 for unused interrupts.
- DPL: Two bits, indicating the escriptor's priveliege level as an integer, with zero being Ring 0.
- S: One bit, set if the descriptor refers to an interrupt in the storage segment.
- GateType: Four bits, indicating the type of the interrupt with the following values (architecture-dependent):
  - 0101: 80386 32-bit task gate
  - 0110: 80286 16-bit interrupt gate
  - 0111: 80286 16-bit trap gate
  - 1110: 80386 32-bit interrupt gate
  - 1111: 80386 32-bit trap gate

添加创建一个 `IdtEntry` 的方法

```Rust
bitflags! {
    pub struct IdtFlags: u8 {
        const PRESENT   = 1 << 7;
        const RING_0    = 0 << 5;
        const RING_1    = 1 << 5;
        const RING_2    = 2 << 5;
        const RING_3    = 3 << 5;
        const SS        = 1 << 4;
        const INTERRUPT = 0xE;
        const TRAP      = 0xF;
    }
}

impl IdtEntry {

    pub const fn new() -> IdtEntry {
        IdtEntry {
            offsetl:   0,
            selector:  0,
            ist:      0,
            flags: 0,
            offsetm:   0,
            offseth:   0,
            zero:     0
        }
    }

    pub fn set_flags(&mut self, flags: IdtFlags) {
        self.flags = flags.bits;
    }

    pub fn set_offset(&mut self, selector: u16, base: usize) {
        self.selector = selector;
        self.offsetl = base as u16;
        self.offsetm = (base >> 16) as u16;
        self.offseth = (base >> 32) as u32;
    }

    pub fn set_func(&mut self, func: unsafe extern fn()) {
        self.set_flags(IdtFlags::PRESENT | IdtFlags::RING_0 | IdtFlags::INTERRUPT);
        // code segment
        self.set_offset(0x08, func as usize);
    }
}
```

`0x08` 为我们 code segment 的偏移量，可以回顾一下我们当初 `GDTT` 的定义

```nasm
section .rodata
gdt64:
    dq 0
.code: equ $ - gdt64
    dq  (1<<41) | (1<<44) | (1<<47) | (1<<43) | (1<<53)
.data: equ $ - gdt64
    dq (1<<44) | (1<<47) | (1<<41)
.pointer:
    dw $ - gdt64 - 1
    dq gdt64
```

实现 IDTR 结构

<table>
<tbody><tr>
<td>Offset </td>
<td> Size </td>
<td> Description
</td></tr>
<tr>
<td>0 </td>
<td> 2 </td>
<td> Limit - Maximum addressable byte in table
</td></tr>
<tr>
<td>2 </td>
<td> 8 </td>
<td> Offset - Linear (paged) base address of IDT
</td></tr></tbody></table>

```Rust
use core::mem::size_of;

use idt::IdtEntry;

#[repr(C, packed)]
pub struct DescriptorTablePointer {
    // Size of the DT.
    pub limit: u16,
    // Pointer to the memory region containing the IDT.
    pub base: *const IdtEntry,
}

impl DescriptorTablePointer {
    fn new(slice: &[IdtEntry]) -> Self {
        let len = slice.len() * size_of::<IdtEntry>();
        assert!(len < 0x10000);
        DescriptorTablePointer {
            base:  slice.as_ptr(),
            limit: len as u16,
        }
    }

    pub fn new_idtp(idt: &[IdtEntry]) -> Self {
        Self::new(idt)
    }
}

// Load IDT table.
pub unsafe fn lidt(idt: &DescriptorTablePointer) {
    asm!("lidt ($0)" :: "r" (idt) : "memory");
}
```

需要注意

**In your interrupt handler routines, remember to use IRETQ instead of IRET, as nasm won't translate that for you. Many 64bit IDT related problems on the forum are caused by that missing 'Q'. Don't let this happen to you.**

### Reference
[Interrupt Descriptor Table - OSDev Wiki](https://wiki.osdev.org/Interrupt_Descriptor_Table)

    