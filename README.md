# sento

Sonarr/Radarr-style automation for manga: monitor series, find releases on
torrent indexers and Tachiyomi/Mihon sources, download, organize into a
clean CBZ library for Komga/Kavita.

**Status: early development — core loop works end-to-end.** Working today:
MangaBaka-backed library (series + collections + dual volume/chapter
ledgers), chapter search/download/import from Tachiyomi/Mihon extensions
through the bundled `sento-extensions` sidecar, a Torznab + qBittorrent volume
path with a scored decision funnel and release profiles, regression-tested
release parser, background automation loops, existing-library import, and a
Mantine web UI.

Documentation lives in [`docs/`](docs/) — code map and data-flow
walkthroughs in [`docs/architecture.md`](docs/architecture.md) and
[`docs/flows/`](docs/flows/), product spec and decision log in
[`docs/spec/`](docs/spec/).

## Development

Tooling is pinned with [mise](https://mise.jdx.dev):

```sh
mise install        # Go, Node, Java, Gradle, vendir, linters — all pinned
mise run dev        # build and run the whole dev stack, then follow its logs
mise run build      # build bin/sento
mise run web:build  # build web UI into web/dist (served by the binary)
mise run test       # go test ./...
```

### Dev stack

The long-running processes are [mise daemons](https://mise.jdx.dev/daemons.html)
(`[daemons]` in `mise.toml`), supervised in the background by
[pitchfork](https://pitchfork.jdx.dev). Each one builds what it needs before
it starts, so there is no separate build step and no terminal per process:

| Daemon       | What                                   | Primary checkout      |
| ------------ | -------------------------------------- | --------------------- |
| `web`        | Vite dev server, proxies `/api` to api | http://127.0.0.1:5173 |
| `api`        | the Go backend (`bin/sento`)           | http://127.0.0.1:8484 |
| `extensions` | the extension sidecar (JVM)            | http://127.0.0.1:8901 |

```sh
mise run dev                # all three, in dependency order, then their logs
mise run dev:stop           # stop everything this checkout started
```

Ctrl-C in `mise run dev` only leaves the log view; the stack keeps running.
The pieces underneath, for finer control:

```sh
mise daemons start          # the same three, without the log view
mise daemons                # what is running
mise daemons logs -t        # follow the output (add a name for one daemon)
mise daemons restart api    # rebuild and restart one
mise daemons stop           # stop everything this checkout started
mise daemons urls           # the ports (and hostnames) of this checkout
```

- **They outlive the terminal.** Nothing stops them but `mise run dev:stop`
  (or `mise daemons stop`).
- **`api` rebuilds itself** when a Go file under `cmd/` or `internal/`
  changes. If that build fails it keeps serving the previous binary and
  logs `BUILD FAILED` — look at `mise daemons logs api` when a change does
  not seem to land.
- **The first start is slow**: it syncs and compiles the vendored Suwayomi
  tree for the sidecar. After that, unchanged builds are skipped.
- **Ports are per checkout.** The table is the primary checkout; a linked
  git worktree gets its own ports (and its own `data/`), so two checkouts
  run side by side. `mise daemons urls` shows them.
- Daemons are an experimental mise feature, enabled for this repo in
  `mise.toml`.

Under [Paseo](https://github.com/h-wb/paseo) the same three run as the
services declared in [`paseo.json`](paseo.json), on Paseo-assigned ports.
Both call the same mise tasks (`extensions:run`, `run`, `web:dev`), so each
process has one definition of how it starts; `mise run extensions:run`,
`mise run run` and `mise run web:dev` also still work by hand in terminals.

### Checks

CI is a thin runner over these tasks, so anything it does reproduces
locally with one command:

```sh
mise run ci         # everything a pull request runs: lint, types, tests, e2e
mise run test:race  # the Go suite under the race detector (what ci runs)
mise run e2e        # Playwright, driving a real sento against stubbed upstreams
mise run test:pg    # the Go suite against PostgreSQL (starts its own server)
mise run gen:check  # fail if web/src/types is stale (fix: mise run gen)
mise run lint:ci    # actionlint over .github/workflows
mise run vuln       # govulncheck + npm audit (also runs nightly)
```

`mise run e2e` boots `cmd/e2estub` — deterministic stand-ins for MangaBaka,
the extensions sidecar, Prowlarr and qBittorrent — alongside a real `sento`, so the
browser exercises the whole Go stack without touching the network.

Two suites deliberately sit outside `go test ./...` because they depend on
services this repo does not control. They run nightly (with `vuln`), and
on a Suwayomi bump. A red night opens one issue labelled `upstream-drift`;
the next green night closes it:

```sh
mise run test:contract   # the live MangaBaka API still matches our decoding
mise run extensions:contract   # the sidecar builds and serves its contract
```

Renovate bumps the vendored Suwayomi pin in `extensions/vendir.yml`. Because that
tree is what the extension sidecar is built on, any change under `extensions/`
triggers [`.github/workflows/sidecar.yml`](.github/workflows/sidecar.yml),
which builds the sidecar and runs `extensions:contract` against it — so a bump PR
arrives already knowing whether it works. Re-sync the vendored tree after a
pin change with `mise run extensions:vendor`.

## Deployment

```sh
cd deploy && docker compose up -d   # http://localhost:8484
```

[`deploy/docker-compose.yml`](deploy/docker-compose.yml) runs Sento and the
`sento-extensions` extension sidecar. Torrent indexers (Torznab endpoints from
Prowlarr or Jackett) and qBittorrent are services you already run: add them
on the Indexers and Connections pages. No extension repos ship with either image —
add a Mihon-compatible repo URL on the Connections page, then add sources
on the Indexers page; Sento installs the extensions they need.

Things worth knowing before the first run:

- **No authentication yet.** Anyone who can reach port 8484 can use the
  API. Keep it on a trusted network or behind an authenticating reverse
  proxy. (`server.api_key` exists in the config but is not enforced.)
- **Paths.** The image keeps its database in `/config`, writes the library
  to `/library` and stages source downloads in `/downloads`.
- **Torrent imports hardlink** from qBittorrent's download folder so
  seeding continues. Mount that folder into the Sento container, on the
  same filesystem as the library, and tell Sento where it is with
  `SENTO_PATH_MAPPINGS` (`remote=local`) if the two containers see it under
  different paths.
- **SQLite by default**; set `SENTO_DB_URL=postgres://…` for PostgreSQL.

Configuration is a YAML file (`/config/config.yml`, optional) overridden by
`SENTO_*` environment variables — see
[`internal/config/config.go`](internal/config/config.go). Connection
settings saved in the UI are stored in the database and win over both.

The extensions sidecar is the exception: `SENTO_EXTENSIONS_URL` and
`SENTO_EXTENSIONS_REPOS` (extension repo index URLs, comma-separated) stay
in force when set, and the UI then shows them read-only. That lets a
deployment declare the sidecar and its repos next to the sidecar itself.

### Container images

Published to the GitHub Container Registry on every push to `main` and on
`v*` tags, for `linux/amd64` and `linux/arm64`:

```sh
docker pull ghcr.io/h-wb/sento:latest        # the server
docker pull ghcr.io/h-wb/sento-extensions:latest   # the extension sidecar
```

Tags: `latest` (main), `<major>.<minor>.<patch>` and `<major>.<minor>` for
releases, plus the full commit sha. Build them locally with
`mise run image:build` and `mise run image:build:extensions`.

A package published from a private repository is private, and GHCR only
accepts a **classic** personal access token (with `read:packages`) for
pulling one — fine-grained tokens do not work there. In Kubernetes:

```sh
kubectl create secret docker-registry ghcr \
  --docker-server=ghcr.io --docker-username=<user> --docker-password=<classic PAT>
```

Making the packages public instead (package settings, independent of the
repository's visibility) drops the secret entirely: public images pull
anonymously.

## Credits

Extension sources run in the `sento-extensions` sidecar, which is built
on [Suwayomi-Server](https://github.com/Suwayomi/Suwayomi-Server)
(MPL-2.0): its fake Android runtime and extension loader are what make
Tachiyomi/[Mihon](https://github.com/mihonapp/mihon) extensions run
outside Android. What is used and under which terms is in
[`extensions/NOTICE.md`](extensions/NOTICE.md).

Series and book metadata comes from [MangaBaka](https://mangabaka.org).
