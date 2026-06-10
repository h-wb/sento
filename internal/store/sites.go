package store

import (
	"context"
	"errors"
)

// SiteSetting is the global configuration of one search site.
type SiteSetting struct {
	Provider    string `json:"provider"`
	SiteID      string `json:"siteId"`
	Enabled     bool   `json:"enabled"`
	UseVolumes  bool   `json:"useVolumes"`
	UseChapters bool   `json:"useChapters"`
	Priority    int    `json:"priority"`
}

// SiteOverride is a per-series override; nil fields inherit the global.
type SiteOverride struct {
	SeriesID    int64  `json:"seriesId"`
	Provider    string `json:"provider"`
	SiteID      string `json:"siteId"`
	Enabled     *bool  `json:"enabled"`
	UseVolumes  *bool  `json:"useVolumes"`
	UseChapters *bool  `json:"useChapters"`
}

func defaultSiteSetting(provider, siteID string) SiteSetting {
	return SiteSetting{
		Provider: provider, SiteID: siteID,
		Enabled: true, UseVolumes: true, UseChapters: true,
	}
}

// ListSiteSettings returns all stored global settings keyed provider/site.
func (s *Store) ListSiteSettings(ctx context.Context) (map[[2]string]SiteSetting, error) {
	settings, err := list[SiteSetting](ctx, s, `
		SELECT provider, site_id, enabled, use_volumes, use_chapters, priority
		FROM site_settings`)
	if err != nil {
		return nil, err
	}
	out := map[[2]string]SiteSetting{}
	for _, st := range settings {
		out[[2]string{st.Provider, st.SiteID}] = st
	}
	return out, nil
}

// GlobalSiteSetting returns the stored setting or the permissive default.
func (s *Store) GlobalSiteSetting(ctx context.Context, provider, siteID string) (SiteSetting, error) {
	st, err := get[SiteSetting](ctx, s, `
		SELECT provider, site_id, enabled, use_volumes, use_chapters, priority
		FROM site_settings WHERE provider = ? AND site_id = ?`, provider, siteID)
	if errors.Is(err, ErrNotFound) {
		return defaultSiteSetting(provider, siteID), nil
	}
	if err != nil {
		return SiteSetting{}, err
	}
	return *st, nil
}

func (s *Store) UpsertSiteSetting(ctx context.Context, st SiteSetting) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO site_settings (provider, site_id, enabled, use_volumes, use_chapters, priority)
		VALUES (?,?,?,?,?,?)
		ON CONFLICT(provider, site_id) DO UPDATE SET
			enabled = excluded.enabled,
			use_volumes = excluded.use_volumes,
			use_chapters = excluded.use_chapters,
			priority = excluded.priority`,
		st.Provider, st.SiteID, st.Enabled, st.UseVolumes, st.UseChapters, st.Priority)
	return err
}

// ListSiteOverrides returns the per-series overrides keyed provider/site.
func (s *Store) ListSiteOverrides(ctx context.Context, seriesID int64) (map[[2]string]SiteOverride, error) {
	overrides, err := list[SiteOverride](ctx, s, `
		SELECT series_id, provider, site_id, enabled, use_volumes, use_chapters
		FROM series_site_settings WHERE series_id = ?`, seriesID)
	if err != nil {
		return nil, err
	}
	out := map[[2]string]SiteOverride{}
	for _, ov := range overrides {
		out[[2]string{ov.Provider, ov.SiteID}] = ov
	}
	return out, nil
}

// UpsertSiteOverride stores an override; all-nil fields delete it (inherit).
func (s *Store) UpsertSiteOverride(ctx context.Context, ov SiteOverride) error {
	if ov.Enabled == nil && ov.UseVolumes == nil && ov.UseChapters == nil {
		_, err := s.db.ExecContext(ctx, `
			DELETE FROM series_site_settings
			WHERE series_id = ? AND provider = ? AND site_id = ?`,
			ov.SeriesID, ov.Provider, ov.SiteID)
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO series_site_settings (series_id, provider, site_id, enabled, use_volumes, use_chapters)
		VALUES (?,?,?,?,?,?)
		ON CONFLICT(series_id, provider, site_id) DO UPDATE SET
			enabled = excluded.enabled,
			use_volumes = excluded.use_volumes,
			use_chapters = excluded.use_chapters`,
		ov.SeriesID, ov.Provider, ov.SiteID,
		nullableBool(ov.Enabled), nullableBool(ov.UseVolumes), nullableBool(ov.UseChapters))
	return err
}

func nullableBool(b *bool) any {
	if b == nil {
		return nil
	}
	return *b
}

// EffectiveSiteSetting overlays a series override on the global setting.
func EffectiveSiteSetting(global SiteSetting, ov *SiteOverride) SiteSetting {
	out := global
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
