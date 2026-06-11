-- App settings: one JSON document per section (key "media" first).
-- User-tweakable domain config lives in the DB (like site_settings);
-- infrastructure (URLs, credentials, paths) stays in yaml/env.
CREATE TABLE settings (
  key TEXT PRIMARY KEY,
  value_json TEXT NOT NULL,
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
