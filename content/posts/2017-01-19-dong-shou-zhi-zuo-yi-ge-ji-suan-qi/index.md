
+++
title = "动手制作一个计算器"
summary = ''
description = ""
categories = []
tags = []
date = 2017-01-19T13:14:46+08:00
draft = false
+++

说到制作一个计算器，我首先想到的是： 对字符进行安全检查然后直接`eval()`
好吧，这种做法是可行的，但是有点作弊了。使用`eval()`，我们忽略了许多细节问题(况且有些语言没有`eval()`，拍桌)

下面来从一个深层次的角度来分析这个问题
一个计算器需要接收一个算术表达式式然后输出结果，我们假定换行符 `\n` 作为算式的终结符。然后我们需要从这个算式中去区分出数字和运算符。数字包括整数和小数，所以我们需要对小数点进行处理。运算符之间还存在着优先级关系。

所以为了提取这些有意义的的字符序列，我们对输入字符流进行扫描，进行模式匹配。专业一点，我们将字符流分割成词法单元(token)，这整个的步骤称为词法分析。
词法单元通常由类型和值组成，

	<token_kind, token_value>

可以用下面的结构体来表示

	typedef struct {
	    TokenKind kind;
	    double value;
	} Token;

对一个计算器而言，`token` 的种类应具有数字，运算符，括号，和终结符这几种。至于负号，我们复用了减号运算符

	typedef enum {
	    UNKNOWN_TOKEN,
	    NUMBER_TOKEN,
	    ADD_OPERATOR_TOKEN,
	    SUB_OPERATOR_TOKEN,
	    MUL_OPERATOR_TOKEN,
	    DIV_OPERATOR_TOKEN,
	    LEFT_PAREN_TOKEN,
	    RIGHT_PAREN_TOKEN,
	    END_TOKEN
	} TokenKind;

比如 `3+2` 这个算式，我们返回

	<NUMBER_TOKEN, 3>
	<ADD_OPERATOR, NULL>
	<NUMBER_TOKEN, 2>
	<END_TOKEN, NULL>


有了以上的准备后，我们便可以对字符流进行解析了，定义一个 `next()`函数来返回 `token`
对于运算符 `+ - *  /` 和 `()` 我们直接返回其 `token`，对于数字的匹配稍微有点麻烦，因为它是由多个字符组成的。这里采用设置状态标志的方法解决。

	typedef enum {
	    INITIAL_STATUS,
	    IN_INT_PART_STATUS,
	    DOT_STATUS,
	    IN_FRAC_PART_STATUS
	} LexerStatus;

初始的状态为 `INITIAL_STATUS`。当遇到 `[0-9]`时，我们便将状态更改为 `IN_INT_PART_STATUS` ，当遇到小数点时，更改为 `DOT_STATUS` ，再遇到`[0-9]`时更改为 `IN_FRAC_PART_STATUS`。在 `IN_INT_PART_STATUS` 或 `IN_FRAC_PART_STATUS` 状态下，如果再无数字或小数点出现，则返回。
*一时语塞，表达不太严谨，看代码好了:)*

	void next()
	{
	    // position of token_value
	    int pos = 0;
	    LexerStatus status = INITIAL_STATUS;
	    char current_char;

	    // default token kind
	    token.kind = UNKNOWN_TOKEN;
	    while (*buffer != '\0') {
	        current_char = *buffer;

	        // match number end
	        if ((status == IN_INT_PART_STATUS || status == IN_FRAC_PART_STATUS)
	            && !isdigit(current_char) && current_char != '.') {
	            token.kind = NUMBER_TOKEN;
	            sscanf(token_value, "%lf", &token.value);
	            return;
	        }

	        // skip
	        if (isspace(current_char)) {
	            if (current_char == '\n') {
	                token.kind = END_TOKEN;
	                return;
	            }
	            buffer++;
	            continue;
	        }

	        token_value[pos++] = *buffer++;
	        token_value[pos] = '\0';

	        if (current_char == '+') {
	            token.kind = ADD_OPERATOR_TOKEN;
	            return;
	        } else if (current_char == '-') {
	            token.kind = SUB_OPERATOR_TOKEN;
	            return;
	        } else if (current_char == '*') {
	            token.kind = MUL_OPERATOR_TOKEN;
	            return;
	        } else if (current_char == '/') {
	            token.kind = DIV_OPERATOR_TOKEN;
	            return;
	        } else if (current_char == '(') {
	            token.kind = LEFT_PAREN_TOKEN;
	            return;
	        } else if (current_char == ')') {
	            token.kind = RIGHT_PAREN_TOKEN;
	            return;
	        } else if (isdigit(current_char)) {
	            if (status == INITIAL_STATUS) {
	                status = IN_INT_PART_STATUS;
	            } else if (status == DOT_STATUS) {
	                status = IN_FRAC_PART_STATUS;
	            }
	        } else if (current_char == '.') {
	            if (status == IN_INT_PART_STATUS) {
	                status = DOT_STATUS;
	            } else {
	                fprintf(stderr, "syntax error\n");
	                exit(1);
	            }
	        }
	    }
	}


