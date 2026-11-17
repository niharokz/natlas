## What and why

## Checks

- [ ] `go vet ./... && go test ./...`
- [ ] `sh scripts/check-public.sh` (no secrets or private words)
- [ ] CHANGELOG.md updated (user-visible changes)
- [ ] Android changes: `bash scripts/build-in-container.sh debug` builds (inside `natlas-builder`)
