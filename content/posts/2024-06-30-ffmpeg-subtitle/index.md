+++
title = "FFmpeg 封装字幕"
summary = ""
description = ""
categories = []
tags = ["FFmpeg"]
date = 2024-06-30T12:01:08+09:00
draft = false

+++



视频示例使用 https://www.h264info.com/clips.html 中的 “The Simpsons Movie – 720p Trailer”，下载后重命名为 input.mp4，然后截取部分片段如下

<video src="./input.mp4" controls="" width="640"></video>



用到的两个字幕文件

- [chs.srt](./chs.srt)

- [eng.srt](./eng.srt)



## 内嵌字幕

通过 `subtitiles` 滤镜可以做到这一点



```shell
$ ffmpeg -i input.mp4 -vf "subtitles=eng.srt" -c:a copy -y hard-embed.mp4
```



参数文档 https://ffmpeg.org/ffmpeg-filters.html#subtitles-1。值得注意的是，对于字体的处理并不像 `drawtext` 滤镜那样指定 `fontfile` ，而是需要在 `force_style` 中指定。比如



```shell
$ ffmpeg -i input.mp4 -vf "subtitles=eng.srt:fontsdir=/usr/share/fonts/TTF/:force_style='Fontname=Hack,PrimaryColour=&HCCFF0000'" -c:a copy -y hard-embed.mp4
```



这里有一个坑就是 `fontsdir` 的路径必须有尾部斜线的。 `/usr/share/fonts/TTF/` 是可以的，但是 `/usr/share/fonts/TTF` 则不行



如果找不到字体，FFmpeg 会自动找一个，你会发现如下的日志

```
[Parsed_subtitles_0 @ 0x788790003cc0] fontselect: (Hack, 400, 0) -> /home/kumiko/.local/share/fonts/panels/noto_sans.ttf, 0, NotoSans-Regular
```



如果字体被正确使用，日志中则是这样的

```
[Parsed_subtitles_0 @ 0x77952c003e40] fontselect: (Hack, 400, 0) -> /usr/share/fonts/TTF/Hack-Regular.ttf, 0, Hack-Regular
```





生成视频效果如下

<video src="./hard-embed.mp4" controls="" width="640"></video>







## 内封字幕

在这里我们需要做的是将字幕文件打包到视频容器中，这种速度相比内嵌字幕快非常多，因为不需要解码重新编码。



```shell
$ ffmpeg -i input.mp4 -i chs.srt -i eng.srt -c:v copy -c:a copy -c:s mov_text -metadata:s:s:0 language=chi -metadata:s:s:1 language=eng -map 0 -map 1 -map 2 -shortest -y soft-embed.mp4
```



在这条命令中：

- `-c:s mov_text`: 将字幕流编码为 `mov_text` 格式，主要用于 MP4 和 MOV 容器

- `-metadata:s:s:0 language=chi`: 设置第一个字幕流的语言为中文

- `-metadata:s:s:1 language=eng`: 设置第二个字幕流的语言为英文

  

<video src="./soft-embed.mp4" controls="" width="640"></video>





当我们通过视频播放器(VLC) 播放视频的时候会发现，字幕并没有被默认选中。搜索了一番，大部分都是给的 `-disposition:s:s:1 default+forced` 这种解答

- [ffmpeg set subtitles track as default](https://stackoverflow.com/questions/26956762/ffmpeg-set-subtitles-track-as-default)
- [How to force & by default subtitles FFmpeg](https://stackoverflow.com/questions/73772765/how-to-force-by-default-subtitles-ffmpeg)
- [How can I force a subtitle track to be set as active on an MP4 video?](https://superuser.com/questions/1720387/how-can-i-force-a-subtitle-track-to-be-set-as-active-on-an-mp4-video)



命令如下

```shell
$ ffmpeg -i input.mp4 -i chs.srt -i eng.srt -c:v copy -c:a copy -c:s mov_text -metadata:s:s:0 language=chi -metadata:s:s:1 language=eng -disposition:s:s:1 default+forced -map 0 -map 1 -map 2 -shortest -y soft-embed.mp4
```



试了一下，依然不行。但是封装到 MKV 格式，即使不添加 `-disposition:s:s:1 default+forced` 也是会有默认字幕的。默认的是 subtitle track 的第一个。这应该和播放器行为有关系



```shell
$ ffmpeg -i input.mp4 -i chs.srt -i eng.srt -c:v copy -c:a copy -c:s ass -metadata:s:s:0 language=chi -metadata:s:s:1 language=eng -map 0 -map 1 -map 2 -shortest -y soft-embed.mkv
```



注意这里不能用 `mov_text` 了，需要使用 `ass`



MKV 格式下指定默认字幕，用 Stack Overflow  的答案就可以

```shell
$ ffmpeg -i input.mp4 -i chs.srt -i eng.srt -c:v copy -c:a copy -c:s ass -metadata:s:s:0 language=chi -metadata:s:s:1 language=eng -disposition:s:s:1 default -map 0 -map 1 -map 2 -shortest -y soft-embed.mkv
```





## 提取字幕



获取视频的字幕信息可以通过 `ffmpeg -i` 命令。比如刚才我们内封了两个字幕的视频，它的输出如下

```
Input #0, mov,mp4,m4a,3gp,3g2,mj2, from 'soft-embed.mp4':
  Metadata:
    major_brand     : isom
    minor_version   : 512
    compatible_brands: isomiso2avc1mp41
    encoder         : Lavf61.1.100
  Duration: 00:00:24.07, start: 0.000000, bitrate: 493 kb/s
  Stream #0:0[0x1](und): Video: h264 (High) (avc1 / 0x31637661), yuv420p(progressive), 640x272, 357 kb/s, 23.98 fps, 23.98 tbr, 24k tbn (default)
      Metadata:
        handler_name    : GPAC ISO Video Handler
        vendor_id       : [0][0][0][0]
        encoder         : Lavc61.3.100 libx264
  Stream #0:1[0x2](und): Audio: aac (LC) (mp4a / 0x6134706D), 48000 Hz, stereo, fltp, 129 kb/s (default)
      Metadata:
        handler_name    : GPAC ISO Audio Handler
        vendor_id       : [0][0][0][0]
  Stream #0:2[0x3](chi): Subtitle: mov_text (tx3g / 0x67337874), 0 kb/s (default)
      Metadata:
        handler_name    : SubtitleHandler
  Stream #0:3[0x4](eng): Subtitle: mov_text (tx3g / 0x67337874), 0 kb/s
      Metadata:
        handler_name    : SubtitleHandler
```



可以通过 `map` 直接提取出来内封过字幕文件

```shell
$ ffmpeg -i soft-embed.mp4 -map 0:s:0 t0.ass  # 第 0 个输入的 s[ubtitle] 的第一个，即我们的 chi 字幕
$ ffmpeg -i soft-embed.mp4 -map 0:s:1 t1.ass  # 第 0 个输入的 s[ubtitle] 的第二个，即我们的 eng 字幕
```



对于字幕文件我们也是可以直接转换的。 `ass` 格式包含了很多 `srt` 不支持的高级操作，所以这种转换是带有信息量损失的

```shell
$ ffmpeg -i t0.ass t0.srt
```





## Reference

https://trac.ffmpeg.org/wiki/HowToBurnSubtitlesIntoVideo

