-- Chapters can live under their own top folder (Chapters/<series>), apart
-- from the series' volumes. Empty = same folder as path.
ALTER TABLE series ADD COLUMN chapter_path TEXT;