剩下的我们便要进行语法分析了
先考虑一下算术表达式的运算符结合性和优先级

	左结合: + -
	左结合: * /

结合性便不用考虑了，因为都相同。优先级的话，我们创建两个非终结符号 `expression` 和 `term` 来对应这两个优先级层次，并使用另一个非终结符号 `factor` 来表示表达式中的基本单元: 数字和带括号的表达式
构建好的[BNF范式](http://foldoc.org/Backus-Naur%20Form)如下

	expression
	    : term
	    | expression ADD term
	    | expression SUB term
	    ;

	term
	    : factor
	    | term MUL factor
	    | term DIV factor
	    ;

	factor
	    : NUM
	    | LP expression RP
	    | MINUS factor
	    ;

但是如果采用这样的语法规则，我们会陷入左递归的陷阱中

	expression
	    | expression ADD term

比如上面，我们不断的向下分析 expression ，会进入无限循环

所以我们要采用一点小技巧对现有规则进行转换
形如

	A
	    | Aα
	    | Aβ
	    | γ
	    ;

的规则可转换为

	A
	    | γR
	    ;

	R
	    | αR
	    | βR
	    | ε
	    ;

套用上面的技巧 ，令

	A = expression
	α = + term
	β = - term
	γ = term

转换后可得

	expression
	    | term rest

	rest
	    | + term
	    | - term
	    | ε

剩下规则的转换便不详细写了

依照上面的语法规则可写出以下代码

	double parse_factor()
	{

	    double value = 0.0;
	    int minus_flag = 0;

	    next();

	    // if token is minus, get next token
	    if (token.kind == SUB_OPERATOR_TOKEN) {
	        minus_flag = 1;
	        next();
	    }

	    if (token.kind == NUMBER_TOKEN) {
	        value = token.value;
	    } else if (token.kind == LEFT_PAREN_TOKEN) {
	        // ( expr )
	        value = parse_expression();
	        if (token.kind != RIGHT_PAREN_TOKEN) {
	            fprintf(stderr, "missing ')'\n");
	            exit(1);
	        }
	    }

	    if (minus_flag) {
	        value = -value;
	    }

	    next();

	    return value;
	}

	double parse_term()
	{
	    double v1;
	    double v2;
	    int operator;

	    v1 = parse_factor();
	    for (;;) {

	        if (token.kind != MUL_OPERATOR_TOKEN
	            && token.kind != DIV_OPERATOR_TOKEN) {
	            break;
	        }
	        operator = token.kind;
	        v2 = parse_factor();
	        if (operator == MUL_OPERATOR_TOKEN) {
	            v1 *= v2;
	        } else if (operator == DIV_OPERATOR_TOKEN) {
	            v1 /= v2;
	        }
	    }
	    return v1;
	}

	double parse_expression()
	{
	    double v1;
	    double v2;
	    int operator;

	    v1 = parse_term();
	    for (;;) {

	        if (token.kind != ADD_OPERATOR_TOKEN
	            && token.kind != SUB_OPERATOR_TOKEN) {
	            break;
	        }
	        operator = token.kind;
	        v2 = parse_term();
	        if (operator == ADD_OPERATOR_TOKEN) {
	            v1 += v2;
	        } else if (operator == SUB_OPERATOR_TOKEN) {
	            v1 -= v2;
	        }
	    }
	    return v1;
	}

好了，一个计算器便完成了
完整代码见 [GitHub](https://github.com/Hanaasagi/calculator)
#### Reference
[BNF 范式](https://zh.wikipedia.org/wiki/%E5%B7%B4%E7%A7%91%E6%96%AF%E8%8C%83%E5%BC%8F)
[左递归](https://zh.wikipedia.org/wiki/%E5%B7%A6%E9%81%9E%E6%AD%B8)
编译原理
编程语言实现模式

