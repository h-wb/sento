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

The design lives in [`specs/`](specs/):

- [`specs/spec.md`](specs/spec.md) — architecture, domain model, decisions log
- [`specs/parser.md`](specs/parser.md) — release-name parser & matcher
- [`specs/suwayomi-adapter.md`](specs/suwayomi-adapter.md) — extension-engine adapter (live-validated)
- [`specs/decision-engine.md`](specs/decision-engine.md) — grab decisions & release profiles
- [`specs/data-api.md`](specs/data-api.md) — DB schema & API surface

## Development

Tooling is pinned with [mise](https://mise.jdx.dev):

```sh
mise install        # Go, Node, sqlc, golangci-lint
mise run build      # build bin/sento
mise run run        # run the backend (http://localhost:8484)
mise run web:dev    # web UI dev server (proxies /api to :8484)
mise run web:build  # build web UI into web/dist (served by the binary)
mise run test       # go test ./...
```

## Deployment

```sh
cd deploy && docker compose up -d
```

Brings up Sento plus a pinned [Suwayomi-Server](https://github.com/Suwayomi/Suwayomi-Server)
(the Tachiyomi-extension engine). Configure extensions/sources through the
Suwayomi WebUI for now (see compose file to expose it).
