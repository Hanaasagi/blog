

+++
title = "通过 FFmpeg 实现关键帧动画的一些尝试"
summary = ''
description = ""
categories = []
tags = []
date = 2024-05-19T12:43:34+09:00
draft = false

+++







## 本文环境



- ffmpeg version n6.1.1 Copyright (c) 2000-2023 the FFmpeg developers

- 原始图片为 579x250 的 PNG 素材







![](./input.png)







## 生成静止动画



首先我们生成一段 10 秒的静止动画

<video src="./seishiga.mp4" controls="controls" width="1280" height="720"></video>

命令如下

```shell
ffmpeg -r 60 -loop 1 -t 10 -i input.png \
-vf "pad=ceil(iw/2)*2:ceil(ih/2)*2" \
-pix_fmt yuva420p \
-y output.mp4
```



这里讲一下基本的参数，重复的参数，后面就不会提及了。

**`-r 60`**:

- 设置输出视频的帧率为 60 fps。

**`-loop 1`**:

- 将输入图片循环播放。

**`-t 10`**:

- 设置输出视频的时长为 10 秒。

**`-i input.png`**:

- 指定输入文件为 `input.png`。



因为 libx264 要求视频的宽度和高度为偶数，如果我们不重新编辑宽和高，那么会有以下的报错

```
[libx264 @ 0x6094398d0e40] width not divisible by 2 (579x250)
[vost#0:0/libx264 @ 0x6094398d0a80] Error while opening encoder - maybe incorrect parameters such as bit_rate, rate, width or height.
Error while filtering: Generic error in an external library
[out#0/mp4 @ 0x6094398cfac0] Nothing was written into output file, because at least one of its streams received no packets.
```

参考 https://stackoverflow.com/questions/20847674/ffmpeg-libx264-height-not-divisible-by-2



**`-vf "pad=ceil(iw/2)\*2:ceil(ih/2)\*2"`**:

- 使用 `vf` 参数进行视频过滤，宽度和高度向上取整到最近的偶数。

**`-pix_fmt yuva420p`**:

- 将输出视频的像素格式设置为 `yuva420p`，这种格式支持透明度（Alpha 通道）。





## 平移效果

平移可以通过 `overlay` 叠加来做，或者使用 `crop` 在大画布上移动裁剪



### 基础平移

假设起始帧是左上，终止帧是右下。我们描绘一个向右下角的平移过程

<video src="./move.mp4" controls="controls" width="1280" height="720"></video>

实现命令

```shell
ffmpeg -r 60 -loop 1 -t 10 -i input.png \
-filter_complex "\
color=green:s=1280x720, fps=fps=60[bg]; \
[bg][0:v]overlay=y='t*72':x='t*128':shortest=1[video] \
" \
-preset ultrafast -map "[video]" -y output.mp4
```



通过 `color=green:s=1280x720, fps=fps=60[bg];` 创建一个 1280x720 分辨率的绿色背景视频，帧率为 60 fps，并将其标记为 `[bg]`。然后将输入图片叠加到绿色背景上，随着时间 t 以每秒 72 像素的速度在 y 轴上移动，以每秒 128 像素的速度在 x 轴上移动，并将输出标记为 `[video]`。`shortest=1` 参数确保输出视频长度与最短输入对齐





### 循环平移

假设起始帧是左上，中间帧左下，终止帧回到左上

<video src="./shift.mp4" controls="controls" width="1280" height="720"></video>

实现命令如下

```shell
ffmpeg -r 60 -loop 1 -t 10 -i input.png \
-filter_complex "\
color=green:s=1280x720, fps=fps=60[bg]; \
[bg][0:v]overlay=y='if(lt(t,5),t*72*2,(t-5)*72*2)':shortest=1[video] \
" \
-preset ultrafast -map "[video]" -y output.mp4
```



叠加的位置在 y 轴上的移动是由时间 `t` 控制的。使用 `if(lt(t,5),t*72*2,(t-5)*72*2)` 表达式：

- `if(lt(t,5),t*72*2,(t-5)*72*2)`：如果 `t` 小于 5 秒，则 `y` 轴的位置为 `t*72*2`；如果 `t` 大于或等于 5 秒，则 `y` 轴的位置为 `(t-5)*72*2`。





#### 简谐运动

研究了一下，自定义弧线运动无法支持。因为 FFmpeg 的这边都是通过公式去做，用户在画布上的轨道无法很好拟合成一个公式。这里用简谐运动做个示例，看一下就好



<video src="./shm.mp4" controls="controls" width="1280" height="720"></video>

实现命令如下

