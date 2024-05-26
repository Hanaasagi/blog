+++
title = "危险的 Django update_or_create"
summary = ''
description = ""
categories = []
tags = []
date = 2024-05-26T17:43:34+09:00
draft = false

+++

## 起因

最近发现一个系统的代码里面经常出现 `Deadlock found when trying to get lock try restarting transaction` 错误，看了一下代码，这部分很多地方直接使用的是 Django 的 `update_or_create` 函数，它是一个框架层面提供的一个 upsert 的 utils

- 文档地址: https://docs.djangoproject.com/en/5.0/ref/models/querysets/#update-or-create

- 代码位置: https://github.com/django/django/blob/9a27c76021f934201cccf12215514a3091325ec8/django/db/models/query.py#L983-L988

```Python
    def update_or_create(self, defaults=None, create_defaults=None, **kwargs):
        """
        Look up an object with the given kwargs, updating one with defaults
        if it exists, otherwise create a new one. Optionally, an object can
        be created with different values than defaults by using
        create_defaults.
        Return a tuple (object, created), where created is a boolean
        specifying whether an object was created.
        """
        update_defaults = defaults or {}
        if create_defaults is None:
            create_defaults = update_defaults

        self._for_write = True
        with transaction.atomic(using=self.db):
            # Lock the row so that a concurrent update is blocked until
            # update_or_create() has performed its save.
            obj, created = self.select_for_update().get_or_create(
                create_defaults, **kwargs
            )
```

上面的代码会生成 `select for update` 查询，这个在 MySQL 中会上 X 锁，阻止其他事务的更新操作。至于锁住的范围，需要综合隔离级别和查询与表的索引的来定，下面稍微分析一下死锁的产生



## 实验

版本是社区版 MySQL 8.4.0，用的默认的 RR 级别。表结构如下:

```sql
CREATE TABLE "camera_widget_light" (
  "id" int NOT NULL AUTO_INCREMENT,
  "name" varchar(64) CHARACTER SET utf8mb3 COLLATE utf8mb3_bin NOT NULL,
  "cn_name" varchar(64) CHARACTER SET utf8mb3 COLLATE utf8mb3_bin NOT NULL,
  "address" longtext CHARACTER SET utf8mb3 COLLATE utf8mb3_bin NOT NULL,
  "light_type" varchar(64) CHARACTER SET utf8mb3 COLLATE utf8mb3_bin NOT NULL,
  "figure" varchar(64) CHARACTER SET utf8mb3 COLLATE utf8mb3_bin NOT NULL,
  "create_time" datetime(6) NOT NULL,
  "update_time" datetime(6) NOT NULL,
  "render_images_oss" longtext CHARACTER SET utf8mb3 COLLATE utf8mb3_bin NOT NULL,
  "detailed_scene" varchar(64) CHARACTER SET utf8mb3 COLLATE utf8mb3_bin NOT NULL,
  "camera_summary_name" varchar(64) CHARACTER SET utf8mb3 COLLATE utf8mb3_bin NOT NULL,
  "look_name" varchar(64) CHARACTER SET utf8mb3 COLLATE utf8mb3_bin NOT NULL,
  "record_cn_name" varchar(64) CHARACTER SET utf8mb3 COLLATE utf8mb3_bin NOT NULL,
  "record_name" varchar(128) CHARACTER SET utf8mb3 COLLATE utf8mb3_bin NOT NULL,
  "shot_shooting_scale_name" varchar(64) CHARACTER SET utf8mb3 COLLATE utf8mb3_bin NOT NULL,
  PRIMARY KEY ("id"),
  KEY "camera_widget_light_detailed_scene_name_8db6c0b0_idx" ("detailed_scene","name"),
  KEY "camera_widget_light_detailed_scene_look_name_4dd3cd96_idx" ("detailed_scene","look_name")
);
```

核心字段数据大概下面这个样子，其他字段随便编就行了

```sql
select count(1), detailed_scene from camera_widget_light group by detailed_scene;

+----------+------------------+
| count(1) | detailed_scene   |
+----------+------------------+
| 29968    | Oralbroadcasting |
| 59923    | buy_2d           |
+----------+------------------+
```

准备好数据之后，按照如下顺序执行，`ABC` 在这里是一条不存在的记录

- T1, console-A

