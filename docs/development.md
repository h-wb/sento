# Development

Sento has a Go backend, a React/TypeScript web UI, and a Kotlin/JVM extension
sidecar. [mise](https://mise.jdx.dev) pins their toolchains in `mise.toml`.
Start from a fresh clone:

```sh
mise trust
mise install
mise run web:install
mise run dev
mise daemons urls
```

Open the **web** URL. Vite proxies `/api` to the backend. The backend does not
serve a development UI unless `web/dist` has been built, so a 404 at its root
is expected. A fresh database answers `GET /api/v1/series` with `200 []`.

## Services and local settings

The dev stack uses mise daemons supervised by pitchfork. Ports are assigned
per checkout, so worktrees can run alongside one another. The first sidecar
build downloads and compiles vendored Suwayomi code; later starts use the cache.

```sh
mise daemons logs -t extensions api web
mise run dev:stop
```

Ctrl-C leaves the log view and keeps the services running. The API rebuilds
on Go changes. If a rebuild fails, `BUILD FAILED` appears in its logs and the
previous binary keeps serving. Vite hot-reloads the UI.

Local SQLite data and library files live under `data/`. Put machine-specific
environment overrides and credentials in `mise.local.toml` (gitignored):

```toml
[env]
SENTO_QBITTORRENT_URL = "http://localhost:8080"
SENTO_PROWLARR_URL = "http://localhost:9696"
```

Saved connection settings take precedence over these initial seeds. The
extension URL and repository environment settings remain enforced overrides.
See [configuration](installation.md#configuration). Never commit credentials
or a populated database.

For a backend-only session, `mise run run` starts Go without the sidecar or UI.
Extension errors are expected until a sidecar is available; library features
still work. `mise run build` creates `bin/sento` with a version derived from git.
Use `mise tasks` to list all tasks.

## Finding the code

[Architecture](architecture.md) maps packages and their dependencies.
[Editions](domain/editions.md) and [acquisition](domain/acquisition.md) explain
the ownership and download rules. The web entry points live in `web/src`,
and generated API types in `web/src/types`.

After changing Go API structs, run `mise run gen` and include the generated
types in the same PR. Schema changes need matching numbered migrations under
both `internal/db/migrations/sqlite` and `internal/db/migrations/postgres`.
Do not edit migrations that have already shipped.

## Optional Paseo integration

`paseo.json` declares the same services with worktree-specific ports. It is
optional; the terminal workflow above needs no Paseo installation. Start
`extensions`, then `api`, then `web` through Paseo's service controls. Restart
web after restarting api because Paseo may assign a new API port.

The `fixtures`, `sandboxapi` and `sandboxweb` services run the offline
[exploratory harness](exploratory-testing.md), using a separate database and
library under `data/exploratory/`.

## Checks

See [Testing](testing/README.md) for focused checks and harness conventions.
`mise run ci` runs the standard local suite; PostgreSQL and JVM contract checks
are separate tasks, as they are in GitHub Actions.
