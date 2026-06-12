package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/h-wb/sento/internal/profile"
	"github.com/h-wb/sento/internal/service"
)

// mountProfileAPI wires release profiles and custom formats.
func (s *Server) mountProfileAPI(r chi.Router) {
	r.Route("/profiles", func(r chi.Router) {
		r.Get("/", s.handleListProfiles)
		r.Post("/", s.handleCreateProfile)
		r.Post("/import", s.handleImportProfile)
		r.Get("/default", s.handleGetDefaultProfile)
		r.Put("/default", s.handleSetDefaultProfile)
		r.Route("/{id}", func(r chi.Router) {
			r.Put("/", s.handleUpdateProfile)
			r.Delete("/", s.handleDeleteProfile)
			r.Get("/export", s.handleExportProfile)
		})
	})
	r.Route("/formats", func(r chi.Router) {
		r.Get("/", s.handleListFormats)
		r.Post("/", s.handleCreateFormat)
		r.Put("/{id}", s.handleUpdateFormat)
		r.Delete("/{id}", s.handleDeleteFormat)
	})
}

func (s *Server) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := s.profiles.ListProfiles(r.Context())
	respondList(w, profiles, err)
}

func (s *Server) handleCreateProfile(w http.ResponseWriter, r *http.Request) {
	var p profile.Profile
	if !decodeBody(w, r, &p) {
		return
	}
	created, err := s.profiles.CreateProfile(r.Context(), p)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var p profile.Profile
	if !decodeBody(w, r, &p) {
		return
	}
	p.ID = id
	if err := s.profiles.UpdateProfile(r.Context(), p); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.profiles.DeleteProfile(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetDefaultProfile(w http.ResponseWriter, r *http.Request) {
	id, err := s.profiles.DefaultProfileID(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"profileId": id})
}

func (s *Server) handleSetDefaultProfile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProfileID int64 `json:"profileId"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if err := s.profiles.SetDefaultProfile(r.Context(), body.ProfileID); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleExportProfile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	doc, err := s.profiles.ExportProfile(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

func (s *Server) handleImportProfile(w http.ResponseWriter, r *http.Request) {
	var doc service.ProfileDoc
	if !decodeBody(w, r, &doc) {
		return
	}
	p, err := s.profiles.ImportProfile(r.Context(), doc)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleListFormats(w http.ResponseWriter, r *http.Request) {
	formats, err := s.profiles.ListFormats(r.Context())
	respondList(w, formats, err)
}

func (s *Server) handleCreateFormat(w http.ResponseWriter, r *http.Request) {
	var f profile.Format
	if !decodeBody(w, r, &f) {
		return
	}
	created, err := s.profiles.CreateFormat(r.Context(), f)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) handleUpdateFormat(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var f profile.Format
	if !decodeBody(w, r, &f) {
		return
	}
	f.ID = id
	if err := s.profiles.UpdateFormat(r.Context(), f); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteFormat(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.profiles.DeleteFormat(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