```sql
begin;
select * from camera_widget_light where detailed_scene = 'Oralbroadcasting' and record_name = 'ABC' for update;
```

- T1, console-B

```sql
begin;
select * from camera_widget_light where detailed_scene = 'buy_2d' and record_name = 'ABC' for update;
```

- T2, console-A

```sql
select * from camera_widget_light where detailed_scene = 'buy_2d' and record_name = 'ABC' for update;
```

此时这个语句会处于等待的状态，如果我们再在另一个 console 中执行

- T2, console B

```sql
select * from camera_widget_light where detailed_scene = 'Oralbroadcasting' and record_name = 'ABC' for update;
```

console A 则会抛出

```
(1213, 'Deadlock found when trying to get lock; try restarting transaction')
```

这里 B 会成功，而 A 是报错的。



## 分析

对于 MySQL 锁相关的问题，我们可以通过以下的方式来进行排查

- `show processlist` 查看当前所有活动的连接和执行的查询信息

- 如果已经发生过死锁，那么通过 ` show engine innodb status\G;` 可以最近的问题报告
- `select * from sys.innodb_lock_waits\G;` 查看锁等待情况，或者 `select * from performance_schema.data_lock_waits\G;`
- `select * from performance_schema.data_locks\G;` 查看哪些数据被上了锁
- `select * from performance_schema.metadata_locks\G;` 元数据锁的信息



下面我们按部就班来排查一下这个问题

在 Console 中执行

```sql
begin;
select * from camera_widget_light where detailed_scene = 'Oralbroadcasting' and record_name = 'ABC' for update;
```

通过 `performance_schema.data_locks`表查询上锁的数据情况，首先来看一下锁的数量

```sql
select count(1) from performance_schema.data_locks;

+----------+
| count(1) |
+----------+
| 60048    |
+----------+
```



这个数据量就不对头，顺着这个往下查，看一下锁的类型

```sql
select count(1), OBJECT_NAME,INDEX_NAME, LOCK_TYPE,LOCK_MODE, LOCK_STATUS FROM performance_schema.data_locks group by OBJECT_NAME,INDEX_NAME, LOCK_TYPE,LOCK_MODE, LOCK_STATUS\G;

***************************[ 1. row ]***************************
count(1)    | 1
OBJECT_NAME | camera_widget_light
INDEX_NAME  | <null>
LOCK_TYPE   | TABLE
LOCK_MODE   | IX
LOCK_STATUS | GRANTED
***************************[ 2. row ]***************************
count(1)    | 30078
OBJECT_NAME | camera_widget_light
INDEX_NAME  | camera_widget_light_detailed_scene_name_8db6c0b0_idx
LOCK_TYPE   | RECORD
LOCK_MODE   | X
LOCK_STATUS | GRANTED
***************************[ 3. row ]***************************
count(1)    | 29968
OBJECT_NAME | camera_widget_light
INDEX_NAME  | PRIMARY
LOCK_TYPE   | RECORD
LOCK_MODE   | X,REC_NOT_GAP
LOCK_STATUS | GRANTED
***************************[ 4. row ]***************************
count(1)    | 1
OBJECT_NAME | camera_widget_light
INDEX_NAME  | camera_widget_light_detailed_scene_name_8db6c0b0_idx
LOCK_TYPE   | RECORD
LOCK_MODE   | X,GAP
LOCK_STATUS | GRANTED
```

`performance_schema.data_locks` 表的所有字段含义可以参考文档 https://dev.mysql.com/doc/refman/8.0/en/performance-schema-data-locks-table.html



可以看到结果中出现了 `camera_widget_light_detailed_scene_name_8db6c0b0_idx`这个索引，这是因为查询部分命中了这个索引的左侧字段 `detailed_scene` 字段，MySQL 选择利用这个索引缩小查询的数据范围。这个是和数据量有关的，如果你的数据量小，可能看到的结果不是这样的，也可能选择直接全表扫描。具体可以通过 `explain` 来看一下查询计划

```sql
explain select * from camera_widget_light where detailed_scene = 'Oralbroadcasting' and record_name = 'ABC' for update\G;

***************************[ 1. row ]***************************
id            | 1
select_type   | SIMPLE
table         | camera_widget_light
partitions    | <null>
type          | ref
possible_keys | camera_widget_light_detailed_scene_name_8db6c0b0_idx,camera_widget_light_detailed_scene_look_name_4dd3cd96_idx
key           | camera_widget_light_detailed_scene_name_8db6c0b0_idx
key_len       | 194
ref           | const
rows          | 3
filtered      | 16.67
Extra         | Using where
```

