package server

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
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
	if err := s.importer.UnblockReleases(r.Context(), body.IDs, body.Protocol, body.Keys); err != nil {
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
	items, err := s.importer.QueueView(r.Context(), active)
	respondList(w, items, err)
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
	writeJSON(w, http.StatusOK, data)
}
