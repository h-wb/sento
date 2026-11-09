-- An edition's own templates for the ComicInfo fields readers file and
-- group it by (series, seriesGroup), keyed by field; a field it does not
-- name follows the media settings. An edition shown as its own series
-- names itself after the title, as own_series did.
ALTER TABLE editions ADD COLUMN reader_fields_json TEXT NOT NULL DEFAULT '{}';
UPDATE editions SET reader_fields_json='{"series":"{Series Title} {edition} {version}"}' WHERE own_series;
ALTER TABLE editions DROP COLUMN own_series;
