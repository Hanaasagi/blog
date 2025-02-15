+++
title = "PEG 语法"
summary = ""
description = ""
categories = ["Compiler"]
tags = ["Compiler",  "LLVM"] 
date = 2025-02-15T11:22:33+09:00
draft = false

+++







学习使用一下 peg/leg 工具，主要是熟练一下 peg 的语法，文档在这里 https://man.archlinux.org/man/peg.1.en



peg 的语法并不统一，或者说是不同工具有不同的扩展。比如这个工具 https://github.com/yhirose/cpp-peglib，支持 `%whitespace` 自定义

```
Additive    <- Multiplicative '+' Additive / Multiplicative
Multiplicative   <- Primary '*' Multiplicative / Primary
Primary     <- '(' Additive ')' / Number
Number      <- < [0-9]+ >
%whitespace <- [ \t]*
```



## peg



下面使用的例子都使用 `man 1 peg` 中规定的语法。基本类似正则表达式，思路是递归下降，我们先看一个最简单的例子

```peg
root    <- skip (int skip)* eof
int     <- [0-9]+
skip    <- [ \n\r\t]*
eof     <- !.
```



输入 `" 123 456 "`，解析流程如下

1. 初始 `skip`：匹配开头空格。
2. 循环匹配 `int skip`：
   - 匹配 `"123"`，随后 `skip` 匹配中间空格。
   - 匹配 `"456"`，随后 `skip` 匹配末尾空格。
3. `eof` 检查：输入结束，解析成功。





我们对这个进行扩展，支持整数进制前缀和浮点数科学记数法

```peg
root    <- skip (float / int)? (one_ws (float / int))* eof

float   <- sign? float_ skip

int     <- sign? int_ skip

sign    <- [+\-]

float_  <- ((dec_int? '.' dec_int) / (dec_int '.')) ([eE] [-+]? dec_int)?
         / dec_int [eE] [-+]? dec_int

int_    <- '0b' bin_int
         / '0o' oct_int
         / '0x' hex_int
         / dec_int

dec_int <- dec dec_*
dec_    <- '_'? dec
dec     <- [0-9]

bin_int <- bin bin_*
bin_    <- '_'? bin
bin     <- [01]

oct_int <- oct oct_*
oct_    <- '_'? oct
oct     <- [0-7]

hex_int <- hex hex_*
hex_    <- '_'? hex
hex     <- [0-9a-fA-F]

ws      <- [ \n\r\t]
skip    <- ws*
one_ws  <- ws+
eof     <- !.

```



编写时候，遇到了几个问题



**1）关于第一行是 `int / float` 还是 `float / int`**

```
# 错误
root    <- skip (int / float)? (one_ws (int / float))* eof

# 正确
root    <- skip (float / int)? (one_ws (float / int))* eof
```



如果我们使用 `int / float `的写法，那么是无法解析 `3.14` 的。原因是

