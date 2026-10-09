<div align="center">

<img src="web/public/icon-current-256.png" alt="Clash Manager" width="128" />

# Clash Manager

[简体中文](README.md) | English

**A Mihomo / Clash manager for Linux and Docker**

Manage proxies, profiles, rules, connections, logs, DNS, TUN and Mihomo Core from your browser.

[![Release](https://img.shields.io/github/v/release/chenpingonline/Clash-Manager?display_name=tag)](https://github.com/chenpingonline/Clash-Manager/releases)
[![Checks](https://github.com/chenpingonline/Clash-Manager/actions/workflows/checks.yaml/badge.svg)](https://github.com/chenpingonline/Clash-Manager/actions/workflows/checks.yaml)
[![Docker Pulls](https://img.shields.io/docker/pulls/chenpingonline/clash-manager)](https://hub.docker.com/r/chenpingonline/clash-manager)
![Linux](https://img.shields.io/badge/Linux-amd64%20%7C%20arm64-2ea44f)
[![Mihomo](https://img.shields.io/badge/Core-Mihomo-6f42c1)](https://github.com/MetaCubeX/mihomo)
[![License](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)

[User guide (Chinese)](docs/user-guide.md) · [Download Releases](https://github.com/chenpingonline/Clash-Manager/releases) · [Docker deployment](docker/README.en.md) · [Issues](https://github.com/chenpingonline/Clash-Manager/issues) · [Mihomo](https://github.com/MetaCubeX/mihomo) · [Clash for fnOS](https://github.com/chenpingonline/Clash-for-fnos)

</div>

---

![Clash Manager example interface](img.png)

The screenshot is illustrative. Features and navigation depend on the version and deployment.

## Overview

Clash Manager provides a graphical Mihomo management interface for **Linux and Docker**, covering proxies, profiles, rules and network settings.

This repository maintains shared Vue / Go code, translations, Docker images and Linux DEB packaging. Clash for fnOS uses this shared source while maintaining native FPKs, host lifecycle, permissions, desktop integration and icons in its own repository. Common features are developed once and adopted by fnOS through a pinned commit.

The Go Web service handles the UI, APIs and Mihomo Controller communication. A separate Root Helper performs Core and network operations through a restricted Unix socket. In Linux DEBs, Web runs as an ordinary system user and Helper runs separately as root.

## Features

| Feature | Description |
| --- | --- |
| Dashboard | Current proxy, exit information, operating mode, connections and live traffic |
| Proxies | Group selection, latency tests, proxy editing and sorting |
| Profiles | Multiple profiles, updates, enhancements and activation |
| Configuration | YAML editing, validation, backups and application |
| Rules | Inspection, runtime toggles and custom rules |
| Connections and logs | Inspect and close connections; view Core logs |
| DNS and networking | Proxy ports, LAN access, IPv6, DNS and Fake IP |
| TUN | Automatic routing, outbound interface, DNS hijacking and exclusions |
| Core and GEO | Managed Mihomo, Core updates and GEO data management |
| Authentication | Independent credentials; Docker supports environment configuration |
| Languages | Simplified Chinese, English and system language; remembered selection |

Docker and Linux show controls according to runtime capabilities. fnOS application icons, FPK updates and fnOS Shell proxy environment settings are provided by the native fnOS edition.

## Languages

The default follows the browser language: Chinese locales use Simplified Chinese; others use English. Switch under **Settings → Language (top right)**. Changes take effect immediately and are remembered in the current browser. Proxy names, profile names, YAML, addresses and original logs keep their original values.

See the [translation guide](docs/i18n.md) for terminology and maintenance.

---

## Installation and deployment

| Deployment | Architecture | Use case | Guide |
| --- | --- | --- | --- |
| Docker Host | Linux amd64 / arm64 | Linux host deployment with optional host TUN routing | [Docker](docker/README.en.md) |
| Docker Bridge | Linux amd64 / arm64 | Explicit HTTP / SOCKS proxy use by applications or devices | [Bridge Compose](docker/compose.bridge.yaml) |
| Linux DEB | amd64 / arm64 | Native services on systemd-based Debian / Ubuntu environments | [DEB](packaging/deb/README.en.md) |
| fnOS FPK | x86_64 / ARM64 | fnOS desktop and host integration | [Clash for fnOS](https://github.com/chenpingonline/Clash-for-fnos) |

### Docker quick start

After obtaining this repository, run from its root:

```bash
cp docker/.env.example docker/.env
# Set APP_AUTH_PASSWORD to at least 8 characters in docker/.env.
# Adjust LISTEN_ADDR, APP_CONTROLLER_PORT and APP_MIXED_PORT for occupied ports.
docker compose --env-file docker/.env -f docker/compose.yaml pull
docker compose --env-file docker/.env -f docker/compose.yaml up -d
```

With the example port settings, open `http://HOST-IP:17890` and sign in as `admin` with your configured password.

The primary image is `chenpingonline/clash-manager`. Default Compose uses the multi-architecture `latest` tag; Docker selects amd64 / arm64 automatically. To pin a published release, set `CLASH_IMAGE=chenpingonline/clash-manager:<published-version>` in `.env`.

Default deployment uses **Linux Host networking**, directly binding host ports without port mappings. TUN requires host `/dev/net/tun` and NET_ADMIN. Import a valid configuration and enable TUN manually in the UI. Choose the separate Bridge Compose for explicit proxy use.

Keep the existing data directory when upgrading, then pull and recreate:

```bash
docker compose --env-file docker/.env -f docker/compose.yaml pull
docker compose --env-file docker/.env -f docker/compose.yaml up -d --force-recreate
```

See the [Docker guide](docker/README.en.md) for authentication, persistence, port conflicts, Host / Bridge and shutdown behavior.

### Linux DEB

DEBs for amd64 / arm64 include systemd services, Mihomo and initial GEO assets. Select the matching package, for example:

```bash
sudo apt install ./clash-manager_<version>_amd64.deb
sudo systemctl status clash-manager
sudo cat /etc/clash-manager/password
```

The default URL is `http://127.0.0.1:8080`, with user `admin` and a randomly generated first-install password. Use an SSH tunnel for remote access. Configuration lives in `/etc/clash-manager/clash-manager.conf`; data lives in `/var/lib/clash-manager`. Upgrades preserve data, credentials and modified configuration.

See the [DEB guide](packaging/deb/README.en.md) for target environments, services, upgrades, removal and builds.

---

## Repository layout

```text
Clash-Manager/
├── web/                 # Vue 3 / TypeScript / Vite and translations
├── backend/             # Go Web, Root Helper and shared application code
├── docker/              # Dockerfile, Compose, environment and deployment docs
├── packaging/deb/       # DEB packaging, systemd and installer scripts
├── resources/core/      # amd64 / arm64 Mihomo assets and verification metadata
├── assets/              # Initial GEO data and third-party licenses
├── scripts/             # Version synchronization, builds, audits and checks
├── docs/                # User guide, translation guide and screenshots
├── VERSION              # Shared application version source
├── CHANGELOG.md
├── LICENSE
└── dist/                # Build artifacts; not committed
```

## Development and builds

Requires **Go 1.22+**, **Node.js 22.12+** and npm. Docker builds require Docker. DEBs also require Python 3 and `dpkg-deb`; macOS uses Docker for DEB assembly.

### Development checks

```bash
npm --prefix web ci
./scripts/sync-version.sh
npm --prefix web run check
(cd backend && go test -race ./... && go vet ./...)
```

### Build a Docker image

```bash
./scripts/build-docker.sh clash-manager:local
```

This builds for the local architecture. See the [Docker documentation](docker/README.en.md) for multi-architecture builds and Docker Hub publication.

### Build DEBs

```bash
./scripts/build-deb.sh all     # or amd64 / arm64
python3 scripts/audit-deb.py dist/*.deb
```

Packages and SHA-256 checksums go to `dist/`. Automated checks and archive audits do not verify installation, upgrades or host TUN behavior on every target device.

### Versions and repository maintenance

`VERSION` supplies the shared application version. Scripts synchronize npm versions, and builds inject the version into both Go binaries.

The fnOS repository pins this source version and full commit SHA in `upstream.lock`. FPK builds inject their manifest version and changelog into an isolated export without modifying the shared checkout. Installed applications contain compiled code and do not fetch source at startup.

Common features belong here; fnOS integration and packaging belong in its repository. Commit and push each repository separately. Source pushes, package releases and Docker publication are separate operations. Original Git history remains available; historical FPK files are no longer maintained in this repository's current tree.

See [repository maintenance](docs/repository-maintenance.md) and [backend architecture](backend/README.en.md).

## FAQ

### Why is there no host system proxy switch?

Docker does not manage host Shell proxy environment variables. Applications and devices can use the Mixed proxy port explicitly; Linux host traffic routing uses TUN with the required device and permissions.

### Does Host networking need port mappings?

No `ports` mappings are needed. Web and Core bind host ports directly; adjust `.env` before startup to avoid existing services.

### Which traffic can TUN route?

Linux Host deployment can route host traffic through TUN. Bridge has a separate network namespace. Forwarded traffic from other containers, VPNs or LAN devices requires validation against actual gateways, routes and firewall rules. See the Docker guide for deployment scope.

### Does a running container upgrade automatically?

No. `latest` refers to a published image; pull and recreate the container to upgrade. Source commits do not update previously published images automatically.

## Contributing and acknowledgments

Issues and pull requests are welcome. Include the application version, architecture, deployment and relevant logs. Do not expose subscription URLs, Secrets or passwords.

- [MetaCubeX/mihomo](https://github.com/MetaCubeX/mihomo): Mihomo Core
- [MetaCubeX/meta-rules-dat](https://github.com/MetaCubeX/meta-rules-dat): GEO data
- [Clash Verge Rev](https://github.com/clash-verge-rev/clash-verge-rev): interaction and terminology reference
- [Clash for fnOS](https://github.com/chenpingonline/Clash-for-fnos): native fnOS integration and FPKs

## License

[GPL-3.0](LICENSE). Mihomo and other components retain their respective licenses in `assets/licenses/` and `web/public/licenses/`.

---

<div align="center">

If this project helps you, please consider giving it a Star ⭐

</div>
