
+++
title = "Python sort 的实现 - Timsort 算法"
summary = ''
description = ""
categories = []
tags = []
date = 2017-05-01T07:59:31+08:00
draft = false
+++

*近日阅读编程珠玑，对算法突然又萌生了兴趣，于是翻看资料查找到了 Python 的排序算法*

### 概述

Timsort 是 Python builtin sort 所使用的一种算法，结合了归并排序与插入排序。最优时间复杂度为 `n`, 最差时间复杂度为 `nlogn`, 平均时间复杂度同为 `nlogn`, 空间复杂度为 `n` ，并且是稳定排序。Java 中对于非基础类型的排序也是使用的这个算法  
各种排序算法时间/空间复杂度可以从 [Sorting algorithm](https://en.wikipedia.org/wiki/Sorting_algorithm) 中得到  

Python 排序代码在源码包的 `Objects/listobject.c` 中，个人看了感觉十分难懂，遂转而去寻找 Java 的算法实现，根据蠢作者还在使用的 Java7 来说，位于 `/usr/lib/jvm/openjdk-7/src.zip` 中。如果你的 Linux 没有这个目录则需要 `apt-get install openjdk-7-source`  

### 算法实现
*如无特殊说明，代码均引自 TimSort.java 中的 TimSort 类， 并将比较器,泛型等去除，修改为直接比较 int 类型*    

Timsort 认为真实世界的数据看似无序实则存在或长或短的有序片段，它将这些片段称为 `run`，如 `[2, 3, 5, 4, 9]` 中的 `[2, 3, 5]` 和 `[4, 9]`。它遍历数组尽可能寻找这些 `run`

```Java
// 此方法被多次调用，用于寻找 run
private static int countRunAndMakeAscending(int[] a, int lo, int hi
                                               ) {
    assert lo < hi;
    int runHi = lo + 1;
    if (runHi == hi)
        return 1;

    // 根据前两个元素的比较，判断具有升序趋势还是降序趋势
    if (a[runHi++]<a[lo]) { // Descending
        while (runHi < hi && a[runHi]<a[runHi - 1])
            runHi++;
        // 降序转换成升序
        reverseRange(a, lo, runHi);
    } else {
        while (runHi < hi && a[runHi]>=a[runHi - 1])
            runHi++;
    }
    // 返回自然有序部分的长度
    return runHi - lo;
}
```

`run` 不能过短，存在一个 `minRun`，这个值是根据列表长度生成的  

```Java
private static int minRunLength(int n) {
    assert n >= 0;
    int r = 0;      // Becomes 1 if any 1 bits are shifted off
    while (n >= MIN_MERGE) {  // MIN_MERGE = 32  Python 中为 64
        r |= (n & 1);
        n >>= 1;
    }
    return n + r;
}
```

有序部分的长度一般不会太长，当小于 `minRun` 时，会将此部分后面的元素插入其中，直至长度满足 `minRun`  

```Java
private static void binarySort(int[] a, int lo, int hi, int start
                                   ) {
    // lo 为 run 的起始位置
		// hi 为 run 应该结束的位置 (即 run 的起始位置 + minRun)
		// start 当前有序部分的结束位置 (start 后的元素需要插入至 run 中)
    assert lo <= start && start <= hi;
    if (start == lo)
        start++;
    for ( ; start < hi; start++) {
        int pivot = a[start];

        // Set left (and right) to the index where a[start] (pivot) belongs
        int left = lo;
        int right = start;
        assert left <= right;
        /*
         * Invariants:
         *   pivot >= all in [lo, left).
         *   pivot <  all in [right, start).
         */
        // 通过二分法查找元素应当出现的位置
        while (left < right) {
            int mid = (left + right) >>> 1;
            if (pivot < a[mid])
                right = mid;
            else
                left = mid + 1;
        }
        assert left == right;

        int n = start - left;  // The number of elements to move
        // 将位置后的元素向后移动一个位置
        // 在元素和要插入的位置很近时，避免使用 arraycopy
        switch (n) {
            case 2:  a[left + 2] = a[left + 1];
            case 1:  a[left + 1] = a[left];
                     break;
            default: System.arraycopy(a, left, a, left + 1, n);
        }
        // 插入元素
        a[left] = pivot;
    }
}
```

满足长度要求的 `run` 会被 push 至一个 stack， Java 中的具体实现为调用了 `pushRun` 方法然后将起始位置和长度存入两个数组(栈)中  

```Java
private void pushRun(int runBase, int runLen) {
    // runBase 为 run 的起始位置
    // runLen  为 run 的长度
    this.runBase[stackSize] = runBase;  // stackSize 初始为 0
    this.runLen[stackSize] = runLen;
    stackSize++;
}
```

并且每次 push 后会调用 `mergeCollapse` 方法，检查下面这两个条件是否满足。若不满足会进行归并排序，直至满足条件为止

```
// i 为栈的大小
1. runLen[i - 3] > runLen[i - 2] + runLen[i - 1]
2. runLen[i - 2] > runLen[i - 1]

可以直观的表述为 // (X Y Z 为 runLen)

| Z |  <- top
| Y |  
| X |  <- bottom
-----

1. X > Y + Z
2. Y > Z
```

当然，如果已经遍历完数组找出了所有的 `run` ，也会进行归并

代码如下

```Java
private void mergeCollapse() {
    while (stackSize > 1) {
        int n = stackSize - 2;
        if (n > 0 && runLen[n-1] <= runLen[n] + runLen[n+1]) {
            // [n-1] [n] [n+1]
            // run[n] 和更小的那个进行 merge
            if (runLen[n - 1] < runLen[n + 1])
                n--;
            // 对 run[n] 和 run[n+1] 进行 merge
            mergeAt(n);
        } else if (runLen[n] <= runLen[n + 1]) {
            mergeAt(n);
        } else {
            break; // Invariant is established
        }
    }
}
```

归并排序使用了一些特殊的技巧  

1) 在 `run1` 中找 `run2` 最小元素的位置
2) 在 `run2` 中找 `run1` 最大元素的位置

