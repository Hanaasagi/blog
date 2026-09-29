+++
title = "soffice 转换 PPT 遇到的几个问题"
summary = ""
description = ""
categories = []
tags = [""]
date = 2026-09-16T18:17:29+09:00
draft = false

+++

记录两个最近遇到的问题

## 环境信息

- LibreOffice 26.8.0.3 680(Build:3)

## 问题一

遇到了一个 `.pptx` 文件，使用 `soffice` 转换成 `.pdf` 无法成功。假设文件是 `badcase.pptx`，执行脚本

```Bash
#!/bin/bash

set -euo pipefail

WORK_DIR=${PWD}
PROFILE=$(mktemp -d /tmp/soffice_user_XXXXXX)

trap 'rm -rf "$PROFILE"' EXIT

soffice \
    -env:UserInstallation="file://${PROFILE}" \
    --headless \
    --convert-to pdf \
    --outdir "$WORK_DIR" \
    "$WORK_DIR/badcase.pptx"

```

`soffice` 的输出是 `Error: source file could not be loaded`。这个表明 `soffice` 在载入原始文件的时候遇到了问题。


我们使用 `unzip -t badcase.pptx` 来校验 `.pptx` 文件是否完整，得到了 `No errors detected in compressed data of badcase.pptx.` ，这说明文件本身没有损坏。但是通过 `unzip -l badcase.pptx` 显示文件列表发现了

```
Archive:  badcase.pptx
  Length      Date    Time    Name
---------  ---------- -----   ----
        0  2026-09-11 17:03   ppt/
     3676  2026-09-11 17:03   ppt/presentation.xml
        0  2026-09-11 17:03   ppt\slideMasters/
     7850  2026-09-11 17:03   ppt/slideMasters/slideMaster1.xml
        0  2026-09-11 17:03   ppt\slideLayouts/
      587  2026-09-11 17:03   ppt/slideLayouts/slideLayout1.xml
        0  2026-09-11 17:03   ppt\theme/
     8078  2026-09-11 17:03   ppt/theme/theme1.xml
        0  2026-09-11 17:03   ppt\slides/
    12957  2026-09-11 17:03   ppt/slides/slide1.xml
     6974  2026-09-11 17:03   ppt/slides/slide10.xml
    40790  2026-09-11 17:03   ppt/slides/slide11.xml
        0  2026-09-11 17:03   ppt\slides\media/
```

可以发现这里 `/` 和 `\` 混用的。查看解压后的 `docProps/app.xml` 发现是腾讯文档生成的

```
$ cat docProps/app.xml
<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes"><Application>Tencent office</Application></Properties>
```


对于 `.zip` 文件格式来说，明确规定 `file name` 中的目录分隔符**必须是正斜杠 `/`**。可以参考 https://pkware.cachefly.net/webdocs/casestudies/APPNOTE.TXT

```
   4.4.17 file name: (Variable)

       4.4.17.1 The name of the file, with optional relative path.
       The path stored MUST NOT contain a drive or
       device letter, or a leading slash.  All slashes
       MUST be forward slashes '/' as opposed to
       backwards slashes '\' for compatibility with Amiga
       and UNIX file systems etc.  If input came from standard
       input, there is no file name field.  

       4.4.17.2 If using the Central Directory Encryption Feature and 
       general purpose bit flag 13 is set indicating masking, the file 
       name stored in the Local Header will not be the actual file name.  
       A masking value consisting of a unique hexadecimal value will 
       be stored.  This value will be sequentially incremented for each 
       file in the archive. See the section on the Strong Encryption 
       Specification for details on retrieving the encrypted file name. 
       Refer to the section in this document entitled "Incorporating PKWARE 
       Proprietary Technology into Your Product" for more information.
```


解决方法是把 `.pptx` 重新打包一次，可以通过 Python 脚本

```Python
import zipfile

zin = zipfile.ZipFile("badcase.pptx")
with zipfile.ZipFile("repacked.pptx", "w", zipfile.ZIP_DEFLATED) as zout:
    for info in zin.infolist():
        name = info.filename
        if name.endswith(("/", "\\")):  # 丢弃所有目录条目
            continue
        zout.writestr(name.replace("\\", "/"), zin.open(info).read())
