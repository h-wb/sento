-- The catalog's own name for an edition is kept beside the one shown, so
-- renaming an edition does not change what it is.
ALTER TABLE editions ADD COLUMN provider_edition_name TEXT NOT NULL DEFAULT '';
UPDATE editions SET provider_edition_name = edition_name
  WHERE custom = FALSE AND overrides_json NOT LIKE '%"edition_name"%';

-- Edits to the removed trait fields are no longer edits to anything.
UPDATE editions SET overrides_json = (overrides_json::jsonb - 'edition_traits' - 'content_variant')::text
  WHERE overrides_json LIKE '%"edition_traits"%' OR overrides_json LIKE '%"content_variant"%';