上面返回的四条记录可以大致可以分为 4 种类型，下面展开说一下:



###  `Table, IX`

- 第一种

```
LOCK_TYPE             | TABLE
LOCK_MODE             | IX
LOCK_STATUS           | GRANTED
LOCK_DATA             | <null>
```

表级别的 IX 类型的锁，即 `intention exclusive lock`，中文普遍叫做「意向排他锁」。它表示事务打算对表中的个别行设置排他锁，一个 `for share` 会设置 `IS` 类型的锁，而 `for update` 会设置 `IX` 类型的锁



互斥关系，参考 MySQL 文档 https://dev.mysql.com/doc/refman/8.0/en/innodb-locking.html

|      | `X`      | `IX`       | `S`        | `IS`       |
| ---- | -------- | ---------- | ---------- | ---------- |
| `X`  | Conflict | Conflict   | Conflict   | Conflict   |
| `IX` | Conflict | Compatible | Conflict   | Compatible |
| `S`  | Conflict | Conflict   | Compatible | Compatible |
| `IS` | Conflict | Compatible | Compatible | Compatible |

注意这里两个 `IX` 是不互斥的，所以我们在实验开始的时候允许两个 console 分别执行 `select for update`

### `Record, X`

剩下的三条记录都是 `LOCK_TYPE` 为 `REOCRD` 类型的锁，根据数据和索引的不同，分成了 3 小类

`X` 虽然看起来是一个行级别互斥锁，但是在 RR 级别下，它实际上是 next-key lock https://dev.mysql.com/doc/refman/8.0/en/innodb-locking.html#innodb-next-key-locks。这个是为了解决幻读问题锁需要的，而`X, REC_NOT_GAP` 这个才是单纯的行记录锁



我们随便挑选两条 `X` 的记录看一下

```sql
***************************[ 1. row ]***************************
ENGINE                | INNODB
ENGINE_LOCK_ID        | 128809141538008:2:22:1:128809156265872
ENGINE_TRANSACTION_ID | 2858
THREAD_ID             | 89
EVENT_ID              | 11
OBJECT_SCHEMA         | test
OBJECT_NAME           | camera_widget_light
PARTITION_NAME        | <null>
SUBPARTITION_NAME     | <null>
INDEX_NAME            | camera_widget_light_detailed_scene_name_8db6c0b0_idx
OBJECT_INSTANCE_BEGIN | 128809156265872
LOCK_TYPE             | RECORD
LOCK_MODE             | X
LOCK_STATUS           | GRANTED
LOCK_DATA             | supremum pseudo-record
***************************[ 2. row ]**************************
ENGINE                | INNODB
ENGINE_LOCK_ID        | 128809141538008:2:22:2:128809156265872
ENGINE_TRANSACTION_ID | 2858
THREAD_ID             | 89
EVENT_ID              | 11
OBJECT_SCHEMA         | test
OBJECT_NAME           | camera_widget_light
PARTITION_NAME        | <null>
SUBPARTITION_NAME     | <null>
INDEX_NAME            | camera_widget_light_detailed_scene_name_8db6c0b0_idx
OBJECT_INSTANCE_BEGIN | 128809156265872
LOCK_TYPE             | RECORD
LOCK_MODE             | X
LOCK_STATUS           | GRANTED
LOCK_DATA             | 'Oralbroadcasting', 'buy_2d_cold_light_female', 2
```

`LOCK_DATA` 的`supremum pseudo-record` 看起来很奇怪，这是一个特殊的标记值，用于表达一个最大的极限范围。因为 next-key-lock 是一个 gap lock 加上一条行记录锁，它的区间是一个左开右闭的，所以我们需要一种值来标记这个右边界



文档中描述如下

> For the last interval, the next-key lock locks the gap above the largest value in the index and the “supremum” pseudo-record having a value higher than any value actually in the index. The supremum is not a real index record, so, in effect, this next-key lock locks only the gap following the largest index value.

