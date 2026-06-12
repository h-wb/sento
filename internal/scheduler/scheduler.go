// Package scheduler runs Sento's background loops (spec.md §10). Every
// loop is crash-isolated: errors are logged, never fatal.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/h-wb/sento/internal/service"
	"github.com/h-wb/sento/internal/store"
)

type Intervals struct {
	QueueProcess     time.Duration
	ExtensionSync    time.Duration
	RefreshSweep     time.Duration
	ChapterCheck     time.Duration
	VolumeSweep      time.Duration
	RefreshReleasing time.Duration
	RefreshEnded     time.Duration
}

func DefaultIntervals() Intervals {
	return Intervals{
		QueueProcess:     time.Minute,
		ExtensionSync:    30 * time.Minute,
		RefreshSweep:     time.Hour,
		ChapterCheck:     2 * time.Hour,
		VolumeSweep:      12 * time.Hour,
		RefreshReleasing: 24 * time.Hour,
		RefreshEnded:     7 * 24 * time.Hour,
	}
}

// task is one background loop plus its observable schedule state (the
// System → Tasks page reads it; Run triggers it out of cycle).
type task struct {
	slug     string
	name     string
	interval time.Duration
	tick     func(context.Context) error

	mu      sync.Mutex
	running bool
	lastRun time.Time
	lastDur time.Duration
	lastErr string
	nextRun time.Time
}

// TaskStatus is the API view of one background loop.
type TaskStatus struct {
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	IntervalSec  int    `json:"intervalSec"`
	LastRun      string `json:"lastRun,omitempty"` // RFC3339
	LastDuration int    `json:"lastDurationMs"`
	LastError    string `json:"lastError,omitempty"`
	NextRun      string `json:"nextRun,omitempty"` // RFC3339
	Running      bool   `json:"running"`
}

type Scheduler struct {
	store      *store.Store
	libraries  *service.Library
	search     *service.SearchService
	importer   *service.Importer
	extensions *service.ExtensionService
	intervals  Intervals
	tasks      []*task
}

func New(st *store.Store, lib *service.Library, search *service.SearchService,
	importer *service.Importer, extensions *service.ExtensionService, intervals Intervals,
) *Scheduler {
	s := &Scheduler{
		store: st, libraries: lib, search: search,
		importer: importer, extensions: extensions, intervals: intervals,
	}
	s.tasks = append(s.tasks, &task{
		slug: "queue-process", name: "Process Download Queue",
		interval: intervals.QueueProcess, tick: s.tickQueue,
	})
	if extensions != nil {
		// Converges a replaced/wiped sidecar back to the desired
		// extension list (no-op for other backends).
		s.tasks = append(s.tasks, &task{
			slug: "extension-sync", name: "Sync Extensions & Source Settings",
			interval: intervals.ExtensionSync, tick: extensions.Reconcile,
		})
	}
	s.tasks = append(s.tasks, &task{
		slug: "metadata-refresh", name: "Refresh Stale Metadata",
		interval: intervals.RefreshSweep, tick: s.tickRefresh,
	})
	if search.HasProviders() {
		s.tasks = append(s.tasks,
			&task{
				slug: "chapter-check", name: "Search Wanted Chapters",
				interval: intervals.ChapterCheck, tick: s.tickChapters,
			},
			&task{
				slug: "volume-sweep", name: "Search Wanted Volumes",
				interval: intervals.VolumeSweep, tick: s.tickVolumes,
			})
	}
	return s
}

// Start launches all loops; they stop when ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	for _, t := range s.tasks {
		go s.loop(ctx, t)
	}
}

func (s *Scheduler) loop(ctx context.Context, t *task) {
	if t.interval <= 0 {
		return
	}
	t.mu.Lock()
	t.nextRun = time.Now().Add(t.interval)
	t.mu.Unlock()
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runTask(ctx, t)
			t.mu.Lock()
			t.nextRun = time.Now().Add(t.interval)
			t.mu.Unlock()
		}
	}
}

// runTask executes one tick, skipping if the task is already running
// (manual trigger racing the ticker).
func (s *Scheduler) runTask(ctx context.Context, t *task) {
	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return
	}
	t.running = true
	t.mu.Unlock()

	start := time.Now()
	err := t.tick(ctx)

	t.mu.Lock()
	t.running = false
	t.lastRun = start
	t.lastDur = time.Since(start)
	t.lastErr = ""
	if err != nil {
		t.lastErr = err.Error()
	}
	t.mu.Unlock()

	if err != nil {
		slog.Warn("scheduler tick failed", "loop", t.slug, "error", err)
	} else {
		slog.Debug("scheduler tick", "loop", t.slug, "took", time.Since(start).Round(time.Millisecond))
	}
}

// Tasks reports every loop's schedule state (System → Tasks).
func (s *Scheduler) Tasks() []TaskStatus {
	out := make([]TaskStatus, 0, len(s.tasks))
	for _, t := range s.tasks {
		t.mu.Lock()
		ts := TaskStatus{
			Slug: t.slug, Name: t.name,
			IntervalSec:  int(t.interval / time.Second),
			LastDuration: int(t.lastDur / time.Millisecond),
			LastError:    t.lastErr,
			Running:      t.running,
		}
		if !t.lastRun.IsZero() {
			ts.LastRun = t.lastRun.Format(time.RFC3339)
		}
		if !t.nextRun.IsZero() {
			ts.NextRun = t.nextRun.Format(time.RFC3339)
		}
		t.mu.Unlock()
		out = append(out, ts)
	}
	return out
}

// Run triggers one loop now; the ticker cadence is unchanged.
func (s *Scheduler) Run(slug string) error {
	for _, t := range s.tasks {
		if t.slug == slug {
			go s.runTask(context.Background(), t)
			return nil
		}
	}
	return fmt.Errorf("unknown task %q", slug)
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
