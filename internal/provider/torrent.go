package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/h-wb/sento/internal/qbittorrent"
	"github.com/h-wb/sento/internal/torznab"
)

// TorrentProvider searches the configured Torznab indexer entries and
// downloads grabs through qBittorrent. Search works without a download
// client (qb nil = no grabs).
type TorrentProvider struct {
	entries  func(ctx context.Context) ([]IndexerEntry, error)
	qbit     *qbittorrent.Client // nil = no grabs
	category string
	// sections caches each indexer's category names by endpoint URL: they
	// only change when the indexer is reconfigured upstream.
	sections sync.Map // string → map[int]string
}

// sectionNames returns an indexer's category names, fetched once. A
// failure costs only the language hint, never the search.
func (p *TorrentProvider) sectionNames(ctx context.Context, e IndexerEntry) map[int]string {
	if cached, ok := p.sections.Load(e.URL); ok {
		return cached.(map[int]string)
	}
	names, err := torznab.New(e.URL, e.APIKey).Categories(ctx)
	if err != nil {
		return nil
	}
	p.sections.Store(e.URL, names)
	return names
}

// categoryLanguage reads a release language off the indexer sections it
// is filed under: Nyaa-style "English-translated" and "Raw". Sections
// that say nothing about language ("Non-English-translated", "Comics")
// yield none.
func categoryLanguage(categories []int, names map[int]string) string {
	found := ""
	for _, id := range categories {
		name := strings.ToLower(names[id])
		lang := ""
		switch {
		case strings.Contains(name, "non-english"):
		case strings.Contains(name, "english"):
			lang = "en"
		case strings.Contains(name, "raw"):
			lang = "ja"
		}
		if lang != "" && found != "" && lang != found {
			return "" // sections disagree
		}
		if lang != "" {
			found = lang
		}
	}
	return found
}

func NewTorrent(entries func(ctx context.Context) ([]IndexerEntry, error),
	qb *qbittorrent.Client, category string,
) Provider {
	return &TorrentProvider{entries: entries, qbit: qb, category: category}
}

func (p *TorrentProvider) ID() string       { return "torrent" }
func (p *TorrentProvider) Protocol() string { return ProtocolTorrent }

func (p *TorrentProvider) Sites(ctx context.Context) ([]Site, error) {
	entries, err := p.entries(ctx)
	if err != nil {
		return nil, err
	}
	sites := make([]Site, 0, len(entries))
	for _, e := range entries {
		sites = append(sites, Site{
			Provider: p.ID(), ID: e.EntryID(), Name: e.Name,
			Protocol: ProtocolTorrent, Enabled: true,
		})
	}
	return sites, nil
}

