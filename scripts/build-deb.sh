#!/bin/bash
set -euo pipefail
export LC_ALL=C LANG=C COPYFILE_DISABLE=1
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
APP_RELEASE_VERSION="$(cat "$ROOT/VERSION")"
ARCH="${1:-all}"
case "$ARCH" in amd64|arm64) ARCHES="$ARCH";; all) ARCHES='amd64 arm64';; *) echo 'Usage: build-deb.sh [amd64|arm64|all]' >&2; exit 1;; esac
"$ROOT/scripts/sync-version.sh"
VITE_APP_BASE=/ npm --prefix "$ROOT/web" run build
WORK="$(mktemp -d)"
PACKAGER=''
cleanup() {
    if [ -n "$PACKAGER" ]; then docker rm -f "$PACKAGER" >/dev/null 2>&1 || true; fi
    rm -rf "$WORK"
}
trap cleanup EXIT
mkdir -p "$ROOT/dist"
for ARCH in $ARCHES; do
    STAGE="$WORK/$ARCH"
    APP="$STAGE/usr/lib/clash-manager"
    mkdir -p "$APP/bin" "$APP/core" "$STAGE/DEBIAN" "$STAGE/etc/clash-manager" "$STAGE/lib/systemd/system" "$STAGE/usr/share/doc/clash-manager"
    for component in web helper; do
        (cd "$ROOT/backend" && CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" go build -trimpath -ldflags "-s -w -X main.version=$APP_RELEASE_VERSION" -o "$APP/bin/clash-$component" "./cmd/clash-for-fnos-$component")
    done
    case "$ARCH" in amd64) CORE_ARCH=x86;; arm64) CORE_ARCH=arm;; esac
    cp -R "$ROOT/resources/core/$CORE_ARCH/." "$APP/core/"
    cp -R "$ROOT/web/dist" "$APP/public"
    cp -R "$ROOT/assets/geodata" "$APP/geodata"
    cp -R "$ROOT/assets/licenses" "$APP/licenses"
    cp "$ROOT/packaging/deb/prepare-linux" "$ROOT/packaging/deb/wait-helper" "$ROOT/packaging/deb/start-helper" "$APP/"
    cp "$ROOT/packaging/deb/"*.service "$STAGE/lib/systemd/system/"
    cp "$ROOT/packaging/deb/clash-manager.conf" "$STAGE/etc/clash-manager/"
    cp "$ROOT/packaging/deb/postinst" "$ROOT/packaging/deb/prerm" "$ROOT/packaging/deb/postrm" "$STAGE/DEBIAN/"
    cp "$ROOT/LICENSE" "$STAGE/usr/share/doc/clash-manager/copyright"
    cp "$ROOT/packaging/deb/README.md" "$ROOT/packaging/deb/README.en.md" "$STAGE/usr/share/doc/clash-manager/"
    printf '%s\n' /etc/clash-manager/clash-manager.conf > "$STAGE/DEBIAN/conffiles"
    cat > "$STAGE/DEBIAN/control" <<CONTROL
Package: clash-manager
Version: $APP_RELEASE_VERSION
Architecture: $ARCH
Maintainer: chenpingonline <chenpingonline@users.noreply.github.com>
Section: net
Priority: optional
Depends: ca-certificates, bash, curl, util-linux, iproute2, iptables, nftables, adduser, systemd (>= 247), init-system-helpers
Homepage: https://github.com/chenpingonline/Clash-Manager
Description: Mihomo proxy manager for Linux
 Web management of subscriptions, proxies, rules, DNS and TUN.
 Includes Mihomo Core and a systemd-managed privileged helper.
CONTROL
    # Archives must not contain macOS AppleDouble files or writable executables.
    find "$STAGE" -name '._*' -delete
    find "$STAGE" -type d -exec chmod 755 {} +
    find "$STAGE" -type f -exec chmod 644 {} +
    chmod 755 "$APP/bin/"* "$APP/prepare-linux" "$APP/wait-helper" "$APP/start-helper" "$STAGE/DEBIAN/postinst" "$STAGE/DEBIAN/prerm" "$STAGE/DEBIAN/postrm"
    python3 - "$STAGE" <<'PYMD5'
import hashlib, pathlib, sys
root = pathlib.Path(sys.argv[1])
lines = []
for path in sorted(root.rglob('*')):
    relative = path.relative_to(root).as_posix()
    if path.is_file() and not relative.startswith('DEBIAN/'):
        lines.append(hashlib.md5(path.read_bytes()).hexdigest() + '  ' + relative)
(root / 'DEBIAN/md5sums').write_text('\n'.join(lines) + '\n')
PYMD5
    FILE="clash-manager_${APP_RELEASE_VERSION}_${ARCH}.deb"
    if command -v dpkg-deb >/dev/null; then
        COPYFILE_DISABLE=1 LC_ALL=C dpkg-deb --root-owner-group -Zxz --build "$STAGE" "$ROOT/dist/$FILE"
    else
        # macOS fallback: package in a Linux dpkg environment without rebuilding.
        case "$(docker info --format '{{.Architecture}}')" in
            aarch64|arm64) PACKAGER_ARCH=arm64;; x86_64|amd64) PACKAGER_ARCH=amd64;; *) exit 1;;
        esac
        if [ "$(docker image inspect --format '{{.Architecture}}' debian:12-slim 2>/dev/null || true)" != "$PACKAGER_ARCH" ]; then
            docker pull --platform "linux/$PACKAGER_ARCH" debian:12-slim
        fi
        PACKAGER=$(docker create --platform "linux/$PACKAGER_ARCH" debian:12-slim dpkg-deb --root-owner-group -Zxz --build /stage "/$FILE")
        COPYFILE_DISABLE=1 docker cp "$STAGE" "$PACKAGER:/stage"
        docker start -a "$PACKAGER"
        [ "$(docker inspect --format '{{.State.ExitCode}}' "$PACKAGER")" = 0 ]
        docker cp "$PACKAGER:/$FILE" "$ROOT/dist/$FILE"
        docker rm "$PACKAGER" >/dev/null
        PACKAGER=''
    fi
    (cd "$ROOT/dist" && shasum -a 256 "$FILE" > "$FILE.sha256")
    printf 'Built %s\n' "$ROOT/dist/$FILE"
done
