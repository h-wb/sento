-- Indexer entries (0009) replaced the global site_settings table; the
-- one-time conversion code is gone, so drop the table and its marker.
-- Per-series overrides live on in series_site_settings.

DROP TABLE IF EXISTS site_settings;
DELETE FROM settings WHERE key = 'indexers_migrated';