因为我们的数据并没有命中索引，所以导致 MySQL 扫描了所有的数据，对于每一条满足 `Oralbroadcasting` 的记录的索引都加了 next-key lock。按照锁的地址 `OBJECT_INSTANCE_BEGIN` 排列后，可以发现每一个 lock 对象都锁住了 N 条数据，然后额外持有一个 `supremum pseudo-record  ` 的锁。这就导致了我们 `X` 类型的锁的数量 30078 大于记录的数量 29968

```sql
select OBJECT_INSTANCE_BEGIN, lock_data from performance_schema.data_locks where lock_mode = 'X' order by OBJECT_INSTANCE_BEGIN desc;

+-----------------------+-------------------------------------------------------------+
| OBJECT_INSTANCE_BEGIN | lock_data                                                   |
+-----------------------+-------------------------------------------------------------+
| 128809156265872       | supremum pseudo-record                                      |
| 128809156265872       | 'Oralbroadcasting', 'buy_2d_cold_light_female', 2           |
| 128809156265872       | 'Oralbroadcasting', 'buy_2d_cold_light_female', 39          |
| 128809156265872       | 'Oralbroadcasting', 'buy_2d_cold_light_female', 40          |
-- ...

| 128809156265872       | 'Oralbroadcasting', 'buy_2d_cold_light_female', 6232        |
| 128809156265872       | 'Oralbroadcasting', 'buy_2d_cold_light_female', 6233        |
| 128808965335184       | supremum pseudo-record                                      |
| 128808965335184       | 'Oralbroadcasting', 'buy_2d_warmroom05_light_male', 32      |
| 128808965335184       | 'Oralbroadcasting', 'buy_2d_warmroom05_light_female', 88304 |
| 128808965335184       | 'Oralbroadcasting', 'buy_2d_warmroom05_light_female', 88305 |
| 128808965335184       | 'Oralbroadcasting', 'buy_2d_warmroom05_light_female', 88375 |

-- ...
| 128808965335184       | 'Oralbroadcasting', 'buy_2d_warmroom05_light_female', 90396 |
| 128808965335048       | supremum pseudo-record                                      |
| 128808965335048       | 'Oralbroadcasting', 'buy_2d_warmroom05_light_female', 81529 |
| 128808965335048       | 'Oralbroadcasting', 'buy_2d_warmroom05_light_female', 81599 |
| 128808965335048       | 'Oralbroadcasting', 'buy_2d_warmroom05_light_female', 81600 |
| 128808965335048       | 'Oralbroadcasting', 'buy_2d_warmroom05_light_female', 81601 |
| 128808965335048       | 'Oralbroadcasting', 'buy_2d_warmroom05_light_female', 8     |

-- ...
| 128808922364576       | 'Oralbroadcasting', 'buy_2d_cold_light_female', 22359       |
| 128808922364576       | 'Oralbroadcasting', 'buy_2d_cold_light_female', 22360       |
| 128808922352584       | supremum pseudo-record                                      |
| 128808922352584       | 'Oralbroadcasting', 'buy_2d_cold_light_female', 7312        |
```



### `X, REC_NOT_CAP`

`X,REC_NOT_GAP`： 没有 GAP，只锁定了记录本身。我们来看一下 `LOCK_MODE` 的值为 `X,REC_NOT_GAP` 的数据样例

```sql
***************************[ 29967. row ]***************************
ENGINE                | INNODB
ENGINE_LOCK_ID        | 128809141538008:2:3563:35:128808952736104
ENGINE_TRANSACTION_ID | 2858
THREAD_ID             | 89
EVENT_ID              | 11
OBJECT_SCHEMA         | test
OBJECT_NAME           | camera_widget_light
PARTITION_NAME        | <null>
SUBPARTITION_NAME     | <null>
INDEX_NAME            | PRIMARY
OBJECT_INSTANCE_BEGIN | 128808952736104
LOCK_TYPE             | RECORD
LOCK_MODE             | X,REC_NOT_GAP
LOCK_STATUS           | GRANTED
LOCK_DATA             | 90394
***************************[ 29968. row ]***************************
ENGINE                | INNODB
ENGINE_LOCK_ID        | 128809141538008:2:3563:36:128808952736104
ENGINE_TRANSACTION_ID | 2858
THREAD_ID             | 89
EVENT_ID              | 11
OBJECT_SCHEMA         | test
OBJECT_NAME           | camera_widget_light
PARTITION_NAME        | <null>
SUBPARTITION_NAME     | <null>
INDEX_NAME            | PRIMARY
OBJECT_INSTANCE_BEGIN | 128808952736104
LOCK_TYPE             | RECORD
LOCK_MODE             | X,REC_NOT_GAP
LOCK_STATUS           | GRANTED
LOCK_DATA             | 90395
```



