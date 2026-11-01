-- A series' history goes with it (store.DeleteSeries). Events a removed
-- series left behind, attached to no series, go too: every event is
-- recorded for a series.
DELETE FROM decisions WHERE series_id IS NULL;
