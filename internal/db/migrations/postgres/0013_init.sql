-- PostgreSQL baseline: the schema as of SQLite migration 0013, consolidated
-- (postgres support landed then; no earlier pg deployments exist). Future
-- migrations are written in both dialect directories under the same number.
--
-- Conventions match the store layer: integer ids; timestamps are TEXT in
-- UTC 'YYYY-MM-DD HH:MM:SS' (written by the app, defaulted here to the
-- same format); booleans are real BOOLEANs (the store compares with
-- TRUE/FALSE literals and Go bool parameters, valid in both dialects).
-- Num convention: volume/chapter numbers stored as integer hundredths.

CREATE OR REPLACE FUNCTION sento_now() RETURNS TEXT
  LANGUAGE sql STABLE
  AS $$ SELECT to_char(now() AT TIME ZONE 'utc', 'YYYY-MM-DD HH24:MI:SS') $$;

CREATE TABLE root_folders (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  path TEXT NOT NULL UNIQUE
);

CREATE TABLE profiles (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  body_json TEXT NOT NULL
);

CREATE TABLE series (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  mangabaka_id BIGINT NOT NULL UNIQUE,
  title TEXT NOT NULL,
  sort_title TEXT NOT NULL,
  type TEXT NOT NULL DEFAULT 'manga',
  status TEXT NOT NULL DEFAULT 'unknown',
  content_rating TEXT,
  year INTEGER,
  total_chapters INTEGER,
  final_volume INTEGER,
  description TEXT,
  cover_url TEXT,
  monitored BOOLEAN NOT NULL DEFAULT TRUE,
  root_folder_id BIGINT REFERENCES root_folders(id),
  path TEXT,
  profile_id BIGINT REFERENCES profiles(id),
  raw_json TEXT NOT NULL,
  raw_schema_version INTEGER NOT NULL DEFAULT 1,
  overrides_json TEXT,
  added_at TEXT NOT NULL DEFAULT sento_now(),
  refreshed_at TEXT
);

CREATE TABLE collections (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  series_id BIGINT NOT NULL REFERENCES series(id) ON DELETE CASCADE,
  mb_collection_id TEXT,
  edition_name TEXT NOT NULL,
  language TEXT NOT NULL,
  publisher TEXT NOT NULL DEFAULT '',
  medium TEXT NOT NULL DEFAULT '',
  ctype TEXT NOT NULL DEFAULT 'volume',
  monitored BOOLEAN NOT NULL DEFAULT FALSE,
  count_main INTEGER,
  count_extra INTEGER,
  count_other INTEGER,
  start_date TEXT,
  end_date TEXT,
  raw_json TEXT,
  updated_at TEXT,
  custom BOOLEAN NOT NULL DEFAULT FALSE,
  profile_id BIGINT REFERENCES profiles(id),
  cutoff_num BIGINT NOT NULL DEFAULT 0,
  UNIQUE(series_id, edition_name, language, publisher, medium, ctype)
);

CREATE TABLE works (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  collection_id BIGINT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
  mb_work_id TEXT,
  sequence_num BIGINT NOT NULL,
  sequence_string TEXT,
  count_type TEXT NOT NULL DEFAULT 'main',
  release_date TEXT,
  isbn TEXT,
  pages INTEGER,
  raw_json TEXT,
  UNIQUE(collection_id, sequence_num, count_type)
);

CREATE TABLE slot_overrides (
  series_id BIGINT NOT NULL REFERENCES series(id) ON DELETE CASCADE,
  ledger TEXT NOT NULL CHECK (ledger IN ('volume','chapter')),
  collection_id BIGINT NOT NULL DEFAULT 0,
  number BIGINT NOT NULL,
  monitored BOOLEAN NOT NULL,
  PRIMARY KEY(series_id, ledger, collection_id, number)
);

CREATE TABLE files (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  series_id BIGINT NOT NULL REFERENCES series(id) ON DELETE CASCADE,
  path TEXT NOT NULL UNIQUE,
  size BIGINT NOT NULL,
  hash TEXT,
  parse_json TEXT,
  provenance TEXT NOT NULL DEFAULT 'import',
  unrequested BOOLEAN NOT NULL DEFAULT FALSE,
  added_at TEXT NOT NULL DEFAULT sento_now(),
  modified_at TEXT
);

CREATE TABLE file_slot_links (
  file_id BIGINT NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  ledger TEXT NOT NULL CHECK (ledger IN ('volume','chapter')),
  collection_id BIGINT NOT NULL DEFAULT 0,
  number BIGINT NOT NULL,
  PRIMARY KEY(file_id, ledger, collection_id, number)
);
CREATE INDEX idx_links_slot ON file_slot_links(ledger, collection_id, number);

CREATE TABLE queue_items (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  series_id BIGINT NOT NULL REFERENCES series(id) ON DELETE CASCADE,
  protocol TEXT NOT NULL CHECK (protocol IN ('torrent','source')),
  external_id TEXT NOT NULL,
  title TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'queued',
  expected_slots_json TEXT NOT NULL,
  error TEXT,
  created_at TEXT NOT NULL DEFAULT sento_now(),
  updated_at TEXT
);

CREATE TABLE decisions (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  series_id BIGINT REFERENCES series(id) ON DELETE SET NULL,
  body_json TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT sento_now()
);

CREATE TABLE blocklist (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  protocol TEXT NOT NULL,
  key TEXT NOT NULL,
  reason TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT sento_now(),
  expires_at TEXT,
  UNIQUE(protocol, key)
);

CREATE TABLE series_site_settings (
  series_id BIGINT NOT NULL REFERENCES series(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  site_id TEXT NOT NULL,
  enabled BOOLEAN,      -- NULL = inherit
  use_volumes BOOLEAN,  -- NULL = inherit
  use_chapters BOOLEAN, -- NULL = inherit
  PRIMARY KEY(series_id, provider, site_id)
);

CREATE TABLE settings (
  key TEXT PRIMARY KEY,
  value_json TEXT NOT NULL,
  updated_at TEXT NOT NULL DEFAULT sento_now()
);

CREATE TABLE source_bindings (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  series_id BIGINT NOT NULL REFERENCES series(id) ON DELETE CASCADE,
  source_id TEXT NOT NULL,
  source_manga_key TEXT NOT NULL,
  source_name TEXT,
  manga_title TEXT,
  state TEXT NOT NULL DEFAULT 'active',
  priority INTEGER NOT NULL DEFAULT 0,
  bound_at TEXT NOT NULL DEFAULT sento_now(),
  last_checked_at TEXT,
  UNIQUE(series_id, source_id, source_manga_key)
);

CREATE TABLE indexers (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name TEXT NOT NULL,
  protocol TEXT NOT NULL CHECK (protocol IN ('torznab','source')),
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  use_volumes BOOLEAN NOT NULL DEFAULT TRUE,
  use_chapters BOOLEAN NOT NULL DEFAULT TRUE,
  priority INTEGER NOT NULL DEFAULT 0,
  settings_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT sento_now()
);

CREATE TABLE custom_formats (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  conditions_json TEXT NOT NULL DEFAULT '[]'
);
