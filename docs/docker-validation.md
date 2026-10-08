# Docker 改造验证记录

日期：2026-10-08。开发分支：`codex/docker-support`。应用版本仍为 `fpk/manifest` 的 1.3.1，已发布 Docker 多架构镜像，未发布新的 FPK 版本。

## 已完成

- Go 全量测试、race 与 vet；认证拒绝、Cookie 签名和过期、跨站写请求拒绝、登录限流、原生路径兼容和 Docker 权限检测测试通过。
- Vue 类型检查、23 个测试文件 / 134 项测试、生产构建通过。
- Go Web / Helper 的 Linux amd64、arm64 交叉编译通过。
- 默认 Host、兼容 Host 与 Bridge 三份 Compose 配置与启动/健康检查脚本语法校验通过。
- 本机 Linux ARM64 Docker Engine 中构建 `clash-for-fnos:docker-dev`，使用 `--local-build` 路径生成与 Dockerfile 相同的最终运行层。完整多阶段源码构建因构建器基础镜像下载缓慢未跑完；最终运行镜像构建成功。新增 GitHub Actions 检查流程尚未在远端执行。
- Bridge 冒烟验证：拒绝未登录请求、正确与错误密码、宿主专属接口拒绝、内置 Mihomo 1.19.32 启动、HTTP Mixed 代理实际请求、卷内设置跨容器重建恢复、Web UID 10001、健康检查、正常停止退出码 0、退出后 API 拒绝访问。无密码部署在启动服务之前被拒绝。
- 隔离的 Bridge 容器网络中添加 NET_ADMIN 和 `/dev/net/tun`，实测 TUN 开启、设备创建、路由安装、关闭和路由恢复。测试没有修改共享 Docker 宿主的路由。
- Computer Use 实际操作登录、仪表盘规则/全局模式切换、Docker 更新设置与退出登录；普通 Bridge 中 TUN 缺少设备时显示不可用并禁用开关。飞牛系统代理和图标入口已隐藏。
- 已发布 `gtaaa/clash-for-fnos:1.3.1-arm64`、`1.3.1-amd64` 和统一多架构标签 `1.3.1`。公开 Registry 清单及两个镜像配置的实际架构一致；本机默认拉取并运行 ARM64，指定 AMD64 的拉取和模拟运行也已确认。统一清单摘要为 `sha256:af7ec985beb2d16fa0cb8ee228781dfdba1395cfb5009d5e4779cab75bd0b300`。
- 默认 Compose 已改为 Host 网络、NET_ADMIN 和 /dev/net/tun 映射；兼容 Host 配置相同，独立 Bridge 配置保留普通代理端口映射。Compose 解析检查不代表真实宿主 TUN 验收。
- 三份 Compose 默认使用多架构 latest；已验证默认镜像、固定版本和本地镜像覆盖均能正确解析，不包含强制架构或本地构建配置。
- AMD64 使用同一运行层、对应架构的 Debian 13 基础镜像及交叉编译程序构建。在 QEMU 中通过登录认证、实际 Mixed 代理请求、持久化、Web 降权和正常停止检查；完整冒烟的 Core 进程识别检查未通过，因为 `/proc/PID/exe` 指向 QEMU 而非 Mihomo，未据此宣称原生 AMD64 或其 TUN 验证通过。临时模拟器注册已移除。
- 登录页此前在桌面和平板宽度下占用了管理布局的侧栏列。登录容器现横跨所有列，在 Computer Use 中验证 1440、900、390 像素宽度、错误密码、成功登录、退出登录及管理布局；类型检查和 Docker 前端构建通过。
- 登录修复镜像基于两个已发布的 1.3.1 架构镜像，只更新同一份前端资源；ARM64 新镜像通过完整普通代理冒烟。固定修复构建标签为 `1.3.1-login-fix`，发布时 latest 指向此多架构清单，随后由启动端口修复版本更新；初始 `1.3.1` 标签保持不变。AMD64 运行层和 Core 未修改，原生 x86 与 TUN 验收限制仍保留。

- Docker 启动端口覆盖：新增 APP_CONTROLLER_PORT / APP_MIXED_PORT，在 Helper 启动 Core 前同步托管配置与用户端口设置。Go race 全量测试和 vet 通过；本地 ARM64 镜像 clash-for-fnos:docker-ports 验证 19090/17890 启动、Controller 连接、Mixed 实际代理请求，并在网页 API 保存回 9090/7890 后重建同一数据卷，确认启动变量重新生效。三份 Compose 变量解析及 Bridge TCP/UDP 映射通过。此后端改动已发布到 Docker Hub，真实 fnOS Host 验收仍未完成。

