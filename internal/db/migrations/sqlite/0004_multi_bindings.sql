-- Multi-binding: a series may bind several manga entries per source (sites
-- like Madokami list duplicates; search fans out over all active bindings
-- and the funnel picks per slot). Uniqueness moves to the manga entry, and
-- the bound manga title is stored so the UI can tell entries apart.
CREATE TABLE source_bindings_new (
  id INTEGER PRIMARY KEY,
  series_id INTEGER NOT NULL REFERENCES series(id) ON DELETE CASCADE,
  source_id TEXT NOT NULL,
  source_manga_id INTEGER NOT NULL,
  source_name TEXT,
  manga_title TEXT,
  state TEXT NOT NULL DEFAULT 'active',
  priority INTEGER NOT NULL DEFAULT 0,
  bound_at TEXT NOT NULL DEFAULT (datetime('now')),
  last_checked_at TEXT,
  UNIQUE(series_id, source_id, source_manga_id)
);

INSERT INTO source_bindings_new
  (id, series_id, source_id, source_manga_id, source_name, state, priority, bound_at, last_checked_at)
SELECT id, series_id, source_id, source_manga_id, source_name, state, priority, bound_at, last_checked_at
FROM source_bindings;

DROP TABLE source_bindings;
ALTER TABLE source_bindings_new RENAME TO source_bindings;