```shell
ffmpeg -r 60 -loop 1 -t 10 -i input.png \
-filter_complex "\
color=green:s=1280x720, fps=fps=60[bg]; \
[bg][0:v]overlay=y='360 - cos(2*PI*2*t/5)*360': \
x='360 - cos(2*PI*2*t/10)*360': \
shortest=1[video] \
" \
-preset ultrafast -map "[video]" -y output.mp4
```





### 移动的另一种方式



<video src="./crop.mp4" controls="controls" width="1280" height="720"></video>

实现命令

```shell
ffmpeg -r 60 -loop 1 -t 10 -i input.png \
-filter_complex " \
[0:v]crop=ih:ih:iw-ih-(iw-ih)*t/20:0,trim=duration=10,scale=-1:720 \
" \
-c:a copy -pix_fmt yuv420p \
-y output.mp4
```







## 旋转



对应文档 https://ffmpeg.org/ffmpeg-filters.html#rotate



### 原地旋转

<video src="./rotate-problem.mp4" controls="controls" width="1280" height="720"></video>

实现命令

```shell
ffmpeg -r 60 -loop 1 -t 10 -i input.png \
-filter_complex "\
[0:v]rotate=PI/6*t:c=none:ow=1280:oh=720, \
scale=1280:720, \
format=yuva420p[video] \
" \
-preset ultrafast \
-map "[video]" \
-y output.mp4

```



`rotate=PI/6*t`:

- 以每秒 `PI/6` 弧度（即 30 度）旋转图像。`t` 表示时间。

`ow=1280:oh=720`:

- 输出视频的宽度和高度分别设置为 1280 和 720 像素。

`scale=1280:720`:

- 将输出视频的分辨率设置为 1280x720 像素。

  

我们可以发现，出现了残影般的轨迹。因为 `c=none` 是不绘制背景，所以我们没有重新绘制因为运动产生的这些白色轨迹， 参考 https://superuser.com/questions/1393105/rotate-jpeg-in-ffmpeg-with-transparent-color。



解法是 `black@0`

<video src="./rotate.mp4" controls="controls" width="1280" height="720"></video>

实现命令

```shell
ffmpeg -r 60 -loop 1 -t 10 -i input.png \
-filter_complex "\
[0:v]rotate=PI/6*t:c=black@0:ow=1280:oh=720, \
scale=1280:720, \
format=yuva420p[video] \
" \
-preset ultrafast \
-map "[video]" \
-y output.mp4
```







### 平移加旋转



<video src="./rotate-2.mp4" controls="controls" width="1280" height="720"></video>

实现命令

```shell
ffmpeg -r 60 -loop 1 -t 10 -i input.png \
-filter_complex "\
color=green:s=1280x720, fps=fps=60[bg]; \
[0:v]rotate=PI/6*t:c=black@0:ow=1280:oh=720,scale=1280:720[rotate]; \
[bg][rotate]overlay=x='t*128':y='t*72':shortest=1[video] \
" \
-preset ultrafast \
-map '[video]' \
-y output.mp4

```





我们简单的将上面的两步组合到了一起，但是产生了一个问题。我们第一帧，图片并不在左上角。因为我们的旋转部分是在一个 1280x720 的区间进行的，所以 `overlay` 叠加的时候便在中间了





但是如果我们保持原始图片的大小作为生成视频大小进行旋转



<video src="./rotate-1.mp4" controls="controls" width="1280" height="720"></video>

实现命令

```shell
ffmpeg -r 60 -loop 1 -t 10 -i input.png \
-filter_complex "\
[0:v]pad=ceil(iw/2)*2:ceil(ih/2)*2[pad];
[pad]rotate=PI/6*t:c=black@0,format=yuva420p[video]; \
" \
-preset ultrafast \
-map "[video]" \
-y output.mp4

```





我们可以发现，图片的一部分在视频的外面了。因为一个圆周的运动对于矩形图片来说，直径应该是对角线。我们在 FFmpeg 中无法自定义圆心，只能通过视频的大小来让 FFmpeg 决定，所以这里推导参数的时候就比较麻烦



我们需要减掉宽和高的一部分，比如





<video src="./move-rotate.mp4" controls="controls" width="1280" height="720"></video>

实现命令

```shell
ffmpeg -r 60 -loop 1 -t 10 -i input.png \
-filter_complex "\
[0:v]pad=ceil(iw/2)*2:ceil(ih/2)*2[pad];
[pad]rotate=PI/6*t:c=black@0:ow='hypot(iw,ih)':oh=ow,format=yuva420p[rotate]; \
color=green:s=1280x720, fps=fps=60[bg]; \
[bg][rotate]overlay=x='t*128-(632/2)+290':y='t*72-(632/2)+125':shortest=1[video] \
" \
-preset ultrafast \
-map '[video]' \
-y output.mp4
```



