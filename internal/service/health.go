package service

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/extensions"
	"github.com/h-wb/sento/internal/qbittorrent"
	"github.com/h-wb/sento/internal/store"
	"github.com/h-wb/sento/internal/torznab"
)

// Health levels. An error stops something from working now; a warning is
// a gap in the setup.
const (
	HealthWarning = "warning"
	HealthError   = "error"
)

// HealthIssue is one problem found by a health check (System → Health).
type HealthIssue struct {
	// Source names the check that raised it ("indexer", "download-client").
	Source  string `json:"source"`
	Level   string `json:"level" tstype:"'warning' | 'error'"`
	Message string `json:"message"`
	// Detail is the underlying error, when there is one.
	Detail string `json:"detail,omitempty"`
	// Link is the UI route where the issue is fixed.
	Link string `json:"link,omitempty"`
}

// HealthReport is the outcome of the last check run.
type HealthReport struct {
	CheckedAt string        `json:"checkedAt"` // RFC3339
	Issues    []HealthIssue `json:"issues"`
}

// taskStatusSource supplies the health issues raised by background tasks
// whose last run failed (the scheduler is one). A nil source reports none.
type taskStatusSource interface {
	TaskIssues() []HealthIssue
}

// probeTimeout bounds one remote probe, so a hung service costs a check
// run seconds rather than the request timeout.
const probeTimeout = 10 * time.Second

// runParallel runs probes concurrently — each bounded by timeout when
// positive — and returns their results in probe order.
func runParallel[T any](ctx context.Context, timeout time.Duration, probes []func(context.Context) T) []T {
	out := make([]T, len(probes))
	var wg sync.WaitGroup
	for i, probe := range probes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}
			out[i] = probe(ctx)
		}()
	}
	wg.Wait()
	return out
}

// probeSource checks that a source is present on the extensions host.
func probeSource(ctx context.Context, host extensions.Host, sourceID string) error {
	if host == nil {
		return apperr.Unavailablef("no extensions sidecar is configured")
	}
	sources, err := host.Sources(ctx)
	if err != nil {
		return apperr.Upstreamf("%w", err)
	}
	for _, s := range sources {
		if s.ID == sourceID {
			return nil
		}
	}
	return apperr.Upstreamf("source %s is not present on the host (extension uninstalled?)", sourceID)
}

// probeTorznab pings one torznab entry (settings decode included).
func probeTorznab(ctx context.Context, ix store.Indexer) error {
	e, err := parseEntry(ix)
	if err == nil {
		err = torznab.New(e.URL, e.APIKey).Ping(ctx)
	}
	return err
}

// HealthService probes what Sento depends on — the library folder, the
// download client, the extensions sidecar and every enabled indexer — and keeps
// the last report. Probing is remote I/O, so reads are served from that
// report until it is invalidated or re-run.
type HealthService struct {
	store       *store.Store
	conns       *Connections
	libraryPath string
	tasks       taskStatusSource // optional

	run sync.Mutex // one check run at a time

	mu     sync.RWMutex
	report HealthReport
	fresh  bool
}

func NewHealthService(st *store.Store, conns *Connections, libraryPath string, tasks taskStatusSource) *HealthService {
	return &HealthService{store: st, conns: conns, libraryPath: libraryPath, tasks: tasks}
}

// Invalidate marks the report stale after a settings change; the next
// Report re-runs the checks.
func (h *HealthService) Invalidate() {
	h.mu.Lock()
	h.fresh = false
	h.mu.Unlock()
}

// Report returns the last report, running the checks first when there is
// none or it has been invalidated.
func (h *HealthService) Report(ctx context.Context) HealthReport {
	h.mu.RLock()
	report, fresh := h.report, h.fresh
	h.mu.RUnlock()
	if !fresh {
		return h.Check(ctx)
	}
	return h.withTaskFailures(report)
}

// Check runs every check now and stores the report.
func (h *HealthService) Check(ctx context.Context) HealthReport {
	h.run.Lock()
	defer h.run.Unlock()

	report := HealthReport{
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
		Issues:    h.collect(ctx),
	}
	h.mu.Lock()
	h.report, h.fresh = report, true
	h.mu.Unlock()
	return h.withTaskFailures(report)
}

// withTaskFailures merges the background tasks whose last run failed into
// the report — live state, not part of the cached report.
func (h *HealthService) withTaskFailures(report HealthReport) HealthReport {
	if h.tasks == nil {
		return report
	}
	issues := append([]HealthIssue{}, report.Issues...)
	issues = append(issues, h.tasks.TaskIssues()...)
	report.Issues = issues
	return report
}

