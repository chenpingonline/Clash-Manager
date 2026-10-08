# Docker 部署

Docker 与 fnOS FPK 共用 Vue、Go Web、Helper 和 Mihomo 管理代码。当前镜像支持 Linux amd64/arm64；版本由 `fpk/manifest` 注入，不另设版本源。

## 普通代理

在项目根目录构建，然后进入部署目录：

```sh
./scripts/build-docker.sh clash-for-fnos:local
cd docker
cp .env.example .env
# 编辑 .env，设置至少 12 字符的 APP_AUTH_PASSWORD
# 如需局域网访问，将 WEB_BIND_IP、PROXY_BIND_IP 改为 NAS 的局域网 IP
# 或 0.0.0.0，并按实际需要限制访问来源。
docker compose up -d
```

默认打开 http://127.0.0.1:8080，使用 `admin` 与配置的密码登录。管理端密码与 Mihomo Controller Secret 是不同凭据。管理登录使用有效期 12 小时的 HttpOnly / SameSite=Strict Cookie；退出登录清除当前浏览器 Cookie。修改管理密码会使已有会话失效。

首次安装会启用容器内托管 Mihomo，Mixed Port 为 7890，允许访问该代理监听器，TUN 默认关闭。设备或应用需主动设置 HTTP/SOCKS 代理才能使用它。进入 Mihomo 的流量仍可选择规则、全局或直连模式。

Bridge 部署默认不提供 NET_ADMIN 或 TUN 设备。界面会显示实际检测到的 TUN 权限；系统代理环境变量、飞牛图标与 FPK 更新功能不可用。

## fnOS/Linux 宿主 TUN

使用独立的 Host 配置，不要将它与普通代理配置叠加：

```sh
cd docker
docker compose -f compose.host.yaml up -d
```

Host 模式共享宿主网络，添加 NET_ADMIN 并映射 `/dev/net/tun`，直接占用宿主端口，不使用 ports 映射。默认部署适用于 Linux Docker Engine。macOS/Windows Docker Desktop 的 Host 网络能力不等同于接管物理宿主 TUN。

Host 部署首次初始化的代理端口仅允许本机访问；需要局域网显式代理时，可在网络设置中开启允许局域网。导入的配置若将 allow-lan 关闭，Bridge 的代理端口映射也无法使用，需在网络设置中重新开启。

启动后先导入可用配置并确认代理连接，再在界面开启 TUN。自动路由、自动重定向、出口接口、IPv6、DNS 和排除网段沿用现有管理逻辑。DNS 劫持需先启用并配置 Mihomo DNS。请根据实际网络排除 NAS 管理网段、VPN 和其他需要直连的网络。

`APP_NETWORK_SCOPE=host/container` 是 Compose 对部署范围的声明，仅用于解释 TUN 作用范围，不会切换 Docker 网络，也不是对 Host 网络模式的自动验证。即便有 root 身份，Core 缺少实际 NET_ADMIN 时仍会被判定为不具备 TUN 权限。

Host TUN 不会自动接管整个局域网；设备流量需经 NAS 网关/转发。其他 Docker Bridge 容器的转发流量需结合 Docker 防火墙规则验证，Macvlan/IPvlan 需按实际网关验证。当前没有在真实 fnOS 上确认这些流量路径。

容器正常停止时依次停止 Web、Helper，由 Helper 关闭托管 Mihomo，以便 Mihomo释放 TUN 与自身创建的规则。强制 kill、宿主掉电或内核异常后的网络恢复尚未得到保证；当前实现不会清空宿主防火墙或盲目删除其他服务的路由。不要同时启动两个负责同一宿主 TUN 的实例。

## 数据、升级与导入

`./data` 持久化配置、订阅、备份、选择状态、日志、流量历史、GEO 和在线更新的 Core。升级时保留此目录，替换镜像并重新创建容器：

```sh
# 拉取已发布的镜像；当前尚未发布，可在项目根目录重新构建 local 镜像
# docker compose pull
docker compose up -d --force-recreate
```

镜像内置与 FPK 相同版本的 Mihomo。更换镜像后通过既有内核升级事务对账：更高版本的内置 Core 会校验并尝试升级，失败时恢复备份；普通重启不覆盖在线升级的 Core，较低或相同版本的内置 Core 不覆盖卷内版本。镜像回退不会自动降级卷内 Core。

需要从宿主目录导入 YAML 时，只挂载指定目录，推荐只读：

```yaml
# 加到 clash 服务下
volumes:
  - ./data:/data
  - /path/to/profiles:/imports:ro
environment:
  APP_IMPORT_PATHS: /imports
```

容器内 Web 使用固定 UID/GID 10001；挂载的导入目录必须允许该用户读取。Host 网络不会共享宿主进程或文件系统，当前扫描仅覆盖容器内进程和指定挂载目录。

## 环境配置

| 变量 | 默认值 / 用途 |
|---|---|
| APP_PLATFORM | 镜像内为 docker，原生 FPK 为 fnos |
| LISTEN_ADDR | :8080；端口变化时同步调整 Bridge ports |
| APP_AUTH_USER | admin |
| APP_AUTH_PASSWORD | HTTP/Docker 部署必填，至少 12 字符 |
| APP_AUTH_PASSWORD_FILE | 可替代密码变量；文件需允许 UID 10001 读取 |
| APP_AUTH_COOKIE_SECURE | HTTPS 反向代理部署时设为 1；HTTP 下无法使用 Secure Cookie |
| APP_CONFIG_DIR | /data/config |
| APP_STATE_DIR | /data/state |
| APP_IMPORT_PATHS | 显式允许导入的容器内目录，多个目录以冒号分隔 |
| APP_NETWORK_SCOPE | container；Host Compose 为 host |
| GATEWAY_PREFIX | Docker 默认为空，fnOS 保留 /app/clash-for-fnos |

远程部署可接入 HTTPS 反向代理，保留原始 Host，并将 `APP_AUTH_COOKIE_SECURE=1`。密码文件可通过 Docker secrets 挂载，避免密码直接写入 Compose。启动器会降权运行 Web，Helper 保留所需权限，内部 Socket 不对外发布；无需 privileged 或 Docker Socket。

## 开发验证与功能同步

```sh
cd backend && go test ./... && go vet ./...
# 回项目根目录
npm --prefix web run check
./scripts/build-docker.sh clash-for-fnos:local
python3 scripts/docker-smoke.py clash-for-fnos:local
# 在独立的容器网络验证 TUN，不修改宿主网络
python3 scripts/docker-smoke.py clash-for-fnos:local --tun
```

可使用本机 Node/Go 编译工具构建同一运行镜像，减少构建器镜像下载（仍需安装镜像内运行依赖）：

```sh
./scripts/build-docker.sh --local-build clash-for-fnos:local
```

Dockerfile 位于 `docker/Dockerfile`，构建上下文保持为项目根目录，使用根目录的 `.dockerignore`。在项目根目录可以直接构建：

```sh
docker build -f docker/Dockerfile -t clash-for-fnos:local .
```

有 Buildx 时可直接构建双架构：

```sh
docker buildx build -f docker/Dockerfile --platform linux/amd64,linux/arm64 -t your-registry/clash:version --push .
```

功能在同一主分支维护；运行环境差异集中在 runtimeenv、部署入口与能力接口中。Docker 开发分支验证后合回 master，后续共用功能应同时经过 Go 测试、前端检查和容器冒烟验证。真实 fnOS 的 Host TUN、DNS、IPv6、Docker 转发共存及异常退出恢复仍需真机验收。
