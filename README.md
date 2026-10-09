<div align="center">

<img src="web/public/icon-current-256.png" alt="Clash Manager" width="128" />

# Clash Manager

简体中文 | [English](README.en.md)

**面向 Linux / Docker 的 Mihomo / Clash 管理器**

通过网页管理代理节点、订阅配置、规则、连接、日志、DNS、TUN 与 Mihomo Core。

[![Release](https://img.shields.io/github/v/release/chenpingonline/Clash-Manager?display_name=tag)](https://github.com/chenpingonline/Clash-Manager/releases)
[![Checks](https://github.com/chenpingonline/Clash-Manager/actions/workflows/checks.yaml/badge.svg)](https://github.com/chenpingonline/Clash-Manager/actions/workflows/checks.yaml)
[![Docker Pulls](https://img.shields.io/docker/pulls/chenpingonline/clash-manager)](https://hub.docker.com/r/chenpingonline/clash-manager)
![Linux](https://img.shields.io/badge/Linux-amd64%20%7C%20arm64-2ea44f)
[![Mihomo](https://img.shields.io/badge/Core-Mihomo-6f42c1)](https://github.com/MetaCubeX/mihomo)
[![License](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)

[操作手册](docs/user-guide.md) · [下载 Releases](https://github.com/chenpingonline/Clash-Manager/releases) · [Docker 部署](docker/README.md) · [问题反馈](https://github.com/chenpingonline/Clash-Manager/issues) · [Mihomo](https://github.com/MetaCubeX/mihomo) · [Clash for fnOS](https://github.com/chenpingonline/Clash-for-fnos)

</div>

---

![Clash Manager 界面示例](img.png)

界面截图仅作示例，功能与入口以当前版本和部署方式为准。

## 项目简介

Clash Manager 是面向 **Linux 和 Docker** 的 Mihomo 管理程序，提供节点、订阅、规则与网络设置的图形化管理入口。

本仓库维护公共 Vue / Go 代码、多语言、Docker 镜像与 Linux DEB 打包。Clash for fnOS 使用这里的公共源码，并在独立仓库维护原生 FPK、宿主生命周期、权限、窗口入口和图标。公共功能只开发一次，fnOS 通过固定提交采用更新。

Go Web 服务负责界面、API 和 Mihomo Controller 通信；独立 Root Helper 通过受限 Unix Socket 执行 Core 与网络操作。Linux DEB 的 Web 服务使用普通系统用户运行，Helper 独立以 root 运行。

## 功能

| 功能 | 说明 |
| --- | --- |
| 仪表盘 | 当前节点、出口信息、运行模式、连接与实时流量 |
| 代理节点 | 代理组切换、延迟测试、节点编辑与排序 |
| 订阅配置 | 多订阅管理、更新、配置增强与应用 |
| 配置文件 | YAML 编辑、校验、备份与应用 |
| 规则 | 规则查看、运行时开关与自定义规则管理 |
| 连接与日志 | 查看连接、关闭连接与检查 Core 日志 |
| DNS 与网络 | 代理端口、局域网访问、IPv6、DNS 与 Fake IP 设置 |
| TUN | 自动路由、出口接口、DNS 劫持与排除网段 |
| Core 与 GEO | 托管 Mihomo、Core 更新与 GEO 数据管理 |
| 管理登录 | 独立用户名与密码，Docker 可在环境变量中设置 |
| 多语言 | 简体中文、English、跟随系统，记住用户选择 |

Docker 与 Linux 界面按运行能力显示功能；fnOS 软件图标、FPK 更新和 fnOS Shell 代理环境变量设置由原生 fnOS 版提供。

## 界面语言

默认跟随浏览器语言，中文使用简体中文，其余使用英文。在**设置页右上角 → 界面语言**切换后立即生效，选择保存在当前浏览器中。节点名称、订阅名称、YAML、地址和原始日志保持原样。

翻译维护与术语约定见 [多语言指南](docs/i18n.md)。

---

## 安装与部署

| 部署方式 | 架构 | 使用场景 | 说明 |
| --- | --- | --- | --- |
| Docker Host | Linux amd64 / arm64 | 在 Linux 宿主运行，可开启 TUN 接管宿主流量 | [Docker 部署](docker/README.md) |
| Docker Bridge | Linux amd64 / arm64 | 应用或设备主动使用 HTTP / SOCKS 代理 | [Bridge 配置](docker/compose.bridge.yaml) |
| Linux DEB | amd64 / arm64 | Debian / Ubuntu 等 systemd 环境，使用原生服务 | [DEB 安装](packaging/deb/README.md) |
| fnOS FPK | x86_64 / ARM64 | 飞牛桌面与宿主集成 | [Clash for fnOS](https://github.com/chenpingonline/Clash-for-fnos) |

### Docker 快速开始

获取本仓库后，在根目录执行：

```bash
cp docker/.env.example docker/.env
# 编辑 docker/.env，设置至少 8 字符的 APP_AUTH_PASSWORD。
# 按实际端口占用修改 LISTEN_ADDR、APP_CONTROLLER_PORT、APP_MIXED_PORT。
docker compose --env-file docker/.env -f docker/compose.yaml pull
docker compose --env-file docker/.env -f docker/compose.yaml up -d
```

按 `.env.example` 的端口配置，打开 `http://宿主IP:17890`，使用 `admin` 和设置的密码登录。

主镜像为 `chenpingonline/clash-manager`。默认 Compose 使用 `latest` 多架构标签，Docker 自动选择 amd64 / arm64；固定版本时在 `.env` 设置 `CLASH_IMAGE=chenpingonline/clash-manager:<已发布版本号>`。

默认配置使用 **Linux Host 网络**，直接占用宿主端口，无需端口映射。TUN 需要宿主提供 `/dev/net/tun` 和 NET_ADMIN；导入可用配置后，在网页手动开启。普通代理可选择独立的 Bridge Compose。

容器升级保留原数据目录，重新拉取镜像并创建容器：

```bash
docker compose --env-file docker/.env -f docker/compose.yaml pull
docker compose --env-file docker/.env -f docker/compose.yaml up -d --force-recreate
```

认证、持久化、端口冲突、Host / Bridge 与停止行为见 [Docker 使用说明](docker/README.md)。

### Linux DEB

DEB 支持 amd64 / arm64，包含 systemd 服务、Mihomo 和基础 GEO 数据。选择对应架构的安装包，例如：

```bash
sudo apt install ./clash-manager_<版本号>_amd64.deb
sudo systemctl status clash-manager
sudo cat /etc/clash-manager/password
```

默认访问 `http://127.0.0.1:8080`，用户名 `admin`，首次安装生成随机密码。远程访问可使用 SSH 隧道。配置文件位于 `/etc/clash-manager/clash-manager.conf`，数据位于 `/var/lib/clash-manager`；升级保留数据、密码和用户修改的配置。

支持环境、服务配置、升级、卸载与构建见 [DEB 使用说明](packaging/deb/README.md)。

---

## 项目结构

```text
Clash-Manager/
├── web/                 # Vue 3 / TypeScript / Vite 与语言包
├── backend/             # Go Web、Root Helper 与公共业务代码
├── docker/              # Dockerfile、Compose、环境变量与部署文档
├── packaging/deb/       # DEB 打包、systemd 与安装脚本
├── resources/core/      # amd64 / arm64 Mihomo 资产与校验元数据
├── assets/              # 基础 GEO 数据与第三方许可证
├── scripts/             # 版本同步、构建、审计与验证
├── docs/                # 操作手册、翻译指南与截图
├── VERSION              # 公共应用版本源
├── CHANGELOG.md
├── LICENSE
└── dist/                # 构建产物，不提交
```

## 开发与构建

需要 Go **1.22+**、Node.js **22.12+** 和 npm。Docker 构建需要 Docker；DEB 构建还需要 Python 3 与 `dpkg-deb`，macOS 使用 Docker 完成 DEB 封装。

### 开发检查

```bash
npm --prefix web ci
./scripts/sync-version.sh
npm --prefix web run check
(cd backend && go test -race ./... && go vet ./...)
```

### 构建 Docker 镜像

```bash
./scripts/build-docker.sh clash-manager:local
```

这会构建本机架构镜像。多架构构建与 Docker Hub 发布方法见 [Docker 文档](docker/README.md)。

### 构建 DEB

```bash
./scripts/build-deb.sh all     # 或 amd64 / arm64
python3 scripts/audit-deb.py dist/*.deb
```

安装包与 SHA-256 校验文件写入 `dist/`。自动检查和包审计不代表所有目标设备上的安装、升级和宿主 TUN 行为均已验证。

### 版本与仓库维护

`VERSION` 是公共程序版本源，前端 npm 版本由脚本同步，两个 Go 二进制在构建时注入版本。

fnOS 仓库通过 `upstream.lock` 固定本仓库的版本和完整 commit SHA。FPK 构建在隔离的导出目录中注入 fnOS manifest 版本与更新日志，不修改公共工作目录。已安装应用内含编译后的程序，启动时不拉取源码。

公共功能在本仓库维护；fnOS 宿主与打包改动在 fnOS 仓库维护。两个仓库分别提交与推送，源码推送、安装包发布、Docker 镜像发布各自执行。原项目 Git 历史保留，历史中的 FPK 文件不再在本仓库当前目录维护。

详见 [仓库维护指南](docs/repository-maintenance.md) 和 [后端说明](backend/README.md)。

## 常见问题

### 为什么界面没有宿主系统代理开关？

Docker 不提供宿主 Shell 代理环境变量管理。应用或设备可主动使用 Mixed 代理端口；Linux 宿主流量接管使用具备设备和权限的 TUN。

### Host 模式需要端口映射吗？

无需 `ports`。网页和 Core 直接监听宿主端口；可在启动前通过 `.env` 调整，避开已有服务。

### TUN 可以接管哪些流量？

Linux Host 部署可通过 TUN 接管宿主流量。Bridge 的网络空间与宿主独立。其他容器、VPN 或局域网设备的转发流量需要按实际网关、路由与防火墙验证，详细范围见 Docker 文档。

### 会自动更新正在运行的容器吗？

不会。`latest` 指向已发布镜像；升级需要拉取并重新创建容器。源码提交也不会自动更新已经发布的镜像。

## 贡献与致谢

欢迎提交 Issue 与 Pull Request。反馈时提供应用版本、架构、部署方式和相关日志；不要公开订阅 URL、Secret 或密码。

- [MetaCubeX/mihomo](https://github.com/MetaCubeX/mihomo)：Mihomo Core
- [MetaCubeX/meta-rules-dat](https://github.com/MetaCubeX/meta-rules-dat)：GEO 数据
- [Clash Verge Rev](https://github.com/clash-verge-rev/clash-verge-rev)：界面交互与术语参考
- [Clash for fnOS](https://github.com/chenpingonline/Clash-for-fnos)：fnOS 原生集成与 FPK

## 许可证

项目采用 [GPL-3.0](LICENSE)。Mihomo 与其他组件遵循各自许可证，相关文本保留在 `assets/licenses/` 和 `web/public/licenses/`。

---

<div align="center">

如果这个项目对你有帮助，欢迎 Star ⭐

</div>
