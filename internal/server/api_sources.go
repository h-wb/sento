package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/service"
	"github.com/h-wb/sento/internal/store"
)

func (s *Server) mountSourceAPI(r chi.Router) {
	r.Get("/sources", s.handleListSources)
	r.Get("/sources/{sid}/search", s.handleSourceSearch)

	r.Get("/indexers", s.handleListIndexers)
	r.Post("/indexers", s.handleAddIndexer)
	r.Post("/indexers/browse", s.handleBrowseProwlarr)
	r.Put("/indexers/{iid}", s.handleUpdateIndexer)
	r.Patch("/indexers/{iid}", s.handlePatchIndexer)
	r.Delete("/indexers/{iid}", s.handleDeleteIndexer)
	r.Post("/indexers/{iid}/test", s.handleTestIndexer)
	r.Get("/series/{id}/indexers", s.handleSeriesIndexers)
	r.Put("/series/{id}/indexers/{iid}", s.handleSeriesIndexerOverride)

	r.Post("/series/{id}/search", s.handleSearch)
	r.Get("/queue", s.handleQueue)
	r.Post("/queue/process", s.handleQueueProcess)
	r.Post("/queue/{qid}/blocklist", s.handleQueueBlocklist)

	r.Get("/history", s.handleHistory)
	r.Get("/series/{id}/history", s.handleHistory)
	r.Get("/blocklist", s.handleListBlocklist)
	r.Delete("/blocklist/{bid}", s.handleDeleteBlocklist)
}

// --- history & blocklist ---

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	var seriesID int64
	if chi.URLParam(r, "id") != "" {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		seriesID = id
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	decisions, err := s.store.ListDecisions(r.Context(), seriesID, limit)
	respondList(w, decisions, err)
}

func (s *Server) handleListBlocklist(w http.ResponseWriter, r *http.Request) {
	entries, err := s.store.ListBlocklist(r.Context())
	respondList(w, entries, err)
}