- 启动端口修复镜像已发布为 `1.3.1-docker-ports-arm64` / `1.3.1-docker-ports-amd64`；对应固定多架构标签 `1.3.1-docker-ports` 的清单摘要（发布时 latest 同步指向该清单）为 `sha256:8c112a9bd17395d4d88425be38be88e0730031605959d4f23fbe21ed091b249a`。两种架构均替换了对应的交叉编译 Helper，并核对 ELF 架构；ARM64 发布镜像通过端口覆盖冒烟及隔离 TUN 路由安装/恢复。远端两份清单的架构与子镜像摘要已核对，指定 AMD64 拉取及 latest 自动选择 ARM64 均通过。AMD64 新后端未在原生 x86 上运行验证。

- Compose 部署配置整理：示例 .env 集中网页 17890 / Controller 19097 / Mixed 17897；默认 Host 无端口映射，Bridge 独立提供 WEB_PORT 和监听地址。新增 CLASH_DATA_DIR 与 Docker 日志 10 MB × 3 轮转。三份配置解析、Host 兼容入口一致性、自定义端口/地址/数据路径/镜像覆盖、空端口变量保留及缺失密码拒绝均通过；未启动 Host 实例或修改宿主路由。本次仅调整部署配置和说明，无需更新镜像。

- 管理密码最低长度从 12 改为 8 字符：同步更新 Web、容器启动器、三份 Compose 提示与部署说明。Go 全量 race 测试和 vet 通过，覆盖 7/8 字符、密码文件和原生 Unix Socket 兼容。ARM64 发布镜像实测 8 字符启动与登录、7 字符在 Helper/Web 启动前拒绝，代理、持久化、端口覆盖及隔离 TUN 路由安装/恢复通过。双架构 Web ELF 已核对；AMD64 原生运行仍待验收。发布标签 `1.3.1-password8-arm64` / `1.3.1-password8-amd64`，固定多架构标签 `1.3.1-password8` 与当前 `latest` 清单摘要均为 `sha256:c85758baf9a7df13466b2f04e14dc9b074bab0330b67c4aae080f254fd33c50b`。主 Registry 入口多次上传 EOF 后，经 Docker Hub 官方 registry.hub.docker.com 入口上传，使用临时凭据配置，未修改用户 Docker 登录配置。Docker Hub 公开 API 已核对两份清单及架构摘要，经该官方入口指定 AMD64 拉取与自动 ARM64 拉取均通过。

截图保存在本地忽略目录 `artifacts/docker-qa/`。

## 仍需验收

- 真实 fnOS 安装或升级，以及真实宿主 Host TUN 的流量覆盖。
- DNS、IPv6、多网卡/VPN、其他 Docker Bridge 或 Macvlan/IPvlan 容器的共存与转发。
- Host TUN 的正常停止恢复，以及强制 kill、掉电或异常退出后的恢复策略；当前没有实现独立宿主看门狗或异常规则回收器。
- amd64 原生运行、多阶段源码构建和远端 CI。
- 新镜像内置 Core 升级与回滚仍复用既有事务测试，本轮仅实测普通容器重建及数据恢复，没有做完整镜像升级矩阵。

部署步骤见 [Docker 部署说明](../docker/README.md)。

- 新账号发布：`chenpingonline/clash-for-fnos:1.3.1` 是新仓库唯一标签，包含 `linux/amd64` 与 `linux/arm64`，不发布架构或修复后缀。多架构清单摘要为 `sha256:c85758baf9a7df13466b2f04e14dc9b074bab0330b67c4aae080f254fd33c50b`。当前源码交叉编译产物、前端实际文件、Core、GEO、启动器和健康检查与发布镜像逐项核对；Go 全量 race/vet、前端 134 项测试与构建通过。Docker Hub 公开 API 已确认标签、双架构和子清单摘要；两种架构分别拉取核对通过，默认拉取自动选择 ARM64，发布地址 ARM64 容器登录、代理、端口覆盖、持久化及正常停止通过，隔离 TUN 路由安装/恢复通过。三份 Compose 与示例、部署文档已同步新地址；版本通过 `CLASH_IMAGE` 选择。AMD64 原生运行和真实 fnOS Host TUN 仍待验收。
