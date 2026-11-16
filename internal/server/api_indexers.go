package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/service"
	"github.com/h-wb/sento/internal/store"
)

// Indexer and extension-source handlers: the indexer entries, Prowlarr
// browse, per-series site overrides, and the manual-bind source search.

// mountIndexerAPI registers the source and indexer routes.
func (s *Server) mountIndexerAPI(r chi.Router) {
	r.Get("/sources", s.handleListSources)
	r.Get("/sources/{sid}/search", s.handleSourceSearch)

	r.Get("/indexers", s.handleListIndexers)
	r.Post("/indexers", s.handleAddIndexer)
	r.Post("/indexers/browse", s.handleBrowseProwlarr)
	r.Put("/indexers/{iid}", s.handleUpdateIndexer)
	r.Patch("/indexers/{iid}", s.handlePatchIndexer)
	r.Delete("/indexers/{iid}", s.handleDeleteIndexer)
	r.Post("/indexers/{iid}/test", s.handleTestIndexer)
	r.Post("/indexers/{iid}/search-test", s.handleTestIndexerSearch)
	r.Get("/series/{id}/indexers", s.handleSeriesIndexers)
	r.Put("/series/{id}/indexers/{iid}", s.handleSeriesIndexerOverride)
}

// --- extensions endpoints (binding fix-ups) ---

func (s *Server) handleListSources(w http.ResponseWriter, r *http.Request) {
	host, err := s.conns.RequireHost()
	if err != nil {
		writeErr(w, err)
		return
	}
	sources, err := host.Sources(r.Context())
	if err != nil {
		writeErr(w, apperr.Upstreamf("%w", err))
		return
	}
	writeJSON(w, http.StatusOK, sources)
}

// handleSourceSearch searches one source indexer entry's manga catalog
// (the manual bind UI). {sid} is the indexer entry id.
func (s *Server) handleSourceSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "missing query parameter q")
		return
	}
	iid, ok := pathID(w, r, "sid")
	if !ok {
		return
	}
	mangas, err := s.sites.SearchSource(r.Context(), iid, q)
	if err != nil {
		writeErr(w, err)
		return
	}
	respondList(w, mangas, nil)
}

// --- indexer entries ---

func (s *Server) handleListIndexers(w http.ResponseWriter, r *http.Request) {
	views, err := s.sites.List(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	respondList(w, views, nil)
}

func indexerFromBody(body IndexerBody) store.Indexer {
	return store.Indexer{
		Name: body.Name, Protocol: body.Protocol,
		Enabled: body.Enabled, UseVolumes: body.UseVolumes, UseChapters: body.UseChapters,
		Priority: body.Priority, Settings: store.JSONText(body.Settings),
	}
}

func (s *Server) handleAddIndexer(w http.ResponseWriter, r *http.Request) {
	var body IndexerBody
	if !decodeBody(w, r, &body) {
		return
	}
	ix, err := s.sites.Add(r.Context(), indexerFromBody(body))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, ix)
}

func (s *Server) handleUpdateIndexer(w http.ResponseWriter, r *http.Request) {
	iid, ok := pathID(w, r, "iid")
	if !ok {
		return
	}
	var body IndexerBody
	if !decodeBody(w, r, &body) {
		return
	}
	ix, err := s.sites.Update(r.Context(), iid, indexerFromBody(body))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ix)
}

// handlePatchIndexer toggles entry fields inline (table switches).
func (s *Server) handlePatchIndexer(w http.ResponseWriter, r *http.Request) {
	iid, ok := pathID(w, r, "iid")
	if !ok {
		return
	}
	var body service.IndexerPatch
	if !decodeBody(w, r, &body) {
		return
	}
	ix, err := s.sites.Patch(r.Context(), iid, body)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ix)
}

func (s *Server) handleDeleteIndexer(w http.ResponseWriter, r *http.Request) {
	iid, ok := pathID(w, r, "iid")
	if !ok {
		return
	}
	// Source entries drive the extension lifecycle: deleting the last
	// entry of an extension uninstalls it from the host.
	if err := s.extensions.DeleteIndexerEntry(r.Context(), iid); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTestIndexerSearch(w http.ResponseWriter, r *http.Request) {
	iid, ok := pathID(w, r, "iid")
	if !ok {
		return
	}
	var body IndexerSearchTestBody
	if !decodeBody(w, r, &body) {
		return
	}
	result, err := s.sites.TestSearch(r.Context(), iid, body.Query)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleTestIndexer(w http.ResponseWriter, r *http.Request) {
	iid, ok := pathID(w, r, "iid")
	if !ok {
		return
	}
	if err := s.sites.Test(r.Context(), iid); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleBrowseProwlarr lists a Prowlarr's indexers as ready-to-add
// Torznab endpoints, remembering the credentials for next time.
func (s *Server) handleBrowseProwlarr(w http.ResponseWriter, r *http.Request) {
	var body BrowseProwlarrBody
	if !decodeBody(w, r, &body) {
		return
	}
	indexers, err := s.conns.BrowseProwlarr(r.Context(), body.URL, body.APIKey)
	if err != nil {
		writeErr(w, err)
		return
	}
	respondList(w, indexers, nil)
}

func (s *Server) handleSeriesIndexers(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	sites, err := s.sites.Effective(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	respondList(w, sites, nil)
}

// handleSeriesIndexerOverride sets/clears per-series override fields.
// All-null body = clear override (inherit globals).
func (s *Server) handleSeriesIndexerOverride(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	iid, ok := pathID(w, r, "iid")
	if !ok {
		return
	}
	ov := store.SiteOverride{SeriesID: id, IndexerID: iid}
	var body OverridePatch
	if !decodeBody(w, r, &body) {
		return
	}
	ov.Enabled = body.Enabled
	ov.UseVolumes = body.UseVolumes
	ov.UseChapters = body.UseChapters
	if err := s.store.UpsertSiteOverride(r.Context(), ov); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ov)
}
