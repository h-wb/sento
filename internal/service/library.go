// Package service orchestrates providers and the store (use-cases behind
// the HTTP handlers).
package service

import (
	"context"
	"fmt"

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
	MangaBakaID   int64    `json:"mangabakaId"`
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

	return l.store.AddSeries(ctx, store.AddSeriesParams{
		Series:        series,
		Collections:   collections,
		Works:         works,
		MonitorAll:    req.MonitorAll,
		MonitorIDs:    req.MonitorIDs,
		ChapterLang:   req.ChapterLang,
		ChapterCutoff: req.ChapterCutoff,
	})
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
