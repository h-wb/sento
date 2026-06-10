// Package provider is the abstraction over release sources and download
// clients (spec.md §6, decision-engine.md §2). A Provider produces uniform
// Candidates and owns the download lifecycle for the ones it grabs. The
// decision funnel and the import pipeline never know which provider a
// candidate came from.
package provider

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	ProtocolTorrent = "torrent"
	ProtocolSource  = "source"
)

// Site is one searchable location (indexer or extension source), shown
// uniformly as provider/name.
type Site struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	Lang     string `json:"lang,omitempty"`
	Enabled  bool   `json:"enabled"`
}

// Binding scopes a search to a series on a specific extension source
// (providers that need it; others ignore it).
type Binding struct {
	BindingID     int64
	SourceID      string
	SourceMangaID int64
	SourceName    string
}

type SearchRequest struct {
	SeriesID int64
	Query    string   // primary title, for text-search providers
	Titles   []string // all titles/aliases, for anchored parsing by callers
	Bindings []Binding
	// Allowed restricts the search to specific sites per provider id;
	// nil map or missing provider key = all sites allowed.
	Allowed map[string]map[string]bool
}

// SiteAllowed reports whether a provider's site passes the filter.
func (r SearchRequest) SiteAllowed(providerID, siteID string) bool {
	if r.Allowed == nil {
		return true
	}
	set, ok := r.Allowed[providerID]
	if !ok {
		return true
	}
	return set[siteID]
}

// TorrentRef carries torrent-protocol handles and health.
type TorrentRef struct {
	GUID        string `json:"guid"`
	InfoHash    string `json:"infoHash,omitempty"`
	MagnetURL   string `json:"-"`
	DownloadURL string `json:"-"`
	Size        int64  `json:"size,omitempty"`
	Seeders     int    `json:"seeders,omitempty"`
}

// SourceRef carries extension-source handles.
type SourceRef struct {
	ChapterID     int64   `json:"chapterId"`
	ChapterNumber float64 `json:"-"` // the source's own recognition (corroboration)
	Scanlator     string  `json:"scanlator,omitempty"`
	SourceID      string  `json:"sourceId"`
	BindingID     int64   `json:"bindingId,omitempty"`
}

// Candidate is one release from any site, before the decision funnel.
// Exactly one of Torrent/Source is set, matching Protocol.
type Candidate struct {
	Provider string      `json:"provider"`
	Site     string      `json:"site"`
	SiteID   string      `json:"siteId"`
	Protocol string      `json:"protocol"`
	Title    string      `json:"title"`
	Torrent  *TorrentRef `json:"torrent,omitempty"`
	Source   *SourceRef  `json:"source,omitempty"`
}

// GUID is a stable identifier for UI round-trips.
func (c Candidate) GUID() string {
	switch {
	case c.Torrent != nil:
		return c.Torrent.GUID
	case c.Source != nil:
		return fmt.Sprintf("source-%s-%d", c.Source.SourceID, c.Source.ChapterID)
	}
	return ""
}

// ExternalKey is the download-side identity of a candidate — the same key
// providers return from Grab (infohash / chapter id), used to match queue
// items and blocklist entries.
func (c Candidate) ExternalKey() string {
	switch {
	case c.Torrent != nil:
		if c.Torrent.InfoHash != "" {
			return strings.ToLower(c.Torrent.InfoHash)
		}
		return strings.ToLower(c.Torrent.GUID)
	case c.Source != nil:
		return strconv.FormatInt(c.Source.ChapterID, 10)
	}
	return ""
}

type DownloadState struct {
	Done     bool
	InFlight bool
	Detail   string // provider-specific state name (DOWNLOADING, ERROR, …)
	Progress float64
}

// Payload hands finished bytes to the importer: either a single archive
// stream (Stream != nil) or a directory of files left in place.
type Payload struct {
	Stream io.ReadCloser
	Dir    string
	Files  []string // relative to Dir
}

type Provider interface {
	// ID is the stable provider key ("suwayomi", "torrent").
	ID() string
	Sites(ctx context.Context) ([]Site, error)
	Search(ctx context.Context, req SearchRequest) ([]Candidate, error)
	// Grab starts the download and returns the external id used for
	// State/Fetch/Cleanup.
	Grab(ctx context.Context, c Candidate) (string, error)
	State(ctx context.Context, externalID string) (DownloadState, error)
	Fetch(ctx context.Context, externalID string) (*Payload, error)
	// Cleanup releases provider-side storage after a successful import.
	// Providers that must keep data (seeding) make this a no-op.
	Cleanup(ctx context.Context, externalID string) error
}

// Recoverer is an optional capability: kick a stuck download queue.
type Recoverer interface {
	Recover(ctx context.Context) error
}

// MangaRef is a series candidate on an extension source (for binding
// resolution).
type MangaRef struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

// MangaResolver is an optional capability: providers whose sites need a
// series→manga identity mapping before they can be searched (extension
// sources). Used by the auto-binding resolver.
type MangaResolver interface {
	ResolveManga(ctx context.Context, siteID, query string) ([]MangaRef, error)
}

// Registry holds the configured providers.
type Registry struct {
	providers []Provider
	byID      map[string]Provider
}

func NewRegistry(providers ...Provider) *Registry {
	r := &Registry{byID: map[string]Provider{}}
	for _, p := range providers {
		if p == nil {
			continue
		}
		r.providers = append(r.providers, p)
		r.byID[p.ID()] = p
	}
	return r
}

func (r *Registry) All() []Provider        { return r.providers }
func (r *Registry) Get(id string) Provider { return r.byID[id] }
func (r *Registry) Empty() bool            { return len(r.providers) == 0 }

// ForProtocol maps a queue item's protocol back to its owning provider.
// Valid while protocol↔provider is 1:1; revisit when a second source-
// protocol provider exists (then queue items must store the provider id).
func (r *Registry) ForProtocol(protocol string) Provider {
	switch protocol {
	case ProtocolSource:
		return r.byID["suwayomi"]
	case ProtocolTorrent:
		return r.byID["torrent"]
	}
	return nil
}
