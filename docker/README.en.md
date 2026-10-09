# Clash Manager — Docker deployment

[简体中文](README.md) | English

The Docker edition is called **Clash Manager**, formerly **Clash for fnOS / clash-for-fnos**. The main repository is [chenpingonline/clash-manager](https://hub.docker.com/r/chenpingonline/clash-manager); [chenpingonline/clash-for-fnos](https://hub.docker.com/r/chenpingonline/clash-for-fnos) remains a compatibility address. Matching version tags and `latest` publish the same multi-architecture manifest. Existing Compose services and `/data` remain compatible: change the image address while keeping the service and volume.

Docker shares Vue, Go Web, Helper and Mihomo management code with the native fnOS edition. `chenpingonline/clash-manager:latest` supports Linux amd64/arm64. Docker selects the architecture automatically; Compose needs no `platform`. Version tags use `chenpingonline/clash-manager:<version>`, with no architecture or repair suffix. Startup ports can be configured through environment variables; management passwords require at least 8 characters. The application version comes from `VERSION`.

The UI supports Chinese and English. Select a language under Settings → Language (top right); the preference persists in the browser. Follow system uses the browser language. Proxy/profile names and YAML/log content are preserved. See [translation maintenance](../docs/i18n.md).

## Default deployment: Linux/fnOS host TUN

Default `compose.yaml` uses Host networking and provides the TUN device and permissions. The host must have `/dev/net/tun`. Set a password, pull and start without building locally:

```sh
cd docker
cp .env.example .env
# Edit .env: set APP_AUTH_PASSWORD to at least 8 characters.
# Example ports: Web 17890, Controller 19097, Mixed 17897.
# Adjust LISTEN_ADDR / APP_CONTROLLER_PORT / APP_MIXED_PORT to avoid host conflicts.
docker compose pull
docker compose up -d
```

With the example `.env`, open `http://<NAS-IP>:17890` and sign in with `admin` and your password. Web listens on all host addresses by default; set `LISTEN_ADDR=127.0.0.1:17890` for local-only access. Management password and Mihomo Controller Secret are separate credentials. Login uses a 12-hour HttpOnly / SameSite=Strict cookie; logout clears the current browser cookie. Changing the password invalidates existing sessions.

`compose.host.yaml` is a compatibility entry point with the same Host configuration. Choose one Compose file; do not combine them.

Host mode shares the host network, grants NET_ADMIN, maps `/dev/net/tun` and binds host ports directly, without `ports` mappings. This is for Linux Docker Engine. Docker Desktop Host networking on macOS/Windows does not provide equivalent control over the physical host's TUN traffic.

Initial Host proxy listeners accept only local connections. Enable **Allow LAN** in network settings for explicit LAN proxy clients. Imported configurations with `allow-lan: false` also prevent access through Bridge proxy port mappings; enable it again in network settings.

Import a usable configuration and confirm proxy connectivity before enabling TUN. Auto route, auto redirect, outbound interface, IPv6, DNS and exclusions use the shared management logic. Configure and enable Mihomo DNS before DNS hijacking. Exclude management networks, VPNs and other networks that should remain direct as appropriate.

`APP_NETWORK_SCOPE=host/container` declares the intended scope for explanatory UI text. It neither changes Docker networking nor verifies Host mode automatically. Even root Core processes without actual NET_ADMIN are reported as lacking TUN permission.

Host TUN does not automatically route the entire LAN. Other devices must route/forward through the NAS. Forwarded traffic from other Bridge containers must be verified against Docker firewall rules; Macvlan/IPvlan depend on actual gateway configuration. These paths have not been confirmed on real fnOS devices.

Normal shutdown stops Web, then Helper, which stops managed Mihomo so it can release TUN and its own rules. Recovery after forced kills, power loss or kernel failures is not guaranteed. The application does not flush host firewall rules or delete unrelated routes. Do not run two instances responsible for the same host TUN.

## Explicit proxy: Bridge deployment

Use separate `compose.bridge.yaml` when clients/applications will explicitly configure HTTP/SOCKS proxy settings:

```sh
cd docker
# Create .env and set the login password first.
# For LAN access, set WEB_BIND_IP and PROXY_BIND_IP to the NAS LAN IP
# or 0.0.0.0, and restrict access sources as appropriate.
docker compose -f compose.bridge.yaml pull
docker compose -f compose.bridge.yaml up -d
```

The example opens Web at `http://127.0.0.1:17890` and Mixed proxy at 17897. Bridge Web's host port uses `WEB_PORT` (default 17890), with 8080 inside the container. `LISTEN_ADDR` applies only to Host configurations. Initial Bridge settings allow access to the container proxy listener and leave TUN disabled. Clients must configure the proxy explicitly. Traffic entering Mihomo can use Rule, Global or Direct mode.

Bridge Compose provides neither NET_ADMIN nor a TUN device. The interface reports actual TUN permissions. All Docker deployments omit host proxy environment-variable controls, fnOS icon changes and FPK updates. Default Host deployment can route host traffic through TUN once enabled in the UI.

To move from Bridge to default Host, retain `./data` and `.env`, then run `docker compose up -d --force-recreate`. Existing LAN settings persist. `WEB_BIND_IP` / `PROXY_BIND_IP` are Bridge-only; Host Web binding uses `LISTEN_ADDR`.

## Configure Core ports before startup

Set Web and Core startup ports in `.env`; this also works with existing data directories and does not require a running Core or web UI:

```dotenv
LISTEN_ADDR=:17890
APP_CONTROLLER_PORT=19097
APP_MIXED_PORT=17897
```

All three Compose files pass the Core port variables; Host files also pass `LISTEN_ADDR`. Controller always listens on `127.0.0.1`. Host binds directly to the specified ports; Bridge's Mixed TCP/UDP mappings follow `APP_MIXED_PORT`. Ports must be integers from 1 to 65535 and must avoid existing services.

Recreate after changes with `docker compose up -d --force-recreate`, adding `-f compose.bridge.yaml` for Bridge. Each startup overrides the saved values for specified ports and records them as user preferences, preserving ports after profile application. Profiles, Secret and TUN settings are unaffected. UI changes remain possible, but the next container startup reapplies environment values. Empty/removed variables retain saved values rather than resetting to defaults.

## Data, upgrades and imports

`CLASH_DATA_DIR` selects the data directory (default `./data`), holding configurations, profiles, backups, selections, logs, traffic history, GEO and online-updated Core. Compose defaults to multi-architecture `latest`, which maintainers update when publishing. Running containers do not upgrade automatically. Keep the data directory, pull and recreate:

```sh
docker compose pull
docker compose up -d --force-recreate
```

Add `-f compose.bridge.yaml` to both commands for Bridge or `-f compose.host.yaml` for the compatibility Host entry point.

To pin or roll back, set `CLASH_IMAGE=chenpingonline/clash-manager:<version>` in `.env`, replacing the placeholder with a published [Docker Hub tag](https://hub.docker.com/r/chenpingonline/clash-manager/tags), then run the same commands. Pinned deployments need a tag change for upgrades; Compose itself need not be edited.

Images bundle the same Core as FPKs. Image replacement reconciles Core through the existing upgrade transaction: a newer bundled Core is validated and installed, restoring backups on failure. Ordinary restarts retain online-updated Core; equal/older bundled versions do not replace volume Core. Rolling back an image does not automatically downgrade volume Core.

Mount only the directory you want to import, preferably read-only:

```yaml
# Under the clash service:
volumes:
  - "${CLASH_DATA_DIR:-./data}:/data"
  - /path/to/profiles:/imports:ro
environment:
  APP_IMPORT_PATHS: /imports
```

Web runs as UID/GID 10001; import directories must be readable by that user. Host networking does not share host processes/filesystems; scanning covers container processes and explicitly mounted directories.

Each Compose file limits Docker output logs to 10 MB per file, three files. This does not limit Mihomo logs in the data directory.

## Environment variables

| Variable | Default / purpose |
| --- | --- |
| CLASH_IMAGE | chenpingonline/clash-manager:latest; pin a version or select a local image |
| CLASH_DATA_DIR | ./data; migrate existing data before changing |
| APP_PLATFORM | docker in images, fnos in native FPKs |
| LISTEN_ADDR | Host only; :8080 if unset; example .env uses :17890 |
| WEB_PORT | Bridge Web host port; default 17890 |
| WEB_BIND_IP / PROXY_BIND_IP | Bridge only; default 127.0.0.1 |
| APP_CONTROLLER_PORT | Empty preserves saved port; example 19097 avoids common 9090 conflicts |
| APP_MIXED_PORT | Empty preserves saved port; example 17897 |
| APP_AUTH_USER | admin |
| APP_AUTH_PASSWORD | Required for HTTP/Docker; at least 8 characters |
| APP_AUTH_PASSWORD_FILE | Password-file alternative; must be readable by UID 10001 |
| APP_AUTH_COOKIE_SECURE | Set to 1 behind HTTPS; Secure cookies cannot be used over HTTP |
| APP_CONFIG_DIR | /data/config |
| APP_STATE_DIR | /data/state |
| APP_IMPORT_PATHS | Explicit container import directories, separated by colons |
| APP_NETWORK_SCOPE | host for default/compatibility Host; container for Bridge |
| GATEWAY_PREFIX | Empty for Docker; /app/clash-for-fnos for fnOS |

For remote access, use an HTTPS reverse proxy that preserves the original Host and set `APP_AUTH_COOKIE_SECURE=1`. Password files can use Docker secrets. The launcher drops Web privileges while Helper retains required permissions. Its socket is private; neither privileged mode nor the Docker socket is required.

## Development checks and shared features

```sh
cd backend && go test ./... && go vet ./...
# Return to the project root:
npm --prefix web run check
./scripts/build-docker.sh clash-manager:local
python3 scripts/docker-smoke.py clash-manager:local
# Startup port overrides and recreation with an existing volume:
python3 scripts/docker-smoke.py clash-manager:local --controller-port 19090 --mixed-port 17890
# Isolated container TUN; does not modify the host network:
python3 scripts/docker-smoke.py clash-manager:local --tun
```

Build with locally installed Node/Go to reduce builder image downloads; runtime dependencies still need installation:

```sh
./scripts/build-docker.sh --local-build clash-manager:local
```

Run a local image with Compose from the project root:

```sh
# docker/.env must still contain the management password.
CLASH_IMAGE=clash-manager:local docker compose -f docker/compose.bridge.yaml up -d --pull never
# For Linux host TUN, use docker/compose.yaml.
```

Dockerfile is under `docker/`, with the repository root as context and root `.dockerignore`:

```sh
docker build -f docker/Dockerfile -t clash-manager:local .
# With Buildx, publish both architectures:
docker buildx build -f docker/Dockerfile --platform linux/amd64,linux/arm64 -t your-registry/clash:version --push .
```

Features stay in the shared main branch; runtime differences are concentrated in runtimeenv, launchers and capabilities. Merge the validated Docker branch into master, then check shared changes with Go tests, frontend checks and container smoke tests. Real fnOS Host TUN, DNS, IPv6, Docker forwarding coexistence and abnormal-shutdown recovery still require device acceptance.

## Publish both repository names together

Tag the same multi-architecture build with version and `latest` in both repositories to keep the compatibility address current. Version still comes from the manifest:

```sh
VERSION=$(cat VERSION)
docker buildx build --platform linux/amd64,linux/arm64 --push \
  -f docker/Dockerfile \
  -t "chenpingonline/clash-manager:$VERSION" \
  -t chenpingonline/clash-manager:latest \
  -t "chenpingonline/clash-for-fnos:$VERSION" \
  -t chenpingonline/clash-for-fnos:latest .
```

Docker Hub descriptions/Overviews include Clash Manager, Clash for fnOS, clash-manager and clash-for-fnos. Search indexing may lag; direct repository URLs work independently of search.