`INDEX_NAME` 为 `PRIMARY`，这个是对于主键上的记录锁。可以验证一下这些数据是啥

```sql
select detailed_scene from camera_widget_light where id in (select lock_data from performance_schema.data_locks where lock_mode = 'X,REC_NOT_GAP') group by detailed_scene;

+------------------+
| detailed_scene   |
+------------------+
| Oralbroadcasting |
+------------------+

select count(1) from camera_widget_light where id in (select lock_data from performance_schema.data_locks where lock_mode = 'X,REC_NOT_GAP');

+----------+
| count(1) |
+----------+
| 29968    |
+----------+
```

这个数量正好是 `Oralbroadcasting` 的总和，所以 MySQL 对于主键上的所有值满足 `detailed_scene = 'Oralbroadcasting'` 的数据在其 clustered index 上了 record lock，防止数据被其他事务修改





### `X, GAP`

再来看一下 `X,GAP` 的数据，这个只有一条记录

```sql
select * from performance_schema.data_locks where lock_mode = 'X,GAP'\G;

***************************[ 1. row ]***************************
ENGINE                | INNODB
ENGINE_LOCK_ID        | 128809141538008:2:164:2:128808952736208
ENGINE_TRANSACTION_ID | 2858
THREAD_ID             | 89
EVENT_ID              | 11
OBJECT_SCHEMA         | test
OBJECT_NAME           | camera_widget_light
PARTITION_NAME        | <null>
SUBPARTITION_NAME     | <null>
INDEX_NAME            | camera_widget_light_detailed_scene_name_8db6c0b0_idx
OBJECT_INSTANCE_BEGIN | 128808952736208
LOCK_TYPE             | RECORD
LOCK_MODE             | X,GAP
LOCK_STATUS           | GRANTED
LOCK_DATA             | 'buy_2d', 'buy_2d_cold_light_female', 8

```

我们可以发现这里的 `LOCK_DATA` 竟然是 `buy_2d` 而不是 `Oralbroadcasting`。我们的 `for update` 的查询条件明明指定的是 `detailed_scene = 'Oralbroadcasting'`，为什么锁被加到了这个上面？

这个和数据有关系，我们的索引是 `camera_widget_light_detailed_scene_name_8db6c0b0_idx`，他是 `("detailed_scene","name")`的联合索引，按照 B+ 树的构造，大概是这样的

```
              -------- [name1, name5]
             /
  Oralbroadcasting --- [name5, name....]
             \
              -------- [...., ....]

              -------- [name1, name5]
             /
  buy_2d	 --- [name5, name....]
             \
              -------- [...., ....]
```

`b` 的 unicode 的值比 `O` 大，所以节点在右。因为我们 `for update` 走了左边的，所以需要对 `Oralbroadcasting` 到 `buy_2d` 之间的一个区间进行加锁，而 gap lock 是左开右开的，所以数据显示的是 id=8 的记录

```sql
select id, detailed_scene, name from camera_widget_light where detailed_scene = 'buy_2d' order by name limit 2;

+----+----------------+--------------------------+
| id | detailed_scene | name                     |
+----+----------------+--------------------------+
| 8  | buy_2d         | buy_2d_cold_light_female |
| 33 | buy_2d         | buy_2d_cold_light_female |
+----+----------------+--------------------------+
```

如果我们将所有 `buy_2d` 的命名为 `AAA`，它导致节点移动到 `Oralbroadcasting` 的左边。然后重新执行