充分利用了两个 `run` 是顺序存储且相邻的特点，缩小了排序的范围

`(1, 3, { 5, 7,) (4, 5, 6, } 10, 12)`
`()` 中为两个 `run`
`{}` 中为真正需要排序的部分

```Java
private void mergeAt(int i) {
    assert stackSize >= 2;
    assert i >= 0;
    assert i == stackSize - 2 || i == stackSize - 3;

    // run1 的起始位置
    int base1 = runBase[i];
    int len1 = runLen[i];
    // run2 的起始位置
    int base2 = runBase[i + 1];
    int len2 = runLen[i + 1];
    assert len1 > 0 && len2 > 0;
    // 在数组中连续
    assert base1 + len1 == base2;

    // 修改 runLen[i] 的长度为合并后的长度
    runLen[i] = len1 + len2;
    // 若合并的是相对于栈顶 2rd 和 3rd 的 run 需要将栈顶向下移动一个单位
    // | Z |  <- top
    // | Y |
    // | X |  <- bottom
    if (i == stackSize - 3) {
        runBase[i + 1] = runBase[i + 2];
        runLen[i + 1] = runLen[i + 2];
    }
    stackSize--;

    /*
     * Find where the first element of run2 goes in run1. Prior elements
     * in run1 can be ignored (because they're already in place).
     */
    int k = gallopRight(a[base2], a, base1, len1, 0);
    assert k >= 0;
    // base1 至 base1 + k 的元素为两个 run 公共最小，不需要参与排序
    base1 += k;
    len1 -= k;
    // run1 即为公共最小，不需要再进行排序
    if (len1 == 0)
        return;

    /*
     * Find where the last element of run1 goes in run2. Subsequent elements
     * in run2 can be ignored (because they're already in place).
     */
    // base2 + len2 后的元素为两个 run 公共最大，不需要参与排序
    len2 = gallopLeft(a[base1 + len1 - 1], a, base2, len2, len2 - 1);
    assert len2 >= 0;
    // run2 即为公共最大，不需要再进行排序
    if (len2 == 0)
        return;

    // Merge remaining runs, using tmp array with min(len1, len2) elements
    if (len1 <= len2)
        mergeLo(base1, len1, base2, len2);
    else
        mergeHi(base1, len1, base2, len2);
}
```

`gallop` 是一种经过优化的二分查找，会通过倍增边界缩小二分查找的范围， `gallopLeft` 和 `gallopRight` 实现相似

