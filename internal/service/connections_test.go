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
	cfg.Extensions.URL = "http://host.example"
	conns := NewConnections(st, cfg)

	// Seeded from config, nothing in the DB yet.
	cs := conns.Load(ctx)
	if cs.Extensions.URL != "http://host.example" {
		t.Fatalf("seed = %+v", cs)
	}

	conns.Apply(cs)
	if conns.Registry().ForProtocol(provider.ProtocolSource) == nil {
		t.Fatal("source provider missing after apply")
	}
	// The torrent provider is always present: it searches whatever
	// torznab indexer entries exist.
	if conns.Registry().ForProtocol(provider.ProtocolTorrent) == nil {
		t.Fatal("torrent provider missing")
	}
	if conns.Host() == nil || conns.Host().Name() != "host" {
		t.Fatalf("host = %v", conns.Host())
	}

	// A sidecar URL given by the configuration stays in force: a save
	// cannot change or clear it, and says so. Other settings persist.
	if !cs.Managed.ExtensionsURL {
		t.Fatalf("a configured sidecar URL should be marked managed: %+v", cs.Managed)
	}
	cs.Extensions.URL = "http://host2.example"
	cs.Prowlarr = ProwlarrConn{URL: "http://prowlarr.example", APIKey: "k"}
	if err := conns.Save(ctx, cs); err != nil {
		t.Fatal(err)
	}
	again := NewConnections(st, cfg).Load(ctx)
	if again.Extensions.URL != "http://host.example" || again.Prowlarr.URL != "http://prowlarr.example" {
		t.Fatalf("reload = %+v", again)
	}

	// Without one in the configuration, the URL is the user's: it is
	// stored, and clearing it disables the source provider.
	free := NewConnections(st, config.Default())
	cs = free.Load(ctx)
	if cs.Managed.ExtensionsURL {
		t.Fatal("nothing configured, nothing managed")
	}
	cs.Extensions.URL = "http://host2.example"
	if err := free.Save(ctx, cs); err != nil {
		t.Fatal(err)
	}
	if got := NewConnections(st, config.Default()).Load(ctx); got.Extensions.URL != "http://host2.example" {
		t.Fatalf("stored url = %+v", got)
	}
	cs.Extensions.URL = ""
	if err := free.Save(ctx, cs); err != nil {
		t.Fatal(err)
	}
	if free.Registry().ForProtocol(provider.ProtocolSource) != nil || free.Host() != nil {
		t.Fatal("source provider should be gone without a host url")
	}

	if err := conns.Save(ctx, ConnectionSettings{
		Prowlarr: ProwlarrConn{URL: "prowlarr.example"}}); err == nil {
		t.Fatal("want validation error for scheme-less url")
	}
}

// Connections saved under the sidecar's old name still load.
func TestConnectionSettingsLegacyKey(t *testing.T) {
	_, st := setup(t)
	ctx := context.Background()
	if err := st.PutSetting(ctx, connectionsKey, []byte(`{"sourceHost":{"url":"http://old.example"},"qbittorrent":{"url":"http://qb.example"}}`)); err != nil {
		t.Fatal(err)
	}
	conns := NewConnections(st, config.Default())
	cs := conns.Load(ctx)
	if cs.Extensions.URL != "http://old.example" || len(cs.DownloadClients) != 1 ||
		cs.DownloadClients[0].URL != "http://qb.example" || cs.DownloadClients[0].ID != defaultClientID || cs.DownloadClients[0].Name == "" {
		t.Fatalf("legacy settings = %+v", cs)
	}
	// Saved again, they move to the new key and stay readable.
	if err := conns.Save(ctx, cs); err != nil {
		t.Fatal(err)
	}
	// A second client gets its own id; one without a URL is dropped.
	cs.DownloadClients = append(cs.DownloadClients, DownloadClientConn{Name: "VPN", URL: "http://vpn.example"}, DownloadClientConn{Name: "Empty"})
	if err := conns.Save(ctx, cs); err != nil {
		t.Fatal(err)
	}
	again := conns.Load(ctx)
	if again.Extensions.URL != "http://old.example" || len(again.DownloadClients) != 2 ||
		again.DownloadClients[0].ID != defaultClientID || again.DownloadClients[1].ID == "" || again.DownloadClients[1].ID == defaultClientID ||
		again.DownloadClients[1].Category != "sento" {
		t.Fatalf("after re-save = %+v", again)
	}
}
