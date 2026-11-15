package server

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/service"
	"github.com/h-wb/sento/internal/store"
)

// Queue, download history, activity and blocklist handlers.

// mountQueueAPI registers the queue, history, activity and blocklist
// routes.
func (s *Server) mountQueueAPI(r chi.Router) {
	r.Get("/queue", s.handleQueue)
	r.Post("/queue/process", s.handleQueueProcess)
	r.Post("/queue/{qid}/retry", s.handleQueueRetry)
	r.Post("/queue/{qid}/cancel", s.handleQueueCancel)
	r.Post("/queue/{qid}/blocklist", s.handleQueueBlocklist)
	r.Post("/queue/another", s.handleQueueAnother)
	r.Get("/activity", s.handleActivity)
	r.Get("/history", s.handleHistory)
	r.Get("/series/{id}/history", s.handleHistory)
	r.Get("/blocklist", s.handleListBlocklist)
	r.Delete("/blocklist", s.handleClearBlocklist)
	r.Post("/blocklist/unblock", s.handleUnblockReleases)
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
	entries, err := s.store.ListBlockedReleases(r.Context())
	respondList(w, entries, err)
}

func (s *Server) handleClearBlocklist(w http.ResponseWriter, r *http.Request) {
	if err := s.store.ClearBlocklist(r.Context()); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUnblockReleases(w http.ResponseWriter, r *http.Request) {
	var body UnblockBody
	if !decodeBody(w, r, &body) {
		return
	}
	byID := len(body.IDs) > 0 && body.Protocol == "" && len(body.Keys) == 0
	byKey := len(body.IDs) == 0 && (body.Protocol == provider.ProtocolSource || body.Protocol == provider.ProtocolTorrent) && len(body.Keys) > 0
	if !byID && !byKey {
		writeError(w, http.StatusBadRequest, "select blocklist IDs or release keys with a protocol")
		return
	}
	for _, id := range body.IDs {
		if id <= 0 {
			writeError(w, http.StatusBadRequest, "blocklist IDs must be positive")
			return
		}
	}
	for _, key := range body.Keys {
		if key == "" {
			writeError(w, http.StatusBadRequest, "release keys must not be empty")
			return
		}
	}
	if err := s.store.UnblockReleases(r.Context(), body.IDs, body.Protocol, body.Keys); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

// --- queue ---

func (s *Server) handleQueue(w http.ResponseWriter, r *http.Request) {
	active := r.URL.Query().Get("all") == ""
	items, err := s.store.ListQueueItems(r.Context(), active)
	if err != nil {
		respondList(w, items, err)
		return
	}
	files := map[int64][]store.File{}
	for i := range items {
		item := &items[i]
		if item.Status != store.QueueFailed && item.Status != store.QueueNeedsReview && item.Status != store.QueueRetryWait {
			continue
		}
		if _, ok := files[item.SeriesID]; !ok {
			files[item.SeriesID], err = s.store.ListFiles(r.Context(), item.SeriesID)
			if err != nil {
				writeErr(w, err)
				return
			}
		}
		item.CanTryAnother = service.CanTryAnother(*item, files[item.SeriesID])
	}
	for i := range items {
		s.enrichQueueProgress(&items[i])
	}
	respondList(w, items, nil)
}

func (s *Server) handleQueueProcess(w http.ResponseWriter, r *http.Request) {
	n, err := s.importer.ProcessQueue(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, QueueProcessResponse{Processed: n})
}

func (s *Server) handleQueueRetry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "qid")
	if !ok {
		return
	}
	var body QueueRetryBody
	if !decodeBody(w, r, &body) {
		return
	}
	if err := s.store.ResumeQueue(r.Context(), id, body.RemoteID); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) handleQueueCancel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "qid")
	if !ok {
		return
	}
	if err := s.store.CancelQueue(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleQueueAnother is "Try another release" for one job or several:
// their releases are blocklisted and their books freed, then each series
// is searched again right away, Sento choosing, instead of at the next
// sweep. It answers once the jobs are given up; the searches run on.
func (s *Server) handleQueueAnother(w http.ResponseWriter, r *http.Request) {
	var body QueueAnotherBody
	if !decodeBody(w, r, &body) {
		return
	}
	if len(body.IDs) == 0 || len(body.IDs) > 500 {
		writeError(w, http.StatusBadRequest, "choose between 1 and 500 downloads")
		return
	}
	targets, err := s.importer.TryAnother(r.Context(), body.IDs)
	if err != nil {
		writeErr(w, err)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		s.search.SearchAgain(ctx, targets)
	}()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) enrichQueueProgress(item *store.QueueItem) {
	observed, _ := time.Parse(time.RFC3339, item.ObservedAt)
	if time.Since(observed) > 5*time.Minute {
		item.Stalled = false
	}
	if item.Status != store.QueueDownloading {
		return
	}
	if item.ObservedAt != "" {
		item.Progress = &item.DownloadProgress
	}
	prov := s.registry.Get(item.ProviderID)
	if pr, ok := prov.(provider.Progresser); ok {
		if frac, ok := pr.Progress(item.ExternalID); ok {
			item.Progress = &frac
		}
	}
}

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	sid, ok := queryID(r, "seriesId")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid series ID")
		return
	}
	data, err := s.library.Activity(r.Context(), sid)
	if err != nil {
		writeErr(w, err)
		return
	}
	for i := range data.Downloads {
		s.enrichQueueProgress(&data.Downloads[i].Job)
	}
	writeJSON(w, http.StatusOK, data)
}
