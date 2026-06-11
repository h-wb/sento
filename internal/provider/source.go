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

	"github.com/h-wb/sento/internal/sourcehost"
)

// SourceProvider adapts any sourcehost.Host: binding-scoped search over
// extension sources, downloads done by Sento itself (page fetch → CBZ in
// a staging dir). The host is a stateless protocol adapter; all download
// state lives here.
type SourceProvider struct {
	host       sourcehost.Host
	stagingDir string

	mu   sync.Mutex
	jobs map[string]*sourceJob // externalID → live or failed job
	sem  chan struct{}         // bounds concurrent chapter downloads
}

// NewSource returns nil (interface) when no host is configured, so it can
// feed NewRegistry directly.
func NewSource(host sourcehost.Host, stagingDir string) Provider {
	if host == nil {
		return nil
	}
	return &SourceProvider{
		host:       host,
		stagingDir: stagingDir,
		jobs:       map[string]*sourceJob{},
		sem:        make(chan struct{}, 2),
	}
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

func (p *SourceProvider) Sites(ctx context.Context) ([]Site, error) {
	sources, err := p.host.Sources(ctx)
	if err != nil {
		return nil, err
	}
	sites := make([]Site, 0, len(sources))
	for _, s := range sources {
		sites = append(sites, Site{
			Provider: p.ID(), ID: s.ID, Name: s.Name,
			Protocol: ProtocolSource, Lang: s.Lang, Enabled: true,
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
	active := 0
	for _, b := range req.Bindings {
		if !req.SiteAllowed(p.ID(), b.SourceID) || b.SourceMangaKey == "" {
			continue
		}
		active++
		go func(b Binding) {
			chapters, err := p.host.Chapters(ctx, b.SourceID, b.SourceMangaKey)
			if err != nil {
				ch <- res{err: fmt.Errorf("binding %d (%s): %w", b.BindingID, b.SourceName, err)}
				return
			}
			site := b.SourceName
			if site == "" {
				site = b.SourceID
			}
			cands := make([]Candidate, 0, len(chapters))
			for _, c := range chapters {
				cands = append(cands, Candidate{
					Provider: p.ID(),
					Site:     site,
					SiteID:   b.SourceID,
					Protocol: ProtocolSource,
					Title:    c.Name,
					Source: &SourceRef{
						ChapterKey:    c.Key,
						ChapterNumber: c.Number,
						Scanlator:     c.Scanlator,
						SourceID:      b.SourceID,
						BindingID:     b.BindingID,
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

// ResolveManga searches a source for series candidates (auto-binding).
func (p *SourceProvider) ResolveManga(ctx context.Context, siteID, query string) ([]sourcehost.Manga, error) {
	return p.host.Search(ctx, siteID, query)
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
	for i, ref := range pages {
		if err := p.writePage(ctx, zw, sourceID, ref, i); err != nil {
			return fmt.Errorf("page %d/%d: %w", i+1, len(pages), err)
		}
		p.mu.Lock()
		job.done = i + 1
		p.mu.Unlock()
		// Politeness pause between page fetches; the host may add its own
		// per-source rate limits on top.
		if i < len(pages)-1 {
			select {
			case <-time.After(250 * time.Millisecond):
			case <-ctx.Done():
				return ctx.Err()
			}
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

// writePage streams one page image into the archive, naming it by index
// with an extension sniffed from the bytes.
func (p *SourceProvider) writePage(ctx context.Context, zw *zip.Writer, sourceID, ref string, index int) error {
	body, err := p.host.Page(ctx, sourceID, ref)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()

	head := make([]byte, 512)
	n, err := io.ReadFull(body, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return err
	}
	head = head[:n]

	name := fmt.Sprintf("%04d%s", index+1, imageExt(head))
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
	if err != nil {
		return err
	}
	if _, err := w.Write(head); err != nil {
		return err
	}
	_, err = io.Copy(w, body)
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
