//go:build e2e

package main

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/h-wb/sento/internal/extensions"
	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/qbittorrent"
	"github.com/h-wb/sento/internal/torznab"
)

func fixtureServer(t *testing.T, control bool) *httptest.Server {
	t.Helper()
	scenarios = &scenarioControl{enabled: control, name: "healthy", attempts: map[string]int{}, events: []scenarioEvent{}, clients: map[string]map[string]fixtureTorrent{}}
	mux := http.NewServeMux()
	mountExtensions(mux)
	mountQBittorrent(mux)
	mountTorznab(mux)
	mountScenarios(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func chooseScenario(t *testing.T, base, name string) {
	t.Helper()
	resp, err := http.Post(base+"/_control/scenario", "application/json", strings.NewReader(`{"name":"`+name+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 204 {
		t.Fatalf("scenario: %d", resp.StatusCode)
	}
}

func TestControlsAreOptIn(t *testing.T) {
	srv := fixtureServer(t, false)
	resp, err := http.Get(srv.URL + "/_control")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 404 {
		t.Fatal("fixture control exposed without -control")
	}
}

func TestScenarioSourceDownload(t *testing.T) {
	for _, name := range []string{"page-503", "rate-limit", "truncated-page", "permanent-401"} {
		t.Run(name, func(t *testing.T) {
			srv := fixtureServer(t, true)
			chooseScenario(t, srv.URL, name)
			host := extensions.NewClient(srv.URL + "/host")
			p := provider.NewSource(host, t.TempDir(), provider.DownloadConfig{PageConcurrency: 1}, func(context.Context) ([]provider.IndexerEntry, error) { return nil, nil })
			id, err := p.Grab(context.Background(), provider.Candidate{Source: &provider.SourceRef{SourceID: scenarioSource, ChapterKey: "recovery/ch-1"}})
			if err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(15 * time.Second)
			settled := false
			for time.Now().Before(deadline) {
				state, stateErr := p.State(context.Background(), id)
				if stateErr != nil {
					var failure *provider.DownloadError
					var upstream *extensions.HTTPError
					if name != "permanent-401" || !errors.As(stateErr, &failure) || failure.Retryable || !errors.As(stateErr, &upstream) || upstream.StatusCode != 401 {
						t.Fatal(stateErr)
					}
					settled = true
					break
				}
				if state.Done {
					settled = true
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if !settled {
				t.Fatal("source did not settle")
			}
			scenarios.Lock()
			counts := map[string]int{}
			for key, n := range scenarios.attempts {
				counts[key] = n
			}
			events := append([]scenarioEvent{}, scenarios.events...)
			scenarios.Unlock()
			want := 3
			if name == "permanent-401" {
				want = 1
			}
			if name == "rate-limit" {
				want = 2
			}
			if counts["recovery/ch-1/page-5"] != want || counts["recovery/ch-1/page-1"] != 1 {
				t.Fatalf("requests: %v", counts)
			}
			if name == "permanent-401" {
				return
			}
			if name == "rate-limit" {
				var first time.Time
				for _, e := range events {
					if e.Key == "recovery/ch-1/page-5" {
						if first.IsZero() {
							first = e.At
						} else if e.At.Sub(first) < 2*time.Second {
							t.Fatal("ignored Retry-After")
						}
					}
				}
			}
			payload, err := p.Fetch(context.Background(), id)
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(payload.Stream)
			_ = payload.Stream.Close()
			if err != nil {
				t.Fatal(err)
			}
			archive, err := zip.NewReader(strings.NewReader(string(data)), int64(len(data)))
			if err != nil || len(archive.File) != 5 {
				t.Fatalf("CBZ: %v", err)
			}
		})
	}
}

func TestDirectoryFixtureUsesActualHostProtocol(t *testing.T) {
	srv := fixtureServer(t, true)
	host := extensions.NewClient(srv.URL + "/host")
	listings, err := host.Listings(context.Background(), scenarioSource, directoryRoot)
	if err != nil || len(listings) != 3 {
		t.Fatalf("listings: %v %v", listings, err)
	}
	for _, key := range []string{directoryRoot, digitalFolder, namesFolder} {
		chapters, err := host.Chapters(context.Background(), scenarioSource, key)
		if err != nil || len(chapters) != 2 {
			t.Fatalf("%s: %v %v", key, chapters, err)
		}
	}
}

func TestFixtureClientRoutingAndRemoval(t *testing.T) {
	srv := fixtureServer(t, true)
	ctx := context.Background()
	feed := torznab.New(srv.URL+"/torznab/api", "stub-key")
	releases, err := feed.Search(ctx, "Directory Kingdom", 7000)
	if err != nil || len(releases) != 3 || releases[0].MagnetURL == "" {
		t.Fatalf("Torznab: %v %v", releases, err)
	}
	entries := []provider.IndexerEntry{{ID: 1, Enabled: true, DownloadClient: "vpn"}}
	regular := qbittorrent.New(srv.URL+"/qbit", "stub", "stub")
	vpn := qbittorrent.New(srv.URL+"/qbit-vpn", "stub", "stub")
	p := provider.NewTorrent(func(context.Context) ([]provider.IndexerEntry, error) { return entries, nil }, provider.TorrentClient{ID: "regular", Client: regular, Category: "sento-test"}, provider.TorrentClient{ID: "vpn", Client: vpn, Category: "sento-test"})
	c := provider.Candidate{SiteID: "1", Torrent: &provider.TorrentRef{InfoHash: releases[0].InfoHash, MagnetURL: releases[0].MagnetURL}}
	id, err := p.Grab(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	old, _ := regular.List(ctx, "sento-test")
	got, _ := vpn.List(ctx, "sento-test")
	if len(old) != 0 || len(got) != 1 || got[0].Hash != id {
		t.Fatalf("wrong client: regular=%v vpn=%v", old, got)
	}
	// Changing the dispatch preference does not move an existing torrent.
	entries[0].DownloadClient = "regular"
	state, err := p.State(ctx, id)
	if err != nil || !state.InFlight {
		t.Fatalf("lost existing VPN job: %+v %v", state, err)
	}
	if err = vpn.Delete(ctx, id, false); err != nil {
		t.Fatal(err)
	}
	scenarios.Lock()
	event := scenarios.events[len(scenarios.events)-1]
	scenarios.Unlock()
	if event.Kind != "torrent-delete" || event.Client != "vpn" || event.DeleteFiles {
		t.Fatalf("removal: %+v", event)
	}
	entries[0].DownloadClient = "removed-client"
	if _, err = p.Grab(ctx, c); err == nil {
		t.Fatal("missing client silently fell back")
	}
}
