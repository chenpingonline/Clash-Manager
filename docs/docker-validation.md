# Docker 改造验证记录

日期：2026-10-08。开发分支：`codex/docker-support`。应用版本仍为 `fpk/manifest` 的 1.3.1，未发布新版本或镜像。

## 已完成

- Go 全量测试、race 与 vet；认证拒绝、Cookie 签名和过期、跨站写请求拒绝、登录限流、原生路径兼容和 Docker 权限检测测试通过。
- Vue 类型检查、23 个测试文件 / 134 项测试、生产构建通过。
- Go Web / Helper 的 Linux amd64、arm64 交叉编译通过。
- 两份 Compose 配置与启动/健康检查脚本语法校验通过。
- 本机 Linux ARM64 Docker Engine 中构建 `clash-for-fnos:docker-dev`，使用 `--local-build` 路径生成与 Dockerfile 相同的最终运行层。完整多阶段源码构建因构建器基础镜像下载缓慢未跑完；最终运行镜像构建成功。新增 GitHub Actions 检查流程尚未在远端执行。
- Bridge 冒烟验证：拒绝未登录请求、正确与错误密码、宿主专属接口拒绝、内置 Mihomo 1.19.32 启动、HTTP Mixed 代理实际请求、卷内设置跨容器重建恢复、Web UID 10001、健康检查、正常停止退出码 0、退出后 API 拒绝访问。无密码部署在启动服务之前被拒绝。
- 隔离的 Bridge 容器网络中添加 NET_ADMIN 和 `/dev/net/tun`，实测 TUN 开启、设备创建、路由安装、关闭和路由恢复。测试没有修改共享 Docker 宿主的路由。
- Computer Use 实际操作登录、仪表盘规则/全局模式切换、Docker 更新设置与退出登录；普通 Bridge 中 TUN 缺少设备时显示不可用并禁用开关。飞牛系统代理和图标入口已隐藏。

截图保存在本地忽略目录 `artifacts/docker-qa/`。

## 仍需验收

- 真实 fnOS 安装或升级，以及真实宿主 Host TUN 的流量覆盖。
- DNS、IPv6、多网卡/VPN、其他 Docker Bridge 或 Macvlan/IPvlan 容器的共存与转发。
- Host TUN 的正常停止恢复，以及强制 kill、掉电或异常退出后的恢复策略；当前没有实现独立宿主看门狗或异常规则回收器。
- amd64 实际运行镜像、多阶段源码构建、远端 CI 和镜像发布。
- 新镜像内置 Core 升级与回滚仍复用既有事务测试，本轮仅实测普通容器重建及数据恢复，没有做完整镜像升级矩阵。

部署步骤见 [Docker 部署说明](../docker/README.md)。
