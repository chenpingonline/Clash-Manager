ARG BASE=debian:12-slim
FROM ${BASE}
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends systemd systemd-sysv ca-certificates bash curl util-linux iproute2 iptables nftables adduser init-system-helpers python3 && rm -rf /var/lib/apt/lists/* && rm -f /usr/sbin/policy-rc.d
STOPSIGNAL SIGRTMIN+3
CMD ["/sbin/init"]
