package provider

import (
	"context"
	"errors"
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
	entries func(ctx context.Context) ([]IndexerEntry, error)
	clients []TorrentClient // empty = no grabs; the first is the default
	// sections caches each indexer's category names by endpoint URL: they
	// only change when the indexer is reconfigured upstream.
	sections sync.Map // string → map[int]string
	searches sync.Map // endpoint + credential → *torrentSearchCache
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

// TorrentClient is one download client torrents can be sent to. Several
// can be configured (say, one behind a VPN and one not); each indexer
// entry names the one its releases go to.
type TorrentClient struct {
	ID       string
	Name     string
	Client   *qbittorrent.Client
	Category string
}

func NewTorrent(entries func(ctx context.Context) ([]IndexerEntry, error), clients ...TorrentClient) Provider {
	return &TorrentProvider{entries: entries, clients: clients}
}

// clientFor picks the download client for a release from the indexer it
// came from: the client that indexer names, or the default (first) one
// when it names none. Anything short of knowing the indexer's choice is an
// error, not a fallback to the default: sending a release to the wrong
// client is exactly what naming one is meant to prevent. With a single
// client there is nothing to choose between.
func (p *TorrentProvider) clientFor(ctx context.Context, siteID string) (TorrentClient, error) {
	if len(p.clients) == 0 {
		return TorrentClient{}, fmt.Errorf("no download client is configured")
	}
	if len(p.clients) == 1 || p.entries == nil {
		return p.clients[0], nil
	}
	entries, err := p.entries(ctx)
	if err != nil {
		return TorrentClient{}, fmt.Errorf("looking up this indexer's download client: %w", err)
	}
	wanted, known := "", false
	for _, e := range entries {
		if e.EntryID() == siteID {
			wanted, known = e.DownloadClient, true
		}
	}
	if !known {
		return TorrentClient{}, fmt.Errorf("the indexer this release came from no longer exists, so its download client is unknown; search again")
	}
	if wanted == "" {
		return p.clients[0], nil
	}
	for _, c := range p.clients {
		if c.ID == wanted {
			return c, nil
		}
	}
	return TorrentClient{}, fmt.Errorf("the download client this indexer uses no longer exists; choose another on the Indexers page")
}

// ClientFor names the client a candidate goes to (provider.ClientRouter).
func (p *TorrentProvider) ClientFor(ctx context.Context, c Candidate) (string, error) {
	dl, err := p.clientFor(ctx, c.SiteID)
	return dl.ID, err
}

// dispatchClient is the client a grab uses: the one already chosen and
// recorded for the job (InClient), or the indexer's.
func (p *TorrentProvider) dispatchClient(ctx context.Context, c Candidate) (TorrentClient, error) {
	if id := boundClient(ctx); id != "" {
		return p.client(id)
	}
	return p.clientFor(ctx, c.SiteID)
}

func (p *TorrentProvider) client(id string) (TorrentClient, error) {
	for _, c := range p.clients {
		if c.ID == id {
			return c, nil
		}
	}
	return TorrentClient{}, fmt.Errorf("the download client this job was sent to is no longer configured; add it back on the Connections page")
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
			releases, err := p.searchAliases(ctx, e, req)
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
			ch <- res{cands: cands, err: err}
		}(e)
	}
	var out []Candidate
	var searchErrors []error
	for range active {
		rr := <-ch
		if rr.err != nil {
			searchErrors = append(searchErrors, rr.err)
		}
		out = append(out, rr.cands...)
	}
	return out, errors.Join(searchErrors...)
}

// Test verifies one entry's endpoint (caps request).
func (p *TorrentProvider) Test(ctx context.Context, e IndexerEntry) error {
	return torznab.New(e.URL, e.APIKey).Ping(ctx)
}

