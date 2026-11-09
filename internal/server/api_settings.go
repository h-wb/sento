package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/h-wb/sento/internal/service"
)

// Settings and sidecar-extension handlers.

// --- media settings ---

func (s *Server) handleGetMediaSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, service.LoadMediaSettings(r.Context(), s.store))
}

func (s *Server) handlePutMediaSettings(w http.ResponseWriter, r *http.Request) {
	var m service.MediaSettings
	if !decodeBody(w, r, &m) {
		return
	}
	if err := service.SaveMediaSettings(r.Context(), s.store, m); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// --- connection settings ---

func (s *Server) handleGetConnections(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.conns.Load(r.Context()))
}

func (s *Server) handlePutConnections(w http.ResponseWriter, r *http.Request) {
	var cs service.ConnectionSettings
	if !decodeBody(w, r, &cs) {
		return
	}
	if err := s.conns.Save(r.Context(), cs); err != nil {
		writeErr(w, err)
		return
	}
	// A (re)connected sidecar may be fresh: converge its extensions.
	s.reconcileExtensions()
	writeJSON(w, http.StatusOK, cs)
}

// handleTestConnections probes the services in the posted settings (the
// UI tests before saving).
func (s *Server) handleTestConnections(w http.ResponseWriter, r *http.Request) {
	var cs service.ConnectionSettings
	if !decodeBody(w, r, &cs) {
		return
	}
	respondList(w, service.TestConnections(r.Context(), cs), nil)
}

func (s *Server) handlePreviewMediaSettings(w http.ResponseWriter, r *http.Request) {
	var m service.MediaSettings
	if !decodeBody(w, r, &m) {
		return
	}
	p := service.PreviewMediaSettings(m)
	writeJSON(w, http.StatusOK, NamingPreview{
		VolumePath: p["volumePath"], ChapterPath: p["chapterPath"],
		BookFolder:   p["bookFolder"],
		SeriesFolder: p["seriesFolder"], VolumeFile: p["volumeFile"], ChapterFile: p["chapterFile"],
		Reader: service.PreviewReaderFields(m),
	})
}

// --- extensions (sidecar host management) ---

func (s *Server) handleListExtensions(w http.ResponseWriter, r *http.Request) {
	views, err := s.extensions.List(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	respondList(w, views, nil)
}

func (s *Server) handleInstallExtension(w http.ResponseWriter, r *http.Request) {
	if err := s.extensions.Install(r.Context(), chi.URLParam(r, "pkg")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleExtensionSources(w http.ResponseWriter, r *http.Request) {
	sources, err := s.extensions.Sources(r.Context(), chi.URLParam(r, "pkg"))
	if err != nil {
		writeErr(w, err)
		return
	}
	respondList(w, sources, nil)
}

func (s *Server) handleGetExtensionRepos(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.extensionRepos(r))
}

func (s *Server) extensionRepos(r *http.Request) ExtensionRepos {
	return ExtensionRepos{
		URLs:    s.extensions.Repos(r.Context()),
		Repos:   s.extensions.RepoStatus(r.Context()),
		Managed: s.extensions.ReposManaged(),
	}
}

func (s *Server) handleSetExtensionRepos(w http.ResponseWriter, r *http.Request) {
	var body ExtensionRepos
	if !decodeBody(w, r, &body) {
		return
	}
	if err := s.extensions.SetRepos(r.Context(), body.URLs); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.extensionRepos(r))
}

// Preference routes address the host source id directly (not the entry
// id): the indexer modal edits prefs before the entry exists — the
// extension is already installed by the pick.
func (s *Server) handleSourcePreferences(w http.ResponseWriter, r *http.Request) {
	prefs, err := s.extensions.Preferences(r.Context(), chi.URLParam(r, "sid"))
	if err != nil {
		writeErr(w, err)
		return
	}
	respondList(w, prefs, nil)
}

func (s *Server) handleSetSourcePreference(w http.ResponseWriter, r *http.Request) {
	var body SetPreferenceBody
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Key == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}
	if err := s.extensions.SetPreference(r.Context(), chi.URLParam(r, "sid"), body.Key, body.Value); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
