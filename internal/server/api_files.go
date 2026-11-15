package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// File handlers: listing, scoring, mapping and deleting files, plus the
// cleanup preview/run and the trash.

// mountFilesAPI registers the file routes.
func (s *Server) mountFilesAPI(r chi.Router) {
	r.Delete("/files/{fid}", s.handleDeleteFile)
	r.Get("/series/{id}/files", s.handleListFiles)
	r.Get("/series/{id}/files/cleanup", s.handleCleanupPreview)
	r.Post("/series/{id}/files/cleanup", s.handleCleanupFiles)
	r.Get("/series/{id}/files/trash", s.handleFileTrash)
	r.Post("/series/{id}/files/trash/{fid}/restore", s.handleRestoreTrash)
	r.Delete("/series/{id}/files/trash/{fid}", s.handlePurgeTrash)
	r.Get("/series/{id}/files/organize", s.handlePlanOrganize)
	r.Get("/series/{id}/versions/{vid}/files/{fid}/score", s.handleFileScore)
	r.Put("/series/{id}/files/{fid}/mapping", s.handleRemapFile)
	r.Post("/series/{id}/check-files", s.handleCheckFiles)
}

func (s *Server) handleFileScore(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	vid, ok := pathID(w, r, "vid")
	if !ok {
		return
	}
	fid, ok := pathID(w, r, "fid")
	if !ok {
		return
	}
	score, err := s.library.FileScore(r.Context(), id, vid, fid)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, score)
}

func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	files, err := s.store.ListFiles(r.Context(), id)
	respondList(w, files, err)
}

// handleDeleteFile unlinks a file and deletes it from disk. The freed books
// become wanted again and may re-download (Sonarr semantics, spec.md §11.4).
func (s *Server) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	fid, ok := pathID(w, r, "fid")
	if !ok {
		return
	}
	if err := s.importer.DeleteFile(r.Context(), fid); err != nil {
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
	mapping, err := s.importer.ApplySafeFileMapping(r.Context(), id, fid, body.Books, body.Replace)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mapping)
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

func (s *Server) handleCleanupPreview(w http.ResponseWriter, r *http.Request) {
	sid, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	eid, ok := queryID(r, "editionId")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid edition ID")
		return
	}
	files, err := s.importer.CleanupPreview(r.Context(), sid, eid)
	respondList(w, files, err)
}
func (s *Server) handleCleanupFiles(w http.ResponseWriter, r *http.Request) {
	sid, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body FileCleanupBody
	if !decodeBody(w, r, &body) {
		return
	}
	result, err := s.importer.CleanupFiles(r.Context(), sid, body.EditionID, body.FileIDs, body.RetentionDays)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *Server) handleFileTrash(w http.ResponseWriter, r *http.Request) {
	sid, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	files, err := s.store.ListFileTrash(r.Context(), sid)
	respondList(w, files, err)
}
func (s *Server) handleRestoreTrash(w http.ResponseWriter, r *http.Request) {
	sid, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	fid, ok := pathID(w, r, "fid")
	if !ok {
		return
	}
	if err := s.importer.RestoreTrashedFile(r.Context(), sid, fid); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) handlePurgeTrash(w http.ResponseWriter, r *http.Request) {
	sid, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	fid, ok := pathID(w, r, "fid")
	if !ok {
		return
	}
	if err := s.importer.PurgeTrashedFile(r.Context(), sid, fid); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
