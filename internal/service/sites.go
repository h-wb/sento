package service

import (
	"context"
	"sort"

	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

// SiteService backs the Indexers configuration UI: a uniform list of every
// site across providers with global settings and per-series overrides.
type SiteService struct {
	store    *store.Store
	registry *provider.Registry
}

func NewSiteService(st *store.Store, registry *provider.Registry) *SiteService {
	return &SiteService{store: st, registry: registry}
}

// SiteView is one site with its global configuration.
type SiteView struct {
	Provider    string `json:"provider"` // owning provider id (torrent/suwayomi)
	SiteID      string `json:"siteId"`
	Name        string `json:"name"`
	Protocol    string `json:"protocol"`
	Lang        string `json:"lang,omitempty"`
	Reachable   bool   `json:"reachable"` // enabled on the remote side
	Enabled     bool   `json:"enabled"`
	UseVolumes  bool   `json:"useVolumes"`
	UseChapters bool   `json:"useChapters"`
	Priority    int    `json:"priority"`
}

// SeriesSiteView adds per-series override state and the binding set (a
// site can bind several manga entries; the funnel picks per slot).
type SeriesSiteView struct {
	SiteView `tstype:",extends"`
	Override *store.SiteOverride `json:"override,omitempty"`
	Bindings []store.Binding     `json:"bindings,omitempty"`
}

func (s *SiteService) liveSites(ctx context.Context) ([]siteState, error) {
	globals, err := s.store.ListSiteSettings(ctx)
	if err != nil {
		return nil, err
	}
	var out []siteState
	for _, p := range s.registry.All() {
		sites, err := p.Sites(ctx)
		if err != nil {
			return nil, err
		}
		for _, site := range sites {
			key := [2]string{p.ID(), site.ID}
			global, ok := globals[key]
			if !ok {
				global = store.SiteSetting{
					Provider: key[0], SiteID: key[1],
					Enabled: true, UseVolumes: true, UseChapters: true,
				}
			}
			out = append(out, siteState{providerID: p.ID(), site: site, setting: global})
		}
	}
	return out, nil
}

// List returns all sites with their global configuration.
func (s *SiteService) List(ctx context.Context) ([]SiteView, error) {
	states, err := s.liveSites(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]SiteView, 0, len(states))
	for _, st := range states {
		out = append(out, SiteView{
			Provider: st.providerID, SiteID: st.site.ID, Name: st.site.Name,
			Protocol: st.site.Protocol, Lang: st.site.Lang, Reachable: st.site.Enabled,
			Enabled: st.setting.Enabled, UseVolumes: st.setting.UseVolumes,
			UseChapters: st.setting.UseChapters, Priority: st.setting.Priority,
		})
	}
	sortSites(out)
	return out, nil
}

// Effective returns sites with the series overrides applied + bindings.
func (s *SiteService) Effective(ctx context.Context, seriesID int64) ([]SeriesSiteView, error) {
	states, err := s.liveSites(ctx)
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

	out := make([]SeriesSiteView, 0, len(states))
	for _, st := range states {
		key := [2]string{st.providerID, st.site.ID}
		var ov *store.SiteOverride
		if o, ok := overrides[key]; ok {
			ov = &o
		}
		eff := store.EffectiveSiteSetting(st.setting, ov)
		view := SeriesSiteView{
			SiteView: SiteView{
				Provider: st.providerID, SiteID: st.site.ID, Name: st.site.Name,
				Protocol: st.site.Protocol, Lang: st.site.Lang, Reachable: st.site.Enabled,
				Enabled: eff.Enabled, UseVolumes: eff.UseVolumes,
				UseChapters: eff.UseChapters, Priority: eff.Priority,
			},
			Override: ov,
		}
		if st.providerID == "suwayomi" {
			view.Bindings = bound[st.site.ID]
		}
		out = append(out, view)
	}
	sortSeriesSites(out)
	return out, nil
}

func sortSites(sites []SiteView) {
	sort.Slice(sites, func(i, j int) bool {
		if sites[i].Provider != sites[j].Provider {
			return sites[i].Provider < sites[j].Provider
		}
		return sites[i].Name < sites[j].Name
	})
}

func sortSeriesSites(sites []SeriesSiteView) {
	sort.Slice(sites, func(i, j int) bool {
		if sites[i].Provider != sites[j].Provider {
			return sites[i].Provider < sites[j].Provider
		}
		return sites[i].Name < sites[j].Name
	})
}
