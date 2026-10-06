package service

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/h-wb/sento/internal/qbittorrent"
	"github.com/h-wb/sento/internal/sourcehost"
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

// probeTimeout bounds one remote probe, so a hung service costs a check
// run seconds rather than the request timeout.
const probeTimeout = 10 * time.Second

// HealthService probes what Sento depends on — the library folder, the
// download client, the source host and every enabled indexer — and keeps
// the last report. Probing is remote I/O, so reads are served from that
// report until it is invalidated or re-run.
type HealthService struct {
	store       *store.Store
	conns       *Connections
	libraryPath string

	run sync.Mutex // one check run at a time

	mu     sync.RWMutex
	report HealthReport
	fresh  bool
}

func NewHealthService(st *store.Store, conns *Connections, libraryPath string) *HealthService {
	return &HealthService{store: st, conns: conns, libraryPath: libraryPath}
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
	if fresh {
		return report
	}
	return h.Check(ctx)
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

	// Remote probes run side by side; each reports into its own slot so
	// the issue order stays stable between runs.
	cs := h.conns.Load(ctx)
	probes := []func(context.Context) []HealthIssue{
		func(ctx context.Context) []HealthIssue {
			return checkDownloadClient(ctx, cs.QBittorrent, len(torznabs) > 0)
		},
		func(ctx context.Context) []HealthIssue { return checkSources(ctx, h.conns.Host(), sources) },
	}
	for _, ix := range torznabs {
		probes = append(probes, func(ctx context.Context) []HealthIssue { return checkTorznab(ctx, ix) })
	}
	found := make([][]HealthIssue, len(probes))
	var wg sync.WaitGroup
	for i, probe := range probes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(ctx, probeTimeout)
			defer cancel()
			found[i] = probe(ctx)
		}()
	}
	wg.Wait()
	for _, f := range found {
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

func checkDownloadClient(ctx context.Context, qb QBittorrentConn, needed bool) []HealthIssue {
	if qb.URL == "" {
		if !needed {
			return nil
		}
		return []HealthIssue{{
			Source: "download-client", Level: HealthWarning, Link: "/connections",
			Message: "No download client is configured, so torrent releases cannot be downloaded",
		}}
	}
	if err := qbittorrent.New(qb.URL, qb.Username, qb.Password).Ping(ctx); err != nil {
		return []HealthIssue{{
			Source: "download-client", Level: HealthError, Link: "/connections",
			Message: "qBittorrent is unavailable", Detail: err.Error(),
		}}
	}
	return nil
}

// checkSources verifies the source host answers and still carries the
// source behind every enabled source indexer. A host that is down raises
// one issue, not one per indexer.
func checkSources(ctx context.Context, host sourcehost.Host, indexers []store.Indexer) []HealthIssue {
	if host == nil {
		if len(indexers) == 0 {
			return nil
		}
		return []HealthIssue{{
			Source: "source-host", Level: HealthWarning, Link: "/connections",
			Message: "No source host is configured, so source indexers cannot be searched",
		}}
	}
	sources, err := host.Sources(ctx)
	if err != nil {
		return []HealthIssue{{
			Source: "source-host", Level: HealthError, Link: "/connections",
			Message: "The source host is unavailable", Detail: err.Error(),
		}}
	}
	present := map[string]bool{}
	for _, s := range sources {
		present[s.ID] = true
	}
	var issues []HealthIssue
	for _, ix := range indexers {
		e, err := parseEntry(ix)
		switch {
		case err != nil:
			issues = append(issues, indexerIssue(ix, err.Error()))
		case !present[e.SourceID]:
			issues = append(issues, indexerIssue(ix, fmt.Sprintf("source %s is not present on the host (extension uninstalled?)", e.SourceID)))
		}
	}
	return issues
}

func checkTorznab(ctx context.Context, ix store.Indexer) []HealthIssue {
	e, err := parseEntry(ix)
	if err == nil {
		err = torznab.New(e.URL, e.APIKey).Ping(ctx)
	}
	if err != nil {
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
