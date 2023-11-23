
+++
title = "PEP 249 DB API 阅读笔记"
summary = ''
description = ""
categories = []
tags = []
date = 2017-11-29T14:31:00+08:00
draft = false
+++

*本篇为 PEP 249 阅读笔记*

PEP 249 是 Python Database API 2.0 版本规范，PEP 248 则是 1.0 版本。此规范是为了使 Python 数据库访问模块提供相似的接口，实现一致性，从而让代码便于移植

蠢作者补充示例的代码所使用的连接库是 `pymysql`

## 模块接口

### `Constructors`

通过 connection 对象可以访问数据库。此模块必须提供如下的构造器

`connect( parameters... )`  
创建数据库连接的构造器，返回 `Connection` 对象。


### `Globals`

#### `apilevel`

提供 dbapi 使用版本的常量

#### `threadsafety`

整数常量表示线程安全级别

- 0 线程可能不会共享模块
- 1 线程可能共享模块，但不共享连接
- 2 想成可能共享模块和连接
- 3 线程可能共享模块，连接和 cursor

*蠢作者注：PEP 原文中使用的是 may，应该是一种提倡的语气。实际上应当是"必须"，否则没有分级的必要了*

`pymysql`

```Python
In [8]: pymysql.threadsafety
Out[8]: 1
```

两个线程可能会在不使用互斥信号量实现资源锁的情况下使用同一个资源。注意不能通过使用互斥的访问管理来确保外部资源总是线程安全的：资源可能会依赖全局变量或者其他的你控制范围之外的外部资源

#### `paramstyle`

字符串常量表示接口所希望的 parameter marker formatting

```Python
In [2]: pymysql.paramstyle
Out[2]: 'pyformat'
```

- qmark 	Question mark style, e.g. ...WHERE name=?
- numeric 	Numeric, positional style, e.g. ...WHERE name=:1
- named 	Named style, e.g. ...WHERE name=:name
- format 	ANSI C printf format codes, e.g. ...WHERE name=%s
- pyformat 	Python extended format codes, e.g. ...WHERE name=%(name)s

### Exceptions

异常层级结构如下

```
StandardError
|__Warning
|__Error
   |__InterfaceError
   |__DatabaseError
      |__DataError
      |__OperationalError
      |__IntegrityError
      |__InternalError
      |__ProgrammingError
      |__NotSupportedError
```

## Connection Objects

connection 对象应当有如下的方法

### Connection methods

#### `.close()`
关闭连接(`.__del__()` 调用时也会关闭连接)  

若连接关闭，依旧尝试对连接进行任何操作，将会抛出 `Error` 或者其子类异常。所有使用此连接的 cursor 也是如此。注意，没有先 commit 就关闭连接将会造成隐式回滚

#### `.commit()`
commit 所有待处理的 transaction 到数据库
注意，如果数据库支持 auto-commit 功能，则必须先关闭该功能。可以提供一个方法来重新开启
Database modules that do not support transactions should implement this method with void functionality.

#### `.rollback()`
此方法可选，因为不是所有的数据库支持 transaction
若数据库支持事务，此方法会使数据库回滚至事务开始的状态

#### `.cursor()`
返回一个新的使用此连接的 Cursor 对象
如果数据库没有直接提供 cursor 的概念，则此模块应当使用此规范所需的其他方法来模拟一个 cursor


## Cursor Objects
这些对象用于表示数据库 cursor，用于管理 fetch 操作的上下文。从同一个连接所创建的 cursor 不是独立的，即 cursor 对数据库所做的任何更改都可以被其他 cursor 直接看到。从不同连接创建的 cursor 也不能保证是独立得到，这取决于 transaction 的支持是如何实现的

