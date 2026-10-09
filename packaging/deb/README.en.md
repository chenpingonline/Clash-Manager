# Linux DEB installation

[简体中文](README.md) | English

Targets systemd-based Debian 12/13 and Ubuntu 22.04/24.04 on amd64/arm64. Consult validation results for tested releases; derivatives require verification. Includes the shared management interface, Mihomo and GEO assets.

## Install

```bash
sudo apt install ./clash-manager_1.3.6_amd64.deb
sudo systemctl status clash-manager
sudo cat /etc/clash-manager/password
```

Default URL: http://127.0.0.1:8080. User: admin. A random password is generated on first installation and is never printed to installation logs. Access a remote installation through SSH:

```bash
ssh -L 8080:127.0.0.1:8080 user@server
```

Open http://127.0.0.1:8080 locally. To allow remote access, change LISTEN_ADDR to `0.0.0.0:8080` in `/etc/clash-manager/clash-manager.conf`; use an HTTPS reverse proxy for remote access.

## Configuration and services

Web runs as the non-login clash-manager user. A root Helper manages Mihomo through a restricted Unix socket. fnOS icons, FPK updates and fnOS Shell proxy environment controls are unavailable. Linux uses a managed Core.

Configure APP_AUTH_USER, APP_AUTH_PASSWORD_FILE, LISTEN_ADDR, APP_CONTROLLER_PORT, APP_MIXED_PORT or APP_IMPORT_PATHS in `/etc/clash-manager/clash-manager.conf`. The password must contain at least eight characters and be readable by clash-manager. The default password file is root:clash-manager, mode 0640. Restart after changes:

```bash
sudo systemctl restart clash-manager
sudo journalctl -u clash-manager -u clash-manager-helper -f
sudo systemctl stop clash-manager
```

TUN defaults to off. It changes host routes and requires `/dev/net/tun`; the installer does not create devices or load kernel modules. Normal service shutdown stops Web and lets Helper stop managed Mihomo, releasing ports; Mihomo removes TUN routes on normal exit. Power loss, SIGKILL and kernel failures are different from graceful shutdown.

Data lives in `/var/lib/clash-manager`. Upgrades preserve data, modified conffiles and the password. Default Controller: 127.0.0.1:9090; Mixed: 7890. Override occupied ports in the configuration before restarting. systemd enables startup at boot; use `systemctl disable clash-manager` to disable it. Environments without systemd PID 1 do not start the service automatically.

## Upgrade and remove

```bash
sudo apt install ./clash-manager_<new-version>_amd64.deb
sudo apt remove clash-manager
sudo apt purge clash-manager
```

Upgrade/removal stops both services. Purge removes package configuration and credentials, but retains `/var/lib/clash-manager` and the system user. Back up data before deleting it manually. Reinstalling with retained data may restore a previously enabled TUN configuration.

## Build

```bash
npm --prefix web ci
./scripts/build-deb.sh all  # or amd64 / arm64
```

The root VERSION file supplies the version. Packages and SHA256 files go to dist. Requires Go, Node/npm, Python 3 and dpkg-deb; macOS automatically uses a debian:12-slim Docker container for assembly. Binaries are statically cross-compiled, and existing architecture-specific Mihomo assets are included. This builds binary DEBs, not source packages for submission to Debian repositories.
