package service

import (
	"context"
	"testing"

	"github.com/h-wb/sento/internal/config"
	"github.com/h-wb/sento/internal/provider"
)

// Connection settings: env/file seeds the first load, saves win after,
// and Apply rebuilds the shared registry in place.
func TestConnectionSettings(t *testing.T) {
	_, st := setup(t)
	ctx := context.Background()

	cfg := config.Default()
	cfg.Suwayomi.URL = "http://suwayomi.example"
	conns := NewConnections(st, cfg)

	// Seeded from config: suwayomi backend inferred, nothing in the DB yet.
	cs := conns.Load(ctx)
	if cs.SourceBackend != "suwayomi" || cs.Suwayomi.URL != "http://suwayomi.example" {
		t.Fatalf("seed = %+v", cs)
	}

	conns.Apply(cs)
	if conns.Registry().ForProtocol(provider.ProtocolSource) == nil {
		t.Fatal("source provider missing after apply")
	}
	if conns.Registry().ForProtocol(provider.ProtocolTorrent) != nil {
		t.Fatal("torrent provider should be absent without prowlarr/qbit")
	}
	if conns.Host() == nil || conns.Host().Name() != "suwayomi" {
		t.Fatalf("host = %v", conns.Host())
	}

	// Saving a switch to the sidecar + prowlarr rebuilds everything.
	cs.SourceBackend = "host"
	cs.SourceHost.URL = "http://host.example"
	cs.Prowlarr = ProwlarrConn{URL: "http://prowlarr.example", APIKey: "k"}
	if err := conns.Save(ctx, cs); err != nil {
		t.Fatal(err)
	}
	if conns.Host().Name() != "host" {
		t.Fatalf("host after save = %q", conns.Host().Name())
	}
	if conns.Registry().ForProtocol(provider.ProtocolTorrent) == nil {
		t.Fatal("torrent provider missing after save")
	}

	// A fresh service sees the saved settings, not the config seed.
	again := NewConnections(st, cfg).Load(ctx)
	if again.SourceBackend != "host" || again.Prowlarr.URL != "http://prowlarr.example" {
		t.Fatalf("reload = %+v", again)
	}

	// Validation: backend selected without its URL.
	bad := again
	bad.SourceHost.URL = ""
	if err := conns.Save(ctx, bad); err == nil {
		t.Fatal("want validation error for host backend without url")
	}
	if err := conns.Save(ctx, ConnectionSettings{SourceBackend: "none",
		Prowlarr: ProwlarrConn{URL: "prowlarr.example"}}); err == nil {
		t.Fatal("want validation error for scheme-less url")
	}
}
