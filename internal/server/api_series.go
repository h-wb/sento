package server

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/h-wb/sento/internal/scheduler"
	"github.com/h-wb/sento/internal/service"
	"github.com/h-wb/sento/internal/store"
)

// Series and library-wide handlers: the library entries, their in-place
// imports and source bindings, and the library-level views (calendar,
// wanted, progress, bulk import and organize).

// mountSeriesAPI registers the library-wide routes and the /series tree.
func (s *Server) mountSeriesAPI(r chi.Router) {
	r.Get("/calendar", s.handleCalendar)
	r.Get("/wanted", s.handleWanted)
	r.Post("/library/import/scan", s.handleImportScan)
	r.Post("/library/import", s.handleImportFolder)
	r.Get("/library/organize", s.handlePlanOrganizeLibrary)
	r.Post("/library/organize", s.handleOrganizeLibrary)
	r.Get("/library/organize/status", s.handleOrganizeLibraryStatus)
	r.Get("/library/progress", s.handleLibraryProgress)

	r.Route("/series", func(r chi.Router) {
		r.Get("/", s.handleListSeries)
		r.Post("/", s.handleAddSeries)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", s.handleGetSeries)
			r.Put("/", s.handleUpdateSeries)
			r.Delete("/", s.handleDeleteSeries)
			r.Post("/refresh", s.handleRefreshSeries)
			r.Post("/import", s.handleScanSeries)
			r.Get("/bindings", s.handleListBindings)
			r.Post("/bindings", s.handleAddBinding)
			r.Put("/bindings/{bid}", s.handleUpdateBinding)
			r.Delete("/bindings/{bid}", s.handleDeleteBinding)
		})
	})
}

// handleImportScan proposes MangaBaka matches for a root folder's
// subfolders (read-only; §11.4 bulk import).
func (s *Server) handleImportScan(w http.ResponseWriter, r *http.Request) {
	var body ImportScanBody
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Root == "" {
		body.Root = s.cfg.Library.Path
	}
	proposals, err := s.library.ProposeImports(r.Context(), body.Root)
	if err != nil {
		writeErr(w, err)
		return
	}
	respondList(w, proposals, nil)
}