```Java
private int gallopLeft(int key, int[] a, int base, int len, int hint
                                 ) {
    // key 为准备插入的元素
    // hint 为开始查找的偏移
    assert len > 0 && hint >= 0 && hint < len;
    int lastOfs = 0;
    int ofs = 1;
    // key 比 a[base + hint] 小，倍增 ofs = 1, 3, 7, 2^n-1 使得
    // key > a[base + hist - ofs ]
    // key 比 a[base + hint] 大，倍增 ofs = 1, 3, 7, 2^n-1 使得
    // key < a[base + hist + ofs ]
    if (key > a[base + hint]) {
        // Gallop right until a[base+hint+lastOfs] < key <= a[base+hint+ofs]
        int maxOfs = len - hint;
        while (ofs < maxOfs && key > a[base + hint + ofs]) {
            lastOfs = ofs;
            // 1, 3, 7, 2^n-1
            ofs = (ofs << 1) + 1;
            if (ofs <= 0)   // int overflow
                ofs = maxOfs;
        }
        if (ofs > maxOfs)
            ofs = maxOfs;

        // Make offsets relative to base
        lastOfs += hint;
        ofs += hint;
    } else { // key <= a[base + hint]
        // Gallop left until a[base+hint-ofs] < key <= a[base+hint-lastOfs]
        final int maxOfs = hint + 1;
        while (ofs < maxOfs && key <= a[base + hint - ofs]) {
            lastOfs = ofs;
            ofs = (ofs << 1) + 1;
            if (ofs <= 0)   // int overflow
                ofs = maxOfs;
        }
        if (ofs > maxOfs)
            ofs = maxOfs;

        // Make offsets relative to base
        // 负向偏移，交换顺序
        int tmp = lastOfs;
        lastOfs = hint - ofs;
        ofs = hint - tmp;
    }
    assert -1 <= lastOfs && lastOfs < ofs && ofs <= len;

    // lastofs = base + 2^(n-1)-1
    // ofs = 2^n-1
    // a[base+lastOfs] < key <= a[base+ofs], 在 base+lastOfs-1到 base+ofs 范围内执行二分查找
    // 确认 key 应当插入的位置
    lastOfs++;
    while (lastOfs < ofs) {
        int m = lastOfs + ((ofs - lastOfs) >>> 1);

        if (key>a[base + m])
            lastOfs = m + 1;  // a[base + m] < key
        else
            ofs = m;          // key <= a[base + m]
    }
    assert lastOfs == ofs;    // so a[base + ofs - 1] < key <= a[base + ofs]
    return ofs;
}
```

归并排序的实现大致可以理解为将 `run1` 移入一个临时的数组空间，然后和 `run2` 进行逐个比较，将较小的元素移入 `run1 + run2` 这个空间中  

