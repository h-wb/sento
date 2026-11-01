-- How many releases in a row a source listing could not give (gone,
-- unreadable). Three switch the listing off; a release that imports
-- resets it.
ALTER TABLE source_bindings ADD COLUMN dead_streak INTEGER NOT NULL DEFAULT 0;
