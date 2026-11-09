-- The parser revision that derived a file's filename-sourced facts. A scan
-- re-reads the name with the current parser when this is older (parse.Version).
ALTER TABLE files ADD COLUMN parser_version INTEGER NOT NULL DEFAULT 0;