```sql
begin;
select * from camera_widget_light where detailed_scene = 'Oralbroadcasting' and record_name = 'ABC' for update;

select count(1), OBJECT_NAME,INDEX_NAME, LOCK_TYPE,LOCK_MODE, LOCK_STATUS FROM performance_schema.data_locks group by OBJECT_NAME,INDEX_NAME, LOCK_TYPE,LOCK_MODE, LOCK_STATUS\G;

***************************[ 1. row ]***************************
count(1)    | 1
OBJECT_NAME | camera_widget_light
INDEX_NAME  | <null>
LOCK_TYPE   | TABLE
LOCK_MODE   | IX
LOCK_STATUS | GRANTED
***************************[ 2. row ]***************************
count(1)    | 30080
OBJECT_NAME | camera_widget_light
INDEX_NAME  | camera_widget_light_detailed_scene_name_8db6c0b0_idx
LOCK_TYPE   | RECORD
LOCK_MODE   | X
LOCK_STATUS | GRANTED
***************************[ 3. row ]***************************
count(1)    | 29968
OBJECT_NAME | camera_widget_light
INDEX_NAME  | PRIMARY
LOCK_TYPE   | RECORD
LOCK_MODE   | X,REC_NOT_GAP
LOCK_STATUS | GRANTED
```

可以发现 `X, GAP` 类型的锁已经没有了，但是总的数量为 60049 条，比之前的 60048 多了一条。具体是在 `X` 类型的上面多了两条，而且都是 `supremum pseudo-record` 的记录。



### 死锁日志

除了像上面那样具体分析，死锁日志提供了较为快捷的方法来判断死锁的成因



```sql
show engine innodb status\G;

------------------------
LATEST DETECTED DEADLOCK
------------------------
2024-05-05 14:56:48 123714584708864
*** (1) TRANSACTION:
TRANSACTION 1807, ACTIVE 21 sec starting index read
mysql tables in use 1, locked 1
LOCK WAIT 2808 lock struct(s), heap size 303224, 60048 row lock(s)
MySQL thread id 10, OS thread handle 123715421468416, query id 49 172.22.0.1 root executing
select * from camera_widget_light where detailed_scene = 'buy_2d' and record_name = 'ABC' for update

*** (1) HOLDS THE LOCK(S):
RECORD LOCKS space id 2 page no 22 n bits 384 index camera_widget_light_detailed_scene_name_8db6c0b0_idx of table `test`.`camera_widget_light` trx id 1807 lock_mode X
Record lock, heap no 1 PHYSICAL RECORD: n_fields 1; compact format; info bits 0
 0: len 8; hex 73757072656d756d; asc supremum;;

Record lock, heap no 2 PHYSICAL RECORD: n_fields 3; compact format; info bits 0
 0: len 16; hex 4f72616c62726f616463617374696e67; asc Oralbroadcasting;;
 1: len 24; hex 6275795f32645f636f6c645f6c696768745f66656d616c65; asc buy_2d_cold_light_female;;
 2: len 4; hex 80000002; asc     ;;

-- 省略

Record lock, heap no 311 PHYSICAL RECORD: n_fields 3; compact format; info bits 0
 0: len 16; hex 4f72616c62726f616463617374696e67; asc Oralbroadcasting;;
 1: len 24; hex 6275795f32645f636f6c645f6c696768745f66656d616c65; asc buy_2d_cold_light_female;;
 2: len 4; hex 80001859; asc    Y;;


*** (1) WAITING FOR THIS LOCK TO BE GRANTED:
RECORD LOCKS space id 2 page no 164 n bits 128 index camera_widget_light_detailed_scene_name_8db6c0b0_idx of table `test`.`camera_widget_light` trx id 1807 lock_mode X waiting
Record lock, heap no 2 PHYSICAL RECORD: n_fields 3; compact format; info bits 0
 0: len 6; hex 6275795f3264; asc buy_2d;;
 1: len 24; hex 6275795f32645f636f6c645f6c696768745f66656d616c65; asc buy_2d_cold_light_female;;
 2: len 4; hex 80000008; asc     ;;


*** (2) TRANSACTION:
TRANSACTION 1808, ACTIVE 11 sec starting index read
mysql tables in use 1, locked 1
LOCK WAIT 2879 lock struct(s), heap size 319608, 120022 row lock(s)
MySQL thread id 14, OS thread handle 123715410982656, query id 50 172.22.0.1 root executing
select * from camera_widget_light where detailed_scene = 'Oralbroadcasting' and record_name = 'ABC' for update

*** (2) HOLDS THE LOCK(S):
RECORD LOCKS space id 2 page no 164 n bits 128 index camera_widget_light_detailed_scene_name_8db6c0b0_idx of table `test`.`camera_widget_light` trx id 1808 lock_mode X
Record lock, heap no 1 PHYSICAL RECORD: n_fields 1; compact format; info bits 0
 0: len 8; hex 73757072656d756d; asc supremum;;

Record lock, heap no 2 PHYSICAL RECORD: n_fields 3; compact format; info bits 0
 0: len 6; hex 6275795f3264; asc buy_2d;;
 1: len 24; hex 6275795f32645f636f6c645f6c696768745f66656d616c65; asc buy_2d_cold_light_female;;
 2: len 4; hex 80000008; asc     ;;

-- 省略

Record lock, heap no 58 PHYSICAL RECORD: n_fields 3; compact format; info bits 0
 0: len 6; hex 6275795f3264; asc buy_2d;;
 1: len 24; hex 6275795f32645f636f6c645f6c696768745f66656d616c65; asc buy_2d_cold_light_female;;
 2: len 4; hex 800002aa; asc     ;;


*** (2) WAITING FOR THIS LOCK TO BE GRANTED:
RECORD LOCKS space id 2 page no 22 n bits 384 index camera_widget_light_detailed_scene_name_8db6c0b0_idx of table `test`.`camera_widget_light` trx id 1808 lock_mode X waiting
Record lock, heap no 2 PHYSICAL RECORD: n_fields 3; compact format; info bits 0
 0: len 16; hex 4f72616c62726f616463617374696e67; asc Oralbroadcasting;;
 1: len 24; hex 6275795f32645f636f6c645f6c696768745f66656d616c65; asc buy_2d_cold_light_female;;
 2: len 4; hex 80000002; asc     ;;

*** WE ROLL BACK TRANSACTION (1)

```



