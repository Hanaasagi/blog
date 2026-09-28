+++
title =  "为什么 curl 127.1 会请求 127.0.0.1"
summary = ""
description = ""
categories = [""]
tags = ["network", "curl"]
date = 2026-09-27T09:20:39+09:00
draft = false

+++

## 现象

比如输入了这样的命令

```
$ curl 127.1:80
curl: (7) Failed to connect to 127.0.0.1 port 80 after 0 ms: Couldn't connect to server

$ curl 0:80
curl: (7) Failed to connect to 0.0.0.0 port 80 after 0 ms: Couldn't connect to server
```



注意错误信息中的地址：`127.1` 变成了 `127.0.0.1`，`0` 变成了 `0.0.0.0`。这并不是 DNS 解析的结果，而是 IP 地址本身被展开了



## inet_aton 的多段式地址

如果你在 mac 或 Linux 上执行 `man inet_aton`，会看到这样一段描述

> inet_aton()  converts the Internet host address cp from the IPv4 numbers-and-dots notation into binary form (in network byte order) and stores it in the structure that inp
> points to.  inet_aton() returns nonzero if the address is valid, zero if not.  The address supplied in cp can have one of the following forms:
>
> a.b.c.d   Each of the four numeric parts specifies a byte of the address; the bytes are assigned in left-to-right order to produce the binary address.
>
> a.b.c     Parts a and b specify the first two bytes of the binary address.  Part c is interpreted as a 16-bit value that defines the rightmost two bytes of the binary  ad‐
>           dress.  This notation is suitable for specifying (outmoded) Class B network addresses.
>
> a.b       Part  a  specifies  the  first byte of the binary address.  Part b is interpreted as a 24-bit value that defines the rightmost three bytes of the binary address.
>           This notation is suitable for specifying (outmoded) Class A network addresses.
>
> a         The value a is interpreted as a 32-bit value that is stored directly into the binary address without any byte rearrangement.



IP 地址并不只有 `a.b.c.d` 这一种写法。`inet_aton()` 函数接受四种格式：

| 格式      | 各部分位宽 | 示例                        |
| --------- | ---------- | --------------------------- |
| `a.b.c.d` | 8.8.8.8    | `127.0.0.1`                 |
| `a.b.c`   | 8.8.16     | `127.0.1` -> `127.0.0.1`    |
| `a.b`     | 8.24       | `127.1` -> `127.0.0.1`      |
| `a`       | 32         | `2130706433` -> `127.0.0.1` |

所以 `127.1` 属于 `a.b` 形式：`a = 127` 占高 8 位，`b = 1` 作为 24 位值填充低 3 字节，拼出来就是 `0x7F000001` 即 `127.0.0.1`。而 `0` 属于单数字 `a` 形式：32 位值为 0，即 `0.0.0.0`



我们下面来看一下具体的代码实现



## Apple Libc 中的 inet_aton

