# sento-host

A thin JVM sidecar that runs Tachiyomi/Mihon extensions (the
[keiyoushi](https://github.com/keiyoushi/extensions) ecosystem) and exposes
them over a small HTTP API shaped like Sento's `internal/sourcehost.Host`
contract: sources, manga search, chapter lists, page refs, page bytes,
plus extension install/uninstall.

The host is a stateless protocol adapter — Sento owns queueing, pacing,
CBZ assembly and import. The only state here is the directory of
installed extension jars (`data/extensions`, with `installed.json` as the
index) and the fake-Android `shared_prefs` for configurable sources.

## How extensions run

Extensions are APKs of Dalvik bytecode. Install converts them with
dex2jar, then loads the jar in a child-first classloader against a fake
Android runtime from [Suwayomi-Server](https://github.com/Suwayomi/Suwayomi-Server)
(MPL-2.0), consumed two ways:

- **`Suwayomi-Server/` git submodule** (shallow, pinned): the
  `AndroidCompat` module and the dependency version catalog build
  straight from it — it is a standalone Gradle module upstream.
- **Vendored shim layer** (`app/src/main/kotlin/eu/kanade`,
  `…/suwayomi/tachidesk/manga`): the extension API surface and APK
  loading machinery, which upstream are entangled with the server tree.
  `scripts/vendor.sh` re-syncs them from the submodule after a pin bump;
  `suwayomi.tachidesk.server.*` stand-ins in `app/` keep them compiling
  without Suwayomi's server.

Clone with `--recursive` (or run `git submodule update --init`) before
building. To pick up upstream fixes: bump the submodule pin, re-run
`scripts/vendor.sh`, fix what moved.

## Run

```sh
gradle :app:installDist
SENTO_HOST_PORT=8901 SENTO_HOST_DATA=data app/build/install/app/bin/app
```

Environment: `SENTO_HOST_PORT` (8901), `SENTO_HOST_DATA` (data),
`SENTO_HOST_REPOS` (comma-separated extension repo index URLs, defaults
to keiyoushi), `SENTO_HOST_FLARESOLVERR_URL` (optional, for
Cloudflare-protected sources).

Point Sento at it with `SENTO_SOURCE_HOST_URL=http://localhost:8901`
(takes precedence over Suwayomi).

## API

```
GET    /health
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
