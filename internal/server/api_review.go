package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/h-wb/sento/internal/service"
	"github.com/h-wb/sento/internal/store"
)

// Review inbox handlers: the file-level corrections, the bulk match and
// resolve actions, and the review choices preview/apply. The four
// /series/{id}/review/* routes sit outside the global request cap and are
// registered in Server.Handler.

// mountReviewAPI registers the review routes that live inside the global
// request cap.
func (s *Server) mountReviewAPI(r chi.Router) {
	r.Get("/review", s.handleReview)
	r.Patch("/series/{id}/files/{fid}/facts", s.handleFileFacts)
	r.Patch("/series/{id}/files/facts", s.handleFilesFacts)
}

func (s *Server) handleReview(w http.ResponseWriter, r *http.Request) {
	sid, ok := queryID(r, "seriesId")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid series ID")
		return
	}
	data, err := s.library.ReviewItems(r.Context(), sid)
	respondList(w, data, err)
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
	if r.URL.Query().Get("preview") == "1" {
		matches, err := s.library.PreviewReleaseFiles(r.Context(), id, body.VersionID, body.FileIDs)
		respondList(w, matches, err)
		return
	}
	matches, err := s.importer.MatchReleaseFiles(r.Context(), id, body.VersionID, body.FileIDs)
	respondList(w, matches, err)
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

// handleFilesFacts records one correction on several files of a series.
func (s *Server) handleFilesFacts(w http.ResponseWriter, r *http.Request) {
	sid, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body FilesFactsBody
	if !decodeBody(w, r, &body) {
		return
	}
	if len(body.FileIDs) == 0 {
		writeError(w, http.StatusBadRequest, "no files given")
		return
	}
	if err := s.store.CorrectFilesFacts(r.Context(), sid, body.FileIDs, store.AssetFactPatch{Language: body.Language, Group: body.Group, Flags: body.Flags}); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReviewChoicesPreview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body service.ReviewPlanRequest
	if !decodeBody(w, r, &body) {
		return
	}
	plan, err := s.library.PreviewReviewChoices(r.Context(), id, body)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func (s *Server) handleReviewChoicesApply(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body service.ReviewApplyRequest
	if !decodeBody(w, r, &body) {
		return
	}
	out, err := s.importer.ApplyReviewChoices(r.Context(), id, body)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
