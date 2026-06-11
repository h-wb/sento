package store

import (
	"context"
	"strconv"
)

// SiteOverride is a per-series override of one indexer entry; nil fields
// inherit the entry's global setting. Rows live in series_site_settings
// with provider='indexer' and site_id = the indexer id.
type SiteOverride struct {
	SeriesID    int64 `json:"seriesId"`
	IndexerID   int64 `json:"indexerId"`
	Enabled     *bool `json:"enabled"`
	UseVolumes  *bool `json:"useVolumes"`
	UseChapters *bool `json:"useChapters"`
}

// ListSiteOverrides returns the per-series overrides keyed by indexer id.
func (s *Store) ListSiteOverrides(ctx context.Context, seriesID int64) (map[int64]SiteOverride, error) {
	type row struct {
		SeriesID    int64
		SiteID      string
		Enabled     *bool
		UseVolumes  *bool
		UseChapters *bool
	}
	rows, err := list[row](ctx, s, `
		SELECT series_id, site_id, enabled, use_volumes, use_chapters
		FROM series_site_settings WHERE series_id = ? AND provider = 'indexer'`, seriesID)
	if err != nil {
		return nil, err
	}
	out := map[int64]SiteOverride{}
	for _, r := range rows {
		id, err := strconv.ParseInt(r.SiteID, 10, 64)
		if err != nil {
			continue
		}
		out[id] = SiteOverride{
			SeriesID: r.SeriesID, IndexerID: id,
			Enabled: r.Enabled, UseVolumes: r.UseVolumes, UseChapters: r.UseChapters,
		}
	}
	return out, nil
}

// UpsertSiteOverride stores an override; all-nil fields delete it (inherit).
func (s *Store) UpsertSiteOverride(ctx context.Context, ov SiteOverride) error {
	if ov.Enabled == nil && ov.UseVolumes == nil && ov.UseChapters == nil {
		_, err := s.db.ExecContext(ctx, `
			DELETE FROM series_site_settings
			WHERE series_id = ? AND provider = 'indexer' AND site_id = ?`,
			ov.SeriesID, ov.IndexerID)
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO series_site_settings (series_id, provider, site_id, enabled, use_volumes, use_chapters)
		VALUES (?,'indexer',?,?,?,?)
		ON CONFLICT(series_id, provider, site_id) DO UPDATE SET
			enabled = excluded.enabled,
			use_volumes = excluded.use_volumes,
			use_chapters = excluded.use_chapters`,
		ov.SeriesID, ov.IndexerID,
		nullableBool(ov.Enabled), nullableBool(ov.UseVolumes), nullableBool(ov.UseChapters))
	return err
}

func nullableBool(b *bool) any {
	if b == nil {
		return nil
	}
	return *b
}

// EffectiveIndexer overlays a series override on an indexer entry.
func EffectiveIndexer(ix Indexer, ov *SiteOverride) Indexer {
	out := ix
	if ov == nil {
		return out
	}
	if ov.Enabled != nil {
		out.Enabled = *ov.Enabled
	}
	if ov.UseVolumes != nil {
		out.UseVolumes = *ov.UseVolumes
	}
	if ov.UseChapters != nil {
		out.UseChapters = *ov.UseChapters
	}
	return out
}
