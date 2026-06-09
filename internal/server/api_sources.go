package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/h-wb/sento/internal/ledger"
	"github.com/h-wb/sento/internal/store"
)

func (s *Server) mountSourceAPI(r chi.Router) {
	r.Get("/sources", s.handleListSources)
	r.Get("/sources/{sid}/search", s.handleSourceSearch)
	r.Post("/series/{id}/search/chapters", s.handleChapterSearch)
	r.Post("/series/{id}/search/volumes", s.handleVolumeSearch)
	r.Get("/queue", s.handleQueue)
	r.Post("/queue/process", s.handleQueueProcess)
}

// handleVolumeSearch runs the Prowlarr funnel; unless dryRun, the best
// eligible candidates are grabbed greedily by slot coverage
// (decision-engine.md §4 selection).
func (s *Server) handleVolumeSearch(w http.ResponseWriter, r *http.Request) {
	if s.volumes == nil {
		writeError(w, http.StatusServiceUnavailable, "prowlarr/qbittorrent are not configured")
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		DryRun bool   `json:"dryRun"`
		Grab   string `json:"grab"` // GUID: force-grab one candidate
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
	}
	candidates, err := s.volumes.FindVolumeCandidates(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	var queued []int64
	if !body.DryRun {
		remaining := map[int64]bool{}
		for _, c := range candidates {
			if body.Grab != "" && c.GUID != body.Grab {
				continue
			}
			if len(c.Rejections) > 0 {
				continue
			}
			// Greedy: skip candidates whose covered slots are all taken.
			news := false
			for _, n := range c.Covered {
				if !remaining[n] {
					news = true
					break
				}
			}
			if !news && len(queued) > 0 {
				continue
			}
			qid, err := s.volumes.GrabVolume(r.Context(), id, c)
			if err != nil {
				continue
			}
			queued = append(queued, qid)
			for _, n := range c.Covered {
				remaining[n] = true
			}
			if body.Grab != "" {
				break
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"candidates": candidates, "queued": queued})
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
	if s.grabber == nil {
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
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
	}

	wanted, err := s.wantedChapterSlots(r, id, body.Slots)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if len(wanted) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"candidates": []any{}, "queued": []any{}})
		return
	}

	candidates, err := s.grabber.FindChapterCandidates(r.Context(), id, wanted)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
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
	total := 0
	if s.grabber != nil {
		n, err := s.grabber.ProcessQueue(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		total += n
	}
	if s.volumes != nil {
		n, err := s.volumes.ProcessTorrentQueue(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		total += n
	}
	writeJSON(w, http.StatusOK, map[string]int{"processed": total})
}
