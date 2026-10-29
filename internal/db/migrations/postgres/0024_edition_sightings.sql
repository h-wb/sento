-- Releases that state an edition the series does not have ("Deluxe
-- Edition (2012)" where only a box set and a standard edition exist):
-- evidence for review to propose adding it (propose-dont-ask.md §A3).
-- One row per kind of edition and language; dismissing it is remembered.
CREATE TABLE edition_sightings (
 series_id BIGINT NOT NULL REFERENCES series(id) ON DELETE CASCADE,
 facet TEXT NOT NULL,
 language TEXT NOT NULL,
 releases INTEGER NOT NULL DEFAULT 0,
 example TEXT NOT NULL DEFAULT '',
 seen_at TEXT NOT NULL,
 dismissed BOOLEAN NOT NULL DEFAULT FALSE,
 PRIMARY KEY(series_id, facet, language)
);
