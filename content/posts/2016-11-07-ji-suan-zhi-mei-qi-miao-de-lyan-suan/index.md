
+++
title = "计算之美——奇妙的λ演算"
summary = ''
description = ""
categories = []
tags = []
date = 2016-11-07T03:23:33+08:00
draft = false
+++

~~此篇文章可能会给你带来些许的不适，出现 烦躁， 头疼 均属正常情况。阅读前，请保证处于头脑清醒的状态~~

先来看一段函数

	factorial = (lambda f: f(f))(
	    lambda f: (lambda n:
	        1 if n == 0 else
	            n * f(f)(n - 1)))

先来想想我们如果用递归来计算阶乘会怎么写

	def factorial(n):
	    return 1 if n == 0 else n * factorial(n - 1)


递归需要调用自身，那我们就需要明确自身是什么。比如这里我们使用了 factorial 这个函数名。那么如果我们非要使用匿名函数进行递归的话，我们该怎么做呢？

引入第一个概念 **λ演算**

>在λ演算中，每个表达式都代表一个函数，这个函数有一个参数，并且返回一个值。不论是参数和返回值，也都是一个单参的函数。可以这么说，λ演算中，只有一种“类型”，那就是这种单参函数。

比如 `f(x)=x+2` 可以用λ演算表示为 `λx.x + 2`。对于 `3+2` 我们这样来求值 `(λx.x + 2) 3` 。我们还可以将函数作为参数 `(λf.f 3)(λx.x + 2)`
对于多个参数的函数通过柯里化来解决
简言之是将一个具有多个参数的函数，转换成一系列的函数链式调用

	def power(x):
	    return lambda n: x **n

	power_of_2 = power(2)

看到这里也许你想到了偏函数

	from functools import partial

	def power(x, n):
	    return x ** n

	power_of_2 = partial(power,2)
	power_of_2(10)

偏函数是用特定的值来具体化参数，而柯里化则是变成了一条函数链。
说的更通俗一点就是偏函数可以固定参数 n ，而柯里化不能，因为这条链是有顺序的

让我们回到λ演算中来，并非所有的λ表达式都可以规约至确定值，考虑 (λx.x x)(λx.x x) 或 (λx.x x x)(λx.x x x) 。 (λx.x x)被称为 ω 组合子，((λx.x x)(λx.x x))被称为Ω，而((λx.x x x) (λx.x x x))被称为Ω2，以此类推。

好吧，你不觉得这个 (λx.x x) 似曾相识么？

这个用 Python 写出来是这样的 `lambda f: f(f)`，如果还记不起来请往上滑

试着将一个匿名函数作为参数传入

    lambda f: (lambda n:
        1 if n == 0 else
            n * f(f)(n - 1))

返回了内层的 lambda 函数，并且由于闭包的性质，携带了 f 即匿名函数自身，这样我们就完成了对匿名函数的递归。

好了，是不是觉得很神奇？

