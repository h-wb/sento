package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/scheduler"
)

func (s *Server) mountAPI(r chi.Router) {
	r.Use(s.invalidateHealth)
	r.Get("/system/health", s.handleHealth)
	r.Get("/system/status", s.handleStatus)
	r.Get("/system/checks", s.handleHealthChecks)
	r.Post("/system/checks", s.handleRunHealthChecks)
	r.Get("/system/tasks", s.handleListTasks)
	r.Post("/system/tasks/{slug}/run", s.handleRunTask)

	s.mountSeriesAPI(r)
	s.mountEditionsAPI(r)
	s.mountBooksAPI(r)
	s.mountFilesAPI(r)
	s.mountReviewAPI(r)
	s.mountQueueAPI(r)
	s.mountSearchAPI(r)
	s.mountIndexerAPI(r)
	s.mountSettingsAPI(r)
	s.mountProfileAPI(r)
}

// --- scheduler tasks (System → Tasks) ---

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	if s.sched == nil {
		respondList(w, []scheduler.TaskStatus{}, nil)
		return
	}
	respondList(w, s.sched.Tasks(), nil)
}

func (s *Server) handleRunTask(w http.ResponseWriter, r *http.Request) {
	if s.sched == nil {
		writeError(w, http.StatusServiceUnavailable, "no scheduler is running")
		return
	}
	if err := s.sched.Run(chi.URLParam(r, "slug")); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// handleHealthChecks reports the last health check run (System → Health),
// plus scheduled tasks whose last run failed — those are live state, not
// part of the cached report.
func (s *Server) handleHealthChecks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.health.Report(r.Context()))
}

func (s *Server) handleRunHealthChecks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.health.Check(r.Context()))
}

// invalidateHealth marks the health report stale after a change to what
// it checks, so a fixed connection clears its issue without waiting for
// the next scheduled run.
func (s *Server) invalidateHealth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		if r.Method == http.MethodGet {
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/v1")
		for _, prefix := range []string{"/indexers", "/settings/connections", "/extensions"} {
			if strings.HasPrefix(path, prefix) {
				s.health.Invalidate()
				return
			}
		}
	})
}

// --- helpers ---

func pathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid "+name)
		return 0, false
	}
	return id, true
}

// queryID reads an optional numeric query parameter: an absent parameter
// is 0 and ok; ok is false when a present value is not a valid ID.
func queryID(r *http.Request, name string) (int64, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return 0, true
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 0 {
		return 0, false
	}
	return id, true
}

// queryIDs reads an optional comma-separated list of IDs ("1, 2,3"); an
// absent parameter is nil and ok. bad is the first offending part when ok
// is false.
func queryIDs(r *http.Request, name string) (ids []int64, bad string, ok bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return nil, "", true
	}
	for _, part := range strings.Split(raw, ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil {
			return nil, part, false
		}
		ids = append(ids, id)
	}
	return ids, "", true
}

// maxBodyBytes caps request bodies. The largest legitimate one is a
// profile import document; nothing comes near a megabyte.
const maxBodyBytes = 1 << 20

// decodeBody parses the JSON body into v; on failure it writes the 400 and
// returns false so handlers can one-line their decoding.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return false
	}
	return true
}

// respondList writes a list (never null: empty becomes []) or the error.
func respondList[T any](w http.ResponseWriter, list []T, err error) {
	if err != nil {
		writeErr(w, err)
		return
	}
	if list == nil {
		list = []T{}
	}
	writeJSON(w, http.StatusOK, list)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// writeErr maps a service/store error to its HTTP status by apperr kind;
// unclassified errors are 500s. Handlers never inspect error strings.
func writeErr(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, apperr.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, apperr.ErrInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, apperr.ErrConflict):
		status = http.StatusConflict
	case errors.Is(err, apperr.ErrUpstream):
		status = http.StatusBadGateway
	case errors.Is(err, apperr.ErrUnavailable):
		status = http.StatusServiceUnavailable
	}
	writeError(w, status, err.Error())
}
