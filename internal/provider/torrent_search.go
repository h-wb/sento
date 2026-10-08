package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/h-wb/sento/internal/match"
	"github.com/h-wb/sento/internal/torznab"
)

// TorrentQueries caps fan-out and keeps aliases deterministic. Titles are
// ordered by the caller, with the selected edition's localized names first
// after the primary name. Normalization also avoids punctuation-only queries.
func TorrentQueries(req SearchRequest) []string {
	var out []string
	seen := map[string]bool{}
	for _, title := range append([]string{req.Query}, req.Titles...) {
		q := match.Normalize(title)
		if q == "" || seen[q] {
			continue
		}
		seen[q] = true
		out = append(out, q)
		if len(out) == 4 {
			break
		}
	}
	return out
}

type cachedTorrentSearch struct {
	at       time.Time
	releases []torznab.Release
}

type torrentSearchCache struct {
	gate    chan struct{}
	entries map[string]cachedTorrentSearch
}

func (p *TorrentProvider) searchAliases(ctx context.Context, e IndexerEntry, req SearchRequest) ([]torznab.Release, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	value, _ := p.searches.LoadOrStore(e.URL+"\x00"+e.APIKey, &torrentSearchCache{gate: make(chan struct{}, 1), entries: map[string]cachedTorrentSearch{}})
	cache := value.(*torrentSearchCache)
	// One request at a time per endpoint, including concurrent searches of
	// different series. A search cannot multiply the indexer's load by four.
	select {
	case cache.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-cache.gate }()
	var out []torznab.Release
	var problems []error
	seen := map[string]bool{}
	for _, query := range TorrentQueries(req) {
		hit, ok := cache.entries[query]
		if !ok || time.Since(hit.at) >= 2*time.Minute {
			releases, err := torznab.New(e.URL, e.APIKey).Search(ctx, query, torznab.CategoryBooks)
			if err != nil {
				problems = append(problems, fmt.Errorf("%s (%s): %w", e.Name, query, err))
				if ctx.Err() != nil {
					break
				}
				continue
			}
			if len(cache.entries) >= 128 {
				clear(cache.entries)
			}
			hit = cachedTorrentSearch{at: time.Now(), releases: releases}
			cache.entries[query] = hit
		}
		for _, release := range hit.releases {
			// Deduplicate repeated offers across aliases, not different
			// offers for the same torrent. Their download URLs can differ.
			key := release.GUID
			if key == "" {
				key = strings.Join([]string{release.InfoHash, release.DownloadURL, release.MagnetURL, release.Title}, "\x00")
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, release)
		}
	}
	return out, errors.Join(problems...)
}
