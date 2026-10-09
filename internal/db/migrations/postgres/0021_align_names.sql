-- The schema takes the names the code and the UI use: an edition has books
-- and collected versions. No data changes shape; ids are untouched.
ALTER TABLE collections RENAME TO editions;
ALTER TABLE works RENAME TO books;
ALTER TABLE download_targets RENAME TO versions;
ALTER TABLE unit_reservations RENAME TO book_reservations;
ALTER TABLE unit_overrides RENAME TO book_overrides;
ALTER TABLE unit_observations RENAME TO book_observations;

ALTER TABLE editions RENAME COLUMN mb_collection_id TO provider_id;
ALTER TABLE editions RENAME COLUMN ctype TO type;
ALTER TABLE books RENAME COLUMN collection_id TO edition_id;
ALTER TABLE books RENAME COLUMN mb_work_id TO provider_id;
ALTER TABLE versions RENAME COLUMN collection_id TO edition_id;
ALTER TABLE book_observations RENAME COLUMN collection_id TO edition_id;
ALTER TABLE source_bindings RENAME COLUMN collection_id TO edition_id;
ALTER TABLE edition_link_declines RENAME COLUMN collection_id TO edition_id;
ALTER TABLE asset_coverage RENAME COLUMN target_id TO version_id;
ALTER TABLE asset_coverage RENAME COLUMN work_id TO book_id;
ALTER TABLE asset_contents RENAME COLUMN work_id TO book_id;
ALTER TABLE book_reservations RENAME COLUMN target_id TO version_id;
ALTER TABLE book_reservations RENAME COLUMN work_id TO book_id;
ALTER TABLE book_overrides RENAME COLUMN target_id TO version_id;
ALTER TABLE book_overrides RENAME COLUMN work_id TO book_id;
ALTER TABLE queue_items RENAME COLUMN expected_slots_json TO books_json;

DROP INDEX idx_collection_ref;
DROP INDEX idx_work_ref;
DROP INDEX idx_work_number;
DROP INDEX idx_default_target;
DROP INDEX idx_asset_contents_work;
CREATE INDEX idx_edition_ref ON editions(series_id, provider_id);
CREATE INDEX idx_book_ref ON books(edition_id, provider_id);
CREATE INDEX idx_book_number ON books(edition_id, sequence_num);
CREATE UNIQUE INDEX idx_default_version ON versions(edition_id) WHERE is_default = TRUE;
CREATE INDEX idx_asset_contents_book ON asset_contents(book_id);

UPDATE external_refs SET kind = 'edition' WHERE kind = 'collection';
UPDATE external_refs SET kind = 'book' WHERE kind = 'work';

-- Stored book references used the old key names.
UPDATE queue_items SET
  books_json = REPLACE(REPLACE(REPLACE(books_json, '"workId"', '"bookId"'), '"targetId"', '"versionId"'), '"collectionId"', '"editionId"'),
  snapshot_json = REPLACE(REPLACE(REPLACE(snapshot_json, '"workId"', '"bookId"'), '"targetId"', '"versionId"'), '"collectionId"', '"editionId"');
UPDATE decisions SET
  body_json = REPLACE(REPLACE(REPLACE(body_json, '"workId"', '"bookId"'), '"targetId"', '"versionId"'), '"collectionId"', '"editionId"');