func (h *HealthService) collect(ctx context.Context) []HealthIssue {
	issues := []HealthIssue{}
	if issue := checkLibraryPath(h.libraryPath); issue != nil {
		issues = append(issues, *issue)
	}

	indexers, err := h.store.ListIndexers(ctx)
	if err != nil {
		return append(issues, HealthIssue{
			Source: "indexer", Level: HealthError,
			Message: "Indexers could not be read from the database", Detail: err.Error(),
		})
	}
	var torznabs, sources []store.Indexer
	for _, ix := range indexers {
		switch {
		case !ix.Enabled:
		case ix.Protocol == store.IndexerTorznab:
			torznabs = append(torznabs, ix)
		case ix.Protocol == store.IndexerSource:
			sources = append(sources, ix)
		}
	}
	if len(torznabs)+len(sources) == 0 {
		issues = append(issues, HealthIssue{
			Source: "indexer", Level: HealthWarning, Link: "/indexers",
			Message: "No indexers are enabled, so Sento cannot search for releases",
		})
	}

	// Remote probes run side by side; each reports into its own book so
	// the issue order stays stable between runs.
	cs := h.conns.Load(ctx)
	probes := []func(context.Context) []HealthIssue{
		func(ctx context.Context) []HealthIssue {
			return checkDownloadClients(ctx, cs.DownloadClients, torznabs)
		},
		func(ctx context.Context) []HealthIssue { return checkSources(ctx, h.conns.Host(), sources) },
	}
	for _, ix := range torznabs {
		probes = append(probes, func(ctx context.Context) []HealthIssue { return checkTorznab(ctx, ix) })
	}
	for _, f := range runParallel(ctx, probeTimeout, probes) {
		issues = append(issues, f...)
	}
	// Errors first; the stable sort keeps the check order within a level.
	sort.SliceStable(issues, func(i, j int) bool {
		return issues[i].Level == HealthError && issues[j].Level != HealthError
	})
	return issues
}

// checkLibraryPath verifies imports have somewhere to land: the folder,
// once it exists, accepts a new file.
func checkLibraryPath(path string) *HealthIssue {
	issue := func(message string, err error) *HealthIssue {
		return &HealthIssue{Source: "library", Level: HealthError, Message: message, Detail: err.Error()}
	}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil // created on the first import
	}
	if err != nil {
		return issue(fmt.Sprintf("Library folder %s cannot be read", path), err)
	}
	if !info.IsDir() {
		return issue(fmt.Sprintf("Library path %s is not a folder", path), fmt.Errorf("expected a directory"))
	}
	probe, err := os.CreateTemp(path, ".sento-health-*")
	if err != nil {
		return issue(fmt.Sprintf("Library folder %s is not writable, so downloads cannot be imported", path), err)
	}
	_ = probe.Close()
	_ = os.Remove(probe.Name())
	return nil
}

// checkDownloadClients verifies every download client answers, that
// there is one when torrent indexers need it, and that each indexer's
// chosen client still exists.
func checkDownloadClients(ctx context.Context, clients []DownloadClientConn, torznabs []store.Indexer) []HealthIssue {
	clients = normalizeClients(clients)
	if len(clients) == 0 {
		if len(torznabs) == 0 {
			return nil
		}
		return []HealthIssue{{
			Source: "download-client", Level: HealthWarning, Link: "/connections",
			Message: "No download client is configured, so torrent releases cannot be downloaded",
		}}
	}
	probes := make([]func(context.Context) []HealthIssue, len(clients))
	for i, dc := range clients {
		probes[i] = func(ctx context.Context) []HealthIssue {
			if err := qbittorrent.New(dc.URL, dc.Username, dc.Password).Ping(ctx); err != nil {
				return []HealthIssue{{
					Source: "download-client", Level: HealthError, Link: "/connections",
					Message: fmt.Sprintf("Download client %s is unavailable", dc.Name), Detail: err.Error(),
				}}
			}
			return nil
		}
	}
	var issues []HealthIssue
	for _, f := range runParallel(ctx, 0, probes) {
		issues = append(issues, f...)
	}
	known := map[string]bool{"": true}
	for _, dc := range clients {
		known[dc.ID] = true
	}
	for _, ix := range torznabs {
		if e, err := parseEntry(ix); err == nil && !known[e.DownloadClient] {
			issues = append(issues, HealthIssue{
				Source: "indexer", Level: HealthError, Link: "/indexers",
				Message: fmt.Sprintf("Indexer %s uses a download client that no longer exists", ix.Name),
			})
		}
	}
	return issues
}

// checkSources verifies the extensions sidecar answers and still carries the
// source behind every enabled source indexer. A host that is down raises
// one issue, not one per indexer.
func checkSources(ctx context.Context, host extensions.Host, indexers []store.Indexer) []HealthIssue {
	if host == nil {
		if len(indexers) == 0 {
			return nil
		}
		return []HealthIssue{{
			Source: "extensions", Level: HealthWarning, Link: "/connections",
			Message: "No extensions sidecar is configured, so source indexers cannot be searched",
		}}
	}
	if _, err := host.Sources(ctx); err != nil {
		return []HealthIssue{{
			Source: "extensions", Level: HealthError, Link: "/connections",
			Message: "The extensions sidecar is unavailable", Detail: err.Error(),
		}}
	}
	var issues []HealthIssue
	for _, ix := range indexers {
		e, err := parseEntry(ix)
		if err != nil {
			issues = append(issues, indexerIssue(ix, err.Error()))
			continue
		}
		if err := probeSource(ctx, host, e.SourceID); err != nil {
			issues = append(issues, indexerIssue(ix, err.Error()))
		}
	}
	return issues
}

func checkTorznab(ctx context.Context, ix store.Indexer) []HealthIssue {
	if err := probeTorznab(ctx, ix); err != nil {
		return []HealthIssue{indexerIssue(ix, err.Error())}
	}
	return nil
}

func indexerIssue(ix store.Indexer, detail string) HealthIssue {
	return HealthIssue{
		Source: "indexer", Level: HealthError, Link: "/indexers",
		Message: fmt.Sprintf("Indexer %s is unavailable", ix.Name), Detail: detail,
	}
}
