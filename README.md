# woodpecker-plugin-release-versions

Woodpecker CI plugin for one build per push, promoted unchanged to every environment. Each build claims the next version on its line; the first successful deploy writes the git tag, so a commit is pinned to one build forever.

[![CI](https://github.com/vcheesbrough/woodpecker-plugin-release-versions/actions/workflows/ci.yml/badge.svg)](https://github.com/vcheesbrough/woodpecker-plugin-release-versions/actions/workflows/ci.yml)

The design and its reasoning: [docs/design/standard-release.md](docs/design/standard-release.md).

## License

Source-available under the [PolyForm Noncommercial License 1.0.0](LICENSE). This is **not** an OSI-approved open source license.

**Commercial use** requires a separate license. See [LICENSE-TIER.md](LICENSE-TIER.md).

## Rules

- **Version** = `MAJOR.MINOR` from the manifest + a gap-free patch counter per line. A build claims its number by creating `refs/builds/<version>` on the remote, which refuses an existing name, so racing builds never share one. A failed build uses its number up.
- **Build** = one push pipeline, one version, images published as `<image>:<version>` only, after every test is green. A version is published once and never overwritten.
- **Deploy** = a Woodpecker deployment. It never builds: it deploys the commit's tagged build, else the newest published build of the commit.
- **Git tag** = "this build was deployed". Written once the deploy steps succeed, whatever smoke tests later find; never moved, never a second one on a commit.
- **Order is free**: dev before prod, after it, or not at all. Whichever deploys first writes the tag; every later deploy reuses that build.

## Modes

| `mode` | Runs on | Does |
|---|---|---|
| `version` | push, manual | Reads `MAJOR.MINOR`, claims the next number (retrying on a clash), writes `.release-tag`. Workflows of one pipeline share one number. Refuses deployment events. |
| `publish` | push, manual, after tests | Checks each local `<image>:<version>` carries the right version and revision labels and is not yet in the registry, then pushes them all. Needs the docker socket. |
| `resolve` | deployment | Checks the target and branch against `environments`, finds the commit's release tag or newest published build, verifies the image labels, writes `.release-tag`. Needs no docker socket. |
| `tag` | deployment, after the deploy steps | Pushes the annotated git tag; a no-op if the commit already has it. `push-tag` is an alias. |

`compute` and `validate` were removed in 0.2.0.

## Settings

| Setting | Used by | Default |
|---|---|---|
| `mode` | all | required |
| `major_minor_source` | version | required: `cargo`, `go`, or `pyproject` |
| `major_minor_file` | version | `Cargo.toml`, `internal/version/version.go`, `pyproject.toml` |
| `images` | publish, resolve | required: image names without a tag |
| `registry_username`, `registry_password` | publish, resolve | anonymous |
| `environments` | resolve | dev from any branch, prod from the repo's default branch (`CI_REPO_DEFAULT_BRANCH`, else the remote's `HEAD`) |
| `github_token` | version, resolve, tag | required |
| `tag_prefix` | all | `""` |
| `dry_run` | all | `false`: read and check, change nothing |
| `output_dir` | all | `.`: the workspace checkout, where `.release-tag` lives |
| `deploy_target` | resolve, tag | `CI_PIPELINE_DEPLOY_TARGET` |
| `git_remote` | version, resolve, tag | `https://github.com/<CI_REPO>.git` |
| `registry_insecure` | publish, resolve | `false`: allow plain-HTTP registries |

`environments` maps each deploy target to the branches allowed to deploy there (`path.Match` globs; none = any branch). A target not listed is refused. There is deliberately no ordering option.

Images must carry `org.opencontainers.image.version=<version>` and `org.opencontainers.image.revision=<commit sha>` labels.

The token travels in a git HTTP header, never in a URL or on a command line.

## Pipelines

The version travels between steps in `.release-tag`; read it with `$(cat .release-tag)`.

```yaml
# .woodpecker/build.yml
when:
  - event: [push, manual]
steps:
  version:
    image: ghcr.io/vcheesbrough/woodpecker-plugin-release-versions:0.2.0
    settings:
      mode: version
      major_minor_source: cargo
      github_token: { from_secret: github_token }
  build:
    image: docker:cli
    volumes: [/var/run/docker.sock:/var/run/docker.sock]
    commands:
      - >
        docker build -t registry.example/app:$(cat .release-tag)
        --build-arg RELEASE_TAG=$(cat .release-tag)
        --label org.opencontainers.image.version=$(cat .release-tag)
        --label org.opencontainers.image.revision=$CI_COMMIT_SHA .
  test:             # repo-specific
    depends_on: [build]
  publish:
    depends_on: [test]
    image: ghcr.io/vcheesbrough/woodpecker-plugin-release-versions:0.2.0
    volumes: [/var/run/docker.sock:/var/run/docker.sock]
    settings:
      mode: publish
      images: [registry.example/app]
      registry_username: { from_secret: registry_user }
      registry_password: { from_secret: registry_password }
```

```yaml
# .woodpecker/deploy.yml
when:
  - event: deployment
steps:
  resolve:
    image: ghcr.io/vcheesbrough/woodpecker-plugin-release-versions:0.2.0
    settings:
      mode: resolve
      images: [registry.example/app]
      environments:      # optional; omit for dev from any branch, prod from the repo's default branch
        dev: {}
        prod: { branches: [main] }
      github_token: { from_secret: github_token }
      registry_username: { from_secret: registry_user }
      registry_password: { from_secret: registry_password }
  deploy-dev:       # repo-specific, deploys registry.example/app:$(cat .release-tag)
    depends_on: [resolve]
    when: [{ evaluate: 'CI_PIPELINE_DEPLOY_TARGET == "dev"' }]
  deploy-prod:      # repo-specific
    depends_on: [resolve]
    when: [{ evaluate: 'CI_PIPELINE_DEPLOY_TARGET == "prod"' }]
  tag:
    depends_on: [deploy-dev, deploy-prod]
    image: ghcr.io/vcheesbrough/woodpecker-plugin-release-versions:0.2.0
    settings:
      mode: tag
      github_token: { from_secret: github_token }
  smoke:            # repo-specific; beside tag: a failure fails the pipeline, never the tag
    depends_on: [deploy-dev, deploy-prod]
```

A pipeline with several workflows (for example web and android) runs `version` in each; they find the number their pipeline already claimed.

## GitHub Actions

The repo is also a container action. Settings become `with:` inputs, GitHub's own variables replace Woodpecker's (`GITHUB_SHA`, `GITHUB_REPOSITORY`, `GITHUB_REF_NAME`, `GITHUB_RUN_ID`), and the version is also a `release_tag` step output. Each run attempt claims its own number; when several jobs share one version, use "Re-run all jobs", since "Re-run failed jobs" gives only the re-run jobs a new number.

```yaml
- id: version
  uses: vcheesbrough/woodpecker-plugin-release-versions@0.2.0
  with:
    mode: version
    major_minor_source: go
    github_token: ${{ secrets.GITHUB_TOKEN }}   # needs contents: write
```

For a deploy job, pass `deploy_target: ${{ job.environment }}` style values explicitly; GitHub Environments give the approval gate before prod.

## Refusals

| Message | Cause | Fix |
|---|---|---|
| `has no published build; push a fix or re-run the build` | Every build of the commit failed before `publish` | Push a fix or re-run the build pipeline |
| `is not in environments` / `may not deploy to` | Target or branch outside `environments` | Deploy from an allowed branch, or change `environments` |
| `more than one release tag` | The commit was tagged twice under an older scheme | Delete the wrong tag by hand, deliberately |
| `no longer in the registry (pruned?)` | The tagged build was removed from the registry | Restore the image; a commit cannot change version |
| `label … want …` | An image was published from another build | Check what pushed that image; never retag to fix it |
| `already published` | The version is in the registry already | A build ran publish twice; investigate, do not overwrite |
| `already has release tag` / `tags never move` | Deploying a different version than the commit carries | Deploy through `resolve`, never a hand-written `.release-tag` |
| `registry cannot answer` | Registry down or credentials wrong | Fix the registry; nothing is assumed missing |

## Development

```bash
go test ./...                       # unit and mode tests, real git against local bare repos
go test -tags=e2e ./test/e2e/...    # dry runs against GitHub (needs GITHUB_TOKEN)
docker build -t ghcr.io/vcheesbrough/woodpecker-plugin-release-versions:local .
```

Local CLI (dry run):

```bash
PLUGIN_MODE=version PLUGIN_DRY_RUN=true \
CI_COMMIT_SHA=$(git rev-parse HEAD) CI_PIPELINE_NUMBER=local \
CI_REPO=vcheesbrough/woodpecker-plugin-release-versions \
PLUGIN_GITHUB_TOKEN=... PLUGIN_MAJOR_MINOR_SOURCE=go \
go run .
```

This repo releases itself through GitHub Actions; see [RELEASING.md](RELEASING.md).
