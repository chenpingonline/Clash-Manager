# 独立 DIRECT 入口验证与回环保护

日期：2026-10-31；设备应用 1.6.24，Mihomo v1.19.33。

## 原因

早期测试从 Mac 经过 NAS 7890 代理连接到同一 Mihomo 的回环独立入口，再使用 DIRECT 出站。这条链路包含同一 DIRECT 适配器创建的连接再次进入 Core。

Mihomo v1.19.33 的 `adapter/outbound/direct.go` 在创建出站连接后调用 `loopBack.NewConn`，记录其本地地址及端口；后续 `CheckConn` 将入站源地址端口与记录比较，命中时拒绝并记录 `reject loopback connection`。这是连接级的回环保护，不是独立入口目标名称校验失败，也不是订阅更新移除了入口。

对应上游源码：

- https://github.com/MetaCubeX/mihomo/blob/v1.19.33/adapter/outbound/direct.go
- https://github.com/MetaCubeX/mihomo/blob/v1.19.33/component/loopback/detector.go

## 正确验证路径

从另一台机器直接连接入口；需要本机限定监听时，应使用独立于被测 Core 的 SSH 转发或在 NAS 本机运行客户端。不要用被测 Core 自己的 HTTP/SOCKS 代理充当入口的传输通道。

临时局域网测试需选择 NAS 的具体地址、未占用高位端口及独立认证；测试结束删除入口、确认监听释放，保留原有系统代理、TUN 和订阅设置。无需更改内核回环检测、DNS 或 TUN 路由以使上述嵌套链路通过。

客户端命令示例（`QA_AUTH` 仅为临时测试账号，不使用 NAS 或订阅凭据）：

```sh
curl --noproxy '' --proxy "http://$QA_NAS:$QA_PORT" --proxy-user "$QA_AUTH" \
  --connect-timeout 5 --max-time 20 --silent --show-error --output /dev/null \
  --write-out '%{http_code}\n' https://www.baidu.com/
curl --noproxy '' --proxy "socks5h://$QA_NAS:$QA_PORT" --proxy-user "$QA_AUTH" \
  --connect-timeout 5 --max-time 20 --silent --show-error --output /dev/null \
  --write-out '%{http_code}\n' https://www.baidu.com/
```

保留默认 TLS 证书验证。正确账号应取得目标 HTTPS 200；错误密码应被代理拒绝（Mihomo HTTP 错误账号可返回 403，SOCKS5 为认证状态非零）。在 Core 日志核对请求来自真实客户端并 `using DIRECT`。

## 真机结果

在 192.168.1.88 创建临时带认证 DIRECT 入口，从 Mac 直接建立 TCP 连接。Mixed 的 HTTP CONNECT 与 SOCKS5 均 HTTPS 200 且 TLS 校验通过，错误密码均被拒绝；独立 HTTP 与独立 SOCKS5 类型也分别通过。将同一个入口改用“7890 → 独立入口”链路可复现 TLS EOF，直接连接正常。Core 日志明确显示真实 Mac 客户端请求 `using DIRECT`。

此次修正的是测试方法，应用和 Core 未修改，FPK 保持已安装的 1.6.24。完整逐协议结果与清理证据在 fnOS 打包仓库 `dist/nas-direct/`，以其中的实际记录为准。此结论不覆盖任意网络的 DNS、UDP、IPv6 或所有站点。
