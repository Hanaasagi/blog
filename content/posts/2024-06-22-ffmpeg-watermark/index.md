+++
title = "FFmpeg 视频水印实战"
summary = ""
description = ""
categories = []
tags = ["FFmpeg"]
date = 2024-06-22T10:17:02+09:00
draft = false

+++





## 环境准备

视频示例使用 https://www.h264info.com/clips.html 中的 “The Simpsons Movie – 720p Trailer”，下载后重命名为 input.mp4，然后截取部分片段如下



<video src="./input.mp4" controls="" width="640"></video>





## 静态水印



固定文本的水印可以使用 `drawtext` 滤镜完成，文档参考 https://ffmpeg.org/ffmpeg-filters.html#drawtext-1



```shell
$ ffmpeg -t 5 -i input.mp4 -vf "drawtext=text='©Hanaasagi':fontsize=20:fontcolor=white:x=25:y=25:fontfile=/usr/share/fonts/TTF/Hack-Bold.ttf" -codec:a copy -y plain-text.mp4
```



`fontsize=20`:

- 字体大小设为 20 像素

`fontcolor=white`:

- 颜色为白色。

`x=25,y=25`:

- 左顶点偏移量

`fontfile=/usr/share/fonts/TTF/Hack-Bold.ttf`:

- 指定了字体文件的路径



生成视频效果如下

<video src="./plain-text.mp4" controls="" width="640"></video>





我们也可以为文字添加底色。在 `drawtext` 滤镜中，我们可依次调整 box, boxborder, font, shadow，四类区域的颜色，其中 box 也可以有自己的位置偏移量，让我们的绘制区域更富有层次感

```shell
$ ffmpeg -t 3 -i input.mp4 -vf "drawtext=text='©Hanaasagi':fontsize=20:fontcolor=#00BFFF:x=25:y=25:box=1:boxcolor=white@1:boxborderw=5:borderw=2:bordercolor=#FFB6C1" -codec:a copy -y plain-text-with-bg.mp4
```

`box=1`:

- 启用文本背景框

`boxcolor=white@1`:

- 背景框为白色，并且完全不透明

`boxborderw=5`:

- 文本框的边框宽度为 5 像素

`borderw=2`:

- 文字的边框宽度为 2 像素



<video src="./plain-text-with-bg.mp4" controls="" width="640"></video>





除了固定的文本，也可以使用一些内建的变量，比如 `timestamp` 来显示时间轴信息

```shell
$ ffmpeg -t 5 -i input.mp4 -vf "drawtext=text='timestamp: %{pts \: hms}':fontsize=35:fontcolor=white:x=w-250:y=h-50" -codec:a copy -y timestamp.mp4
```



<video src="./timestamp.mp4" controls="" width="640"></video>





或者直接加载一个文件，达到一种转场谢幕的效果

```shell
$ cat << EOF > poem.txt
Shall I compare thee to a summer's day?
Thou art more lovely and more temperate:
Rough winds do shake the darling buds of May,
And summer's lease hath all too short a date:
Sometime too hot the eye of heaven shines,
And often is his gold complexion dimmed;
And every fair from fair sometime declines,
By chance or nature's changing course untrimmed;
But thy eternal summer shall not fade
Nor lose possession of that fair thou owest;
Nor shall Death brag thou wanderest in his shade,
When in eternal lines to time thou growest:
So long as men can breathe or eyes can see,
So long lives this, and this gives life to thee.
EOF

$ ffmpeg -t 5 -i input.mp4 -vf "drawtext=textfile=poem.txt:fontsize=20:fontcolor=white:x=25:y=h-80*t" -codec:a copy -y credits.mp4
```





<video src="./credits.mp4" controls="" width="640"></video>





## 动态水印



### 自定义时间间隔



如果想要做到水印只在固定的几分钟内显示，达到时隐时现的效果。可以借助 `enable` 加上一个表达式，决定滤镜的生效时间。比如视频中每隔 2 秒显示一次水印，水印持续时间为 1 秒



```bash
$ ffmpeg -i input.mp4 -vf "drawtext=text='©Hanaasagi':fontsize=20:fontcolor=white:x=25:y=25:enable='lt(mod(t,3),1)'" -codec:a copy -y custom-interval.mp4
```



表达式的部分可以参考 https://ffmpeg.org//ffmpeg-utils.html#Expression-Evaluation



生成的效果如下

<video src="./custom-interval.mp4" controls="" width="640"></video>



### 位置变化

和 `overlay` 滤镜的用法一样，通过在 `x` 和 `y` 中使用复杂的表达式，借助 `t` 实现一元函数就可以了

```shell
$ ffmpeg -i input.mp4 -vf "drawtext=text='©Hanaasagi':fontsize=20:fontcolor=white:x='if(lt(mod(t\,8)\,4)\,25 + (w-tw-50)*(mod(t\,4)/4)\,25 + (w-tw-50)*(1-(mod(t\,4)/4)))':y=25" -codec:a copy -y translation.mp4

```







<video src="./translation.mp4" controls="" width="640"></video>







### GIF 水印



FFmpeg 默认提供的动态效果有限，而且会造成命令拼接的比较复杂。我们可以直接通过 GIF 来预先生成一段水印的文本，然后通过 `overlay` 直接叠加在视频上面



![](./text.gif)



```shell
$ ffmpeg -i input.mp4 -ignore_loop 0 -i text.gif -filter_complex "[1:v]scale=120:-1[v1];[0:v][v1]overlay=0:0:shortest=1:x=25:y=25" -c:v libx264 -c:a copy -pix_fmt yuv420p -y overlay.mp4

```





<video src="./overlay.mp4" controls="" width="640"></video>



叠加视频有多少种玩法，这个就有多少种了
