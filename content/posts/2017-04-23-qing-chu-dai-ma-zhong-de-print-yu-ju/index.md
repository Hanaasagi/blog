
+++
title = "清除代码中的 print 语句"
summary = ''
description = ""
categories = []
tags = []
date = 2017-04-23T13:27:48+08:00
draft = false
+++

因为最近在阅读 Tornado 源码，会利用 `print` 语句去探查内部结构。但是遗留在各种文件的 `print` 语句一旦不想再使用，清理起来很是烦人  
蠢作者记得曾看过使用 AST 模块去修改语法树节点的例子  

```python
import ast

class RewriteAddToSub(ast.NodeTransformer):
    def visit_Add(self, node):
        node = ast.Sub()
        return node

node = ast.parse('1 + 2', mode='eval')
node = RewriteAddToSub().visit(node)
print(eval(compile(node, '<string>', 'eval')))
```

那么这样一来，只要遍历一遍 AST 将 `print` 替换为 `pass` 便可以了。但是如何才能将 AST 还原为 Python 代码呢  
经过一番查找，发现有一个 astor 模块封装了 ast 的多数操作，并实现了转换  

```python
import ast
import astor

class RemovePrint(ast.NodeTransformer):
    def visit_Print(self, node):
        return ast.Pass()

def remove_print(filepath, output=None):
    source = astor.to_source(RemovePrint().visit(astor.parsefile(filepath)))
    if output is None:
        output = filepath
    with open(output, 'w') as f:
        f.write(source)
```
### Reference
[Parse a .py file, read the AST, modify it, then write back the modified source code](http://stackoverflow.com/questions/768634/parse-a-py-file-read-the-ast-modify-it-then-write-back-the-modified-source-c)
    