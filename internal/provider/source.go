package provider

import (
	"archive/zip"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/h-wb/sento/internal/extensions"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
)

// SourceProvider adapts any extensions.Host: binding-scoped search over
// extension sources, downloads done by Sento itself (page fetch → CBZ in
// a staging dir). The host is a stateless protocol adapter; all download
// state lives here.
type SourceProvider struct {
	host       extensions.Host
	stagingDir string
	entries    func(ctx context.Context) ([]IndexerEntry, error)
	pageConc   int     // max pages fetched in parallel per chapter
	pageRate   float64 // page requests/sec per source; 0 disables

	mu       sync.Mutex
	jobs     map[string]*sourceJob    // externalID → live or failed job
	limiters map[string]*rate.Limiter // sourceID → page-fetch limiter
	sem      chan struct{}            // bounds concurrent chapter downloads
}

// DownloadConfig tunes per-chapter page fetching. Zero values fall back
// to conservative defaults.
type DownloadConfig struct {
	PageConcurrency   int     // max pages in flight per chapter
	PageRatePerSecond float64 // page requests/sec per source; 0 disables
}

// NewSource returns nil (interface) when no host is configured, so it can
// feed NewRegistry directly.
func NewSource(host extensions.Host, stagingDir string, dl DownloadConfig,
	entries func(ctx context.Context) ([]IndexerEntry, error),
) Provider {
	if host == nil {
		return nil
	}
	if dl.PageConcurrency <= 0 {
		dl.PageConcurrency = 3
	}
	return &SourceProvider{
		host:       host,
		stagingDir: stagingDir,
		entries:    entries,
		pageConc:   dl.PageConcurrency,
		pageRate:   dl.PageRatePerSecond,
		jobs:       map[string]*sourceJob{},
		limiters:   map[string]*rate.Limiter{},
		sem:        make(chan struct{}, 2),
	}
}

