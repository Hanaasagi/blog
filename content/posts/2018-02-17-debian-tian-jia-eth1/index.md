
+++
title = "debian 添加 eth1"
summary = ''
description = ""
categories = []
tags = []
date = 2018-02-17T13:57:01+08:00
draft = false
+++

大年三十那天不幸收到了 GFW 的新年礼物，服务器 IP 被封禁。所以临时搞了一个新的 IP，简单记录一下 debian 下如何添加新的 ethernet interface

本机目前 IP 状况如下(非真实IP)

<table>
<tr>
<th></th><th>IP</th><th>Netmask</th><th>Gateway</th>
</tr>
<tr>
<td>IP_1(DHCP)</td><td>163.82.105.56</td><td>255.255.254.0</td><td>163.82.105.1</td>
</tr>
</table>

新增 IP 如下

<table>
<tr>
<th></th><th>IP</th><th>Netmask</th><th>Gateway</th>
</tr>
<tr>
<td>IP_2(STATIC)</td><td>133.130.122.77</td><td>255.255.254.0</td><td>133.130.122.1</td>
</tr>
</table>

编辑 `/etc/network/interfaces`

```
# The primary network interface
auto eth0
iface eth0 inet dhcp
iface eth0 inet6 dhcp
accept_ra 1

# 添加
auto eth1
iface eth1 inet static
address 133.130.122.77
netmask 255.255.254.0 
gateway 133.130.122.1
```

需要重启 `systemctl restart networking.service`

这时可以服务器自身可以 ping 通此地址，但是外界无法 ping 通

编辑 `/etc/iproute2/rt_tables`

```
#
# reserved values
#
255	local
254	main
253	default
0	unspec
#
# local
#
#1	inr.ruhep

# 添加
252 net_133
```

进行路由配置

```
➜ ip route add 133.130.122.0/23 dev eth1 src 133.130.122.77 table net_133
➜ ip route add default via 133.130.122.1 dev eth1 table net_133
➜ ip rule add from 133.130.122.0/23 table net_133
➜ ip rule add to 133.130.122.0/23 table net_133
```

#### Update 2018.02.25
第二个 IP 也被封了 （’へ’）
    