```



----


## 问题二

这个问题依然是 `Error: source file could not be loaded` 报错，但是问题原因不同。我们通过 `soffice` 增加 `--infilter="Impress MS PowerPoint 2007 XML"` 参数也无法进行转换。`--infilter` 只能解决"过滤器自动识别"的问题;连它都失败,说明错误发生在更早的阶段


根据 `docProps/app.xml`，这是一个 WPS 导出的文件。我们先通过脚本对于 `.pptx` 文件进行校验

```Python
import collections, zipfile

z = zipfile.ZipFile("badcase2.pptx")
infos = z.infolist()
names = [i.filename for i in infos]
print("总条目数:", len(names), " 去重后:", len(set(names)))
for label, bad in [
    ("前导斜杠", [n for n in names if n.startswith("/")]),
    ("含反斜杠", [n for n in names if "\\" in n]),
    ("含盘符", [n for n in names if len(n) > 1 and n[1] == ":"]),
    ("重名条目", [n for n, c in collections.Counter(names).items() if c > 1]),
    (
        "空文件条目(0字节)",
        [
            i.filename
            for i in infos
            if i.file_size == 0 and not i.filename.endswith("/")
        ],
    ),
]:
    print(f"{label}: {len(bad)}", bad[:8])
methods = collections.Counter(i.compress_type for i in infos)
print("压缩方式分布:", dict(methods), "(0=store 8=deflate 12=bzip2 14=lzma)")

```

得到

```
总条目数: 1748  去重后: 1748
前导斜杠: 0 []
含反斜杠: 0 []
含盘符: 0 []
重名条目: 0 []
空文件条目(0字节): 0 []
压缩方式分布: {8: 1729, 0: 19}   ← 全部标准 deflate/store
```

可以得出容器层是干净的

然后我们看 OPC 信息

```Python
import xml.dom.minidom, zipfile

z = zipfile.ZipFile("badcase2.pptx")
for part in ["[Content_Types].xml", "_rels/.rels", "ppt/presentation.xml"]:
    try:
        data = z.read(part)
        status = f"{len(data)} bytes"
        try:
            xml.dom.minidom.parseString(data)
            status += ", XML 合法"
        except Exception as e:
            status += f", !!! XML 不合法: {e}"
    except KeyError:
        status = "!!! 不存在"
    print(f"{part}: {status}")

import re

ct = z.read("[Content_Types].xml").decode("utf-8", "replace")
print(
    "presentation 相关 Override:",
    re.findall(r'PartName="([^"]+)"[^>]*ContentType="([^"]*presentation[^"]*)"', ct)[
        :5
    ],
)
print("--- _rels/.rels 全文 ---")
print(z.read("_rels/.rels").decode("utf-8", "replace")[:800])
print("--- presentation.xml 前 400 字节(看命名空间,purl.oclc.org = Strict OOXML) ---")
print(z.read("ppt/presentation.xml").decode("utf-8", "replace")[:400])
```

得到

```XML
[Content_Types].xml: 209374 bytes, XML 合法
_rels/.rels: 740 bytes, XML 合法
ppt/presentation.xml: 7967 bytes, XML 合法
presentation 相关 Override: [('/ppt/commentAuthors.xml', 'application/vnd.openxmlformats-officedocument.presentationml.commentAuthors+xml'), ('/ppt/handoutMasters/handoutMaster1.xml', 'application/vnd.openxmlformats-officedocument.presentationml.handoutMaster+xml'), ('/ppt/notesMasters/notesMaster1.xml', 'application/vnd.openxmlformats-officedocument.presentationml.notesMaster+xml'), ('/ppt/notesSlides/notesSlide1.xml', 'application/vnd.openxmlformats-officedocument.presentationml.notesSlide+xml'), ('/ppt/notesSlides/notesSlide10.xml', 'application/vnd.openxmlformats-officedocument.presentationml.notesSlide+xml')]
--- _rels/.rels 全文 ---
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="ppt/presentation.xml"/><Relationship Id="rId3" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/><Relationship Id="rId4" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/custom-properties" Target="docProps/custom.xml"/></Relationships>
--- presentation.xml 前 400 字节(看命名空间,purl.oclc.org = Strict OOXML) ---
<?xml version='1.0' encoding='utf-8'?>
<p:presentation xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:p15="http://schemas.microsoft.com/office/powerpoint/2012/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1" /><p
```

看起来没有什么问题。我们试着使用 `python-pptx` 重新保存一下这个文件

```Python
from pptx import Presentation
Presentation('badcase2.pptx').save('roundtrip.pptx')
```


这个新生成的文件是可以被 `soffice` 正常处理的。

python-pptx 重新保存时，会把不认识的 part 原样透传（XML 内容语义不变），但会用规范的方式重新序列化整个包——`[Content_Types].xml`、`.rels`、条目名、压缩方式全部重写。round-trip 之后的文件能被正常处理，说明 XML 内容本身没有问题，问题出在 WPS 序列化这个包的"写法"上。嫌疑范围就此从"XML 内容层"收窄到"包序列化层"，后面要做的就是找出到底是哪种写法。

我们再来扫描一下 `.rels` 的数据

```Python
import posixpath, re, collections, zipfile
import xml.etree.ElementTree as ET