macOS 的 `inet_aton` 实现来自 FreeBSD（前身是 4.2BSD），源码位于 Apple 的 Libc 项目中。以下分析基于 commit [`71bbe35`](https://github.com/apple-oss-distributions/Libc/tree/71bbe350ab79eef58113991d817ccc6165061a64)



 `inet_aton` 只是一个 wrapper：

```c
// Libc/net/FreeBSD/inet_addr.c L217-221
int
inet_aton(const char *cp, struct in_addr *addr)
{
	return _inet_aton_check(cp, addr, 0);
}
```

核心逻辑在 `_inet_aton_check` 中

```c
int
_inet_aton_check(const char *cp, struct in_addr *addr, int strict)
{
	u_long val;
	int base, n;
	char c;
	u_int8_t parts[4];
	u_int8_t *pp = parts;
	int digit;

	c = *cp;
	for (;;) {
		/*
		 * Collect number up to ``.''.
		 * Values are specified as for C:
		 * 0x=hex, 0=octal, isdigit=decimal.
		 */
		if (!isdigit((unsigned char)c))
			return (0);
		val = 0; base = 10; digit = 0;
		if (c == '0') {
			c = *++cp;
			if (c == 'x' || c == 'X')
				base = 16, c = *++cp;
			else {
				base = 8;
				digit = 1 ;
			}
		}
		for (;;) {
			if (isascii(c) && isdigit((unsigned char)c)) {
				if (base == 8 && (c == '8' || c == '9'))
					return (0);
				val = (val * base) + (c - '0');
				c = *++cp;
				digit = 1;
			} else if (base == 16 && isascii(c) && 
				   isxdigit((unsigned char)c)) {
				val = (val << 4) |
					(c + 10 - (islower((unsigned char)c) ? 'a' : 'A'));
				c = *++cp;
				digit = 1;
			} else
				break;
		}
		if (c == '.') {
			/*
			 * Internet format:
			 *	a.b.c.d
			 *	a.b.c	(with c treated as 16 bits)
			 *	a.b	(with b treated as 24 bits)
			 */
			if (pp >= parts + 3 || val > 0xffU)
				return (0);
			*pp++ = val;
			c = *++cp;
		} else
			break;
	}
	/*
	 * Check for trailing characters.
	 */
	if (c != '\0') {
		if (strict) return (0);
		if (!isascii(c) || !isspace(c)) return (0);
	}
	/*
	 * Did we get a valid digit?
	 */
	if (!digit)
		return (0);
	/*
	 * Concoct the address according to
	 * the number of parts specified.
	 */
	n = pp - parts + 1;
	switch (n) {
	case 1:				/*%< a -- 32 bits */
		break;

	case 2:				/*%< a.b -- 8.24 bits */
		if (val > 0xffffffU)
			return (0);
		val |= parts[0] << 24;
		break;

	case 3:				/*%< a.b.c -- 8.8.16 bits */
		if (val > 0xffffU)
			return (0);
		val |= (parts[0] << 24) | (parts[1] << 16);
		break;

	case 4:				/*%< a.b.c.d -- 8.8.8.8 bits */
		if (val > 0xffU)
			return (0);
		val |= (parts[0] << 24) | (parts[1] << 16) | (parts[2] << 8);
		break;
	}
	if (addr != NULL)
		addr->s_addr = htonl(val);
	return (1);
}

```



我们逐步拆解

```c
// L117-122
u_long val;
int base, n;
char c;
u_int8_t parts[4];
u_int8_t *pp = parts;
int digit;
```

`parts[4]` 用来存储已经遇到 `.` 之后的完成段，`pp` 是写入指针，`val` 保存当前正在解析的数值。

外层是一个 `for (;;)` 循环，每次解析一个段直到遇到 `.` 或者字符串结束：

```c
// L131-142: 进制检测
if (!isdigit((unsigned char)c))
    return (0);
val = 0; base = 10; digit = 0;
if (c == '0') {
    c = *++cp;
    if (c == 'x' || c == 'X')
        base = 16, c = *++cp;
    else {
        base = 8;
        digit = 1;
    }
}
```

字符串表示的进制规则：`0x` 前缀为十六进制，`0` 前缀为八进制，否则十进制。

```c
// L143-158: 数字累积
for (;;) {
    if (isascii(c) && isdigit((unsigned char)c)) {
        if (base == 8 && (c == '8' || c == '9'))
            return (0);
        val = (val * base) + (c - '0');
        c = *++cp;
        digit = 1;
    } else if (base == 16 && isascii(c) && isxdigit((unsigned char)c)) {
        val = (val << 4) |
            (c + 10 - (islower((unsigned char)c) ? 'a' : 'A'));
        c = *++cp;
        digit = 1;
    } else
        break;
}
```



### 遇到 `.` 时存储段值

```c
// L159-171
if (c == '.') {
    /*
     * Internet format:
     *	a.b.c.d
     *	a.b.c	(with c treated as 16 bits)
     *	a.b	(with b treated as 24 bits)
     */
    if (pp >= parts + 3 || val > 0xffU)
        return (0);
    *pp++ = val;
    c = *++cp;
} else
    break;
```

注意两个限制：

1. `pp >= parts + 3`：最多 3 个点（4 段）
2. `val > 0xffU`：**已完成段**的值必须 ≤ 255

但最后一段（尚未遇到 `.`，循环结束时 `val` 中的值）的范围限制是在后面的 `switch` 中完成的



### 按段数组装 32 位地址

```c
// L189
n = pp - parts + 1;
```

`n` 就是总段数：`parts[]` 中已存储的段数 +1 (最后一段在 `val` 中)

```c
// L190-211
switch (n) {
case 1:             /*%< a -- 32 bits */
    break;

case 2:             /*%< a.b -- 8.24 bits */
    if (val > 0xffffffU)
        return (0);
    val |= parts[0] << 24;
    break;

case 3:             /*%< a.b.c -- 8.8.16 bits */
    if (val > 0xffffU)
        return (0);
    val |= (parts[0] << 24) | (parts[1] << 16);
    break;

case 4:             /*%< a.b.c.d -- 8.8.8.8 bits */
    if (val > 0xffU)
        return (0);
    val |= (parts[0] << 24) | (parts[1] << 16) | (parts[2] << 8);
    break;
}
```



最后将结果写入 `in_addr`：

```c
// L212-214
if (addr != NULL)
    addr->s_addr = htonl(val);
return (1);
```



### 示例 `127.1`

用输入 `127.1` 走一遍这段代码：

1. 第一轮循环：解析 `127`，遇到 `.`，`val=127 ≤ 255`，存入 `parts[0]=127`，`pp` 前进
2. 第二轮循环：解析 `1`，遇到 `\0`，退出循环，`val=1`
3. `n = pp - parts + 1 = 1 + 1 = 2`
4. 进入 `case 2`：`val=1 <= 0xFFFFFF`，`val |= 127 << 24` -> `val = 0x7F000001`
5. `htonl(0x7F000001)` -> 网络字节序存入 `addr->s_addr`
6. 转换回点分十进制：`127.0.0.1`

### 示例 `0`

1. 第一轮循环：解析 `0`（八进制模式，但值为 0），遇到 `\0`，退出循环，`val=0`
2. `n = 0 + 1 = 1`
3. 进入 `case 1`：直接 break，`val=0`
4. `htonl(0)` -> `0.0.0.0`



----



## glibc 中的 inet_aton

Linux 上的 `inet_aton` 由 glibc 提供，源码位于 `glibc/resolv/inet_addr.c`。以下分析基于 commit [`04e750e`](https://sourceware.org/git/?p=glibc.git;a=commit;h=04e750e75b73957cf1c791535a3f4319534a52fc)



glibc 的公开 `inet_aton` 是这样导出的：

```c
// glibc/resolv/inet_addr.c L197-204
/* inet_aton ignores trailing garbage.  */
int
__inet_aton_ignore_trailing (const char *cp, struct in_addr *addr)
{
  const char *endp;
  return  inet_aton_end (cp, addr, &endp);
}
weak_alias (__inet_aton_ignore_trailing, inet_aton)
```



核心逻辑在静态函数 `inet_aton_end` 中（L105-179）。和 Apple Libc 的 `_inet_aton_check` 相比，它有几个结构性的不同



### 差异一：用 strtoul 代替手动解析

Apple Libc 手动逐字符检测进制和累积数值，而 glibc 直接调用 `__strtoul_internal`：

```c
// glibc/resolv/inet_addr.c L131-141
{
    char *endp;
    unsigned long ul = __strtoul_internal (cp, &endp, 0, 0);
    if (ul == ULONG_MAX && errno == ERANGE)
        goto ret_0;
    if (ul > 0xfffffffful)
        goto ret_0;
    val = ul;
    digit = cp != endp;
    cp = endp;
}
```

`base = 0` 意味着遵循 C 语言的进制规则：`0x` -> 十六进制，`0` 前缀 -> 八进制，否则十进制。效果和 Apple Libc 手动解析一致，但代码更简洁。



### 差异二：用 max 数组代替 switch

Apple Libc 用 `switch (n)` 分别检查每种段数下最后一段的上限。glibc 则用一个预计算的数组：

```c
// glibc/resolv/inet_addr.c L108
static const in_addr_t max[4] = { 0xffffffff, 0xffffff, 0xffff, 0xff };
```

| `pp - res.bytes` | 格式                   | 最后一段上限 |
| ---------------- | ---------------------- | ------------ |
| 0                | `a` (32 位)            | `0xffffffff` |
| 1                | `a.b` (8.24 位)        | `0xffffff`   |
| 2                | `a.b.c` (8.8.16 位)    | `0xffff`     |
| 3                | `a.b.c.d` (8.8.8.8 位) | `0xff`       |

```c
// L166-167
if (val > max[pp - res.bytes])
    goto ret_0;
```

一行代码就代替了 BSD 版本中 `switch` 的四个 case



### 差异三：地址组装方式

Apple Libc 在 `switch` 中用显式的位移来组装 32 位地址。glibc 则利用 `union` 和 `htonl` 的组合：

```c
// glibc/resolv/inet_addr.c L111-115
union iaddr
{
    uint8_t bytes[4];
    uint32_t word;
} res;
```

在循环中，遇到 `.` 时直接将值写入 `res.bytes[]`：

```c
// L149-151
if (pp > res.bytes + 2 || val > 0xff)
    goto ret_0;
*pp++ = val;
```

最终组装：

```c
// L169-170
if (addr != NULL)
    addr->s_addr = res.word | htonl (val);
```

`res.word` 中已经按内存顺序（网络字节序）存储了前面的段，`htonl(val)` 将最后一段也转为网络字节序后做 OR 运算。结果和 Apple Libc 的 `htonl(val)` 是等价的，只是 Apple Libc 在 `val` 中用位移完成了所有组装。



### 示例 `127.1`

1. `__strtoul_internal("127.1", ..., 0, 0)` -> `ul=127`，`cp` 移动到 `.` 位置
2. 遇到 `.`，`val=127 <= 0xff`，`res.bytes[0] = 127`，`pp` 前进
3. `__strtoul_internal("1", ..., 0, 0)` -> `ul=1`，`cp` 移动到 `\0`
4. 不是 `.`，退出循环，`val=1`
5. `pp - res.bytes = 1`，`val=1 <= max[1]=0xffffff`，通过
6. `addr->s_addr = res.word | htonl(1)`。这一步需要区分「整数值」和「内存字节」两个视角，以小端机器为例：
   - `bytes[0]=127` 写入后内存字节为 `7f 00 00 00`，但 `res.word` 作为 `uint32_t` 的整数读值是 `0x0000007f`
   - `htonl(1)` 的整数值是 `0x01000000`（其内存字节为 `00 00 00 01`）
   - 整数层面 OR：`0x0000007f | 0x01000000 = 0x0100007f`，而 `0x0100007f` 的小端内存字节恰好是 `7f 00 00 01`
   - 即 `127.0.0.1`：union 按字节写入的是网络序，`htonl` 的结果在内存布局上与之对齐，所以 OR 起来严丝合缝（整数读值随主机字节序变化，不变的是内存字节）



### 差异四：`__inet_aton_exact`（CVE-2016-10739）

glibc 在 2.29 版本（2019 年）引入了一个内部函数 `__inet_aton_exact`，用于修复 [CVE-2016-10739](https://access.redhat.com/security/cve/cve-2016-10739)：

```c
// glibc/resolv/inet_addr.c L181-194
int
__inet_aton_exact (const char *cp, struct in_addr *addr)
{
  struct in_addr val;
  const char *endp;
  /* Check that inet_aton_end parsed the entire string.  */
  if (inet_aton_end (cp, &val, &endp) != 0 && *endp == 0)
    {
      *addr = val;
      return 1;
    }
  else
    return 0;
}
```

问题在于，`inet_aton` 会接受 IP 地址后面跟的空白字符及后续内容。例如 `"127.0.0.1\r\npayload"` 会被 `inet_aton` 认为是有效的 `127.0.0.1`。`getaddrinfo` 内部使用了 `inet_aton`，导致攻击者可以在 IP 地址后面注入 HTTP Header。修复方案是让 `getaddrinfo` 改用 `__inet_aton_exact`，严格要求解析到字符串末尾。



Apple Libc 中没有这个 `exact` 版本的分离——不过 `_inet_aton_check` 的第三个参数 `strict` 正是等价机制的雏形



---



## 其实 curl 并不使用 inet_aton

如果你以为 curl 是直接调用系统的 `inet_aton()` 来处理 `127.1`，那就错了。

搜索 curl 的源码（commit [`3f1c033`](https://github.com/curl/curl/tree/3f1c033afe008123680306663cc01279e831ac2d)），会发现 `inet_aton` 在库代码中**完全没有被使用**。curl 自带了一个 `curlx_inet_pton()` 实现（位于 `lib/curlx/inet_pton.c`），其中的 `inet_pton4()` 函数注释写得很直白：

```c
// lib/curlx/inet_pton.c L53-54
/* int inet_pton4(src, dst)
 *      like inet_aton() but without all the hexadecimal and shorthand. */
```



这个函数严格要求四段十进制格式，在 L95-96 直接拒绝了不足 4 段的输入：

```c
if(octets < 4)
    return 0;
```

那 `127.1` 的展开是谁做的？



### curl 中的 `ipv4_normalize`

答案在 `lib/urlapi.c` 的 `ipv4_normalize()` 函数中（L524-627）。这是 curl 自己实现的一个类似 `inet_aton` 的解析器，专门用于 URL 中的主机名部分。

调用链如下：

```
curl_url_set(CURLUPART_URL, "http://127.1/")
  -> parseurl()                        // L1150
    -> parse_authority()               // L656
      -> parse_hostname_login()        // L669: 提取 user:password@
      -> parse_port()                  // L679: 提取 :port
      -> urldecode_host()              // L685: URL 解码
      -> ipv4_normalize(&host)         // L689: 这里
```



`parse_authority` 在完成登录信息提取、端口解析和 URL 解码后，调用 `ipv4_normalize` 来判断主机名是否为 IPv4 地址并进行规范化：

```c
// lib/urlapi.c L689-704
switch(ipv4_normalize(host)) {
case HOST_IPV4:
    break;
case HOST_IPV6:
    uc = ipv6_parse(u, curlx_dyn_ptr(host), curlx_dyn_len(host));
    break;
case HOST_NAME:
    uc = hostname_check(u, curlx_dyn_ptr(host), curlx_dyn_len(host));
    break;
// ...
}
```

### 

```c
// lib/urlapi.c L524
UNITTEST int ipv4_normalize(struct dynbuf *host)
{
  bool done = FALSE;
  int n = 0;
  const char *c = curlx_dyn_ptr(host);
  unsigned int parts[4] = { 0, 0, 0, 0 };
  // ...
```



它的结构和 `_inet_aton_check` 非常相似，同样支持十六进制（`0x`）、八进制（`0` 前缀）以及 1~4 段的地址格式。解析完之后按段数进行组装：

```c
// lib/urlapi.c L582-623
switch(n) {
case 0: /* a -- 32 bits */
    // ...
    result = curlx_dyn_addf(host, "%u.%u.%u.%u",
                            (parts[0] >> 24),
                            ((parts[0] >> 16) & 0xff),
                            ((parts[0] >> 8) & 0xff),
                            (parts[0] & 0xff));
    break;
case 1: /* a.b -- 8.24 bits */
    if((parts[0] > 0xff) || (parts[1] > 0xffffff))
        return HOST_NAME;
    // ...
    result = curlx_dyn_addf(host, "%u.%u.%u.%u",
                            parts[0],
                            ((parts[1] >> 16) & 0xff),
                            ((parts[1] >> 8) & 0xff),
                            (parts[1] & 0xff));
    break;
case 2: /* a.b.c -- 8.8.16 bits */
    // ...
case 3: /* a.b.c.d -- 8.8.8.8 bits */
    // ...
}
```

注意和 libc `_inet_aton_check` / `inet_aton_end` 的一个区别：curl 的 `ipv4_normalize` 直接将结果格式化为标准的 `a.b.c.d` 字符串写回 `host` buffer。而 libc 的版本是组装成一个 32 位的 `in_addr`，输出的是二进制地址。

也就是说，在 URL 解析阶段，`127.1` 就已经被规范化成了 `127.0.0.1` 这个字符串。后续 DNS 解析/连接阶段拿到的 host 已经是标准格式了，比如后续调用的 `curlx_inet_pton` 只需要处理标准格式即可



### 从 URL 到 connect(2)

继续分析，`ipv4_normalize` 把 `127.1` 改写为 `127.0.0.1` 之后，这个字符串还要经过解析(resolve)阶段变成 socket 地址，最后交给 `connect(2)`。完整链路是：

```
parseurl -> ipv4_normalize              lib/urlapi.c          host 字符串变为 "127.0.0.1"
cf_dns_start -> Curl_resolv             lib/vdns/cf-dns.c L192, hostip.c L1023
  -> hostip_resolv -> hostip_resolv_start                      hostip.c L703, L557
cf_tcp_connect -> do_connect            lib/cf-socket.c L1406, L1351  -> connect(2)
```



#### Linux 默认构建：IP 字面量走捷径，完全跳过 libc 的解析器

`hostip_resolv_start` 的开头有一段针对 IP 字面量的快捷路径（`lib/vdns/hostip.c` L584-595）：

```c
#ifndef USE_RESOLVE_ON_IPS
    if(Curl_is_ipaddr(peer->hostname)) {
      /* ... */
      /* shortcut literal IP addresses, if we are not told to resolve them. */
      result = Curl_str2addr(peer->hostname, peer->port, &addr);
      goto out;
    }
#endif
```

- `Curl_is_ipaddr()`（`lib/curl_addrinfo.c` L447）内部调用 `curlx_inet_pton(AF_INET, ...)` 判断是否为数字地址
- `Curl_str2addr()`（L423-439）同样用 `curlx_inet_pton` 完成解析，然后由 `ip2addr()` 直接填充 `Curl_addrinfo` 结构（`sin_addr` + `htons(port)`）

也就是说在 Linux 上，从识别字面量到得到地址结构，全程都是 curl 自带的代码——`getaddrinfo` 不会被调用。之后 `lib/cf-socket.c` 的 `do_connect()` 直接对这个地址发起 `connect(2)`



#### macOS 默认构建：故意绕道 getaddrinfo

但 macOS 是个例外。`lib/curl_setup.h` L400-406：

```c
/*
 * Use getaddrinfo to resolve the IPv4 address literal. If the current network
 * interface does not support IPv4, but supports IPv6, NAT64, and DNS64,
 * performing this task will result in a synthesized IPv6 address.
 */
#if defined(__APPLE__) && !defined(USE_ARES)
#  define USE_RESOLVE_ON_IPS 1
```

在 macOS 上（且未启用 c-ares）`USE_RESOLVE_ON_IPS` 被定义，上面的捷径被编译掉，IP 字面量改走正常的解析路径：`Curl_async_getaddrinfo`（线程解析器，`lib/vdns/asyn-thrdd.c` L630）在线程中调用系统的 `getaddrinfo()`，并且对数字地址**不设置** `AI_NUMERICHOST` 标志（`asyn-thrdd.c` L392-404）。同步解析器代码里的一条注释解释了为什么不设置（`lib/vdns/hostip6.c` L90-93——注意这段注释位于 `#ifndef USE_RESOLVE_ON_IPS` 块内，在 macOS 默认构建下并不参与编译，此处引用是为了说明设计动机）：

```c
/*
 * The AI_NUMERICHOST must not be set to get synthesized IPv6 address from
 * an IPv4 address on iOS and macOS.
 */
```

原因是 NAT64/DNS64 网络（iOS/蜂窝网络环境的常见配置）：当本机只有 IPv6 连接时，`getaddrinfo` 可以把 IPv4 字面量合成为可达的 NAT64 IPv6 地址；如果 curl 自己解析掉就会失去这个能力



---



## ping 127.1 呢

另一个常用工具 ping 也有同样的行为

```
$ ping -c 1 127.1
PING 127.1 (127.0.0.1): 56 data bytes
64 bytes from 127.0.0.1: icmp_seq=0 ttl=64 time=0.047 ms
```



和 curl 不同，ping 没有自带解析器，两个平台的 ping 最终都把数字地址解析交给了 libc 的 `inet_aton()`，只是路径不同



### macOS ping：直接调用 inet_aton

macOS 的 ping 来自 network_cmds 项目，以下分析基于 commit [`97e27e6`](https://github.com/apple-oss-distributions/network_cmds/tree/97e27e6244c16d399bfeb254315ddc5828711c56)

`ping.tproj/ping.c` L684：

```c
if (inet_aton(target, &to->sin_addr) != 0) {
    hostname = target;
} else {
    hp = gethostbyname2(target, AF_INET);
    // ...
}
```

`inet_aton` 成功就直接使用，失败才交给 `gethostbyname2` 走系统解析（`/etc/hosts`、mDNS、DNS）



### Linux ping：getaddrinfo 定族，inet_aton 定址

Linux 的 ping 来自于 iputils 项目， commit [18717a3](https://github.com/iputils/iputils/tree/18717a3984c83452138d6bed76426237dbe41ae7)。iputils 的 ping 对目标地址做了两次解析（`ping/ping.c`）：

1. `main()` 在 L710 调用 `getaddrinfo(target, NULL, &hints, &result)`，用来确定地址族（走 IPv4 还是 IPv6）
2. 进入 `ping4_run()` 后，L830 再次调用 `inet_aton(target, &rts->whereto.sin_addr)`——目标是数字地址时（`inet_aton` 成功），**实际使用的地址来自这次调用**；主机名目标则使用 `getaddrinfo` 的结果（L844）

所以 Linux 上简写生效同样直接落在 `inet_aton` 上。即便只看 `getaddrinfo` 那条路，glibc 的数字地址解析内部也是 `__inet_aton_exact`（`nss/getaddrinfo.c` L886，即前文 CVE-2016-10739 修复引入的函数），inet_aton 的语义换个入口又出来了。



### ping 0：源码与内核的分工

`ping 0` 在两个平台的表现不同：

```
# macOS
$ ping -c 1 0
PING 0 (0.0.0.0): 56 data bytes
ping: sendto: Socket is not connected

# Linux
$ ping -c 1 0
PING 0 (127.0.0.1) 56(84) bytes of data.
64 bytes from 127.0.0.1: icmp_seq=1 ttl=64 time=0.013 ms
```



两边的 `inet_aton` 都把 `0` 解析为 `0.0.0.0`（case 1，单数字 = 32 位），分歧发生在解析之后：

- **macOS ping 对 `0.0.0.0` 不做任何替换**，原样传入 `sendto()`（ping.c L1235），内核返回 `ENOTCONN`，即 `Socket is not connected `，ping 源码只是如实传递，拒绝发生在内核协议栈（示例为非 root 的 SOCK_DGRAM socket；root 使用 SOCK_RAW，ping.c L314-317）
- **iputils ping 有一段主动替换逻辑**（ping.c L939-940）：

```c
if (rts->whereto.sin_addr.s_addr == 0)
    rts->whereto.sin_addr.s_addr = rts->source.sin_addr.s_addr;
```

目标为 `INADDR_ANY` 时，用探测 socket 拿到的本机源地址顶替目标（L890 `connect(0.0.0.0:1025)` 成功、L909 `getsockname` 返回 `127.0.0.1`），于是 header 显示 `PING 0 (127.0.0.1)`，后续正常收到 packet 之后。其中`connect(0.0.0.0)` 能成功且源地址是 `127.0.0.1` 属于 Linux 内核把 `0.0.0.0` 视为本机的行为；地址替换则是 ping 自己的代码。





## 这个设计从哪里来

### 4.2BSD 和有类别寻址

这种多段式 IP 地址记法起源于 1983 年的 **4.2BSD**。当时的 IP 网络使用有类别寻址（Classful Addressing），IP 地址被划分为若干类别，其中单播地址主要为 A/B/C 三类（D 类组播、E 类保留）：

| 类别    | 网络位 | 主机位 | 地址范围                    |
| ------- | ------ | ------ | --------------------------- |
| Class A | 8 bit  | 24 bit | 0.0.0.0 ~ 127.255.255.255   |
| Class B | 16 bit | 16 bit | 128.0.0.0 ~ 191.255.255.255 |
| Class C | 24 bit | 8 bit  | 192.0.0.0 ~ 223.255.255.255 |

简写形式和类别恰好对应：

- `a.b`（8.24 位）-> Class A 地址的简写，`net.host`
- `a.b.c`（8.8.16 位）-> Class B 地址的简写，`net.net.host`
- `a.b.c.d`（8.8.8.8 位）-> Class C 或精确指定

Apple Libc 和 glibc 的代码注释中都保留了这段历史。Apple Libc `inet_addr.c` L159-165：

```c
if (c == '.') {
    /*
     * Internet format:
     *	a.b.c.d
     *	a.b.c	(with c treated as 16 bits)
     *	a.b	(with b treated as 24 bits)
     */
```

glibc `inet_addr.c` L145-148 几乎一字不差：

```c
if (c == '.')
    {
      /* Internet format:
         a.b.c.d
         a.b.c	(with c treated as 16 bits)
         a.b	(with b treated as 24 bits).  */
```



### IETF draft 和 RFC 3986

2005 年 IETF 曾有一份 [draft-main-ipaddr-text-rep-02](https://datatracker.ietf.org/doc/html/draft-main-ipaddr-text-rep-02) 试图规范化 IP 地址的文本表示。其中提到：

> 4.2BSD introduced a function inet_aton(), whose job was to interpret character strings as IP addresses. It interpreted both of the syntaxes mentioned in [MTP] (see above): a single number giving the entire 32-bit address, and dot-separated octet values. It also interpreted two intermediate syntaxes: octet-dot-octet-dot-16bits, intended for class B addresses, and octet-dot-24bits, intended for class A addresses.

> The 4.2BSD inet_aton() has been widely copied and imitated, and so is a de facto standard for the textual representation of IPv4 addresses. Nevertheless, these alternative syntaxes have now fallen out of use (if they ever had significant use).

这份 draft 最终过期了，没有成为正式 RFC。

而 [RFC 3986](https://www.rfc-editor.org/rfc/rfc3986.html)（URI 规范）在 Section 3.2.2 中明确限定了 URI 中 IPv4 地址的语法**只接受**标准四段十进制格式：

> A host identified by an IPv4 literal address is represented in dotted-decimal notation (a sequence of four decimal numbers in the range 0 to 255, separated by "."), as described in [RFC1123] by reference to [RFC0952]. Note that other forms of dotted notation may be interpreted on some platforms, as described in Section 7.4, but only the dotted-decimal form of four octets is allowed by this grammar.

所以严格来说，`http://127.1/` 中的 `127.1` 并不是 RFC 3986 定义的合法 IPv4 地址。但 curl 选择支持它，这是为了和 `inet_aton` 的行为保持一致。



## inet_aton vs inet_pton

`inet_pton` 是更现代的函数（来自 BIND 4.9.4，后被 POSIX.1-2001 标准化），它**不支持**简写形式。NetBSD 的 man page 写得很清楚：

> Note that inet_pton() does not accept 1-, 2-, or 3-part dotted addresses; all four parts must be specified. Additionally all four parts of a dotted address must be decimal. This is a narrower input set than that accepted by inet_aton().

值得一提的是，`inet_aton` 本身并未被 POSIX 标准化。现行 Linux `inet(3)` man page 的 STANDARDS 一节对 `inet_aton()` 只写了一个词：

> None.

（旧版 man page 的说法是「inet_aton() is not specified in POSIX.1, but is available on most systems」，现行页面已删除这句。）所以它虽然是一个跨平台的标准，但从未被正式标准化过。`inet_pton` 才是 POSIX 认可的接口



## 小结

- `curl 127.1:80` 请求到 `127.0.0.1` 是因为 curl 在 URL 解析阶段通过 `ipv4_normalize()` 将 `127.1` 按照 `a.b`（8.24 位）格式展开为 `127.0.0.1`
- `curl 0:80` 请求到 `0.0.0.0` 是因为 `0` 被按照单数字（32 位）格式直接解释为 `0x00000000`
- curl 并没有调用系统的 `inet_aton()`，而是自己实现了一套功能类似的 `ipv4_normalize()`
- `127.1` 的简写展开完全由 curl 在 URL 解析阶段完成，与平台 libc 无关。Linux 上连后续的地址解析都不触达 libc（`Curl_str2addr` 直接构造地址）；macOS 默认构建为支持 NAT64/DNS64 会让 `getaddrinfo` 参与，但它拿到的已是标准格式
- 这种多段式 IP 地址写法源自 1983 年 4.2BSD 的 `inet_aton()`，为有类别寻址而设计
- macOS（Apple Libc）和 Linux（glibc）虽然实现细节不同，但 `inet_aton` 的多段式语义完全一致，都来自同一个 4.2BSD 祖先
- ping 同样接受简写：macOS ping 直接调用 `inet_aton()`，Linux 的 iputils ping 也在 `ping4_run()` 中用 `inet_aton()` 解析数字地址（`getaddrinfo` 负责确定地址族）——是否支持简写取决于哪个组件负责解析
- `inet_aton` 从未被 POSIX 标准化；POSIX 标准化的 `inet_pton` 不接受这种简写



## Reference

- [curl/curl](https://github.com/curl/curl) `lib/urlapi.c` `ipv4_normalize()` (L524-627)、`lib/vdns/hostip.c` `hostip_resolv_start()` (L557)、`lib/curl_addrinfo.c` `Curl_str2addr()` (L423)、`lib/cf-socket.c` `do_connect()` (L1351)、`lib/curl_setup.h` `USE_RESOLVE_ON_IPS` (L405-406), commit `3f1c033`
- [apple-oss-distributions/Libc](https://github.com/apple-oss-distributions/Libc) `net/FreeBSD/inet_addr.c` `_inet_aton_check()` (L115-215), commit `71bbe35`
- [glibc](https://sourceware.org/git/?p=glibc.git) `resolv/inet_addr.c` `inet_aton_end()` (L105-179), commit `04e750e`
- [apple-oss-distributions/network_cmds](https://github.com/apple-oss-distributions/network_cmds) `ping.tproj/ping.c`, commit `97e27e6`
- [iputils/iputils](https://github.com/iputils/iputils) `ping/ping.c`, commit `18717a3`
- [CVE-2016-10739](https://access.redhat.com/security/cve/cve-2016-10739) - getaddrinfo: Fully parse IPv4 address strings
- [draft-main-ipaddr-text-rep-02](https://datatracker.ietf.org/doc/html/draft-main-ipaddr-text-rep-02) - Textual Representation of IP Addresses (expired)
- [RFC 3986](https://www.rfc-editor.org/rfc/rfc3986.html) Section 3.2.2 - Host
- [inet(3) - Linux manual page](https://www.man7.org/linux/man-pages/man3/inet_aton.3.html)
- [inet(3) - NetBSD Manual Pages](https://man.netbsd.org/NetBSD-7.1/inet.3)
- [Dot-decimal notation - Wikipedia](https://en.wikipedia.org/wiki/Dot-decimal_notation)
