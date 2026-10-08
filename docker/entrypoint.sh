#!/bin/bash
set -euo pipefail

# Reject invalid credentials before Helper can start Core or activate persisted TUN.
if [ -n "${APP_AUTH_PASSWORD_FILE:-}" ]; then
  auth_password=$(setpriv --reuid=clash --regid=clash --init-groups cat "$APP_AUTH_PASSWORD_FILE") || {
    echo 'Web user cannot read APP_AUTH_PASSWORD_FILE' >&2; exit 1;
  }
else
  auth_password="${APP_AUTH_PASSWORD:-}"
fi
[ "${#auth_password}" -ge 12 ] || { echo 'Set a management password of at least 12 characters' >&2; exit 1; }
unset auth_password

# Helper owns Core and privileged config; Web only receives access to its state.
mkdir -p "$APP_CONFIG_DIR" "$APP_STATE_DIR" /run/clash
chown -R clash:clash "$APP_CONFIG_DIR" "$APP_STATE_DIR"
mkdir -p "$APP_CONFIG_DIR/mihomo" "$APP_STATE_DIR/managed-core" "$APP_STATE_DIR/core-stage"
chown -R root:root "$APP_CONFIG_DIR/mihomo" "$APP_STATE_DIR/managed-core"
chmod 750 "$APP_CONFIG_DIR/mihomo" "$APP_STATE_DIR/managed-core"
chown root:clash /run/clash
chmod 750 /run/clash
for name in Country.mmdb geoip.dat geosite.dat; do
  if [ ! -f "$APP_CONFIG_DIR/mihomo/$name" ]; then
    cp "$APP_DIR/geodata/$name" "$APP_CONFIG_DIR/mihomo/$name"
  fi
done

# Reuse the transactional bundled upgrade when a new image carries a new Core.
if ! cmp -s "$APP_DIR/core/bundled-core.json" "$APP_STATE_DIR/image-core.json"; then
  touch "$APP_STATE_DIR/bundled-core-upgrade.pending"
  cp "$APP_DIR/core/bundled-core.json" "$APP_STATE_DIR/image-core.json"
fi

helper_pid=''
web_pid=''
stop_child() {
  local child_pid="$1" timeout="$2"
  [ -n "$child_pid" ] || return 0
  kill -TERM "$child_pid" 2>/dev/null || true
  for ((i=0; i<timeout; i++)); do
    kill -0 "$child_pid" 2>/dev/null || break
    sleep 1
  done
  if kill -0 "$child_pid" 2>/dev/null; then kill -KILL "$child_pid" 2>/dev/null || true; fi
  wait "$child_pid" 2>/dev/null || true
}
cleanup() {
  trap '' TERM INT
  stop_child "$web_pid" 10
  stop_child "$helper_pid" 25
}
trap 'exit 0' TERM INT
trap cleanup EXIT

env -u APP_AUTH_PASSWORD -u APP_AUTH_PASSWORD_FILE "$APP_DIR/bin/clash-helper" &
helper_pid=$!
for ((i=0; i<100; i++)); do
  if [ -S "$PRIV_SOCKET_PATH" ]; then break; fi
  if ! kill -0 "$helper_pid" 2>/dev/null; then echo 'Helper exited during startup' >&2; exit 1; fi
  sleep 0.1
done
[ -S "$PRIV_SOCKET_PATH" ] || { echo 'Helper startup timed out' >&2; exit 1; }
chown root:clash "$PRIV_SOCKET_PATH"
setpriv --reuid=clash --regid=clash --init-groups --bounding-set=-all --inh-caps=-all --ambient-caps=-all "$APP_DIR/bin/clash-web" &
web_pid=$!
# Fail the container if either service exits, so Docker's restart policy can recover.
set +e
wait -n "$helper_pid" "$web_pid"
status=$?
set -e
[ "$status" -ne 0 ] || status=1
exit "$status"