z = zipfile.ZipFile("badcase2.pptx")
names = set(z.namelist())

top = collections.Counter()
for n in names:
    if not n.endswith("/"):
        top["/".join(n.split("/")[:2])] += 1
print("== 条目分布(top 20) ==")
for k, c in top.most_common(20):
    print(f"{c:6d}  {k}")

ct = z.read("[Content_Types].xml").decode("utf-8", "replace")
m = re.search(r'PartName="/ppt/presentation\.xml"[^>]*ContentType="([^"]+)"', ct)
print("/ppt/presentation.xml Override:", m.group(1) if m else "!!! 缺失")
print(
    "Default 声明数:",
    len(re.findall(r"<Default ", ct)),
    " Override 数:",
    len(re.findall(r"<Override ", ct)),
)

print("== 扫描全部 .rels 的 Target 合规性 ==")
bad, missing = [], []
for n in sorted(names):
    if not n.endswith(".rels"):
        continue
    base = posixpath.dirname(posixpath.dirname(n))
    root = ET.fromstring(z.read(n))
    for rel in root:
        t, mode = rel.get("Target", ""), rel.get("TargetMode", "")
        if mode == "External" or re.match(r"^[a-zA-Z]+:", t):
            continue
        if "\\" in t or " " in t:
            bad.append((n, t))
        norm = (
            posixpath.normpath(t.lstrip("/"))
            if t.startswith("/")
            else posixpath.normpath(posixpath.join(base, t))
        )
        if norm not in names:
            missing.append((n, t))
print("含反斜杠/空格的 Target:", len(bad))
for x in bad[:10]:
    print("  ", x)
print("指向不存在条目的 Target:", len(missing))
for x in missing[:10]:
    print("  ", x)

```


得到

```
== 条目分布(top 20) ==
  1392  ppt/tags
   184  ppt/slideLayouts
    54  ppt/notesSlides
    54  ppt/slides
    13  ppt/media
    10  ppt/slideMasters
     7  ppt/theme
     2  ppt/handoutMasters
     2  ppt/notesMasters
     1  ppt/presentation.xml
     1  docProps/custom.xml
     1  docProps/app.xml
     1  ppt/commentAuthors.xml
     1  ppt/viewProps.xml
     1  ppt/_rels
     1  ppt/presProps.xml
     1  _rels/.rels
     1  [Content_Types].xml
     1  ppt/tableStyles.xml
     1  docProps/core.xml
/ppt/presentation.xml Override: application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml
Default 声明数: 0  Override 数: 0
== 扫描全部 .rels 的 Target 合规性 ==
含反斜杠/空格的 Target: 0
指向不存在条目的 Target: 0
```


至此条目干净、引用干净、主文档 XML 合法、`python-pptx` 能读，但是 `soffice` 就是无法处理


但是这个脚本的输出里面有一组自相矛盾的数据

```
/ppt/presentation.xml Override: ...presentationml.presentation.main+xml   #  正则明明找到了
Default 声明数: 0  Override 数: 0                                          # 但字面搜索一个都没有?!
```


正则按属性搜(`PartName="..." ContentType="..."`)能找到 Override 元素，按字面量搜 `<Default ` / `<Override ` 却一个都没有。说明元素可能带着命名空间前缀(形如 `<ns0:Override ...>`)。而 `docProps/app.xml` 里的 `<ep:Properties>`(而非 `<Properties>`)也是同款写法。再来看一眼 `[Content_Types].xml` 的内容



```XML
<?xml version='1.0' encoding='utf-8'?>
<ns0:Types xmlns:ns0="http://schemas.openxmlformats.org/package/2006/content-types">
  <ns0:Default Extension="png" ContentType="image/png" />
  ...
