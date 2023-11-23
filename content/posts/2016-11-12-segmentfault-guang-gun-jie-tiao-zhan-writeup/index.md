
+++
title = "SegmentFault 光棍节挑战writeup"
summary = ''
description = ""
categories = []
tags = []
date = 2016-11-12T10:33:49+08:00
draft = false
+++

第一关  
  
查看 html 源码可以看到还有一个 a 标签被css隐藏了  

![](../../images/2016/11/c4ca4238a0b923820dcc509a6f75849b.png) 

第二关  

还是在 html 源码中  

![](../../images/2016/11/c81e728d9d4c2f636f067f89cc14862c.png)

第三关  

key 在 响应头中

![](../../images/2016/11/eccbc87e4b5ce2fe28308fd9f2a7baf3.png)

第四关  

提示是 观察密码的规律  

https://1111.segmentfault.com/?k=a87ff679a2f3e71d9181a67b7542122c

k 是32位的，应该是 md5

![](../../images/2016/11/a87ff679a2f3e71d9181a67b7542122c.png) 

那么第五关的话，应当为 5 的 md5 值 e4da3b7fbbce2345d7772b0674a318d5


第五关  

![](../../images/2016/11/0299c06aed970473ae41d986b308cd09.png) 

扫描后，进入到 sf 的官网，应该不是这样的。

我们将图片保存，查看 hex 得到 key

![](../../images/2016/11/17380ddb842e984302034e1bb66c24e4.png) 

第六关  

![](../../images/2016/11/1679091c5a880faf6fb5e6087eb1b2dc.png) 

你只能告诉我这么多，但 Google 可以告诉我更多  

![](../../images/2016/11/fae594628f003e7d8250252baa6a83b2.png) 

![](../../images/2016/11/a5cb00d7c8fffe5fb2c79c540a54817a.png) 

看评论发现，原来n年前就有了，好像这个光棍节挑战 n 年没变过了

第七关  

![](../../images/2016/11/8f14e45fceea167a5a36dedd4bea2543.png) 

上一题是 Google 到的，这一题也 Google ？

直接 k=a75f7e012f4da3efff4d3700cfa9c54c 访问就行了  

第八关  

![](../../images/2016/11/9634715ca7e046cdd0fc857cdc38dcb6.png) 

没写提交按钮，提交方式是 GET

![](../../images/2016/11/1981e4a762b39858dc33f9ea28ed065a.png) 

如果是 GET 就不用大费周章了，因为我们可以直接将 value 的值放到url 中访问，改成 POST 试一下

![](../../images/2016/11/f2a057fc73359a2781f0fd48f63d6fde.png)

第九关  

这关有点难

![](../../images/2016/11/b00bdaf8d970b7df664953f63a698374.png)

这么整齐，8位8位的分开，看起来像是二进制。横线是未知的4位

瞄了一眼域名`https://1111.segmentfault.com` 说不定是 1111

替换后每8位转换成ascii表对应的字符，得到了base64加密的字符串(尾部有=)

解密后的内容，一堆乱码。

看了看头几个字节，好像见过

查了一下是 tar.gz 的文件头

![](../../images/2016/11/435c44c266bc0c05f7b6f48e7a454f1c.png)

打开后，得到苍老师

![](../../images/2016/11/a4385fda98a439aede464b18924abaea.png) 

![](../../images/2016/11/d3d9446802a44259755d38e6d163e820.png) 

一共不应该十关么？
    