// limiterFor returns the per-source page-fetch limiter, lazily created.
// Returns nil when rate limiting is disabled.
func (p *SourceProvider) limiterFor(sourceID string) *rate.Limiter {
	if p.pageRate <= 0 {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	l := p.limiters[sourceID]
	if l == nil {
		// Burst of 1: pace strictly at the configured rate rather than
		// letting a backlog of tokens release a thundering herd.
		l = rate.NewLimiter(rate.Limit(p.pageRate), 1)
		p.limiters[sourceID] = l
	}
	return l
}

// entry resolves one indexer entry by its site id.
func (p *SourceProvider) entry(ctx context.Context, entryID string) (IndexerEntry, error) {
	entries, err := p.entries(ctx)
	if err != nil {
		return IndexerEntry{}, err
	}
	for _, e := range entries {
		if e.EntryID() == entryID {
			return e, nil
		}
	}
	return IndexerEntry{}, fmt.Errorf("source: no indexer entry %s", entryID)
}

func (p *SourceProvider) ID() string       { return p.host.Name() }
func (p *SourceProvider) Protocol() string { return ProtocolSource }

// sourceExternalID encodes the download identity of a source candidate.
// Source ids never contain '|'; the chapter key may contain anything else.
func sourceExternalID(sourceID, chapterKey string) string {
	return sourceID + "|" + chapterKey
}

func parseSourceExternalID(externalID string) (sourceID, chapterKey string, err error) {
	sourceID, chapterKey, ok := strings.Cut(externalID, "|")
	if !ok || sourceID == "" || chapterKey == "" {
		return "", "", fmt.Errorf("source: bad external id %q", externalID)
	}
	return sourceID, chapterKey, nil
}

// Sites lists the source indexer entries; an entry whose source is
// missing on the host (extension uninstalled) reports Enabled=false.
func (p *SourceProvider) Sites(ctx context.Context) ([]Site, error) {
	entries, err := p.entries(ctx)
	if err != nil {
		return nil, err
	}
	available := map[string]extensions.Source{}
	if sources, err := p.host.Sources(ctx); err == nil {
		for _, s := range sources {
			available[s.ID] = s
		}
	}
	sites := make([]Site, 0, len(entries))
	for _, e := range entries {
		src, ok := available[e.SourceID]
		sites = append(sites, Site{
			Provider: p.ID(), ID: e.EntryID(), Name: e.Name,
			Protocol: ProtocolSource, Lang: src.Lang, Enabled: ok,
		})
	}
	return sites, nil
}

// Search lists chapters of every bound source manga, in parallel. Each
// entry is a candidate; what it covers (chapter? volume-shaped file?) is
// the parser's call, not ours. Failing bindings degrade with an error.
func (p *SourceProvider) Search(ctx context.Context, req SearchRequest) ([]Candidate, error) {
	type res struct {
		cands []Candidate
		err   error
	}
	ch := make(chan res, len(req.Bindings))
	// Source language map: a source's language is the release language of
	// every chapter it lists (chapter names rarely tag it themselves).
	langs := map[string]string{}
	if sources, err := p.host.Sources(ctx); err == nil {
		for _, s := range sources {
			langs[s.ID] = s.Lang
		}
	}
	active := 0
	for _, b := range req.Bindings {
		if !req.SiteAllowed(p.ID(), b.SourceID) || b.SourceMangaKey == "" {
			continue
		}
		active++
		go func(b Binding) {
			// Binding.SourceID is the indexer entry id; the entry maps to
			// the host-side source.
			e, err := p.entry(ctx, b.SourceID)
			if err != nil {
				ch <- res{err: err}
				return
			}
			chapters, err := p.host.Chapters(ctx, e.SourceID, b.SourceMangaKey)
			if err != nil {
				ch <- res{err: fmt.Errorf("binding %d (%s): %w", b.BindingID, e.Name, err)}
				return
			}
			cands := make([]Candidate, 0, len(chapters))
			for _, c := range chapters {
				cands = append(cands, Candidate{
					Provider: p.ID(),
					Site:     e.Name,
					SiteID:   b.SourceID,
					Protocol: ProtocolSource,
					Title:    c.Name,
					Source: &SourceRef{
						ChapterKey:    c.Key,
						ChapterNumber: c.Number,
						Scanlator:     c.Scanlator,
						SourceID:      e.SourceID,
						BindingID:     b.BindingID,
						WebURL:        c.WebURL,
						Lang:          langs[e.SourceID],
					},
				})
			}
			ch <- res{cands: cands}
		}(b)
	}
	var out []Candidate
	var firstErr error
	for range active {
		rr := <-ch
		if rr.err != nil && firstErr == nil {
			firstErr = rr.err
		}
		out = append(out, rr.cands...)
	}
	return out, firstErr
}

// ResolveManga searches an entry's source for series candidates
// (auto-binding). siteID is the indexer entry id.
func (p *SourceProvider) ResolveManga(ctx context.Context, siteID, query string) ([]extensions.Manga, error) {
	e, err := p.entry(ctx, siteID)
	if err != nil {
		return nil, err
	}
	return p.host.Search(ctx, e.SourceID, query)
}

// --- download lifecycle (Sento is the download client for sources) ---

// sourceJob tracks one chapter download. Fields are guarded by the
// provider mutex; the fetch loop snapshots under lock.
type sourceJob struct {
	total, done int
	err         error
	running     bool
}

// stagedPath is the finished-CBZ location for an external id. Hashed: keys
// may be URLs.
func (p *SourceProvider) stagedPath(externalID string) string {
	sum := sha1.Sum([]byte(externalID))
	return filepath.Join(p.stagingDir, hex.EncodeToString(sum[:])+".cbz")
}

// Grab starts the chapter download into the staging dir.
func (p *SourceProvider) Grab(ctx context.Context, c Candidate) (string, error) {
	if c.Source == nil {
		return "", fmt.Errorf("source: candidate has no source ref")
	}
	externalID := sourceExternalID(c.Source.SourceID, c.Source.ChapterKey)
	p.startJob(externalID)
	return externalID, nil
}

// startJob launches the download unless it's already running or staged.
func (p *SourceProvider) startJob(externalID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if job, ok := p.jobs[externalID]; ok && (job.running || job.err == nil) {
		return
	}
	if _, err := os.Stat(p.stagedPath(externalID)); err == nil {
		return // already staged
	}
	job := &sourceJob{running: true}
	p.jobs[externalID] = job
	go p.download(externalID, job)
}

// download fetches all pages of one chapter and assembles the CBZ. Runs
// detached from the request context: a grab outlives its HTTP call.
func (p *SourceProvider) download(externalID string, job *sourceJob) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	p.sem <- struct{}{}
	defer func() { <-p.sem }()

	err := p.fetchChapter(ctx, externalID, job)
	p.mu.Lock()
	job.running = false
	job.err = err
	p.mu.Unlock()
}

