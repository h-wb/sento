# Installation and configuration

## Containers

[The Compose file](../deploy/docker-compose.yml) starts Sento and its extension
sidecar. It publishes Sento on port 8484 and keeps the sidecar on the private
Compose network. No extension repositories are bundled.

```sh
cd deploy
docker compose up -d
```

The default `edge` tag follows main and can contain unfinished changes.
For a released version, set `SENTO_VERSION` in your shell or in `deploy/.env`
so **both** images use the same version. Copy a version from the
[releases page](https://github.com/h-wb/sento/releases). `latest` tracks stable
releases once the first release has been published.

Configure qBittorrent and Torznab indexers in the UI. To use extension sources,
add a Mihon-compatible repository under Connections and add a source under
Indexers. The first connection may take time while extensions install.

Sento does not enforce authentication. Put it behind an authenticating reverse
proxy or restrict it to a trusted network. Do not publish the sidecar's port.

## Storage

| Container path | Purpose |
| --- | --- |
| `/config` | SQLite database and optional `config.yml` |
| `/library` | Imported CBZ files |
| `/downloads` | Staging directory for extension downloads |
| Additional torrent mount | qBittorrent's completed downloads, visible to Sento |

For torrents, Sento must be able to read the actual files, not just connect to
qBittorrent's API. Put the completed-download mount and library on the same
filesystem for hardlinks. If qBittorrent reports `/downloads/book.cbz` but Sento
sees it at `/torrents/book.cbz`, set:

```yaml
environment:
  SENTO_PATH_MAPPINGS: /downloads=/torrents
```

The Compose example includes a commented mount for this case. Ensure the
container can write to the library and staging directories.

## Configuration

An optional YAML file is loaded first, then `SENTO_*` variables override it.
The image looks for `/config/config.yml`. These are the common variables;
[config.go](../internal/config/config.go) contains the full set and defaults.

| Variable | Purpose |
| --- | --- |
| `SENTO_HOST`, `SENTO_PORT` | HTTP listener |
| `SENTO_DB_PATH` | SQLite database path |
| `SENTO_DB_URL` | PostgreSQL connection URL; takes precedence over SQLite |
| `SENTO_LIBRARY_PATH` | Library root |
| `SENTO_DOWNLOADS_DIR` | Source download staging |
| `SENTO_STATIC_DIR` | Built web UI directory (already set in the image) |
| `SENTO_PATH_MAPPINGS` | Comma-separated `remote=local` torrent paths |
| `SENTO_EXTENSIONS_URL` | Sidecar URL |
| `SENTO_EXTENSIONS_REPOS` | Comma- or space-separated extension repository URLs |

qBittorrent and Prowlarr environment settings seed their initial connections;
settings saved in the UI take precedence after that. `SENTO_EXTENSIONS_URL`
and `SENTO_EXTENSIONS_REPOS` stay in force and are read-only in the UI when
supplied by the environment. The `SENTO_API_KEY` configuration field currently
does not protect routes; do not rely on it for authentication.

## Server binaries

Release archives contain `sento` and `web-dist/`. Extract the archive and run
from that directory:

```sh
SENTO_STATIC_DIR=./web-dist ./sento
```

The extension runtime is a separate JVM application; the simplest way to run
it is the matching `ghcr.io/h-wb/sento-extensions` container. Point Sento at its
private URL with `SENTO_EXTENSIONS_URL`. There is no standalone native sidecar
binary in the server archive.

## Backups and upgrades

Stop Sento before copying a SQLite database; copy the config volume and library,
and preserve your original download files. Use PostgreSQL's own backup tools
when using that backend. Backups can include source credentials and should be
kept private.

Read the release notes, take a backup, then change `SENTO_VERSION` for both
containers and run:

```sh
docker compose pull
docker compose up -d
```

Database migrations run at startup. Downgrading an image does not reverse a
migration; restore a compatible database backup when rolling back. Pre-1.0
minor releases may contain breaking changes.
