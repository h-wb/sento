-- Source bindings move to opaque manga keys: the source host mints the
-- identifier (Suwayomi uses database ids, a stateless host uses source
-- URLs) and Sento never interprets it. Existing integer ids survive as
-- their decimal string form, which is exactly the key the Suwayomi host
-- mints for them.
CREATE TABLE source_bindings_new (
  id INTEGER PRIMARY KEY,
  series_id INTEGER NOT NULL REFERENCES series(id) ON DELETE CASCADE,
  source_id TEXT NOT NULL,
  source_manga_key TEXT NOT NULL,
  source_name TEXT,
  manga_title TEXT,
  state TEXT NOT NULL DEFAULT 'active',
  priority INTEGER NOT NULL DEFAULT 0,
  bound_at TEXT NOT NULL DEFAULT (datetime('now')),
  last_checked_at TEXT,
  UNIQUE(series_id, source_id, source_manga_key)
);

INSERT INTO source_bindings_new
  (id, series_id, source_id, source_manga_key, source_name, manga_title, state, priority, bound_at, last_checked_at)
SELECT id, series_id, source_id, CAST(source_manga_id AS TEXT), source_name, manga_title, state, priority, bound_at, last_checked_at
FROM source_bindings;

DROP TABLE source_bindings;
ALTER TABLE source_bindings_new RENAME TO source_bindings;
