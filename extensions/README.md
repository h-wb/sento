# sento-extensions

> Built on [Suwayomi-Server](https://github.com/Suwayomi/Suwayomi-Server)
> (MPL-2.0) and the [Tachiyomi](https://github.com/tachiyomiorg) /
> [Mihon](https://github.com/mihonapp/mihon) extension API. See
> [Credits and licensing](#credits-and-licensing).

A thin JVM sidecar that runs Tachiyomi/Mihon extensions and exposes them
over a small HTTP API shaped like Sento's `internal/extensions.Host`
contract: sources, manga search, chapter lists, page refs, page bytes,
plus extension repo/install management. No extensions or repos ship with
it — users add Mihon-compatible extension repo index URLs in Sento.

The sidecar is a stateless protocol adapter — Sento owns queueing, pacing,
CBZ assembly and import. The only state here is the directory of
installed extension jars (`data/extensions`, with `installed.json` as the
index) and the fake-Android `shared_prefs` for configurable sources.

## How extensions run

Extensions are APKs of Dalvik bytecode. Install converts them with
dex2jar, then loads the jar in a child-first classloader against a fake
Android runtime from [Suwayomi-Server](https://github.com/Suwayomi/Suwayomi-Server)
(MPL-2.0). The Suwayomi pieces are declared in `vendir.yml` and
materialized into the gitignored `vendor/` by
[vendir](https://carvel.dev/vendir/) (`mise run extensions:vendor`), pinned by
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
`mise run extensions:vendor`, fix what moved.

## Extension repos

A repo is added by its index URL. The sidecar reads the current store format
— a protobuf (usually gzipped) or JSON document listing every extension
with its APK URL — and follows the two addresses older repos still hand
out, `index.min.json` and `repo.json`, to the store they now point at
(`index_v2`). The old flat `index.min.json` list itself is not read: repos
that moved on serve only "update your app" placeholders there.

When a repo also publishes a ready-built jar per extension, it is used
instead of converting the APK with dex2jar.

## Run

```sh
mise daemons start extensions   # vendir sync + gradle :app:installDist, then run it
mise daemons logs -t extensions
```

That is the `extensions` daemon from the repo's `mise.toml`: port 8901 in the
primary checkout, data in `extensions/data`. By hand, without the supervisor:

```sh
mise run extensions:build   # vendir sync + gradle :app:installDist
SENTO_EXTENSIONS_PORT=8901 SENTO_EXTENSIONS_DATA=data mise run extensions:run
```

Environment: `SENTO_EXTENSIONS_PORT` (8901), `SENTO_EXTENSIONS_DATA` (data),
`SENTO_EXTENSIONS_REPOS` (optional comma-separated extension repo index URLs
for standalone use — Sento normally pushes its configured repo list),
`SENTO_EXTENSIONS_FLARESOLVERR_URL` (optional, for Cloudflare-protected
sources).

Point Sento at it with `SENTO_EXTENSIONS_URL=http://localhost:8901` (or
on the Connections page).

As a container: `mise run image:build:extensions` builds `extensions/Dockerfile`
(published as `ghcr.io/h-wb/sento-extensions`); `deploy/docker-compose.yml` runs
it next to Sento.

## API

```
GET    /health
GET/PUT /api/v1/repos
GET    /api/v1/repos/status
GET    /api/v1/sources
GET    /api/v1/sources/{sid}/search?q=
GET    /api/v1/sources/{sid}/chapters?manga={key}
GET    /api/v1/sources/{sid}/pages?chapter={key}
GET    /api/v1/sources/{sid}/page?ref={ref}
GET    /api/v1/extensions
POST   /api/v1/extensions/{pkg}
DELETE /api/v1/extensions/{pkg}
```

Keys and page refs are opaque JSON envelopes minted by the sidecar; Sento
stores and round-trips them without interpreting.

## Credits and licensing

The hard part of this sidecar — running Android extension APKs on a
plain JVM — is the work of the
[Suwayomi](https://github.com/Suwayomi/Suwayomi-Server) contributors.
sento-extensions compiles Suwayomi-Server's fake Android runtime and
extension loader unmodified, at the release pinned in `vendir.yml`, and
adds a small HTTP API on top. The extension API itself comes from
Tachiyomi and Mihon.

- Suwayomi-Server is licensed under the Mozilla Public License 2.0
  ([`LICENSES/MPL-2.0.txt`](LICENSES/MPL-2.0.txt)). The vendored files
  keep their headers; `ExtensionManager.kt`, adapted from Suwayomi's
  `Extension.kt`, is under the MPL 2.0 as well.
- [`NOTICE.md`](NOTICE.md) lists exactly what is used, what is adapted,
  and where the corresponding source is. It ships inside the container
  image next to the license text.

If you find this sidecar useful, the project to thank and support is
Suwayomi.
