-- Custom formats (decision-engine.md §5): named condition lists matched
-- against release candidates; profiles assign them scores. The profiles
-- table itself shipped in v1 (body_json) and starts being used now.
CREATE TABLE custom_formats (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  conditions_json TEXT NOT NULL DEFAULT '[]'
);
