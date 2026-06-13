-- Indexer/site configuration (global defaults + per-series overrides).
-- A site absent from site_settings is enabled for both ledgers (zero-config
-- default). Series overrides use NULL = inherit.

CREATE TABLE site_settings (
  provider TEXT NOT NULL,
  site_id TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  use_volumes INTEGER NOT NULL DEFAULT 1,
  use_chapters INTEGER NOT NULL DEFAULT 1,
  priority INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY(provider, site_id)
);

CREATE TABLE series_site_settings (
  series_id INTEGER NOT NULL REFERENCES series(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  site_id TEXT NOT NULL,
  enabled INTEGER,      -- NULL = inherit
  use_volumes INTEGER,  -- NULL = inherit
  use_chapters INTEGER, -- NULL = inherit
  PRIMARY KEY(series_id, provider, site_id)
);
