+++
title = "整理一些 Git 的实用配置/技巧"
summary = ""
description = ""
categories = [""]
tags = []
date = 2024-06-02T19:01:23+09:00
draft = false

+++

## Blame 中忽略特定 commit

比如 code format 等场景，会生成干扰 commit history 的无用 commit。这个时候我们可以通过在 `.git-blame-ignore-revs` 中罗列这些需要被忽略的 commit

```
 # Format commit 1 SHA:
 1234af5.....
 # Format commit 2 SHA:
 2e4ac56.....
```

本地 git 配置 `git config blame.ignoreRevsFile .git-blame-ignore-revs`，这样就会在 `git blame` 中忽略它们。GitHub 的 blame view 同样支持这个功能

文档参考

- https://git-scm.com/docs/git-blame#Documentation/git-blame.txt---ignore-revs-fileltfilegt
- https://docs.github.com/en/repositories/working-with-files/using-files/viewing-a-file#ignore-commits-in-the-blame-view

## 修改语言统计信息

GitHub 的项目语言统计信息是通过 https://github.com/github-linguist/linguist 这个库扫描计算的。其支持在 `.gitattributes` 文件中 patch 掉一些属性来修改统计行为

### linguist-detectable

默认情况下，只有 [`languages.yml`](https://github.com/github-linguist/linguist/blob/master/lib/linguist/languages.yml) 中类型为 `programming` 或 `markup` 的语言才会包含在语言统计中。

比如下面的 `2-Dimensional Array` 的类型为 `data` 是不会被包含在语言统计中的，但是我们可以通过 `linguist-detectable` 将其加入

```yaml
1C Enterprise:
  type: programming
  color: "#814CCC"
  extensions:
    - ".bsl"
    - ".os"
  tm_scope: source.bsl
  ace_mode: text
  language_id: 0
2-Dimensional Array:
  type: data
  color: "#38761D"
  extensions:
    - ".2da"
  tm_scope: source.2da
  ace_mode: text
  language_id: 387204628
```

用法示例，`-linguist-detectable` 为移除，`linguist-detectable` 为添加

```gitattributes
*.kicad_pcb linguist-detectable
*.sch linguist-detectable
tools/export_bom.py -linguist-detectable
```

如果语言本身没有定义在 [`languages.yml`](https://github.com/github-linguist/linguist/blob/master/lib/linguist/languages.yml) 中定义，那么无效

### linguist-documentation

与 `linguist-vendored` 类似，linguist 默认从项目的语言统计中移除了文档路径。预定义路径在 [`documentation.yml`](https://github.com/github-linguist/linguist/blob/master/lib/linguist/documentation.yml) 中，如果我们想自定义，那么需要使用 `linguist-documentation`

```gitattributes
# Apply override to all files in the directory
project-docs/* linguist-documentation
# Apply override to a specific file
docs/formatter.rb -linguist-documentation
# Apply override to all files and directories in the directory
ano-dir/** linguist-documentation
```

### linguist-generated

用于标记生成的代码文件。额外的好处是，与 vendor 或者 documentation 不同，这些生成文件不会显示在 diff 中。 [`generated.rb`](https://github.com/github-linguist/linguist/blob/master/lib/linguist/generated.rb) 列出了常见的生成路径，并将其从语言统计中移除

```gitattributes
Api.elm linguist-generated
```

```gitattributes
# Example of a `.gitattributes` file which reclassifies `.rb` files as Java:
*.rb linguist-language=Java

# Replace any whitespace in the language name with hyphens:
*.glyphs linguist-language=OpenStep-Property-List

# Language names are case-insensitive and may be specified using an alias.
# So, the following three lines are all functionally equivalent:
*.es linguist-language=js
*.es linguist-language=JS
*.es linguist-language=JAVASCRIPT
```

### linguist-vendored

自定义 `vendor` 路径，在语言统计信息中移除

```gitattributes
# Apply override to all files in the directory
special-vendored-path/* linguist-vendored
# Apply override to a specific file
jquery.js -linguist-vendored
# Apply override to all files and directories in the directory
ano-dir/** linguist-vendored
```

文档参考

- https://github.com/github-linguist/linguist/blob/master/docs/overrides.md
- https://git-scm.com/docs/gitattributes

## 根据目录使用不同的 Git 配置

比如工作所使用的项目都在 `xxx-company` 中，我们可以为它单独指定一个 gitconfig。在 `~/gitconfig` 中添加 `includeIf`

```
[user]
	name = xychan
	email = xychan@gmail.com

[includeIf "gitdir:**/xxx-company/**"]
path = ~/.shitworkconfig
```

`path` 中指定的路径，写自己工作时使用的 config 信息，同名项可以覆盖掉 `~/.gitconfig`

```
[user]
	name = fuck-996
	email = fuck996@gmail.com
```

文档参考

- https://git-scm.com/docs/git-config#_includes

## git-rerere

记录之前冲突的解决方式，之后自动应用，可以避免重复解决冲突。默认关闭，需要通过 `git config rerere.enabled true` 开启

比如

```
master: 1 2
feat-A: 1 3 4
feat-B: 1 3 5
```

如果 2 和 3 有冲突，那么 `feat-A` 去 rebase master 的时候，人工解决一次。之后 `feat-B` 的去 rebase 的时候可以自动重放。如果平时没有并行开发功能的需要，那么可以关掉，这个会占用一定的空间的

文档参考

- https://git-scm.com/docs/git-rerere

## git-worktree

比如你在开发一个功能，临时又插进来一个改动点，又让你马上提交。这个时候可以直接通过 `git worktree` 命令来解决

```shell

# git 会 clone `<source_branch>` 至 `<folder_path>` 目录，这个新 branch 会叫做 `<new_branch_name>`
$ git worktree add -b <new_branch_name> <folder_path> <source_branch>

# 列出来当前的 worktree
$ git worktree list

# 删除
# git worktree remove <folder_path>
```

文档参考

- https://git-scm.com/docs/git-worktree

## git-notes

为历史的 commit 添加额外的注释信息，可以用来写讨贼檄文

```shell
# 为指定 commit 添加信息
$ git notes add 76aa6ee

# 查看
$ git notes show 76aa6ee

# 删除
$ git notes remove 76aa6ee
```

notes 会显示在 commit message 的下面

```
commit 76aa6ee381fa02419970d97e6cc385bfd7bbb92a (HEAD -> test, master)
Author: Hanaasagi <ambiguous404@gmail.com>
Date:   Sun Jun 2 21:45:22 2024 +0900

    first

Notes:
    notes.....

```

文档参考

- https://git-scm.com/docs/git-notes
