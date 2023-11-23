
+++
title = "How To Prove It 笔记 - Introduction"
summary = ''
description = ""
categories = []
tags = []
date = 2019-03-09T12:41:28+08:00
draft = false
+++

[ How to Prove It ](https://book.douban.com/subject/2869796/) Introduction 一节的读书笔记


命题一: n 为任意一个大于 1 的合数，那么 \\(2^n-1\\) 也为合数

证明: 因为 n 为合数，那么存在正整数 a 和 b(a< n, b < n)，使得 n = ab。令 \\(x = 2^b-1\\), \\(y = 1 + 2^b + 2^{2b} + \cdots + 2^{(a-1)b}\\)。那么有

$$
\begin{aligned}
xy ={}& (2^b-1) \cdot (1 + 2^b + 2^{2b} + \cdots + 2^{(a-1)b}){} \\\\
&=2^b \cdot (1 + 2^b + 2^{2b} + \cdots + 2^{(a-1)b}) - (1 + 2^b + 2^{2b} + \cdots + 2^{(a-1)b}) \\\\
&=(2^b + 2^{2b} + 2^{3b} + \cdots + 2^{ab}) - (1 + 2^b + 2^{2b} + \cdots + 2^{(a-1)b}) \\\\
&= 2^{ab} - 1 \\\
&= 2^n - 1
\end{aligned}
$$

现在只需要证明 \\(x < 2^n-1\\) 且 \\(y < 2^n-1\\) 即可

因为 \\(b < n\\), 所以 \\(x = 2^b - 1 < 2^n - 1\\)。因为 \\(ab = n > a\\), 所以 \\(b = n/a > 1\\)。因此 \\(x = 2^b-1>2^1-1 = 1\\)，所以可得 \\(y < xy = 2 ^ n-1\\)。

1) 注形如 \\(2^n-1\\) 这样的数称为梅森数(Mersenne Number)，记作 \\(M_n\\)。如果一个梅森数是素数那么它称为梅森素数(Mersenne prime)  
2) 梅森素数与完全数相关。完全数(perfect number)，指一个数的所有真因子（即除了自身以外的约数）的和，恰好等于它本身。欧几里得证明如果 \\(2^n-1\\) 是梅森素数，那么 \\(2^{n-1}(2^n-1)\\) 一定是完全数。欧拉则证明每一个偶完全数都符合这一条件。目前未发现奇完全数

---

命题二: 质数集为无限集

证明: 假设质数集为有限集, 若 \\(p\_{1}, p\_{2}, \cdots, p\_{n}\\) 表示所有质数。令 \\(m = p\_{1}p\_{2} \cdots p\_{n} + 1\\)。m 不能被 \\(p\_1, p\_2, \cdots,p\_n\\) 所整除，因为余数为 1。

根据[正整数唯一分解定理](https://zh.wikipedia.org/wiki/%E7%AE%97%E6%9C%AF%E5%9F%BA%E6%9C%AC%E5%AE%9A%E7%90%86)，任何一个大于 1 的整数，要么本身就是质数，要么可以写为 2 个以上的质数的积

- 若 m 为质数，但是它却不在 \\(p\_{1}, p\_{2}, \cdots, p\_{n}\\) 之中
- 若 m 为合数，但是它却不能被 \\(p\_{1}, p\_{2}, \cdots, p\_{n}\\) 中任意一个所整除

这与我们的假设相违背，因此质数集是无限集

---

命题三: 对于任何一个正整数 n, 存在一个不包含素数的长度为 n 的正整数连续序列
证明: 假设 n 是一个正整数，令 \\(x = (n + 1)! + 2\\)。只需证明 \\(x, x + 1, x + 2, \cdots, x + (n - 1)\\)这个序列中不存在素数即可。

因为 

$$
\begin{aligned}
x =& 1 \cdot 2 \cdot 3 \cdot 4 \cdots (n+1) + 2 \\\\
&=2 \cdot (1 \cdot 3 \cdot 4 \cdots (n+1) + 1)
\end{aligned}
$$

所以 x 不是素数，同理对于 x + 1

$$
\begin{aligned}
x + 1=& 1 \cdot 2 \cdot 3 \cdot 4 \cdots (n+1) + 3 \\\\
&3 \cdot (1 \cdot 3 \cdot 4 \cdots (n+1) + 1)
\end{aligned}
$$

所以 x + 1 也不是素数  

对于 \\(x + i,(0 \leq i \leq n-1)\\)我们有

$$
\begin{aligned}
x + i=& 1 \cdot 2 \cdot 3 \cdot 4 \cdots (n+1) + (i + 2) \\\\
&(i + 2) \cdot (1 \cdot 2 \cdot 3 \cdot 4 \cdots (n+1) + 1)
\end{aligned}
$$

所以 \\(x, x + 1, x + 2, \cdots, x + (n - 1)\\) 中不存在素数
    