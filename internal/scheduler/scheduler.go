// Package scheduler runs Sento's background loops (spec.md §10). Every
// loop is crash-isolated: errors are logged, never fatal.
package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/h-wb/sento/internal/ledger"
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
	grabber   *service.Grabber        // nil ok
	volumes   *service.VolumeSearcher // nil ok
	intervals Intervals
}

func New(st *store.Store, lib *service.Library, grabber *service.Grabber,
	volumes *service.VolumeSearcher, intervals Intervals,
) *Scheduler {
	return &Scheduler{store: st, libraries: lib, grabber: grabber, volumes: volumes, intervals: intervals}
}

// Start launches all loops; they stop when ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	go s.loop(ctx, "queue-process", s.intervals.QueueProcess, s.tickQueue)
	go s.loop(ctx, "metadata-refresh", s.intervals.RefreshSweep, s.tickRefresh)
	if s.grabber != nil {
		go s.loop(ctx, "chapter-check", s.intervals.ChapterCheck, s.tickChapters)
	}
	if s.volumes != nil {
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
	if s.grabber != nil {
		if _, err := s.grabber.ProcessQueue(ctx); err != nil {
			return err
		}
	}
	if s.volumes != nil {
		if _, err := s.volumes.ProcessTorrentQueue(ctx); err != nil {
			return err
		}
	}
	return nil
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
// series: fetch source lists (bumps source_seen_max, D7), grab wanted.
func (s *Scheduler) tickChapters(ctx context.Context) error {
	series, err := s.store.ListSeries(ctx)
	if err != nil {
		return err
	}
	for _, sr := range series {
		if !sr.Monitored || sr.Status != "releasing" {
			continue
		}
		wanted, err := s.wantedChapters(ctx, sr.ID, sr.TotalChapters)
		if err != nil || len(wanted) == 0 {
			continue
		}
		candidates, err := s.grabber.FindChapterCandidates(ctx, sr.ID, wanted, false)
		if err != nil {
			slog.Warn("chapter check failed", "series", sr.ID, "error", err)
			continue
		}
		if len(candidates) > 0 {
			if _, err := s.grabber.GrabChapters(ctx, sr.ID, candidates); err != nil {
				slog.Warn("chapter grab failed", "series", sr.ID, "error", err)
			} else {
				slog.Info("chapters grabbed", "series", sr.Title, "count", len(candidates))
			}
		}
	}
	return nil
}

func (s *Scheduler) tickVolumes(ctx context.Context) error {
	series, err := s.store.ListSeries(ctx)
	if err != nil {
		return err
	}
	for _, sr := range series {
		if !sr.Monitored {
			continue
		}
		candidates, err := s.volumes.FindVolumeCandidates(ctx, sr.ID, nil)
		if err != nil {
			slog.Warn("volume sweep failed", "series", sr.ID, "error", err)
			continue
		}
		if queued := s.volumes.GrabBest(ctx, sr.ID, candidates); len(queued) > 0 {
			slog.Info("volumes grabbed", "series", sr.Title, "count", len(queued))
		}
	}
	return nil
}

func (s *Scheduler) wantedChapters(ctx context.Context, seriesID int64, totalChapters int) (map[int64]bool, error) {
	cl, err := s.store.GetChapterLedger(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	if !cl.Monitored {
		return nil, nil
	}
	links, err := s.store.SlotLinks(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	pending, err := s.store.PendingSlotKeys(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	wanted := map[int64]bool{}
	for _, slot := range ledger.BuildChapterSlots(*cl, totalChapters, links, nil) {
		if slot.Wanted && !pending[store.SlotKey("chapter", 0, slot.Number)] {
			wanted[slot.Number] = true
		}
	}
	return wanted, nil
}

func parseDBTime(s string) time.Time {
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
