# Sento — Data Model (SQLite) & API Surface

Mechanical translation of `spec.md` §4 into DDL and endpoints. This file is
the reference for migration `0001`; once code exists, **migrations are the
truth** and this doc tracks intent, not every column.

Conventions:

- `Num` columns (volume/chapter numbers) are **integer hundredths**
  (parser.md §2): chapter 34.5 → `3450`. Counts (`total_chapters`) are
  plain integers.
- Timestamps are ISO-8601 TEXT (UTC, `datetime('now')`).
- Raw MangaBaka payloads stored as JSON TEXT with `raw_schema_version`
  (D13).
- Booleans are INTEGER 0/1.
- `file_slot_links.collection_id = 0` means "chapter ledger" (SQLite PKs
  can't hold NULL).

## 1. Schema

```sql
CREATE TABLE root_folders (
  id INTEGER PRIMARY KEY,
  path TEXT NOT NULL UNIQUE
);

CREATE TABLE profiles (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  body_json TEXT NOT NULL          -- decision-engine.md §5 document
);

CREATE TABLE series (
  id INTEGER PRIMARY KEY,
  mangabaka_id INTEGER NOT NULL UNIQUE,
  title TEXT NOT NULL,
  sort_title TEXT NOT NULL,
  type TEXT NOT NULL DEFAULT 'manga',
  status TEXT NOT NULL DEFAULT 'unknown',     -- releasing|completed|…
  content_rating TEXT,
  year INTEGER,
  total_chapters INTEGER,                     -- MangaBaka count (denominator)
  final_volume INTEGER,
  description TEXT,
  cover_url TEXT,
  monitored INTEGER NOT NULL DEFAULT 1,
  root_folder_id INTEGER REFERENCES root_folders(id),
  path TEXT,                                  -- series folder
  profile_id INTEGER REFERENCES profiles(id),
  raw_json TEXT NOT NULL,
  raw_schema_version INTEGER NOT NULL DEFAULT 1,
  overrides_json TEXT,                        -- user overrides, reapplied post-refresh
  added_at TEXT NOT NULL DEFAULT (datetime('now')),
  refreshed_at TEXT
);

-- Editions. Identity = descriptor tuple (D13), MangaBaka UUID is a cache.
CREATE TABLE collections (
  id INTEGER PRIMARY KEY,
  series_id INTEGER NOT NULL REFERENCES series(id) ON DELETE CASCADE,
  mb_collection_id TEXT,                      -- volatile, re-resolved on refresh
  edition_name TEXT NOT NULL,
  language TEXT NOT NULL,
  publisher TEXT NOT NULL DEFAULT '',
  medium TEXT NOT NULL DEFAULT '',
  ctype TEXT NOT NULL DEFAULT 'volume',
  monitored INTEGER NOT NULL DEFAULT 0,
  count_main INTEGER, count_extra INTEGER, count_other INTEGER,
  start_date TEXT, end_date TEXT,
  raw_json TEXT,
  updated_at TEXT,
  UNIQUE(series_id, edition_name, language, publisher, medium, ctype)
);

-- Volume slots (derived from works; per-slot deviations in slot_overrides).
CREATE TABLE works (
  id INTEGER PRIMARY KEY,
  collection_id INTEGER NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
  mb_work_id TEXT,
  sequence_num INTEGER NOT NULL,              -- Num (hundredths)
  sequence_string TEXT,
  count_type TEXT NOT NULL DEFAULT 'main',    -- main|extra|other
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
  cutoff_num INTEGER NOT NULL DEFAULT 0,      -- Num; monitor from here (D6)
  source_seen_max INTEGER NOT NULL DEFAULT 0  -- Num; highest seen on sources (D7)
);

-- Only deviations from default monitoring (spec.md §4.1).
CREATE TABLE slot_overrides (
  series_id INTEGER NOT NULL REFERENCES series(id) ON DELETE CASCADE,
  ledger TEXT NOT NULL CHECK (ledger IN ('volume','chapter')),
  collection_id INTEGER NOT NULL DEFAULT 0,   -- 0 = chapter ledger
  number INTEGER NOT NULL,                    -- Num
  monitored INTEGER NOT NULL,
  PRIMARY KEY(series_id, ledger, collection_id, number)
);

CREATE TABLE files (
  id INTEGER PRIMARY KEY,
  series_id INTEGER NOT NULL REFERENCES series(id) ON DELETE CASCADE,
  path TEXT NOT NULL UNIQUE,
  size INTEGER NOT NULL,
  hash TEXT,
  parse_json TEXT,                            -- parser Result incl. trace
  provenance TEXT NOT NULL DEFAULT 'import',  -- torrent|source|library-import|manual
  unrequested INTEGER NOT NULL DEFAULT 0,     -- spec.md §4.3
  added_at TEXT NOT NULL DEFAULT (datetime('now')),
  modified_at TEXT
);

CREATE TABLE file_slot_links (
  file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  ledger TEXT NOT NULL CHECK (ledger IN ('volume','chapter')),
  collection_id INTEGER NOT NULL DEFAULT 0,   -- 0 = chapter ledger
  number INTEGER NOT NULL,                    -- Num
  PRIMARY KEY(file_id, ledger, collection_id, number)
);
CREATE INDEX idx_links_slot ON file_slot_links(ledger, collection_id, number);

CREATE TABLE source_bindings (
  id INTEGER PRIMARY KEY,
  series_id INTEGER NOT NULL REFERENCES series(id) ON DELETE CASCADE,
  source_id TEXT NOT NULL,                    -- Suwayomi source id (stringified int64)
  source_manga_id INTEGER NOT NULL,           -- Suwayomi manga row id
  source_name TEXT,
  state TEXT NOT NULL DEFAULT 'active',       -- active|broken|disabled
  priority INTEGER NOT NULL DEFAULT 0,
  bound_at TEXT NOT NULL DEFAULT (datetime('now')),
  last_checked_at TEXT,
  UNIQUE(series_id, source_id)
);

CREATE TABLE queue_items (
  id INTEGER PRIMARY KEY,
  series_id INTEGER NOT NULL REFERENCES series(id) ON DELETE CASCADE,
  protocol TEXT NOT NULL CHECK (protocol IN ('torrent','source')),
  external_id TEXT NOT NULL,                  -- infohash | suwayomi chapter id
  title TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'queued',
    -- queued|downloading|completed|importing|imported|failed
  expected_slots_json TEXT NOT NULL,          -- pending slot set
  error TEXT,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT
);

CREATE TABLE decisions (
  id INTEGER PRIMARY KEY,
  series_id INTEGER REFERENCES series(id) ON DELETE SET NULL,
  body_json TEXT NOT NULL,                    -- decision-engine.md §8 record
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE blocklist (
  id INTEGER PRIMARY KEY,
  protocol TEXT NOT NULL,
  key TEXT NOT NULL,                          -- infohash | source:chapterId
  reason TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  expires_at TEXT,                            -- NULL = until cleared
  UNIQUE(protocol, key)
);
```

Plus `schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT)`.

Derivation reminders (not tables): volume slots = `works` × default-
monitored ∓ `slot_overrides`; chapter slots = `cutoff..max(total_chapters,
source_seen_max)`; *wanted* = monitored ∧ released ∧ no `file_slot_links`
row ∧ not pending in `queue_items` ∧ not blocklisted.

## 2. API surface (v1)

REST/JSON under `/api/v1`, API-key header auth, OpenAPI generated from
code. SSE at `/api/v1/events` (queue, import, health event stream).

| Area | Endpoints |
|---|---|
| System | `GET /system/health` (liveness) · `GET /system/status` (version, peripherals health) |
| Lookup | `GET /lookup?q=` — MangaBaka search for add-flow |
| Series | `GET /series` · `POST /series` (mangabaka_id, root folder, collections to monitor, options) · `GET /series/{id}` · `PUT /series/{id}` · `DELETE /series/{id}?deleteFiles=` · `POST /series/{id}/refresh` |
| Collections | `PUT /series/{id}/collections/{cid}` (monitored, …) |
| Ledgers/slots | `GET /series/{id}/slots?ledger=` (slot status incl. files) · `PUT /series/{id}/slots` (bulk monitor) · `PUT /series/{id}/chapter-ledger` (cutoff, language, monitored) |
| Wanted | `GET /wanted` (paged, both ledgers) |
| Search/grab | `POST /series/{id}/search` (interactive; full funnel w/ traces) · `POST /grab` (candidate ref, force flag) |
| Queue | `GET /queue` · `DELETE /queue/{id}?blocklist=` |
| Files | `GET /series/{id}/files` · `DELETE /files/{id}` · `POST /series/{id}/organize` (dry-run flag) |
| Import | `POST /import/scan` (root folder → job) · `GET /import/jobs/{id}` · `GET /import/manual` · `POST /import/manual` (resolve queued items) |
| Bindings | `GET/POST/DELETE /series/{id}/bindings` |
| Profiles | CRUD `/profiles` |
| Root folders | CRUD `/rootfolders` |
| History | `GET /history` (decisions, grabs, imports) |
| Settings | `GET/PUT /settings/{suwayomi,prowlarr,qbittorrent,mangabaka,naming,general}` |

Job-shaped operations (refresh, scan, search-all) return `202 {jobId}` and
publish progress on SSE.
