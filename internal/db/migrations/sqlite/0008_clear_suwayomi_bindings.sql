-- The Suwayomi backend is gone: existing bindings carry its database-id
-- manga keys, which the sento-host sidecar cannot interpret. Clearing
-- them lets auto-binding re-resolve every series against the host on the
-- next search (bindings are a cache of ≥0.90 title matches, rebuilt
-- automatically).
DELETE FROM source_bindings;
