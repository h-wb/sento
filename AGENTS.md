# Agent notes

Read [CONTRIBUTING.md](CONTRIBUTING.md) and [docs/architecture.md](docs/architecture.md)
first. Domain behavior is described in [docs/domain/](docs/domain) and the
stable specs in [docs/spec/](docs/spec). Current work belongs in issues.

- Use `mise run <task>` with the pinned toolchains in `mise.toml`. Do not use
  system Go/npm. Setup, service commands and diagnostics are in
  [docs/development.md](docs/development.md).
- Start services with `mise run dev` or the services in `paseo.json`. Do not
  leave manually backgrounded processes running. Open the web URL from
  `mise daemons urls` or the Paseo service listing.
- Keep local settings in gitignored `mise.local.toml`. Do not assume access
  to production services or the maintainer's credentials.
- Run checks appropriate to the change; [docs/testing/README.md](docs/testing/README.md)
  explains the suites. `mise run ci` runs the standard checks.
- In the Paseo agent image only, run browser tests with the hidden
  `mise run e2e:paseo` task. [docs/testing/paseo.md](docs/testing/paseo.md)
  explains its shared dependency cache and why the root installer fails there.
- Run `mise run gen` after changing API structs and include `web/src/types`.
  Schema migrations need both SQLite and PostgreSQL versions.
- Preserve spec filenames, section numbers and decision IDs cited by code.
  Update domain docs when changing behavior.
- Use Conventional Commit PR titles. Do not create releases or change live
  services as part of a code or documentation task unless explicitly asked.
