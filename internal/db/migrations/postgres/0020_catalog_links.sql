-- A linked edition is a catalog edition, whoever created it: "custom" now
-- means exactly "not backed by the catalog".
UPDATE collections SET custom = FALSE WHERE custom = TRUE AND sync_state = 'linked';

-- Linking used to keep the unused catalog row as 'superseded'. It is merged
-- into the linked edition instead, so the leftovers go.
DELETE FROM external_refs WHERE
  (kind = 'collection' AND local_id IN (SELECT id FROM collections WHERE sync_state = 'superseded'))
  OR (kind = 'work' AND local_id IN (
    SELECT w.id FROM works w JOIN collections c ON c.id = w.collection_id WHERE c.sync_state = 'superseded'));
DELETE FROM collections WHERE sync_state = 'superseded';

-- Catalog editions a person said are not the same as their local edition,
-- so the suggestion to link them is not made again.
CREATE TABLE edition_link_declines (
  collection_id BIGINT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
  external_id TEXT NOT NULL,
  PRIMARY KEY (collection_id, external_id)
);
