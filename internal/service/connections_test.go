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
	cfg.SourceHost.URL = "http://host.example"
	conns := NewConnections(st, cfg)

	// Seeded from config, nothing in the DB yet.
	cs := conns.Load(ctx)
	if cs.SourceHost.URL != "http://host.example" {
		t.Fatalf("seed = %+v", cs)
	}

	conns.Apply(cs)
	if conns.Registry().ForProtocol(provider.ProtocolSource) == nil {
		t.Fatal("source provider missing after apply")
	}
	if conns.Registry().ForProtocol(provider.ProtocolTorrent) != nil {
		t.Fatal("torrent provider should be absent without prowlarr/qbit")
	}
	if conns.Host() == nil || conns.Host().Name() != "host" {
		t.Fatalf("host = %v", conns.Host())
	}

	// Saving prowlarr + a new host URL rebuilds everything.
	cs.SourceHost.URL = "http://host2.example"
	cs.Prowlarr = ProwlarrConn{URL: "http://prowlarr.example", APIKey: "k"}
	if err := conns.Save(ctx, cs); err != nil {
		t.Fatal(err)
	}
	if conns.Registry().ForProtocol(provider.ProtocolTorrent) == nil {
		t.Fatal("torrent provider missing after save")
	}

	// A fresh service sees the saved settings, not the config seed.
	again := NewConnections(st, cfg).Load(ctx)
	if again.SourceHost.URL != "http://host2.example" || again.Prowlarr.URL != "http://prowlarr.example" {
		t.Fatalf("reload = %+v", again)
	}

	// Clearing the host URL disables the source provider.
	cs.SourceHost.URL = ""
	if err := conns.Save(ctx, cs); err != nil {
		t.Fatal(err)
	}
	if conns.Registry().ForProtocol(provider.ProtocolSource) != nil || conns.Host() != nil {
		t.Fatal("source provider should be gone without a host url")
	}

	if err := conns.Save(ctx, ConnectionSettings{
		Prowlarr: ProwlarrConn{URL: "prowlarr.example"}}); err == nil {
		t.Fatal("want validation error for scheme-less url")
	}
}
