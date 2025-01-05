+++
title = "MySQL 中 ORDER BY 导致的索引选择问题"
summary = ""
description = ""
categories = ["MySQL"]
tags = ["slow-query",  "MySQL", "index"]
date = 2025-01-05T12:01:39+09:00
draft = false

+++



最近遇到一个问题，MySQL 选择使用了耗时更长索引





## 问题复现

假设我们的表名为 `digital_assets`，这张表的索引如下

| Table          | Non_unique | Key_name                                          | Seq_in_index | Column_name  | Collation | Cardinality | Sub_part | Packed | Null | Index_type | Comment | Index_comment | Visible | Expression |
| -------------- | ---------- | ------------------------------------------------- | ------------ | ------------ | --------- | ----------- | -------- | ------ | ---- | ---------- | ------- | ------------- | ------- | ---------- |
| digital_assets | 0          | PRIMARY                                           | 1            | id           | A         | 3242968     |          |        |      | BTREE      |         |               | YES     |            |
| digital_assets | 1          | digital_assets_work_id_storage_id_f466ba7f_idx    | 1            | work_id      | A         | 300676      |          |        | YES  | BTREE      |         |               | YES     |            |
| digital_assets | 1          | digital_assets_work_id_storage_id_f466ba7f_idx    | 2            | storage_id   | A         | 3390024     |          |        |      | BTREE      |         |               | YES     |            |
| digital_assets | 1          | digital_assets_user_id_storage_type_a447ac13_idx  | 1            | user_id      | A         | 76032       |          |        |      | BTREE      |         |               | YES     |            |
| digital_assets | 1          | digital_assets_user_id_storage_type_a447ac13_idx  | 2            | storage_type | A         | 133068      |          |        |      | BTREE      |         |               | YES     |            |
| digital_assets | 1          | digital_assets_created_at_f8622066                | 1            | created_at   | A         | 3174621     |          |        |      | BTREE      |         |               | YES     |            |
| digital_assets | 1          | digital_assets_storage_id_is_deleted_859e1678_idx | 1            | storage_id   | A         | 575536      |          |        |      | BTREE      |         |               | YES     |            |
| digital_assets | 1          | digital_assets_storage_id_is_deleted_859e1678_idx | 2            | is_deleted   | A         | 664478      |          |        |      | BTREE      |         |               | YES     |            |

------



- `digital_assets_created_at_f8622066` 的 Cardinality 为 3174621

- `digital_assets_storage_id_is_deleted_859e1678_idx` 索引显示了 `storage_id` 列的 `Cardinality` 为 575536，而 `is_deleted` 列的 `Cardinality` 为 664478。



存在问题的查询如下

```sql
EXPLAIN ANALYZE
SELECT
    `digital_assets`.`id`,
    `digital_assets`.`name`,
    `digital_assets`.`original_name`,
    `digital_assets`.`work_id`,
    `digital_assets`.`user_id`,
    `digital_assets`.`is_published`,
    `digital_assets`.`is_private`,
    `digital_assets`.`source`,
    `digital_assets`.`storage_id`,
    `digital_assets`.`storage_type`,
    `digital_assets`.`is_deleted`,
    `digital_assets`.`created_at`,
    `digital_assets`.`updated_at`
FROM
    `digital_assets`
WHERE
    (
        `digital_assets`.`is_published` = 0
        AND `digital_assets`.`is_deleted` = 0
        AND `digital_assets`.`storage_id` = 1341070
    )
ORDER BY
    `digital_assets`.`created_at` DESC
LIMIT
    1;
```



返回结果

```
-> Limit: 1 row(s) (cost=0.90 rows=0) (actual time=599.639..599.640 rows=1 loops=1)
   -> Filter: ((digital_assets.storage_id = 1341070) and (digital_assets.is_deleted = 0) and (digital_assets.is_published = 0)) (cost=0.90 rows=0) (actual time=599.639..599.639 rows=1 loops=1)
   -> Index scan on digital_assets using digital_assets_created_at_f8622066 (reverse) (cost=0.90 rows=846) (actual time=0.028..575.702 rows=378455 loops=1)
```