日志里面已经写的很全的。通常建议使用中间件或者修改类库来在 SQL 语句中注入 tracing ID 的注释，这样我们在死锁日志中可以反向找到对应的请求。开源的方案有 https://google.github.io/sqlcommenter/



## 解决方法

方法有很多种：

- 对于 `update_or_create` 用到的条件需要上索引，比如上面的例子，加一个 `record_name` 或者 `(detailed_scene, record_name)` 的索引都能解决问题
- 乐观控制，使用 `insert on duplicate` https://dev.mysql.com/doc/refman/8.3/en/insert-on-duplicate.html

- 看业务是否允许，降低事务级别为 RC，参考

  - [17.7.2.1 Transaction Isolation Levels](https://dev.mysql.com/doc/refman/8.0/en/innodb-transaction-isolation-levels.html)
  - [MySQL 8.0 RC级别下 select...for update 锁不存在的记录](https://zhuanlan.zhihu.com/p/686257408)

  

## Reference

- [select…for update再insert造成deadlock的陷阱](https://notes.andywu.tw/2021/select-for-update%E5%86%8Dinsert%E9%80%A0%E6%88%90deadlock%E7%9A%84%E9%99%B7%E9%98%B1/)
- [ 【原创】惊！史上最全的select加锁分析(MySQL)](https://www.cnblogs.com/rjzheng/p/9950951.html)

- [innodb select for update 没有满足条件的记录的情况下 是怎么加锁的呢？](https://segmentfault.com/q/1010000010296374)

- [Does "SELECT FOR UPDATE" prevent other connections inserting when the row is not present?](https://stackoverflow.com/questions/3601895/does-select-for-update-prevent-other-connections-inserting-when-the-row-is-not)
- [17.7.4 Phantom Rows](https://dev.mysql.com/doc/refman/8.0/en/innodb-next-key-locking.html)
- [MySQL条件查询不存在行，使用for update加锁的分析](https://blog.winsky.wang/%E6%95%B0%E6%8D%AE%E5%BA%93/MySQL%E6%9D%A1%E4%BB%B6%E6%9F%A5%E8%AF%A2%E4%B8%8D%E5%AD%98%E5%9C%A8%E8%A1%8C%EF%BC%8C%E4%BD%BF%E7%94%A8for-update%E5%8A%A0%E9%94%81%E7%9A%84%E5%88%86%E6%9E%90/)
- [postgres 如何锁住一条不存在的记录？](https://www.v2ex.com/t/608374q)
- [MySQL-InnoDB中的锁](https://smartkeyerror.com/MySQL-InnoDB-Lock)



  
