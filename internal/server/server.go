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
	"github.com/h-wb/sento/internal/library"
	"github.com/h-wb/sento/internal/metadata"
	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/prowlarr"
	"github.com/h-wb/sento/internal/qbittorrent"
	"github.com/h-wb/sento/internal/service"
	"github.com/h-wb/sento/internal/store"
	"github.com/h-wb/sento/internal/suwayomi"
	"github.com/h-wb/sento/internal/version"
)

type Server struct {
	cfg      config.Config
	db       *sql.DB
	store    *store.Store
	provider metadata.Provider
	library  *service.Library
	suwa     *suwayomi.Client // nil when unconfigured; binding endpoints only
	registry *provider.Registry
	search   *service.SearchService
	sites    *service.SiteService
	importer *service.Importer
	scanner  *service.Scanner
}

func New(cfg config.Config, db *sql.DB, mdProvider metadata.Provider) *Server {
	st := store.New(db)
	naming := library.DefaultNaming()
	s := &Server{
		cfg:      cfg,
		db:       db,
		store:    st,
		provider: mdProvider,
		library:  service.NewLibrary(st, mdProvider),
		scanner:  service.NewScanner(st, naming, cfg.Library.Path),
	}

	if cfg.Suwayomi.URL != "" {
		s.suwa = suwayomi.New(cfg.Suwayomi.URL, cfg.Suwayomi.Username, cfg.Suwayomi.Password)
	}
	var pr *prowlarr.Client
	if cfg.Prowlarr.URL != "" {
		pr = prowlarr.New(cfg.Prowlarr.URL, cfg.Prowlarr.APIKey)
	}
	var qb *qbittorrent.Client
	if cfg.QBittorrent.URL != "" {
		qb = qbittorrent.New(cfg.QBittorrent.URL, cfg.QBittorrent.Username, cfg.QBittorrent.Password)
	}

	// Everything downstream of here is provider-agnostic.
	s.registry = provider.NewRegistry(
		provider.NewSuwayomi(s.suwa),
		provider.NewTorrent(pr, qb, cfg.QBittorrent.Category),
	)
	s.search = service.NewSearchService(st, s.registry)
	s.sites = service.NewSiteService(st, s.registry)
	s.importer = service.NewImporter(st, s.registry, naming, cfg.Library.Path)
	return s
}

// Volumes exposes the volume searcher for background loops.

// Importer exposes the queue importer for background loops.
func (s *Server) Importer() *service.Importer { return s.importer }

// SearchSvc exposes the unified search for background loops.
func (s *Server) SearchSvc() *service.SearchService { return s.search }

// Store exposes the persistence layer for background loops.
func (s *Server) Store() *store.Store { return s.store }

// LibraryService exposes the metadata use-cases for background loops.
func (s *Server) LibraryService() *service.Library { return s.library }

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	// Interactive searches fan out to live sources (some behind Cloudflare)
	// and may auto-resolve bindings on first run.
	r.Use(middleware.Timeout(120 * time.Second))

	r.Route("/api/v1", func(r chi.Router) {
		s.mountAPI(r)
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
