
+++
title = "Kurumi Atelier Day2"
summary = ''
description = ""
categories = []
tags = []
date = 2018-01-21T07:39:06+08:00
draft = false
+++

目前我们处于 protected mode(32bit) 接下面来切换到 long mode。long mode 包含两种子模式:64-bit mode 和 compatibility mode(32bit)。这里我们想要的是 64-bit mode。在这种模式下寄存器扩展到 64 位(rax, rcx, rdx 等)，并且增加了新的 8 个通用寄存器(r8-r15)

### CPUID

`CPUID` 指令可以用于获取 CPU 的信息，我们可以用其来检测 long mode 是否存在。首先需要判断是否支持 `CPUID` 指令(代码从 [OSDev WiKi](http://wiki.osdev.org/Setting_Up_Long_Mode#x86_or_x86-64)) 上拷贝的)

```
; Check if CPUID is supported by attempting to flip the ID bit (bit 21) in
; the FLAGS register. If we can flip it, CPUID is available.

; Copy FLAGS in to EAX via stack
pushfd
pop eax

; Copy to ECX as well for comparing later on
mov ecx, eax

; Flip the ID bit
xor eax, 1 << 21

; Copy EAX to FLAGS via the stack
push eax
popfd

; Copy FLAGS back to EAX (with the flipped bit if CPUID is supported)
pushfd
pop eax

; Restore FLAGS from the old version stored in ECX (i.e. flipping the ID bit
; back if it was ever flipped).
push ecx
popfd

; Compare EAX and ECX. If they are equal then that means the bit wasn't
; flipped, and CPUID isn't supported.
xor eax, ecx
jz .no_cpuid
ret
```

如果不支持我们需要将这一信息打印到屏幕上,所以我们需要制作一个通用的错误处理

```
; boot.asm

; Prints `ERR: ` and the given error code to screen and hangs.
; parameter: error code (in ascii) in al
error:
    mov WORD [0xb8000], 0x1f45
    mov WORD [0xb8002], 0x1f52
    mov WORD [0xb8004], 0x1f52
    mov WORD [0xb8006], 0x1f3a
    mov WORD [0xb8008], 0x1f20
    mov BYTE [0xb800A], al
    hlt
```

接下来我们需要一个栈空间(stack)，保存调用函数时的返回地址。

```
section .bss
stack_bottom:
    resb 64
stack_top:
```

初始化 `esp` 寄存器

```
section .text
bits 32
start:
    mov esp, stack_top
```

增加检测支持 `CPUID` 的

```
; boot.asm

check_cpuid:
    ; 上面的代码
.no_cpuid:
    mov al, "1"
    jmp error
```

以 `.` 开头的 Label 称为 [Local Label](http://www.tortall.net/projects/yasm/manual/html/nasm-local-label.html)，它会和前面最近的 non-local lable 进行关联

### x86 or x86-64

接下来检测是否可以进入 long mode

```
; boot.asm

check_long_mode:
    mov eax, 0x80000000    ; Set the A-register to 0x80000000.
    cpuid                  ; CPU identification.
    cmp eax, 0x80000001    ; Compare the A-register with 0x80000001.
    jb .no_long_mode       ; if it's less, the CPU is too old for long mode

    ; use extended info to test if long mode is available
    mov eax, 0x80000001    ; Set the A-register to 0x80000001.
    cpuid                  ; CPU identification.
    test edx, 1 << 29      ; Test if the LM-bit, which is bit 29, is set in the D-register.
    jz .no_long_mode       ; If it's not set, there is no long mode
    ret
.no_long_mode:
    mov al, "2"
    jmp error
```

### merge

在 `start` 后应当立即进行这些检测

```
section .text
bits 32
start:
    mov esp, stack_top
    call check_cpuid
    call check_long_mode
```

### Auto Make

关于 Makefile 的语法可以参考 [跟我一起写 Makefile](http://wiki.ubuntu.org.cn/%E8%B7%9F%E6%88%91%E4%B8%80%E8%B5%B7%E5%86%99Makefile)

```
arch ?= x86_64
kernel := build/kernel-$(arch).bin
iso := build/os-$(arch).iso

linker_script := src/arch/$(arch)/linker.ld
grub_cfg := src/arch/$(arch)/grub.cfg
assembly_source_files := $(wildcard src/arch/$(arch)/*.asm)
assembly_object_files := $(patsubst src/arch/$(arch)/%.asm, \
    build/arch/$(arch)/%.o, $(assembly_source_files))

.PHONY: all clean run iso

all: $(kernel)

clean:
    @rm -r build

run: $(iso)
    @qemu-system-x86_64 -cdrom $(iso)

iso: $(iso)

$(iso): $(kernel) $(grub_cfg)
    @mkdir -p build/isofiles/boot/grub
    @cp $(kernel) build/isofiles/boot/kernel.bin
    @cp $(grub_cfg) build/isofiles/boot/grub
    @grub-mkrescue -o $(iso) build/isofiles 2> /dev/null
    @rm -r build/isofiles

$(kernel): $(assembly_object_files) $(linker_script)
    @ld -n -T $(linker_script) -o $(kernel) $(assembly_object_files)

# compile assembly files
build/arch/$(arch)/%.o: src/arch/$(arch)/%.asm
    @mkdir -p $(shell dirname $@)
    @nasm -f elf64 $< -o $@
```

### Reference
[Setting Up Long Mode - OSDev Wiki](http://wiki.osdev.org/Setting_Up_Long_Mode#x86_or_x86-64)

    