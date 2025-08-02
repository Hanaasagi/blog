+++
title = "shlex 实现笔记"
summary = ""
description = ""
categories = []
tags =  ["zig"]
date = 2025-08-02T11:30:00+09:00
draft = false

+++



上周使用 Zig 实现了 Python std 里面的 shelx。简单记录一下 shlex 的状态机实现

- 源代码: https://github.com/python/cpython/blob/d7e12a362a2a4a4b7d52a343ab5940be2cbcc909/Lib/shlex.py
- 测试: https://github.com/python/cpython/blob/d7e12a362a2a4a4b7d52a343ab5940be2cbcc909/Lib/test/test_shlex.py
- Zig 版本实现: https://github.com/dying-will-bullet/shlex



`quote` 和 `join` 两个函数并没有使用到状态机。这里主要围绕 `split` 函数进行分析



## 字符集处理



shlex 使用了一些字符集合作为状态跳转时候的判断依据。这里可以借助 Zig 的 compile time 在编译期内使用 bitset 来优化查找。虽然都是 O(1) ，但这比 runtime 时构建的 HashMap 效率更高。对于 shlex 的字符集来说，需要使用 4 个 usize 的大小来构建字符集。



```Zig
const StaticCharSet = struct {
    bitset: std.bit_set.StaticBitSet(256) = std.bit_set.StaticBitSet(256).initEmpty(),

    const Self = @This();

    fn init(comptime chars: []const u8) Self {
        var set = Self{};
        inline for (chars) |char| {
            set.add(char);
        }
        return set;
    }

    fn add(self: *Self, comptime char: u8) void {
        self.bitset.set(char);
    }

    fn contains(self: Self, char: u8) bool {
        return self.bitset.isSet(char);
    }
    
	// ...

    fn subtract(self: *Self, comptime other: Self) void {
        var tmp = other.bitset;
        tmp.toggleAll();
        self.bitset.setIntersection(tmp);
    }
};

```



字符集定义如下

```zig
const CharSets = struct {
    wordchars: StaticCharSet,        // 词汇字符: a-z, A-Z, 0-9, _
    punctuation_chars: StaticCharSet, // 标点符号: ();<>|&
    whitespace: StaticCharSet,       // 空白字符: space, tab, \r, \n
    commenters: StaticCharSet,       // 注释字符: #
    quotes: StaticCharSet,           // 引号字符: ', "
    escape: StaticCharSet,           // 转义字符: \
    escapedquotes: StaticCharSet,    // 可被转义的引号: "
};
```





## 状态机设计



### 状态定义

shlex 使用一个有限状态机来处理字符串分词。状态机分为以下 7 种状态

```zig
const TokenState = enum(u8) {
    /// 等待有效输入，跳过空白/注释
    initial,
    /// 构建常规词汇 token，处理引号嵌入
    word,
    /// 聚合连续标点符号为单一 token
    punctuation,
    /// 单引号内容，POSIX 下支持有限转义
    single_quoted,
    /// 双引号内容，完整转义支持
    double_quoted,
    /// 转义序列处理，需维护前置状态
    escape,
    /// 输入结束，清理并完成解析
    eof,
};
```



我们需要借助 `pushback` buffer 来缓冲一些已经被解析过但是不能理解弹出的 `token`



```zig
// 字符级 pushback：处理标点符号边界
pushback_chars: ArrayList(u8),

fn pushChar(self: *Self, char: u8) !void {
    try self.pushback_chars.insert(0, char);  // LIFO 队列
}

fn getNextChar(self: *Self) ?u8 {
    if (!char_sets.punctuation_chars.isEmpty() and 
        self.pushback_chars.items.len > 0) {
        return self.popChar();
    }
    return self.readChar();  // 正常读取
}
```



比如下面这种场景

```
输入: "word|punctuation"
过程: word状态遇到'|' → pushback '|' → finish word token → 
      下次调用从pushback取'|' → 进入punctuation状态
```



### 状态转换



状态转换图如下

```
{{< mermaid >}}
stateDiagram-v2
    [*] --> initial
    initial --> word: wordchar
    initial --> punctuation: punctuation
    initial --> single_quoted: '
    initial --> double_quoted: "
    initial --> escape: \ (POSIX)
    initial --> eof: EOF
    
    word --> initial: whitespace
    word --> single_quoted: ' (POSIX)
    word --> double_quoted: " (POSIX)
    word --> escape: \ (POSIX)
    word --> eof: EOF
    
    punctuation --> initial: non-punctuation
    punctuation --> eof: EOF
    
    single_quoted --> word: ' (POSIX)
    single_quoted --> initial: ' (non-POSIX)
    single_quoted --> escape: \ (POSIX, limited)
    
    double_quoted --> word: " (POSIX)
    double_quoted --> initial: " (non-POSIX)
    double_quoted --> escape: \ (POSIX)
    
    escape --> single_quoted: return to quoted
    escape --> double_quoted: return to quoted
    escape --> word: return to word
    
    eof --> [*]
{{< /mermaid >}}
```

