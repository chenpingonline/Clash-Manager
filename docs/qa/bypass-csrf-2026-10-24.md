# fnOS 接管请求来源校验修复

## 原因与改动

接管的 preview、configure、stop 接口原先直接比较浏览器 Origin 与后端 Host。fnOS Gateway 转发至 Unix socket 后可能改写 Host，因此合法页面请求也返回“拒绝跨站请求”。

新增只读的 `/api/network/bypass/csrf` 接口（沿用已有入口鉴权，响应 no-store），前端每次操作先获取当前网关进程的随机令牌，再用 `X-Clash-Bypass-Token` 请求头提交。复用应用升级已有的原生请求校验逻辑；只有 fnOS 原生 Unix socket、未启用独立TCP监听和密码认证、有效HTTP(S) Origin、非明确 cross-site 且令牌正确时允许 Host 不匹配。普通同源检查及CLI兼容行为不变。不信任 X-Forwarded-Host 或用户标识头，不自动重试接管操作。

## 本地验证

- `go test -race ./...` 与 `go vet ./...` 通过。
- 实际 Unix socket HTTP 集成测试覆盖三个路由：Host 被改写、缺少 Fetch Metadata、原有同源请求、缺失/错误令牌、明确跨站、opaque/无效 Origin。
- 直接 TCP 请求不能通过伪造转发头/用户头进入兼容路径；Docker、Linux、显式TCP监听和密码认证入口不启用该路径。
- 配置事务测试使用 Host 不匹配的原生请求，验证正常提交及防火墙失败后的原配置恢复。
- `npm --prefix web run check` 通过：247项测试、类型检查、翻译检查和生产构建。新增服务测试验证三个动作携带新令牌、载荷保留、令牌获取失败不提交及403不重复提交。

仅修正请求链路，没有修改弹窗布局或系统防火墙规则生成逻辑。尚未在实际 fnOS Gateway 部署复测。后续 FPK 打包版本与审计记录由 fnOS 仓库记录。
