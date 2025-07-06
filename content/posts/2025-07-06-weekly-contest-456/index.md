+++
title = "LeetCode Weekly Contest 456"
summary = ""
description = ""
categories = ["algorithm"]
tags = ["LeetCode"]
date = 2025-07-06T08:26:51+09:00
draft = false

+++



## 3597. Partition String

[3597. Partition String](https://leetcode.com/problems/partition-string/description/)



阅读理解题目，按照描述的步骤做就行了

| Index | Segment After Adding | Seen Segments               | Current Segment Seen Before? | New Segment | Updated Seen Segments            |
|-------|----------------------|-----------------------------|------------------------------|-------------|----------------------------------|
| 0     | "a"                  | []                          | No                           | ""          | ["a"]                            |
| 1     | "b"                  | ["a"]                       | No                           | ""          | ["a", "b"]                       |
| 2     | "b"                  | ["a", "b"]                  | Yes                          | "b"         | ["a", "b"]                       |
| 3     | "bc"                 | ["a", "b"]                  | No                           | ""          | ["a", "b", "bc"]                 |
| 4     | "c"                  | ["a", "b", "bc"]            | No                           | ""          | ["a", "b", "bc", "c"]            |
| 5     | "c"                  | ["a", "b", "bc", "c"]       | Yes                          | "c"         | ["a", "b", "bc", "c"]            |
| 6     | "cc"                 | ["a", "b", "bc", "c"]       | No                           | ""          | ["a", "b", "bc", "c", "cc"]      |
| 7     | "d"                  | ["a", "b", "bc", "c", "cc"] | No                           | ""          | ["a", "b", "bc", "c", "cc", "d"] |



```go
func partitionString(s string) []string {
	segments := make([]string, 0)
	seen := make(map[string]bool)

	start := 0
	for i := 1; i <= len(s); i++ {
		segment := s[start:i]
		if !seen[segment] {
			seen[segment] = true
			segments = append(segments, segment)
			start = i
		}

	}
	return segments
}
```

这里的 `seen` 可以使用 HashMap 也可以使用 Trie 来做





## 3598. Longest Common Prefix Between Adjacent Strings After Removals



### 题目

[3598. Longest Common Prefix Between Adjacent Strings After Removals](https://leetcode.com/problems/longest-common-prefix-between-adjacent-strings-after-removals/)



给你一个字符串数组 `words`，对于范围 `[0, words.length - 1]` 内的每个下标 `i`，执行以下步骤：

1. 从 `words` 数组中移除下标 `i` 处的元素。
2. 计算修改后的数组中所有 **相邻对** 之间的 **最长公共前缀** 的长度。

返回一个数组 `answer`，其中 `answer[i]` 是移除下标 `i` 后，相邻对之间最长公共前缀的长度。
如果 **不存在相邻对**，或者 **不存在公共前缀**，则 `answer[i]` 应为 `0`。

**注**：字符串的前缀是从字符串的开头开始延伸到任意位置的子字符串。

例子1

Input: words = `["jump","run","run","jump","run"]`

Output: `[3,0,0,3,3]`

解释

* 移除下标 0：
  `words` 变为 `["run", "run", "jump", "run"]`
  最长的相邻对是 `["run", "run"]`，其公共前缀为 `"run"`（长度为 3）

* 移除下标 1：
  `words` 变为 `["jump", "run", "jump", "run"]`
  没有相邻对有公共前缀（长度为 0）

* 移除下标 2：
  `words` 变为 `["jump", "run", "jump", "run"]`
  没有相邻对有公共前缀（长度为 0）

* 移除下标 3：
  `words` 变为 `["jump", "run", "run", "run"]`
  最长的相邻对是 `["run", "run"]`，其公共前缀为 `"run"`（长度为 3）

* 移除下标 4：
  `words` 变为 `["jump", "run", "run", "jump"]`
  最长的相邻对是 `["run", "run"]`，其公共前缀为 `"run"`（长度为 3）



### 思路




第一点是需要注意在移除元素 `i` 后，`words[i-1]` 和 `words[i+1]` 会变成新的相邻对。 比如 原始：`["fcc", "cdfda", "fcec"]` 移除下标 1 后变成 `["fcc", "fcec"]` 我们应该比较 `fcc` 和 `fcec`



第二个点就是这个题直接暴力求解会超时的



其实我们可以将数组分为三个部分来求解



1. 首先计算每一对 `i` 和 `i+1` 的 LCP。
2. 存入数组 `lcpPairs[i] = LCP(words[i], words[i+1])`
3. 每次移除一个下标 `i`，只需在：

   * `lcpPairs[0..i-2]`
   * `lcp(words[i-1], words[i+1])`（如果 `i-1` 和 `i+1` 都存在）
   * `lcpPairs[i+1..n-2]`
     中找最大值。



通过这样的预处理可以大大减少重复计算量



```go
package main

import (
	"fmt"
)

func lcp(a, b string) int {
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	for i := 0; i < minLen; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return minLen
}

func longestCommonPrefix(words []string) []int {
	n := len(words)
	ret := make([]int, n)
	if n <= 1 {
		return ret
	}

	// Step 1: 预处理所有相邻对的 LCP
	lcpPairs := make([]int, n-1)
	for i := 0; i < n-1; i++ {
		lcpPairs[i] = lcp(words[i], words[i+1])
	}

	// Step 2: 构建左区间最大值 & 右区间最大值数组
	leftPartMax := make([]int, n-1)
	rightPartMax := make([]int, n-1)

	leftPartMax[0] = lcpPairs[0]
	for i := 1; i < n-1; i++ {
		if lcpPairs[i] > leftPartMax[i-1] {
			leftPartMax[i] = lcpPairs[i]
		} else {
			leftPartMax[i] = leftPartMax[i-1]
		}
	}

	rightPartMax[n-2] = lcpPairs[n-2]
	for i := n - 3; i >= 0; i-- {
		if lcpPairs[i] > rightPartMax[i+1] {
			rightPartMax[i] = lcpPairs[i]
		} else {
			rightPartMax[i] = rightPartMax[i+1]
		}
	}

	// Step 3: 遍历每个被删除的 i
	for i := 0; i < n; i++ {
		maxLcp := 0
        // 处理好 i 的所有 case
		if i > 1 {
			maxLcp = leftPartMax[i-2]
		}
		if i+1 < n-1 {
			if rightPartMax[i+1] > maxLcp {
				maxLcp = rightPartMax[i+1]
			}
		}
		// 中间连接 i-1 和 i+1
		if i > 0 && i < n-1 {
			join := lcp(words[i-1], words[i+1])
			if join > maxLcp {
				maxLcp = join
			}
		}
		ret[i] = maxLcp
	}
	return ret
}

func main() {
	fmt.Println(longestCommonPrefix([]string{"jump", "run", "run", "jump", "run"}))                                               // [3 0 0 3 3]
	fmt.Println(longestCommonPrefix([]string{"dog", "racer", "car"}))                                                             // [0 0 0]
	fmt.Println(longestCommonPrefix([]string{"f", "cfe", "feab", "fcc", "cdfda", "fcec", "afae", "cdeb", "dc", "bffd", "edabe"})) //  [1,1,0,0,2,1,1,1,1,1,1]
}

```





## 3599. Partition Array to Minimize XOR



### 题目

[3599. Partition Array to Minimize XOR](https://leetcode.com/problems/partition-array-to-minimize-xor/)



给你一个整数数组 `nums` 和一个整数 `k`。你的任务是将 `nums` 分成 `k` 个非空的 **子数组** 。对每个子数组，计算其所有元素的按位 **XOR** 值。返回这 `k` 个子数组中 **最大 XOR** 的 **最小值** 。

**子数组** 是数组中连续的 **非空** 元素序列。

 

例子1

Input: nums = `[1,2,3]`, k = 2

Output: 1

解释：

最优划分是 `[1]` 和 `[2, 3]`。

- 第一个子数组的 XOR 是 `1`。
- 第二个子数组的 XOR 是 `2 XOR 3 = 1`。

子数组中最大的 XOR 是 1，是最小可能值。





### 思路



动态规划题目，不过首先我们需要知道 XOR 的一些性质

- `a ^ a = 0`
- `a ^ 0 = a`

所以可以得出
- `a ^ a ^ b = b`



我们首先构建一下前缀 XOR 结果的数组

```go
	prefix := make([]int, n+1)
	for i := 0; i < n; i++ {
		prefix[i+1] = prefix[i] ^ nums[i]
	}
```


* `prefix[i]` 存储的是 `nums[0] ^ nums[1] ^ ... ^ nums[i-1]` 的 XOR 结果
- 对于连续子数组 `nums[a...b]` 的结果就是 `prefix[a] ^ prefix[b+1]`。
  因为
  
   - `prefix[a] = nums[0] ^ nums[1] ^ .. ^ nums[a-1]`
   - `prefix[b+1] = nums[0] ^ nums[1] ^ .. ^ nums[a-1] ^ nums[a] ^ ... ^ nums[b]`
  
   这两个 XOR 之后可以进行消元，得到 `nums[a] ^ nums[a+1] ^ ... ^ nums[b]`



同样运用这个思路的题目是 [1310. XOR Queries of a Subarray](https://leetcode.com/problems/xor-queries-of-a-subarray/description/)



然后我们建立动态规划方程

```go
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, k+1)
		for j := range dp[i] {
			dp[i][j] = math.MaxInt32
		}
	}
	dp[0][0] = 0

```

`dp[i][j]` 表示将 `nums` 的前 `i` 个元素 (`nums[0...i-1]`) 划分为 `j` 个连续子数组时，所有这些子数组 XOR 和中的最大值的最小值




```go
	for i := 1; i <= n; i++ {
		for j := 1; j <= k && j <= i; j++ {
			for p := j - 1; p < i; p++ {
				x := prefix[i] ^ prefix[p]
				dp[i][j] = min(dp[i][j], max(dp[p][j-1], x))
			}
		}
	}
```

*   **外层循环 `i`**: 遍历数组元素的数量，从 `1` 到 `n`。`i` 代表当前考虑的是前 `i` 个元素 (`nums[0...i-1]`)。
*   **中间循环 `j`**: 遍历划分的子数组数量，从 `1` 到 `k`。同时 `j` 不能超过 `i` (因为每个子数组非空)。
*   **内层循环 `p`**: 遍历可能的划分点。`p` 表示第 `j` 个子数组的起始位置。具体来说，`nums[p...i-1]` 是第 `j` 个子数组。那么前 `j-1` 个子数组必定使用了 `nums[0...p-1]` 这些元素。因此，`dp[p][j-1]` 存储的就是前 `j-1` 个子数组的解。
    *   `p` 的范围：
        *   `p` 最小为 `j-1`：因为前 `j-1` 个子数组至少需要 `j-1` 个元素，这意味着 `p-1` 至少是 `j-2`，所以 `p` 至少是 `j-1`。
        *   `p` 最大为 `i-1`：因为第 `j` 个子数组至少要包含 `nums[i-1]` 这个元素，所以 `p` 必须小于 `i`。
*   **XOR 计算 `x`**: `x = prefix[i] ^ prefix[p]` 计算的是子数组 `nums[p...i-1]` 的异或和。
*   **`max(dp[p][j-1], x)`**: 对于一个特定的划分点 `p`，这表示当前这种划分方式（前 `j-1` 个子数组的最大 XOR 是 `dp[p][j-1]`，第 `j` 个子数组的 XOR 是 `x`）的总的最大 XOR 值。
*   **`min(...)` 更新 `dp[i][j]`**: 我们要找到所有可能的 `p` 中，使得 `max(dp[p][j-1], x)` 最小的那个值，因为这是 `dp[i][j]` 的定义。



完整代码如下

```go

func minXor(nums []int, k int) int {
	n := len(nums)

	prefix := make([]int, n+1)
	for i := 0; i < n; i++ {
		prefix[i+1] = prefix[i] ^ nums[i]
	}

	const inf = math.MaxInt32
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, k+1)
		for j := range dp[i] {
			dp[i][j] = inf
		}
	}
	dp[0][0] = 0

	for i := 1; i <= n; i++ {
		for j := 1; j <= k && j <= i; j++ {
			for p := j - 1; p < i; p++ {
				x := prefix[i] ^ prefix[p]
				dp[i][j] = min(dp[i][j], max(dp[p][j-1], x))
			}
		}
	}

	return dp[n][k]
}
```





P.S.  [1310. XOR Queries of a Subarray](https://leetcode.com/problems/xor-queries-of-a-subarray/description/) 的解法

```go
func xorQueries(arr []int, queries [][]int) []int {
	n := len(arr)
	prefix := make([]int, n+1)

	for i := 0; i < n; i++ {
		prefix[i+1] = prefix[i] ^ arr[i]
	}

	ret := make([]int, len(queries))

	for i, query := range queries {
		start:= query[0]
		end := query[1]
		ret[i] = prefix[start] ^ prefix[end+1]
	}

	return ret
}
```





## 3600. Maximize Spanning Tree Stability with Upgrades

### 题目

[3600. Maximize Spanning Tree Stability with Upgrades](https://leetcode.com/problems/maximize-spanning-tree-stability-with-upgrades/description/)



### 思路

好像是要用并查集，留个位置以后再补



https://cp-algorithms.com/data_structures/disjoint_set_union.html
