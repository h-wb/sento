package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/service"
)

// Unified search handlers: the metadata lookup and the per-series search
// with its NDJSON stream.

// mountSearchAPI registers the lookup and search routes.
func (s *Server) mountSearchAPI(r chi.Router) {
	r.Get("/lookup", s.handleLookup)
	r.Post("/series/{id}/search", s.handleSearch)
}

// --- lookup ---

func (s *Server) handleLookup(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "missing query parameter q")
		return
	}
	refs, err := s.provider.Search(r.Context(), q)
	if err != nil {
		writeErr(w, apperr.Upstreamf("lookup: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, refs)
}

// --- unified search ---

// handleSearch is the single search endpoint for both ledgers:
// {ledger, dryRun, book?, grab?, force?, stream?}. stream implies dryRun
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
		s.streamSearch(w, r, id, body.Ledger, body.Scope)
		return
	}

	result, err := s.search.FindCandidates(r.Context(), id, body.Ledger, body.Scope)
	if err != nil {
		writeErr(w, err)
		return
	}

	var queued []int64
	var outcomes []service.PlanOutcome
	switch {
	case body.DryRun:
	case body.Picks != nil:
		if len(body.Picks) == 0 || len(body.Picks) > 200 {
			writeError(w, http.StatusBadRequest, "select between 1 and 200 releases")
			return
		}
		outcomes = s.search.SubmitPlan(r.Context(), id, body.Ledger, body.Scope, result.Candidates, body.Picks)
		for _, outcome := range outcomes {
			if outcome.QueueID > 0 {
				queued = append(queued, outcome.QueueID)
			}
		}
	case body.Grab != "":
		matches := []service.ReleaseCandidate{}
		for _, c := range result.Candidates {
			if c.GUID == body.Grab && (body.GrabSiteID == "" || body.GrabSiteID == c.SiteID) && (body.GrabProvider == "" || body.GrabProvider == c.Provider) && (body.GrabVersionID == 0 || body.GrabVersionID == c.VersionID) {
				matches = append(matches, c)
			}
		}
		if len(matches) == 0 {
			writeErr(w, apperr.NotFoundf("release is no longer available for this selection; search again"))
			return
		}
		if len(matches) > 1 {
			writeErr(w, apperr.Conflictf("several releases share this identifier; select a specific provider and copy"))
			return
		}
		selected := matches[0]
		selected.RequestedReplacements = body.RequestedReplacements
		qid, err := s.search.Grab(r.Context(), id, body.Ledger, selected, body.Force)
		if err != nil {
			writeErr(w, err)
			return
		}
		queued = append(queued, qid)

	default:
		queued = s.search.GrabBest(r.Context(), id, body.Ledger, result.Candidates)
	}
	if queued == nil {
		queued = []int64{}
	}
	writeJSON(w, http.StatusOK, SearchResponse{SearchResult: *result, Queued: queued, Outcomes: outcomes})
}

// streamSearch writes NDJSON SearchEvents, flushing after each so the UI
// can render provider batches as they arrive.
func (s *Server) streamSearch(w http.ResponseWriter, r *http.Request, id int64, searchLedger string, slot *service.SearchScope) {
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
