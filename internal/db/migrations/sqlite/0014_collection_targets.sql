-- Adopt the edition model with an empty library. Settings, profiles and file bytes are preserved.
DELETE FROM decisions WHERE series_id IS NOT NULL;
DELETE FROM series;
ALTER TABLE collections RENAME TO collections_legacy;
ALTER TABLE works RENAME TO works_legacy;
CREATE TABLE collections (
 id INTEGER PRIMARY KEY AUTOINCREMENT, series_id BIGINT NOT NULL REFERENCES series(id) ON DELETE CASCADE,
 mb_collection_id TEXT, edition_name TEXT NOT NULL, language TEXT NOT NULL,
 publisher TEXT NOT NULL DEFAULT '', medium TEXT NOT NULL DEFAULT '', ctype TEXT NOT NULL DEFAULT 'volume',
 monitored INTEGER NOT NULL DEFAULT FALSE, count_main INTEGER, count_extra INTEGER, count_other INTEGER,
 start_date TEXT, end_date TEXT, raw_json TEXT, updated_at TEXT,
 custom INTEGER NOT NULL DEFAULT FALSE, profile_id BIGINT REFERENCES profiles(id), cutoff_num BIGINT NOT NULL DEFAULT 0,
 sync_state TEXT NOT NULL DEFAULT 'local', overrides_json TEXT NOT NULL DEFAULT '{}'
);
CREATE TABLE works (
 id INTEGER PRIMARY KEY AUTOINCREMENT, collection_id BIGINT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
 mb_work_id TEXT, sequence_num BIGINT NOT NULL, sequence_string TEXT, count_type TEXT NOT NULL DEFAULT 'main',
 release_date TEXT, isbn TEXT, pages INTEGER, raw_json TEXT,
 number_known INTEGER NOT NULL DEFAULT TRUE, revision TEXT NOT NULL DEFAULT '',
 sync_state TEXT NOT NULL DEFAULT 'local', overrides_json TEXT NOT NULL DEFAULT '{}'
);
DROP TABLE works_legacy;
DROP TABLE collections_legacy;

CREATE INDEX idx_collection_ref ON collections(series_id, mb_collection_id);
CREATE INDEX idx_work_ref ON works(collection_id, mb_work_id);
CREATE TABLE external_refs (
 provider TEXT NOT NULL, kind TEXT NOT NULL, external_id TEXT NOT NULL, local_id BIGINT NOT NULL,
 last_seen TEXT NOT NULL DEFAULT '', PRIMARY KEY(provider,kind,external_id,local_id)
);
CREATE TABLE download_targets (
 id INTEGER PRIMARY KEY AUTOINCREMENT, collection_id BIGINT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
 name TEXT NOT NULL, is_default INTEGER NOT NULL DEFAULT FALSE,
 monitored INTEGER NOT NULL DEFAULT TRUE, profile_id BIGINT REFERENCES profiles(id),
 cutoff_num BIGINT NOT NULL DEFAULT 0, required_group TEXT NOT NULL DEFAULT '',
 allow_unknown INTEGER NOT NULL DEFAULT TRUE, acquire_unknown INTEGER NOT NULL DEFAULT TRUE,
 UNIQUE(collection_id,name)
);
CREATE UNIQUE INDEX idx_default_target ON download_targets(collection_id) WHERE is_default = TRUE;
CREATE TABLE asset_coverage (
 file_id BIGINT NOT NULL REFERENCES files(id) ON DELETE CASCADE,
 target_id BIGINT NOT NULL REFERENCES download_targets(id) ON DELETE CASCADE,
 work_id BIGINT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
 verified INTEGER NOT NULL DEFAULT TRUE, pinned INTEGER NOT NULL DEFAULT FALSE,
 PRIMARY KEY(file_id,target_id,work_id)
);
ALTER TABLE files ADD COLUMN facts_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE files ADD COLUMN needs_review INTEGER NOT NULL DEFAULT FALSE;
ALTER TABLE queue_items ADD COLUMN provider_id TEXT NOT NULL DEFAULT '';
ALTER TABLE queue_items ADD COLUMN release_key TEXT NOT NULL DEFAULT '';
ALTER TABLE queue_items ADD COLUMN snapshot_json TEXT NOT NULL DEFAULT '{}';
CREATE TABLE unit_reservations (
 target_id BIGINT NOT NULL REFERENCES download_targets(id) ON DELETE CASCADE,
 work_id BIGINT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
 queue_id BIGINT NOT NULL REFERENCES queue_items(id) ON DELETE CASCADE,
 PRIMARY KEY(target_id,work_id)
);
CREATE TABLE unit_overrides (
 target_id BIGINT NOT NULL REFERENCES download_targets(id) ON DELETE CASCADE,
 work_id BIGINT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
 monitored INTEGER NOT NULL, PRIMARY KEY(target_id,work_id)
);
CREATE TABLE unit_observations (
 id INTEGER PRIMARY KEY AUTOINCREMENT, collection_id BIGINT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
 source TEXT NOT NULL, external_key TEXT NOT NULL, label TEXT NOT NULL,
 number BIGINT NOT NULL, state TEXT NOT NULL DEFAULT 'pending',
 UNIQUE(collection_id,source,external_key)
);
CREATE TABLE work_relations (
 from_id BIGINT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
 to_id BIGINT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
 relation TEXT NOT NULL CHECK(relation IN ('contains','equivalent','revision')),
 PRIMARY KEY(from_id,to_id,relation)
);

ALTER TABLE collections ADD COLUMN generated_bound INTEGER NOT NULL DEFAULT 0;


ALTER TABLE source_bindings ADD COLUMN collection_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE collections ADD COLUMN discover_chapters INTEGER NOT NULL DEFAULT FALSE;
CREATE INDEX idx_work_number ON works(collection_id,sequence_num);

CREATE UNIQUE INDEX idx_active_offer ON queue_items(provider_id,release_key) WHERE release_key<>'' AND status IN('queued','downloading','importing');

ALTER TABLE collections ADD COLUMN edition_layout TEXT NOT NULL DEFAULT '';
ALTER TABLE collections ADD COLUMN content_variant TEXT NOT NULL DEFAULT '';

ALTER TABLE collections ADD COLUMN numbering_source TEXT NOT NULL DEFAULT 'manual';

ALTER TABLE asset_coverage ADD COLUMN evidence TEXT NOT NULL DEFAULT 'observed';

DROP TABLE file_slot_links;
DROP TABLE slot_overrides;
DROP TABLE IF EXISTS chapter_ledgers;
