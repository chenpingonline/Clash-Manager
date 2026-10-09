# Clash Manager — Docker 部署

简体中文 | [English](README.en.md)

Docker 版现名 **Clash Manager**，原名 **Clash for fnOS / clash-for-fnos**。主仓库为 [chenpingonline/clash-manager](https://hub.docker.com/r/chenpingonline/clash-manager)，[chenpingonline/clash-for-fnos](https://hub.docker.com/r/chenpingonline/clash-for-fnos) 保留为兼容地址；对应版本与 `latest` 发布同一份双架构清单。旧 Compose 和 `/data` 数据目录可继续使用；迁移时只需更换镜像地址，保留服务名和数据卷。

Clash Manager 是本项目的 Docker 版名称，与 fnOS 原生版 Clash for fnOS 共用 Vue、Go Web、Helper 和 Mihomo 管理代码。默认镜像 `chenpingonline/clash-manager:latest` 支持 Linux amd64/arm64，Docker 会自动选择宿主架构，无需在 Compose 中指定 `platform`。发布统一使用 `chenpingonline/clash-manager:<版本号>`，每个版本标签包含双架构清单，不附加架构或修复后缀。Docker 部署支持启动端口环境变量配置，管理密码至少 8 字符。应用版本由 `fpk/manifest` 注入，不另设版本源。

## 默认部署：fnOS/Linux 宿主 TUN

默认 `compose.yaml` 使用 Host 网络并提供 TUN 设备与权限。宿主必须存在 `/dev/net/tun`；设置密码后拉取镜像即可启动，无需本地构建：

```sh
cd docker
cp .env.example .env
# 编辑 .env，设置至少 8 字符的 APP_AUTH_PASSWORD
# 示例端口：网页 17890、Controller 19097、Mixed 代理 17897
# 根据宿主端口占用情况修改 LISTEN_ADDR / APP_CONTROLLER_PORT / APP_MIXED_PORT
docker compose pull
docker compose up -d
```

按示例 `.env` 打开 `http://NAS的IP:17890`，使用 `admin` 与配置的密码登录。默认 Web 监听宿主所有地址；可通过 `LISTEN_ADDR=127.0.0.1:17890` 限制为本机访问。管理端密码与 Mihomo Controller Secret 是不同凭据。管理登录使用有效期 12 小时的 HttpOnly / SameSite=Strict Cookie；退出登录清除当前浏览器 Cookie。修改管理密码会使已有会话失效。

`compose.host.yaml` 保留为相同 Host 配置的兼容入口。选择一个配置启动即可，不要叠加这些 Compose 文件。

Host 模式共享宿主网络，添加 NET_ADMIN 并映射 `/dev/net/tun`，直接占用宿主端口，不使用 ports 映射。默认部署适用于 Linux Docker Engine。macOS/Windows Docker Desktop 的 Host 网络能力不等同于接管物理宿主 TUN。

Host 部署首次初始化的代理端口仅允许本机访问；需要局域网显式代理时，可在网络设置中开启允许局域网。导入的配置若将 allow-lan 关闭，Bridge 的代理端口映射也无法使用，需在网络设置中重新开启。

启动后先导入可用配置并确认代理连接，再在界面开启 TUN。自动路由、自动重定向、出口接口、IPv6、DNS 和排除网段沿用现有管理逻辑。DNS 劫持需先启用并配置 Mihomo DNS。请根据实际网络排除 NAS 管理网段、VPN 和其他需要直连的网络。

`APP_NETWORK_SCOPE=host/container` 是 Compose 对部署范围的声明，仅用于解释 TUN 作用范围，不会切换 Docker 网络，也不是对 Host 网络模式的自动验证。即便有 root 身份，Core 缺少实际 NET_ADMIN 时仍会被判定为不具备 TUN 权限。

Host TUN 不会自动接管整个局域网；设备流量需经 NAS 网关/转发。其他 Docker Bridge 容器的转发流量需结合 Docker 防火墙规则验证，Macvlan/IPvlan 需按实际网关验证。当前没有在真实 fnOS 上确认这些流量路径。

容器正常停止时依次停止 Web、Helper，由 Helper 关闭托管 Mihomo，以便 Mihomo释放 TUN 与自身创建的规则。强制 kill、宿主掉电或内核异常后的网络恢复尚未得到保证；当前实现不会清空宿主防火墙或盲目删除其他服务的路由。不要同时启动两个负责同一宿主 TUN 的实例。

## 普通代理：Bridge 部署

只需让设备或应用主动使用 HTTP/SOCKS 代理时，使用独立的 `compose.bridge.yaml`：

```sh
cd docker
# 先按上文创建 .env 并设置登录密码
# 如需局域网访问，将 WEB_BIND_IP、PROXY_BIND_IP 改为 NAS 的局域网 IP
# 或 0.0.0.0，并按实际需要限制访问来源。
docker compose -f compose.bridge.yaml pull
docker compose -f compose.bridge.yaml up -d
```

按示例 `.env` 打开 http://127.0.0.1:17890，Mixed 代理端口为 17897。Bridge 的网页发布端口由 `WEB_PORT` 控制（默认 17890），容器内 Web 固定使用 8080；`LISTEN_ADDR` 只作用于 Host 配置。首次安装允许访问容器内代理监听器，TUN 默认关闭；设备或应用需主动设置代理。进入 Mihomo 的流量仍可选择规则、全局或直连模式。

Bridge 配置不提供 NET_ADMIN 或 TUN 设备。界面会显示实际检测到的 TUN 权限。所有 Docker 部署均不提供宿主系统代理环境变量开关，飞牛图标与 FPK 更新功能也不可用；默认 Host 配置通过 TUN 接管宿主流量，仍需在网页中手动开启。

从已有 Bridge 部署切换至默认 Host 部署时，保留 `./data` 和 `.env`，执行 `docker compose up -d --force-recreate`。原有的局域网访问设置会保留；`WEB_BIND_IP` 和 `PROXY_BIND_IP` 仅作用于 Bridge，Host 下使用 `LISTEN_ADDR` 控制 Web 监听地址。

## 在 Compose 中指定内核端口

在 `.env` 中集中设置网页与内核启动端口，已有数据目录也会生效，无需先启动内核或进入网页：

```dotenv
LISTEN_ADDR=:17890
APP_CONTROLLER_PORT=19097
APP_MIXED_PORT=17897
```

三份 Compose 均传入内核端口变量；Host 配置另外传入 `LISTEN_ADDR`。Controller 固定监听 `127.0.0.1`；Host 网络下直接使用指定的宿主端口，Bridge 的 Mixed TCP/UDP 映射同步使用 `APP_MIXED_PORT`。端口必须为 1–65535 的整数，并应避开宿主已有服务。

修改后执行 `docker compose up -d --force-recreate`（Bridge 加 `-f compose.bridge.yaml`）。变量在每次容器启动时覆盖对应的已保存端口，并同步写入用户设置，使后续订阅应用保留端口；订阅、Secret 和 TUN 设置不受影响。网页内仍可修改端口，下次容器启动会重新应用环境变量。变量留空或移除时沿用已保存的值，不自动恢复默认端口。

已有部署修改端口后，重新创建容器使环境变量生效。

## 数据、升级与导入

`CLASH_DATA_DIR` 指定数据目录（默认 `./data`），持久化配置、订阅、备份、选择状态、日志、流量历史、GEO 和在线更新的 Core。Compose 默认使用 `latest` 多架构标签，发布新版时由维护者同步更新该标签。升级时保留数据目录，拉取镜像并重新创建容器；运行中的容器不会自行升级：

```sh
docker compose pull
docker compose up -d --force-recreate
```

Bridge 部署在上述两个命令中都加上 `-f compose.bridge.yaml`；使用兼容 Host 入口时加上 `-f compose.host.yaml`。

需要固定版本或回滚时，在 `.env` 中将 `CLASH_IMAGE` 设置为 `chenpingonline/clash-manager:<版本号>`，将占位符替换为 [Docker Hub Tags](https://hub.docker.com/r/chenpingonline/clash-manager/tags) 中所需的已发布版本，再执行相同命令。使用固定版本升级时需自行更换标签。镜像版本通过环境变量选择，无需修改 Compose 文件。

镜像内置与 FPK 相同版本的 Mihomo。更换镜像后通过既有内核升级事务对账：更高版本的内置 Core 会校验并尝试升级，失败时恢复备份；普通重启不覆盖在线升级的 Core，较低或相同版本的内置 Core 不覆盖卷内版本。镜像回退不会自动降级卷内 Core。

需要从宿主目录导入 YAML 时，只挂载指定目录，推荐只读：

```yaml
# 加到 clash 服务下
volumes:
  - "${CLASH_DATA_DIR:-./data}:/data"
  - /path/to/profiles:/imports:ro
environment:
  APP_IMPORT_PATHS: /imports
```

容器内 Web 使用固定 UID/GID 10001；挂载的导入目录必须允许该用户读取。Host 网络不会共享宿主进程或文件系统，当前扫描仅覆盖容器内进程和指定挂载目录。

三份配置均限制 Docker 容器输出日志为每份 10 MB、最多 3 份。此限制不作用于数据目录内的 Mihomo 日志。

## 环境配置

| 变量 | 默认值 / 用途 |
|---|---|
| CLASH_IMAGE | chenpingonline/clash-manager:latest；Compose 镜像地址，可覆盖为固定版本或本地镜像 |
| CLASH_DATA_DIR | 数据目录，默认 ./data；更换路径时需先迁移已有数据 |
| APP_PLATFORM | 镜像内为 docker，原生 FPK 为 fnos |
| LISTEN_ADDR | 仅 Host 使用；未设置时为 :8080，示例 .env 为 :17890 |
| WEB_PORT | 仅 Bridge 使用，宿主网页端口，默认 17890 |
| WEB_BIND_IP / PROXY_BIND_IP | 仅 Bridge 配置使用，默认 127.0.0.1 |
| APP_CONTROLLER_PORT | 留空沿用已保存端口；示例 .env 使用 19097 避开常见的 9090 冲突 |
| APP_MIXED_PORT | 留空沿用已保存端口；示例 .env 使用 17897 |
| APP_AUTH_USER | admin |
| APP_AUTH_PASSWORD | HTTP/Docker 部署必填，至少 8 字符 |
| APP_AUTH_PASSWORD_FILE | 可替代密码变量；文件需允许 UID 10001 读取 |
| APP_AUTH_COOKIE_SECURE | HTTPS 反向代理部署时设为 1；HTTP 下无法使用 Secure Cookie |
| APP_CONFIG_DIR | /data/config |
| APP_STATE_DIR | /data/state |
| APP_IMPORT_PATHS | 显式允许导入的容器内目录，多个目录以冒号分隔 |
| APP_NETWORK_SCOPE | 默认与兼容 Host 配置为 host；Bridge 配置为 container |
| GATEWAY_PREFIX | Docker 默认为空，fnOS 保留 /app/clash-for-fnos |

远程部署可接入 HTTPS 反向代理，保留原始 Host，并将 `APP_AUTH_COOKIE_SECURE=1`。密码文件可通过 Docker secrets 挂载，避免密码直接写入 Compose。启动器会降权运行 Web，Helper 保留所需权限，内部 Socket 不对外发布；无需 privileged 或 Docker Socket。

## 开发验证与功能同步

```sh
cd backend && go test ./... && go vet ./...
# 回项目根目录
npm --prefix web run check
./scripts/build-docker.sh clash-manager:local
python3 scripts/docker-smoke.py clash-manager:local
# 验证启动端口覆盖和已有数据卷重建
python3 scripts/docker-smoke.py clash-manager:local --controller-port 19090 --mixed-port 17890
# 在独立的容器网络验证 TUN，不修改宿主网络
python3 scripts/docker-smoke.py clash-manager:local --tun
```

可使用本机 Node/Go 编译工具构建同一运行镜像，减少构建器镜像下载（仍需安装镜像内运行依赖）：

```sh
./scripts/build-docker.sh --local-build clash-manager:local
```

需要使用 Compose 运行本地镜像时，在项目根目录执行：

```sh
# docker/.env 中仍需设置登录密码
CLASH_IMAGE=clash-manager:local docker compose -f docker/compose.bridge.yaml up -d --pull never
# Linux 宿主 TUN 部署将文件名改为 docker/compose.yaml
```

Dockerfile 位于 `docker/Dockerfile`，构建上下文保持为项目根目录，使用根目录的 `.dockerignore`。在项目根目录可以直接构建：

```sh
docker build -f docker/Dockerfile -t clash-manager:local .
```

有 Buildx 时可直接构建双架构：

```sh
docker buildx build -f docker/Dockerfile --platform linux/amd64,linux/arm64 -t your-registry/clash:version --push .
```

功能在同一主分支维护；运行环境差异集中在 runtimeenv、部署入口与能力接口中。Docker 开发分支验证后合回 master，后续共用功能应同时经过 Go 测试、前端检查和容器冒烟验证。真实 fnOS 的 Host TUN、DNS、IPv6、Docker 转发共存及异常退出恢复仍需真机验收。

## 新旧仓库同步发布

发布时将同一份多架构构建结果同时标记为两个仓库的版本标签和 `latest`，避免兼容地址落后。版本仍从 `fpk/manifest` 读取：

```sh
VERSION=$(awk -F= '/^version[[:space:]]*=/{gsub(/[[:space:]]/,"",$2);print $2;exit}' fpk/manifest)
docker buildx build --platform linux/amd64,linux/arm64 --push \
  -f docker/Dockerfile \
  -t "chenpingonline/clash-manager:$VERSION" \
  -t chenpingonline/clash-manager:latest \
  -t "chenpingonline/clash-for-fnos:$VERSION" \
  -t chenpingonline/clash-for-fnos:latest .
```

Docker Hub 简介与 Overview 同时包含 Clash Manager、Clash for fnOS、clash-manager、clash-for-fnos 名称。搜索索引可能延迟，直接访问仓库地址不依赖搜索更新。
