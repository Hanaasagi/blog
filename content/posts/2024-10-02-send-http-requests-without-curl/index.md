+++
title = "没有 curl 的情况下发送 HTTP 请求"
summary = ""
description = ""
categories = [""]
tags = [""]
date = 2024-10-02T06:03:53+09:00
draft = false

+++



`debian:slim` 镜像中是默认没有安装 `curl`、`wget` 等工具的。如果基于这些镜像构建的时候没有额外去安装，那么遇到离线部署调试问题的时候，可能会比较麻烦。如果我们有宿主机的权限，那么可以通过 `nsenter` 命令进入容器对应的 network namespace 然后使用宿主机本身的 curl 进行调试。如果没有呢，这里科普一个小办法



```bash
#!/bin/bash

exec 3<>/dev/tcp/example.org/80

lines=(
    'GET /index.html HTTP/1.1'
    'Host: example.org'
    'Connection: close'
    ''
)

printf '%s\r\n' "${lines[@]}" >&3

while read -r data <&3; do
    echo "< $data"
done

exec 3>&-
```



上面是一个脚本创建了一个 socket，然后通过读写 socket 的方式发送 HTTP 的报文。`exec` 这个命令可以用于打开、关闭、复制文件描述符，比如

> 1. 将文件 `readfile` 作为文件描述符 3 打开进行读取：
>
>    ```bash
>    exec 3< readfile
>    ```
>
> 2. 将文件 `writefile` 作为文件描述符 4 打开进行写入：
>
>    ```bash
>    exec 4> writefile
>    ```
>
> 3. 将文件描述符 0 复制为文件描述符 5：
>
>    ```bash
>    exec 5<&0
>    ```
>
> 4. 关闭文件描述符 3：
>
>    ```bash
>    exec 3<&-
>    ```
>
> 5. 使用 `cat` 实用程序替换当前 shell 以读取文件 `maggie`：
>
>    ```bash
>    exec cat maggie
>    ```



详情可以查看一下 `man 1 exec`



这里的 `/dev/tcp/example.org/80` 是一个伪文件路径，格式是 `/dev/tcp/$host/$port`。当在这个路径上执行命令时，Bash 会打开与相关 socket 的 TCP 连接。同样的还有 `/dev/udp`，详情可以查看 https://tldp.org/LDP/abs/html/devref1.html#DEVTCP



我们可以基于这个脚本，封装出来 `nc` 和 `curl`



## 仿 nc 命令



```bash
#!/bin/bash

# Usage:
# ./nc.sh example.org 80
# > GET /index.html HTTP/1.1
# > Host: example.org
# >

stty erase ^H

if [ "$#" -lt 2 ]; then
    echo "Usage: $0 <host> <port>"
    exit 1
fi

host="$1"
port="$2"

exec 3<>/dev/tcp/"$host"/"$port"

{
    while IFS= read -r line <&3; do
        echo "< $line"
    done
} &

while true; do
    read -p "> " -r input
    if [[ "$input" == "exit" ]]; then
        break
    fi
    printf '%s\r\n' "$input" >&3
done

exec 3>&-
echo "Connection closed."

```



`stty erase ^H` 这一行是为了正确处理退格键的行为，也可以直接使用 `rlwrap` 工具来解决



## 仿 curl 命令



```bash
#!/bin/bash

# Usage:
# ./script.sh -X GET http://httpbin.org/get
# ./script.sh -X POST -H "Content-Type: application/json" -d '{"name":"test"}' http://httpbin.org:80/post

usage() {
  echo "Usage: $0 -X <METHOD> -d <DATA> -H <HEADER> <URL>"
  exit 1
}

method="GET"
data=""
headers=()
url=""

while getopts "X:d:H:" opt; do
  case $opt in
    X)
      method="$OPTARG"
      ;;
    d)
      data="$OPTARG"
      ;;
    H)
      headers+=("$OPTARG")
      ;;
    *)
      usage
      ;;
  esac
done
shift $((OPTIND -1))

url="$1"

if [[ -z "$url" ]]; then
  usage
fi

protocol=$(echo "$url" | grep :// | sed -e's,^\(.*://\).*,\1,g')
url_no_protocol="${url/$protocol/}"
host_with_port=$(echo "$url_no_protocol" | cut -d/ -f1)
path="/$(echo "$url_no_protocol" | grep / | cut -d/ -f2-)"

host=$(echo "$host_with_port" | cut -d: -f1)
port=$(echo "$host_with_port" | cut -d: -f2)

if [[ "$port" == "$host" ]]; then
  # TODO: need tls
  if [[ "$protocol" == "https://" ]]; then
    port=443
  else
    port=80
  fi
fi

exec 3<>/dev/tcp/"$host"/"$port"

request=("${method} ${path} HTTP/1.1" "Host: ${host}")
for header in "${headers[@]}"; do
  request+=("$header")
done

if [[ -n "$data" ]]; then
  request+=("Content-Length: ${#data}")

  content_type_set=false
  for header in "${headers[@]}"; do
    if [[ "$header" =~ ^Content-Type ]]; then
      content_type_set=true
      break
    fi
  done

  if [[ "$content_type_set" == false ]]; then
    request+=("Content-Type: application/x-www-form-urlencoded")
  fi
fi

request+=("Connection: close")
request+=("")

for line in "${request[@]}"; do
  printf '%s\r\n' "$line" >&3
done

if [[ -n "$data" ]]; then
  printf '%s' "$data" >&3
fi

while read -r line <&3; do
  echo "$line"
done

exec 3>&-
```



##  SSL/TLS

有些情况内网互相访问，为了安全会使用自签名证书强制走 https。这种如果容器里面有 `openssl`，可以使用其提供的 `s_client` 子命令帮助建立连接，否则不太好搞

```bash
openssl s_client -connect example.org:443
```



