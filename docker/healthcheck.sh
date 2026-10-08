#!/bin/sh
set -eu
address="${LISTEN_ADDR:-:8080}"
case "$address" in
  :*) address="127.0.0.1$address" ;;
  0.0.0.0:*) address="127.0.0.1:${address##*:}" ;;
  '[::]:'*) address="[::1]:${address##*:}" ;;
esac
curl --fail --silent --max-time 4 "http://$address/api/health" >/dev/null