func (s *Server) handleDeleteBlocklist(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "bid")
	if !ok {
		return
	}
	if err := s.store.DeleteBlocklist(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleQueueBlocklist manually bans a queue item's release (typically a
// failed import whose release is actually bad) and marks the item failed.
func (s *Server) handleQueueBlocklist(w http.ResponseWriter, r *http.Request) {
	qid, ok := pathID(w, r, "qid")
	if !ok {
		return
	}
	if err := s.importer.BlocklistQueueItem(r.Context(), qid); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	ix := indexerFromBody(body)
	if err := service.ValidateIndexer(ix); err != nil {
		writeErr(w, err)
		return
	}
	id, err := s.store.AddIndexer(r.Context(), ix)
	if err != nil {
		writeErr(w, err)
		return
	}
	ix.ID = id
	writeJSON(w, http.StatusCreated, ix)
}

func (s *Server) handleUpdateIndexer(w http.ResponseWriter, r *http.Request) {
	iid, ok := pathID(w, r, "iid")
	if !ok {
		return
	}
	existing, err := s.store.GetIndexer(r.Context(), iid)
	if err != nil {
		writeErr(w, err)
		return
	}
	var body IndexerBody
	if !decodeBody(w, r, &body) {
		return
	}
	ix := indexerFromBody(body)
	ix.ID = iid
	ix.Protocol = existing.Protocol // protocol is immutable
	if err := service.ValidateIndexer(ix); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.store.UpdateIndexer(r.Context(), ix); err != nil {
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
	ix, err := s.store.GetIndexer(r.Context(), iid)
	if err != nil {
		writeErr(w, err)
		return
	}
	var body IndexerPatch
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Enabled != nil {
		ix.Enabled = *body.Enabled
	}
	if body.UseVolumes != nil {
		ix.UseVolumes = *body.UseVolumes
	}
	if body.UseChapters != nil {
		ix.UseChapters = *body.UseChapters
	}
	if body.Priority != nil {
		ix.Priority = *body.Priority
	}
	if err := s.store.UpdateIndexer(r.Context(), *ix); err != nil {
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

func (s *Server) handleTestIndexer(w http.ResponseWriter, r *http.Request) {
	iid, ok := pathID(w, r, "iid")
	if !ok {
		return
	}
	ix, err := s.store.GetIndexer(r.Context(), iid)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := service.TestIndexer(r.Context(), *ix, s.conns.Host()); err != nil {
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
	indexers, err := service.BrowseProwlarr(r.Context(), body.URL, body.APIKey)
	if err != nil {
		writeErr(w, err)
		return
	}
	cs := s.conns.Load(r.Context())
	if cs.Prowlarr.URL != body.URL || cs.Prowlarr.APIKey != body.APIKey {
		cs.Prowlarr.URL, cs.Prowlarr.APIKey = body.URL, body.APIKey
		_ = s.conns.Save(r.Context(), cs)
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

// --- unified search ---

// handleSearch is the single search endpoint for both ledgers:
// {ledger, dryRun, slot?, grab?, force?, stream?}. stream implies dryRun
// and switches the response to NDJSON SearchEvents, one per provider, so
// fast sites render before slow ones finish.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body SearchBody
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Ledger != service.LedgerVolume && body.Ledger != service.LedgerChapter {
		writeError(w, http.StatusBadRequest, "ledger must be volume or chapter")
		return
	}

	if body.Stream {
		s.streamSearch(w, r, id, body.Ledger, body.Slot)
		return
	}

	result, err := s.search.FindCandidates(r.Context(), id, body.Ledger, body.Slot)
	if err != nil {
		writeErr(w, err)
		return
	}

	var queued []int64
	switch {
	case body.DryRun:
	case body.Grab != "":
		for _, c := range result.Candidates {
			if c.GUID != body.Grab {
				continue
			}
			qid, err := s.search.Grab(r.Context(), id, body.Ledger, c, body.Force)
			if err != nil {
				writeErr(w, err)
				return
			}
			queued = append(queued, qid)
			break
		}
	default:
		queued = s.search.GrabBest(r.Context(), id, body.Ledger, result.Candidates)
	}
	writeJSON(w, http.StatusOK, SearchResponse{SearchResult: *result, Queued: queued})
}

// streamSearch writes NDJSON SearchEvents, flushing after each so the UI
// can render provider batches as they arrive.
func (s *Server) streamSearch(w http.ResponseWriter, r *http.Request, id int64, searchLedger string, slot *service.TargetSlot) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)
	enc := json.NewEncoder(w)
	send := func(v any) {
		_ = enc.Encode(v)
		if flusher != nil {
			flusher.Flush()
		}
	}
	if err := s.search.FindCandidatesStream(r.Context(), id, searchLedger, slot, func(ev service.SearchEvent) {
		send(ev)
	}); err != nil {
		// Headers may already be out; report in-band.
		send(map[string]string{"type": "error", "error": err.Error()})
	}
}

// --- source-host endpoints (binding fix-ups) ---

func (s *Server) handleListSources(w http.ResponseWriter, r *http.Request) {
	host := s.conns.Host()
	if host == nil {
		writeErr(w, apperr.Unavailablef("no source host is configured"))
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
	host := s.conns.Host()
	if host == nil {
		writeErr(w, apperr.Unavailablef("no source host is configured"))
		return
	}
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "missing query parameter q")
		return
	}
	iid, ok := pathID(w, r, "sid")
	if !ok {
		return
	}
	ix, err := s.store.GetIndexer(r.Context(), iid)
	if err != nil {
		writeErr(w, err)
		return
	}
	var ss service.SourceSettings
	if err := json.Unmarshal(ix.Settings, &ss); err != nil || ss.SourceID == "" {
		writeError(w, http.StatusBadRequest, "indexer is not an extension source")
		return
	}
	mangas, err := host.Search(r.Context(), ss.SourceID, q)
	if err != nil {
		writeErr(w, apperr.Upstreamf("%w", err))
		return
	}
	respondList(w, mangas, nil)
}

// --- queue ---

func (s *Server) handleQueue(w http.ResponseWriter, r *http.Request) {
	active := r.URL.Query().Get("all") == ""
	items, err := s.store.ListQueueItems(r.Context(), active)
	respondList(w, items, err)
}

func (s *Server) handleQueueProcess(w http.ResponseWriter, r *http.Request) {
	n, err := s.importer.ProcessQueue(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"processed": n})
}
