package provider

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/h-wb/sento/internal/sourcehost"
)

// fakeHost serves two single-pixel "pages" per chapter and counts fetches.
// pageFetches is atomic: pages are now fetched concurrently.
type fakeHost struct {
	pageFetches atomic.Int64
	failPages   bool
}

// pngHeader makes DetectContentType say image/png.
var pngHeader = []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("x", 16))

func (f *fakeHost) Name() string { return "fake" }
func (f *fakeHost) Sources(context.Context) ([]sourcehost.Source, error) {
	return []sourcehost.Source{{ID: "src1", Name: "Fake Source", Lang: "en"}}, nil
}
func (f *fakeHost) Search(_ context.Context, _, q string) ([]sourcehost.Manga, error) {
	return []sourcehost.Manga{{Key: "/manga/op", Title: "One Piece"}}, nil
}
func (f *fakeHost) Chapters(_ context.Context, _, mangaKey string) ([]sourcehost.Chapter, error) {
	return []sourcehost.Chapter{
		{Key: mangaKey + "/ch-1100", Name: "Chapter 1100", Number: 1100},
	}, nil
}
func (f *fakeHost) Pages(_ context.Context, _, chapterKey string) ([]string, error) {
	if f.failPages {
		return nil, fmt.Errorf("site down")
	}
	return []string{chapterKey + "/p0", chapterKey + "/p1"}, nil
}
func (f *fakeHost) Page(_ context.Context, _, _ string) (io.ReadCloser, error) {
	f.pageFetches.Add(1)
	return io.NopCloser(strings.NewReader(string(pngHeader))), nil
}

// testEntries serves one source indexer entry ("1" → src1).
func testEntries(ctx context.Context) ([]IndexerEntry, error) {
	return []IndexerEntry{{ID: 1, Name: "Fake Source", Enabled: true, SourceID: "src1"}}, nil
}

