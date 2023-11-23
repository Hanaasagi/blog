
+++
title = "Combinators in Rust"
summary = ''
description = ""
categories = []
tags = []
date = 2018-01-18T09:32:16+08:00
draft = false
+++

Rust 中的 `Option` 和 `Result` 类型和 Haskell 中的 `Maybe` 与 `Either` 十分相似，并且借鉴了许多函数式编程理念

#### 写在前面的扯淡

Rust 是一门没有异常处理的语言，对比同是没有异常处理的 Go。在 Go 中是通过多值返回的形式，返回异常信息

```Go
inputFile, inputError := os.Open("input.dat")
if inputError != nil {
    fmt.Printf("An error occurred on opening the inputfile\n")
    return // exit the function on error
}
defer inputFile.Close()
```

因为 Go 有 `Null`(`nil`) 值，所以它可以这么任性。而 Rust 中则是通过 `Result`(`Option`) 来做的，它相当于一个箱子，里面包裹了正常的返回值或者异常。我们可以通过一些 combinator function 来对此类型进行进一步的处理，最后将其拆开得到结果值或者异常。代码写起来就像是一个 pipeline

本文以 `Result` 为例(其定义如下)介绍这些 combinator function

```Rust
enum Result<T, E> {
   Ok(T),
   Err(E),
}
```

#### `and_then`

`and_then` combinator 当且仅当枚举类型 `Result` 为 `Ok(T)` 时才会调用作为参数的 closure。

```Rust
let res: Result<u8, &'static str> = Ok(5);
let value = res.and_then(|n: u8| Ok(n * 2));
assert_eq!(Ok(10), value);

// 与下面的写法等价

let value = match res {
    Ok(n) => Ok(n * 2),
    Err(e) => Err(e),
};
assert_eq!(Ok(10), value);

// 如果为 Err 则不会调用
let res: Result<u8, &'static str> = Err("error");
let value = res.and_then(|n: u8| Ok(n * 2));
assert_eq!(value, res);
```

我们可以将多个 `and_then` 进行串联

```Rust
let res: Result<u8, &'static str> = Ok(0);
let value = res.and_then(|n: u8| {
    if n == 0 {
        Err("cannot divide by zero")
    } else {
        Ok(n)
    }
}).and_then(|n: u8| Ok(2 / n));
assert_eq!(Err("cannot divide by zero"), value);
```

在 F# 中这种编程范式被称为 Railway Oriented Programming，其使用的是 `bind` 函数来完成

```F#
let bind f opt =  
  match opt with
  | Some v -> f v
  | None -> None
```

在 Haskell 中可以认为 Result 实现了 Monad 的 typeclass

```Haskell
Prelude> let divby x = if x == 0 then Nothing else Just(2 / x)
Prelude> Just(0) >>= divby
Nothing
Prelude> Just(2) >>= divby
Just 1.0
```

`and_then` 的源码如下

```Rust
#[inline]
#[stable(feature = "rust1", since = "1.0.0")]
pub fn and_then<U, F: FnOnce(T) -> Result<U, E>>(self, op: F) -> Result<U, E> {
    match self {
        Ok(t) => op(t),
        Err(e) => Err(e),
    }
}
```

#### `map`

`map` combinator 可以用于转换，注意这个和 `Iterator` 的 `map` 不同。`Result` 类型的 `map` 函数当且仅当枚举类型 `Result` 为 `Ok(T)` 时才会调用作为参数的 closure。好吧，这玩意听起来貌似和 `and_then` 一样

```Rust
// use `and_then`
let res: Result<u8, &'static str> = Ok(5);
let value = res.map(|n: u8| n * 2);
assert_eq!(Ok(10), value);

// use `map`
let res: Result<u8, &'static str> = Ok(5);
let value = res.and_then(|n: u8| Ok(n * 2));
assert_eq!(Ok(10), value);
```

看起来貌似也和 `and_then` 的示例差不多。不过，请仔细看 closure 的返回值。`map` 总是会将返回值包装成 `OK`，所以这里我们直接返回 `n * 2`，而在 `and_then` 中我们需要返回 `Ok(n * 2)`。说白了就是 Haskell 中的 Functor

```Haskell
Prelude> fmap (*2) (Just 5)
Just 10
Prelude> fmap (*2) (Nothing)
Nothing
```

Rust 中 `map` 的源码如下

```Rust
#[inline]
#[stable(feature = "rust1", since = "1.0.0")]
pub fn map<U, F: FnOnce(T) -> U>(self, op: F) -> Result<U,E> {
    match self {
        Ok(t) => Ok(op(t)),
        Err(e) => Err(e)
    }
}
```

#### `map_err`
`map_err` 是一个相对于 `map` 的函数，它只在 `Result` 实际为 `Err(E)` 的时候才会调用 closure

```Rust
use std::io::{Error, ErrorKind};

fn main() {
    let res: Result<u8, Error> = Err(Error::new(ErrorKind::Other, "oh no!"));

    let value = res.map(|n: u8| n * 2)
        .map_err(|_e: Error| "find a mistake");

    assert_eq!(Err("find a mistake"), value);
}
```

#### `or_else`

既然有相对于 `map` 的 `map_err`，那么也有相对于 `and_then` 的 `or_else`

```Rust
let res: Result<u8, &'static str> = Err("oh no!");

let value = res.or_else(|_s: &str| Ok(2))
    .or_else(|_s: &str| Err("oh no!!"));

assert_eq!(Ok(2), value);
```

#### Conclusion

简单来说就是 `map` 可以将 `OK(T)` 转换成 `OK(U)`，`and_then` 可以将 `OK(T)` 变成 `OK(U)` 或者 `Err(F)`

推荐阅读 [Functors, Applicatives, And Monads In Pictures](http://adit.io/posts/2013-04-17-functors,_applicatives,_and_monads_in_pictures.html)  
扩展阅读 [Option Monads in Rust](https://hoverbear.org/2014/08/12/option-monads-in-rust/) *注意此文章中的 Rust 代码版本比较低*

    