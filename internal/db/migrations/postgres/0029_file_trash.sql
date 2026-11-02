-- Retain file identity, contents and selections during reversible cleanup.
CREATE TABLE file_trash (
  file_id BIGINT PRIMARY KEY REFERENCES files(id) ON DELETE CASCADE,
  trash_path TEXT NOT NULL UNIQUE,
  digest TEXT NOT NULL,
  trashed_at TEXT NOT NULL,
  purge_after TEXT NOT NULL
);
CREATE INDEX idx_file_trash_expiry ON file_trash(purge_after);