func waitDone(t *testing.T, p Provider, externalID string) DownloadState {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		st, err := p.State(context.Background(), externalID)
		if err != nil {
			t.Fatal(err)
		}
		if st.Done || (!st.InFlight && st.Detail != "" && st.Detail != "restarted") {
			return st
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("download did not settle")
	return DownloadState{}
}

// The full source download lifecycle: grab → pages staged as CBZ → fetch →
// cleanup. URL-shaped keys (with slashes) must round-trip through the
// external id.
func TestSourceDownloadLifecycle(t *testing.T) {
	host := &fakeHost{}
	p := NewSource(host, t.TempDir(), DownloadConfig{}, testEntries)
	ctx := context.Background()

	cands, err := p.Search(ctx, SearchRequest{
		Bindings: []Binding{{BindingID: 1, SourceID: "1", SourceMangaKey: "/manga/op", SourceName: "Fake"}},
	})
	if err != nil || len(cands) != 1 {
		t.Fatalf("search: %v %v", cands, err)
	}

	externalID, err := p.Grab(ctx, cands[0])
	if err != nil {
		t.Fatal(err)
	}
	if externalID != "src1|/manga/op/ch-1100" {
		t.Fatalf("external id = %q", externalID)
	}

	if st := waitDone(t, p, externalID); !st.Done {
		t.Fatalf("want done, got %+v", st)
	}

	payload, err := p.Fetch(ctx, externalID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(payload.Stream)
	_ = payload.Stream.Close()
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(strings.NewReader(string(data)), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if len(zr.File) != 2 || zr.File[0].Name != "0001.png" {
		t.Fatalf("staged CBZ entries: %v", zr.File)
	}

	if err := p.Cleanup(ctx, externalID); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Fetch(ctx, externalID); err == nil {
		t.Fatal("fetch after cleanup should fail")
	}
}

// A restart loses the in-memory job table: State on an unstaged item must
// relaunch the download instead of reporting it dead.
func TestSourceDownloadResume(t *testing.T) {
	dir := t.TempDir()
	host := &fakeHost{}
	// Fresh provider simulates a process restart: queue knows the id, the
	// provider doesn't.
	p := NewSource(host, dir, DownloadConfig{}, testEntries)
	externalID := "src1|/manga/op/ch-1100"

	st, err := p.State(context.Background(), externalID)
	if err != nil {
		t.Fatal(err)
	}
	if !st.InFlight {
		t.Fatalf("want restart-in-flight, got %+v", st)
	}
	if st = waitDone(t, p, externalID); !st.Done {
		t.Fatalf("want done after resume, got %+v", st)
	}
}

// Failed jobs report their error; Recover clears them for a retry.
func TestSourceDownloadFailureAndRecover(t *testing.T) {
	host := &fakeHost{failPages: true}
	p := NewSource(host, t.TempDir(), DownloadConfig{}, testEntries)
	externalID := "src1|/manga/op/ch-1100"

	st, _ := p.State(context.Background(), externalID) // kicks the job
	if st.Done {
		t.Fatalf("unexpected done: %+v", st)
	}
	st = waitDone(t, p, externalID)
	if st.Done || st.InFlight || !strings.Contains(st.Detail, "site down") {
		t.Fatalf("want failed state, got %+v", st)
	}

	host.failPages = false
	if err := p.(*SourceProvider).Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if st = waitDone(t, p, externalID); !st.Done {
		t.Fatalf("want done after recover, got %+v", st)
	}
}

// orderHost serves n pages whose bytes encode their index, and makes later
// pages return sooner so they complete out of order. The assembled CBZ must
// still be in page order — the byte-identical guarantee of concurrent fetch.
type orderHost struct{ n int }

func (h *orderHost) Name() string { return "fake" }
func (h *orderHost) Sources(context.Context) ([]sourcehost.Source, error) {
	return []sourcehost.Source{{ID: "src1", Name: "Fake Source", Lang: "en"}}, nil
}
func (h *orderHost) Search(_ context.Context, _, _ string) ([]sourcehost.Manga, error) {
	return []sourcehost.Manga{{Key: "/manga/op", Title: "One Piece"}}, nil
}
func (h *orderHost) Chapters(_ context.Context, _, mangaKey string) ([]sourcehost.Chapter, error) {
	return []sourcehost.Chapter{{Key: mangaKey + "/ch-1", Name: "Chapter 1", Number: 1}}, nil
}
func (h *orderHost) Pages(_ context.Context, _, chapterKey string) ([]string, error) {
	refs := make([]string, h.n)
	for i := range refs {
		refs[i] = fmt.Sprintf("%s/p%d", chapterKey, i)
	}
	return refs, nil
}
func (h *orderHost) Page(_ context.Context, _, ref string) (io.ReadCloser, error) {
	var idx int
	_, _ = fmt.Sscanf(ref[strings.LastIndex(ref, "/p")+2:], "%d", &idx)
	// Later pages return sooner: forces out-of-order completion.
	time.Sleep(time.Duration(h.n-idx) * 2 * time.Millisecond)
	body := string(pngHeader) + fmt.Sprintf("PAGE-%d", idx)
	return io.NopCloser(strings.NewReader(body)), nil
}

// Concurrent page fetches that complete out of order must still assemble
// into a page-ordered CBZ with each page's own bytes.
func TestSourceDownloadOrdering(t *testing.T) {
	const n = 8
	host := &orderHost{n: n}
	p := NewSource(host, t.TempDir(), DownloadConfig{PageConcurrency: 4}, testEntries)
	ctx := context.Background()

	cands, err := p.Search(ctx, SearchRequest{
		Bindings: []Binding{{BindingID: 1, SourceID: "1", SourceMangaKey: "/manga/op", SourceName: "Fake"}},
	})
	if err != nil || len(cands) != 1 {
		t.Fatalf("search: %v %v", cands, err)
	}
	externalID, err := p.Grab(ctx, cands[0])
	if err != nil {
		t.Fatal(err)
	}
	if st := waitDone(t, p, externalID); !st.Done {
		t.Fatalf("want done, got %+v", st)
	}

	payload, err := p.Fetch(ctx, externalID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(payload.Stream)
	_ = payload.Stream.Close()
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(strings.NewReader(string(data)), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if len(zr.File) != n {
		t.Fatalf("got %d entries, want %d", len(zr.File), n)
	}
	for i, zf := range zr.File {
		if want := fmt.Sprintf("%04d.png", i+1); zf.Name != want {
			t.Fatalf("entry %d name = %q, want %q", i, zf.Name, want)
		}
		rc, err := zf.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		if want := fmt.Sprintf("PAGE-%d", i); !strings.HasSuffix(string(b), want) {
			t.Fatalf("entry %d (%s) missing %q", i, zf.Name, want)
		}
	}
}
