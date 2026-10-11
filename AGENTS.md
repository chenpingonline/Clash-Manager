# Clash Manager shared repository

- Own the single Vue / Go / translation implementation and Docker / Linux DEB builds. Do not add a second copy of FPK packaging here.
- Retain fnOS capability adapters and API/path compatibility; host lifecycle/configuration/icons belong to Clash-for-fnos.
- VERSION is the public application version source. fnOS may override it only in an isolated exported build directory.
- Run npm --prefix web run check and (cd backend && go test -race ./... && go vet ./...). Validate Docker changes with scripts/docker-smoke.py.
- fnOS adopts an explicit commit through its upstream.lock after checks. Do not advance its lock automatically from master/latest.
- Keep original history. A Git push does not publish an image or release.

## Version management before packaging

- When changed shared application code is to be packaged as an FPK, Docker image or DEB, increment VERSION (patch by default; honor an explicit version), update CHANGELOG.md and run scripts/sync-version.sh. Commit the shared code and matching version metadata before packaging; new functionality must not keep the previous packaged source's version label.
- For fnOS builds, update Clash-for-fnos/upstream.lock to that committed source's full SHA and exact VERSION after checks. The lock's version and commit must be updated together and validated against each other.
- The fnOS installed application version remains controlled by its fpk/manifest and is injected only into the isolated export. Packaging-only fnOS changes may reuse an unchanged shared commit and version while incrementing the FPK version independently.
- Before delivery, verify the shared VERSION, pinned SHA, target package/image version and corresponding changelog. Report the shared source version/commit and delivered package/image version clearly.
- Preserve the existing authorization boundaries for commits, Git pushes and artifact publication; a version update does not authorize publishing or replacing an existing release.
