+++

title = "FFmpeg Picture size is invalid"
summary = ""
description = ""
categories = []
tags = ["FFmpeg",  "image"]
date = 2024-12-22T15:32:23+09:00
draft = false

+++





## High-Resolution Image



最近遇到了一个问题，FFmpeg 在处理一些分辨率特别大的图片时候会报错，错误如下

```
[IMGUTILS @ 0x7ffc82f1c7c0] Picture size 14583x29167 is invalid
[png @ 0x5f93fcc46e00] [IMGUTILS @ 0x7ffc82f1c0b0] Picture size 29167x14583 is invalid
[png @ 0x5f93fcc46e00] Invalid image size
[png_pipe @ 0x5f93fcc45ac0] Could not find codec parameters for stream 0 (Video: png, none): unspecified size
Consider increasing the value for the 'analyzeduration' (0) and 'probesize' (5000000) options
Input #0, png_pipe, from 'test.png':
  Duration: N/A, bitrate: N/A
  Stream #0:0: Video: png, none, 25 fps, 25 tbr, 25 tbn
```

这个图片的文件大小仅有 6Mb，但是分辨率高达 `14583x29167`。搜了一下这个问题相关的 Solution

- [How to fix: ffmpeg: Picture size 32768x16384 is invalid?](https://video.stackexchange.com/questions/28408/how-to-fix-ffmpeg-picture-size-32768x16384-is-invalid)

- [Largest input image size when encoding a video?](https://video.stackexchange.com/questions/21000/largest-input-image-size-when-encoding-a-video)

FFmpeg 在 `avformat_open_input` 的时候会调用 `av_image_check_size2` 进行检测

```c
//  https://github.com/FFmpeg/FFmpeg/blob/b2cba76d4fe55c944bb4277c1c1def8597c1106b/libavutil/imgutils.c#L301-L304
int av_image_check_size2(unsigned int w, unsigned int h, int64_t max_pixels, enum AVPixelFormat pix_fmt, int log_offset, void *log_ctx)
{
    ImgUtils imgutils = {
        .class      = &imgutils_class,
        .log_offset = log_offset,
        .log_ctx    = log_ctx,
    };
    int64_t stride = av_image_get_linesize(pix_fmt, w, 0);
    if (stride <= 0)
        stride = 8LL*w;
    stride += 128*8;

    if (w==0 || h==0 || w > INT32_MAX || h > INT32_MAX || stride >= INT_MAX || stride*(h + 128ULL) >= INT_MAX) {
        av_log(&imgutils, AV_LOG_ERROR, "Picture size %ux%u is invalid\n", w, h);
        return AVERROR(EINVAL);
    }

    if (max_pixels < INT64_MAX) {
        if (w*(int64_t)h > max_pixels) {
            av_log(&imgutils, AV_LOG_ERROR,
                    "Picture size %ux%u exceeds specified max pixel count %"PRId64", see the documentation if you wish to increase it\n",
                    w, h, max_pixels);
            return AVERROR(EINVAL);
        }
    }

    return 0;
}
```

这里使用的是 `INT_MAX`，对于 `int` 被定义为 4 字节的系统来说，值大小为 `2147483647`。此处是为了防止类似 decompression bomb DOS attack 对系统负载造成影响，这是一个值得去做的限制



## 各工具内存占用对比

这里简单对比几个工具在图片缩放时的内存占用



### FFmpeg

- 基于 n7.1 修改源码后

```
$ pidstat -r 1 -e ./ffmpeg -i test.png -vf "scale=1920:-1" -y output.png -loglevel quiet
Linux 6.12.4-arch1-1 (misaka)   12/22/2024      _x86_64_        (16 CPU)

07:17:29 PM   UID       PID  minflt/s  majflt/s     VSZ     RSS   %MEM  Command
07:17:30 PM  1000    268282   3812.00      0.00 1730960 1687492   5.90  ffmpeg
07:17:31 PM  1000    268282      0.00      0.00 1730960 1687492   5.90  ffmpeg
07:17:32 PM  1000    268282      0.00      0.00 1730960 1687492   5.90  ffmpeg
07:17:33 PM  1000    268282      0.00      0.00 1730960 1687492   5.90  ffmpeg
07:17:34 PM  1000    268282   1137.00      0.00 2215708 1682304   5.88  ffmpeg
07:17:35 PM  1000    268282      0.00      0.00 2215708 1682304   5.88  ffmpeg
07:17:36 PM  1000    268282      0.00      0.00 2215708 1682304   5.88  ffmpeg
07:17:37 PM  1000    268282      0.00      0.00 2215708 1682304   5.88  ffmpeg

Average:     1000    268282    618.62      0.00 1973334 1684898   5.89  ffmpeg
```



### ImageMagick

- ImageMagick 7.1.1-41

```
$ pidstat -r 1 -e magick test.png -resize 1920x output.png

Linux 6.12.4-arch1-1 (misaka)   12/22/2024      _x86_64_        (16 CPU)

07:34:55 PM   UID       PID  minflt/s  majflt/s     VSZ     RSS   %MEM  Command
07:34:56 PM  1000    339864   9986.00      0.00 11833328 7081340  24.77  magick
07:34:57 PM  1000    339864   2225.00      0.00 11833328 11499132  40.22  magick
07:34:58 PM  1000    339864    271.00      0.00 6848844 6683620  23.37  magick
07:34:59 PM  1000    339864     12.00      0.00 6848844 6684388  23.38  magick
07:35:00 PM  1000    339864     13.00      0.00 6848844 6685284  23.38  magick
07:35:01 PM  1000    339864     21.00      0.00 6848844 6686564  23.39  magick
07:35:02 PM  1000    339864     22.00      0.00 6848844 6687972  23.39  magick
07:35:03 PM  1000    339864     18.00      0.00 6848844 6689124  23.39  magick
07:35:04 PM  1000    339864    464.00      0.00 7255624 6830140  23.89  magick
07:35:05 PM  1000    339864    113.00      0.00 7255624 7061556  24.70  magick

Average:     1000    339864   1314.50      0.00 7927097 7258912  25.39  magick
```



### libvips

- vips-8.16.0

```
$ pidstat -r 1 -e vips thumbnail test.png output.png 1920

Linux 6.12.4-arch1-1 (misaka)   12/22/2024      _x86_64_        (16 CPU)

07:44:37 PM   UID       PID  minflt/s  majflt/s     VSZ     RSS   %MEM  Command
07:44:38 PM  1000    383594  72104.00      0.00 1380940  337740   1.18  vips
07:44:39 PM  1000    383594  17521.00      0.00 1460056  360072   1.26  vips
07:44:40 PM  1000    383594  23021.00      0.00 1530896  473988   1.66  vips
07:44:41 PM  1000    383594  17532.00      0.00 1595468  555320   1.94  vips
07:44:42 PM  1000    383594  21905.00      0.00 1661004  669348   2.34  vips
07:44:43 PM  1000    383594   9972.00      0.00 1660040  708328   2.48  vips
07:44:44 PM  1000    383594   5987.00      0.00 1660040  732136   2.56  vips
07:44:45 PM  1000    383594   9049.00      0.00 1725576  780520   2.73  vips

Average:     1000    383594  22136.38      0.00 1584252  577182   2.02  vips
```



可以看到占用最少的 libvips 依然使用了 563Mb。不太建议直接处理这种类型的图片，最好直接前置拦截
