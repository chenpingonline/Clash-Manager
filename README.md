# Clash Manager

简体中文 | [English](README.en.md)

面向 Linux / Docker 的 Mihomo 管理程序，也是 [Clash for fnOS](https://github.com/chenpingonline/Clash-for-fnos) 使用的公共源码。

提供节点、订阅、规则、配置编辑、连接、日志、DNS、TUN、Core 与 GEO 管理，支持简体中文、English 和跟随系统。

## Linux DEB 安装

支持 amd64/arm64，提供 systemd 服务、独立登录和托管 Mihomo。安装、升级与构建见 [DEB 使用说明](packaging/deb/README.md)。

## Docker 部署

镜像：`chenpingonline/clash-manager`，支持 Linux amd64 / arm64。已有镜像不会因源码迁移自动更新；构建与发布方法见 [Docker 使用说明](docker/README.md)。

```bash
cp docker/.env.example docker/.env
# 编辑 docker/.env，设置至少 8 字符的管理密码。
docker compose --env-file docker/.env -f docker/compose.yaml up -d
```

默认 Compose 使用 Linux Host 网络，TUN 需要 NET_ADMIN 与 /dev/net/tun；其他部署方式见 Docker 文档。

## 开发

需要 Go、Node.js（满足 web/package.json engines）和 npm。

```bash
npm --prefix web ci
./scripts/sync-version.sh
npm --prefix web run check
(cd backend && go test -race ./... && go vet ./...)
./scripts/build-docker.sh clash-manager:local
```

`VERSION` 是公共程序唯一版本源。前端默认以 `/` 构建；fnOS 打包方使用 `/app/clash-for-fnos/`，并在隔离构建目录注入 FPK 版本和更新日志。Linux 原生 DEB 使用独立的 systemd 生命周期和打包脚本。

## 两个仓库怎么维护

- 本仓库维护公共 Vue / Go / 多语言与 Docker / Linux DEB 构建；保留 fnOS 能力适配，保证只有一份业务实现。
- fnOS 仓库维护 FPK 生命周期、宿主配置、窗口入口、图标及原生发布。
- fnOS 的 upstream.lock 固定本仓库的版本与完整 commit SHA；打包时下载该提交，编译后装入 FPK。
- 公共功能只在本仓库修改；发布并验证后，fnOS 更新版本记录。两个仓库各用 master，各自提交、推送。
- 原项目历史保留在此仓库；旧提交中的 FPK 文件仅属于历史，不再在当前目录维护。

详见 [仓库维护指南](docs/repository-maintenance.md)、[操作手册](docs/user-guide.md)、[多语言指南](docs/i18n.md) 和 [后端说明](backend/README.md)。

## 许可证

项目采用 [GPL-3.0](LICENSE)。Mihomo 与其他组件的许可证保留在 assets/licenses 和 web/public/licenses。
