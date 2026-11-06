package server

import (
	"net/http"

	"github.com/h-wb/sento/internal/service"
)

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
