// Package server wires the HTTP API and serves the web UI.
package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/h-wb/sento/internal/config"
	"github.com/h-wb/sento/internal/metadata"
	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/scheduler"
	"github.com/h-wb/sento/internal/service"
	"github.com/h-wb/sento/internal/store"
	"github.com/h-wb/sento/internal/version"
)

type Server struct {
	cfg        config.Config
	db         *sql.DB
	store      *store.Store
	provider   metadata.Provider
	library    *service.Library
	conns      *service.Connections
	registry   *provider.Registry
	search     *service.SearchService
	sites      *service.SiteService
	importer   *service.Importer
	scanner    *service.Scanner
	extensions *service.ExtensionService
	profiles   *service.ProfileService
	health     *service.HealthService
	sched      *scheduler.Scheduler
}

// SetScheduler wires the background scheduler in after construction
// (main builds it from this server's services) so the System → Tasks
// endpoints can report and trigger its loops.
func (s *Server) SetScheduler(sched *scheduler.Scheduler) { s.sched = sched }

func New(cfg config.Config, db *sql.DB, dialect string, mdProvider metadata.Provider) *Server {
	st := store.New(db, dialect)
	s := &Server{
		cfg:      cfg,
		db:       db,
		store:    st,
		provider: mdProvider,
		library:  service.NewLibrary(st, mdProvider),
		scanner:  service.NewScanner(st, cfg.Library.Path),
	}

	// Service connections live in settings (file/env seeds the first
	// run); the registry is shared and rebuilt in place on save.
	s.conns = service.NewConnections(st, cfg)
	s.conns.Apply(s.conns.Load(context.Background()))
	s.registry = s.conns.Registry()

	// Everything downstream of here is provider-agnostic.
	s.search = service.NewSearchService(st, s.registry)
	s.sites = service.NewSiteService(st, s.registry)
	s.importer = service.NewImporter(st, s.registry, cfg.Library.Path, cfg.Library.PathMappings)
	s.extensions = service.NewExtensionService(st, s.conns.Host)
	s.extensions.ManageRepos(cfg.Extensions.Repos)
	s.profiles = service.NewProfiles(st)
	s.health = service.NewHealthService(st, s.conns, cfg.Library.Path)
	if err := s.profiles.EnsureDefaults(context.Background()); err != nil {
		slog.Warn("seeding default profile failed", "error", err)
	}
	s.reconcileExtensions()
	return s
}

// reconcileExtensions converges the host's installed extensions with the
// desired list, off the request path (installs dex2jar APKs and can take
// a while).
func (s *Server) reconcileExtensions() {
	if !s.extensions.HasManager() {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := s.extensions.Reconcile(ctx); err != nil {
			slog.Warn("extension reconcile failed", "error", err)
		}
	}()
}

// Importer exposes the queue importer for background loops.
func (s *Server) Importer() *service.Importer { return s.importer }

// SearchSvc exposes the unified search for background loops.
func (s *Server) SearchSvc() *service.SearchService { return s.search }

// ExtensionsSvc exposes extension reconciliation for background loops.
func (s *Server) ExtensionsSvc() *service.ExtensionService { return s.extensions }

// HealthSvc exposes the health checks for background loops.
func (s *Server) HealthSvc() *service.HealthService { return s.health }

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
	writeJSON(w, http.StatusOK, StatusResponse{
		App:      "sento",
		Version:  version.Version,
		Commit:   version.Commit,
		Database: DatabaseStatus{OK: s.db.PingContext(r.Context()) == nil},
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
			// Vite fingerprints everything under assets/, so those can be
			// cached forever; the rest (index.html, favicon) must revalidate
			// or a browser keeps running the previous release's bundle.
			if strings.HasPrefix(req.URL.Path, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			fileServer.ServeHTTP(w, req)
			return
		}
		if _, err := os.Stat(index); err == nil {
			w.Header().Set("Cache-Control", "no-cache")
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
