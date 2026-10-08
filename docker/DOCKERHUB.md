# Clash for fnOS — Docker 部署

基于 Mihomo 的代理管理界面，支持订阅、代理节点、规则、连接、日志与 TUN 设置。

镜像支持 **Linux AMD64 / ARM64**，Docker 自动选择架构，无需指定 `platform`。默认使用 `chenpingonline/clash-for-fnos:latest` 获取最新稳定版；已发布版本见 [Tags](https://hub.docker.com/r/chenpingonline/clash-for-fnos/tags)。

## 快速部署：Linux / fnOS Host 网络

在同一个目录保存下面的 `compose.yaml` 和 `.env`，填写管理密码，然后执行启动命令。宿主需有 `/dev/net/tun`。

### compose.yaml

```yaml
# Default Linux/fnOS deployment; enable TUN manually in the Web UI.
services:
  clash:
    image: ${CLASH_IMAGE:-chenpingonline/clash-for-fnos:latest}
    init: true
    restart: unless-stopped
    stop_grace_period: 40s
    network_mode: host
    cap_add:
      - NET_ADMIN
    devices:
      - /dev/net/tun:/dev/net/tun
    environment:
      # Host 模式直接使用宿主端口，无需 ports 映射。
      LISTEN_ADDR: "${LISTEN_ADDR:-:8080}"
      # 非空端口变量在内核启动前生效，已有数据目录也会应用。
      APP_CONTROLLER_PORT: "${APP_CONTROLLER_PORT:-}"
      APP_MIXED_PORT: "${APP_MIXED_PORT:-}"
      APP_AUTH_USER: ${APP_AUTH_USER:-admin}
      APP_AUTH_PASSWORD: ${APP_AUTH_PASSWORD:?Set a password of at least 8 characters in .env}
      # 与上方 network_mode 保持一致。
      APP_NETWORK_SCOPE: host
    volumes:
      - "${CLASH_DATA_DIR:-./data}:/data"
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "3"
```

### .env.example

保存为 `.env`，设置至少 8 字符的 `APP_AUTH_PASSWORD`，不要保留为空。

```dotenv
# 管理登录：启动前必须设置至少 8 字符的密码。
APP_AUTH_USER=admin
APP_AUTH_PASSWORD=

# Host 部署：实际占用的 NAS 端口，无需配置端口映射。
# 网页允许局域网访问；仅允许本机访问时改为 127.0.0.1:17890。
LISTEN_ADDR=:17890
APP_CONTROLLER_PORT=19097
APP_MIXED_PORT=17897

# 内核端口变量在每次容器启动时生效，已有数据目录也会应用。
# APP_CONTROLLER_PORT / APP_MIXED_PORT 留空时沿用已保存的端口。
# 其他设备访问代理，还需在网页开启“允许局域网连接”。

# 可选：固定版本或回滚时取消注释，并填写 Tags 页中已发布的版本号。
# CLASH_IMAGE=chenpingonline/clash-for-fnos:<版本号>
# 可选：默认保存到 ./data，也可以使用 NAS 上的绝对目录。
# CLASH_DATA_DIR=./data

# 仅 compose.bridge.yaml 使用：宿主网页端口与端口映射监听地址。
# Bridge 容器内 Web 固定监听 8080，LISTEN_ADDR 仅作用于 Host 配置。
# WEB_PORT=17890
# WEB_BIND_IP=127.0.0.1
# PROXY_BIND_IP=127.0.0.1
```

默认配置使用 `latest`，无需在 `.env` 中额外指定镜像。需要固定版本或回滚时，取消 `.env` 中 `CLASH_IMAGE` 的注释，将 `<版本号>` 替换为 [Tags](https://hub.docker.com/r/chenpingonline/clash-for-fnos/tags) 中所需的已发布版本。版本标签同样支持双架构。

### 启动

```bash
docker compose pull
docker compose up -d
```

按上述端口设置访问 `http://NAS的IP:17890`，用户名为 `admin`，密码为 `.env` 中填写的值。默认配置和订阅持久化到当前目录的 `./data`。

Host 模式直接占用宿主端口，无需 `ports` 映射。网页端口由 `LISTEN_ADDR` 设置；Controller 端口由 `APP_CONTROLLER_PORT` 设置；HTTP/SOCKS Mixed 端口由 `APP_MIXED_PORT` 设置。请避开已有服务占用的端口，Controller 仅监听本机回环地址。

导入订阅或配置后确认代理可用，再在网页开启 TUN。首次安装 TUN 默认关闭；NET_ADMIN 和 TUN 设备提供启用所需能力。TUN 会改变共享网络中的路由与 DNS 流向，需按实际网络配置直连网段。其他局域网设备不会仅因启动容器而自动经过代理。macOS/Windows Docker Desktop 的网络环境不能等同于 Linux 物理宿主。

## 普通 HTTP/SOCKS 代理：Bridge 网络

只需要显式代理时，将以下内容保存为 `compose.bridge.yaml`，使用相同 `.env`。此配置不提供 TUN 设备或 NET_ADMIN。

```yaml
# Standalone Bridge deployment for explicit HTTP/SOCKS proxy use.
services:
  clash:
    image: ${CLASH_IMAGE:-chenpingonline/clash-for-fnos:latest}
    init: true
    restart: unless-stopped
    stop_grace_period: 40s
    environment:
      # 非空端口变量在内核启动前生效，已有数据目录也会应用。
      APP_CONTROLLER_PORT: "${APP_CONTROLLER_PORT:-}"
      APP_MIXED_PORT: "${APP_MIXED_PORT:-}"
      APP_AUTH_USER: ${APP_AUTH_USER:-admin}
      APP_AUTH_PASSWORD: ${APP_AUTH_PASSWORD:?Set a password of at least 8 characters in .env}
      APP_NETWORK_SCOPE: container
    ports:
      - "${WEB_BIND_IP:-127.0.0.1}:${WEB_PORT:-17890}:8080"
      - "${PROXY_BIND_IP:-127.0.0.1}:${APP_MIXED_PORT:-7890}:${APP_MIXED_PORT:-7890}/tcp"
      - "${PROXY_BIND_IP:-127.0.0.1}:${APP_MIXED_PORT:-7890}:${APP_MIXED_PORT:-7890}/udp"
    volumes:
      - "${CLASH_DATA_DIR:-./data}:/data"
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "3"
```

```bash
docker compose -f compose.bridge.yaml pull
docker compose -f compose.bridge.yaml up -d
```

网页默认为 `http://127.0.0.1:17890`，代理为 `127.0.0.1:17897`（使用上面的 `.env`）。其他设备需要访问时，在 `.env` 中将 `WEB_BIND_IP`、`PROXY_BIND_IP` 设置为 NAS 的局域网地址或 `0.0.0.0`，并在网页网络设置中开启“允许局域网连接”。Host 配置仅使用 `LISTEN_ADDR` 控制网页监听地址。

## 升级与停止

保留 `.env` 与 `data`；使用固定版本时先修改 `CLASH_IMAGE` 为已发布的新版本。

```bash
docker compose pull
docker compose up -d --force-recreate
```

Bridge 部署为上述命令添加 `-f compose.bridge.yaml`。正常停止使用 `docker compose stop` 或 `docker compose down`，容器会依次关闭 Web、Helper 和托管 Mihomo，释放应用自身端口。隔离容器测试已验证 TUN 路由安装与正常关闭后的恢复；真实 fnOS Host 网络、强制终止或宿主掉电后的恢复需按部署环境验收。

Docker 部署通过 TUN 接管共享网络流量，界面不提供 fnOS 系统代理环境变量开关。

## 项目

[GitHub 源码与使用说明](https://github.com/chenpingonline/Clash-for-fnos)

Docker Hub Overview 提供可复制的部署配置；`docker pull` 只获取镜像，不下载 Compose 或 `.env` 文件。