```


`[Content_Types].xml` 是 OPC 包的类型清单，拿不到它， LibreOffice 连主文档是什么类型都不知道，只能在包打开阶段放弃。这也是为什么强制 `--infilter` 无效，过滤器是加载文档阶段的事，而失败发生在更早的打开包的阶段



我们尝试将 `[Content_Types].xml` 的 `ns0:` 前缀改写为默认命名空间



```Python
import re, zipfile

zin = zipfile.ZipFile("badcase2.pptx")
ct = zin.read("[Content_Types].xml").decode("utf-8")
m = re.search(
    r'xmlns:(\w+)="http://schemas.openxmlformats.org/package/2006/content-types"', ct
)
print("检测到的命名空间前缀:", m.group(1) if m else None)

if m:
    p = m.group(1)
    ct2 = (
        ct.replace(f"<{p}:", "<")
        .replace(f"</{p}:", "</")
        .replace(f"xmlns:{p}=", "xmlns=")
    )
    assert "<Default " in ct2 or "<Default>" in ct2, "改写后仍找不到 Default"
    with zipfile.ZipFile("ct_fixed.pptx", "w", zipfile.ZIP_DEFLATED) as zout:
        for info in zin.infolist():
            if info.filename.endswith("/"):
                continue
            data = (
                ct2.encode("utf-8")
                if info.filename == "[Content_Types].xml"
                else zin.open(info).read()
            )
            zout.writestr(info.filename, data)
    print("已生成 ct_fixed.pptx(仅 [Content_Types].xml 去前缀,其余原样)")