func (p *TorrentProvider) Grab(ctx context.Context, c Candidate) (string, error) {
	if c.Torrent == nil {
		return "", fmt.Errorf("torrent: candidate has no torrent ref")
	}
	dl, err := p.dispatchClient(ctx, c)
	if err != nil {
		return "", err
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
		if err := dl.Client.AddTorrent(ctx, c.Torrent.MagnetURL, dl.Category); err != nil {
			return "", err
		}
	case c.Torrent.DownloadURL != "":
		// Fetch the .torrent ourselves and upload the bytes: qBittorrent
		// resolves URL adds asynchronously from its own network (where the
		// indexer proxy may be unreachable) and drops failures silently.
		data, magnet, err := fetchTorrent(ctx, c.Torrent.DownloadURL)
		if err != nil {
			return "", err
		}
		if magnet != "" {
			// The indexer has no .torrent for this release: its download
			// link redirects to a magnet (Nyaa through Prowlarr does).
			if hash == "" {
				hash = magnetInfoHash(magnet)
			}
			if err := dl.Client.AddTorrent(ctx, magnet, dl.Category); err != nil {
				return "", err
			}
			break
		}
		if hash == "" {
			hash = torrentInfoHash(data)
		}
		if err := dl.Client.AddTorrentFile(ctx, c.Title+".torrent", data, dl.Category); err != nil {
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

// magnetRedirect stops a download-link fetch that turns out to lead to a
// magnet link, and carries the link out.
type magnetRedirect string

func (m magnetRedirect) Error() string { return "download link redirects to a magnet link" }

// torrentClient fetches .torrent files. Bounded: a grab from a background
// sweep carries no deadline of its own, and a stalled indexer must not
// hang the sweep. A redirect to a magnet link is not followed — HTTP
// cannot fetch one — but reported.
var torrentClient = &http.Client{
	Timeout: 60 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme == "magnet" {
			return magnetRedirect(req.URL.String())
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	},
}

// fetchTorrent resolves an indexer's download link to what the client
// should be given: the .torrent bytes, or the magnet link it redirects to.
func fetchTorrent(ctx context.Context, url string) (data []byte, magnet string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := torrentClient.Do(req)
	if err != nil {
		var m magnetRedirect
		if errors.As(err, &m) {
			return nil, string(m), nil
		}
		return nil, "", fmt.Errorf("torrent: fetching .torrent: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("torrent: .torrent fetch returned %d", resp.StatusCode)
	}
	data, err = io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 || data[0] != 'd' { // bencode dict
		return nil, "", fmt.Errorf("torrent: response is not a .torrent file")
	}
	return data, "", nil
}

// find looks a download up by infohash in the client that holds it
// (InClient). The same torrent in another client is someone else's: it is
// never adopted. Jobs recorded before clients were tracked are looked up
// in every client; there, a client that cannot be asked only matters when
// the torrent was not found elsewhere, and then the answer is unknown,
// not "gone".
func (p *TorrentProvider) find(ctx context.Context, externalID string) (*qbittorrent.Torrent, *qbittorrent.Client, error) {
	if len(p.clients) == 0 {
		return nil, nil, fmt.Errorf("no download client is configured")
	}
	clients := p.clients
	if id := boundClient(ctx); id != "" {
		c, err := p.client(id)
		if err != nil {
			return nil, nil, err
		}
		clients = []TorrentClient{c}
	}
	var unreachable error
	for _, c := range clients {
		torrents, err := c.Client.List(ctx, c.Category)
		if err != nil {
			unreachable = fmt.Errorf("%s: %w", c.Name, err)
			continue
		}
		for _, t := range torrents {
			if strings.EqualFold(t.Hash, externalID) {
				return &t, c.Client, nil
			}
		}
	}
	return nil, nil, unreachable
}

func (p *TorrentProvider) State(ctx context.Context, externalID string) (DownloadState, error) {
	t, _, err := p.find(ctx, externalID)
	if err != nil {
		return DownloadState{}, err
	}
	if t == nil {
		// Not (yet) visible in qBittorrent: neither done nor in flight.
		return DownloadState{}, nil
	}
	return DownloadState{
		StallEligible: !t.Completed() && torrentCanStall(t.State),
		Transferring:  t.DownloadSpeed > 0,
		Done:          t.Completed(),
		InFlight:      !t.Completed(),
		Detail:        t.State,
		Progress:      t.Progress,
	}, nil
}

func torrentCanStall(state string) bool {
	switch state {
	case "downloading", "stalledDL", "metaDL", "forcedDL", "forcedMetaDL":
		return true
	default:
		return false
	}
}

func (p *TorrentProvider) Fetch(ctx context.Context, externalID string) (*Payload, error) {
	t, client, err := p.find(ctx, externalID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, fmt.Errorf("torrent %s not found in any download client", externalID)
	}
	files, err := client.Files(ctx, t.Hash)
	if err != nil {
		return nil, err
	}
	return &Payload{Dir: t.SavePath, Files: files}, nil
}

// Remove takes a torrent Sento added out of its client, with its data.
// Only Sento's category is searched, so a torrent someone else added is
// never touched; one the client no longer has is already gone.
func (p *TorrentProvider) Remove(ctx context.Context, externalID string) error {
	t, client, err := p.find(ctx, externalID)
	if err != nil || t == nil {
		return err
	}
	return client.Delete(ctx, t.Hash, true)
}

// Cleanup is a no-op: torrent data must stay in place for seeding
// (spec.md §7); imports hardlink.
func (p *TorrentProvider) Cleanup(context.Context, string) error { return nil }

var _ Provider = (*TorrentProvider)(nil)
var _ ClientRouter = (*TorrentProvider)(nil)
var _ Remover = (*TorrentProvider)(nil)
