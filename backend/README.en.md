# Go backend architecture

[简体中文](README.md) | English

`cmd/clash-for-fnos-web` is the main public fnOS service. It runs as the application's dedicated user and listens on a public Unix socket. Responsibilities include:

- Gateway path normalization and entry redirects
- Vue/Vite static assets and SPA fallback
- `GET /api/health`
- Direct Controller APIs: providers, connections, rules, groups, latency tests, runtime configuration and traffic
- Group ordering, persistent selections and direct fallback for Rule Provider updates
- Dashboard aggregation and Controller connectivity tests
- Private-socket calls to the allowlisted Go Root Helper

```text
fnOS Gateway → Go Web service → Go Root Helper → Mihomo/system
```

The two processes communicate through a private Unix socket. Web runs as the dedicated application user; only the allowlisted Helper runs as root.

## v1.0.0 architecture boundaries

- Web owns static assets, business APIs, scheduling, aggregation and Controller communication.
- Helper accepts only allowlisted private-socket requests for Core, configuration transactions, TUN/DNS, GEO, proxy environment variables and icons.
- Managed configuration changes use prepare, validate, activate, verify runtime state, commit or roll back.
- fnOS stops Web before Helper. Helper waits for managed Mihomo to exit gracefully. Startup resynchronizes Controller settings and reconciles managed TUN runtime state.
- Frontend, backend, Go build versions and FPK filenames all read `fpk/manifest`.

## Completed migration phases

The migration followed seven independently tested and committed stages based on shared state and transaction boundaries:

1. **Read-only status and logs**: Mihomo log capture, history, clearing and SSE; dashboard status and Controller checks. Node no longer owned long-lived Mihomo streams.
2. **Settings and lifecycle**: manager settings, Controller discovery, startup selection restoration, initialization and cleanup. One owner for `settings.json` and `selected.json` avoided split caches.
3. **Configuration transactions**: original/effective configuration, validation, application, startup synchronization, backups and rollback. Privileged files still went through Helper.
4. **Profiles and imports**: CRUD, downloads, scheduled updates, apply jobs, task status, local scanning/imports. Imports share profile state and reuse configuration transactions.
5. **System/update facade**: status, authorized paths, network settings, proxy variables, icons, application and Core update orchestration; privileged changes remained delegated.
6. **Go Root Helper**: configuration, network/TUN/DNS, Core lifecycle, proxy environment and icon modules replaced `privileged-helper.js`, preserving a separate root process and explicit API allowlist. Core updates recheck official Release, size, SHA-256 and executable version.
7. **Remove Node runtime**: delete compatibility services, legacy JavaScript backend and runtime dependencies; simplify lifecycle scripts, remove `nodejs_v22`, finish dual-architecture builds and fnOS installation/upgrade/rollback verification.

Each phase preserved frontend API paths and JSON contracts, added Go behavior tests before deleting old routes, and passed Go/Vue/architecture checks before its own commit. Phase 7 packaging incremented the patch version through the single manifest source.

All seven phases are complete. Settings, discovery and startup selection restoration are in Go; application-time selection restoration is part of configuration transactions. Profiles/imports exclusively share Go state. System/update orchestration delegates privileges to Helper. fnOS lifecycle starts only Go Web and Go Root Helper.

## Docker runtime

`internal/runtimeenv` centralizes configuration, state, runtime and import directories. Without generic variables, existing fnOS variables/defaults remain. Web can use TCP through `LISTEN_ADDR`; native FPKs retain their public Unix socket.

Docker Web uses separate cookie authentication. Private Helper communication, managed Core and transactions retain the privilege boundaries. `GET /api/runtime` describes platform and capabilities for UI adaptation. Docker Helper does not edit host proxy files or fnOS icons; TUN checks actual Linux capabilities of Core.

See [Docker deployment](../docker/README.en.md) for building, deployment, version reconciliation and verification scope.

## Interface languages

Language selection and application-owned message translation are handled in the frontend. Chinese source messages remain in API responses; English catalogs cover application feedback and progress. Original Mihomo logs, external diagnostics and user configuration remain unchanged. See [translation maintenance](../docs/i18n.md).
