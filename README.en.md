# Clash Manager

[简体中文](README.md) | English

A Mihomo manager for Linux / Docker and the shared source used by [Clash for fnOS](https://github.com/chenpingonline/Clash-for-fnos).

Manage proxies, profiles, rules, configuration, connections, logs, DNS, TUN, Core and GEO data. The UI supports Simplified Chinese, English and the browser language.

## Docker

Image: `chenpingonline/clash-manager`, supporting Linux amd64 / arm64. Moving source does not update existing images. See the [Docker guide](docker/README.en.md).

```bash
cp docker/.env.example docker/.env
# Set a management password of at least 8 characters in docker/.env.
docker compose --env-file docker/.env -f docker/compose.yaml up -d
```

The default Compose uses Linux Host networking. TUN requires NET_ADMIN and /dev/net/tun.

## Development

Install Go, Node.js (see web/package.json engines) and npm.

```bash
npm --prefix web ci
./scripts/sync-version.sh
npm --prefix web run check
(cd backend && go test -race ./... && go vet ./...)
./scripts/build-docker.sh clash-manager:local
```

VERSION is the sole shared application version source. The default frontend base is `/`. The fnOS packager sets `/app/clash-for-fnos/` and injects its package version and changelog in an isolated build directory. Native Linux package delivery scripts are not implemented yet; the Docker entrypoint is not a host installer.

## Repository maintenance

This repository owns the shared Vue / Go implementation, translations and Docker builds, including compatibility adapters for fnOS capabilities. The fnOS repository owns native packaging, lifecycle, host configuration, desktop integration and icons. Its upstream.lock pins this repository by version and full commit SHA. Shared features are implemented once here, then adopted and validated by the fnOS packager. Both repositories use master and are pushed independently. Original Git history is retained.

See [maintenance](docs/repository-maintenance.md), the [user guide](docs/user-guide.md), [translations](docs/i18n.md) and [backend](backend/README.en.md).

## License

[GPL-3.0](LICENSE). Third-party licenses are retained in assets/licenses and web/public/licenses.
