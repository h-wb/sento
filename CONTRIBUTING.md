# Contributing

Bug reports, documentation fixes and pull requests are welcome. For a larger
change, open an issue first so we can agree on the behavior before you build it.

## Report a problem

Search [existing issues](https://github.com/h-wb/sento/issues) first, then use
one of the [issue forms](https://github.com/h-wb/sento/issues/new/choose).
Include the Sento version, what you expected, what happened, and steps that
someone else can follow. A release name, edition, error from Activity or a
screenshot often helps. Remove passwords, tokens and private tracker URLs.

Issues track current bugs and planned work.

For vulnerabilities, follow the [security policy](SECURITY.md) rather than
posting details in a public issue.

## Work on the code

Follow [the development guide](docs/development.md). Tools are pinned in
`mise.toml`; use the tasks there rather than a system Go or Node installation.
You do not need the maintainer's services, credentials or agent tools.

Keep a pull request focused on one problem. Describe the resulting behavior
and how you checked it. Add a regression test for a bug when practical, and
update the relevant docs when changing behavior. New architecture should fit
the [domain model](docs/domain/editions.md); explain any change to that model.

Before submitting:

```sh
mise run ci
```

Database changes also need `mise run test:pg`. Sidecar changes need
`mise run extensions:contract`. The [testing guide](docs/testing/README.md)
explains these checks, including which ones use the network. If an environment
prevents a check, say which check and why in the PR.

Contributions to original Sento code use the project's [GPL-3.0-only license](LICENSE).
Preserve upstream notices when modifying third-party code.

AI-assisted contributions are welcome. The contributor is responsible for
understanding the change, checking its behavior and reviewing its diff.

## Commit messages

Use [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/)
for PR titles. We squash merge, so the PR title becomes the release-relevant
commit; your intermediate commits do not need to follow this format.

```text
fix(import): preserve the original file when a hardlink fails
feat(profiles): add a color edition profile
docs: explain remote download paths
chore(deps): update the web dependencies
```

Accepted types: `feat`, `fix`, `perf`, `refactor`, `docs`, `test`, `build`,
`ci`, `chore`, and `revert`. The scope is optional. Add `!` for a breaking
change, for example `feat(api)!: replace the queue response format`, and
explain the migration in the PR body. Before 1.0, features and breaking changes bump the
minor version; fixes bump the patch version. See
[the release guide](docs/releases.md) for the policy and maintainer setup.
