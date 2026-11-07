-- The stat signature (device, inode, size, mtime, ctime) of the archive whose
-- fingerprint is recorded in hash. A live archive with the same signature need
-- not be fingerprinted again; empty means it must be.
ALTER TABLE files ADD COLUMN hash_stat TEXT NOT NULL DEFAULT '';