*此图由 AI 生成*





#### 初始状态 (initial)

初始状态是状态机的起始状态，主要职责是：

1. 跳过空白字符
2. 处理注释（跳过整行）
3. 根据字符类型转换到相应状态



#### 词汇状态 (word)



在词汇状态下，分析器会持续收集字符直到遇到分隔符：

```zig
fn handleWordState(self: *Self, char: ?u8) !?TokenAction {
    if (char == null) {
        self.lexer_state.state = .eof;
        return .finish_token;
    }

    const c = char.?;

    // 遇到空白字符，结束当前词汇
    if (char_sets.whitespace.contains(c)) {
        self.lexer_state.state = .initial;
        return .finish_token;
    }

    // POSIX 模式下可以在词汇中嵌入引号
    if (options.posix and c == '\'') {
        self.lexer_state.state = .single_quoted;
        return .continue_parsing;
    }

    // 继续添加字符到当前词汇
    if (char_sets.wordchars.contains(c)) {
        try self.token_buffer.append(c);
        return .continue_parsing;
    }

    // 遇到标点符号，需要回退处理
    if (!char_sets.punctuation_chars.isEmpty()) {
        try self.pushChar(c);  // 回退字符
    }

    self.lexer_state.state = .initial;
    return .finish_token;
}
```



#### 引号状态 (single_quoted / double_quoted)



在 POSIX 模式和非 POSIX 模式下有不同的行为：

```zig
fn handleQuotedState(self: *Self, char: ?u8, quote_char: u8) !?TokenAction {
    if (char == null) {
        return ShlexError.NoClosingQuotation;  // 未闭合的引号
    }

    const c = char.?;

    // 遇到闭合引号
    if (c == quote_char) {
        if (!options.posix) {
            // 非 POSIX 模式：引号本身也是 token 的一部分
            try self.token_buffer.append(c);
            self.lexer_state.state = .initial;
            return .finish_token;
        } else {
            // POSIX 模式：引号不是 token 的一部分
            self.lexer_state.state = .word;
            return .continue_parsing;
        }
    }

    // 双引号内可以处理转义字符
    if (options.posix and char_sets.escape.contains(c) and quote_char == '"') {
        self.lexer_state.escaped_state = self.lexer_state.state;
        self.lexer_state.state = .escape;
        return .continue_parsing;
    }

    // 普通字符直接添加
    try self.token_buffer.append(c);
    return .continue_parsing;
}
```



#### 转义状态 (escape)



当在其他状态遇到转义字符（默认为 `\`）时，会暂时进入转义状态处理下一个字符。

```zig
fn handleEscapeState(self: *Self, char: ?u8) !?TokenAction {
    if (char == null) {
        return ShlexError.NoEscapedCharacter;  // 转义后没有字符
    }

    const c = char.?;

    // POSIX shell中，只有引号本身或转义字符可以在引号内被转义
    switch (self.lexer_state.escaped_state) {
        .single_quoted => {
            if (c != '\\' and c != '\'') {
                try self.token_buffer.append('\\');  // 保留反斜杠
            }
        },
        .double_quoted => {
            if (c != '\\' and c != '"') {
                try self.token_buffer.append('\\');  // 保留反斜杠
            }
        },
        else => {},  // 其他状态下不特殊处理
    }

    try self.token_buffer.append(c);  // 添加被转义的字符
    self.lexer_state.state = self.lexer_state.escaped_state;  // 恢复前置状态
    return .continue_parsing;
}
```



关键特征

1. **前置状态保存**: 进入转义状态时，通过 `escaped_state` 字段保存前一个状态，处理完成后会恢复到该状态。
2. **POSIX兼容的转义规则**: 
   - 在单引号内，只有 `\'` 和 `\\` 可以被转义，其他字符前的反斜杠会被保留
   - 在双引号内，只有 `\"` 和 `\\` 可以被转义，其他字符前的反斜杠会被保留
   - 在词汇状态下，所有字符都可以被转义
4. **状态恢复**: 处理完转义字符后，状态机会自动恢复到 `escaped_state` 中保存的前置状态，并将 `escaped_state` 重置为 `initial`。







## 完整解析示例



在 `comments=true`, `posix = true`, `punctuation_chars = .enabled` 的条件下。假设输入字符串为 

```
echo -e "hello \"world\"" && #comment
docker ps
```



状态机的转换情况如下



