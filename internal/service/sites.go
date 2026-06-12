package service

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"

	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

// SiteService backs the Indexers UI: the configured entries with
// reachability, plus per-series override views.
type SiteService struct {
	store    *store.Store
	registry *provider.Registry
}

func NewSiteService(st *store.Store, registry *provider.Registry) *SiteService {
	return &SiteService{store: st, registry: registry}
}

// IndexerView is one entry with live reachability and a display summary.
type IndexerView struct {
	store.Indexer `tstype:",extends"`
	Reachable     bool   `json:"reachable"`
	Summary       string `json:"summary"` // endpoint host / source name
}

// SeriesIndexerView adds per-series override state and the binding set (a
// source entry can bind several manga entries; the funnel picks per slot).
type SeriesIndexerView struct {
	IndexerView `tstype:",extends"`
	Override    *store.SiteOverride `json:"override,omitempty"`
	Bindings    []store.Binding     `json:"bindings,omitempty"`
}

// siteState is the search-time view of one entry: effective settings
// after per-series overrides, plus usability for the requested ledger.
type siteState struct {
	providerID string
	entryID    string
	name       string
	protocol   string
	reachable  bool
	eff        store.Indexer
	usable     bool
}

// reachability asks the providers which entries answer right now (source
// entries go stale when their extension is uninstalled).
func reachability(ctx context.Context, registry *provider.Registry) map[string]bool {
	out := map[string]bool{}
	for _, p := range registry.All() {
		sites, err := p.Sites(ctx)
		if err != nil {
			continue
		}
		for _, site := range sites {
			out[site.ID] = site.Enabled
		}
	}
	return out
}

func providerIDFor(registry *provider.Registry, protocol string) string {
	var proto string
	switch protocol {
	case store.IndexerTorznab:
		proto = provider.ProtocolTorrent
	case store.IndexerSource:
		proto = provider.ProtocolSource
	}
	if p := registry.ForProtocol(proto); p != nil {
		return p.ID()
	}
	return ""
}

// liveSiteStates resolves every configured entry against the live
// providers — the one source of truth for entry state, shared by the
// Indexers views (SiteService) and the search pipeline (SearchService).
func liveSiteStates(ctx context.Context, st *store.Store, registry *provider.Registry) ([]siteState, error) {
	entries, err := st.ListIndexers(ctx)
	if err != nil {
		return nil, err
	}
	reach := reachability(ctx, registry)
	out := make([]siteState, 0, len(entries))
	for _, ix := range entries {
		out = append(out, siteState{
			providerID: providerIDFor(registry, ix.Protocol),
			entryID:    entryID(ix.ID),
			name:       ix.Name,
			protocol:   protocolOf(ix),
			reachable:  reach[entryID(ix.ID)],
			eff:        ix,
		})
	}
	return out, nil
}

func protocolOf(ix store.Indexer) string {
	if ix.Protocol == store.IndexerTorznab {
		return provider.ProtocolTorrent
	}
	return provider.ProtocolSource
}

func entryID(id int64) string {
	return strconv.FormatInt(id, 10)
}

// summary renders the entry's protocol-specific detail for lists.
func summary(ix store.Indexer) string {
	switch ix.Protocol {
	case store.IndexerTorznab:
		var ts TorznabSettings
		_ = json.Unmarshal(ix.Settings, &ts)
		return hostString(ts.URL)
	case store.IndexerSource:
		var ss SourceSettings
		_ = json.Unmarshal(ix.Settings, &ss)
		return "source " + ss.SourceID
	}
	return ""
}

// List returns all entries for the Indexers page.
func (s *SiteService) List(ctx context.Context) ([]IndexerView, error) {
	entries, err := s.store.ListIndexers(ctx)
	if err != nil {
		return nil, err
	}
	reach := reachability(ctx, s.registry)
	out := make([]IndexerView, 0, len(entries))
	for _, ix := range entries {
		out = append(out, IndexerView{
			Indexer:   ix,
			Reachable: reach[entryID(ix.ID)],
			Summary:   summary(ix),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Protocol != out[j].Protocol {
			return out[i].Protocol < out[j].Protocol
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// Effective returns entries with the series overrides applied + bindings.
func (s *SiteService) Effective(ctx context.Context, seriesID int64) ([]SeriesIndexerView, error) {
	views, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	overrides, err := s.store.ListSiteOverrides(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	bindings, err := s.store.ListBindings(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	bound := map[string][]store.Binding{}
	for _, b := range bindings {
		bound[b.SourceID] = append(bound[b.SourceID], b)
	}

	out := make([]SeriesIndexerView, 0, len(views))
	for _, v := range views {
		var ov *store.SiteOverride
		if o, ok := overrides[v.ID]; ok {
			ov = &o
		}
		v.Indexer = store.EffectiveIndexer(v.Indexer, ov)
		view := SeriesIndexerView{IndexerView: v, Override: ov}
		if v.Protocol == store.IndexerSource {
			view.Bindings = bound[entryID(v.ID)]
		}
		out = append(out, view)
	}
	return out, nil
}
