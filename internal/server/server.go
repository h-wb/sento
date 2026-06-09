// Package server wires the HTTP API and serves the web UI.
package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/h-wb/sento/internal/config"
	"github.com/h-wb/sento/internal/version"
)

type Server struct {
	cfg config.Config
	db  *sql.DB
}

func New(cfg config.Config, db *sql.DB) *Server {
	return &Server{cfg: cfg, db: db}
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/system/health", s.handleHealth)
		r.Get("/system/status", s.handleStatus)
	})

	s.mountStatic(r)
	return r
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	dbOK := s.db.PingContext(r.Context()) == nil
	writeJSON(w, http.StatusOK, map[string]any{
		"app":     "sento",
		"version": version.Version,
		"commit":  version.Commit,
		"database": map[string]bool{
			"ok": dbOK,
		},
	})
}

// mountStatic serves the built SPA (if present) with an index.html fallback
// for client-side routing. API routes always take precedence.
func (s *Server) mountStatic(r chi.Router) {
	dir := s.cfg.Server.StaticDir
	if dir == "" {
		return
	}
	index := filepath.Join(dir, "index.html")
	fileServer := http.FileServer(http.Dir(dir))

	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/api/") {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		path := filepath.Join(dir, filepath.Clean("/"+req.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, req)
			return
		}
		if _, err := os.Stat(index); err == nil {
			http.ServeFile(w, req, index)
			return
		}
		http.Error(w, "web UI not built (run: mise run web:build)", http.StatusNotFound)
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body) // headers already sent; nothing to recover
}
