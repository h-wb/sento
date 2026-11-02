# Releases

Sento uses one version for the server, UI and extension sidecar. The Git tag
is the source of the runtime version; `web/package.json` is private tooling
and does not carry the product version.

## Version policy

PR titles follow [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/).
Use squash merges so each PR contributes one classified commit.

| Change | Before 1.0 | From 1.0 |
| --- | --- | --- |
| `fix`, `perf` | Patch | Patch |
| `feat` | Minor | Minor |
| `!` or `BREAKING CHANGE:` | Minor | Major |
| `docs`, `test`, `ci`, `chore`, `build`, `refactor` | No release on their own | No release on their own |

A change to existing configuration, API or library behavior that requires user
action must be marked breaking and include migration instructions. Review
schema migrations before release: downgrading does not undo them. The first
release is configured as `0.1.0`; the manifest records versions thereafter.
Pre-release tags are not supported by the publication workflows yet.

## Maintainer setup (once)

1. Enable squash merges in the repository settings and use the PR title as
   the squash commit title. Protect main with PR review and required CI checks.
2. Create `RELEASE_PLEASE_TOKEN` as an Actions secret. Use a fine-grained token
   from your account or a bot account, scoped to this repository, with Contents,
   Issues and Pull requests read/write permissions.
   The bot also needs permission to create releases and tags under any rulesets.
3. Set the Actions variable `RELEASE_AUTOMATION_ENABLED` to `true`. The Release PR
   workflow is disabled until then, including on forks. Never expose its token
   to workflows that run pull-request code.

Release Please needs a bot token because events created with `GITHUB_TOKEN`
do not trigger ordinary CI and tag workflows. See [the action's token guidance](https://github.com/googleapis/release-please-action#github-credentials).
The checked-in action is pinned by commit. Default workflow tokens only get
permissions needed by each workflow.

## Cutting a release

After a successful **push CI** run on main, Release Please opens or updates a
release PR with a version and changelog, based on commits since the last
release. Review the version, edit the notes if necessary, and merge that PR
when its checks pass. After main's CI passes, Release Please creates the tag
and GitHub release. Nothing is automatically deployed to a live instance.

[Release](../.github/workflows/release.yml) attaches server binaries, a source
archive and SHA-256 checksums. [Image](../.github/workflows/image.yml) publishes
both multi-architecture images. Both independently require a stable
`vMAJOR.MINOR.PATCH` tag on main with a successful main CI run for that commit.
Release notes can appear before these publication jobs finish; check both
workflows before announcing availability.

If a publication job fails, fix its cause and rerun the failed jobs. If the
workflow itself needed a correction, run **Release → Run workflow** from main
and supply the existing release tag (for example `v0.1.0`). Recovery uses the
current workflow but checks out the tagged source, validates its original CI
run, and stamps its original commit. Artifact upload is repeatable and keeps
the curated release notes. Do not move an existing release tag. A manual
annotated tag on a passing main commit is supported for recovery, but the
release-PR path is preferred.

## Artifacts and image channels

| Artifact | Contents |
| --- | --- |
| `sento-X.Y.Z-OS-ARCH.tar.gz` | Server binary, `web-dist/`, GPL license |
| `sento-X.Y.Z-source.tar.gz` | Source at the release commit, including pinned dependency manifests |
| `checksums.txt` | SHA-256 sums of the archives |

Binary targets are Linux and macOS, amd64 and arm64. Run extracted binaries
with `SENTO_STATIC_DIR=./web-dist ./sento`. The JVM sidecar stays a separate
container. See its [third-party notices](../extensions/NOTICE.md) for the
vendored Suwayomi source and license.

Both `ghcr.io/h-wb/sento` and `ghcr.io/h-wb/sento-extensions` publish:

- `X.Y.Z`, `X.Y` and `latest` for stable releases (no `v` in image tags).
- `edge` and `main` for development builds from main.
- `sha-<full commit>` for commit-pinned builds.

**Channel change:** `latest` follows stable releases, rather than main.
Existing installs that want continuous development updates should use `edge`
for both images. The Compose example defaults to `edge` until you select a
release with `SENTO_VERSION`. Existing users of `latest` will stay on its last
image until a stable release is published.

Local `mise run build` stamps the binary with `git describe` output. Container
and release builds inject their tag and commit at link time.
