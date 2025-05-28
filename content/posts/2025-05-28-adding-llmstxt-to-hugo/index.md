+++
title = "为博客增加 llms.txt"
summary = ""
description = ""
categories = [""]
tags = []
date = 2025-05-28T20:07:00+09:00
draft = false

+++



llms.tx 用于提供简洁、结构化且易于解析的 Markdown 格式内容，帮助 LLM 更好地读取网站内容。比如 Bun.js 就支持 https://bun.sh/llms.txt



本博客目前使用 Hugo 进行搭建。如果为 Hugo 增加 llms.txt，那么我们首先需要提供 md 格式的页面内容，在 `config.toml` 中增加

```toml
[outputs]
  page = ["HTML", "Markdown"]

[mediaTypes."text/markdown"]
  suffixes = ["md"]

[outputFormats.Markdown]
  mediaType = "text/markdown"
  isPlainText = true
  isHTML = false
  baseName = "index"
  rel = "alternate"
```



如果想生成 `index.html.md` 这种名称，那么 `baseName` 可以修改为 `index.html`



然后新增 `layouts/_default/single.md` 文件，内容如下


```
{{ $params := slice }}
{{- range $key, $value := .Params -}}
{{- if $value -}}
{{ $params = $params | append (printf "%s: %v" $key $value) }}
{{- end -}}
{{- end -}}
---
{{ range $param := $params }}{{ $param }}
{{ end }}---
{{ .RawContent }}
```



第二步，我们需要增加 `llms.txt` 文件，其内容是所有 `md` 文件的 URL，同样是修改 `config.toml`

```toml
[outputs]
  home = ["HTML", "RSS", "JSON", "TXT"]

[outputFormats.TXT]
  mediaType = "text/plain"
  baseName = "llms"
  isPlainText = true
```



新增 `layouts/_default/index.txt` 文件，内容如下

```
# {{ .Site.Title }}

> {{ .Site.Params.description }}
> Author: {{ .Site.Params.author.name }}

## Content
{{ range $type := slice "posts" }}
### {{ title $type }}

{{ range (where $.Site.RegularPages "Type" $type) }}- [{{ .Title }}]({{ .Permalink }}index.md): Published {{ .Date.Format "2006-01-02" }}
{{ end }}{{ end }}
```





生成的效果可以参考

- https://blog.dreamfever.me/llms.txt
- https://blog.dreamfever.me/posts/2025-02-08-learn-string-layout-optimization/index.md



### Reference

- https://llmstxt.org/#proposal

