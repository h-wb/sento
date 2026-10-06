package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/extensions"
	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/prowlarr"
	"github.com/h-wb/sento/internal/store"
	"github.com/h-wb/sento/internal/torznab"
)

// TorznabSettings configures one torznab indexer entry.
type TorznabSettings struct {
	URL    string `json:"url"`
	APIKey string `json:"apiKey"`
	// Language is the language of this indexer's releases when a release
	// does not state one ("fr" for a French tracker). Empty = mixed.
	Language string `json:"language,omitempty"`
	// DownloadClient is the id of the download client this indexer's
	// releases go to (a tracker that must go through the VPN client, for
	// instance). Empty = the default client.
	DownloadClient string `json:"downloadClient,omitempty"`
}

// SourceSettings configures one extension-source indexer entry. PkgName
// is the owning extension: the entry is what makes the extension wanted
// (reconcile installs it; deleting the last entry uninstalls it).
type SourceSettings struct {
	SourceID string `json:"sourceId"`
	PkgName  string `json:"pkgName,omitempty"`
}

func parseEntry(ix store.Indexer) (provider.IndexerEntry, error) {
	e := provider.IndexerEntry{ID: ix.ID, Name: ix.Name, Enabled: ix.Enabled}
	switch ix.Protocol {
	case store.IndexerTorznab:
		var ts TorznabSettings
		if err := json.Unmarshal(ix.Settings, &ts); err != nil {
			return e, fmt.Errorf("indexer %d settings: %w", ix.ID, err)
		}
		e.URL, e.APIKey, e.Language = ts.URL, ts.APIKey, strings.ToLower(strings.TrimSpace(ts.Language))
		e.DownloadClient = ts.DownloadClient
	case store.IndexerSource:
		var ss SourceSettings
		if err := json.Unmarshal(ix.Settings, &ss); err != nil {
			return e, fmt.Errorf("indexer %d settings: %w", ix.ID, err)
		}
		e.SourceID = ss.SourceID
	default:
		return e, fmt.Errorf("indexer %d: unknown protocol %q", ix.ID, ix.Protocol)
	}
	return e, nil
}

// IndexerEntries is the provider-facing accessor for one protocol's
// entries (read live so edits apply without rewiring).
func IndexerEntries(st *store.Store, protocol string) func(ctx context.Context) ([]provider.IndexerEntry, error) {
	return func(ctx context.Context) ([]provider.IndexerEntry, error) {
		all, err := st.ListIndexers(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]provider.IndexerEntry, 0, len(all))
		for _, ix := range all {
			if ix.Protocol != protocol {
				continue
			}
			e, err := parseEntry(ix)
			if err != nil {
				slog.Warn("skipping unparseable indexer", "id", ix.ID, "error", err)
				continue
			}
			out = append(out, e)
		}
		return out, nil
	}
}

var reLanguageCode = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z]{2,4})?$`)

// ValidateIndexer checks an entry before save.
func ValidateIndexer(ix store.Indexer) error {
	if strings.TrimSpace(ix.Name) == "" {
		return apperr.Invalidf("name is required")
	}
	switch ix.Protocol {
	case store.IndexerTorznab:
		var ts TorznabSettings
		if err := json.Unmarshal(ix.Settings, &ts); err != nil {
			return apperr.Invalidf("settings: %w", err)
		}
		if !strings.HasPrefix(ts.URL, "http://") && !strings.HasPrefix(ts.URL, "https://") {
			return apperr.Invalidf("torznab url must start with http:// or https://")
		}
		if lang := strings.TrimSpace(ts.Language); lang != "" && !reLanguageCode.MatchString(lang) {
			return apperr.Invalidf("default language must be a language code such as en, fr or pt-BR")
		}
	case store.IndexerSource:
		var ss SourceSettings
		if err := json.Unmarshal(ix.Settings, &ss); err != nil {
			return apperr.Invalidf("settings: %w", err)
		}
		if ss.SourceID == "" {
			return apperr.Invalidf("a source must be selected")
		}
	default:
		return apperr.Invalidf("protocol must be torznab or source")
	}
	return nil
}

// TestIndexer probes one entry: torznab caps, or source presence on the
// host.
func TestIndexer(ctx context.Context, ix store.Indexer, host extensions.Host) error {
	e, err := parseEntry(ix)
	if err != nil {
		return err
	}
	switch ix.Protocol {
	case store.IndexerTorznab:
		if err := torznab.New(e.URL, e.APIKey).Ping(ctx); err != nil {
			return apperr.Upstreamf("%w", err)
		}
		return nil
	case store.IndexerSource:
		if host == nil {
			return apperr.Unavailablef("no extensions sidecar is configured")
		}
		sources, err := host.Sources(ctx)
		if err != nil {
			return apperr.Upstreamf("%w", err)
		}
		for _, s := range sources {
			if s.ID == e.SourceID {
				return nil
			}
		}
		return apperr.Upstreamf("source %s is not present on the host (extension uninstalled?)", e.SourceID)
	}
	return nil
}

// ProwlarrIndexer is one discovery result from the browse assist.
type ProwlarrIndexer struct {
	Name string `json:"name"`
	// URL is the per-indexer Torznab endpoint, ready for an entry.
	URL string `json:"url"`
}

// BrowseProwlarr lists a Prowlarr's indexers as ready-to-add Torznab
// endpoints (the Add Indexer assist; the entered credentials are
// remembered in connection settings by the caller).
func BrowseProwlarr(ctx context.Context, baseURL, apiKey string) ([]ProwlarrIndexer, error) {
	indexers, err := prowlarr.New(baseURL, apiKey).ListIndexers(ctx)
	if err != nil {
		return nil, apperr.Upstreamf("%w", err)
	}
	base := strings.TrimRight(baseURL, "/")
	out := make([]ProwlarrIndexer, 0, len(indexers))
	for _, ix := range indexers {
		if !ix.Enable {
			continue
		}
		out = append(out, ProwlarrIndexer{
			Name: ix.Name,
			URL:  fmt.Sprintf("%s/%d/api", base, ix.ID),
		})
	}
	return out, nil
}

// hostString renders a URL's host for entry summaries.
func hostString(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	return raw
}
