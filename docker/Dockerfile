# syntax=docker/dockerfile:1
ARG BUILDPLATFORM
FROM --platform=$BUILDPLATFORM node:24-alpine AS frontend
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN VITE_APP_BASE=/ npm run build

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS backend
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY fpk/manifest /src/manifest
ARG TARGETARCH
RUN APP_VERSION=$(awk -F= '/^version[[:space:]]*=/{gsub(/[[:space:]]/,"",$2);print $2;exit}' /src/manifest) && \
    CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w -X main.version=$APP_VERSION" -o /out/clash-web ./cmd/clash-for-fnos-web && \
    CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w -X main.version=$APP_VERSION" -o /out/clash-helper ./cmd/clash-for-fnos-helper
COPY resources/core/ /src/core/
RUN case "$TARGETARCH" in amd64) CORE_ARCH=x86;; arm64) CORE_ARCH=arm;; *) exit 1;; esac && \
    mkdir -p /out/core && cp /src/core/$CORE_ARCH/* /out/core/

FROM debian:13-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates bash curl util-linux iproute2 iptables nftables tzdata && \
    rm -rf /var/lib/apt/lists/* && groupadd -g 10001 clash && useradd -u 10001 -g clash -M -s /usr/sbin/nologin clash
WORKDIR /opt/clash
COPY --from=backend /out/clash-web /out/clash-helper ./bin/
COPY --from=backend /out/core ./core/
COPY --from=frontend /src/web/dist ./public/
COPY fpk/app/geodata ./geodata/
COPY fpk/app/licenses ./licenses/
COPY fpk/app/core/Mihomo-LICENSE-GPL-3.txt ./licenses/
COPY docker/entrypoint.sh docker/healthcheck.sh ./
RUN chmod 755 ./entrypoint.sh ./healthcheck.sh
ENV APP_PLATFORM=docker APP_DIR=/opt/clash APP_CONFIG_DIR=/data/config APP_STATE_DIR=/data/state \
    PUBLIC_DIR=/opt/clash/public PRIV_SOCKET_PATH=/run/clash/helper.sock LISTEN_ADDR=:8080 \
    APP_AUTH_USER=admin APP_NETWORK_SCOPE=container
EXPOSE 8080 7890/tcp 7890/udp
VOLUME /data
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD /opt/clash/healthcheck.sh
ENTRYPOINT ["/opt/clash/entrypoint.sh"]
