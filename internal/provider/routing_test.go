package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/h-wb/sento/internal/qbittorrent"
)

// Not knowing which client an indexer uses must stop the grab: falling
// back to the default would send a release meant for one client (a VPN
// one, say) to another.
func TestClientRoutingFailsClosed(t *testing.T) {
	clients := []TorrentClient{{ID: "open"}, {ID: "vpn"}}
	entries := func(list []IndexerEntry, err error) func(context.Context) ([]IndexerEntry, error) {
		return func(context.Context) ([]IndexerEntry, error) { return list, err }
	}
	ctx := context.Background()

	p := &TorrentProvider{clients: clients, entries: entries(nil, errors.New("database unavailable"))}
	if got, err := p.clientFor(ctx, "2"); err == nil {
		t.Errorf("failed lookup chose %q", got.ID)
	}
	p = &TorrentProvider{clients: clients, entries: entries([]IndexerEntry{{ID: 2, DownloadClient: "vpn"}}, nil)}
	if got, err := p.clientFor(ctx, "9"); err == nil {
		t.Errorf("unknown indexer chose %q", got.ID)
	}
	if got, err := p.clientFor(ctx, "2"); err != nil || got.ID != "vpn" {
		t.Errorf("named client: got %q, %v", got.ID, err)
	}
	p = &TorrentProvider{clients: clients, entries: entries([]IndexerEntry{{ID: 2}}, nil)}
	if got, err := p.clientFor(ctx, "2"); err != nil || got.ID != "open" {
		t.Errorf("indexer naming no client should use the default: got %q, %v", got.ID, err)
	}
	p = &TorrentProvider{clients: clients[:1], entries: entries(nil, errors.New("database unavailable"))}
	if got, err := p.clientFor(ctx, "2"); err != nil || got.ID != "open" {
		t.Errorf("a single client leaves nothing to choose: got %q, %v", got.ID, err)
	}
}

// A job is polled and imported from the client it was sent to. The same
// torrent sitting in another client is not this job's download.
func TestDownloadStaysWithItsClient(t *testing.T) {
	const hash = "11627358221eb249736e653a48948edf7b334bd3"
	client := func(savePath string) *qbittorrent.Client {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/v2/torrents/info":
				_, _ = w.Write([]byte(`[{"hash":"` + hash + `","state":"uploading","progress":1,"save_path":"` + savePath + `"}]`))
			case "/api/v2/torrents/files":
				_, _ = w.Write([]byte(`[{"name":"book.cbz"}]`))
			default:
				_, _ = w.Write([]byte("Ok."))
			}
		}))
		t.Cleanup(srv.Close)
		return qbittorrent.New(srv.URL, "", "")
	}
	p := &TorrentProvider{
		entries: func(context.Context) ([]IndexerEntry, error) {
			return []IndexerEntry{{ID: 2, DownloadClient: "vpn"}}, nil
		},
		clients: []TorrentClient{{ID: "open", Client: client("/open")}, {ID: "vpn", Client: client("/vpn")}},
	}
	ctx := context.Background()
	cand := Candidate{SiteID: "2", Torrent: &TorrentRef{InfoHash: hash, MagnetURL: "magnet:?xt=urn:btih:" + hash}}
	id, err := p.ClientFor(ctx, cand)
	if err != nil || id != "vpn" {
		t.Fatalf("ClientFor = %q, %v", id, err)
	}
	external, err := p.Grab(InClient(ctx, id), cand)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := p.Fetch(InClient(ctx, id), external)
	if err != nil || payload.Dir != "/vpn" {
		t.Fatalf("fetch read %v (%v), want the VPN client's copy", payload, err)
	}
	if _, err := p.State(InClient(ctx, "removed"), external); err == nil {
		t.Error("a job whose client is gone must not be answered by another client")
	}
}
