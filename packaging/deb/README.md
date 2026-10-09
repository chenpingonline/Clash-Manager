# Linux DEB 安装 / Linux DEB installation

简体中文 | [English](README.en.md)

支持 Debian 12/13、Ubuntu 22.04/24.04 等使用 systemd 的 amd64/arm64 系统。实际测试范围见构建记录；其他衍生系统需自行验证。包含与 Docker/fnOS 共用的管理界面、Mihomo 和 GEO 数据。

## 安装与访问

从项目 dist 选择对应架构的 DEB；例如：

```bash
sudo apt install ./clash-manager_1.3.6_amd64.deb
sudo systemctl status clash-manager
sudo cat /etc/clash-manager/password
```

默认地址 http://127.0.0.1:8080，用户名 admin，首次安装自动生成随机密码。密码不会输出到安装日志。远程可通过 SSH 隧道访问：

```bash
ssh -L 8080:127.0.0.1:8080 user@server
```

在本机浏览器打开 http://127.0.0.1:8080。也可编辑 `/etc/clash-manager/clash-manager.conf` 的 LISTEN_ADDR 为 `0.0.0.0:8080`；远程 HTTP 建议配合 HTTPS 反向代理。

## 设置与服务

Web 使用无登录权限的 clash-manager 用户，Helper 以 root 运行，通过权限受限的 Unix socket 管理 Mihomo。前端不会提供 fnOS 软件图标、FPK 更新和 fnOS Shell 代理环境变量管理；Linux 使用托管 Core。

编辑 `/etc/clash-manager/clash-manager.conf`，可设置 APP_AUTH_USER、APP_AUTH_PASSWORD_FILE、LISTEN_ADDR、APP_CONTROLLER_PORT、APP_MIXED_PORT 和 APP_IMPORT_PATHS。密码文件至少 8 字符，且需允许 clash-manager 用户读取。默认密码文件 root:clash-manager 0640。修改后重启：

```bash
sudo systemctl restart clash-manager
sudo journalctl -u clash-manager -u clash-manager-helper -f
sudo systemctl stop clash-manager
```

默认 TUN 关闭。开启会接管宿主流量，需要系统提供 `/dev/net/tun`；不自动创建设备或修改内核模块。服务停止时先停止 Web，再让 Helper 关闭托管 Mihomo，释放监听端口；正常退出由 Mihomo 清理 TUN 和路由。断电、SIGKILL 和内核故障不等同于正常退出。

数据保存在 `/var/lib/clash-manager`，升级保留数据和用户修改的 conffile；密码不会因升级重新生成。Core 默认 Controller 127.0.0.1:9090、Mixed 7890；占用时通过配置文件启动覆写后重启。服务由 systemd 管理并默认开机启动；可用 systemctl disable clash-manager 关闭开机启动。无 systemd PID 1 的环境不会自动启动。

## 升级与卸载

```bash
sudo apt install ./clash-manager_<新版本>_amd64.deb
sudo apt remove clash-manager
# 清除包配置和登录密码；仍保留 /var/lib/clash-manager 数据及系统用户
sudo apt purge clash-manager
```

升级/卸载先停止两个服务。完全删除数据前请自行备份；重新安装时保留的数据可能恢复之前的 TUN 配置。

## 构建

```bash
npm --prefix web ci
./scripts/build-deb.sh all     # 或 amd64 / arm64
```

版本来自根目录 VERSION，输出到 dist，带 SHA256。需要 Go、Node/npm、Python 3 和 dpkg-deb；macOS 上自动使用 debian:12-slim Docker 容器完成 DEB 封装。程序二进制静态交叉编译；Mihomo 使用项目已有架构资源。此流程构建二进制 DEB，不是向 Debian 官方仓库上传的源码包。
