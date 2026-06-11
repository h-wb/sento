# sento-host

A thin JVM sidecar that runs Tachiyomi/Mihon extensions and exposes them
over a small HTTP API shaped like Sento's `internal/sourcehost.Host`
contract: sources, manga search, chapter lists, page refs, page bytes,
plus extension repo/install management. No extensions or repos ship with
it — users add Mihon-compatible extension repo index URLs in Sento.

The host is a stateless protocol adapter — Sento owns queueing, pacing,
CBZ assembly and import. The only state here is the directory of
installed extension jars (`data/extensions`, with `installed.json` as the
index) and the fake-Android `shared_prefs` for configurable sources.

## How extensions run

Extensions are APKs of Dalvik bytecode. Install converts them with
dex2jar, then loads the jar in a child-first classloader against a fake
Android runtime from [Suwayomi-Server](https://github.com/Suwayomi/Suwayomi-Server)
(MPL-2.0). The Suwayomi pieces are declared in `vendir.yml` and
materialized into the gitignored `vendor/` by
[vendir](https://carvel.dev/vendir/) (`mise run host:vendor`), pinned by
`vendir.lock.yml`:

- **`AndroidCompat` (+ Config)**: the fake Android runtime — a
  standalone Gradle module upstream, built as-is; the dependency version
  catalog also loads from the sync.
- **The extension API surface + APK loading machinery**
  (`eu.kanade.tachiyomi`, `suwayomi.tachidesk.manga.impl`): compiled
  into `app/` against the `suwayomi.tachidesk.server.*` stand-ins in
  `app/src` (extend the stand-ins; never patch vendored files — files we
  replace, like `GetCatalogueSource`, are excluded in `vendir.yml`).

To pick up upstream fixes: bump the ref in `vendir.yml`, run
`mise run host:vendor`, fix what moved.

## Run

```sh
mise run host:build   # vendir sync + gradle :app:installDist
SENTO_HOST_PORT=8901 SENTO_HOST_DATA=data app/build/install/app/bin/app
```

Environment: `SENTO_HOST_PORT` (8901), `SENTO_HOST_DATA` (data),
`SENTO_HOST_REPOS` (optional comma-separated extension repo index URLs
for standalone use — Sento normally pushes its configured repo list),
`SENTO_HOST_FLARESOLVERR_URL` (optional, for Cloudflare-protected
sources).

Point Sento at it with `SENTO_SOURCE_HOST_URL=http://localhost:8901`
(takes precedence over Suwayomi).

## API

```
GET    /health
GET/PUT /api/v1/repos
GET    /api/v1/sources
GET    /api/v1/sources/{sid}/search?q=
GET    /api/v1/sources/{sid}/chapters?manga={key}
GET    /api/v1/sources/{sid}/pages?chapter={key}
GET    /api/v1/sources/{sid}/page?ref={ref}
GET    /api/v1/extensions
POST   /api/v1/extensions/{pkg}
DELETE /api/v1/extensions/{pkg}
```

Keys and page refs are opaque JSON envelopes minted by the host; Sento
stores and round-trips them without interpreting.