| 步骤  | 输入字符            | 状态 (state/escaped_state)  | 说明                                                     | `token_buffer` | `pushback_chars` | 输出 Token              |
| :---- | :------------------ | :-------------------------- | :----------------------------------------------------------- | :------------- | :--------------- | :---------------------- |
| 1     | `e`                 | `initial` / `initial`       | **词汇字符**: 新 token，状态转为 `word`              | `e`            | `[]`             |                         |
| 2-4   | `c`,`h`,`o`         | `word` / `initial`          | **词汇字符**: 持续追加到 `token_buffer`                    | `echo`         | `[]`             |                         |
| 5     | ` `                 | `word` / `initial`          | **空白字符**: token 结束。生成 `echo` token，状态返回 `initial` | `[]`           | `[]`             | **"echo"**              |
| 6     | `-`                 | `initial` / `initial`       | **非特殊字符**: `whitespace_split` 模式下新 token，状态转为 `word` | `-`            | `[]`             |                         |
| 7     | `e`                 | `word` / `initial`          | **词汇字符**: 追加到 `token_buffer`                        | `-e`           | `[]`             |                         |
| 8     | ` `                 | `word` / `initial`          | **空白字符**: token 结束。生成 `-e` token，状态返回 `initial` | `[]`           | `[]`             | **"-e"**                |
| 9     | `"`                 | `initial` / `initial`       | **双引号**: 状态转为 `double_quoted`。POSIX 模式下，引号本身不进入缓冲区 | `[]`           | `[]`             |                         |
| 10-14 | `h`,`e`,`l`,`l`,`o` | `double_quoted` / `initial` | **普通字符**: 在引号状态下，直接追加到缓冲区          | `hello`        | `[]`             |                         |
| 15    | ` `                 | `double_quoted` / `initial` | **普通字符**: 空格在双引号内也是普通字符，追加             | `hello `       | `[]`             |                         |
| 16    | `\`                 | `double_quoted` / `initial` | **转义字符**: 状态临时转为 `escape`，记录前置状态为 `double_quoted` | `hello `       | `[]`             |                         |
| 17    | `"`                 | `escape` / `double_quoted`  | **被转义的引号**: 将 `"` 追加到缓冲区，状态恢复为 `double_quoted`，重置 `escaped_state` | `hello "`      | `[]`             |                         |
| 18-22 | `w`,`o`,`r`,`l`,`d` | `double_quoted` / `initial` | **普通字符**: 继续追加                                    | `hello "world` | `[]`             |                         |
| 23    | `\`                 | `double_quoted` / `initial` | **转义字符**: 状态临时转为 `escape`，记录前置状态为 `double_quoted` | `hello "world` | `[]`             |                         |
| 24    | `"`                 | `escape` / `double_quoted`  | **被转义的引号**: 将 `"` 追加到缓冲区，状态恢复为 `double_quoted`，重置 `escaped_state` | `hello "world"` | `[]`             |                         |
| 25    | `"`                 | `double_quoted` / `initial` | **结束双引号**: 引号状态结束。POSIX 模式下，状态转为 `word`，允许后续字符拼接 | `hello "world"` | `[]`             |                         |
| 26    | ` `                 | `word` / `initial`          | **空白字符**: token 结束。生成 token，状态返回 `initial` | `[]`           | `[]`             | **"hello \\"world\\""** |
| 27    | `&`                 | `initial` / `initial`       | **非特殊字符**: `whitespace_split` 模式下新 token，状态转为 `word` | `&`            | `[]`             |                         |
| 28    | `&`                 | `word` / `initial`          | **非特殊字符**: 在 `word` 状态下继续追加到 `token_buffer`  | `&&`           | `[]`             |                         |
| 29    | ` `                 | `word` / `initial`          | **空白字符**: token 结束。生成 `&&` token，状态返回 `initial` | `[]`           | `[]`             | **"&&"**                |
| 30    | `#`                 | `initial` / `initial`       | **注释符**: `handleInitialState` 触发 `readLine`，跳过所有字符直到换行符 `\n` | `[]`           | `[]`             |                         |
| 31    | `\n`                | `initial` / `initial`       | **空白字符**: `readLine` 停止后，主循环读取到换行符，视为空白，直接忽略 | `[]`           | `[]`             |                         |
| 32    | `d`                 | `initial` / `initial`       | **词汇字符**: 新 token，状态转为 `word`              | `d`            | `[]`             |                         |
| 33-37 | `o`,`c`,`k`,`e`,`r` | `word` / `initial`          | **词汇字符**: 持续追加到 `token_buffer`                    | `docker`       | `[]`             |                         |
| 38    | ` `                 | `word` / `initial`          | **空白字符**: token 结束。生成 `docker` token，状态返回 `initial` | `[]`           | `[]`             | **"docker"**            |
| 39    | `p`                 | `initial` / `initial`       | **词汇字符**: 新 token，状态转为 `word`               | `p`            | `[]`             |                         |
| 40    | `s`                 | `word` / `initial`          | **词汇字符**: 追加到 `token_buffer`                        | `ps`           | `[]`             |                         |
| 41    | EOF                 | `word` / `initial`          | **输入结束**: 文件末尾。结束最后一个 token                | `[]`           | `[]`             | **"ps"**                |



经过上述流程，`shlex.split()` 会返回以下token数组：

```
["echo", "-e", "hello \"world\"", "&&", "docker", "ps"]
```











