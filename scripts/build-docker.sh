#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DOCKERFILE="$ROOT/docker/Dockerfile"
APP_RELEASE_VERSION="$(cat "$ROOT/VERSION")"
"$ROOT/scripts/sync-version.sh"
LOCAL_BUILD=false
if [ "${1:-}" = "--local-build" ]; then LOCAL_BUILD=true; shift; fi
IMAGE="${1:-clash-manager:$APP_RELEASE_VERSION}"
shift "$(( $# > 0 ? 1 : 0 ))"
if [ "$LOCAL_BUILD" = true ]; then
  case "$(docker info --format '{{.Architecture}}')" in
    aarch64|arm64) ARCH=arm64;; x86_64|amd64) ARCH=amd64;; *) exit 1;;
  esac
  WORK="$(mktemp -d)"
  trap 'rm -rf "$WORK"' EXIT
  VITE_APP_BASE=/ npm --prefix "$ROOT/web" run build
  mkdir -p "$WORK/out/core" "$WORK/assets"
  for component in web helper; do
    (cd "$ROOT/backend" && CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" go build -trimpath -ldflags "-s -w -X main.version=$APP_RELEASE_VERSION" -o "$WORK/out/clash-$component" "./cmd/clash-for-fnos-$component")
  done
  case "$ARCH" in amd64) CORE_ARCH=x86;; arm64) CORE_ARCH=arm;; esac
  cp -a "$ROOT/resources/core/$CORE_ARCH/." "$WORK/out/core/"
  cp -a "$ROOT/web/dist" "$WORK/public"
  cp -a "$ROOT/assets/." "$WORK/assets/"
  mkdir -p "$WORK/docker"
  cp "$ROOT/docker/entrypoint.sh" "$ROOT/docker/healthcheck.sh" "$WORK/docker/"
  # Use exactly the same final runtime stage, replacing only builder-stage copies.
  awk '/^FROM debian:13-slim/{copy=1} copy{print}' "$DOCKERFILE" | \
    sed -e 's|--from=backend /out/|out/|g' -e 's|--from=frontend /src/web/dist|public|g' > "$WORK/Dockerfile"
  docker build -t "$IMAGE" "$@" "$WORK"
  exit
fi
if docker buildx version >/dev/null 2>&1; then
  docker buildx build --load -f "$DOCKERFILE" -t "$IMAGE" "$@" "$ROOT"
else
  # Legacy builders do not supply BuildKit's automatic platform arguments.
  case "$(docker info --format '{{.Architecture}}')" in
    aarch64|arm64) ARCH=arm64;; x86_64|amd64) ARCH=amd64;; *) echo 'Unsupported Docker architecture' >&2; exit 1;;
  esac
  docker build -f "$DOCKERFILE" --build-arg "BUILDPLATFORM=linux/$ARCH" --build-arg "TARGETARCH=$ARCH" -t "$IMAGE" "$@" "$ROOT"
fi
