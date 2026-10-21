# Releasing

This repository releases through **GitHub Actions** and dogfoods **woodpecker-plugin-release-versions** inside the release workflow.

## Release line (Go)

Bump `internal/version/version.go` when starting a new iteration:

```go
const Version = "0.3.0" // MAJOR.MINOR.0; the plugin owns the patch
```

The plugin reads **MAJOR.MINOR** from that file (`major_minor_source: go`).

## Release flow (GitHub Actions)

Go to **Actions → release → Run workflow** and set **publish**:

Shared steps:

1. **compute** — plugin mode `version` claims the next `RELEASE_TAG` on the line (`refs/builds/<tag>`)
2. **build-candidate** — build with burn-in, push `:rc-<RELEASE_TAG>`
3. **e2e** — tagged tests + candidate image smoke

Then:

| `publish` | Result |
|---|---|
| **false** | Candidate only (`:rc-<RELEASE_TAG>`). No `:latest`, git tag, or GitHub Release. |
| **true** | Promote to `:<RELEASE_TAG>` + `:latest`, plugin `tag`, GitHub Release. |

Each run claims its own number, so a `publish=false` run followed by a `publish=true` run releases the second run's number; the first is a gap.

## CI (GitHub Actions)

On PR and push to `master`: `go test ./...` and an amd64 docker build (no push).

Woodpecker is used for **PR review** only.
