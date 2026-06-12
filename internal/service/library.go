// Package service orchestrates providers and the store (use-cases behind
// the HTTP handlers).
package service

import (
	"context"
	"fmt"
	"strings"

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
	// "none" nothing. MonitorAll/MonitorIDs override it when set.
	Monitor       string   `json:"monitor"`
	MonitorAll    bool     `json:"monitorAll"`    // monitor all volume-type collections
	MonitorIDs    []string `json:"monitorIds"`    // or explicit provider collection ids
	ChapterLang   string   `json:"chapterLang"`   // default "en"
	ChapterCutoff int64    `json:"chapterCutoff"` // hundredths; 0 = latest at add (D6)
}

func (l *Library) AddSeries(ctx context.Context, req AddSeriesRequest) (int64, error) {
	series, err := l.provider.GetSeries(ctx, req.MangaBakaID)
	if err != nil {
		return 0, fmt.Errorf("fetching series %d: %w", req.MangaBakaID, err)
	}
	collections, err := l.provider.GetCollections(ctx, series.MangaBakaID)
	if err != nil {
		return 0, fmt.Errorf("fetching collections: %w", err)
	}
	works, err := l.provider.GetWorks(ctx, series.MangaBakaID)
	if err != nil {
		return 0, fmt.Errorf("fetching works: %w", err)
	}

	monitorAll := req.MonitorAll || req.Monitor == "all"
	monitorIDs := req.MonitorIDs
	if !monitorAll && len(monitorIDs) == 0 && req.Monitor != "none" {
		if id := defaultEditionID(collections, req.ChapterLang); id != "" {
			monitorIDs = []string{id}
		}
	}

	return l.store.AddSeries(ctx, store.AddSeriesParams{
		Series:        series,
		Collections:   collections,
		Works:         works,
		MonitorAll:    monitorAll,
		MonitorIDs:    monitorIDs,
		ChapterLang:   req.ChapterLang,
		ChapterCutoff: req.ChapterCutoff,
	})
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
			score += 2
		}
		if strings.EqualFold(c.Language, lang) {
			score++
		}
		if score > bestScore {
			best, bestScore = c.ProviderID, score
		}
	}
	return best
}

// Refresh re-fetches metadata and reconciles it into the store. Collections
// are matched by descriptor, not provider UUID (D13); slot/file links are
// never touched.
func (l *Library) Refresh(ctx context.Context, seriesID int64) error {
	existing, err := l.store.GetSeries(ctx, seriesID)
	if err != nil {
		return err
	}
	series, err := l.provider.GetSeries(ctx, existing.MangaBakaID)
	if err != nil {
		return err
	}
	collections, err := l.provider.GetCollections(ctx, series.MangaBakaID)
	if err != nil {
		return err
	}
	works, err := l.provider.GetWorks(ctx, series.MangaBakaID)
	if err != nil {
		return err
	}
	return l.store.ReconcileMetadata(ctx, seriesID, series, collections, works)
}
