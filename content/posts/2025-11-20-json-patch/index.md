+++
title = "RFC 6902 JSON Patch"
summary = ""
description = ""
categories = ["RFC"]
tags = []
date = 2025-11-20T13:03:22+09:00
draft = false

+++

为了避免使用 PUT 全量更新整个 JSON 文档，JSON Patch (RFC 6902) 定义了一种能够描述增量更新操作序列的结构化格式，并通常与 HTTP PATCH 方法一起使用

一个 JSON Patch 的 HTTP 请求示例如下

```
PATCH /my/data HTTP/1.1
Host: example.org
Content-Length: 326
Content-Type: application/json-patch+json
If-Match: "abc123"

[
  { "op": "test", "path": "/a/b/c", "value": "foo" },
  { "op": "remove", "path": "/a/b/c" },
  { "op": "add", "path": "/a/b/c", "value": [ "foo", "bar" ] },
  { "op": "replace", "path": "/a/b/c", "value": 42 },
  { "op": "move", "from": "/a/b/c", "path": "/a/b/d" },
  { "op": "copy", "from": "/a/b/d", "path": "/a/b/e" }
]
```

- `op`: 操作类型
- `path`: JSON Pointer 格式的文档路径

根据 `op` 不同，可以具有 `value` 或者 `from` 等字段

## JSON Pointer (RFC 6901)

RFC 6901 提出了 JSON Pointer 这一个概念，用于表示 JSON 文档中的路径

ABNF 定义

```
json-pointer    = *( "/" reference-token )
reference-token = *( unescaped / escaped )
unescaped       = %x00-2E / %x30-7D / %x7F-10FFFF
   ; %x2F ('/') and %x7E ('~') are excluded from 'unescaped'
escaped         = "~" ( "0" / "1" )
  ; representing '~' and '/', respectively
```

规则说明

- JSON Pointer 是一个 Unicode 字符串，由多个 `reference-token` 组成
- 每个 token 前必须有 `/` 开头（除非是空指针表示根）
- 字符 `~` 和 `/` 在 JSON Pointer 中有特殊含义，因此必须进行转义:
  - `~` -> `~0`
  - `/` -> `~1`
- 如果指向的是数组，那么 `path` 的 token 必须是:
  - 一个无前导零的十进制整数，表示数组下标
  - 或者是单独的 `-`，表示数组末尾之后的位置

解码时 **必须先将 `~1` 替换为 `/`，再将 `~0` 替换为 `~`**，以避免错误解析

## RFC-6902

JSON Patch 是一个由多个操作组成的数组，每个操作按顺序执行，且整个 PATCH 必须具备原子性 (参考 RFC 5789)

### `add` 操作

向目标位置新增一个值:

- 若目标是数组:
  - 按索引插入
  - 索引不能大于数组长度
  - 索引为 `-` 表示 append

- 若目标是对象:
  - 如果 key 存在则替换
  - 如果 key 不存在则新增

- 若路径不存在中间结构，需要抛出错误


比如目标文档为

```json
{ "foo": "bar" }
```


JSON Patch 为

```json
[
  { "op": "add", "path": "/baz/bat", "value": "qux" }
]

```

因为 `/baz` 不存在，此 patch 失败


### `remove` 操作

删除指定路径的值。如果路径不存在，必须返回错误


### `replace` 操作

等价于：
1.  对该路径执行 `remove`
2.  再执行 `add`（使用替换值）

但它必须视为一个原子操作


### `move` 操作

从 `from` 路径取值，删除它，然后将其加入 `path` 的位置

注意
- 路径不能移动到其自身的子路径下
- `from` 必须存在


### `copy` 操作

复制 `from` 位置的值，并将其新增到 `path`


### `test` 操作

测试目标路径的值是否与给定的 `value` 完全相等。不相等则返回错误，整个 PATCH 被视为失败且不修改文档
比较规则必须遵循 JSON 数据类型，如字符串、数字、布尔值必须严格相等

### 错误与原子性

PATCH 操作必须具备原子性: 如果其中任何一个操作失败，整个 PATCH **不能产生部分修改**

示例：

```json
[
  { "op": "replace", "path": "/a/b/c", "value": 42 },
  { "op": "test", "path": "/a/b/c", "value": "C" }
]
```

因为 `test` 操作失败，所以:

- `replace` 的效果不会被保留
- 文档完全不修改

## Reference

- https://datatracker.ietf.org/doc/html/rfc6901
- https://datatracker.ietf.org/doc/html/rfc6902

