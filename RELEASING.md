# Releasing

This repository releases through **GitHub Actions** and dogfoods **woodpecker-plugin-release-versions** inside the release workflow.

## Release line (Go)

Bump `internal/version/version.go` when starting a new iteration:

```go
const Version = "0.2.0" // MAJOR.MINOR.PATCH placeholder; deploy owns patch
```

The plugin reads **MAJOR.MINOR** from that file (`major_minor_source: go`).

## Release flow (GitHub Actions)

Go to **Actions → release → Run workflow** and set **publish**:

Shared steps:

1. **compute** — plugin allocates or reuses `RELEASE_TAG`
2. **build-candidate** — build with burn-in, push `:rc-<RELEASE_TAG>`
3. **e2e** — tagged tests + candidate image smoke

Then:

| `publish` | Result |
|---|---|
| **false** | Candidate only (`:rc-<RELEASE_TAG>`). No `:latest`, git tag, or GitHub Release. |
| **true** | Must run from the repo **default branch** (`master`). Promote to `:<RELEASE_TAG>` + `:latest`, plugin `push-tag`, GitHub Release. |

Typical flow: `publish=false` from a feature branch to validate, merge, then `publish=true` from `master`.

## CI (GitHub Actions)

On PR and push to `master`: `go test ./...` and multi-arch docker build (no push).

Woodpecker is used for **PR review** only.

## Pipeline vs plugin

The plugin does **not** know about dev/prod environments. Each project's pipeline decides:

- when to call `compute` vs `push-tag`
- branch and environment gates (e.g. bored's `validate-deployment` step)
- whether a deploy pushes a candidate image or the release tag

The plugin only allocates semver, writes artifacts, and pushes git tags when invoked.
