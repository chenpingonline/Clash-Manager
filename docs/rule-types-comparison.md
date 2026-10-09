# 规则类型下拉对照

参考 Clash Verge Rev v2.5.8 的[规则编辑器](https://github.com/clash-verge-rev/clash-verge-rev/blob/v2.5.8/src/components/profile/rules-editor-viewer.tsx)和[中文文案](https://github.com/clash-verge-rev/clash-verge-rev/blob/v2.5.8/src/locales/zh/rules.json)。

双方均有 33 种类型，没有缺失项。本次按该版本对齐说明文案及顺序（SUB-RULE 位于 RULE-SET 前），保留原有规则生成、保存和应用流程。下拉支持搜索中文说明或类型代码，长文案换行展示。

| 序号 | 类型 | 下拉说明 |
|---|---|---|
| 1 | `DOMAIN` | 匹配完整域名 |
| 2 | `DOMAIN-SUFFIX` | 匹配域名后缀 |
| 3 | `DOMAIN-KEYWORD` | 匹配域名关键字 |
| 4 | `DOMAIN-REGEX` | 匹配域名正则表达式 |
| 5 | `GEOSITE` | 匹配 GeoSite 内的域名 |
| 6 | `GEOIP` | 匹配 IP 所属国家代码 |
| 7 | `SRC-GEOIP` | 匹配来源 IP 所属国家代码 |
| 8 | `IP-ASN` | 匹配 IP 所属 ASN |
| 9 | `SRC-IP-ASN` | 匹配来源 IP 所属 ASN |
| 10 | `IP-CIDR` | 匹配 IP 地址范围 |
| 11 | `IP-CIDR6` | 匹配 IP 地址范围 |
| 12 | `SRC-IP-CIDR` | 匹配来源 IP 地址范围 |
| 13 | `IP-SUFFIX` | 匹配 IP 后缀范围 |
| 14 | `SRC-IP-SUFFIX` | 匹配来源 IP 后缀范围 |
| 15 | `SRC-PORT` | 匹配请求来源端口范围 |
| 16 | `DST-PORT` | 匹配请求目标端口范围 |
| 17 | `IN-PORT` | 匹配入站端口 |
| 18 | `DSCP` | 匹配 DSCP 标记 |
| 19 | `PROCESS-NAME` | 匹配进程名称 |
| 20 | `PROCESS-PATH` | 匹配完整进程路径 |
| 21 | `PROCESS-NAME-REGEX` | 正则匹配完整进程名称 |
| 22 | `PROCESS-PATH-REGEX` | 正则匹配完整进程路径 |
| 23 | `NETWORK` | 匹配 TCP/UDP |
| 24 | `UID` | 匹配 Linux USER ID |
| 25 | `IN-TYPE` | 匹配入站类型 |
| 26 | `IN-USER` | 匹配入站用户名 |
| 27 | `IN-NAME` | 匹配入站名称 |
| 28 | `SUB-RULE` | 匹配至子规则 |
| 29 | `RULE-SET` | 引用规则集合 |
| 30 | `AND` | 逻辑与 |
| 31 | `OR` | 逻辑或 |
| 32 | `NOT` | 逻辑非 |
| 33 | `MATCH` | 匹配所有请求 |
