-- Block identities must outlive the bounded queue history. Keep the full
-- protocol-scoped alias set and display title with each active block.
ALTER TABLE blocklist ADD COLUMN aliases_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE blocklist ADD COLUMN title TEXT NOT NULL DEFAULT '';

-- Recover existing relationships while the original jobs are still present,
-- including aliases connected through multiple attempts of the same release.
WITH RECURSIVE
snapshots AS (
  SELECT id, protocol, title,
    CASE WHEN json_valid(snapshot_json) THEN snapshot_json ELSE '{}' END AS snapshot
  FROM queue_items
),
job_keys AS (
  SELECT id, protocol, title, external_id AS key FROM queue_items WHERE external_id <> ''
  UNION
  SELECT id, protocol, title, release_key AS key FROM queue_items WHERE release_key <> ''
  UNION
  SELECT id, protocol, title, json_extract(snapshot, '$.guid') AS key FROM snapshots
  WHERE json_type(snapshot, '$.guid') = 'text' AND json_extract(snapshot, '$.guid') <> ''
),
edges AS (
  SELECT a.protocol, a.key, b.key AS alias FROM job_keys a JOIN job_keys b ON a.id = b.id
),
reachable(protocol, original_key, key) AS (
  SELECT protocol, key, key FROM blocklist
  UNION
  SELECT r.protocol, r.original_key, e.alias
  FROM reachable r JOIN edges e ON e.protocol = r.protocol AND e.key = r.key
)
UPDATE blocklist AS b SET
  aliases_json = (SELECT json_group_array(key) FROM reachable r WHERE r.protocol = b.protocol AND r.original_key = b.key),
  title = COALESCE((SELECT title FROM job_keys j WHERE j.protocol = b.protocol AND j.key = b.key ORDER BY id DESC LIMIT 1), '');
