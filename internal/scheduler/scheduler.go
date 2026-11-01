// Package scheduler runs Sento's background loops (spec.md §10). Every
// loop is crash-isolated: errors are logged, never fatal.
package scheduler

import (
	"context"
	"encoding/json"
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
	LibraryScan      time.Duration
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
		LibraryScan:      12 * time.Hour,
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
	scanner    *service.Scanner
	extensions *service.ExtensionService
	intervals  Intervals
	tasks      []*task

	// startupGrace is the earliest an overdue task runs after Start, so a
	// restart doesn't fire every sweep in the same instant as boot.
	startupGrace time.Duration

	mu  sync.Mutex      // guards ctx and the persisted state document
	ctx context.Context // the Start context; manual runs stop with it too
}

// stateKey is the settings document holding each task's last run time
// (slug → RFC3339), so cadences survive restarts.
const stateKey = "scheduler"

func New(st *store.Store, lib *service.Library, search *service.SearchService,
	importer *service.Importer, scanner *service.Scanner,
	extensions *service.ExtensionService, intervals Intervals,
) *Scheduler {
	s := &Scheduler{
		store: st, libraries: lib, search: search,
		importer: importer, scanner: scanner, extensions: extensions, intervals: intervals,
		startupGrace: 30 * time.Second,
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
	if scanner != nil {
		s.tasks = append(s.tasks, &task{
			slug: "library-scan", name: "Scan Library Folders",
			interval: intervals.LibraryScan, tick: s.tickLibraryScan,
		})
	}
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
	s.tasks = append(s.tasks, &task{
		slug: "housekeeping", name: "Prune Finished Queue & History",
		interval: 24 * time.Hour, tick: s.tickHousekeeping,
	})
	return s
}

// tickHousekeeping keeps the database bounded: terminal queue jobs and old
// history beyond what the UI shows are pruned.
func (s *Scheduler) tickHousekeeping(ctx context.Context) error {
	if n, err := s.store.PruneFinishedQueue(ctx, 200); err != nil {
		return err
	} else if n > 0 {
		slog.Info("pruned finished queue items", "count", n)
	}
	if n, err := s.store.PruneDecisions(ctx, 1000); err != nil {
		return err
	} else if n > 0 {
		slog.Info("pruned old decisions", "count", n)
	}
	return nil
}

// Register adds a loop owned by another component (the health checks).
// Call it before Start.
func (s *Scheduler) Register(slug, name string, interval time.Duration, tick func(context.Context) error) {
	s.tasks = append(s.tasks, &task{slug: slug, name: name, interval: interval, tick: tick})
}

// Start launches all loops; they stop when ctx is cancelled.
//
// Each task resumes its cadence from its persisted last run: a 12-hour
// sweep that last ran 11 hours ago is due in one hour, not twelve. Without
// that, an instance restarted more often than a task's interval (nightly
// image updates are enough) would never run that task at all.
func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	s.ctx = ctx
	s.mu.Unlock()

	last := s.loadState(ctx)
	for _, t := range s.tasks {
		t.mu.Lock()
		t.lastRun = last[t.slug]
		t.mu.Unlock()
		go s.loop(ctx, t)
	}
}

// firstDelay is how long a task waits after Start: the rest of its
// interval when it has run before (at least startupGrace), a full
// interval when it never has.
func (s *Scheduler) firstDelay(t *task, now time.Time) time.Duration {
	t.mu.Lock()
	lastRun := t.lastRun
	t.mu.Unlock()
	if lastRun.IsZero() {
		return t.interval
	}
	return min(max(lastRun.Add(t.interval).Sub(now), s.startupGrace), t.interval)
}

func (s *Scheduler) loop(ctx context.Context, t *task) {
	if t.interval <= 0 {
		return
	}
	delay := s.firstDelay(t, time.Now())
	t.mu.Lock()
	t.nextRun = time.Now().Add(delay)
	t.mu.Unlock()
	timer := time.NewTimer(delay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			s.runTask(ctx, t)
			t.mu.Lock()
			t.nextRun = time.Now().Add(t.interval)
			t.mu.Unlock()
			timer.Reset(t.interval)
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
	err := safeTick(ctx, t)

	t.mu.Lock()
	t.running = false
	t.lastRun = start
	t.lastDur = time.Since(start)
	t.lastErr = ""
	if err != nil {
		t.lastErr = err.Error()
	}
	t.mu.Unlock()
	s.saveLastRun(t.slug, start)

	if err != nil {
		slog.Warn("scheduler tick failed", "loop", t.slug, "error", err)
	} else {
		slog.Debug("scheduler tick", "loop", t.slug, "took", time.Since(start).Round(time.Millisecond))
	}
}

// safeTick runs one tick and turns a panic into its error: the loops are
// crash-isolated, and an unrecovered panic in a goroutine would take the
// whole process down with it.
func safeTick(ctx context.Context, t *task) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return t.tick(ctx)
}

