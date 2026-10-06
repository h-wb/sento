package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/h-wb/sento/internal/qbittorrent"
)

// Some indexers have no .torrent for a release: their download link
// redirects to a magnet (Nyaa through Prowlarr). The grab must hand that
// magnet to the client instead of trying to fetch it over HTTP.
func TestGrabFollowsDownloadLinkToMagnet(t *testing.T) {
	const magnet = "magnet:?xt=urn:btih:11627358221eb249736e653a48948edf7b334bd3&dn=Nausicaa"
	indexer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/magnet":
			http.Redirect(w, r, magnet, http.StatusFound)
		case "/hop":
			http.Redirect(w, r, "/magnet", http.StatusFound)
		case "/file":
			_, _ = w.Write([]byte("d4:infod4:name1:aee"))
		default:
			_, _ = w.Write([]byte("<html>login</html>"))
		}
	}))
	defer indexer.Close()

	var added []string
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/torrents/add" {
			_ = r.ParseMultipartForm(1 << 20)
			_ = r.ParseForm()
			added = append(added, r.FormValue("urls"))
		}
		_, _ = w.Write([]byte("Ok."))
	}))
	defer qb.Close()

	ctx := context.Background()
	for _, path := range []string{"/magnet", "/hop"} {
		data, got, err := fetchTorrent(ctx, indexer.URL+path)
		if err != nil || got != magnet || data != nil {
			t.Fatalf("%s: data=%q magnet=%q err=%v", path, data, got, err)
		}
	}
	if data, got, err := fetchTorrent(ctx, indexer.URL+"/file"); err != nil || got != "" || len(data) == 0 {
		t.Fatalf("a real .torrent: data=%q magnet=%q err=%v", data, got, err)
	}
	if _, _, err := fetchTorrent(ctx, indexer.URL+"/login"); err == nil || !strings.Contains(err.Error(), "not a .torrent") {
		t.Fatalf("a login page should be refused: %v", err)
	}

	p := &TorrentProvider{clients: []TorrentClient{{ID: "default", Name: "qBittorrent", Client: qbittorrent.New(qb.URL, "", ""), Category: "sento"}}}
	id, err := p.Grab(ctx, Candidate{Title: "Nausicaa", Torrent: &TorrentRef{GUID: "g", DownloadURL: indexer.URL + "/magnet"}})
	if err != nil {
		t.Fatal(err)
	}
	if id != "11627358221eb249736e653a48948edf7b334bd3" {
		t.Errorf("external id = %q, want the magnet's infohash", id)
	}
	if len(added) != 1 || added[0] != magnet {
		t.Errorf("handed to the client: %q, want the magnet", added)
	}
}

// With several download clients, a release goes to the one its indexer
// names, the default otherwise, and nowhere when that client is gone.
func TestGrabRoutesToTheIndexersClient(t *testing.T) {
	const magnet = "magnet:?xt=urn:btih:11627358221eb249736e653a48948edf7b334bd3&dn=x"
	type fake struct {
		srv     *httptest.Server
		added   []string
		holding string // infohash this client reports
		down    bool
	}
	client := func() *fake {
		f := &fake{}
		f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case f.down:
				w.WriteHeader(http.StatusBadGateway)
			case r.URL.Path == "/api/v2/torrents/add":
				_ = r.ParseForm()
				f.added = append(f.added, r.FormValue("urls")+"|"+r.FormValue("category"))
				_, _ = w.Write([]byte("Ok."))
			case r.URL.Path == "/api/v2/torrents/info":
				if f.holding != "" {
					_, _ = w.Write([]byte(`[{"hash":"` + f.holding + `","state":"uploading","progress":1,"save_path":"/dl"}]`))
				} else {
					_, _ = w.Write([]byte(`[]`))
				}
			default:
				_, _ = w.Write([]byte("Ok."))
			}
		}))
		t.Cleanup(f.srv.Close)
		return f
	}
	open, vpn := client(), client()
	entries := func(context.Context) ([]IndexerEntry, error) {
		return []IndexerEntry{
			{ID: 1, Name: "Public", Enabled: true},
			{ID: 2, Name: "Private", Enabled: true, DownloadClient: "vpn"},
			{ID: 3, Name: "Orphan", Enabled: true, DownloadClient: "removed"},
		}, nil
	}
	p := NewTorrent(entries,
		TorrentClient{ID: "default", Name: "Direct", Client: qbittorrent.New(open.srv.URL, "", ""), Category: "sento"},
		TorrentClient{ID: "vpn", Name: "VPN", Client: qbittorrent.New(vpn.srv.URL, "", ""), Category: "sento-vpn"},
	)
	ctx := context.Background()
	grab := func(site string) error {
		_, err := p.Grab(ctx, Candidate{Title: "x", SiteID: site, Torrent: &TorrentRef{GUID: "g", MagnetURL: magnet}})
		return err
	}
	if err := grab("1"); err != nil {
		t.Fatal(err)
	}
	if err := grab("2"); err != nil {
		t.Fatal(err)
	}
	if len(open.added) != 1 || open.added[0] != magnet+"|sento" {
		t.Errorf("default client got %q", open.added)
	}
	if len(vpn.added) != 1 || vpn.added[0] != magnet+"|sento-vpn" {
		t.Errorf("VPN client got %q", vpn.added)
	}
	if err := grab("3"); err == nil || !strings.Contains(err.Error(), "no longer exists") {
		t.Errorf("an indexer pointing at a removed client must not fall back: %v", err)
	}
	if len(open.added) != 1 || len(vpn.added) != 1 {
		t.Errorf("the orphan grab reached a client: %q %q", open.added, vpn.added)
	}

	// A download is found in whichever client holds it.
	vpn.holding = "11627358221eb249736e653a48948edf7b334bd3"
	if st, err := p.State(ctx, vpn.holding); err != nil || !st.Done {
		t.Errorf("state from the second client: %+v %v", st, err)
	}
	// Not found anywhere, and one client could not be asked: unknown, not gone.
	vpn.down = true
	if _, err := p.State(ctx, "ffffffffffffffffffffffffffffffffffffffff"); err == nil || !strings.Contains(err.Error(), "VPN") {
		t.Errorf("an unreachable client should make the answer an error: %v", err)
	}
}
