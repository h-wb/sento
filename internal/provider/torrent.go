package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/h-wb/sento/internal/prowlarr"
	"github.com/h-wb/sento/internal/qbittorrent"
)

// TorrentProvider adapts Prowlarr (search) + qBittorrent (download).
// Either half may be absent: search-only or download-only configurations
// degrade cleanly.
type TorrentProvider struct {
	prowlarr *prowlarr.Client    // nil = no search
	qbit     *qbittorrent.Client // nil = no grabs
	category string
}

// NewTorrent returns nil (interface) when neither half is configured, so
// it can feed NewRegistry directly.
func NewTorrent(pr *prowlarr.Client, qb *qbittorrent.Client, category string) Provider {
	if pr == nil && qb == nil {
		return nil
	}
	return &TorrentProvider{prowlarr: pr, qbit: qb, category: category}
}

func (p *TorrentProvider) ID() string       { return "torrent" }
func (p *TorrentProvider) Protocol() string { return ProtocolTorrent }

func (p *TorrentProvider) Sites(ctx context.Context) ([]Site, error) {
	if p.prowlarr == nil {
		return nil, nil
	}
	indexers, err := p.prowlarr.ListIndexers(ctx)
	if err != nil {
		return nil, err
	}
	sites := make([]Site, 0, len(indexers))
	for _, i := range indexers {
		sites = append(sites, Site{
			Provider: p.ID(), ID: strconv.Itoa(i.ID), Name: i.Name,
			Protocol: ProtocolTorrent, Enabled: i.Enable,
		})
	}
	return sites, nil
}

func (p *TorrentProvider) Search(ctx context.Context, req SearchRequest) ([]Candidate, error) {
	if p.prowlarr == nil {
		return nil, nil
	}
	// Restrict at the Prowlarr level when a site filter is present.
	var indexerIDs []int
	if allowed, ok := req.Allowed[p.ID()]; ok {
		if len(allowed) == 0 {
			return nil, nil // provider allowed-set exists but is empty
		}
		for id := range allowed {
			if n, err := strconv.Atoi(id); err == nil {
				indexerIDs = append(indexerIDs, n)
			}
		}
	}
	releases, err := p.prowlarr.Search(ctx, req.Query, nil, indexerIDs)
	if err != nil {
		return nil, err
	}
	out := make([]Candidate, 0, len(releases))
	for _, rel := range releases {
		if !req.SiteAllowed(p.ID(), strconv.Itoa(rel.IndexerID)) {
			continue
		}
		out = append(out, Candidate{
			Provider: p.ID(),
			Site:     rel.Indexer,
			SiteID:   strconv.Itoa(rel.IndexerID),
			Protocol: ProtocolTorrent,
			Title:    rel.Title,
			Torrent: &TorrentRef{
				GUID:        rel.GUID,
				InfoHash:    strings.ToLower(rel.InfoHash),
				MagnetURL:   rel.MagnetURL,
				DownloadURL: rel.DownloadURL,
				Size:        rel.Size,
				Seeders:     rel.Seeders,
			},
		})
	}
	return out, nil
}

func (p *TorrentProvider) Grab(ctx context.Context, c Candidate) (string, error) {
	if p.qbit == nil {
		return "", fmt.Errorf("qbittorrent is not configured")
	}
	if c.Torrent == nil {
		return "", fmt.Errorf("torrent: candidate has no torrent ref")
	}
	switch {
	case c.Torrent.MagnetURL != "":
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
		if err := p.qbit.AddTorrentFile(ctx, c.Title+".torrent", data, p.category); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("torrent: candidate has no magnet or download url")
	}
	return c.ExternalKey(), nil
}

func fetchTorrentFile(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
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
