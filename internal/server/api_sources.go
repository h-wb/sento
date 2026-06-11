package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/h-wb/sento/internal/service"
	"github.com/h-wb/sento/internal/store"
)

func (s *Server) mountSourceAPI(r chi.Router) {
	r.Get("/sources", s.handleListSources)
	r.Get("/sources/{sid}/search", s.handleSourceSearch)

	r.Get("/settings/indexers", s.handleListIndexerSettings)
	r.Put("/settings/indexers/{provider}/{sid}", s.handleUpdateIndexerSetting)
	r.Get("/series/{id}/indexers", s.handleSeriesIndexers)
	r.Put("/series/{id}/indexers/{provider}/{sid}", s.handleSeriesIndexerOverride)

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
		writeStoreError(w, err)
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
	items, err := s.store.ListQueueItems(r.Context(), false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, item := range items {
		if item.ID != qid {
			continue
		}
		if err := s.store.AddBlocklist(r.Context(), item.Protocol, item.ExternalID, "blocked manually"); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		_ = s.store.UpdateQueueItem(r.Context(), item.ID, store.QueueFailed, "blocked manually")
		_ = s.store.AddDecision(r.Context(), item.SeriesID, store.DecisionBody{
			Event: "blocklisted", Title: item.Title, Key: item.ExternalID,
		})
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeError(w, http.StatusNotFound, "queue item not found")
}

// --- indexer settings ---

func (s *Server) handleListIndexerSettings(w http.ResponseWriter, r *http.Request) {
	sites, err := s.sites.List(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondList(w, sites, nil)
}

func (s *Server) handleUpdateIndexerSetting(w http.ResponseWriter, r *http.Request) {
	providerID := chi.URLParam(r, "provider")
	siteID := chi.URLParam(r, "sid")
	current, err := s.store.GlobalSiteSetting(r.Context(), providerID, siteID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var body IndexerSettingPatch
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Enabled != nil {
		current.Enabled = *body.Enabled
	}
	if body.UseVolumes != nil {
		current.UseVolumes = *body.UseVolumes
	}
	if body.UseChapters != nil {
		current.UseChapters = *body.UseChapters
	}
	if body.Priority != nil {
		current.Priority = *body.Priority
	}
	if err := s.store.UpsertSiteSetting(r.Context(), current); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, current)
}

func (s *Server) handleSeriesIndexers(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	sites, err := s.sites.Effective(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
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
	ov := store.SiteOverride{
		SeriesID: id,
		Provider: chi.URLParam(r, "provider"),
		SiteID:   chi.URLParam(r, "sid"),
	}
	var body OverridePatch
	if !decodeBody(w, r, &body) {
		return
	}
	ov.Enabled = body.Enabled
	ov.UseVolumes = body.UseVolumes
	ov.UseChapters = body.UseChapters
	if err := s.store.UpsertSiteOverride(r.Context(), ov); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeError(w, http.StatusBadGateway, err.Error())
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
				writeError(w, http.StatusBadRequest, err.Error())
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
		writeError(w, http.StatusServiceUnavailable, "no source host is configured")
		return
	}
	sources, err := host.Sources(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sources)
}

func (s *Server) handleSourceSearch(w http.ResponseWriter, r *http.Request) {
	host := s.conns.Host()
	if host == nil {
		writeError(w, http.StatusServiceUnavailable, "no source host is configured")
		return
	}
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "missing query parameter q")
		return
	}
	mangas, err := host.Search(r.Context(), chi.URLParam(r, "sid"), q)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
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
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"processed": n})
}