我们再来看点专业点的解释，引自[wiki](https://zh.wikipedia.org/wiki/%CE%9B%E6%BC%94%E7%AE%97)

递归是使用函数自身的函数定义；在表面上，lambda演算不允许这样。但是这种印象是误解。考虑个例子，阶乘函数f(n)递归的定义为

    f(n):= if n = 0 then 1 else n·f(n-1)

在λ演算中，你不能定义包含自身的函数。要避免这样，你可以开始于定义一个函数，这里叫g，它接受一个函数f作为参数并返回接受n作为参数的另一个函数：

    g := λf n.(if n = 0 then 1 else n·f(n-1))

函数g返回要么常量1，要么函数f对n-1的n次应用。使用ISZERO谓词，和上面描述的布尔和代数定义，函数g可以用lambda演算来定义。

但是，g自身仍然不是递归的；为了使用g来建立递归函数，作为参数传递给g的f函数必须有特殊的性质。也就是说，作为参数传递的f函数必须展开为调用带有一个参数的函数g，并且这个参数必须再次为f函数!

换句话说，f必须展开为g(f)。这个到g的调用将接着展开为上面的阶乘函数并计算下至另一层递归。在这个展开中函数f将再次出现，并将被再次展开为g(f)并继续递归。这种函数，这里的 f = g(f)，叫做g的不动点，并且它可以在λ演算中使用叫做悖论算子或不动点算子来实现，它被表示为 Y 组合子：

    Y = λf.(λx.(f (x x)) λx.(f (x x)))

对于 Y 组合子而言 Y g 等价于 g(Y g)， 要推出这个还需要了解三条归约，概括如下

- α-变换
变量的名称是不重要的，比如说λx.x和λy.y是同一个函数
- β-归约
只允许从左至右来代入参数
- η-变换
两个函数对于所有的参数得到的结果都一致，当且仅当它们是同一个函数

推导过程如下

	(Y g)
	(λf.(λx.(f (x x)) λx.(f (x x))) g)
	;（λf的β-归约 - 应用主函数于g）
	(λx.(g (x x)) λx.(g (x x)))
	;（α-转换 - 重命名约束变量）
	(λy.(g (y y)) λx.(g (x x)))
	;（λy的β-归约 - 应用左侧函数于右侧函数）
	(g (λx.(g (x x)) λx.(g (x x))))
	(g (Y g))


现在让我们改进一开始的例子，说实话用 Python 写这种代码挺费劲的

	(lambda f:(
	    lambda n:(
	        1 if n == 0 else n * f(f)(n - 1))))

令 g=f(f)

	(lambda f:(
	    lambda g:(
	        lambda n:(
	            1 if n == 0 else n * g(n - 1))
	    )(f(f))))

我们可以看到

	lambda g:
	    lambda n:
	        1 if n == 0 else n * g(n - 1)

这一部分已经变得完美了，将这一部分再提取出来成为 f0

	(lambda f0:(
	    lambda f:(
	        f0)(
	    f(f)))
	)(lambda g:
	    lambda n:
	    1 if n == 0 else n * g(n - 1))

让我们来试试

	factorial = (lambda f:
	                    f(f))(
	                (lambda f0:(
	                    lambda f:(
	                        f0)(
	                    f(f)))
	                )(lambda g:
	                    lambda n:
	                    1 if n == 0 else n * g(n - 1)))

	factorial(10)


,,Ծ‸Ծ,,  栈溢出了， 什么鬼？

因为我们执行到这里

	lambda f:(
	    f0)(
	f(f))

对 f(f) 进行求值再应用于 f0 时，会陷入无穷。可以通过 η-变换 来解决



	factorial = (lambda f:
	                    f(f))(
	                (lambda f0:(
	                    lambda f:(
	                        f0)(
	                    lambda s: f(f)(s)))
	                )(lambda g:
	                    lambda n:
	                    1 if n == 0 else n * g(n - 1)))


再进一步抽象

	factorial = (lambda y:(
	                    lambda f:
	                        f(f))(
	                    (lambda f0:(
	                        lambda f:(
	                            f0)(
	                        lambda s: f(f)(s)))
	                    )(y)))(
	                lambda g:
	                    lambda n:
	                        1 if n == 0 else n * g(n - 1))



大功告成, 这个就是 Y 了

	(lambda y:(
	    lambda f:
	        f(f))(
	    (lambda f0:(
	        lambda f:(
	            f0)(
	        lambda s: f(f)(s)))
	    )(y)))

## Reference
- [λ演算](https://zh.wikipedia.org/wiki/%CE%9B%E6%BC%94%E7%AE%97)
- [不动点组合子](https://zh.wikipedia.org/wiki/%E4%B8%8D%E5%8A%A8%E7%82%B9%E7%BB%84%E5%90%88%E5%AD%90)
- [函数式编程的 Y Combinator 有哪些实用价值？](https://www.zhihu.com/question/20115649)

