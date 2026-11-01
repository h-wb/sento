# Contributing

Thanks for wanting to help — Sento grew out of one person's workflow (built
with AI, see the [README](README.md)), so adapting it to fit more people's
needs is exactly the kind of contribution it needs.

## Issues

- **Bugs and friction**: [open an issue](https://github.com/h-wb/sento/issues/new/choose).
  Describe what you did, what you expected, and what happened; screenshots
  and the Activity page's error details help a lot.
- Before filing, skim [docs/dogfooding.md](docs/dogfooding.md) — known
  friction points are collected there, and your report may sharpen an
  existing entry rather than open a new one.

## Pull requests

- Keep the change focused: one concern per PR is easier to review and land.
- Match the existing style; there is no separate style guide — the code is
  the guide.
- CI runs lint, tests, e2e and type generation; run them locally first:

  ```sh
  mise run ci
  ```

- If the change touches product behavior, say why in the PR description;
  behavior decisions are logged in [docs/spec/spec.md](docs/spec/spec.md)
  (§1, the decision log), and a maintainer can help log a new one.

## Development setup

See the [Development](README.md#development) section of the README and
[AGENTS.md](AGENTS.md) for the day-to-day conventions (pinned tooling via
mise, the dev stack, the checks). The guided code tour in
[docs/tour.md](docs/tour.md) is a good way in if you want to understand a
whole subsystem.