// loadState reads the persisted last-run times; anything unreadable is
// treated as "never ran".
func (s *Scheduler) loadState(ctx context.Context) map[string]time.Time {
	out := map[string]time.Time{}
	if s.store == nil {
		return out
	}
	raw, err := s.store.GetSetting(ctx, stateKey)
	if err != nil {
		return out
	}
	var stored map[string]string
	if json.Unmarshal(raw, &stored) != nil {
		return out
	}
	for slug, v := range stored {
		if ts, err := time.Parse(time.RFC3339, v); err == nil {
			out[slug] = ts
		}
	}
	return out
}

// saveLastRun records one task's run in the state document. Best effort:
// a failed write only costs cadence accuracy after the next restart. It
// deliberately ignores the run's context — the last tick before shutdown
// is the one worth remembering.
func (s *Scheduler) saveLastRun(slug string, at time.Time) {
	if s.store == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stored := map[string]string{}
	if raw, err := s.store.GetSetting(ctx, stateKey); err == nil {
		_ = json.Unmarshal(raw, &stored)
	}
	stored[slug] = at.UTC().Format(time.RFC3339)
	raw, err := json.Marshal(stored)
	if err == nil {
		err = s.store.PutSetting(ctx, stateKey, raw)
	}
	if err != nil {
		slog.Warn("persisting scheduler state failed", "loop", slug, "error", err)
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
	s.mu.Lock()
	ctx := s.ctx
	s.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	for _, t := range s.tasks {
		if t.slug == slug {
			go s.runTask(ctx, t)
			return nil
		}
	}
	return fmt.Errorf("unknown task %q", slug)
}

func (s *Scheduler) tickQueue(ctx context.Context) error {
	_, err := s.importer.ProcessQueue(ctx)
	return err
}

// tickRefresh refreshes series whose metadata is stale: active ones
// (releasing or on hiatus) every RefreshReleasing, everything else every
// RefreshEnded (spec.md §5.2).
func (s *Scheduler) tickRefresh(ctx context.Context) error {
	series, err := s.store.ListSeries(ctx)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, sr := range series {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		maxAge := s.intervals.RefreshEnded
		if active(sr) {
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

// active reports whether a series can still get new books. A hiatus
// counts: it ends without notice, and the status only changes on the next
// refresh, so waiting for "releasing" would miss the return.
func active(sr store.Series) bool {
	return sr.Status == "releasing" || sr.Status == "hiatus"
}

// tickLibraryScan keeps the library state honest (spec.md §11.4 rescan):
// for every series it re-checks file presence — a deleted file stops
// counting as owned and its book becomes wanted again, an unreachable
// folder blocks acquisition and shows in review — and imports files that
// appeared on disk since the last pass, in place, without renaming.
func (s *Scheduler) tickLibraryScan(ctx context.Context) error {
	series, err := s.store.ListSeries(ctx)
	if err != nil {
		return err
	}
	var linked, changed int
	for _, sr := range series {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if sr.Path == "" && sr.ChapterPath == "" {
			continue
		}
		res, err := s.scanner.ScanSeriesFolder(ctx, sr.ID, "")
		if err != nil {
			slog.Warn("library scan failed", "series", sr.ID, "title", sr.Title, "error", err)
			continue
		}
		linked += res.Linked
		changed += res.Changed
		if res.Linked > 0 || res.Changed > 0 {
			slog.Info("library scan", "series", sr.Title,
				"linked", res.Linked, "changed", res.Changed, "unrequested", res.Unrequested)
		}
	}
	if linked > 0 || changed > 0 {
		slog.Info("library scan complete", "linked", linked, "changed", changed)
	}
	return nil
}

// tickChapters runs the automatic chapter search for active, monitored
// series.
func (s *Scheduler) tickChapters(ctx context.Context) error {
	return s.sweep(ctx, service.LedgerChapter, func(sr store.Series) bool {
		return sr.Monitored && active(sr)
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
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !include(sr) {
			continue
		}
		result, err := s.search.FindCandidates(ctx, sr.ID, searchLedger, nil)
		if err != nil {
			slog.Warn("sweep failed", "ledger", searchLedger, "series", sr.ID, "error", err)
			continue
		}
		if queued := s.search.GrabBest(service.Scheduled(ctx), sr.ID, searchLedger, result.Candidates); len(queued) > 0 {
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
