package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/h-wb/sento/internal/store"
)

func (s *Server) handleListVersions(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	data, err := s.store.ListVersions(r.Context(), id)
	respondList(w, data, err)
}
func (s *Server) handleSaveVersion(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var t store.CollectedVersion
	if !decodeBody(w, r, &t) {
		return
	}
	if r.Method == http.MethodPut {
		vid, ok := pathID(w, r, "vid")
		if !ok {
			return
		}
		t.ID = vid
	}
	vid, err := s.library.SaveVersion(r.Context(), id, t)
	if err != nil {
		writeErr(w, err)
		return
	}
	data, err := s.store.GetVersion(r.Context(), vid)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}
func (s *Server) handleDeleteVersion(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	vid, ok := pathID(w, r, "vid")
	if !ok {
		return
	}
	if err := s.store.DeleteVersion(r.Context(), id, vid); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) handleBooksByVersion(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	data, err := s.library.BooksByVersion(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}
func (s *Server) handleSaveBook(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var unit store.Book
	if !decodeBody(w, r, &unit) {
		return
	}
	if r.Method == http.MethodPut {
		bid, ok := pathID(w, r, "bid")
		if !ok {
			return
		}
		unit.ID = bid
	}
	bid, err := s.store.SaveBook(r.Context(), id, unit)
	if err != nil {
		writeErr(w, err)
		return
	}
	data, err := s.store.GetBook(r.Context(), bid)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}
func (s *Server) handleDeleteBook(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	bid, ok := pathID(w, r, "bid")
	if !ok {
		return
	}
	if err := s.store.DeleteBook(r.Context(), id, bid); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) handleEditEditionMetadata(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	cid, ok := pathID(w, r, "eid")
	if !ok {
		return
	}
	var p store.EditionMetadataPatch
	if !decodeBody(w, r, &p) {
		return
	}
	if err := s.store.EditEdition(r.Context(), id, cid, p); err != nil {
		writeErr(w, err)
		return
	}
	if len(p.Reset) > 0 {
		if err := s.library.Refresh(r.Context(), id); err != nil {
			writeErr(w, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

type CatalogLinkBody struct {
	CatalogID int64 `json:"catalogId"`
}

func (s *Server) handleCatalogLink(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	cid, ok := pathID(w, r, "eid")
	if !ok {
		return
	}
	var b CatalogLinkBody
	if !decodeBody(w, r, &b) {
		return
	}
	if r.URL.Query().Get("preview") == "1" {
		data, err := s.store.PreviewCatalogLink(r.Context(), id, cid, b.CatalogID)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, data)
		return
	}
	if r.URL.Query().Get("decline") == "1" {
		if err := s.store.DeclineCatalogLink(r.Context(), id, cid, b.CatalogID); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := s.store.LinkCatalog(r.Context(), id, cid, b.CatalogID); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleBookWorks lists the catalog works behind one book.
func (s *Server) handleBookWorks(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	bid, ok := pathID(w, r, "bid")
	if !ok {
		return
	}
	book, err := s.store.GetBook(r.Context(), bid)
	if err == nil {
		var edition *store.Edition
		if edition, err = s.store.GetEdition(r.Context(), book.EditionID); err == nil && edition.SeriesID != id {
			err = store.ErrNotFound
		}
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	works, err := s.store.ListWorks(r.Context(), bid)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, works)
}

// handleCatalogEditions lists the catalog's edition names.
func (s *Server) handleCatalogEditions(w http.ResponseWriter, r *http.Request) {
	editions, err := s.library.CatalogEditions(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, editions)
}

// handleSeparateCollection gives one collection an edition of its own.
func (s *Server) handleSeparateCollection(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	cid, ok := pathID(w, r, "cid")
	if !ok {
		return
	}
	edition, err := s.store.SeparateCollection(r.Context(), id, cid)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err = s.store.EnsureModels(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"editionId": edition})
}

func (s *Server) handleCoverage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	data, err := s.store.Coverage(r.Context(), id)
	respondList(w, data, err)
}
func (s *Server) handlePinCoverage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var a store.AssetCoverage
	if !decodeBody(w, r, &a) {
		return
	}
	if err := s.store.PinCoverage(r.Context(), id, a); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) handleRemapFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	fid, ok := pathID(w, r, "fid")
	if !ok {
		return
	}
	var body FileMappingBody
	if !decodeBody(w, r, &body) {
		return
	}
	// Mapping the file may displace bettered owners (upgrade path); the
	// response says which, so the UI can report the replacement.
	mapping, err := s.importer.ApplyUpgrades(r.Context(), id, fid, body.Books, body.Replace)
	if err != nil {
		writeErr(w, err)
		return
	}
	organizeErr := s.importer.OrganizeReviewedFile(r.Context(), id, fid)
	// The mapping is saved either way; a job it completes is closed now.
	if err := s.importer.SettleReviewedJobs(r.Context(), id); err != nil {
		slog.Warn("settling reviewed jobs failed", "series", id, "error", err)
	}
	if organizeErr != nil {
		writeErr(w, fmt.Errorf("mapping saved, but organizing the file failed: %w", organizeErr))
		return
	}
	writeJSON(w, http.StatusOK, mapping)
}

type FileMappingBody struct {
	Books []store.BookRef `json:"books"`
	// Replace forces the upgrade path: this file takes the books even when
	// it does not outscore the current owner.
	Replace bool `json:"replace,omitempty"`
}

type ReviewResolveBody struct {
	FileIDs []int64 `json:"fileIds"`
}

// handleResolveFileReviews declines a pack of file reviews at once: every
// file is marked reviewed and unmapped, the bulk counterpart of the bulk
// accept. The response says which files were resolved and which were
// skipped because they no longer waited.
func (s *Server) handleResolveFileReviews(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body ReviewResolveBody
	if !decodeBody(w, r, &body) {
		return
	}
	res, err := s.importer.ResolveFileReviews(r.Context(), id, body.FileIDs)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type ReviewMatchBody struct {
	VersionID int64   `json:"versionId"`
	FileIDs   []int64 `json:"fileIds"`
}

// handleMatchRelease applies one collected version to a whole pack: each
// file is mapped by its own parsed numbers, and files whose numbers do not
// resolve are left for individual review.
func (s *Server) handleMatchRelease(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body ReviewMatchBody
	if !decodeBody(w, r, &body) {
		return
	}
	matches, err := s.importer.MatchReleaseFiles(r.Context(), id, body.VersionID, body.FileIDs)
	respondList(w, matches, err)
}

func (s *Server) handleObservations(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		data, err := s.store.ListObservations(r.Context(), id)
		respondList(w, data, err)
		return
	}
	var o store.BookObservation
	if !decodeBody(w, r, &o) {
		return
	}
	if err := s.store.ObserveBook(r.Context(), id, o); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type ObservationReviewBody struct {
	Accept bool `json:"accept"`
}

func (s *Server) handleResolveObservation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	oid, ok := pathID(w, r, "oid")
	if !ok {
		return
	}
	var b ObservationReviewBody
	if !decodeBody(w, r, &b) {
		return
	}
	if err := s.store.ResolveObservation(r.Context(), id, oid, b.Accept); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type BookMonitorBody struct {
	VersionID int64 `json:"versionId"`
	BookID    int64 `json:"bookId"`
	Monitored bool  `json:"monitored"`
}

func (s *Server) handleBookMonitor(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var b BookMonitorBody
	if !decodeBody(w, r, &b) {
		return
	}
	if err := s.store.SetBookMonitored(r.Context(), id, b.VersionID, b.BookID, b.Monitored); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleResetBook(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	bid, ok := pathID(w, r, "bid")
	if !ok {
		return
	}
	if err := s.store.ResetBook(r.Context(), id, bid); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.library.Refresh(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleEditionOverview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	data, err := s.library.EditionOverview(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}
func (s *Server) handleVersionBooks(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	vid, ok := pathID(w, r, "vid")
	if !ok {
		return
	}
	data, err := s.library.VersionBooks(r.Context(), id, vid)
	respondList(w, data, err)
}
func (s *Server) handleBulkMonitor(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	vid, ok := pathID(w, r, "vid")
	if !ok {
		return
	}
	var body struct {
		BookIDs   []int64 `json:"bookIds"`
		Monitored bool    `json:"monitored"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if err := s.store.BulkMonitor(r.Context(), id, vid, body.BookIDs, body.Monitored); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReview(w http.ResponseWriter, r *http.Request) {
	sid, _ := strconv.ParseInt(r.URL.Query().Get("seriesId"), 10, 64)
	data, err := s.library.ReviewItems(r.Context(), sid)
	respondList(w, data, err)
}

func (s *Server) handleFileFacts(w http.ResponseWriter, r *http.Request) {
	sid, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	fid, ok := pathID(w, r, "fid")
	if !ok {
		return
	}
	var p store.AssetFactPatch
	if !decodeBody(w, r, &p) {
		return
	}
	if err := s.store.CorrectFileFacts(r.Context(), sid, fid, p); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleSplitBox turns one catalog item into the box of volumes it is.
func (s *Server) handleSplitBox(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	bid, ok := pathID(w, r, "bid")
	if !ok {
		return
	}
	var body SplitBoxBody
	if !decodeBody(w, r, &body) {
		return
	}
	volumes, err := s.library.SplitBox(r.Context(), id, bid, body.Count)
	respondList(w, volumes, err)
}

// handlePlanOrganize previews what organising a series' files would
// rename and re-tag under the current media settings.
func (s *Server) handlePlanOrganize(w http.ResponseWriter, r *http.Request) {
	sid, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	changes, err := s.importer.PlanOrganize(r.Context(), sid)
	respondList(w, changes, err)
}

func (s *Server) handleOrganize(w http.ResponseWriter, r *http.Request) {
	sid, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body OrganizeBody
	if !decodeBody(w, r, &body) {
		return
	}
	res, err := s.importer.Organize(r.Context(), sid, body.Rename, body.Retag)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleCheckFiles(w http.ResponseWriter, r *http.Request) {
	sid, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.store.CheckFilePresence(r.Context(), sid); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
