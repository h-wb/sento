# sento

Sonarr/Radarr-style automation for manga: monitor series, find releases on
torrent indexers and Tachiyomi/Mihon sources, download, organize into a
clean CBZ library for Komga/Kavita.

**Status: early development — core loop works end-to-end.** Working today:
MangaBaka-backed library (series + editions + dual volume/chapter ledgers),
Suwayomi-powered chapter search/download/import with ComicInfo, Prowlarr +
qBittorrent volume path with a scored decision funnel, regression-tested
release parser, background automation loops, existing-library import, and a
Mantine web UI.

Documentation lives in [`docs/`](docs/) — code map and data-flow
walkthroughs in [`docs/architecture.md`](docs/architecture.md) and
[`docs/flows/`](docs/flows/), product spec and decision log in
[`docs/spec/`](docs/spec/).

## Development

Tooling is pinned with [mise](https://mise.jdx.dev):

```sh
mise install        # Go, Node, Java, sqlc, golangci-lint, vendir
mise run build      # build bin/sento
mise run run        # run the backend (http://localhost:8484)
mise run web:dev    # web UI dev server (proxies /api to :8484)
mise run web:build  # build web UI into web/dist (served by the binary)
mise run test       # go test ./...
```

### Checks

CI is a thin runner over these tasks, so anything it does reproduces
locally with one command:

```sh
mise run ci         # everything a pull request runs: lint, types, tests, e2e
mise run e2e        # Playwright, driving a real sento against stubbed upstreams
mise run test:pg    # the Go suite against PostgreSQL (dialect conformance)
mise run gen:check  # fail if web/src/types is stale (fix: mise run gen)
```

`mise run e2e` boots `cmd/e2estub` — deterministic stand-ins for MangaBaka,
the source host, Prowlarr and qBittorrent — alongside a real `sento`, so the
browser exercises the whole Go stack without touching the network.

Two suites deliberately sit outside `go test ./...` because they depend on
services this repo does not control. They run nightly, and on a Suwayomi
bump:

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

### Container image

Published to the GitHub Container Registry on every push to `main` and on
`v*` tags, for `linux/amd64` and `linux/arm64`:

```sh
docker pull ghcr.io/h-wb/sento:latest
```

Tags: `latest` (main), `<major>.<minor>.<patch>` and `<major>.<minor>` for
releases, plus the full commit sha. Build it locally with
`mise run image:build`.

A package published from a private repository is private, and GHCR only
accepts a **classic** personal access token (with `read:packages`) for
pulling one — fine-grained tokens do not work there. In Kubernetes:

```sh
kubectl create secret docker-registry ghcr \
  --docker-server=ghcr.io --docker-username=<user> --docker-password=<classic PAT>
```

Making the package public instead (package settings, independent of the
repository's visibility) drops the secret entirely: public images pull
anonymously.


```sh
cd deploy && docker compose up -d
```

Brings up Sento plus a pinned [Suwayomi-Server](https://github.com/Suwayomi/Suwayomi-Server)
(the Tachiyomi-extension engine). Configure extensions/sources through the
Suwayomi WebUI for now (see compose file to expose it).