```Java
private void mergeLo(int base1, int len1, int base2, int len2) {
    assert len1 > 0 && len2 > 0 && base1 + len1 == base2;

    // Copy first run into temp array
    int[] a = this.a; // For performance
    // 申请临时数组空间，并将 run1 复制进去
    int[] tmp = ensureCapacity(len1);
    System.arraycopy(a, base1, tmp, 0, len1);

    int cursor1 = 0; 			 // Indexes into tmp array
    int cursor2 = base2;   // Indexes int a
    int dest = base1;      // Indexes int a

    // Move first element of second run and deal with degenerate cases
    a[dest++] = a[cursor2++];
    // 若 run2 只有一个元素，将临时数组中的元素拷贝到后面即可
    if (--len2 == 0) {
        System.arraycopy(tmp, cursor1, a, dest, len1);
        return;
    }
    // 若 run1 只有一个元素，将 run2 的元素全部前移，然后添加 run1 中的元素
    if (len1 == 1) {
        System.arraycopy(a, cursor2, a, dest, len2);
        a[dest + len2] = tmp[cursor1]; // Last elt of run 1 to end of merge
        return;
    }

    // Use local variable for performance
    // minGallop = 7
    int minGallop = this.minGallop;    //  "    "       "     "      "
outer:
    while (true) {
        int count1 = 0; // Number of times in a row that first run won
        int count2 = 0; // Number of times in a row that second run won

        /*
         * Do the straightforward thing until (if ever) one run starts
         * winning consistently.
         */
        // 对 run1 和 run2 进行 merge
        do {
            assert len1 > 1 && len2 > 0;
            if (a[cursor2] < tmp[cursor1]) {
                a[dest++] = a[cursor2++];
                count2++;
                count1 = 0;
                if (--len2 == 0)
                    break outer;
            } else {
                a[dest++] = tmp[cursor1++];
                count1++;
                count2 = 0;
                if (--len1 == 1)
                    break outer;
            }
        // WTF 这个相当于 count1 < minGallop && count2 < minGallop
        // 因为 count1 或 count2 总有一个为 0
        // 如果在这里跳出说明遇到了某一个 run 中连续存在比另一个 run 的某个元素大的情况
        } while ((count1 | count2) < minGallop);

        /*
         * One run is winning so consistently that galloping may be a
         * huge win. So try that, and continue galloping until (if ever)
         * neither run appears to be winning consistently anymore.
         */
        // 再次利用 gallop 缩小范围
        do {
            assert len1 > 1 && len2 > 0;
            count1 = gallopRight(a[cursor2], tmp, cursor1, len1, 0);
            if (count1 != 0) {
                System.arraycopy(tmp, cursor1, a, dest, count1);
                dest += count1;
                cursor1 += count1;
                len1 -= count1;
                if (len1 <= 1) // len1 == 1 || len1 == 0
                    break outer;
            }
            a[dest++] = a[cursor2++];
            if (--len2 == 0)
                break outer;

            count2 = gallopLeft(tmp[cursor1], a, cursor2, len2, 0);
            if (count2 != 0) {
                System.arraycopy(a, cursor2, a, dest, count2);
                dest += count2;
                cursor2 += count2;
                len2 -= count2;
                if (len2 == 0)
                    break outer;
            }
            a[dest++] = tmp[cursor1++];
            if (--len1 == 1)
                break outer;
            minGallop--;
        } while (count1 >= MIN_GALLOP | count2 >= MIN_GALLOP);
        if (minGallop < 0)
            minGallop = 0;
        minGallop += 2;  // Penalize for leaving gallop mode
    }  // End of "outer" loop
    this.minGallop = minGallop < 1 ? 1 : minGallop;  // Write back to field

    if (len1 == 1) {
        assert len2 > 0;
        System.arraycopy(a, cursor2, a, dest, len2);
        a[dest + len2] = tmp[cursor1]; //  Last elt of run 1 to end of merge
    } else if (len1 == 0) {
        throw new IllegalArgumentException(
            "Comparison method violates its general contract!");
    } else {
        assert len2 == 0;
        assert len1 > 1;
        System.arraycopy(tmp, cursor1, a, dest, len1);
    }
}
```

补充一点在 Java 版本的 Timsort 中，如果当数组的元素小于 `MIN_MERGE`(32) 个时，会执行一个简化版本 mini-TimSort。直接找出第一个 `run` 然后将剩下的元素通过 `binarySort` 方法插入进去  

### 流程示例
*无视 mini-TimSort*  
原始待排数组  

```
[3, 6, 8, 9, 15, 13, 11, 7, 42, 58, 100, 22, 26, 39, 38, 43, 50]
```

`minRunLength` 为 9，遍历可得第一个有序部分为 `[3, 6, 8, 9, 15]`，长度小于 `minRunLength`。所以将后面的 4 个元素通过二分法找到其在有序部分的位置然后插入得到 `run1` `[3, 6, 7, 8, 9, 11, 13, 15, 42]`。入栈后检查约束条件，因为此时栈中只有一个元素，所以条件满足。之后，寻找第二个 `run` 得到 `[22, 26, 38, 39, 43, 50, 58, 100]`。入栈后条件虽然满足，但是因为已经遍历至数组尾部。所以需要执行最终的归并  

`gallopLeft` 比较 `run1` 和 `run2` 的末尾元素 42 和 100
通过倍增缩小边界
`oft = 1` 比较 42 和 58
`oft = 3` 比较 42 和 43
`oft = 7` 比较 42 和 22

确定应当在 `[22, 26, 38, 39, 43]` 中进行二分查找

`gallopRight` 同理

可得实际需要进行归并排序的范围如下 `{}` 所示

```
[3, 6, 7, 8, 9, 11, 13, 15,{ 42, 22, 26, 38, 39,} 43, 50, 58,} 100]
```

然后将 42 拷贝至 `tmp` 中，比较，归并

本文配合下面影片食用更佳  

<iframe width="560" height="315" src="https://www.youtube.com/embed/uVWGZyekGos?ecver=1" frameborder="0" allowfullscreen></iframe>
    