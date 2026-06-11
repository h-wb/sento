-- Indexers become explicit entries (Sonarr's model): each row is one
-- searchable thing with a protocol — a Torznab endpoint (torrent) or an
-- extension source on the host (source). The user names the entry; that
-- name appears in candidates, history and the queue. Search/ledger
-- toggles live on the entry, replacing the torrent/source rows of
-- site_settings (converted at startup, where connection settings are
-- available to derive per-indexer Torznab URLs).
CREATE TABLE indexers (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  protocol TEXT NOT NULL CHECK (protocol IN ('torznab','source')),
  enabled INTEGER NOT NULL DEFAULT 1,
  use_volumes INTEGER NOT NULL DEFAULT 1,
  use_chapters INTEGER NOT NULL DEFAULT 1,
  priority INTEGER NOT NULL DEFAULT 0,
  settings_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