首先根据过滤条件来说，`digital_assets_storage_id_is_deleted_859e1678_idx` 索引是可以命中 `WHERE` 中的两个条件的。如果使用这个索引，那么 MySQL 需要回表过滤一次 `is_published = 0` 并且根据 `created_at` 进行倒排，选择出 `created_at` 值最大的记录返回。最坏的情况是满足 `WHERE` 条件的数据集很大，但是在此种情形下，因为 `LIMIT 1` 的条件，大可使用诸如 Top-K 的优化策略避免全排序



但是我们通过 `EXPLAIN ANALYZE` 可以看到，MySQL 在这个时候使用的是 `digital_assets_created_at_f8622066` 这个索引。这会直接倒序遍历索引，逐个回表比较 `WHERE` 中的三个条件，如果满足那么直接返回。最坏的情况下是全表扫描，最好的情况下是第一条数据正好满足条件



## USE INDEX

当我们遇到这种 MySQL 索引选择不佳的问题时，可以通过 `USE INDEX ` 来指定索引。我们看一下同样的语句在使用 `digital_assets_storage_id_is_deleted_859e1678_idx` 后的结果

```sql
EXPLAIN ANALYZE
SELECT
    `digital_assets`.`id`,
    `digital_assets`.`name`,
    `digital_assets`.`original_name`,
    `digital_assets`.`work_id`,
    `digital_assets`.`user_id`,
    `digital_assets`.`is_published`,
    `digital_assets`.`is_private`,
    `digital_assets`.`source`,
    `digital_assets`.`storage_id`,
    `digital_assets`.`storage_type`,
    `digital_assets`.`is_deleted`,
    `digital_assets`.`created_at`,
    `digital_assets`.`updated_at`
FROM
    `digital_assets`
  
USE INDEX (digital_assets_storage_id_is_deleted_859e1678_idx)
WHERE
    (
        `digital_assets`.`is_published` = 0
        AND `digital_assets`.`is_deleted` = 0
        AND `digital_assets`.`storage_id` = 1341070
    )
ORDER BY
    `digital_assets`.`created_at` DESC
LIMIT
    1;
```



返回结果

```
-> Limit: 1 row(s) (cost=3192.66 rows=1) (actual time=11.266..11.266 rows=1 loops=1)
 -> Sort row IDs: digital_assets.created_at DESC, limit input to 1 row(s) per chunk (cost=3192.66 rows=4002) (actual time=11.265..11.265 rows=1 loops=1)
 -> Filter: (digital_assets.is_published = 0) (actual time=0.020..10.796 rows=4001 loops=1)
 -> Index lookup on digital_assets using digital_assets_storage_id_is_deleted_859e1678_idx (storage_id=1341070, is_deleted=0) (actual time=0.019..10.425 rows=4002 loops=1)
```



对比一下可以发现，使用 `digital_assets_storage_id_is_deleted_859e1678_idx` 索引，每一步的 `cost` 值都很高，但是实际执行下来比之前的快非常多。这种情况的耗时主要在 `Sort row` 上面。而使用 `digital_assets_created_at_f8622066` 索引时，每一步的 `cost` 值很低，但是 `Filter` 这一步的时候，`actual time` 高达 599.639



当然耗时并非唯一考量因素，不过我们注意一下我们所建立的索引是否被 MySQL 真正地使用到了。顺便说一下，这个 SQL 在表数据量小的情况下是使用的 `digital_assets_storage_id_is_deleted_859e1678_idx`  索引





## prefer_ordering_index

有一个参数是和这个行为相关的，根据 MySQL 的文档 https://dev.mysql.com/doc/refman/8.0/en/limit-optimization.html

> 在 MySQL 8.0.21 之前，对于包含 `ORDER BY` 或 `GROUP BY` 且带有 `LIMIT` 的查询，优化器默认会优先选择有序索引来提升查询性能，这种行为无法手动关闭。但从 MySQL 8.0.21 开始，可以通过设置 `optimizer_switch` 系统变量中的 `prefer_ordering_index` 参数为 `off` 来关闭这一优化，允许使用其他更高效的查询策略。



MySQL 会倾向于使用 `ORDER BY` 中的字段的索引来避免 filesort。当然这个 `ORDER BY` 的具体内容还相关，比如我们经常 `ORDER BY id`，但是即使开启 `prefer_ordering_index` ，MySQL 也没有直接使用主键排序然后逐条回表过滤的。



