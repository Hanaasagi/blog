
+++
title = "利用 foldl 实现 foldr"
summary = ''
description = ""
categories = []
tags = []
date = 2017-07-01T11:47:50+08:00
draft = false
+++

*水文一篇*  
`fold` 是常用的聚合函数，一般有两种形式：从列表头部至尾部的 `foldl`，从列表尾部至头部的 `foldr`(他们的区别不只是这样，后面会说明)，大致实现如下

```
foldl :: (b -> a -> b) -> b -> [a] -> b
foldl _ acc [] = acc
foldl f acc (x:xs) = foldl f (f acc x) xs


foldr :: (a -> b -> b) -> b -> [a] -> b
foldr _ acc [] = acc
foldr f acc (x:xs) = f x (foldr f acc xs)
```

那么下面考虑一个问题如何使用 `foldl` 来实现 `foldr`(或者由 `foldr` 实现 `foldl`)

先来观察一下 `foldl` 的展开，比如 `foldl (+) 0 [1, 2, 3]`

```
foldl (+) 0 [1, 2, 3]
foldl (+) ((+) 0 1) [2, 3]
foldl (+) ((+) ((+) 0 1) 2) [3]
foldl (+) ((+) ((+) ((+) 0 1) 2) 3) []
(+) ((+) ((+) 0 1) 2) 3
((+) ((+) 0 1) 2) + 3
(((+) 0 1) + 2) + 3
((0 + 1) + 2) + 3
```

很显然只要将

```
(+) ((+) ((+) 0 1) 2) 3
```

想办法搞成这个样子就行了

```
-- 0 是初始值
(+) ((+) ((+) 0 3) 2) 1
```

不妨让我们将初始值抽出，让其成为一个函数

```
\x -> f (... (f (f (f x bn) bn-1) bn-2) ... b0)
```

这样我们只要传入初始值就可以了。再从上面的式子抽象出共同的部分，可以看作是下面式子反复 apply 所得

```
-- g 为累加值 acc
\x -> g (f x b0)
\x -> (\x -> g (f x b0)) (f x b1)
-- 化简后
\x -> (g (f (f x b1) b0))
```

很明显 `g` 是多余的，所以可以利用一下 Haskell 中的 `id`，它能够将值原样返回，所以函数应当是这个样子的

```
\g b ->
  (\x -> g (f x b))
```

由于 Haskell 提供的部分应用的特性，可以改写成下面这样

```
\g b x -> g (f x b)
```

最终如下所示

```
foldr f a bs = (foldl (\g b x -> g (f x b)) id bs) a
-- 由于函数优先级默认最高，所以等价下面表示
foldr f a bs = foldl (\g b x -> g (f x b)) id bs a
```

这个运行结果和原本的 `foldr` 在做 `+` 或 `*` 时是相同的，但到了 `-` 就出现问题了

```
-- foldr' 是自己实现的
*Main> foldr (-) 0 [1]
1
*Main> foldr' (-) 0 [1]
-1
*Main> foldr (-) 0 [1, 2]
-1
*Main> foldr' (-) 0 [1, 2]
-3
*Main> foldr (-) 0 [1, 2, 3]
2
*Main> foldr' (-) 0 [1, 2, 3]
-6
```

为什么呢？出现这种 bug 的原因显然是因为 `+` 和 `*` 是具有交换律的，但是 `-` 则不同。也就是说我们的操作数顺序有问题
仔细想想 `foldr` 的展开应该是这样的

```
        f
      /   \
     1     f
         /   \
        2     f
            /   \
           3     a
```

而蠢作者展开成这样了

```
          f
        /   \
       f     1
     /   \
    f     2
  /   \
 a     3
```

所以真正的结果应该是这样的

```
foldr f a bs = foldl (\g b x -> g (f b x)) id bs a
```

用 `foldr` 实现 `foldl` 也是一个道理

```
foldl f a bs = foldr (\b g x -> g (f x b)) id bs a
```

    