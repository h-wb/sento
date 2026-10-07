// Package service orchestrates providers and the store (use-cases behind
// the HTTP handlers).
package service

import (
	"context"
	"strings"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/metadata"
	"github.com/h-wb/sento/internal/store"
)

type Library struct {
	store    *store.Store
	provider metadata.Provider
}

func NewLibrary(st *store.Store, provider metadata.Provider) *Library {
	return &Library{store: st, provider: provider}
}

type AddSeriesRequest struct {
	MangaBakaID int64 `json:"mangabakaId"`
	// Monitor is the edition-monitoring mode: "" or "default" monitors the
	// default edition (see defaultEditionID), "all" every volume edition,
	// "none" nothing. MonitorIDs overrides it with explicit picks.
	Monitor       string   `json:"monitor"`
	MonitorIDs    []string `json:"monitorIds"`    // explicit provider collection ids
	ChapterLang   string   `json:"chapterLang"`   // default "en"
	ChapterCutoff int64    `json:"chapterCutoff"` // hundredths; 0 = latest at add (D6)
	ProfileID     int64    `json:"profileId"`     // 0 = instance default
}

func (l *Library) AddSeries(ctx context.Context, req AddSeriesRequest) (int64, error) {
	if req.MangaBakaID == 0 {
		return 0, apperr.Invalidf("mangabakaId is required")
	}
	series, err := l.provider.GetSeries(ctx, req.MangaBakaID)
	if err != nil {
		return 0, apperr.Upstreamf("fetching series %d: %w", req.MangaBakaID, err)
	}
	collections, err := l.provider.GetCollections(ctx, series.MangaBakaID)
	if err != nil {
		return 0, apperr.Upstreamf("fetching collections: %w", err)
	}
	works, err := l.provider.GetWorks(ctx, series.MangaBakaID)
	if err != nil {
		return 0, apperr.Upstreamf("fetching works: %w", err)
	}

	monitorAll := req.Monitor == "all"
	monitorIDs := req.MonitorIDs
	if !monitorAll && len(monitorIDs) == 0 && req.Monitor != "none" {
		if id := defaultEditionID(collections, req.ChapterLang); id != "" {
			monitorIDs = []string{id}
		}
	}

	id, err := l.store.AddSeries(ctx, store.AddSeriesParams{
		MonitorNone:   req.Monitor == "none",
		Series:        series,
		Collections:   collections,
		Works:         works,
		MonitorAll:    monitorAll,
		MonitorIDs:    monitorIDs,
		ChapterLang:   req.ChapterLang,
		ChapterCutoff: req.ChapterCutoff,
		ProfileID:     req.ProfileID,
	})
	if err != nil {
		return 0, err
	}
	return id, l.store.EnsureModels(ctx, id)
}

// defaultEditionID picks the edition monitored when the caller doesn't
// choose: MangaBaka has no default flag, so prefer a "Standard Edition"
// and the preferred language, falling back to the first volume edition.
func defaultEditionID(collections []metadata.Collection, lang string) string {
	if lang == "" {
		lang = "en"
	}
	best, bestScore := "", -1
	for _, c := range collections {
		if c.Type != "volume" {
			continue
		}
		score := 0
		if strings.Contains(strings.ToLower(c.EditionName), "standard") {
			score += 1
		}
		if strings.EqualFold(c.Language, lang) {
			score += 4
		}
		if score > bestScore {
			best, bestScore = c.ProviderID, score
		}
	}
	return best
}

// Refresh reconciles provider references and local overrides, preserving stable local IDs.
func (l *Library) Refresh(ctx context.Context, seriesID int64) error {
	existing, err := l.store.GetSeries(ctx, seriesID)
	if err != nil {
		return err
	}
	series, err := l.provider.GetSeries(ctx, existing.MangaBakaID)
	if err != nil {
		return apperr.Upstreamf("fetching series %d: %w", existing.MangaBakaID, err)
	}
	collections, err := l.provider.GetCollections(ctx, series.MangaBakaID)
	if err != nil {
		return apperr.Upstreamf("fetching collections: %w", err)
	}
	works, err := l.provider.GetWorks(ctx, series.MangaBakaID)
	if err != nil {
		return apperr.Upstreamf("fetching works: %w", err)
	}
	return l.store.ReconcileMetadata(ctx, seriesID, series, collections, works)
}

