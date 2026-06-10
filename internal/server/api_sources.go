package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/h-wb/sento/internal/ledger"
	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/service"
	"github.com/h-wb/sento/internal/store"
)

func (s *Server) mountSourceAPI(r chi.Router) {
	r.Get("/sources", s.handleListSources)
	r.Get("/indexers", s.handleListIndexers)
	r.Get("/sources/{sid}/search", s.handleSourceSearch)
	r.Post("/series/{id}/search/chapters", s.handleChapterSearch)
	r.Post("/series/{id}/search/volumes", s.handleVolumeSearch)
	r.Post("/series/{id}/grab/chapter", s.handleGrabChapter)
	r.Get("/queue", s.handleQueue)
	r.Post("/queue/process", s.handleQueueProcess)
}

// handleVolumeSearch runs the Prowlarr funnel; unless dryRun, the best
// eligible candidates are grabbed greedily by slot coverage
// (decision-engine.md §4 selection).
func (s *Server) handleVolumeSearch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		DryRun bool                `json:"dryRun"`
		Grab   string              `json:"grab"`  // GUID: grab one candidate
		Force  bool                `json:"force"` // bypass soft rejections
		Slot   *service.TargetSlot `json:"slot"`  // interactive: target one slot
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
	}
	candidates, err := s.volumes.FindVolumeCandidates(r.Context(), id, body.Slot)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if candidates == nil {
		candidates = []service.VolumeCandidate{}
	}

	var queued []int64
	switch {
	case body.DryRun:
	case body.Grab != "":
		for _, c := range candidates {
			if c.GUID != body.Grab {
				continue
			}
			qid, err := s.volumes.GrabVolume(r.Context(), id, c, body.Force)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			queued = append(queued, qid)
			break
		}
	default:
		queued = s.volumes.GrabBest(r.Context(), id, candidates)
	}
	writeJSON(w, http.StatusOK, map[string]any{"candidates": candidates, "queued": queued})
}

// handleListIndexers returns every search site Sento can use, across all
// providers, in one uniform list.
func (s *Server) handleListIndexers(w http.ResponseWriter, r *http.Request) {
	out := []provider.Site{}
	for _, p := range s.registry.All() {
		sites, err := p.Sites(r.Context())
		if err != nil {
			writeError(w, http.StatusBadGateway, p.ID()+": "+err.Error())
			return
		}
		out = append(out, sites...)
	}
	writeJSON(w, http.StatusOK, out)
}

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

// handleChapterSearch finds candidates for wanted chapter slots and, unless
// dryRun, grabs them.
func (s *Server) handleChapterSearch(w http.ResponseWriter, r *http.Request) {
	if !s.grabber.HasSourceProvider() {
		writeError(w, http.StatusServiceUnavailable, "suwayomi is not configured")
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		DryRun bool    `json:"dryRun"`
		Slots  []int64 `json:"slots"` // hundredths; empty = all wanted
		All    bool    `json:"all"`   // interactive: every matching candidate
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
	}

	var wanted map[int64]bool
	var err error
	if body.All && len(body.Slots) > 0 {
		// Interactive: explicit slots are searchable regardless of
		// wanted/satisfied/pending state.
		wanted = map[int64]bool{}
		for _, n := range body.Slots {
			wanted[n] = true
		}
	} else {
		wanted, err = s.wantedChapterSlots(r, id, body.Slots)
		if err != nil {
			writeStoreError(w, err)
			return
		}
	}
	if len(wanted) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"candidates": []any{}, "queued": []any{}})
		return
	}

	candidates, err := s.grabber.FindChapterCandidates(r.Context(), id, wanted, body.All)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if candidates == nil {
		candidates = []service.ChapterCandidate{}
	}
	resp := map[string]any{"candidates": candidates, "queued": []int64{}}
	if !body.DryRun && len(candidates) > 0 {
		queued, err := s.grabber.GrabChapters(r.Context(), id, candidates)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		resp["queued"] = queued
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleGrabChapter grabs one specific candidate from an interactive
// chapter search.
func (s *Server) handleGrabChapter(w http.ResponseWriter, r *http.Request) {
	if !s.grabber.HasSourceProvider() {
		writeError(w, http.StatusServiceUnavailable, "suwayomi is not configured")
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var c service.ChapterCandidate
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil || c.ChapterID == 0 || c.SlotNumber == 0 {
		writeError(w, http.StatusBadRequest, "chapterId and slotNumber are required")
		return
	}
	queued, err := s.grabber.GrabChapters(r.Context(), id, []service.ChapterCandidate{c})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"queued": queued})
}

// wantedChapterSlots computes the wanted chapter slot set, optionally
// restricted to an explicit list, excluding pending queue items.
func (s *Server) wantedChapterSlots(r *http.Request, seriesID int64, restrict []int64) (map[int64]bool, error) {
	series, err := s.store.GetSeries(r.Context(), seriesID)
	if err != nil {
		return nil, err
	}
	cl, err := s.store.GetChapterLedger(r.Context(), seriesID)
	if err != nil {
		return nil, err
	}
	links, err := s.store.SlotLinks(r.Context(), seriesID)
	if err != nil {
		return nil, err
	}
	pending, err := s.store.PendingSlotKeys(r.Context(), seriesID)
	if err != nil {
		return nil, err
	}

	allowed := map[int64]bool{}
	for _, n := range restrict {
		allowed[n] = true
	}

	wanted := map[int64]bool{}
	for _, slot := range ledger.BuildChapterSlots(*cl, series.TotalChapters, links, nil) {
		if !slot.Wanted || pending[store.SlotKey("chapter", 0, slot.Number)] {
			continue
		}
		if len(allowed) > 0 && !allowed[slot.Number] {
			continue
		}
		wanted[slot.Number] = true
	}
	return wanted, nil
}

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
