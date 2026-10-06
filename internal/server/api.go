package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/scheduler"
	"github.com/h-wb/sento/internal/service"
)

func (s *Server) mountAPI(r chi.Router) {
	r.Use(s.invalidateHealth)
	r.Get("/system/health", s.handleHealth)
	r.Get("/system/status", s.handleStatus)
	r.Get("/system/checks", s.handleHealthChecks)
	r.Post("/system/checks", s.handleRunHealthChecks)
	r.Get("/system/tasks", s.handleListTasks)
	r.Post("/system/tasks/{slug}/run", s.handleRunTask)

	r.Get("/lookup", s.handleLookup)
	r.Get("/calendar", s.handleCalendar)
	r.Get("/review", s.handleReview)
	r.Delete("/files/{fid}", s.handleDeleteFile)
	r.Post("/library/import/scan", s.handleImportScan)
	r.Post("/library/import", s.handleImportFolder)
	r.Get("/settings/media", s.handleGetMediaSettings)
	r.Put("/settings/media", s.handlePutMediaSettings)
	r.Post("/settings/media/preview", s.handlePreviewMediaSettings)
	r.Get("/settings/connections", s.handleGetConnections)
	r.Put("/settings/connections", s.handlePutConnections)
	r.Post("/settings/connections/test", s.handleTestConnections)
	r.Get("/extensions", s.handleListExtensions)
	r.Get("/extensions/{pkg}/sources", s.handleExtensionSources)
	r.Get("/extensions/repos", s.handleGetExtensionRepos)
	r.Put("/extensions/repos", s.handleSetExtensionRepos)
	r.Post("/extensions/{pkg}", s.handleInstallExtension)
	r.Get("/extensions/sources/{sid}/preferences", s.handleSourcePreferences)
	r.Put("/extensions/sources/{sid}/preferences", s.handleSetSourcePreference)
	s.mountSourceAPI(r)
	s.mountProfileAPI(r)

	r.Route("/series", func(r chi.Router) {
		r.Get("/", s.handleListSeries)
		r.Post("/", s.handleAddSeries)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", s.handleGetSeries)
			r.Put("/", s.handleUpdateSeries)
			r.Delete("/", s.handleDeleteSeries)
			r.Post("/refresh", s.handleRefreshSeries)
			r.Get("/slots", s.handleSlots)
			r.Get("/targets", s.handleListTargets)
			r.Post("/targets", s.handleSaveTarget)
			r.Put("/targets/{tid}", s.handleSaveTarget)
			r.Delete("/targets/{tid}", s.handleDeleteTarget)
			r.Get("/target-slots", s.handleTargetSlots)
			r.Get("/edition-overview", s.handleEditionOverview)
			r.Get("/targets/{tid}/books", s.handleTargetBooks)
			r.Post("/targets/{tid}/monitor", s.handleBulkMonitor)
			r.Post("/units", s.handleSaveUnit)
			r.Put("/units/{uid}", s.handleSaveUnit)
			r.Delete("/units/{uid}", s.handleDeleteUnit)
			r.Post("/units/{uid}/catalog-reset", s.handleResetUnit)
			r.Put("/collections/{cid}/metadata", s.handleEditCollectionMetadata)
			r.Post("/collections/{cid}/catalog-link", s.handleCatalogLink)
			r.Get("/coverage", s.handleCoverage)
			r.Put("/coverage/pin", s.handlePinCoverage)
			r.Put("/unit-monitor", s.handleUnitMonitor)
			r.Put("/files/{fid}/mapping", s.handleRemapFile)
			r.Patch("/files/{fid}/facts", s.handleFileFacts)
			r.Post("/check-files", s.handleCheckFiles)
			r.Get("/observations", s.handleObservations)
			r.Post("/observations", s.handleObservations)
			r.Put("/observations/{oid}", s.handleResolveObservation)
			r.Get("/relations", s.handleRelations)
			r.Post("/relations", s.handleRelations)
			r.Delete("/relations", s.handleRelations)
			r.Post("/collections", s.handleCreateCollection)
			r.Put("/collections/{cid}", s.handleUpdateCollection)
			r.Post("/collections/{cid}/merge", s.handleMergeCollection)
			r.Delete("/collections/{cid}", s.handleDeleteCollection)
			r.Get("/files", s.handleListFiles)
			r.Get("/files/organize", s.handlePlanOrganize)
			r.Post("/files/organize", s.handleOrganize)
			r.Post("/import", s.handleScanSeries)
			r.Get("/bindings", s.handleListBindings)
			r.Post("/bindings", s.handleAddBinding)
			r.Put("/bindings/{bid}", s.handleUpdateBinding)
			r.Delete("/bindings/{bid}", s.handleDeleteBinding)
		})
	})
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
	writeJSON(w, http.StatusOK, s.withTaskFailures(s.health.Report(r.Context())))
}

func (s *Server) handleRunHealthChecks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.withTaskFailures(s.health.Check(r.Context())))
}

func (s *Server) withTaskFailures(report service.HealthReport) service.HealthReport {
	if s.sched == nil {
		return report
	}
	issues := append([]service.HealthIssue{}, report.Issues...)
	for _, t := range s.sched.Tasks() {
		if t.LastError != "" {
			issues = append(issues, service.HealthIssue{
				Source: "task", Level: service.HealthWarning, Link: "/system",
				Message: fmt.Sprintf("Scheduled task %s failed on its last run", t.Name), Detail: t.LastError,
			})
		}
	}
	report.Issues = issues
	return report
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

// handleCalendar lists dated volume releases of monitored collections:
// two weeks back, `days` ahead (default 60).
func (s *Server) handleCalendar(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 {
		days = 60
	}
	now := time.Now()
	from := now.AddDate(0, 0, -14).Format("2006-01-02")
	to := now.AddDate(0, 0, days).Format("2006-01-02")
	entries, err := s.library.Calendar(r.Context(), from, to)
	respondList(w, entries, err)
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
