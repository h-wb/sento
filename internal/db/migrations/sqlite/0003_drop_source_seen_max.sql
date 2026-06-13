-- D7 reversed (D15): the chapter ledger derives from MangaBaka metadata
-- only; source data never feeds slot generation.
ALTER TABLE chapter_ledgers DROP COLUMN source_seen_max;
