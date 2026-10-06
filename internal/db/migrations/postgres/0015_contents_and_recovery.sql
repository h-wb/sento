-- File contents survive collection-policy changes and target deletion.
CREATE TABLE asset_contents (
 file_id BIGINT NOT NULL REFERENCES files(id) ON DELETE CASCADE,
 work_id BIGINT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
 verified BOOLEAN NOT NULL DEFAULT TRUE,
 evidence TEXT NOT NULL DEFAULT 'observed',
 PRIMARY KEY(file_id,work_id)
);
INSERT INTO asset_contents(file_id,work_id,verified,evidence)
 SELECT file_id,work_id,CASE WHEN SUM(CASE WHEN verified THEN 1 ELSE 0 END)>0 THEN TRUE ELSE FALSE END,
 CASE WHEN SUM(CASE WHEN evidence='manual' THEN 1 ELSE 0 END)>0 THEN 'manual' ELSE 'observed' END
 FROM asset_coverage GROUP BY file_id,work_id;
ALTER TABLE asset_coverage ADD COLUMN selected BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE download_targets ADD COLUMN include_extras BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE files ADD COLUMN claims_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE files ADD COLUMN presence TEXT NOT NULL DEFAULT 'unknown';
ALTER TABLE files ADD COLUMN presence_error TEXT NOT NULL DEFAULT '';
ALTER TABLE files ADD COLUMN checked_at TEXT NOT NULL DEFAULT '';
ALTER TABLE queue_items ADD COLUMN retry_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE queue_items ADD COLUMN retry_at TEXT NOT NULL DEFAULT '';
DROP INDEX idx_active_offer;
CREATE UNIQUE INDEX idx_active_offer ON queue_items(provider_id,release_key)
 WHERE release_key<>'' AND status IN('queued','downloading','importing','needs_review','retry_wait','dispatch_unknown');
ALTER TABLE source_bindings ADD COLUMN trust_chapters BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE collections DROP COLUMN monitored;
ALTER TABLE collections DROP COLUMN cutoff_num;

-- Independent publication traits; an empty list means unknown, not standard.
ALTER TABLE collections ADD COLUMN edition_traits TEXT NOT NULL DEFAULT '[]';
UPDATE collections SET edition_traits='[' || RTRIM(
 CASE WHEN edition_layout='boxset' OR LOWER(edition_name) LIKE '%box%' THEN '"boxset",' ELSE '' END ||
 CASE WHEN edition_layout='deluxe' OR LOWER(edition_name) LIKE '%deluxe%' THEN '"deluxe",' ELSE '' END ||
 CASE WHEN edition_layout='omnibus' OR LOWER(edition_name) LIKE '%omnibus%' THEN '"omnibus",' ELSE '' END ||
 CASE WHEN (edition_layout='standard' OR LOWER(edition_name) LIKE '%standard%') AND edition_layout<>'omnibus' AND LOWER(edition_name) NOT LIKE '%omnibus%' THEN '"standard",' ELSE '' END, ',') || ']';
UPDATE collections SET overrides_json=REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(overrides_json,
 '"edition_layout":"omnibus"','"edition_traits":["omnibus"]'),
 '"edition_layout":"deluxe"','"edition_traits":["deluxe"]'),
 '"edition_layout":"boxset"','"edition_traits":["boxset"]'),
 '"edition_layout":"standard"','"edition_traits":["standard"]'),
 '"edition_layout":"custom"','"edition_traits":[]');
UPDATE collections SET content_variant='unknown'
 WHERE content_variant='original' AND LOWER(edition_name) NOT LIKE '%original%' AND overrides_json NOT LIKE '%"content_variant"%';
ALTER TABLE collections DROP COLUMN edition_layout;

CREATE INDEX idx_asset_contents_work ON asset_contents(work_id);