// CreateCollectionParams describes a user-defined collection: volume
// editions clone numbering from BasedOn or generate 1..Volumes; chapter
// editions start with explicit chapters unless count inference is selected.
type CreateCollectionParams struct {
	EditionName string
	Language    string
	Publisher   string
	Medium      string
	Type        string // "" = volume
	BasedOn     int64
	Volumes     int
	CutoffNum   int64
	Monitored   bool
}

// CreateCollection validates and creates a custom collection.
func (l *Library) CreateCollection(ctx context.Context, seriesID int64, p CreateCollectionParams) (*store.Collection, error) {
	if strings.TrimSpace(p.EditionName) == "" || strings.TrimSpace(p.Language) == "" {
		return nil, apperr.Invalidf("editionName and language are required")
	}
	ctype := p.Type
	if ctype == "" {
		ctype = "volume"
	}
	if ctype != "volume" && ctype != "chapter" {
		return nil, apperr.Invalidf("type must be volume or chapter")
	}
	if ctype == "volume" && p.BasedOn == 0 && (p.Volumes < 0 || p.Volumes > 500) {
		return nil, apperr.Invalidf("volumes must be 0-500 when not based on an existing edition")
	}
	cid, err := l.store.AddCustomCollection(ctx, store.Collection{
		SeriesID: seriesID, EditionName: p.EditionName, Language: p.Language,
		Publisher: p.Publisher, Medium: p.Medium, Type: ctype,
		Monitored: p.Monitored, CutoffNum: p.CutoffNum,
	}, p.BasedOn, p.Volumes)
	if err != nil {
		return nil, err
	}
	if err := l.store.EnsureModels(ctx, seriesID); err != nil {
		return nil, err
	}
	return l.store.GetCollection(ctx, cid)
}

// CollectionUpdate is a partial collection patch (nil = keep).
type CollectionUpdate struct {
	Monitored *bool
	ProfileID *int64
	CutoffNum *int64
	Language  *string
}

// UpdateCollection applies a partial update under the field rules: the
// cutoff exists on chapter collections only, language identity is only
// editable on custom collections (MangaBaka rows are reference data).
func (l *Library) UpdateCollection(ctx context.Context, seriesID, collectionID int64, u CollectionUpdate) error {
	if u.Monitored != nil {
		if err := l.store.SetCollectionMonitored(ctx, seriesID, collectionID, *u.Monitored); err != nil {
			return err
		}
	}
	if u.ProfileID != nil {
		if err := l.store.SetCollectionProfile(ctx, seriesID, collectionID, *u.ProfileID); err != nil {
			return err
		}
	}
	if u.CutoffNum == nil && u.Language == nil {
		return nil
	}
	col, err := l.store.GetCollection(ctx, collectionID)
	if err != nil {
		return err
	}
	if col.SeriesID != seriesID {
		return store.ErrNotFound
	}
	if u.CutoffNum != nil {
		if col.Type != "chapter" {
			return apperr.Invalidf("cutoff applies to chapter collections only")
		}
		if err := l.store.SetCollectionCutoff(ctx, seriesID, collectionID, *u.CutoffNum); err != nil {
			return err
		}
	}
	if u.Language != nil {
		if !col.Custom {
			return apperr.Invalidf("language can only be changed on custom collections")
		}
		if err := l.store.EditCollection(ctx, seriesID, collectionID, store.CollectionMetadataPatch{Language: u.Language}); err != nil {
			return err
		}
	}
	return nil
}
