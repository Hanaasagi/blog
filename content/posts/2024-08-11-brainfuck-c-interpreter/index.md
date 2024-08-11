+++
title = "实现 Brainfuck 解释器"
summary = ""
description = ""
categories = [""]
tags = []
date = 2024-08-11T03:20:53+09:00
draft = false

+++



## 语法

语法规则参考 [Wiki - Brainfuck](https://en.wikipedia.org/wiki/Brainfuck?useskin=vector)

| Character | Meaning                                                      |
| :-------: | ------------------------------------------------------------ |
|    `>`    | Increment the [data pointer](https://en.wikipedia.org/wiki/Pointer_(computer_programming)) by one (to point to the next cell to the right). |
|    `<`    | Decrement the data pointer by one (to point to the next cell to the left). |
|    `+`    | Increment the byte at the data pointer by one.               |
|    `-`    | Decrement the byte at the data pointer by one.               |
|    `.`    | Output the byte at the data pointer.                         |
|    `,`    | Accept one byte of input, storing its value in the byte at the data pointer. |
|    `[`    | If the byte at the data pointer is zero, then instead of moving the [instruction pointer](https://en.wikipedia.org/wiki/Program_Counter) forward to the next command, [jump](https://en.wikipedia.org/wiki/Branch_(computer_science)) it *forward* to the command after the *matching* `]` command. |
|    `]`    | If the byte at the data pointer is nonzero, then instead of moving the instruction pointer forward to the next command, jump it *back* to the command after the *matching* `[` command |



对于实现这门语言我们需要准备一块初始化为 0 的内存区域 `data`。通过一个变量 `ptr` 来记录当前 `data` 的指向，读取到 `>` 或者  `<` 的字符的时候我们就右移/左移这个指针。读取到 `+` 和 `-` 的时候就将 `data[ptr]` 的值进行加一/减一。读取到 `.` 和 `,` 的时候需要 IO 操作，这里需要借助 syscall。读取到 `[` 和 `]` 则需要对于代码段进行跳转，这里也需要一个变量来记录当前的代码段的偏移量。每次执行的时候读取当前偏移量的指令，如果遇到 `[` 那么可以通过修改变量进行跳转



因为这样的语法规则，导致 Brainfuck 的代码十分晦涩。比如所有的常量都需要通过不断的 `+` 来进行初始化；没有直接的加法，需要通过循环来实现；复制一个数也需要通过循环，但是循环本身带来的副作用又会让一个变量归零



比如我们计算一个 `fib(10)`，可以这么写

```
++++++++++
>
>+
<<
[
	-
	>>
	[
		-
		>+
		>+
		<<
	]
	<
	[
		-
		>>>+
		<<<
	]
	>>
	[
		-
		<<+
		>>
	]
	>
	[
		-
		<<+
		>>
	]
	<<<<
]
```



数据区域需要依次存储循环次数，`fib(0)`, `fib(1)`还有两个临时变量。在高级语言里面我们只需要一个临时变量，但是因为在 Brainfuck 中我们循环必须递减一个变量的值。所以需要在循环中对于一个额外的变量进行自增来还原。即 `[loop_count, fib(n), fib(n+1), tmp, fib(n+2)]`



以 `n=3` 为例

```
[6, 2, 3, 0, 0]
```



然后我们将 `fib(n+1)` 的值通过循环复制到 `tmp` 和 `fib(n+2)` 的位置上，变成

```
[6, 2, 0, 3, 3]
```



然后递减 `fib(n)` 的值，对于 `fib(n+2)` 进行加一



```
[6, 0, 0, 3, 5]
```



然后通过循环将 `tmp` 移动到 `fib(n)` 的位置

```
[6, 3, 0, 0, 5]
```



然后通过循环将 `fib(n+2)` 移动到 `fib(n+1)` 的位置

```
[6, 3, 5, 0, 0]
```



当然这个代码是可以简化的。比如最后的两个移动，5 一定大于 3 的，所以可以在移动 `3` 的时候同时对于 `fib(n)` 和 `fib(n+1)` 的位置进行加一，`fib(n+2)` 的位置进行减一。这样得出的是

```
[6, 3, 3, 0, 2]
```

然后将 2 加到 3 上即可





再来看一个 hello world 的示例

```
[ This program prints "Hello World!" and a newline to the screen; its
  length is 106 active command characters. [It is not the shortest.]

  This loop is an "initial comment loop", a simple way of adding a comment
  to a BF program such that you don't have to worry about any command
  characters. Any ".", ",", "+", "-", "<" and ">" characters are simply
  ignored, the "[" and "]" characters just have to be balanced. This
  loop and the commands it contains are ignored because the current cell
  defaults to a value of 0; the 0 value causes this loop to be skipped.
]
++++++++                Set Cell #0 to 8
[
    >++++               Add 4 to Cell #1; this will always set Cell #1 to 4
    [                   as the cell will be cleared by the loop
        >++             Add 2 to Cell #2
        >+++            Add 3 to Cell #3
        >+++            Add 3 to Cell #4
        >+              Add 1 to Cell #5
        <<<<-           Decrement the loop counter in Cell #1
    ]                   Loop until Cell #1 is zero; number of iterations is 4
    >+                  Add 1 to Cell #2
    >+                  Add 1 to Cell #3
    >-                  Subtract 1 from Cell #4
    >>+                 Add 1 to Cell #6
    [<]                 Move back to the first zero cell you find; this will
                        be Cell #1 which was cleared by the previous loop
    <-                  Decrement the loop Counter in Cell #0
]                       Loop until Cell #0 is zero; number of iterations is 8

The result of this is:
Cell no :   0   1   2   3   4   5   6
Contents:   0   0  72 104  88  32   8
Pointer :   ^

>>.                     Cell #2 has value 72 which is 'H'
>---.                   Subtract 3 from Cell #3 to get 101 which is 'e'
+++++++..+++.           Likewise for 'llo' from Cell #3
>>.                     Cell #5 is 32 for the space
<-.                     Subtract 1 from Cell #4 for 87 to give a 'W'
<.                      Cell #3 was set to 'o' from the end of 'Hello'
+++.------.--------.    Cell #3 for 'rl' and 'd'
>>+.                    Add 1 to Cell #5 gives us an exclamation point
>++.                    And finally a newline from Cell #6
```



## 解释器实现



首先定一个我们自己的数据结构

```c
typedef enum {
    INCREMENT_PTR,
    DECREMENT_PTR,
    INCREMENT_VAL,
    DECREMENT_VAL,
    OUTPUT_VAL,
    INPUT_VAL,
    LOOP_BEGIN,
    LOOP_END
} OpcodeType;

typedef struct Opcode {
    OpcodeType type;
    size_t operand;
} Opcode __attribute__((aligned(8)));

```



逐个字符解析原始的 `.bf` 文件，连续的 `>>>` 可以合并成一个。对于循环的解析可以通过使用一个栈来记录位置，遇到 `[` 则记录一下偏移量，遇到 `]` 则从栈中 pop 出对应的 `[` ，这样就是成对的 `[]` 了。栈的大小会限制循环嵌套的最大层数

```c
int compiler_parse_file(compiler_t* compiler, FILE* fp)
{
    size_t stack[100];
    int stack_ptr = -1;

    char c;
    while ((c = fgetc(fp)) != EOF) {
        Opcode op;
        op.operand = 0;

        switch (c) {
        case '>':
            do {
                op.operand++;
            } while ((c = fgetc(fp)) == '>');
            ungetc(c, fp);
            op.type = INCREMENT_PTR;
            break;
        case '<':
            do {
                op.operand++;
            } while ((c = fgetc(fp)) == '<');
            ungetc(c, fp);
            op.type = DECREMENT_PTR;
            break;
        case '+':
            do {
                op.operand++;
            } while ((c = fgetc(fp)) == '+');
            ungetc(c, fp);
            op.type = INCREMENT_VAL;
            break;
        case '-':
            do {
                op.operand++;
            } while ((c = fgetc(fp)) == '-');
            ungetc(c, fp);
            op.type = DECREMENT_VAL;
            break;
        case '.':
            op.operand = 1;
            op.type = OUTPUT_VAL;
            break;
        case ',':
            op.operand = 1;
            op.type = INPUT_VAL;
            break;
        case '[':
            op.type = LOOP_BEGIN;
            stack[++stack_ptr] = compiler->opcodes.len;
            break;
        case ']':
            if (stack_ptr < 0) {
                perror("Unmatched closing bracket\n");
                return EPARSE_ERROR;
            }
            op.type = LOOP_END;
            ((Opcode*)vec_get(&compiler->opcodes, stack[stack_ptr]))->operand = compiler->opcodes.len;
            op.operand = stack[stack_ptr--];
            break;
        default:
            continue;
        }
        vec_push(&compiler->opcodes, &op);
    }

    if (stack_ptr >= 0) {
        perror("Unmatched opening bracket\n");
        return EPARSE_ERROR;
    }
    return 0;
}

```



解析之后的大概是这样的格式

```
0    INCREMENT_VAL   8
1    LOOP_BEGIN      29
2    INCREMENT_PTR   1
3    INCREMENT_VAL   4
4    LOOP_BEGIN      15
5    INCREMENT_PTR   1
6    INCREMENT_VAL   2
7    INCREMENT_PTR   1
8    INCREMENT_VAL   3
9    INCREMENT_PTR   1
10   INCREMENT_VAL   3
11   INCREMENT_PTR   1
12   INCREMENT_VAL   1
13   DECREMENT_PTR   4
14   DECREMENT_VAL   1
15   LOOP_END        4
```





解释器需要维护两个状态

- `pc` 指令的偏移量，决定下一条执行的代码
- `sp` 栈顶指针。严格来说 Brainfuck 的这个不是一个栈，而是纯粹的线性内存空间



然后一步步执行即可

```c
typedef struct interpreter {
    vec_t* opcodes;
    size_t pc;
    char bp[512];
    char* sp;
} interpreter_t __attribute__((aligned(8)));

static inline Opcode* next_op(interpreter_t* interpreter)
{
    if (interpreter->pc >= interpreter->opcodes->len) {
        return NULL;
    }

    Opcode* op = vec_get(interpreter->opcodes, interpreter->pc);
    return op;
}

int interpreter_run(interpreter_t* interpreter)
{
    interpreter_show_opcodes(interpreter);

    Opcode* op;
    while ((op = next_op(interpreter)) != NULL) {
        interpreter_show_state(interpreter);
        switch (op->type) {
        case INCREMENT_PTR:
            interpreter->sp += op->operand;
            if (interpreter->sp - interpreter->bp >= RUNTIME_STACK_SIZE) {
                perror("stack overflow\n");
                return ESTACK_OVERFLOW;
            }
            break;
        case DECREMENT_PTR:
            interpreter->sp -= op->operand;
            if (interpreter->sp < interpreter->bp) {
                perror("stack overflow\n");
                return ESTACK_OVERFLOW;
            }
            break;
        case INCREMENT_VAL:
            *interpreter->sp += op->operand;
            break;
        case DECREMENT_VAL:
            *interpreter->sp -= op->operand;
            break;
        case OUTPUT_VAL:
            for (size_t i = 0; i < op->operand; ++i) {
                putchar(*interpreter->sp);
            }
            break;
        case INPUT_VAL:
            for (size_t i = 0; i < op->operand; ++i) {
                *interpreter->sp = getchar();
            }
            break;
        case LOOP_BEGIN:
            if (*interpreter->sp == 0) {
                assert(op->operand >= 0 && op->operand < interpreter->opcodes->len);
                interpreter->pc = op->operand;
            }
            break;
        case LOOP_END:
            if (*interpreter->sp != 0) {
                assert(op->operand >= 0 && op->operand < interpreter->opcodes->len);
                interpreter->pc = op->operand;
            }
            break;
        }
        interpreter->pc += 1;
    }
    return 0;
}

```



完整的[实现代码](https://github.com/Hanaasagi/kaleido/tree/b9623586b73eecc85ad0be2b9a70c9fa9d5e8ccb/brainfuck) 
