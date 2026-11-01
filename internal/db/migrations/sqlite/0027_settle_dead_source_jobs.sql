-- Source downloads that waited for review because the source no longer has
-- the release (404, 410) or serves an archive it cannot read, from before
-- such failures were settled automatically: they fail, and their release
-- is blocklisted, as a failure would now be. A search then looks for
-- another release; nothing about them was a person's to decide.
INSERT INTO blocklist (protocol, key, reason)
SELECT DISTINCT protocol, key, 'the source cannot give this release'
FROM (
  SELECT protocol, external_id AS key, error FROM queue_items
  WHERE status = 'needs_review' AND protocol = 'source' AND external_id <> ''
  UNION ALL
  SELECT protocol, release_key AS key, error FROM queue_items
  WHERE status = 'needs_review' AND protocol = 'source' AND release_key <> ''
) dead
WHERE error LIKE '%HTTP error 404%' OR error LIKE '%HTTP error 410%' OR error LIKE '%Not a local file header%'
ON CONFLICT (protocol, key) DO NOTHING;

UPDATE queue_items SET status = 'failed'
WHERE status = 'needs_review' AND protocol = 'source'
  AND (error LIKE '%HTTP error 404%' OR error LIKE '%HTTP error 410%' OR error LIKE '%Not a local file header%');