> The sequence operator *e*1 *e*2 first invokes *e*1, and if *e*1 succeeds, subsequently invokes *e*2 on the remainder of the input string left unconsumed by *e*1, and returns the result. If either *e*1 or *e*2 fails, then the sequence expression *e*1 *e*2 fails (consuming no input).
>
> The choice operator *e*1 / *e*2 first invokes *e*1, and if *e*1 succeeds, returns its result immediately. Otherwise, if *e*1 fails, then the choice operator [backtracks](https://en.wikipedia.org/wiki/Backtracking) to the original input position at which it invoked *e*1, but then calls *e*2 instead, returning *e*2's result.



这里解析了 `3` 是成功的，之后期望一个 `one_ws` 然后再次解析 `(int / float)`，导致没有回溯。所以我们需要将 `float` 放到前面



**2）为什么需要引入 `one_ws`**



第一行最开始写的时候是

```
root    <- skip (float / int)* eof
```



因为需要支持 `.2` 这种浮点数。所以 `float`的规则前面是 `dec_int?`

```
float_  <- ((dec_int? '.' dec_int) / (dec_int '.')) ([eE] [-+]? dec_int)?
         / dec_int [eE] [-+]? dec_int
```



比如 `3.1.4` ，我们会先解析到 `3.1` 这个浮点数，然后右解析到 `.4` 这个浮点数。所以这里必须要强制插入一个空白符，引入了一个 `one_ws`

```
root    <- skip (float / int)? (one_ws (float / int))* eof
```





测试用例代码如下

```c
// main.c

#include <stdio.h>
#include <string.h>

FILE* input = NULL;
static int lineno = 0;
static char* filename = NULL;

#define YY_CTX_LOCAL

#define YY_INPUT(ctx, buf, result, max)            \
    {                                              \
        int c = getc(input);                       \
        if (c == '\n')                             \
            ++lineno;                              \
        result = (EOF == c) ? 0 : (*(buf) = c, 1); \
    }

#include "parser.c"  // include 自动生成的 peg 代码

void yyerror(yycontext* ctx, char* message)
{
    fprintf(stderr, "%s:%d: %s", filename, lineno, message);

    if (ctx->__pos < ctx->__limit || !feof(input)) {
        int start;
        int pos = ctx->__limit;
        while (ctx->__pos < pos) {
            if (ctx->__buf[pos] == '\n') {
                ++pos;
                break;
            }
            --pos;
        }

        start = pos;
        ctx->__buf[ctx->__limit] = '\0';
        while (pos < ctx->__limit) {
            if (ctx->__buf[pos] == '\n')
                break;
            fputc(ctx->__buf[pos], stderr);
            pos += 1;
        }
        if (pos == ctx->__limit) {
            int c = fgetc(input);
            while (c != EOF && c != '\n') {
                fputc(c, stderr);
                c = fgetc(input);
            }
        }
        fprintf(stderr, "\n%*c", ctx->__limit - start, '^');
    }

    fprintf(stderr, "\n");
}

int test_literal(const char* literal)
{
    input = fmemopen((void*)literal, strlen(literal), "r");
    lineno = 1;
    filename = "<stdin>";

    yycontext ctx;
    memset(&ctx, 0, sizeof(yycontext));

    if (yyparse(&ctx) == 0) {
        yyerror(&ctx, "syntax error\n");
        return 0;
    }

    return 1;
}

const char* valid_literals[] = {
    "",
    " ",
    "\n \n",
    "42",
    "100",
    "987654321",
    "3.14",
    "0.5",
    ".25",
    "3.",
    "-0.001",
    "1.0",
    "1e3",
    "2.5e-4",
    "3.14e+2",
    "0b101010",
    "0b1101",
    "0o52",
    "0o13",
    "0x2A",
    "0x1F",
    "1_000_000",
    "3.141_592",
    "0b1010_1101",
    "-123",
    "-3.14",
    "-0b1010",
    "-0o17",
    "-0x1f"
};

const char* invalid_literals[] = {
    "3.1.2",
    "3..12",
    "0x",
    "1.23.45",
    "e5",
    ".e5",
    ".5.2",
    "0b12g",
    "1.2e",
    "1.2e3e2",
    "1e3.2",
    "-3.14.56"
};

int main()
{

    printf("Testing valid literals:\n");
    for (int i = 0; i < sizeof(valid_literals) / sizeof(valid_literals[0]); i++) {
        printf("Testing: %s\n", valid_literals[i]);
        if (!test_literal(valid_literals[i])) {
            printf("[❌]Failed to parse valid literal: %s\n", valid_literals[i]);
            goto failed;
        } else {
            printf("[✅]Successfully parsed: %s\n", valid_literals[i]);
        }
    }

    printf("\nTesting invalid literals:\n");
    for (int i = 0; i < sizeof(invalid_literals) / sizeof(invalid_literals[0]); i++) {
        printf("Testing: %s\n", invalid_literals[i]);
        if (!test_literal(invalid_literals[i])) {
            printf("[✅]Correctly identified invalid literal: %s\n", invalid_literals[i]);
        } else {
            printf("[❌]Incorrectly parsed invalid literal: %s\n", invalid_literals[i]);
            goto failed;
        }
    }

    return 0;
failed:
    return 1;
}

```



命令行执行

```bash
peg grammar.peg > parser.c && gcc main.c && ./a.out
```





## leg



leg 工具可以方便的在 C 中嵌入 peg 实现一个 parser。语法和 peg 又不太一样，比如 `<-` 是要修改成 `=`。实际用下来感觉非常困难尤其在调试的时候



1. 调试信息不友好，直接是 `parser.leg:1: syntax error before text "%{"` 
2. `-` 这个符号很特殊，报错的时候会变成 `rule '_' used but not defined`



上面的 peg 可以使用 leg 实现，有些情况没有处理好

```
%{

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <ctype.h>
#include <stdbool.h>

void remove_underscores(char *str) {
    char *p = str;
    while (*str) {
        if (*str != '_') {
            *p++ = *str;
        }
        str++;
    }
    *p = '\0';
}

static bool state_has_exp = false;
static int state_sign = -1;

%}

root    = (float | int) eol @{ state_has_exp = false; state_sign = 1; }
        | (!eol . )* eol { printf("failed to parse\n"); }

int     = sign? int_
int_    = "0b" i:bin_int { printf("=> 0b%b\n", i * state_sign); }
        | "0o" i:oct_int { printf("=> 0o%o\n", i * state_sign); }
        | "0x" i:hex_int { printf("=> 0x%x\n", i * state_sign); }
        | i:dec_int      { printf("=> %d\n", i * state_sign); }

dec_int = <dec dec_*> { remove_underscores(yytext); $$ = strtol(yytext, NULL, 10); }
dec_    = '_'? dec
dec     = [0-9]

bin_int = <(bin bin_*)> { remove_underscores(yytext); $$ = strtol(yytext, NULL, 2); }
bin_    = '_'? bin
bin     = [01]

oct_int = <oct oct_*> { remove_underscores(yytext); $$ = strtol(yytext, NULL, 8); }
oct_    = '_'? oct
oct     = [0-7]

hex_int = <hex hex_*> { remove_underscores(yytext); $$ = strtol(yytext, NULL, 16); }
hex_    = '_'? hex
hex     = [0-9a-fA-F]

float   = sign? float_
float_  = (int_part:dec_int? dot frac_part:dec_int (exp_sign s:sign? e:dec_int)?) { if (state_has_exp) { printf("=> %d.%d * 10^%d\n", int_part, frac_part, e); } else { printf("=> %d.%d\n", int_part, frac_part); } }
        | (int_part:dec_int exp_sign s:sign? e:dec_int) {  printf("=> %d * 10^%d\n", int_part, e); }

exp_sign = <[eE]> { state_has_exp = true; }

sign    = '+' { state_sign = 1; }
        | '-' { state_sign = -1; }

dot     = '.'
eol     = '\n' | '\r\n' | '\r' | ';'

%%

int main() {
    while (yyparse());
    return 0;
}

```



执行

```bash
leg grammar.leg > main.c && gcc main.c && ./a.out
```





## 小结



体验一下就好。目前做 parser 的工具非常多，而且几乎语法是不相同的，但是核心思路差不多。leg / peg 这种工具很老了，不建议使用