我们试一下关闭 `prefer_ordering_index`

```sql
SELECT @@optimizer_switch LIKE '%prefer_ordering_index=on%';
SET optimizer_switch = "prefer_ordering_index=off";

EXPLAIN ANALYZE
SELECT
    `digital_assets`.`id`,
    `digital_assets`.`name`,
    `digital_assets`.`original_name`,
    `digital_assets`.`work_id`,
    `digital_assets`.`user_id`,
    `digital_assets`.`is_published`,
    `digital_assets`.`is_private`,
    `digital_assets`.`source`,
    `digital_assets`.`storage_id`,
    `digital_assets`.`storage_type`,
    `digital_assets`.`is_deleted`,
    `digital_assets`.`created_at`,
    `digital_assets`.`updated_at`
FROM
    `digital_assets`
WHERE
    (
        `digital_assets`.`is_published` = 0
        AND `digital_assets`.`is_deleted` = 0
        AND `digital_assets`.`storage_id` = 1341070
    )
ORDER BY
    `digital_assets`.`created_at` DESC
LIMIT
    1;
```





返回结果

```
> Limit: 1 row(s)  (cost=3192.66 rows=1) (actual time=11.097..11.097 rows=1 loops=1)
  -> Sort row IDs: digital_assets.created_at DESC, limit input to 1 row(s) per chunk  (cost=3192.66 rows=4002) (actual time=11.096..11.096 rows=1 loops=1)
    -> Filter: (digital_assets.is_published = 0)  (actual time=0.021..10.625 rows=4001 loops=1)
      -> Index lookup on digital_assets using digital_assets_storage_id_is_deleted_859e1678_idx (storage_id=1341070, is_deleted=0)  (actual time=0.019..10.255 rows=4002 loops=1)
```





可以看到 MySQL 现在选择 `digital_assets_storage_id_is_deleted_859e1678_idx` 索引



再来看一下 `ORDER BY id` 的结果

```sql
EXPLAIN ANALYZE
SELECT
    `digital_assets`.`id`,
    `digital_assets`.`name`,
    `digital_assets`.`original_name`,
    `digital_assets`.`work_id`,
    `digital_assets`.`user_id`,
    `digital_assets`.`is_published`,
    `digital_assets`.`is_private`,
    `digital_assets`.`source`,
    `digital_assets`.`storage_id`,
    `digital_assets`.`storage_type`,
    `digital_assets`.`is_deleted`,
    `digital_assets`.`created_at`,
    `digital_assets`.`updated_at`
FROM
    `digital_assets`
WHERE
    (
        `digital_assets`.`is_published` = 0
        AND `digital_assets`.`is_deleted` = 0
        AND `digital_assets`.`storage_id` = 1341070
    )
ORDER BY
    `digital_assets`.`id` DESC
LIMIT
    1;
```



返回结果

```
-> Limit: 1 row(s) (cost=3632.51 rows=1) (actual time=0.026..0.026 rows=1 loops=1)
   -> Filter: (digital_assets.is_published = 0) (cost=3632.51 rows=400) (actual time=0.025..0.025 rows=1 loops=1)
   -> Index lookup on digital_assets using digital_assets_storage_id_is_deleted_859e1678_idx (storage_id=1341070, is_deleted=0; iterate backwards) (cost=3632.51 rows=4002) (actual time=0.024..0.024 rows=1 loops=1)
```



总的来说，MySQL 索引的选择受多种因素的影响，包括表的大小、索引的 Cardinality、查询的具体条件和排序方式等。即使在理论上某个索引看似最优，但在实际执行时，优化器可能会基于统计信息做出不同的选择。



因此最终还是

- 索引根据业务场景建立
- 不能数据规模下索引的使用情况是不一定的
- MySQL 有可能选择非最佳索引，需要实际在生产试一下



关于索引选择不佳的问题一搜一大片，可以看一下 Referenece 中的几个链接

###  Reference

- https://medium.com/@xuan11290/understand-mysql-query-optimizer-in-preferring-order-index-e974c07c77bc

- https://jfg-mysql.blogspot.com/2022/11/bad-optimizer-plan-on-queries-combining-where-order-by-and-limit.html