蠢作者认为这里有点奇怪，[aiomysql](https://github.com/aio-libs/aiomysql/issues/24) 有关于此的一个 issue。

```Python
In [64]: conn = pymysql.connect(password='root')

In [65]: cur = conn.cursor()

In [66]: cur.execute("select 1; select 2;")
Out[66]: 1

In [67]: cur.fetchall()
Out[67]: ((1,),)

In [68]: cur.nextset()
Out[68]: True

In [69]: cur.fetchall()
Out[69]: ((2,),)

In [70]: cur1 = conn.cursor()

In [71]: cur2 = conn.cursor()

In [72]: cur1.execute("SELECT 1; SELECT 2;")
Out[72]: 1

In [73]: cur2.execute("SELECT 42")
Out[73]: 1

In [74]: cur1.fetchall()
Out[74]: ((1,),)

In [75]: cur1.nextset()

In [76]: cur1.fetchall()
Out[76]: ()
```

可见 cursor 之间的确会相互影响，所以为什么这样做呢？


cursor 对象应当具有如下的方法和属性

### Cursor attributes
#### `.description`
只读属性，具有如下 7 个元素，包含了结果列的信息

- name
- type_code
- display_size
- internal_size
- precision
- scale
- null_ok

name 和 type_code 是强制的，其他的 5 项是可选的。如果没有提供有意义的值，这设置为 `None`
对于不返回结果行的操作或者 cursor 没有操作调用过 `execute()` 的情况，属性将会是 `None`

#### `.rowcount`

只读属性指出上一次 `.execute()` 操作所产生的行(比如 SELECT 之类的 DQL 语句)或者影响的行(比如 UPDATE, INSERT 之类的 DML 语句)

当 cursor 没有执行过 `.execute()` 或者上一次的操作的 rowcount 不能被接口所确定时，属性的值将为 `-1`

### Cursor methods
#### `.callproc( procname [, parameters ] )`
调用给定名称的数据库存储过程。此方法是可选的，因为不是所有的数据库都支持存储过程

#### `.close()`
关闭 cursor

#### `.execute(operation [, parameters])`
准备并执行数据库操作(查询或者命令)

参数可以以序列或者映射的方式提供，并会绑定到操作中的变量。变量使用数据库特定的表示法指定，见上面的 paramstyle

cursor 将保留对此操作的引用。如果再次传入相同的操作对象，cursor 可以优化其行为。这对于使用相同操作但是参数不同的情况十分有效。蠢作者认为这里好像是指的是复用预编译对象

为了最大效率的重用操作，最好是使用 `.setinputsizes()` 方法来提前指定参数的类型和大小。参数和预定义的信息不符合是合法的，只是会影响效率

参数可以以列表或者元组的方式指定，比如在单个操作中插入多行。不过这种操作将要被废弃，应当使用 `.executemany()` 作为替代

返回值是不确定的

#### `.executemany( operation, seq_of_parameters )`
`.execute()` 的 multi 版本。模块可以通过多次调用 `.execute()` 方法来实现，或者通过使用数组来让数据库在一次调用中处理整个序列。对产生一个或者多个结果集的操作，使用此方法是一种未定义的行为，数据库模块可以对于此种情况抛出异常

#### `.fetchone()`
获取结果集的下一行，返回单个序列或者 `None`。当之前没有调用或者调用的 `.execute()` 没有产生任何结果集，将会抛出异常 `Error` 或其子类

#### `.fetchmany([size=cursor.arraysize])`
获取结果集的下一行集合，一个序列的序列。当没有可用的行时会返回一个空的序列

每次调用所获取的行的数量可以由参数指定。如果没有给定此参数，cursor 的数组大小有决定了将要获取的行的数量。此方法应当尝试去获取 `size` 参数所指示的行数。如果无法获取足够多的行时，则会返回那些可以获取的行

注意，`size` 参数涉及性能。为了最佳的性能，最好使用 `.arraysize` 属性。如果使用了 `size` 参数，那么下一次调用时最好也保持相同的值

#### `.fetchall()`
获取结果集所剩下的所有行，返回一个序列的序列。注意 cursor 的 arraysize 属性会影响此操作的性能

#### `.nextset()`
此方法是可选的，因为不是所有的数据库支持多结果集。该方法会使 cursor 跳到下一个可用的结果集，舍弃当前结果集的剩余行
如果没有剩余的结果集，方法会返回 `None`。除此以外，它返回 `true`，随后对 `fetch()` 的调用将返回下一个结果集中的行

#### `.arraysize`
读写属性指定 `.fetchmany()` 一次可以获取到的行数目。默认为 1 意味着一次只能获取一行。

#### `.setinputsizes(sizes)`
在 `.execute()` 调用前使用来预定义操作参数的内存区域

`sizes` 是一个序列，每个元素对应一个输入参数。元素应当是一个 TypeObject 对应着将要使用的输入，或者是一个整数指定字符串参数的最大长度。如果元素是 `None`，那么不会为此列预留内存区域(这在避免为一个过大的输入预定义内存区域时很有用)

PyMysql 的此方法是一个空方法

#### `.setoutputsize(size [, column])`
在获取一个大型的列(`LONGS`，`BLOBS` 等)时，设置列的缓冲大小(column buffer size)。列被指定为结果序列的索引。不指定列时将会对 cursor 中的所有大型列设置默认大小

此方法需要在 `.execute()` 前调用

PyMysql 中此方法是一个空方法

## Type Objects and Constructors
许多数据库需要以特定的输入格式来绑定操作的输入参数。举个例子，如果一个输入的目标是 `DATE` 列，那么它必须以特定的字符串格式绑定至数据库。"Row ID" 列或者大型二进制列(`BLOBS` 或者 `RAW`)也存在相同的问题。这给 Python 带来了问题，因为 `.execute()` 方法的参数是无类型的。当数据库模块看到一个 Python 的字符串对象时，它不知道是否应当将其作为 `CHAR`， `BINARY` 还是 `DATE`

*Row ID 是 Oracle 中的*

为了处理此问题，模块必须提供下面定义的构造函数来创建可以保存特殊值的对象。当传递给 cursor 的方法时，模块可以检测到输入参数的合适类型并相应地进行绑定

构造函数
#### `Date(year, month, day)`
#### `Time(hour, minute, second)`
#### `Timestamp(year, month, day, hour, minute, second)`
#### `DateFromTicks(ticks)`
ticks 参数为从 epoch 至今的秒数
#### `TimeFromTicks(ticks)`
#### `TimestampFromTicks(ticks)`
#### `Binary(string)`

类型
#### `STRING type`
用于描述数据库中基于字符串(比如 `CHAR`)的列
#### `BINARY type`
#### `NUMBER type`
#### `DATETIME type`
#### `ROWID type`

由 Python 的 `None` 表示 SQL 的 `NULL`

## Implementation Hints for Module Authors
关于实现的一些[提示](https://www.python.org/dev/peps/pep-0249/#implementation-hints-for-module-authors)

## Optional DB API Extensions
模块的作者经常会增加一些 DB API 规范外的实现。为了增强兼容性，并提供一个途径来升级至未来版本的规范。这一小节定义了一些通用扩展。这些方法是否实现由模块作者自行抉择。

建议使用这些扩展时应显示一个警告

#### `Cursor.rownumber`
只读属性，提供结果集基于 0 的当前 cursor 索引，或者在索引无法确定时返回 `None`。索引可以看做是结果集序列中的 cursor 索引。下一次的 fetch 操作将会获取 `.rownumber` 为索引的行

#### `Connection.Error, Connection.ProgrammingError, etc.`
所有在 DB API 中定义的异常应当在 Connection 对象作为属性暴露出来
`pymysql` 支持

#### `Cursor.connection`
只读属性返回 cursor 所引用的 Connection 对象
`pymysql` 支持

#### `Cursor.scroll(value [, mode='relative' ])`
将 cursor 滚动值指定的位置，如果 `mode` 是 `relative`，值将会视为当前位置的偏移。如果是 `absolute`，值将被视为绝对位置。如果滚动操作离开结果集则应当抛出 `IndexError`。在这种情况下，cursor 的位置是不确定的。理想的情况下是不移动 cursor

#### `Cursor.messages`
Python 的 `list` 对象，附加了所有从数据库底层接收的消息。这个 `list` 会在除了 `.fetch()` 外的标准 cursor 方法调用后自动清除，以避免过高的内存使用。也可以使用 `del cursor.messages[:]` 来手动清除

数据库生成的所有 error 和 warning 消息都会被放置到此 `list` 中，所以检查此 `list` 可以让用户验证操作是否正确

The aim of this attribute is to eliminate the need for a Warning exception which often causes problems (some warnings really only have informational character).

`pymysql` 不支持

#### `Connection.messages`
与 `Cursor.messages` 类似，除了 `list` 中的信息是面向连接的

`pymysql` 不支持

#### `Cursor.next()`
和 `.fetchone()` 具有相同的语义。在 Python 2.2 之后，当结果集被消耗尽会抛出 `StopIteration`。之前的版本抛出 `IndexError`

`pymysql` 不支持

#### `Cursor.__iter__()`
兼容迭代协议，返回一个迭代器


#### `Cursor.lastrowid`
只读属性提供上一次修改行的 rowid(大部分数据库只在执行单个 insert 操作是才返回 rowid)。如果操作未设置 rowid 或者数据库不支持 rowid，这个属性应当为 `None`

当上一次执行的语句会修改多行时，`.lastrowid` 的语义是不明确的

`pymysql` 不支持

## Optional Error Handling Extensions
DB API 规范的核心部分只引入了一组异常。在一些情况下，异常对程序流程具有破坏性，甚至造成拒绝服务。为了简化错误处理，数据库模块的作者可以选择实现用户可定义的错误处理程序。本小节介绍定义这些错误处理程序的标准方法

#### `Connection.errorhandler, Cursor.errorhandler`
读写属性，用于当遇到错误条件时调用对应的错误处理。必须为一个 Python 的 callable，接受以下参数：

```
errorhandler(connection, cursor, errorclass, errorvalue)
```

`connection` 是 cursor 所在连接的引用，`cursor` 是 cursor 的引用。如果错误不需要 cursor 则可以为 `None`，`errorclass` 是错误的类，会使用 `errorvalue` 作为构造参数进行实例化

标准错误处理应当将错误信息添加至相应的 `.messages` 属性(`Connection.messages` 或者 `Cursor.messages`)，并抛出由 `errorclass` 和 `errorvalue` 参数给定的异常

如果没有设置 `.errorhandler`(属性为 `None`)，则使用上面列出的标准错误处理。

cursor 应当在其创建时继承 connection 对象的 `.errorhandler` 设置

## Optional Two-Phase Commit Extensions
许多数据库支持两段提交(Two-Phase commit aka TPC)，允许跨多数据库连接和其他资源管理 transaction。如果数据库后端支持 two-phase commit 并且数据库模块的作者想要对此进行支持，应当实现如下的 API。

延伸阅读[XA事务处理](http://www.infoq.com/cn/articles/xa-transactions-handle)

### TPC Transaction IDs
许多数据库支持 XA 规范，transaction IDS 有下面的三个部分组成：

- a format ID
- a global transaction ID
- a branch qualifier

对于特定的全局事务，前两个部分应当是相同的。全局事务中的每个资源应当被赋予不同的 branch qualifier

各个部分必须满足下面的标准：

- format ID: 非负 32 位整数
- global transaction ID and branch qualifier: 不长于 64 字符的字节字符串

Transaction IDS 可以使用 Connection 的 `.xid()` 方法创建：

#### `.xid(format_id, global_transaction_id, branch_qualifier)`
返回适合传递给该连接 `.tpc()` 方法的 transaction ID 对象

`.xid()` 返回对象的类型没有定义，但是必须提供类似序列的接口，允许访问这三个部分。一致性数据库模块能够选择使用元组来表示 transaction IDS 而不是自定义对象

### TPC Connection Methods

#### `.tpc_begin(xid)`
使用给定的 transaction ID `xid` 开始一个 TPC transaction
这个方法应当在一个事务之外被调用，此外在 TPC事务中调用 `.commit()` 或者 `.rollback()` 是错误的。如果程序在一个 active 的 TPC transaction 中调用 `.commit()` 或者 `.rollback()` 将会抛出 `ProgrammingError`

#### `.tpc_prepare()`
执行以 `.tpc_begin()` 开始的第一阶段事务。如果在一个 TPC 事务外调用则会抛出 `ProgrammingError`。当调用 `.tpc_prepare()` 后，没有语句可以执行直到 `.tpc_commit()` 或者 `.tpc_rollback()` 被调用

#### `.tpc_commit([ xid ])`

无参调用时，`.tpc_commit()` 提交一个之前由 `.tpc_prepare()` 准备好的 TPC 事务。如果 `.tpc_commit()` 在 `.tpc_prepare()` 之前被调用，则执行单阶段提交。当只有一个资源参与全局事务时，事务管理器(transaction manager)可以选择这样做

当使用 transaction ID `xid` 调用时，将提交给定的事务。如果是一个无效的 transaction ID，会抛出 `ProgrammingError`。这种形式应当在事务之外调用

返回时，TPC 事务结束

#### `.tpc_rollback([ xid ])`
无参调用时，回滚 TPC 事务。`.tpc_prepare()`　之前或者之后调用均可。
当使用 transaction ID `xid` 调用时，将回滚给定的事务。如果是一个无效的 transaction ID，会抛出 `ProgrammingError`。这种形式应当在事务之外调用

返回时，TPC 事务结束

#### `.tpc_recover()`
返回适用于 `.tpc_commit(xid)` 或者 `.tpc_rollback(xid)` 的待处理事务列表


## Reference
[PEP 249 -- Python Database API Specification v2.0](https://www.python.org/dev/peps/pep-0249)

    