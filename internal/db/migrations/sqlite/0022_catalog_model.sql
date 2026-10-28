-- The catalog is mirrored as MangaBaka has it, under its own names: a
-- collection is one product line of a series (language, publisher, edition,
-- medium) and a work is one book of a collection. An edition is what is
-- collected: MangaBaka's edition for one series, language and type. It
-- groups the collections that differ only in medium or publisher, and a
-- book is the works of those collections that carry the same number.

CREATE TABLE collections (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  edition_id BIGINT NOT NULL REFERENCES editions(id) ON DELETE CASCADE,
  provider_id TEXT NOT NULL,
  publisher TEXT NOT NULL DEFAULT '',
  medium TEXT NOT NULL DEFAULT '',
  format TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT '',
  related_collection_id TEXT NOT NULL DEFAULT '',
  count_main INTEGER, count_extra INTEGER, count_other INTEGER,
  start_date TEXT, end_date TEXT, raw_json TEXT, updated_at TEXT,
  sync_state TEXT NOT NULL DEFAULT 'linked'
);
CREATE INDEX idx_collection_edition ON collections(edition_id);
CREATE INDEX idx_collection_ref ON collections(provider_id);

INSERT INTO collections (edition_id, provider_id, publisher, medium, count_main, count_extra, count_other,
    start_date, end_date, raw_json, updated_at, sync_state)
  SELECT id, provider_id, publisher, medium, count_main, count_extra, count_other,
    start_date, end_date, raw_json, updated_at, CASE WHEN sync_state = 'stale' THEN 'stale' ELSE 'linked' END
  FROM editions WHERE custom = FALSE AND COALESCE(provider_id, '') <> '';

CREATE TABLE works (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  collection_id BIGINT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
  book_id BIGINT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
  provider_id TEXT NOT NULL,
  sequence_num BIGINT NOT NULL DEFAULT 0,
  sequence_string TEXT NOT NULL DEFAULT '',
  number_known INTEGER NOT NULL DEFAULT TRUE,
  count_type TEXT NOT NULL DEFAULT 'main',
  release_date TEXT NOT NULL DEFAULT '',
  isbn TEXT NOT NULL DEFAULT '',
  pages INTEGER NOT NULL DEFAULT 0,
  sub_title TEXT NOT NULL DEFAULT '',
  revision TEXT NOT NULL DEFAULT '',
  raw_json TEXT,
  sync_state TEXT NOT NULL DEFAULT 'linked'
);
CREATE INDEX idx_work_collection ON works(collection_id, provider_id);
CREATE INDEX idx_work_book ON works(book_id);

INSERT INTO works (collection_id, book_id, provider_id, sequence_num, sequence_string, number_known, count_type,
    release_date, isbn, pages, revision, raw_json, sync_state)
  SELECT c.id, b.id, b.provider_id, b.sequence_num, COALESCE(b.sequence_string, ''), b.number_known, b.count_type,
    COALESCE(b.release_date, ''), COALESCE(b.isbn, ''), COALESCE(b.pages, 0), b.revision, b.raw_json,
    CASE WHEN b.sync_state = 'stale' THEN 'stale' ELSE 'linked' END
  FROM books b JOIN collections c ON c.edition_id = b.edition_id
  WHERE COALESCE(b.provider_id, '') <> '';

-- The trait list goes: what an edition is, is what its name says, in the
-- same words release names use. A trait someone set by hand that the name
-- does not carry moves into the name, as an edit that survives refreshes.
UPDATE editions SET edition_name = 'Omnibus ' || edition_name,
    overrides_json = json_set(overrides_json, '$.edition_name', 'Omnibus ' || edition_name)
  WHERE edition_traits LIKE '%"omnibus"%' AND lower(edition_name) NOT LIKE '%omnibus%' AND lower(edition_name) NOT LIKE '%in-1%';
UPDATE editions SET edition_name = 'Deluxe ' || edition_name,
    overrides_json = json_set(overrides_json, '$.edition_name', 'Deluxe ' || edition_name)
  WHERE edition_traits LIKE '%"deluxe"%' AND lower(edition_name) NOT LIKE '%deluxe%';
UPDATE editions SET edition_name = 'Colored ' || edition_name,
    overrides_json = json_set(overrides_json, '$.edition_name', 'Colored ' || edition_name)
  WHERE content_variant = 'colored' AND lower(edition_name) NOT LIKE '%color%' AND lower(edition_name) NOT LIKE '%colour%';

-- An edition's catalog reference is now MangaBaka's edition id; the
-- collection ids moved to the collections table. The next refresh fills it.
DROP INDEX idx_edition_ref;
ALTER TABLE editions RENAME COLUMN provider_id TO provider_edition_id;
UPDATE editions SET provider_edition_id = '';
CREATE INDEX idx_edition_ref ON editions(series_id, provider_edition_id);
ALTER TABLE editions DROP COLUMN raw_json;
ALTER TABLE editions DROP COLUMN edition_traits;
ALTER TABLE editions DROP COLUMN content_variant;
-- Set when someone separates a collection from its edition, so it is not
-- grouped back.
ALTER TABLE editions ADD COLUMN split INTEGER NOT NULL DEFAULT FALSE;

-- Declined links name the catalog edition itself.
CREATE TABLE edition_link_declines_new (
  edition_id BIGINT NOT NULL REFERENCES editions(id) ON DELETE CASCADE,
  declined_id BIGINT NOT NULL REFERENCES editions(id) ON DELETE CASCADE,
  PRIMARY KEY (edition_id, declined_id)
);
INSERT INTO edition_link_declines_new (edition_id, declined_id)
  SELECT DISTINCT d.edition_id, c.edition_id FROM edition_link_declines d JOIN collections c ON c.provider_id = d.external_id;
DROP TABLE edition_link_declines;
ALTER TABLE edition_link_declines_new RENAME TO edition_link_declines;

-- Collections and works carry the catalog references now.
DROP TABLE external_refs;