func (p *SourceProvider) fetchChapter(ctx context.Context, externalID string, job *sourceJob) error {
	sourceID, chapterKey, err := parseSourceExternalID(externalID)
	if err != nil {
		return err
	}
	pages, err := p.host.Pages(ctx, sourceID, chapterKey)
	if err != nil {
		return fmt.Errorf("listing pages: %w", err)
	}
	if len(pages) == 0 {
		return fmt.Errorf("chapter has no pages")
	}
	p.mu.Lock()
	job.total = len(pages)
	p.mu.Unlock()

	if err := os.MkdirAll(p.stagingDir, 0o755); err != nil {
		return err
	}

	// Fetch pages concurrently into temp files, bounded by pageConc and
	// paced by a per-source rate limiter. The CBZ is then assembled in
	// page order, so the archive is byte-identical to a sequential fetch.
	fetched, err := p.fetchPages(ctx, sourceID, pages, job)
	if err != nil {
		return err
	}
	defer func() {
		for _, f := range fetched {
			if f.path != "" {
				_ = os.Remove(f.path)
			}
		}
	}()

	dest := p.stagedPath(externalID)
	tmp, err := os.CreateTemp(p.stagingDir, ".part-*.cbz")
	if err != nil {
		return err
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()

	zw := zip.NewWriter(tmp)
	for i, f := range fetched {
		if err := writePageFile(zw, f, i); err != nil {
			return fmt.Errorf("page %d/%d: %w", i+1, len(pages), err)
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dest)
}

// fetchedPage is one downloaded page staged to a temp file, with the image
// extension already sniffed from its leading bytes.
type fetchedPage struct {
	path string
	ext  string
}

// fetchPages downloads every page concurrently (up to pageConc in flight)
// into temp files, pacing requests through the per-source limiter. It
// returns results in page order; on any error every temp file written so
// far is removed before returning.
func (p *SourceProvider) fetchPages(ctx context.Context, sourceID string, pages []string, job *sourceJob) ([]fetchedPage, error) {
	out := make([]fetchedPage, len(pages))
	limiter := p.limiterFor(sourceID)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(p.pageConc)

	var done int
	for i, ref := range pages {
		i, ref := i, ref
		g.Go(func() error {
			if limiter != nil {
				if err := limiter.Wait(ctx); err != nil {
					return err
				}
			}
			f, err := p.fetchPage(ctx, ref, sourceID)
			if err != nil {
				return fmt.Errorf("page %d: %w", i+1, err)
			}
			out[i] = f
			p.mu.Lock()
			done++
			job.done = done
			p.mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		for _, f := range out {
			if f.path != "" {
				_ = os.Remove(f.path)
			}
		}
		return nil, err
	}
	return out, nil
}

// fetchPage streams one page image to a temp file, sniffing its extension
// from the leading bytes.
func (p *SourceProvider) fetchPage(ctx context.Context, ref, sourceID string) (fetchedPage, error) {
	body, err := p.host.Page(ctx, sourceID, ref)
	if err != nil {
		return fetchedPage{}, err
	}
	defer func() { _ = body.Close() }()

	tmp, err := os.CreateTemp(p.stagingDir, ".page-*")
	if err != nil {
		return fetchedPage{}, err
	}
	defer func() { _ = tmp.Close() }()

	head := make([]byte, 512)
	n, err := io.ReadFull(body, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		_ = os.Remove(tmp.Name())
		return fetchedPage{}, err
	}
	head = head[:n]
	ext := imageExt(head)

	if _, err := tmp.Write(head); err != nil {
		_ = os.Remove(tmp.Name())
		return fetchedPage{}, err
	}
	if _, err := io.Copy(tmp, body); err != nil {
		_ = os.Remove(tmp.Name())
		return fetchedPage{}, err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return fetchedPage{}, err
	}
	return fetchedPage{path: tmp.Name(), ext: ext}, nil
}

// writePageFile copies a staged page into the archive, naming it by index
// with the sniffed extension.
func writePageFile(zw *zip.Writer, f fetchedPage, index int) error {
	src, err := os.Open(f.path)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	name := fmt.Sprintf("%04d%s", index+1, f.ext)
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
	if err != nil {
		return err
	}
	_, err = io.Copy(w, src)
	return err
}

func imageExt(head []byte) string {
	switch http.DetectContentType(head) {
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".jpg"
	}
}

// State reports download progress. A staged file is done; a missing job
// for an unstaged item restarts the download (resume after a restart —
// the queue is durable, the job table is not).
func (p *SourceProvider) State(ctx context.Context, externalID string) (DownloadState, error) {
	if _, _, err := parseSourceExternalID(externalID); err != nil {
		return DownloadState{}, err
	}
	if _, err := os.Stat(p.stagedPath(externalID)); err == nil {
		return DownloadState{Done: true, Detail: "staged"}, nil
	}
	p.mu.Lock()
	job, ok := p.jobs[externalID]
	if ok && job.running {
		st := DownloadState{InFlight: true, Detail: "downloading"}
		if job.total > 0 {
			st.Progress = float64(job.done) / float64(job.total)
		}
		p.mu.Unlock()
		return st, nil
	}
	if ok && job.err != nil {
		st := DownloadState{Detail: job.err.Error()}
		p.mu.Unlock()
		return st, nil
	}
	p.mu.Unlock()
	p.startJob(externalID)
	return DownloadState{InFlight: true, Detail: "restarted"}, nil
}

// Progress reports the live page-fetch fraction of a running download. It
// only reads in-memory job state — no file stat, no job restart — so it is
// safe to call per HTTP request. ok is false unless the job is running with
// a known page total.
func (p *SourceProvider) Progress(externalID string) (float64, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	job, ok := p.jobs[externalID]
	if !ok || !job.running || job.total == 0 {
		return 0, false
	}
	return float64(job.done) / float64(job.total), true
}

func (p *SourceProvider) Fetch(ctx context.Context, externalID string) (*Payload, error) {
	f, err := os.Open(p.stagedPath(externalID))
	if err != nil {
		return nil, err
	}
	return &Payload{Stream: f}, nil
}

func (p *SourceProvider) Cleanup(ctx context.Context, externalID string) error {
	p.mu.Lock()
	delete(p.jobs, externalID)
	p.mu.Unlock()
	err := os.Remove(p.stagedPath(externalID))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// Recover clears failed jobs so the next State poll retries them (the
// importer kicks this for stuck items before declaring them dead).
func (p *SourceProvider) Recover(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, job := range p.jobs {
		if !job.running && job.err != nil {
			delete(p.jobs, id)
		}
	}
	return nil
}

var (
	_ Provider      = (*SourceProvider)(nil)
	_ Recoverer     = (*SourceProvider)(nil)
	_ MangaResolver = (*SourceProvider)(nil)
)
