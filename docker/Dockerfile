# syntax=docker/dockerfile:1
ARG BUILDPLATFORM
FROM --platform=$BUILDPLATFORM node:24-alpine AS frontend
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
COPY VERSION /src/VERSION
COPY CHANGELOG.md /src/CHANGELOG.md
COPY scripts/sync-version.sh /src/scripts/sync-version.sh
RUN sh /src/scripts/sync-version.sh && VITE_APP_BASE=/ npm run build

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS backend
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY VERSION /src/VERSION
ARG TARGETARCH
RUN APP_VERSION=$(cat /src/VERSION) && \
    CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w -X main.version=$APP_VERSION" -o /out/clash-web ./cmd/clash-for-fnos-web && \
    CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w -X main.version=$APP_VERSION" -o /out/clash-helper ./cmd/clash-for-fnos-helper
COPY resources/core/ /src/core/
RUN case "$TARGETARCH" in amd64) CORE_ARCH=x86;; arm64) CORE_ARCH=arm;; *) exit 1;; esac && \
    mkdir -p /out/core && cp /src/core/$CORE_ARCH/* /out/core/

FROM debian:13-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates bash curl util-linux iproute2 iptables nftables tzdata && \
    rm -rf /var/lib/apt/lists/* && groupadd -g 10001 clash && useradd -u 10001 -g clash -M -s /usr/sbin/nologin clash
LABEL org.opencontainers.image.title="Clash Manager" \
    org.opencontainers.image.description="Clash Manager (formerly Clash for fnOS / clash-for-fnos), a Mihomo proxy manager for Linux and fnOS" \
    org.opencontainers.image.source="https://github.com/chenpingonline/Clash-Manager" \
    org.opencontainers.image.licenses="GPL-3.0-only"
WORKDIR /opt/clash
COPY --from=backend /out/clash-web /out/clash-helper ./bin/
COPY --from=backend /out/core ./core/
COPY --from=frontend /src/web/dist ./public/
COPY assets/geodata ./geodata/
COPY assets/licenses ./licenses/
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
