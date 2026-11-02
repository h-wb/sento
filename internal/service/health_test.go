package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/h-wb/sento/internal/config"
	"github.com/h-wb/sento/internal/store"
)

func healthIndexer(t *testing.T, st *store.Store, name, protocol, settings string, enabled bool) {
	t.Helper()
	if _, err := st.AddIndexer(context.Background(), store.Indexer{
		Name: name, Protocol: protocol, Enabled: enabled, UseVolumes: true,
		Settings: store.JSONText(settings),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestHealthChecks(t *testing.T) {
	t.Parallel()
	_, st := setup(t)
	ctx := context.Background()

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `<caps><server title="ok"/></caps>`)
	}))
	defer up.Close()
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer down.Close()

	cfg := config.Default()
	conns := NewConnections(st, cfg)
	library := filepath.Join(t.TempDir(), "library")
	health := NewHealthService(st, conns, library)

	// A fresh install: nothing to search with, and a library folder that
	// has not been created yet is not a problem.
	report := health.Report(ctx)
	if len(report.Issues) != 1 || report.Issues[0].Source != "indexer" || report.Issues[0].Level != HealthWarning {
		t.Fatalf("fresh install = %+v", report.Issues)
	}

	healthIndexer(t, st, "Good", store.IndexerTorznab, fmt.Sprintf(`{"url":%q}`, up.URL), true)
	healthIndexer(t, st, "Broken", store.IndexerTorznab, fmt.Sprintf(`{"url":%q}`, down.URL), true)
	healthIndexer(t, st, "Paused", store.IndexerTorznab, fmt.Sprintf(`{"url":%q}`, down.URL), false)
	healthIndexer(t, st, "Gone", store.IndexerSource, `{"sourceId":"42"}`, true)

	// The report is cached until invalidated.
	if got := health.Report(ctx); len(got.Issues) != 1 {
		t.Fatalf("cached report was recomputed: %+v", got.Issues)
	}
	health.Invalidate()
	report = health.Report(ctx)

	got := map[string]string{}
	for _, issue := range report.Issues {
		got[issue.Message] = issue.Level
	}
	want := map[string]string{
		"Indexer Broken is unavailable":                                              HealthError,
		"No download client is configured, so torrent releases cannot be downloaded": HealthWarning,
		"No extensions sidecar is configured, so source indexers cannot be searched": HealthWarning,
	}
	if len(got) != len(want) {
		t.Fatalf("issues = %+v", report.Issues)
	}
	for message, level := range want {
		if got[message] != level {
			t.Errorf("missing %s issue %q in %+v", level, message, report.Issues)
		}
	}
	if report.Issues[0].Level != HealthError {
		t.Errorf("errors should sort first: %+v", report.Issues)
	}

	// A reachable host that no longer carries the source, and a download
	// client that does not answer.
	conns.mu.Lock()
	conns.host = &fakeExtHost{}
	conns.mu.Unlock()
	raw := fmt.Sprintf(`{"qbittorrent":{"url":%q}}`, down.URL)
	if err := st.PutSetting(ctx, connectionsKey, []byte(raw)); err != nil {
		t.Fatal(err)
	}
	report = health.Check(ctx)
	got = map[string]string{}
	for _, issue := range report.Issues {
		got[issue.Message] = issue.Level
	}
	for _, message := range []string{"Indexer Broken is unavailable", "Indexer Gone is unavailable", "Download client qBittorrent is unavailable"} {
		if got[message] != HealthError {
			t.Errorf("missing error %q in %+v", message, report.Issues)
		}
	}
	if len(got) != 3 {
		t.Errorf("issues = %+v", report.Issues)
	}
}

func TestHealthLibraryPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if issue := checkLibraryPath(dir); issue != nil {
		t.Fatalf("writable folder flagged: %+v", issue)
	}
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if issue := checkLibraryPath(file); issue == nil || issue.Level != HealthError {
		t.Fatalf("a file as library path = %+v", issue)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("the write probe left files behind: %v %v", entries, err)
	}
}