// handleImportFolder confirms one proposal: add the series with the
// folder as its path and link its files in place.
func (s *Server) handleImportFolder(w http.ResponseWriter, r *http.Request) {
	var body ImportFolderBody
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Dir == "" || body.MangaBakaID == 0 {
		writeError(w, http.StatusBadRequest, "dir and mangabakaId are required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 55*time.Second)
	defer cancel()
	id, result, existing, err := s.library.ImportFolder(ctx, s.scanner, body.Dir, body.MangaBakaID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ImportFolderResponse{SeriesID: id, Scan: result, Existing: existing})
}

// --- series ---

func (s *Server) handleListSeries(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListSeries(r.Context())
	respondList(w, list, err)
}

func (s *Server) handleAddSeries(w http.ResponseWriter, r *http.Request) {
	var req service.AddSeriesRequest
	if !decodeBody(w, r, &req) {
		return
	}
	// Metadata fetch (3+ calls, rate-limited, retried when throttled) can be
	// slow; give up with a clean error before the router's timeout does.
	ctx, cancel := context.WithTimeout(r.Context(), 55*time.Second)
	defer cancel()

	id, err := s.library.AddSeries(ctx, req)
	if err != nil {
		writeErr(w, err)
		return
	}
	created, err := s.store.GetSeries(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleGetSeries(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	series, err := s.store.GetSeries(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	collections, err := s.store.ListEditions(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	publishers, err := s.library.SeriesPublishers(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, SeriesDetail{
		Series:     series,
		Editions:   collections,
		Publishers: publishers,
	})
}

func (s *Server) handleUpdateSeries(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var patch SeriesPatch
	if !decodeBody(w, r, &patch) {
		return
	}
	if err := s.library.UpdateSeries(r.Context(), id, patch.Monitored, patch.ProfileID); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDeleteSeries removes a series; ?deleteFiles=true deletes its files
// too, ?removeDownloads=true its torrents and their data. Both opt in.
func (s *Server) handleDeleteSeries(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	q := r.URL.Query()
	opts := service.RemoveOptions{DeleteFiles: q.Get("deleteFiles") == "true", RemoveDownloads: q.Get("removeDownloads") == "true"}
	removed, err := s.files.RemoveSeries(r.Context(), id, opts)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, removed)
}

func (s *Server) handleRefreshSeries(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.library.Refresh(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, StatusMessage{Status: "refreshed"})
}

// handleScanSeries imports existing files in place (spec.md §11.4):
// non-destructive, links what it can identify, keeps the rest as
// unrequested files.
func (s *Server) handleScanSeries(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body ScanBody
	if r.ContentLength > 0 && !decodeBody(w, r, &body) {
		return
	}
	result, err := s.scanner.ScanAndAdoptCutoff(r.Context(), id, body.Path)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// --- library-wide views ---

// handleCalendar lists dated volume releases of monitored editions:
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

// handleWanted lists the library's missing and upgrade-eligible books
// across every monitored series (the Wanted page).
func (s *Server) handleWanted(w http.ResponseWriter, r *http.Request) {
	rows, err := s.library.Wanted(r.Context())
	respondList(w, rows, err)
}

// handleLibraryProgress counts each series' books on disk and wanted, for
// the Library's posters.
func (s *Server) handleLibraryProgress(w http.ResponseWriter, r *http.Request) {
	rows, err := s.library.Progress(r.Context())
	respondList(w, rows, err)
}

// handlePlanOrganizeLibrary previews organizing every series, or those
// in ?series=1,2,3, listing the series it would change.
func (s *Server) handlePlanOrganizeLibrary(w http.ResponseWriter, r *http.Request) {
	ids, bad, ok := queryIDs(r, "series")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid series id: "+bad)
		return
	}
	plans, err := s.files.PlanOrganizeLibrary(r.Context(), ids)
	respondList(w, plans, err)
}

// handleOrganizeLibrary starts organizing the library in the background:
// the run outlives the request and reports through the status endpoint.
func (s *Server) handleOrganizeLibrary(w http.ResponseWriter, r *http.Request) {
	if s.sched == nil {
		writeError(w, http.StatusServiceUnavailable, "no scheduler is running")
		return
	}
	var body service.LibraryOrganizeRequest
	if !decodeBody(w, r, &body) {
		return
	}
	status, err := s.files.QueueLibraryOrganize(body)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.sched.Run(scheduler.OrganizeLibrary); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, status)
}

func (s *Server) handleOrganizeLibraryStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.files.LibraryOrganizeStatus())
}

// --- bindings ---

func (s *Server) handleListBindings(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	bindings, err := s.store.ListBindings(r.Context(), id)
	respondList(w, bindings, err)
}

func (s *Server) handleAddBinding(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var b store.Binding
	if !decodeBody(w, r, &b) {
		return
	}
	b.SeriesID = id
	if b.SourceID == "" || b.SourceMangaKey == "" {
		writeError(w, http.StatusBadRequest, "sourceId and sourceMangaKey are required")
		return
	}
	bid, err := s.store.AddBinding(r.Context(), b)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, IDResponse{ID: bid})
}

func (s *Server) handleUpdateBinding(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	bid, ok := pathID(w, r, "bid")
	if !ok {
		return
	}
	var patch BindingPatch
	if !decodeBody(w, r, &patch) {
		return
	}
	if err := s.library.UpdateBinding(r.Context(), id, bid, patch.TrustChapters, patch.EditionID, patch.State); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteBinding(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	bid, ok := pathID(w, r, "bid")
	if !ok {
		return
	}
	if err := s.store.DeleteBinding(r.Context(), id, bid); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
