package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/h-wb/sento/internal/ledger"
	"github.com/h-wb/sento/internal/service"
	"github.com/h-wb/sento/internal/store"
)

func (s *Server) mountAPI(r chi.Router) {
	r.Get("/system/health", s.handleHealth)
	r.Get("/system/status", s.handleStatus)

	r.Get("/lookup", s.handleLookup)
	r.Get("/calendar", s.handleCalendar)
	r.Delete("/files/{fid}", s.handleDeleteFile)
	s.mountSourceAPI(r)

	r.Route("/series", func(r chi.Router) {
		r.Get("/", s.handleListSeries)
		r.Post("/", s.handleAddSeries)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", s.handleGetSeries)
			r.Delete("/", s.handleDeleteSeries)
			r.Post("/refresh", s.handleRefreshSeries)
			r.Get("/slots", s.handleSlots)
			r.Put("/collections/{cid}", s.handleUpdateCollection)
			r.Put("/chapter-ledger", s.handleUpdateChapterLedger)
			r.Get("/files", s.handleListFiles)
			r.Post("/import", s.handleScanSeries)
			r.Get("/bindings", s.handleListBindings)
			r.Post("/bindings", s.handleAddBinding)
			r.Delete("/bindings/{bid}", s.handleDeleteBinding)
		})
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
		writeError(w, http.StatusBadGateway, err.Error())
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
	entries, err := s.store.Calendar(r.Context(), from, to)
	respondList(w, entries, err)
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
	if req.MangaBakaID == 0 {
		writeError(w, http.StatusBadRequest, "mangabakaId is required")
		return
	}
	// Metadata fetch (3+ calls, rate-limited) can be slow; don't tie it to
	// the handler's 60s middleware timeout budget more than needed.
	ctx, cancel := context.WithTimeout(r.Context(), 55*time.Second)
	defer cancel()

	id, err := s.library.AddSeries(ctx, req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	created, err := s.store.GetSeries(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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
		writeStoreError(w, err)
		return
	}
	collections, err := s.store.ListCollections(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	chapterLedger, err := s.store.GetChapterLedger(r.Context(), id)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, SeriesDetail{
		Series:        series,
		Collections:   collections,
		ChapterLedger: chapterLedger,
	})
}

func (s *Server) handleDeleteSeries(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.store.DeleteSeries(r.Context(), id); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRefreshSeries(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.library.Refresh(r.Context(), id); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "refreshed"})
}

// --- slots ---

func (s *Server) handleSlots(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	series, err := s.store.GetSeries(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	links, err := s.store.SlotLinks(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	collections, err := s.store.ListCollections(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := SlotsResponse{Volumes: map[string][]ledger.VolumeSlot{}}
	for _, col := range collections {
		works, err := s.store.ListWorks(r.Context(), col.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out.Volumes[strconv.FormatInt(col.ID, 10)] = ledger.BuildVolumeSlots(col, works, links, nil, time.Now())
	}
	if cl, err := s.store.GetChapterLedger(r.Context(), id); err == nil {
		out.Chapters = ledger.BuildChapterSlots(*cl, series.TotalChapters, links, nil)
	}
	writeJSON(w, http.StatusOK, out)
}

// --- collection / chapter ledger updates ---

func (s *Server) handleUpdateCollection(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	cid, ok := pathID(w, r, "cid")
	if !ok {
		return
	}
	var body CollectionPatch
	if !decodeBody(w, r, &body) {
		return
	}
	if err := s.store.SetCollectionMonitored(r.Context(), id, cid, body.Monitored); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": cid, "monitored": body.Monitored})
}

func (s *Server) handleUpdateChapterLedger(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	current, err := s.store.GetChapterLedger(r.Context(), id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	var body ChapterLedgerPatch
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Monitored != nil {
		current.Monitored = *body.Monitored
	}
	if body.Language != nil {
		current.Language = *body.Language
	}
	if body.CutoffNum != nil {
		current.CutoffNum = *body.CutoffNum
	}
	if err := s.store.UpdateChapterLedger(r.Context(), *current); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, current)
}

// --- files ---

func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	files, err := s.store.ListFiles(r.Context(), id)
	respondList(w, files, err)
}

// handleDeleteFile unlinks a file and deletes it from disk. The freed slots
// become wanted again and may re-download (Sonarr semantics, spec.md §11.4).
func (s *Server) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	fid, ok := pathID(w, r, "fid")
	if !ok {
		return
	}
	path, err := s.store.DeleteFile(r.Context(), fid)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		// Row is gone but the file persists: surface it, a rescan re-adopts.
		writeError(w, http.StatusInternalServerError, "unlinked, but disk delete failed: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	result, err := s.scanner.ScanSeriesFolder(r.Context(), id, body.Path)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	// §11.4: with chapter files present, default the cutoff to the lowest
	// owned chapter so owned chapters land in-ledger.
	if result.Linked > 0 {
		if lowest, err := s.scanner.LowestOwnedChapter(r.Context(), id); err == nil && lowest > 0 {
			if cl, err := s.store.GetChapterLedger(r.Context(), id); err == nil && lowest-100 < cl.CutoffNum {
				cl.CutoffNum = lowest - 100
				_ = s.store.UpdateChapterLedger(r.Context(), *cl)
			}
		}
	}
	writeJSON(w, http.StatusOK, result)
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
	if b.SourceID == "" || b.SourceMangaID == 0 {
		writeError(w, http.StatusBadRequest, "sourceId and sourceMangaId are required")
		return
	}
	bid, err := s.store.AddBinding(r.Context(), b)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": bid})
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
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

// decodeBody parses the JSON body into v; on failure it writes the 400 and
// returns false so handlers can one-line their decoding.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return false
	}
	return true
}

// respondList writes a list (never null: empty becomes []) or the error.
func respondList[T any](w http.ResponseWriter, list []T, err error) {
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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

func writeStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}
