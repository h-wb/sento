-- Chapters become collections (ctype='chapter'): the chapter_ledgers
-- singleton is promoted to a real collection row so chapter slots get the
-- same identity (language, edition descriptors), monitoring, profile
-- override and routing machinery as volume editions. The cutoff ("monitor
-- from N onward") moves onto the collection.

ALTER TABLE collections ADD COLUMN cutoff_num INTEGER NOT NULL DEFAULT 0;

-- Adopt the ledger state into a MangaBaka-provided chapter collection when
-- one exists (the lowest id wins) …
UPDATE collections SET
  monitored = (SELECT cl.monitored FROM chapter_ledgers cl WHERE cl.series_id = collections.series_id),
  language = (SELECT cl.language FROM chapter_ledgers cl WHERE cl.series_id = collections.series_id),
  cutoff_num = (SELECT cl.cutoff_num FROM chapter_ledgers cl WHERE cl.series_id = collections.series_id)
WHERE ctype = 'chapter'
  AND id = (SELECT MIN(c2.id) FROM collections c2
            WHERE c2.series_id = collections.series_id AND c2.ctype = 'chapter')
  AND EXISTS (SELECT 1 FROM chapter_ledgers cl WHERE cl.series_id = collections.series_id);

-- … otherwise create the default chapter collection (custom: user-managed,
-- deletable, survives metadata refresh).
INSERT INTO collections (series_id, edition_name, language, publisher, medium,
  ctype, monitored, custom, cutoff_num, updated_at)
SELECT cl.series_id, 'Chapters', cl.language, '', '', 'chapter', cl.monitored, 1,
  cl.cutoff_num, datetime('now')
FROM chapter_ledgers cl
WHERE NOT EXISTS (SELECT 1 FROM collections c
                  WHERE c.series_id = cl.series_id AND c.ctype = 'chapter');

-- Re-key chapter slot links/overrides from the 0 sentinel to the series'
-- chapter collection.
UPDATE file_slot_links SET collection_id = (
  SELECT MIN(c.id) FROM collections c
  WHERE c.series_id = (SELECT f.series_id FROM files f WHERE f.id = file_slot_links.file_id)
    AND c.ctype = 'chapter')
WHERE ledger = 'chapter' AND collection_id = 0;

UPDATE slot_overrides SET collection_id = (
  SELECT MIN(c.id) FROM collections c
  WHERE c.series_id = slot_overrides.series_id AND c.ctype = 'chapter')
WHERE ledger = 'chapter' AND collection_id = 0;

DROP TABLE chapter_ledgers;
