-- Per-collection release profile override: collection > series > instance
-- default. NULL = inherit from the series.
ALTER TABLE collections ADD COLUMN profile_id INTEGER REFERENCES profiles(id);
