package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/prowlarr"
	"github.com/h-wb/sento/internal/sourcehost"
	"github.com/h-wb/sento/internal/store"
	"github.com/h-wb/sento/internal/torznab"
)

// TorznabSettings configures one torznab indexer entry.
type TorznabSettings struct {
	URL    string `json:"url"`
	APIKey string `json:"apiKey"`
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
		e.URL, e.APIKey = ts.URL, ts.APIKey
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

// ValidateIndexer checks an entry before save.
func ValidateIndexer(ix store.Indexer) error {
	if strings.TrimSpace(ix.Name) == "" {
		return fmt.Errorf("name is required")
	}
	switch ix.Protocol {
	case store.IndexerTorznab:
		var ts TorznabSettings
		if err := json.Unmarshal(ix.Settings, &ts); err != nil {
			return fmt.Errorf("settings: %w", err)
		}
		if !strings.HasPrefix(ts.URL, "http://") && !strings.HasPrefix(ts.URL, "https://") {
			return fmt.Errorf("torznab url must start with http:// or https://")
		}
	case store.IndexerSource:
		var ss SourceSettings
		if err := json.Unmarshal(ix.Settings, &ss); err != nil {
			return fmt.Errorf("settings: %w", err)
		}
		if ss.SourceID == "" {
			return fmt.Errorf("a source must be selected")
		}
	default:
		return fmt.Errorf("protocol must be torznab or source")
	}
	return nil
}

// TestIndexer probes one entry: torznab caps, or source presence on the
// host.
func TestIndexer(ctx context.Context, ix store.Indexer, host sourcehost.Host) error {
	e, err := parseEntry(ix)
	if err != nil {
		return err
	}
	switch ix.Protocol {
	case store.IndexerTorznab:
		return torznab.New(e.URL, e.APIKey).Ping(ctx)
	case store.IndexerSource:
		if host == nil {
			return fmt.Errorf("no source host is configured")
		}
		sources, err := host.Sources(ctx)
		if err != nil {
			return err
		}
		for _, s := range sources {
			if s.ID == e.SourceID {
				return nil
			}
		}
		return fmt.Errorf("source %s is not present on the host (extension uninstalled?)", e.SourceID)
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
		return nil, err
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

// MigrateIndexers converts the pre-entry world once: every Prowlarr
// indexer of the configured connection becomes a Torznab entry (Prowlarr
// exposes each as {base}/{id}/api with the same key), every host source
// becomes a source entry, carrying over site_settings toggles. Legacy
// per-series overrides and bindings re-key to the new entry ids.
func MigrateIndexers(ctx context.Context, st *store.Store, cs ConnectionSettings, host sourcehost.Host) {
	if done, _ := migrationDone(ctx, st); done {
		return
	}
	legacy, _ := st.LegacySiteSettings(ctx)

	// Torrent entries from the Prowlarr connection.
	if cs.Prowlarr.URL != "" {
		indexers, err := prowlarr.New(cs.Prowlarr.URL, cs.Prowlarr.APIKey).ListIndexers(ctx)
		if err != nil {
			slog.Warn("indexer migration: prowlarr unreachable, torrent entries not converted", "error", err)
		}
		base := strings.TrimRight(cs.Prowlarr.URL, "/")
		for _, ix := range indexers {
			settings, _ := json.Marshal(TorznabSettings{
				URL: fmt.Sprintf("%s/%d/api", base, ix.ID), APIKey: cs.Prowlarr.APIKey,
			})
			entry := store.Indexer{
				Name: ix.Name, Protocol: store.IndexerTorznab,
				Enabled: ix.Enable, UseVolumes: true, UseChapters: true, Settings: settings,
			}
			applyLegacy(&entry, legacy["torrent/"+fmt.Sprint(ix.ID)])
			id, err := st.AddIndexer(ctx, entry)
			if err != nil {
				slog.Warn("indexer migration failed", "name", ix.Name, "error", err)
				continue
			}
			st.RekeyLegacySite(ctx, "torrent", fmt.Sprint(ix.ID), id)
		}
	}

	// Source entries from the host's installed sources.
	if host != nil {
		sources, err := host.Sources(ctx)
		if err != nil {
			slog.Warn("indexer migration: source host unreachable, source entries not converted", "error", err)
		}
		for _, src := range sources {
			settings, _ := json.Marshal(SourceSettings{SourceID: src.ID})
			name := src.Name
			if src.Lang != "" {
				name = fmt.Sprintf("%s (%s)", src.Name, src.Lang)
			}
			entry := store.Indexer{
				Name: name, Protocol: store.IndexerSource,
				Enabled: true, UseVolumes: true, UseChapters: true, Settings: settings,
			}
			applyLegacy(&entry, legacy["host/"+src.ID])
			id, err := st.AddIndexer(ctx, entry)
			if err != nil {
				slog.Warn("indexer migration failed", "name", name, "error", err)
				continue
			}
			st.RekeyLegacySite(ctx, "host", src.ID, id)
		}
	}

	_ = st.PutSetting(ctx, "indexers_migrated", []byte("true"))
	slog.Info("indexer entries migrated from legacy site settings")
}

func migrationDone(ctx context.Context, st *store.Store) (bool, error) {
	raw, err := st.GetSetting(ctx, "indexers_migrated")
	return string(raw) == "true", err
}

func applyLegacy(entry *store.Indexer, legacy *store.LegacySiteSetting) {
	if legacy == nil {
		return
	}
	entry.Enabled = entry.Enabled && legacy.Enabled
	entry.UseVolumes = legacy.UseVolumes
	entry.UseChapters = legacy.UseChapters
	entry.Priority = legacy.Priority
}

// hostString renders a URL's host for entry summaries.
func hostString(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	return raw
}