```


生成的 `ct_fixed.pptx` 可以被 `soffice` 正常读取了。`ns0:` 更像是 Python `xml.etree.ElementTree` 序列化时没有注册默认命名空间产生的结果。如果没有调用 `ET.register_namespace('', ns)`，`ElementTree` 会自动分配 `ns0`、`ns1` 等前缀。可以推测，这个 WPS 的文件在后续流转过程中，可能经过了基于 Python ElementTree 的 OOXML 处理流程，例如在线转换、去水印或压缩等。`[Content_Types].xml` 在这个过程中被重新序列化，最终变成了带 `ns0` 前缀的形式。



## 背景知识：PPTX 的标准格式



### PPTX = ZIP 容器 + OPC 包结构



`.pptx` / `.docx` / `.xlsx` 都是 OOXML 文档（ECMA-376 / ISO/IEC 29500）：物理上是一个 ZIP 压缩包，容器格式遵循 PKWARE 的 ZIP 规范 [APPNOTE.TXT](https://pkware.cachefly.net/webdocs/casestudies/APPNOTE.TXT)；逻辑上遵循 OPC（Open Packaging Conventions，ECMA-376 Part 2）。包里的文件称为 part，读取器不靠遍历目录来发现它们，而是靠两份显式索引：`[Content_Types].xml` 声明每个 part 的 Content-Type，各级 `.rels` 文件按关系把 part 串起来。



一个标准 pptx 的典型结构:



```
[Content_Types].xml          # 必须存在,包的"清单",声明每个 part 的 Content-Type
_rels/.rels                  # 包级关系文件,officeDocument 关系指向主文档
docProps/app.xml             # 扩展属性(生成工具、页数等)—— 生成工具识别就靠它
docProps/core.xml            # 核心属性(标题、作者、创建时间)
ppt/presentation.xml         # 主文档:幻灯片列表、页面尺寸
ppt/_rels/presentation.xml.rels
ppt/slides/slide1.xml ...    # 每页一张幻灯片
ppt/slides/_rels/slideN.xml.rels
ppt/slideMasters/ ...        # 母版
ppt/slideLayouts/ ...        # 版式
ppt/theme/theme1.xml         # 主题
ppt/media/ ...               # 图片等媒体资源
```



读取器定位主文档的标准方式：读 `_rels/.rels`，找 `Type=".../relationships/officeDocument"` 那条关系的 `Target`。问题二里对 `[Content_Types].xml`、`_rels/.rels`、`ppt/presentation.xml` 三个文件的检查，就是在逐项核对这个结构。



### `[Content_Types].xml` 和 XML 命名空间前缀



`[Content_Types].xml` 是 OPC 强制要求的部件，用 `Default`（按扩展名）和 `Override`（按 PartName）两种方式声明包里每个 part 的 Content-Type。读取器打开包的第一步就是解析它——拿不到或解析不了，每个 part 的类型都无从得知，更谈不上找到主文档。这就是为什么问题二的失败发生在"打开包"阶段：`--infilter` 指定的是文档加载阶段的过滤器，而失败比它更早。



要注意，带 `ns0:` 前缀的写法在 XML 层面是合法的。命名空间前缀只是一个别名，`<ns0:Types xmlns:ns0="...">` 和默认命名空间的 `<Types xmlns="...">` 在语义上完全等价，规范的解析器应该按 namespace URI + local name 来识别元素。所以两个问题性质不同：问题一是明确违反 ZIP 规范，问题二是合法写法触发了 LibreOffice 解析器的实现问题——它大概率是按字面标签名匹配的，不认别名前缀。



`ns0` 这个前缀本身还是一枚指纹：Python 的 `xml.etree.ElementTree` 序列化时，如果事先没有调用 `register_namespace` 注册命名空间，会自动给每个命名空间分配 `ns0`、`ns1` 这样的前缀；而 lxml 等库会保留文档原有的写法。所以看到 `ns0`，基本可以断定文件被某个基于 ElementTree 的流程重新序列化过。



### ZIP 规范对路径分隔符的要求


ZIP 里每个条目（entry）的名字是该文件的相对路径，同时存储在 local file header 和文件末尾的中央目录（central directory）中。APPNOTE.TXT 4\.4\.17\.1 节（原文见问题一）规定：条目名不允许盘符和前导斜杠，路径分隔符必须是正斜杠 `/`。由此有两个和本文直接相关的推论：

- 目录条目（名字以 `/` 结尾的空条目）只是给解压工具预建目录的提示，规范上是可选的。Python 的 zipfile、python-pptx 这些生成器根本就不写目录条目，文件照样合法。问题一的修复脚本直接丢弃所有目录条目，依据就是这条

- 反斜杠是典型的 Windows 程序错误：导出程序如果直接拿操作系统路径拼接的结果当条目名、不做规范化，就会写入 `ppt\slideMasters/` 这样的非法条目。问题一那个文件的顶层目录（`ppt/`、`docProps/`、`_rels/`）是正斜杠，子目录却全是反斜杠，很可能是导出器两层代码路径不一致：顶层硬编码写对了，递归遍历子目录时却直接用了 OS 原生分隔符




### 重打包为什么是安全的

问题一的修复只有两步：丢弃所有目录条目、反斜杠转正。这两步对文档语义没有影响，逐条看：


- 目录条目在规范上就是可选的，OOXML 消费端（LibreOffice、PowerPoint、python-pptx）从不按目录枚举内容，寻址全部走 `[Content_Types].xml` 和 `.rels` 的显式索引，目录条目不参与。python-pptx 生成的标准 pptx 本来就没有任何目录条目，空目录（比如没有媒体的 `ppt/media/`）在 OPC 里也没有任何语义。实际验证过：重打包后零目录条目的文件，以及 python-pptx 生成的对照文件，都能被 LibreOffice 正常转换。



- 反斜杠转正是向规范对齐，不是改写内容。OOXML 包内的所有引用（`[Content_Types].xml` 的 `PartName`、`.rels` 的 `Target`）一律使用 `/`，名字带 `\` 的条目本来就不可能符合规范地被引用，转正只是让它叫回本来的名字。本例的文件条目本来就全是正斜杠，转正对它们是 no-op。理论上的边界情况是同一个 zip 里同时存在 `a\b.png` 和 `a/b.png` 两个文件条目，转正后会重名——这属于病理构造，现实中没有遇到。



- 重打包真正改变的东西都不在 OOXML 语义范围内：条目时间戳、Unix 权限、extra fields 这些 ZIP 容器层元数据会丢，所有条目重新 deflate 压缩，条目顺序可能变化，OOXML 都不关心。（作为对比，ODF 和 EPUB 有 "mimetype 条目必须第一个且不压缩" 的硬规则，重打包那类格式才需要小心；OOXML 没有这条规则。）所有 part 的 XML 内容是字节级不变的，`zin.open(info).read()` 原样复制。







