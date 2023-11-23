
+++
title = "covariant and contravariant in Python"
summary = ''
description = ""
categories = []
tags = []
date = 2019-03-12T15:09:02+08:00
draft = false
+++

本文所使用的记号

A ≼ B means A is a subtype of B.
A -> B is the type of functions for which the argument type is A and the return type is B.
x : A means x has type A.

另外本文所使用的 MyPy 版本为 0.670，
### Motivation


假设有下面三种类型

Greyhound ≼ Dog ≼ Animal

由于子类型关系是可传递，所以 Greyhound ≼ Animal 也成立。那么对于函数 Dog → Dog 而言下面四种类型，谁是合法的子类型呢?

- Greyhound → Greyhound
- Animal → Animal
- Greyhound → Animal
- Animal → Greyhound

具体的讲，比如我们有一个函数 f，它接收 Dog → Dog 类型的函数作为其参数，那么上面四种哪一个可以作为参数传入?

1. Greyhound → Greyhound

不安全，因为 f 的实现可能会是

```
def f(g: Callable[[Dog], Dog]) -> None:
    g(GermanShepherd())  # GermanShepherd is another subclass of Dog
```

因为 g 的参数是 Dog，所以可以向其传入任何 Dog 的子类型。这一点也是里氏替换原则(Liskov Substitution principle)所强调的——派生类（子类）对象可以在程式中代替其基类（超类）对象。但是这里如果我们实际传入了 `Callable[[Greyhound], Greyhound]`，这便会产生不安全的操作。`Greyhound` 可能具有 `GermanShepherd` 所不具备的方法/属性。

2. Animal → Animal

不安全，因为 f 的实现可能会是

```
def f(g: Callable[[Dog], Dog]) -> None:
    dog = g(GermanShepherd())  # GermanShepherd is another subclass of Dog
    dog.bark()
```

因为我们实际传入的 g 为 `Callable[[Animal], Animal]` 类型，所以在 g 中只会使用基类 Animal 中存在的方法/属性。而 g 的仅要求 `Callable[[Dog], Dog]`，又因为 Dog 是 Animal 的子类，具有基类的所有方法及属性。所以这里传入任何一个 Dog 的子类都是安全的，比如 GermanShepherd。但是上例依然是不安全的，原因在于 g 本来要求返回 Dog，但是我们实际却返回了 Animal。这会导致我们使用仅存在于 Dog 而不存在于 Animal 上的方法/属性，比如 bark 方法

3. Greyhound → Animal

不安全，原因综合 1 和 2

4. Animal → Greyhound

安全，原因综合 1 和 2


这里我们引入两个概念

- covariant(协变) if it preserves the ordering of types (≤), which orders types from more specific to more generic;
- contravariant(逆变) if it reverses this ordering;

Luca Cardelli 提出 type constructor is contravariant in the input type and covariant in the output type

即下面的公式

$$
S\_{1} -> S\_{2} ≤ T\_{1} -> T\_{2} 如果 T\_{1} ≤ S\_{1} 且 S\{2} ≤ T\_{2}
$$

对于面向对象来说便是 overriding method should return a more specific type (return type covariance), and accept a more general argument (argument type contravariance)

但是虽然说上面的结论是类型安全的，但是并不是所有的语言严格地遵守，比如 Java

```Java
public class X {

}

public class Y extends X {

}

public class B {
    public X[] method(X a) {
        return new X[10];
    }
}

public class C {
    public Y[] method(Y a) {
        return new Y[10];
    }
}
```

