-- An IO-wrapped range/header failure did not establish a damaged archive.
-- Remove automatic blocks from that classification, including the generic
-- reason written by 0027. Retain manual blocks and evidence of a gone release.
-- Failed jobs remain history; normal planning decides whether to try again.
DELETE FROM blocklist
WHERE protocol = 'source'
  AND (
    reason LIKE '%Not a local file header%'
    OR (
      reason = 'the source cannot give this release'
      AND key IN (
        SELECT external_id FROM queue_items
        WHERE protocol = 'source' AND error LIKE '%Not a local file header%'
        UNION
        SELECT release_key FROM queue_items
        WHERE protocol = 'source' AND error LIKE '%Not a local file header%'
      )
    )
  )
  AND key NOT IN (
    SELECT external_id FROM queue_items
    WHERE protocol = 'source' AND (error LIKE '%HTTP error 404%' OR error LIKE '%HTTP error 410%')
    UNION
    SELECT release_key FROM queue_items
    WHERE protocol = 'source' AND (error LIKE '%HTTP error 404%' OR error LIKE '%HTTP error 410%')
  );
