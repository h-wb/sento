// Package scheduler runs Sento's background loops (spec.md §10). Every
// loop is crash-isolated: errors are logged, never fatal.
package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/h-wb/sento/internal/service"
	"github.com/h-wb/sento/internal/store"
)

type Intervals struct {
	QueueProcess     time.Duration
	RefreshSweep     time.Duration
	ChapterCheck     time.Duration
	VolumeSweep      time.Duration
	RefreshReleasing time.Duration
	RefreshEnded     time.Duration
}

func DefaultIntervals() Intervals {
	return Intervals{
		QueueProcess:     time.Minute,
		RefreshSweep:     time.Hour,
		ChapterCheck:     2 * time.Hour,
		VolumeSweep:      12 * time.Hour,
		RefreshReleasing: 24 * time.Hour,
		RefreshEnded:     7 * 24 * time.Hour,
	}
}

type Scheduler struct {
	store     *store.Store
	libraries *service.Library
	search    *service.SearchService
	importer  *service.Importer
	intervals Intervals
}

func New(st *store.Store, lib *service.Library, search *service.SearchService,
	importer *service.Importer, intervals Intervals,
) *Scheduler {
	return &Scheduler{
		store: st, libraries: lib, search: search,
		importer: importer, intervals: intervals,
	}
}

// Start launches all loops; they stop when ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	go s.loop(ctx, "queue-process", s.intervals.QueueProcess, s.tickQueue)
	go s.loop(ctx, "metadata-refresh", s.intervals.RefreshSweep, s.tickRefresh)
	if s.search.HasProviders() {
		go s.loop(ctx, "chapter-check", s.intervals.ChapterCheck, s.tickChapters)
		go s.loop(ctx, "volume-sweep", s.intervals.VolumeSweep, s.tickVolumes)
	}
}

func (s *Scheduler) loop(ctx context.Context, name string, every time.Duration, tick func(context.Context) error) {
	if every <= 0 {
		return
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			start := time.Now()
			if err := tick(ctx); err != nil {
				slog.Warn("scheduler tick failed", "loop", name, "error", err)
			} else {
				slog.Debug("scheduler tick", "loop", name, "took", time.Since(start).Round(time.Millisecond))
			}
		}
	}
}

func (s *Scheduler) tickQueue(ctx context.Context) error {
	_, err := s.importer.ProcessQueue(ctx)
	return err
}

// tickRefresh refreshes series whose metadata is stale: releasing every
// RefreshReleasing, everything else every RefreshEnded (spec.md §5.2).
func (s *Scheduler) tickRefresh(ctx context.Context) error {
	series, err := s.store.ListSeries(ctx)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, sr := range series {
		maxAge := s.intervals.RefreshEnded
		if sr.Status == "releasing" {
			maxAge = s.intervals.RefreshReleasing
		}
		refreshed := parseDBTime(sr.RefreshedAt)
		if now.Sub(refreshed) < maxAge {
			continue
		}
		if err := s.libraries.Refresh(ctx, sr.ID); err != nil {
			slog.Warn("metadata refresh failed", "series", sr.ID, "title", sr.Title, "error", err)
		}
	}
	return nil
}

// tickChapters runs the automatic chapter search for releasing, monitored
// series.
func (s *Scheduler) tickChapters(ctx context.Context) error {
	return s.sweep(ctx, service.LedgerChapter, func(sr store.Series) bool {
		return sr.Monitored && sr.Status == "releasing"
	})
}

func (s *Scheduler) tickVolumes(ctx context.Context) error {
	return s.sweep(ctx, service.LedgerVolume, func(sr store.Series) bool {
		return sr.Monitored
	})
}

// sweep runs the unified wanted-search for one ledger across the library.
func (s *Scheduler) sweep(ctx context.Context, searchLedger string, include func(store.Series) bool) error {
	series, err := s.store.ListSeries(ctx)
	if err != nil {
		return err
	}
	for _, sr := range series {
		if !include(sr) {
			continue
		}
		result, err := s.search.FindCandidates(ctx, sr.ID, searchLedger, nil)
		if err != nil {
			slog.Warn("sweep failed", "ledger", searchLedger, "series", sr.ID, "error", err)
			continue
		}
		if queued := s.search.GrabBest(ctx, sr.ID, searchLedger, result.Candidates); len(queued) > 0 {
			slog.Info("releases grabbed", "ledger", searchLedger, "series", sr.Title, "count", len(queued))
		}
	}
	return nil
}

func parseDBTime(s string) time.Time {
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
