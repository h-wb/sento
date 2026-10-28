// Package service orchestrates providers and the store (use-cases behind
// the HTTP handlers).
package service

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/metadata"
	"github.com/h-wb/sento/internal/store"
)

type Library struct {
	store    *store.Store
	provider metadata.Provider

	editionsMu sync.Mutex
	editions   []metadata.Edition
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
	MonitorIDs    []string `json:"monitorIds"`    // explicit provider edition ids
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

// CreateEditionParams describes a user-defined edition: volume
// editions clone numbering from BasedOn or generate 1..Volumes; chapter
// editions start with explicit chapters unless count inference is selected.
type CreateEditionParams struct {
	EditionName string
	Language    string
	Publisher   string
	Medium      string
	// ProviderEditionID names the catalog edition this local one stands in
	// for, when it was picked from the catalog's list.
	ProviderEditionID string
	Type              string // "" = volume
	BasedOn           int64
	Volumes           int
	CutoffNum         int64
	Monitored         bool
}

// SeriesPublisher is one publisher of a series as the catalog's series
// record lists it.
type SeriesPublisher struct {
	Name     string `json:"name"`
	Language string `json:"language"`
	Type     string `json:"type,omitempty"` // Original | English | Other
	Note     string `json:"note,omitempty"` // free text: "42 Volumes - Ongoing"
}

// SeriesPublishers reads the publishers out of the stored series record.
func (l *Library) SeriesPublishers(ctx context.Context, seriesID int64) ([]SeriesPublisher, error) {
	raw, _, err := l.store.CatalogPayloads(ctx, seriesID, 0)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Publishers []SeriesPublisher `json:"publishers"`
	}
	_ = json.Unmarshal([]byte(raw), &doc) // an older or empty record lists none
	if doc.Publishers == nil {
		doc.Publishers = []SeriesPublisher{}
	}
	return doc.Publishers, nil
}

// CatalogEditions lists the catalog's edition names, for naming a local
// edition after the catalog edition it stands in for. The list is small
// and changes rarely, so it is fetched once per run.
func (l *Library) CatalogEditions(ctx context.Context) ([]metadata.Edition, error) {
	l.editionsMu.Lock()
	defer l.editionsMu.Unlock()
	if l.editions == nil {
		list, err := l.provider.GetEditions(ctx)
		if err != nil {
			return nil, err
		}
		if list == nil {
			list = []metadata.Edition{}
		}
		l.editions = list
	}
	return l.editions, nil
}

// CreateEdition validates and creates a custom edition.
func (l *Library) CreateEdition(ctx context.Context, seriesID int64, p CreateEditionParams) (*store.Edition, error) {
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
	edition := store.Edition{
		SeriesID: seriesID, EditionName: p.EditionName, Language: p.Language,
		Publisher: p.Publisher, Medium: p.Medium, Type: ctype,
		Monitored: p.Monitored, CutoffNum: p.CutoffNum,
	}
	if p.ProviderEditionID != "" {
		known, err := l.CatalogEditions(ctx)
		if err != nil {
			return nil, err
		}
		for _, e := range known {
			if e.ID == p.ProviderEditionID {
				edition.ProviderEditionID, edition.ProviderEditionName = e.ID, e.Name
			}
		}
		if edition.ProviderEditionID == "" {
			return nil, apperr.Invalidf("unknown catalog edition")
		}
	}
	cid, err := l.store.AddCustomEdition(ctx, edition, p.BasedOn, p.Volumes)
	if err != nil {
		return nil, err
	}
	if err := l.store.EnsureModels(ctx, seriesID); err != nil {
		return nil, err
	}
	return l.store.GetEdition(ctx, cid)
}

// EditionUpdate is a partial edition patch (nil = keep).
type EditionUpdate struct {
	Monitored *bool
	ProfileID *int64
	CutoffNum *int64
	Language  *string
}

// UpdateEdition applies a partial update under the field rules: the
// cutoff exists on chapter editions only, language identity is only
// editable on custom collections (MangaBaka rows are reference data).
func (l *Library) UpdateEdition(ctx context.Context, seriesID, editionID int64, u EditionUpdate) error {
	if u.Monitored != nil {
		if err := l.store.SetEditionMonitored(ctx, seriesID, editionID, *u.Monitored); err != nil {
			return err
		}
	}
	if u.ProfileID != nil {
		if err := l.store.SetEditionProfile(ctx, seriesID, editionID, *u.ProfileID); err != nil {
			return err
		}
	}
	if u.CutoffNum == nil && u.Language == nil {
		return nil
	}
	col, err := l.store.GetEdition(ctx, editionID)
	if err != nil {
		return err
	}
	if col.SeriesID != seriesID {
		return store.ErrNotFound
	}
	if u.CutoffNum != nil {
		if col.Type != "chapter" {
			return apperr.Invalidf("cutoff applies to chapter editions only")
		}
		if err := l.store.SetEditionCutoff(ctx, seriesID, editionID, *u.CutoffNum); err != nil {
			return err
		}
	}
	if u.Language != nil {
		if !col.Custom {
			return apperr.Invalidf("language can only be changed on local editions")
		}
		if err := l.store.EditEdition(ctx, seriesID, editionID, store.EditionMetadataPatch{Language: u.Language}); err != nil {
			return err
		}
	}
	return nil
}
