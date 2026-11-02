package server

import (
	"net/http"
	"strconv"
)

type FileCleanupBody struct {
	EditionID     int64   `json:"editionId"`
	FileIDs       []int64 `json:"fileIds"`
	RetentionDays int     `json:"retentionDays"`
}

func (s *Server) handleCleanupPreview(w http.ResponseWriter, r *http.Request) {
	sid, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	eid := int64(0)
	if value := r.URL.Query().Get("editionId"); value != "" {
		var err error
		eid, err = strconv.ParseInt(value, 10, 64)
		if err != nil || eid < 0 {
			writeError(w, 400, "invalid edition ID")
			return
		}
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
