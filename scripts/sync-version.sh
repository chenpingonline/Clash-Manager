#!/bin/sh
set -eu
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
APP_RELEASE_VERSION="${1:-$(cat "$ROOT/VERSION")}"
printf '%s\n' "$APP_RELEASE_VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' || { echo 'Invalid application version' >&2; exit 1; }
node --input-type=module - "$ROOT/web" "$APP_RELEASE_VERSION" <<'JS'
import fs from 'node:fs'
import path from 'node:path'
const [directory, version] = process.argv.slice(2)
for (const name of ['package.json', 'package-lock.json']) {
  const file = path.join(directory, name)
  const value = JSON.parse(fs.readFileSync(file, 'utf8'))
  value.version = version
  if (value.packages?.['']) value.packages[''].version = version
  const text = JSON.stringify(value, null, 2) + '\n'
  if (fs.readFileSync(file, 'utf8') !== text) fs.writeFileSync(file, text)
}
JS