`632` 这里是 `hypot(iw,ih)` 的值



## 缩放



对应文档参考 https://ffmpeg.org/ffmpeg-filters.html#zoompan



#### 中心放大

<video src="./zoom.mp4" controls="controls" width="1280" height="720"></video>

实现命令

```shell
ffmpeg -r 60 -loop 1 -t 10 -i input.png \
-filter_complex "\
[0:v]scale=1280:720, \
zoompan=z='zoom+0.001':x=iw/2-(iw/zoom/2):y=ih/2-(ih/zoom/2):d=10*60:s=1280x720:fps=60, \
format=yuva420p[video]\
" \
-t 10 \
-preset ultrafast \
-map "[video]" \
-y output.mp4
```





`z='zoom+0.001'`:

- 每一帧的缩放比例增加 0.001，实现逐渐放大的效果。

`x=iw/2-(iw/zoom/2)` 和 `y=ih/2-(ih/zoom/2)`:

- 设置每一帧的平移中心为输入图像的中心。



直接这样会会产生抖动，解决方法参考 https://superuser.com/questions/1112617/ffmpeg-smooth-zoompan-with-no-jiggle





### 左上顶点 Zoom



<video src="./zoom-left-top.mp4" controls="controls" width="1280" height="720"></video>



实现命令

```shell
ffmpeg -r 60 -loop 1 -t 10 -i input.png \
-filter_complex "\
[0:v]scale=1280:720, \
zoompan=z='pzoom+0.003':d=1:s=1280x720:fps=60, \
format=yuva420p[video]\
" \
-t 10 \
-preset ultrafast \
-map "[video]" \
-y output.mp4
```





### 平移旋转放大



<video src="./zoom-move-rotate.mp4" controls="controls" width="1280" height="720"></video>

实现命令

```shell
ffmpeg -r 60 -loop 1 -t 10 -i input.png \
-filter_complex "\
[0:v]pad=ceil(iw/2)*2:ceil(ih/2)*2[pad];
[pad]rotate=PI/6*t:c=black@0:ow='hypot(iw,ih)':oh=ow,format=yuva420p[rotate]; \
color=green:s=1280x720, fps=fps=60[bg]; \
[bg][rotate]overlay=x='t*128-(632/2)+290':y='t*72-(632/2)+125':shortest=1[v1]; \
[v1]scale=3840:2160, \
zoompan=z='pzoom+0.001':x=iw/2-(iw/zoom/2):y=ih/2-(ih/zoom/2):d=1:s=1280x720:fps=60, \
format=yuva420p[video]\
" \
-t 10 \
-preset ultrafast \
-map "[video]" \
-y output.mp4
```





## 淡入淡出



<video src="./fade.mp4" controls="controls" width="1280" height="720"></video>



实现命令如下

```shell
ffmpeg -r 60 -loop 1 -t 10 -i input.png \
-filter_complex "\
[0:v]pad=ceil(iw/2)*2:ceil(ih/2)*2[pad]; \
[pad]rotate=PI/6*t:c=black@0:ow='hypot(iw,ih)':oh=ow,format=yuva420p[rotate]; \
color=green:s=1280x720[bg]; \
[bg][rotate]overlay=x='t*128-(632/2)+290':y='t*72-(632/2)+125':shortest=1[v1]; \
[v1]scale=3840:2160, \
zoompan=z='pzoom+0.001':x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':d=1:s=1280x720:fps=60[v2]; \
[v2]fade=in:st=0:d=2[video] \
" \
-t 10 \
-preset ultrafast \
-map "[video]" \
-y output.mp4

```





这边的问题点在于 `zoompan` 调节的是视角，视角拉近就是放大，参数需要推导一下。



## 结论

如果你现在的需求是临时糊一个视频关键帧的功能，比如前端 canvas 预览，后端渲染生成视频。那么直接使用 FFmpeg 的命令行来做这种比较麻烦。在 FFmpeg 中，我们无法直接通过描述起始和终止两个帧，然后自动补上中间的动画，需要推理中间的过程。另一种暴力的方法是采用 [Video Rendering with Node.js and FFmpeg](https://creatomate.com/blog/video-rendering-with-nodejs-and-ffmpeg) 中的方式，前端后端统一一套 canvas 的标准，后端直接通过图片拼接成视频