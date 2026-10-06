# sento

Sonarr/Radarr-style automation for manga: monitor series, find releases on
torrent indexers and Tachiyomi/Mihon sources, download, organize into a
clean CBZ library for Komga/Kavita.

**Status: early development — core loop works end-to-end.** Working today:
MangaBaka-backed library (series + collections + dual volume/chapter
ledgers), chapter search/download/import from Tachiyomi/Mihon extensions
through the bundled `sento-host` sidecar, a Torznab + qBittorrent volume
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
mise run build      # build bin/sento
mise run run        # run the backend (http://localhost:8484)
mise run host:build # build the extension sidecar (JVM; needed for sources)
mise run host:run   # run it on :8901, where the dev backend expects it
mise run web:dev    # web UI dev server (proxies /api to :8484)
mise run web:build  # build web UI into web/dist (served by the binary)
mise run test       # go test ./...
```

### Checks

CI is a thin runner over these tasks, so anything it does reproduces
locally with one command:

```sh
mise run ci         # everything a pull request runs: lint, types, tests, e2e
mise run test:race  # the Go suite under the race detector (what ci runs)
mise run e2e        # Playwright, driving a real sento against stubbed upstreams
mise run test:pg    # the Go suite against PostgreSQL (dialect conformance)
mise run gen:check  # fail if web/src/types is stale (fix: mise run gen)
mise run lint:ci    # actionlint over .github/workflows
mise run vuln       # govulncheck + npm audit (also runs nightly)
```

`mise run e2e` boots `cmd/e2estub` — deterministic stand-ins for MangaBaka,
the source host, Prowlarr and qBittorrent — alongside a real `sento`, so the
browser exercises the whole Go stack without touching the network.

Two suites deliberately sit outside `go test ./...` because they depend on
services this repo does not control. They run nightly (with `vuln`), and
on a Suwayomi bump. A red night opens one issue labelled `upstream-drift`;
the next green night closes it:

```sh
mise run test:contract   # the live MangaBaka API still matches our decoding
mise run host:contract   # the sidecar builds and serves its contract
```

Renovate bumps the vendored Suwayomi pin in `host/vendir.yml`. Because that
tree is what the extension sidecar is built on, any change under `host/`
triggers [`.github/workflows/sidecar.yml`](.github/workflows/sidecar.yml),
which builds the sidecar and runs `host:contract` against it — so a bump PR
arrives already knowing whether it works. Re-sync the vendored tree after a
pin change with `mise run host:vendor`.

## Deployment

```sh
cd deploy && docker compose up -d   # http://localhost:8484
```

[`deploy/docker-compose.yml`](deploy/docker-compose.yml) runs Sento and the
`sento-host` extension sidecar. Torrent indexers (Torznab endpoints from
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

### Container images

Published to the GitHub Container Registry on every push to `main` and on
`v*` tags, for `linux/amd64` and `linux/arm64`:

```sh
docker pull ghcr.io/h-wb/sento:latest        # the server
docker pull ghcr.io/h-wb/sento-host:latest   # the extension sidecar
```

Tags: `latest` (main), `<major>.<minor>.<patch>` and `<major>.<minor>` for
releases, plus the full commit sha. Build them locally with
`mise run image:build` and `mise run image:build:host`.

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
