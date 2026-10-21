# Standard build, release and promotion design

Status: design agreed with Vincent on 2026-10-21. No implementation yet.
Living copy (with comments): https://claude.ai/artifact/FPZcFvSt4MzgNaUctRoKCg

Donor repos: bored, v-note, sovereign-config, code-vitals.

## Summary

Every repo builds once per push and gets version `MAJOR.MINOR.<next number in sequence>`. That one image is promoted unchanged to dev and then prod. The git tag is written by the first successful deploy, so a commit is pinned to one build forever and prod only ever runs a tagged version. All of the versioning, lookup, policy and tagging logic moves into this plugin; each repo keeps only its own `docker build`, tests and deploy commands.

Decisions (all made by Vincent):

1. **A git tag means "this build was deployed"** (bored's rule), written on the first successful deploy to any environment. v-note and sovereign-config stop tagging on push.
2. **Dev before prod is optional.** When a commit has been on dev, prod must deploy that same build; the tag dev wrote guarantees it. A per-environment setting can still force dev first, off by default.
3. **Environment policy lives in the plugin, with an escape hatch:** the release rules are fixed; which environments a repo has, and which branches may deploy to each, is one per-repo `environments` setting.
4. **Sequential versions:** patch numbers are a gap-free per-line counter, not `CI_PIPELINE_NUMBER`.
5. **No `:commit-<sha>` docker tags:** a deploy finds a commit's build through git build refs.
6. **Stay on Woodpecker 3.15** (3.19 release notes checked; nothing here needs it). Its button is Deploy and fires the `deployment` event.
7. **Must remain usable from GitHub Actions** if a repo moves (see below).

## Where the donors are today

All four dropped this plugin's `compute` mode: it allocated "highest remote patch + 1", so two pipelines on one line got the same version and overwrote each other's images (v-note #462, bored #461). Each repo computes `MAJOR.MINOR` from its manifest plus `CI_PIPELINE_NUMBER` in its own script. Only bored still uses the plugin, for `push-tag` (pinned at 0.1.1).

| Repo | Manifest | Images | Push pipeline | Git tag written | Deploy targets | How a deploy finds its build | Release scripts in repo |
| --- | --- | --- | --- | --- | --- | --- | --- |
| bored | Cargo.toml | 1 | build, e2e, then push `:<ver>` and `:commit-<sha>` | after a successful dev or prod deploy | dev from any branch, prod from main | commit's tag, else version label of `:commit-<sha>` | compute-release-tag, find-commit-tag, resolve-deploy-tag, publish-release-image, check-image-metadata |
| v-note | Cargo.toml | web + android | 3 workflows build and test, then verify and tag | on every green push | dev only (prod deferred, #388) | commit's tag; refuses if none | release-version.sh |
| sovereign-config | Cargo.toml | 3 (server, broker, cli) | push to `*-ci:<ver>`, copy to release repo, tag | on every green push | dev from any branch, prod from main | commit's tag; refuses if none | ci-release-tag.sh plus inline shell |
| code-vitals | pyproject.toml | 1 | build and test, push `:<ver>` and `:commit-<sha>` | not yet (no deploy) | none yet | n/a | compute-release-tag, publish-release-image, check-image-metadata |

## Semantics

One rule ties it together: a commit runs under exactly one version, chosen by its first deploy and fixed by a git tag.

| Term | Rule |
| --- | --- |
| Version | `MAJOR.MINOR` from the manifest + `PATCH` = next number on that line, from 0. A build claims it by pushing `refs/builds/<version>` create-only; GitHub refuses an existing name, so racing builds never share one and the loser retries with the next. The max includes existing release tags, so old pipeline-number tags are never reused. |
| Build | One push pipeline = one version = one set of images, built, tested, published as `<repo>:<version>` only. Publishing an existing version is refused. Images carry `org.opencontainers.image.version` and `.revision` labels. |
| Deploy | A manual Woodpecker deployment to a target. Never builds. Resolves the build, verifies the revision label equals the commit, runs the repo's deploy steps, then tags. |
| Resolve | Commit has a release tag → deploy that version. Otherwise take the commit's build refs, newest first, and deploy the first whose image is in the registry (failed builds never published, so are skipped). More than one tag on a commit is an error. |
| Git tag | Annotated tag `<version>`, byte-identical to the docker tag, pushed only after a successful deploy and smoke tests. Idempotent on redeploy; refuses a second tag on a commit or a tag moving commits. |
| Prod | Only from allowed branches (default `main`). Reuses dev's build through its tag if dev ran the commit; otherwise takes the latest green build and writes the tag itself. |
| Feature branches | May deploy to any pre-prod target; that tags the branch commit. With merge commits or fast-forward, prod can promote the same build; a squash merge makes a new commit needing its own build. |

## How steps share the version

Woodpecker cannot pass a variable between steps, so the version always travels in `.release-tag`; every step reads `$(cat .release-tag)`.

| Between | How |
| --- | --- |
| Steps of one workflow | Shared workspace: `version` writes the file; build args, labels, `publish` read it. |
| Workflows of one pipeline (v-note web, android) | Each runs `version`, which finds the number its pipeline already claimed (`refs/build-pipelines/<pipeline>/<version>`, pushed atomically with the build ref) instead of claiming another. |
| Build pipeline to deploy pipeline | Nothing passed: `resolve` works it out from the commit and writes `.release-tag`. |

## Where tests sit

Tests stay in each repo; the plugin fixes the two gates around them.

1. **Before `publish`** (build pipeline): build, then every test (unit, lint, Dockerfile test stages, e2e, `checks` workflows). `publish` runs only when all are green, so the registry holds tested builds only.
2. **Before `tag`** (deploy pipeline): `resolve`, deploy, live smoke tests. `tag` runs only when they pass; a failed smoke leaves the commit untagged.

## When a build fails

Its claimed number is used up (1.69.8 is never handed out again) but nothing is published, so nothing can deploy it. A commit with no green build is refused with "push a fix or re-run the build". The next attempt claims 1.69.9; gaps mark failed builds.

## Build, deploy dev, deploy prod in Woodpecker

Woodpecker has no gated multi-stage run like Azure DevOps. The equivalent is three runs: the push pipeline (build), Deploy with target `dev`, Deploy with target `prod`. The artifact is handed on through the registry and the git tag, keyed on the commit, so it does not matter which pipeline is promoted.

```
            writes tag            reads tag
              ┌──── Git tag 1.69.7 (pins the commit) ────┐
              │                                          ▼
 1. Build ──► 2. Deploy dev (optional, any branch) ──► 3. Deploy prod (main)
    version     resolve: tag, else build ref             resolve: tag, else build ref
    test        deploy same image                        deploy same image
    publish     push tag                                 push tag if not there
      │              ▲                                        ▲
      └── publish ── Registry: app:1.69.7, built once ── pull ┘
```

## Plugin changes (0.2.0)

| Mode | Runs on | Does | Replaces |
| --- | --- | --- | --- |
| `version` | push, manual | Reads `MAJOR.MINOR`, claims the next number via `refs/builds/<version>` (retry on clash), writes `.release-tag`. Refuses deployment events. | compute-release-tag / ci-release-tag / release-version compute |
| `publish` | push, manual, after tests | Pushes each locally built image as `:<version>`; refuses if it exists or the registry cannot answer; checks labels first. | publish-release-image, check-image-metadata, sovereign-config promote |
| `resolve` | deployment | Checks target and branch against `environments`, finds the release tag or newest published build ref, verifies revision labels, writes `.release-tag`. | find-commit-tag, resolve-deploy-tag, validate-deployment, verify-image |
| `tag` | deployment, after deploy and smoke | Pushes the annotated git tag; no-op if already there. `push-tag` kept as an alias. | push-tag, release-version push-tag, tag-release |

`compute` and `validate` are removed (`compute` is the collision-prone mode; `validate` only served it).

| Setting | Used by | Default | Example |
| --- | --- | --- | --- |
| `mode` | all | required | `resolve` |
| `major_minor_source` | version | required | `cargo`, `go`, `pyproject` (new) |
| `major_minor_file` | version | per source | `Cargo.toml` |
| `images` (new) | publish, resolve | required | `[registry.desync.link/sovereign-config, registry.desync.link/sovereign-config-cli]` |
| `registry_username`, `registry_password` (new) | publish, resolve | required | from_secret |
| `environments` (new) | resolve | `dev` any branch, `prod` from `main` | `{dev: {}}` for v-note; `{dev: {}, staging: {branches: [main]}, prod: {branches: [main], require_tag: true}}` |
| `github_token` | version, resolve, tag | required | from_secret |
| `tag_prefix`, `dry_run` | all | `""`, `false` | |

Implementation notes:

- Registry reads use the registry API (go-containerregistry), so `resolve` needs no docker socket. `publish` needs the socket because the image is in the agent's daemon.
- `resolve` writes `.release-tag` only after every check passes.
- The GitHub token goes in a git HTTP header, never the URL.
- Build refs live outside `refs/tags`, so they never show in GitHub tags or releases and plain clones do not fetch them. Pruning is optional (a line only needs its highest ref kept).
- **Same-commit race:** each build ref points at its own small annotated tag object (commit, branch, pipeline), not straight at the commit. Git skips the create-only lease check when a push would leave a ref at the value it already has, so two builds of one commit claiming the same number could otherwise both succeed. Found while prototyping and verified against a real git remote.

## A donor pipeline afterwards

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
  build:            # repo-specific: docker build --build-arg RELEASE_TAG=$(cat .release-tag) --label ...
    depends_on: [version]
  e2e:              # repo-specific tests
    depends_on: [build]
  publish:
    depends_on: [e2e]
    image: ghcr.io/vcheesbrough/woodpecker-plugin-release-versions:0.2.0
    volumes: [/var/run/docker.sock:/var/run/docker.sock]
    settings:
      mode: publish
      images: [registry.desync.link/bored]
      registry_username: { from_secret: zot_ci_user }
      registry_password: { from_secret: zot_ci_password }
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
      images: [registry.desync.link/bored]
      environments:      # optional; this is the default
        dev: {}
        prod: { branches: [main] }
      github_token: { from_secret: github_token }
      registry_username: { from_secret: zot_ci_user }
      registry_password: { from_secret: zot_ci_password }
  deploy-dev:       # repo-specific
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
```

App-specific deploy scripts (e.g. bored's `deploy-app.sh`) are out of scope.

## Moving to GitHub Actions

All state lives in git and the registry; the plugin is a container reading environment variables.

| Concern | Woodpecker | GitHub Actions |
| --- | --- | --- |
| Run the plugin | `image:` step with `settings:` | container action, `uses: vcheesbrough/woodpecker-plugin-release-versions@<version>` (needs an `action.yml`) |
| Commit, branch, run id | `CI_COMMIT_SHA`, `CI_COMMIT_BRANCH`, `CI_PIPELINE_NUMBER` | `GITHUB_SHA`, `GITHUB_REF_NAME`, `GITHUB_RUN_ID` when `GITHUB_ACTIONS=true` |
| Deploy target | `CI_PIPELINE_DEPLOY_TARGET` | the job's `environment:` name, passed as `deploy_target` |
| Share `.release-tag` | shared workspace | job `outputs:`, or each job re-runs `resolve` |
| Build, dev, prod | three runs | one workflow, three jobs with `needs:`; GitHub Environments give an approval gate before prod |

## Templating (considered, deferred)

Woodpecker has no cross-repo `template:`/`extends:`. Options: YAML anchors (one file only), a configuration extension (`WOODPECKER_CONFIG_EXTENSION_ENDPOINT`, an HTTP service returning pipeline YAML; separate from the sovereign-config broker, which is a secret extension on `WOODPECKER_SECRET_EXTENSION_ENDPOINT`), or plugins. This design stops at the plugin.

## Migration plan

1. **Plugin 0.2.0.** Four modes with tests ported from the donors' script tests. Remove `compute` and `validate`. Update this repo's own release workflow (it uses `compute`).
2. **code-vitals.** Proves `version` + `publish` with pyproject.
3. **bored.** Proves `resolve` + `tag` on dev and prod.
4. **sovereign-config.** Proves multiple images; moves from `-ci` repositories to one release repository per image, and to tag-on-deploy.
5. **v-note.** Proves multiple workflows agreeing on one version; moves to tag-on-deploy.

Existing tags stay valid: a commit tagged under an old rule counts as already deployed.

## Open questions

- v-note and sovereign-config lose "tagged = passed CI". Is that signal used anywhere?
- Merge style (merge, rebase, squash)? Squash means a dev-tested branch build can never itself be promoted to prod.
- Should code-vitals get a deploy file now?
- Registry retention in zot: `resolve` fails loudly if a tagged build has been pruned. Old `:commit-<sha>` tags can be cleared by retention.

## Prototype

A paused, unreviewed draft of 0.2.0 (modes, config, git ref claims, registry client, tests for config/git/manifest parsing) exists as a patch outside the repo. Mode-level tests (`version`, `resolve`, `tag`, `publish`), the GitHub Actions adapter and `action.yml` are not written.
