package qbittorrent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeQBit is a qBittorrent 5.x stand-in whose sessions can be expired on
// demand (the real one drops idle sessions after an hour by default).
type fakeQBit struct {
	mu      sync.Mutex
	session int // current valid session number; cookies carry theirs
	logins  int
	adds    []string // content types of accepted add requests
}

func (f *fakeQBit) expire() {
	f.mu.Lock()
	f.session++
	f.mu.Unlock()
}

func (f *fakeQBit) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.URL.Path == "/api/v2/auth/login" {
		_ = r.ParseForm()
		if r.Form.Get("username") != "admin" || r.Form.Get("password") != "secret" {
			_, _ = w.Write([]byte("Fails."))
			return
		}
		f.logins++
		http.SetCookie(w, &http.Cookie{Name: "SID", Value: strings.Repeat("s", f.session+1), Path: "/"})
		w.WriteHeader(http.StatusNoContent)
		return
	}
	cookie, err := r.Cookie("SID")
	if err != nil || cookie.Value != strings.Repeat("s", f.session+1) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	switch r.URL.Path {
	case "/api/v2/torrents/add":
		f.adds = append(f.adds, r.Header.Get("Content-Type"))
		_, _ = w.Write([]byte("Ok."))
	case "/api/v2/torrents/info":
		_, _ = w.Write([]byte(`[{"hash":"ABC","name":"x","progress":1,"state":"uploading","save_path":"/dl"}]`))
	case "/api/v2/app/version":
		_, _ = w.Write([]byte("v5.1.0"))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func newFake(t *testing.T) (*fakeQBit, *Client) {
	t.Helper()
	fake := &fakeQBit{}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	return fake, New(srv.URL, "admin", "secret")
}

func TestLoginAndList(t *testing.T) {
	fake, c := newFake(t)
	torrents, err := c.List(context.Background(), "sento")
	if err != nil {
		t.Fatal(err)
	}
	if len(torrents) != 1 || !torrents[0].Completed() {
		t.Fatalf("torrents = %+v, want one completed", torrents)
	}
	if fake.logins != 1 {
		t.Fatalf("logins = %d, want 1", fake.logins)
	}
}

func TestBadCredentials(t *testing.T) {
	fake := &fakeQBit{}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	if err := New(srv.URL, "admin", "wrong").Ping(context.Background()); err == nil {
		t.Fatal("want a login error for bad credentials")
	}
}

// A .torrent upload is often the first call after hours of quiet (nothing
// polls qBittorrent while the queue is empty), so it is exactly the request
// that meets an expired session. It must log in again like every other call.
func TestAddTorrentFileSurvivesAnExpiredSession(t *testing.T) {
	fake, c := newFake(t)
	ctx := context.Background()
	if err := c.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	fake.expire()

	if err := c.AddTorrentFile(ctx, "x.torrent", []byte("d4:infod4:name1:xee"), "sento"); err != nil {
		t.Fatalf("upload after session expiry: %v", err)
	}
	if fake.logins != 2 {
		t.Fatalf("logins = %d, want a second login after expiry", fake.logins)
	}
	if len(fake.adds) != 1 || !strings.HasPrefix(fake.adds[0], "multipart/form-data") {
		t.Fatalf("accepted adds = %v, want one multipart upload", fake.adds)
	}

	fake.expire()
	if err := c.AddTorrent(ctx, "magnet:?xt=urn:btih:abc", "sento"); err != nil {
		t.Fatalf("magnet add after session expiry: %v", err)
	}
}
