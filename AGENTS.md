# Clash Manager shared repository

- Own the single Vue / Go / translation implementation and Docker / Linux DEB builds. Do not add a second copy of FPK packaging here.
- Retain fnOS capability adapters and API/path compatibility; host lifecycle/configuration/icons belong to Clash-for-fnos.
- VERSION is the public application version source. fnOS may override it only in an isolated exported build directory.
- Run npm --prefix web run check and (cd backend && go test -race ./... && go vet ./...). Validate Docker changes with scripts/docker-smoke.py.
- fnOS adopts an explicit commit through its upstream.lock after checks. Do not advance its lock automatically from master/latest.
- Keep original history. A Git push does not publish an image or release.
