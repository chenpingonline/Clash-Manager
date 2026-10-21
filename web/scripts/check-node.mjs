// Security floors for CVE-2026-111497; reject unverified/unsupported release lines.
const [major, minor, patch] = process.versions.node.split('.').map(Number)
const floors = {22: [23, 4], 24: [22, 1], 26: [12, 1]}
const floor = floors[major]
if (!floor || minor < floor[0] || (minor === floor[0] && patch < floor[1])) {
  console.error(`Unsafe Node.js ${process.versions.node}: use 22.23.4+, 24.22.1+ or 26.12.1+ in the corresponding release line.`)
  process.exit(1)
}
