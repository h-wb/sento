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
}

// --- indexer settings ---

func (s *Server) handleListIndexerSettings(w http.ResponseWriter, r *http.Request) {
	sites, err := s.sites.List(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if sites == nil {
		sites = []service.SiteView{}
	}
	writeJSON(w, http.StatusOK, sites)
}

func (s *Server) handleUpdateIndexerSetting(w http.ResponseWriter, r *http.Request) {
	providerID := chi.URLParam(r, "provider")
	siteID := chi.URLParam(r, "sid")
	current, err := s.store.GlobalSiteSetting(r.Context(), providerID, siteID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var body struct {
		Enabled     *bool `json:"enabled"`
		UseVolumes  *bool `json:"useVolumes"`
		UseChapters *bool `json:"useChapters"`
		Priority    *int  `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
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
	if sites == nil {
		sites = []service.SeriesSiteView{}
	}
	writeJSON(w, http.StatusOK, sites)
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
	var body struct {
		Enabled     *bool `json:"enabled"`
		UseVolumes  *bool `json:"useVolumes"`
		UseChapters *bool `json:"useChapters"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
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
	var body struct {
		Ledger string              `json:"ledger"`
		DryRun bool                `json:"dryRun"`
		Slot   *service.TargetSlot `json:"slot"`
		Grab   string              `json:"grab"` // GUID of one candidate
		Force  bool                `json:"force"`
		Stream bool                `json:"stream"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
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
	writeJSON(w, http.StatusOK, map[string]any{
		"ledger":         result.Ledger,
		"candidates":     result.Candidates,
		"queued":         queued,
		"searched":       result.Searched,
		"dropped":        result.Dropped,
		"providerErrors": result.ProviderErrors,
		"bindingNotes":   result.BindingNotes,
	})
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

// --- suwayomi-specific endpoints (binding fix-ups) ---

func (s *Server) handleListSources(w http.ResponseWriter, r *http.Request) {
	if s.suwa == nil {
		writeError(w, http.StatusServiceUnavailable, "suwayomi is not configured")
		return
	}
	sources, err := s.suwa.ListSources(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sources)
}

func (s *Server) handleSourceSearch(w http.ResponseWriter, r *http.Request) {
	if s.suwa == nil {
		writeError(w, http.StatusServiceUnavailable, "suwayomi is not configured")
		return
	}
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "missing query parameter q")
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	result, err := s.suwa.SearchManga(r.Context(), chi.URLParam(r, "sid"), q, page)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// --- queue ---

func (s *Server) handleQueue(w http.ResponseWriter, r *http.Request) {
	active := r.URL.Query().Get("all") == ""
	items, err := s.store.ListQueueItems(r.Context(), active)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if items == nil {
		items = []store.QueueItem{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleQueueProcess(w http.ResponseWriter, r *http.Request) {
	n, err := s.importer.ProcessQueue(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"processed": n})
}
