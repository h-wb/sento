-- A release profile is chosen where downloads are configured: on a series
-- or on a copy of an edition (Download settings), no longer on an edition.
-- Copies that inherited an edition's profile keep it, set on themselves.
UPDATE versions SET profile_id = (SELECT e.profile_id FROM editions e WHERE e.id = versions.edition_id)
  WHERE profile_id IS NULL
    AND (SELECT e.profile_id FROM editions e WHERE e.id = versions.edition_id) IS NOT NULL;
UPDATE editions SET profile_id = NULL WHERE profile_id IS NOT NULL;
