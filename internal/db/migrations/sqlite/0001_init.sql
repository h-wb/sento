-- Sento schema v1. Reference: specs/data-api.md.
-- Num convention: volume/chapter numbers stored as integer hundredths.

CREATE TABLE root_folders (
  id INTEGER PRIMARY KEY,
  path TEXT NOT NULL UNIQUE
);

CREATE TABLE profiles (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  body_json TEXT NOT NULL
);

CREATE TABLE series (
  id INTEGER PRIMARY KEY,
  mangabaka_id INTEGER NOT NULL UNIQUE,
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
  monitored INTEGER NOT NULL DEFAULT 1,
  root_folder_id INTEGER REFERENCES root_folders(id),
  path TEXT,
  profile_id INTEGER REFERENCES profiles(id),
  raw_json TEXT NOT NULL,
  raw_schema_version INTEGER NOT NULL DEFAULT 1,
  overrides_json TEXT,
  added_at TEXT NOT NULL DEFAULT (datetime('now')),
  refreshed_at TEXT
);

CREATE TABLE collections (
  id INTEGER PRIMARY KEY,
  series_id INTEGER NOT NULL REFERENCES series(id) ON DELETE CASCADE,
  mb_collection_id TEXT,
  edition_name TEXT NOT NULL,
  language TEXT NOT NULL,
  publisher TEXT NOT NULL DEFAULT '',
  medium TEXT NOT NULL DEFAULT '',
  ctype TEXT NOT NULL DEFAULT 'volume',
  monitored INTEGER NOT NULL DEFAULT 0,
  count_main INTEGER,
  count_extra INTEGER,
  count_other INTEGER,
  start_date TEXT,
  end_date TEXT,
  raw_json TEXT,
  updated_at TEXT,
  UNIQUE(series_id, edition_name, language, publisher, medium, ctype)
);

CREATE TABLE works (
  id INTEGER PRIMARY KEY,
  collection_id INTEGER NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
  mb_work_id TEXT,
  sequence_num INTEGER NOT NULL,
  sequence_string TEXT,
  count_type TEXT NOT NULL DEFAULT 'main',
  release_date TEXT,
  isbn TEXT,
  pages INTEGER,
  raw_json TEXT,
  UNIQUE(collection_id, sequence_num, count_type)
);

CREATE TABLE chapter_ledgers (
  series_id INTEGER PRIMARY KEY REFERENCES series(id) ON DELETE CASCADE,
  monitored INTEGER NOT NULL DEFAULT 1,
  language TEXT NOT NULL DEFAULT 'en',
  cutoff_num INTEGER NOT NULL DEFAULT 0,
  source_seen_max INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE slot_overrides (
  series_id INTEGER NOT NULL REFERENCES series(id) ON DELETE CASCADE,
  ledger TEXT NOT NULL CHECK (ledger IN ('volume','chapter')),
  collection_id INTEGER NOT NULL DEFAULT 0,
  number INTEGER NOT NULL,
  monitored INTEGER NOT NULL,
  PRIMARY KEY(series_id, ledger, collection_id, number)
);

CREATE TABLE files (
  id INTEGER PRIMARY KEY,
  series_id INTEGER NOT NULL REFERENCES series(id) ON DELETE CASCADE,
  path TEXT NOT NULL UNIQUE,
  size INTEGER NOT NULL,
  hash TEXT,
  parse_json TEXT,
  provenance TEXT NOT NULL DEFAULT 'import',
  unrequested INTEGER NOT NULL DEFAULT 0,
  added_at TEXT NOT NULL DEFAULT (datetime('now')),
  modified_at TEXT
);

CREATE TABLE file_slot_links (
  file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  ledger TEXT NOT NULL CHECK (ledger IN ('volume','chapter')),
  collection_id INTEGER NOT NULL DEFAULT 0,
  number INTEGER NOT NULL,
  PRIMARY KEY(file_id, ledger, collection_id, number)
);
CREATE INDEX idx_links_slot ON file_slot_links(ledger, collection_id, number);

CREATE TABLE source_bindings (
  id INTEGER PRIMARY KEY,
  series_id INTEGER NOT NULL REFERENCES series(id) ON DELETE CASCADE,
  source_id TEXT NOT NULL,
  source_manga_id INTEGER NOT NULL,
  source_name TEXT,
  state TEXT NOT NULL DEFAULT 'active',
  priority INTEGER NOT NULL DEFAULT 0,
  bound_at TEXT NOT NULL DEFAULT (datetime('now')),
  last_checked_at TEXT,
  UNIQUE(series_id, source_id)
);

CREATE TABLE queue_items (
  id INTEGER PRIMARY KEY,
  series_id INTEGER NOT NULL REFERENCES series(id) ON DELETE CASCADE,
  protocol TEXT NOT NULL CHECK (protocol IN ('torrent','source')),
  external_id TEXT NOT NULL,
  title TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'queued',
  expected_slots_json TEXT NOT NULL,
  error TEXT,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT
);

CREATE TABLE decisions (
  id INTEGER PRIMARY KEY,
  series_id INTEGER REFERENCES series(id) ON DELETE SET NULL,
  body_json TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE blocklist (
  id INTEGER PRIMARY KEY,
  protocol TEXT NOT NULL,
  key TEXT NOT NULL,
  reason TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  expires_at TEXT,
  UNIQUE(protocol, key)
);
