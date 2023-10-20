+++
title = 'Hello world'
summary = 'This is a test page'
description = ""
categories = []
tags = []
date = 2023-10-19T13:46:21+09:00
draft = false
+++

**GRUB loading...**  
**Welcome to GRUB!**  
**Loading kernel, hugo v0.119.0, theme blowfish**  
**Booting the blog system...**  
**Mounting all posts...**  
**Starting markdown renderer...**

这是一个测试页面，用于调整样式

---

## Headers

# H1

some text

## H2

some text

### H3

some text

#### H4

some text

##### H5

some text

## Emphasis

_This text will be italic._  
_This will also be italic._  
**This text will be bold.**  
**This will also be bold.**  
_You **can** combine them._

## Lists

### Unordered

- Item 1
- Item 2
  - Item 2a
  - Item 2b

### Ordered

1. Item 1
2. Item 2
3. Item 3
   1. Item 3a
   2. Item 3b

### Task Lists

- [ ] Task 1
- [x] Task 2

## Links

- [Google](https://www.google.com.hk/)
- [GitHub](https://github.com/)

## Image

![](https://camo.githubusercontent.com/f9a322c724f1cbb47a2bbb5407a1abbd9b1f2a7481f0fce08bd177b59719e1b9/68747470733a2f2f6f63746f6465782e6769746875622e636f6d2f696d616765732f68756c615f6c6f6f705f6f63746f64657830332e676966)

## Blockquotes

> This is a blockquote.
>
> - A. Person

## Code

```rust
// This is a comment, and is ignored by the compiler.
// You can test this code by clicking the "Run" button over there ->
// or if you prefer to use your keyboard, you can use the "Ctrl + Enter"
// shortcut.

// This code is editable, feel free to hack it!
// You can always return to the original code by clicking the "Reset" button ->

// This is the main function.
fn main() {
    // Statements here are executed when the compiled binary is called.

    // Print text to the console.
    println!("Hello World!");
}
```

## Iframe

<iframe frameborder="no" border="0" marginwidth="0" marginheight="0" width=330 height=86 src="//music.163.com/outchain/player?type=2&id=1876388990&auto=1&height=66"></iframe>

## Table

| 姓名 | 年龄 | 城市 |
| ---- | ---- | ---- |
| 张三 | 25   | 北京 |
| 李四 | 30   | 上海 |
| 王五 | 28   | 广州 |

## Math Equations

You can write math equations by katex

{{< katex >}}

$$
\int_2^3 \exp(10^x) \, dx = \left[ \frac{10^x}{\ln(x)} \right]_2^3 = \frac{10^2}{\ln(2)} - \frac{10^1}{\ln(1)} \approx 765.97
$$

## GitHub Card

{{< github repo="Hanaasagi/blog" >}}

Welcome to Ubuntu 18.04.5 LTS (GNU/Linux 5.4.0-72-generic x86_64)

Username: _  
Password: _
