// Sento — Sonarr-style automation for manga. See docs/spec/spec.md.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/h-wb/sento/internal/config"
	"github.com/h-wb/sento/internal/db"
	"github.com/h-wb/sento/internal/metadata/mangabaka"
	"github.com/h-wb/sento/internal/scheduler"
	"github.com/h-wb/sento/internal/server"
	"github.com/h-wb/sento/internal/version"
)

func main() {
	configPath := flag.String("config", "config.yml", "path to config file")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	if err := run(*configPath); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	database, dialect, err := db.Open(cfg.Database.URL, cfg.Database.Path)
	if err != nil {
		return err
	}
	defer func() { _ = database.Close() }()

	if cfg.Server.APIKey != "" {
		// Honest about a setting that reads like protection but isn't yet.
		slog.Warn("server.api_key / SENTO_API_KEY is set but not enforced: the API has no authentication; restrict access at the network or reverse-proxy level")
	}

	provider := mangabaka.New(cfg.MangaBaka.BaseURL)
	app := server.New(cfg, database, dialect, provider)

	schedCtx, stopSched := context.WithCancel(context.Background())
	defer stopSched()
	sched := scheduler.New(app.Store(), app.LibraryService(), app.SearchSvc(),
		app.Importer(), app.Scanner(), app.ExtensionsSvc(), scheduler.DefaultIntervals())
	health := app.HealthSvc()
	sched.Register("health-check", "Check Health", 15*time.Minute, func(ctx context.Context) error {
		health.Check(ctx)
		return nil
	})
	app.SetScheduler(sched)
	sched.Start(schedCtx)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: app.Handler(),
		// Bounds how long a client may take to send its request headers
		// (slow-loris). Bodies and responses stay unbounded here: searches
		// stream for minutes and are capped by the router's own timeout.
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("sento started", "version", version.Version, "addr", addr)
		errCh <- srv.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case sig := <-stop:
		slog.Info("shutting down", "signal", sig.String())
		stopSched() // no new sweeps while requests drain
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	return nil
}
