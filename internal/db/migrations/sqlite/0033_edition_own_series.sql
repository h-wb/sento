-- An edition marked as its own series in readers: its ComicInfo Series
-- names the edition beside the title, so a reader that groups by metadata
-- keeps it apart from its siblings.
ALTER TABLE editions ADD COLUMN own_series INTEGER NOT NULL DEFAULT FALSE;
