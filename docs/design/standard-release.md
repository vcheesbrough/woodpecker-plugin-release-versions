# Standard build, release and promotion design

Status: design agreed with Vincent on 2026-10-21. No implementation yet.
Living copy (with comments): https://claude.ai/artifact/FPZcFvSt4MzgNaUctRoKCg

Donor repos: bored, v-note, sovereign-config, code-vitals.

## Summary

Every repo builds once per push and gets version `MAJOR.MINOR.<next number in sequence>`. That one image is promoted unchanged to dev and prod, in either order. The git tag is written by the first successful deploy, so a commit is pinned to one build forever and prod only ever runs a tagged version. All of the versioning, lookup, policy and tagging logic moves into this plugin; each repo keeps only its own `docker build`, tests and deploy commands.

Decisions (all made by Vincent):

1. **A git tag means "this build was deployed"** (bored's rule), written on the first successful deploy to any environment. v-note and sovereign-config stop tagging on push.
2. **Deploy order is free.** Dev may come before prod, after it, or not at all, and there is no setting to force an order. Whichever deploys a commit first writes the tag, and every later deploy reuses that build.
3. **Environment policy lives in the plugin, with an escape hatch:** the release rules are fixed; which environments a repo has, and which branches may deploy to each, is one per-repo `environments` setting.
4. **Sequential versions:** patch numbers are a gap-free per-line counter, not `CI_PIPELINE_NUMBER`.
5. **No `:commit-<sha>` docker tags:** a deploy finds a commit's build through git build refs.
6. **Stay on Woodpecker 3.15** (3.19 release notes checked; nothing here needs it). Its button is Deploy and fires the `deployment` event.
7. **Must remain usable from GitHub Actions** if a repo moves (see below).
8. **The plugin enforces the rules; an agent-shared skill teaches them.** Agents wire, change and operate these pipelines, so they need the contract before they touch a pipeline, not after a refusal (see "Agent skill").

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
| Git tag | Annotated tag `<version>`, byte-identical to the docker tag, pushed once the deploy steps succeed, whatever the smoke tests later find. Idempotent on redeploy; refuses a second tag on a commit or a tag moving commits. |
| Prod | Only from allowed branches (default `main`). Dev is optional and may come before or after prod. Whichever deploys the commit first takes its latest green build and writes the tag; every later deploy, dev or prod, reuses that build through the tag. |
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
2. **Around `tag`** (deploy pipeline): `resolve`, deploy, then `tag` as soon as the deploy steps succeed. Live smoke tests run beside it: a failure fails the pipeline but the commit stays tagged, because that build was deployed, just broken, as if manual QA had found the bug. Only a failed deploy step leaves the commit untagged.

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
| `tag` | deployment, after the deploy steps | Pushes the annotated git tag; no-op if already there. `push-tag` kept as an alias. | push-tag, release-version push-tag, tag-release |

`compute` and `validate` are removed (`compute` is the collision-prone mode; `validate` only served it).

| Setting | Used by | Default | Example |
| --- | --- | --- | --- |
| `mode` | all | required | `resolve` |
| `major_minor_source` | version | required | `cargo`, `go`, `pyproject` (new) |
| `major_minor_file` | version | per source | `Cargo.toml` |
| `images` (new) | publish, resolve | required | `[registry.desync.link/sovereign-config, registry.desync.link/sovereign-config-cli]` |
| `registry_username`, `registry_password` (new) | publish, resolve | required | from_secret |
| `environments` (new) | resolve | `dev` any branch, `prod` from `main` | `{dev: {}}` for v-note; `{dev: {}, staging: {branches: [main]}, prod: {branches: [main]}}`; only `branches` per entry, deliberately no ordering option |
| `github_token` | version, resolve, tag | required | from_secret |
| `tag_prefix`, `dry_run` | all | `""`, `false` | |

Implementation notes:

- Registry reads use the registry API (go-containerregistry), so `resolve` needs no docker socket. `publish` needs the socket because the image is in the agent's daemon.
- `resolve` writes `.release-tag` only after every check passes.
- The GitHub token goes in a git HTTP header, never the URL.
- Build refs live outside `refs/tags`, so they never show in GitHub tags or releases and plain clones do not fetch them. They are never pruned: the counter needs the highest ref on each line, `resolve` needs a commit's ref to find an untagged build, and even 15,000 refs a year costs little.
- **Same-commit race:** each build ref points at its own small annotated tag object (commit, branch, pipeline), not straight at the commit. Git skips the create-only lease check when a push would leave a ref at the value it already has, so two builds of one commit claiming the same number could otherwise both succeed. Found while prototyping and verified against a real git remote.
- **Finding a commit's builds** takes one call, with nothing fetched. `resolve` runs `git ls-remote origin 'refs/builds/*'`; for each ref that points at a tag object, it also prints a `<ref>^{}` line with the commit inside it. Filtering those lines by the commit's SHA gives its builds. `version` lists only its own line (`refs/builds/1.69.*`). Git itself was checked on protocols v0 and v2; the prototype must confirm GitHub returns the `^{}` lines for refs outside `refs/tags`. If it does not, the fallback is fetching `refs/builds/*` and reading the tag objects locally.

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
  smoke:            # repo-specific; beside tag: a failure fails the pipeline, never the tag
    depends_on: [deploy-dev, deploy-prod]
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

## Agent skill (agent-shared)

The plugin refuses bad releases, but only at run time. An agent that does not know the rules hand-writes a tag, rebuilds to deploy or edits a patch number, and finds out when CI fails. A shared skill, `release-flow`, gives agents the contract before they act. It ships with plugin 0.2.0 in `agent-shared/skills/release-flow/`, next to `ci-watch` and `observability`.

| Part | Holds |
| --- | --- |
| Load when | Wiring or changing a repo's build or deploy pipeline; adding an image or environment; deploying a version; answering which version runs where; migrating a repo; taking a product to `1.0.0`. |
| The contract | The Semantics rules in agent terms: one build per push, promote and never rebuild, the first successful deploy writes the tag, the plugin owns `PATCH`. |
| Never | Create, move or delete a release tag or `refs/builds/*` by hand; set `PATCH` in a manifest; rebuild or retag an image to deploy it; push `:latest` or `:commit-<sha>`; tag on push; make `tag` wait for the smoke tests (a broken deploy is still a deploy). |
| Pipeline templates | `references/woodpecker-build.yml`, `references/woodpecker-deploy.yml`, `references/github-actions.yml`: the donor pipelines above, pinned to a plugin version. |
| Operating it | Deploy = a Woodpecker deployment for a commit and target, then `ci-watch` on that pipeline. Which version is where: `git tag --points-at <sha>` for deployed, `git ls-remote origin 'refs/builds/*'` for built. |
| Refusals | Each `version`, `publish`, `resolve` and `tag` refusal, with its cause and fix (no green build, branch not allowed, second tag, revision label mismatch, image pruned). Plugin error messages name the section to read. |
| Migrating a repo | Checklist: delete the repo's release scripts, add the two pipeline files, set `environments`, keep existing tags, prove it with `dry_run` on a branch. |

What stays out of the skill: the settings reference (the plugin README, linked at the pinned version, so the two cannot disagree) and per-repo facts such as image names and environments (the repo's own `AGENTS.md`).

The skill also changes three existing agent-shared files, in the same commit:

| File | Today | Becomes |
| --- | --- | --- |
| `global/AGENTS.md` §2 | Patch "resets to 0 at iteration start and bumps only on the active branch"; MVP "set the workspace to `1.0.0` and tag it". | The manifest carries `MAJOR.MINOR.0`; the plugin assigns `PATCH` per build. The `1.0.0` tag is written by that build's first deploy, never by hand. |
| `global/AGENTS.md` §10 | Lists `api-versioning`, `observability`. | Adds `release-flow`. |
| `skills/start-iteration` step 7 | "Patch resets to `0` at iteration start." | Set the manifest to `0.N.0` / `1.N.0`; patch numbers come from builds, so a feature branch builds `0.N.x` while trunk is still on `0.(N-1).x`. |

## Templating (considered, deferred)

Woodpecker has no cross-repo `template:`/`extends:`. Options: YAML anchors (one file only), a configuration extension (`WOODPECKER_CONFIG_EXTENSION_ENDPOINT`, an HTTP service returning pipeline YAML; separate from the sovereign-config broker, which is a secret extension on `WOODPECKER_SECRET_EXTENSION_ENDPOINT`), or plugins. This design stops at the plugin.

## Migration plan

1. **Plugin 0.2.0.** Four modes with tests ported from the donors' script tests. Remove `compute` and `validate`. Update this repo's own release workflow (it uses `compute`).
2. **agent-shared `release-flow` skill.** Written against 0.2.0, with the baseline §2, §10 and `start-iteration` changes. Committed to agent-shared `main` (its own rule: no branch). Every repo migration below is done by following it, and whatever a migration finds missing goes back into the skill.
3. **code-vitals.** Proves `version` + `publish` with pyproject.
4. **bored.** Proves `resolve` + `tag` on dev and prod.
5. **sovereign-config.** Proves multiple images; moves from `-ci` repositories to one release repository per image, and to tag-on-deploy.
6. **v-note.** Proves multiple workflows agreeing on one version; moves to tag-on-deploy.

Existing tags stay valid: a commit tagged under an old rule counts as already deployed.

## Open questions

- v-note and sovereign-config lose "tagged = passed CI". Is that signal used anywhere?
- Merge style (merge, rebase, squash)? Squash means a dev-tested branch build can never itself be promoted to prod.
- Should code-vitals get a deploy file now?
- May agents start deployments, and to which targets? The Woodpecker MCP has no deploy action, so it would take `woodpecker-cli` and a token. If prod stays human-only, the skill says so and stops at "ready to deploy".
- Registry retention in zot: `resolve` fails loudly if a tagged build has been pruned. Old `:commit-<sha>` tags can be cleared by retention.

## Prototype

A paused, unreviewed draft of 0.2.0 (modes, config, git ref claims, registry client, tests for config/git/manifest parsing) exists as a patch outside the repo. Mode-level tests (`version`, `resolve`, `tag`, `publish`), the GitHub Actions adapter and `action.yml` are not written.
