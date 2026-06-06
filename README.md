# woodpecker-plugin-release-versions

Woodpecker CI plugin that allocates deploy semver tags, writes workspace artifacts for docker burn-in, and pushes matching git tags after deploy.

[![CI](https://github.com/vcheesbrough/woodpecker-plugin-release-versions/actions/workflows/ci.yml/badge.svg)](https://github.com/vcheesbrough/woodpecker-plugin-release-versions/actions/workflows/ci.yml)

## Why this exists

Release identity should be consistent across git, docker, and compiled binaries:

- At most **one semver per commit** (reuse on re-deploy or dev-then-prod promotion)
- **Git tag string == docker tag string**
- **Version computed before build** so `RELEASE_TAG` can be burned in
- **Git tag pushed after deploy** succeeds

This replaces inline shell in project pipelines (for example bored's `compute-version` / `tag-release` steps).

## License

Source-available under the [PolyForm Noncommercial License 1.0.0](LICENSE). This is **not** an OSI-approved open source license.

**Commercial use** requires a separate license. See [LICENSE-TIER.md](LICENSE-TIER.md).

## Modes

| `mode` | When | Action |
|---|---|---|
| `compute` | Before `docker build` | Read release line, allocate or reuse `RELEASE_TAG`, write workspace files |
| `push-tag` | After deploy succeeds | Create/push annotated git tag matching `.release-tag` |
| `validate` | Optional | Check manifest and remote tag policy |

## Major/minor sources

| `major_minor_source` | Reads | Default file |
|---|---|---|
| `go` | `const Version = "x.y.z"` in a Go file | `internal/version/version.go` |
| `cargo` | `[workspace.package].version` or `[package].version` | `Cargo.toml` |

Patch in the manifest is a placeholder; deploy tooling owns the patch digit from remote tags.

## Woodpecker settings

| Setting | Env var | Required | Default |
|---|---|---|---|
| `mode` | `PLUGIN_MODE` | yes | — |
| `major_minor_source` | `PLUGIN_MAJOR_MINOR_SOURCE` | yes | — |
| `major_minor_file` | `PLUGIN_MAJOR_MINOR_FILE` | no | source-specific |
| `github_token` | `PLUGIN_GITHUB_TOKEN` | yes | — |
| `tag_prefix` | `PLUGIN_TAG_PREFIX` | no | `""` |
| `output_dir` | `PLUGIN_OUTPUT_DIR` | no | `.` |
| `dry_run` | `PLUGIN_DRY_RUN` | no | `false` |

Woodpecker also provides `CI_COMMIT_SHA`, `CI_REPO`, `CI_COMMIT_BRANCH`, and `CI_PIPELINE_NUMBER` for tag messages.

**Environment policy** (dev vs prod, branch gates, when to git-tag) belongs in your pipeline — not in this plugin.

### Workspace outputs (`compute`)

| File | Example |
|---|---|
| `.release-tag` | `0.1.4` |
| `.release-tag-reused` | `true` / `false` |
| `.release-tag-major-minor` | `0.1` |

### Example — Rust consumer (bored)

```yaml
  validate-deployment:
    # branch / target checks live here — not in the plugin

  compute-version:
    image: ghcr.io/vcheesbrough/woodpecker-plugin-release-versions:0.1.4
    settings:
      mode: compute
      major_minor_source: cargo
      github_token:
        from_secret: github_token

  tag-release:
    when:
      - event: deployment
        # invoke only when your pipeline should record a git tag
    image: ghcr.io/vcheesbrough/woodpecker-plugin-release-versions:0.1.4
    settings:
      mode: push-tag
      major_minor_source: cargo
      github_token:
        from_secret: github_token
```

### Example — Go consumer (this repo)

```yaml
  compute-version:
    image: ghcr.io/vcheesbrough/woodpecker-plugin-release-versions:0.1.4
    settings:
      mode: compute
      major_minor_source: go
      major_minor_file: internal/version/version.go
      github_token:
        from_secret: github_token
```

Downstream steps read `$(cat .release-tag)` for docker build/push.

## Dogfooding

This repo releases through **GitHub Actions** (like `woodpecker-plugin-authentik-blueprint`) and runs the plugin inside the release workflow for `compute` and `push-tag`. See [RELEASING.md](RELEASING.md).

In that workflow, **publish=false** leaves a candidate (`:rc-<tag>`) only; **publish=true** promotes, tags git, and creates the GitHub Release.

Woodpecker is used for PR review only.

## Development

```bash
go test ./...
docker build -t ghcr.io/vcheesbrough/woodpecker-plugin-release-versions:local .
```

Local CLI:

```bash
PLUGIN_MODE=compute \
CI_COMMIT_SHA=abc123 \
CI_REPO=vcheesbrough/woodpecker-plugin-release-versions \
PLUGIN_GITHUB_TOKEN=... \
PLUGIN_MAJOR_MINOR_SOURCE=go \
go run . compute
```