// Search queries every allowed Torznab entry in parallel. A failing
// indexer degrades with an error; the rest still answer.
func (p *TorrentProvider) Search(ctx context.Context, req SearchRequest) ([]Candidate, error) {
	entries, err := p.entries(ctx)
	if err != nil {
		return nil, err
	}
	type res struct {
		cands []Candidate
		err   error
	}
	ch := make(chan res, len(entries))
	active := 0
	for _, e := range entries {
		if !e.Enabled || !req.SiteAllowed(p.ID(), e.EntryID()) {
			continue
		}
		active++
		go func(e IndexerEntry) {
			// Books only: unrestricted, a general tracker fills its result
			// cap with films and games of the same name and the actual
			// volumes never arrive.
			releases, err := torznab.New(e.URL, e.APIKey).Search(ctx, req.Query, torznab.CategoryBooks)
			if err != nil {
				ch <- res{err: fmt.Errorf("%s: %w", e.Name, err)}
				return
			}
			names := p.sectionNames(ctx, e)
			cands := make([]Candidate, 0, len(releases))
			for _, rel := range releases {
				cands = append(cands, Candidate{
					Provider: p.ID(),
					Site:     e.Name,
					SiteID:   e.EntryID(),
					Protocol: ProtocolTorrent,
					Title:    rel.Title,
					Torrent: &TorrentRef{
						GUID:        rel.GUID,
						InfoHash:    rel.InfoHash,
						InfoURL:     rel.InfoURL,
						MagnetURL:   rel.MagnetURL,
						DownloadURL: rel.DownloadURL,
						Size:        rel.Size,
						Seeders:     rel.Seeders,

						CategoryLanguage: categoryLanguage(rel.Categories, names),
						DefaultLanguage:  e.Language,
					},
				})
			}
			ch <- res{cands: cands}
		}(e)
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

// Test verifies one entry's endpoint (caps request).
func (p *TorrentProvider) Test(ctx context.Context, e IndexerEntry) error {
	return torznab.New(e.URL, e.APIKey).Ping(ctx)
}

func (p *TorrentProvider) Grab(ctx context.Context, c Candidate) (string, error) {
	if p.qbit == nil {
		return "", fmt.Errorf("qbittorrent is not configured")
	}
	if c.Torrent == nil {
		return "", fmt.Errorf("torrent: candidate has no torrent ref")
	}
	// The external id must be the infohash: State and Fetch find the
	// download in qBittorrent by it. Many indexers publish none in their
	// feed, so it is derived from what is actually handed to the client.
	hash := strings.ToLower(c.Torrent.InfoHash)
	switch {
	case c.Torrent.MagnetURL != "":
		if hash == "" {
			hash = magnetInfoHash(c.Torrent.MagnetURL)
		}
		if err := p.qbit.AddTorrent(ctx, c.Torrent.MagnetURL, p.category); err != nil {
			return "", err
		}
	case c.Torrent.DownloadURL != "":
		// Fetch the .torrent ourselves and upload the bytes: qBittorrent
		// resolves URL adds asynchronously from its own network (where the
		// indexer proxy may be unreachable) and drops failures silently.
		data, err := fetchTorrentFile(ctx, c.Torrent.DownloadURL)
		if err != nil {
			return "", err
		}
		if hash == "" {
			hash = torrentInfoHash(data)
		}
		if err := p.qbit.AddTorrentFile(ctx, c.Title+".torrent", data, p.category); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("torrent: candidate has no magnet or download url")
	}
	if hash == "" {
		// Nothing to derive it from (a v2-only torrent, an odd magnet):
		// fall back to the feed identity, as before.
		return c.ExternalKey(), nil
	}
	return hash, nil
}

// torrentClient fetches .torrent files. Bounded: a grab from a background
// sweep carries no deadline of its own, and a stalled indexer must not
// hang the sweep.
var torrentClient = &http.Client{Timeout: 60 * time.Second}

func fetchTorrentFile(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := torrentClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("torrent: fetching .torrent: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("torrent: .torrent fetch returned %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || data[0] != 'd' { // bencode dict
		return nil, fmt.Errorf("torrent: response is not a .torrent file")
	}
	return data, nil
}

func (p *TorrentProvider) find(ctx context.Context, externalID string) (*qbittorrent.Torrent, error) {
	if p.qbit == nil {
		return nil, fmt.Errorf("qbittorrent is not configured")
	}
	torrents, err := p.qbit.List(ctx, p.category)
	if err != nil {
		return nil, err
	}
	for _, t := range torrents {
		if strings.EqualFold(t.Hash, externalID) {
			return &t, nil
		}
	}
	return nil, nil
}

func (p *TorrentProvider) State(ctx context.Context, externalID string) (DownloadState, error) {
	t, err := p.find(ctx, externalID)
	if err != nil {
		return DownloadState{}, err
	}
	if t == nil {
		// Not (yet) visible in qBittorrent: neither done nor in flight.
		return DownloadState{}, nil
	}
	return DownloadState{
		Done:     t.Completed(),
		InFlight: !t.Completed(),
		Detail:   t.State,
		Progress: t.Progress,
	}, nil
}

func (p *TorrentProvider) Fetch(ctx context.Context, externalID string) (*Payload, error) {
	t, err := p.find(ctx, externalID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, fmt.Errorf("torrent %s not found in qbittorrent", externalID)
	}
	files, err := p.qbit.Files(ctx, t.Hash)
	if err != nil {
		return nil, err
	}
	return &Payload{Dir: t.SavePath, Files: files}, nil
}

// Cleanup is a no-op: torrent data must stay in place for seeding
// (spec.md §7); imports hardlink.
func (p *TorrentProvider) Cleanup(context.Context, string) error { return nil }

var _ Provider = (*TorrentProvider)(nil)