也有些语言的函数参数默认是 contravariant 的，但是提供了一些方式去开天窗，比如 Dart 的 `covariant` 关键字，参考[The covariant keyword](https://www.dartlang.org/guides/language/sound-problems#the-covariant-keyword)

```Dart
class Animal {
  void chase(Animal x) { ... }
}

class Mouse extends Animal { ... }

class Cat extends Animal {
  void chase(covariant Mouse x) { ... }
}
```

### Test And Relax

下面准备了五份 Test Code，请凭直觉判断哪几份在 strict 模式下不会报错

##### 例一

```
class X:
    pass


class Y(X):
    pass


class A:
    def get_one(self) -> X:
        return X()


class B(A):
    def get_one(self) -> Y:
        return Y()
```


##### 例二

```
from typing import List


class X:
    pass


class Y(X):
    pass


class A:
    def get_all(self) -> List[X]:
        return [X()]


class B(A):
    def get_all(self) -> List[Y]:
        return [Y()]
```


##### 例三

```
from typing import Tuple


class X:
    pass


class Y(X):
    pass


class A:
    def get_all(self) -> Tuple[X]:
        return (X(), )


class B(A):
    def get_all(self) -> Tuple[Y]:
        return (Y(), )

```

##### 例四

```
from typing import Tuple


class X:
    pass


class Y(X):
    pass


class A:
    def set(self, arg: X) -> None:
        pass


class B(A):
    def set(self, arg: Y) -> None:
        pass
```


##### 例五

```
from typing import Tuple


class X:
    pass


class Y(X):
    pass


class A:
    def set(self, arg: Y) -> None:
        pass


class B(A):
    def set(self, arg: X) -> None:
        pass
```


上面代码只有 1, 3, 5 是合格的。如果存在疑问可以参考 [Covariance and contravariance](https://en.wikipedia.org/wiki/Covariance_and_contravariance_(computer_science)) 的 Inheritance in object-oriented languages 一节

### Mutable Covariant ?

下面来重点解释 2 和 3 的区别，其实主要在 mutable 上。Mypy 的文档中提到过 [Covariant subtyping of mutable protocol members is rejected](https://mypy.readthedocs.io/en/latest/common_issues.html#covariant-subtyping-of-mutable-protocol-members-is-rejected)

> Read-only data types (sources) can be covariant; write-only data types (sinks) can be contravariant. Mutable data types which act as both sources and sinks should be invariant. To illustrate this general phenomenon, consider the array type


如果满足 covariant，那么 List[Y] ≤ List[X]

考虑下面的代码

```Python
def f(a: A):
    l: List[X] = a.get_all()
    l.append(Z())  # Z is also a subclass of X
```

函数内第一行的赋值是被允许的，那么紧接着如果我们向其插入 Z 的实例，因为 l 是 List[X]，且 Z ≤ X，所以这里是允许插入的。但是实际上我们这里会造成 List[Y] 中混入 Z，这并不是安全的

反过来如果满足 contravariant，那么 List[X] ≤ List[Y]。这也是不安全的，因为 List[X] 中可以包含 Z，但是 Z 并不具备 Y 的方法

```Python
a: A = a
l: List[Y] = a.get_all()
for item in l:
    item.method_y()  # crash when meet z
```

如果数组是协变的，就不能容许数组的写操作，如果数组是逆变的，就不能容许数组的读操作。这也是为什么我们使用 `Tuple` 便可以通过检查，因为它是 immutable。

引入新的概念

- bivariant if both of these apply (i.e., both `I<A> ≤ I<B> and I<B> ≤ I<A>` at the same time)
- invariant or nonvariant if neither of these applies.

数组应当是 invariant 的。早期的 Java 数组类型是 covariant 的。因为要考虑一些无关于具体类型的操作，比如

```Java
boolean equalArrays(Object[] a1, Object[] a2);
```

如果是 invariant 的，那么我们只能传入 `Object[]` 而不能是 `String[]`。Java 在运行时会对数组的修改操作进行检查，如果类型不匹配那么便会抛出 `ArrayStoreException`。后来 Java 添加了泛型机制，解决了这一问题

```Java
<T> boolean equalArrays(T[] a1, T[] a2);
```

### Reference
[WiKi Covariance and contravariance](https://en.wikipedia.org/wiki/Covariance_and_contravariance_(computer_science))  
[What are covariance and contravariance?](https://www.stephanboyer.com/post/132/what-are-covariance-and-contravariance)

    