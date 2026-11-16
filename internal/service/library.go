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
	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

type Library struct {
	store    *store.Store
	provider metadata.Provider
	registry *provider.Registry

	editionsMu sync.Mutex
	editions   []metadata.Edition
}

func NewLibrary(st *store.Store, provider metadata.Provider, registry *provider.Registry) *Library {
	return &Library{store: st, provider: provider, registry: registry}
}

type AddSeriesRequest struct {
	MangaBakaID int64 `json:"mangabakaId"`
	// Monitor is the edition-monitoring mode: "" or "default" monitors the
	// default edition (see defaultEditionID), "all" every volume edition,
	// "none" nothing. MonitorIDs overrides it with explicit picks.
	Monitor       string   `json:"monitor"`
	MonitorIDs    []string `json:"monitorIds"`    // explicit provider edition ids
	ChapterLang   string   `json:"chapterLang"`   // default "en"
	ChapterCutoff int64    `json:"chapterCutoff"` // hundredths; 0 = latest at add when volumes are collected, else every chapter (D6)
	ProfileID     int64    `json:"profileId"`     // 0 = instance default
}

// fetchCatalog loads the provider's series, collections and works records
// for one MangaBaka id.
func (l *Library) fetchCatalog(ctx context.Context, mangabakaID int64) (*metadata.Series, []metadata.Collection, map[string][]metadata.Work, error) {
	series, err := l.provider.GetSeries(ctx, mangabakaID)
	if err != nil {
		return nil, nil, nil, apperr.Upstreamf("fetching series %d: %w", mangabakaID, err)
	}
	collections, err := l.provider.GetCollections(ctx, series.MangaBakaID)
	if err != nil {
		return nil, nil, nil, apperr.Upstreamf("fetching collections: %w", err)
	}
	works, err := l.provider.GetWorks(ctx, series.MangaBakaID)
	if err != nil {
		return nil, nil, nil, apperr.Upstreamf("fetching works: %w", err)
	}
	return series, collections, works, nil
}

func (l *Library) AddSeries(ctx context.Context, req AddSeriesRequest) (int64, error) {
	if req.MangaBakaID == 0 {
		return 0, apperr.Invalidf("mangabakaId is required")
	}
	series, collections, works, err := l.fetchCatalog(ctx, req.MangaBakaID)
	if err != nil {
		return 0, err
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
	series, collections, works, err := l.fetchCatalog(ctx, existing.MangaBakaID)
	if err != nil {
		return err
	}
	return l.store.ReconcileMetadata(ctx, seriesID, series, collections, works)
}

// UpdateSeries applies a series-level patch (nil = keep): monitoring gates
// the scheduler's automatic searches, the profile is 0 for the instance
// default.
func (l *Library) UpdateSeries(ctx context.Context, seriesID int64, monitored *bool, profileID *int64) error {
	if monitored != nil {
		if err := l.store.SetSeriesMonitored(ctx, seriesID, *monitored); err != nil {
			return err
		}
	}
	if profileID != nil {
		if err := l.store.SetSeriesProfile(ctx, seriesID, *profileID); err != nil {
			return err
		}
	}
	return nil
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
	CutoffNum *int64
	Language  *string
	// ReaderFields replaces the edition's own reader field templates.
	ReaderFields *map[string]string
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
	if u.CutoffNum == nil && u.Language == nil && u.ReaderFields == nil {
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
	if u.ReaderFields != nil {
		series, err := l.store.GetSeries(ctx, seriesID)
		if err != nil {
			return err
		}
		if err := validateReaderFields(*u.ReaderFields, series, *col, nil); err != nil {
			return err
		}
		own := store.ReaderFields{}
		for key, template := range *u.ReaderFields {
			if strings.TrimSpace(template) != "" {
				own[key] = template
			}
		}
		if err := l.store.SetEditionReaderFields(ctx, seriesID, editionID, own); err != nil {
			return err
		}
	}
	return nil
}

// SaveVersion saves a way of collecting an edition, as a person sets it:
// an edition is collected once per release profile (two of its cards with
// one profile would want the same releases for the same books), and
// release restrictions live in the profile, never here. A card beside the
// first is named after its profile: the name is its folder.
func (l *Library) SaveVersion(ctx context.Context, seriesID int64, t store.CollectedVersion) (int64, error) {
	taken, err := l.store.ProfileTaken(ctx, t.EditionID, t.ID, t.ProfileID)
	if err != nil {
		return 0, err
	}
	if taken {
		return 0, apperr.Conflictf("this edition is already collected with that release profile")
	}
	if !t.IsDefault && t.ProfileID > 0 {
		p, err := l.store.GetProfile(ctx, t.ProfileID)
		if err != nil {
			return 0, err
		}
		t.Name = p.Name
	}
	return l.store.SaveVersion(ctx, seriesID, t)
}

// UpdateBinding applies a binding patch field by field, in order (nil =
// keep, "" = no state change).
func (l *Library) UpdateBinding(ctx context.Context, seriesID, bindingID int64, trustChapters *bool, editionID *int64, state string) error {
	if trustChapters != nil {
		if err := l.store.SetBindingTrust(ctx, seriesID, bindingID, *trustChapters); err != nil {
			return err
		}
	}
	if editionID != nil {
		if err := l.store.SetBindingEdition(ctx, seriesID, bindingID, *editionID); err != nil {
			return err
		}
	}
	if state == "" && (trustChapters != nil || editionID != nil) {
		return nil
	}
	if state != "active" && state != "disabled" {
		return apperr.Invalidf("state must be active or disabled")
	}
	return l.store.SetBindingState(ctx, seriesID, bindingID, state)
}

// SeparateCollection gives one collection an edition of its own and
// rebuilds the series' book models.
func (l *Library) SeparateCollection(ctx context.Context, seriesID, collectionID int64) (int64, error) {
	edition, err := l.store.SeparateCollection(ctx, seriesID, collectionID)
	if err != nil {
		return 0, err
	}
	if err := l.store.EnsureModels(ctx, seriesID); err != nil {
		return 0, err
	}
	return edition, nil
}

// EditEditionMetadata edits one edition's metadata; resetting fields
// refreshes the series from the provider afterwards.
func (l *Library) EditEditionMetadata(ctx context.Context, seriesID, editionID int64, p store.EditionMetadataPatch) error {
	if err := l.store.EditEdition(ctx, seriesID, editionID, p); err != nil {
		return err
	}
	if len(p.Reset) > 0 {
		return l.Refresh(ctx, seriesID)
	}
	return nil
}

// ResetBook clears one book's catalog state, then refreshes the series.
func (l *Library) ResetBook(ctx context.Context, seriesID, bookID int64) error {
	if err := l.store.ResetBook(ctx, seriesID, bookID); err != nil {
		return err
	}
	return l.Refresh(ctx, seriesID)
}

// BookWorks lists the catalog works behind one book; a book that belongs to
// another series is not found here.
func (l *Library) BookWorks(ctx context.Context, seriesID, bookID int64) ([]store.Work, error) {
	book, err := l.store.GetBook(ctx, bookID)
	if err == nil {
		var edition *store.Edition
		if edition, err = l.store.GetEdition(ctx, book.EditionID); err == nil && edition.SeriesID != seriesID {
			err = store.ErrNotFound
		}
	}
	if err != nil {
		return nil, err
	}
	return l.store.ListWorks(ctx, bookID)
}
