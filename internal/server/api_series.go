package server

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/h-wb/sento/internal/ledger"
	"github.com/h-wb/sento/internal/service"
	"github.com/h-wb/sento/internal/store"
)

// Series-scoped handlers: the library entries, their collections and
// slots, files, in-place imports and source bindings.

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
	id, result, err := s.library.ImportFolder(ctx, s.scanner, body.Dir, body.MangaBakaID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ImportFolderResponse{SeriesID: id, Scan: result})
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
	collections, err := s.store.ListCollections(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, SeriesDetail{
		Series:      series,
		Collections: collections,
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
	if patch.Monitored != nil {
		if err := s.store.SetSeriesMonitored(r.Context(), id, *patch.Monitored); err != nil {
			writeErr(w, err)
			return
		}
	}
	if patch.ProfileID != nil {
		if err := s.store.SetSeriesProfile(r.Context(), id, *patch.ProfileID); err != nil {
			writeErr(w, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteSeries(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.store.DeleteSeries(r.Context(), id); err != nil {
		writeErr(w, err)
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
		writeErr(w, err)
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
	volumes, chapters, err := s.library.Slots(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	out := SlotsResponse{
		Volumes:  map[string][]ledger.VolumeSlot{},
		Chapters: map[string][]ledger.ChapterSlot{},
	}
	for colID, slots := range volumes {
		out.Volumes[strconv.FormatInt(colID, 10)] = slots
	}
	for colID, slots := range chapters {
		out.Chapters[strconv.FormatInt(colID, 10)] = slots
	}
	writeJSON(w, http.StatusOK, out)
}

// --- collection updates ---

func (s *Server) handleCreateCollection(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body CreateCollectionBody
	if !decodeBody(w, r, &body) {
		return
	}
	col, err := s.library.CreateCollection(r.Context(), id, service.CreateCollectionParams{
		EditionName: body.EditionName, Language: body.Language,
		Publisher: body.Publisher, Medium: body.Medium, Type: body.Type,
		BasedOn: body.BasedOn, Volumes: body.Volumes,
		CutoffNum: body.CutoffNum, Monitored: body.Monitored,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, col)
}

func (s *Server) handleMergeCollection(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	cid, ok := pathID(w, r, "cid")
	if !ok {
		return
	}
	var body MergeCollectionBody
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Into == 0 || body.Into == cid {
		writeError(w, http.StatusBadRequest, "merge target must be a different collection")
		return
	}
	if r.URL.Query().Get("preview") == "1" {
		data, err := s.store.PreviewMerge(r.Context(), id, cid, body.Into)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, data)
		return
	}
	unmatched, err := s.store.MergeCollection(r.Context(), id, cid, body.Into)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, MergeCollectionResponse{Unmatched: unmatched})
}

func (s *Server) handleDeleteCollection(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	cid, ok := pathID(w, r, "cid")
	if !ok {
		return
	}
	if err := s.store.DeleteCustomCollection(r.Context(), id, cid); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

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
	if err := s.library.UpdateCollection(r.Context(), id, cid, service.CollectionUpdate{
		Monitored: body.Monitored, ProfileID: body.ProfileID,
		CutoffNum: body.CutoffNum, Language: body.Language,
	}); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
		writeErr(w, err)
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
	result, err := s.scanner.ScanAndAdoptCutoff(r.Context(), id, body.Path)
	if err != nil {
		writeErr(w, err)
		return
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
	if b.SourceID == "" || b.SourceMangaKey == "" {
		writeError(w, http.StatusBadRequest, "sourceId and sourceMangaKey are required")
		return
	}
	bid, err := s.store.AddBinding(r.Context(), b)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": bid})
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
	if patch.TrustChapters != nil {
		if err := s.store.SetBindingTrust(r.Context(), id, bid, *patch.TrustChapters); err != nil {
			writeErr(w, err)
			return
		}
		if patch.CollectionID == nil && patch.State == "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	if patch.CollectionID != nil {
		if err := s.store.SetBindingEdition(r.Context(), id, bid, *patch.CollectionID); err != nil {
			writeErr(w, err)
			return
		}
		if patch.State == "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	if patch.State != "active" && patch.State != "disabled" {
		writeError(w, http.StatusBadRequest, "state must be active or disabled")
		return
	}
	if err := s.store.SetBindingState(r.Context(), id, bid, patch.State); err != nil {
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
