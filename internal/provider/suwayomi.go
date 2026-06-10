package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/h-wb/sento/internal/suwayomi"
)

// SuwayomiProvider adapts a Suwayomi instance: binding-scoped search over
// extension sources, downloads via Suwayomi's queue, CBZ payloads.
type SuwayomiProvider struct {
	client *suwayomi.Client
}

// NewSuwayomi returns nil (interface) when no client is configured, so it
// can feed NewRegistry directly.
func NewSuwayomi(client *suwayomi.Client) Provider {
	if client == nil {
		return nil
	}
	return &SuwayomiProvider{client: client}
}

func (p *SuwayomiProvider) ID() string { return "suwayomi" }

func (p *SuwayomiProvider) Sites(ctx context.Context) ([]Site, error) {
	sources, err := p.client.ListSources(ctx)
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
func (p *SuwayomiProvider) Search(ctx context.Context, req SearchRequest) ([]Candidate, error) {
	type res struct {
		cands []Candidate
		err   error
	}
	ch := make(chan res, len(req.Bindings))
	active := 0
	for _, b := range req.Bindings {
		if !req.SiteAllowed(p.ID(), b.SourceID) || b.SourceMangaID == 0 {
			continue
		}
		active++
		go func(b Binding) {
			chapters, err := p.client.GetChapters(ctx, b.SourceMangaID)
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
						ChapterID:     c.ID,
						ChapterNumber: c.ChapterNumber,
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

func (p *SuwayomiProvider) Grab(ctx context.Context, c Candidate) (string, error) {
	if c.Source == nil {
		return "", fmt.Errorf("suwayomi: candidate has no source ref")
	}
	if err := p.client.EnqueueDownload(ctx, c.Source.ChapterID); err != nil {
		return "", err
	}
	return strconv.FormatInt(c.Source.ChapterID, 10), nil
}

func (p *SuwayomiProvider) State(ctx context.Context, externalID string) (DownloadState, error) {
	chapterID, err := strconv.ParseInt(externalID, 10, 64)
	if err != nil {
		return DownloadState{}, fmt.Errorf("suwayomi: bad external id %q", externalID)
	}
	st, err := p.client.DownloadStateOf(ctx, chapterID)
	if err != nil {
		return DownloadState{}, err
	}
	return DownloadState{
		Done:     st.Downloaded,
		InFlight: st.Queued,
		Detail:   st.State,
		Progress: st.Progress,
	}, nil
}

func (p *SuwayomiProvider) Fetch(ctx context.Context, externalID string) (*Payload, error) {
	chapterID, err := strconv.ParseInt(externalID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("suwayomi: bad external id %q", externalID)
	}
	body, _, err := p.client.FetchCBZ(ctx, chapterID)
	if err != nil {
		return nil, err
	}
	return &Payload{Stream: body}, nil
}

func (p *SuwayomiProvider) Cleanup(ctx context.Context, externalID string) error {
	chapterID, err := strconv.ParseInt(externalID, 10, 64)
	if err != nil {
		return nil
	}
	return p.client.DeleteDownload(ctx, chapterID)
}

// Recover kicks Suwayomi's downloader (adapter spec §1.4 gotcha).
func (p *SuwayomiProvider) Recover(ctx context.Context) error {
	return p.client.Recover(ctx)
}

// ResolveManga searches a source for series candidates (auto-binding).
func (p *SuwayomiProvider) ResolveManga(ctx context.Context, siteID, query string) ([]MangaRef, error) {
	page, err := p.client.SearchManga(ctx, siteID, query, 1)
	if err != nil {
		return nil, err
	}
	refs := make([]MangaRef, 0, len(page.Mangas))
	for _, m := range page.Mangas {
		refs = append(refs, MangaRef{ID: m.ID, Title: m.Title})
	}
	return refs, nil
}

var (
	_ Provider      = (*SuwayomiProvider)(nil)
	_ Recoverer     = (*SuwayomiProvider)(nil)
	_ MangaResolver = (*SuwayomiProvider)(nil)
)
