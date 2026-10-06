package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

// payloadProvider is a download client whose every item is finished and
// sits in dir (the torrent shape: files left in place for seeding).
type payloadProvider struct {
	dir   string
	files []string
}

func (p *payloadProvider) ID() string       { return "torrent" }
func (p *payloadProvider) Protocol() string { return provider.ProtocolTorrent }
func (p *payloadProvider) Sites(context.Context) ([]provider.Site, error) {
	return nil, nil
}
func (p *payloadProvider) Search(context.Context, provider.SearchRequest) ([]provider.Candidate, error) {
	return nil, nil
}
func (p *payloadProvider) Grab(context.Context, provider.Candidate) (string, error) {
	return "", nil
}
func (p *payloadProvider) State(context.Context, string) (provider.DownloadState, error) {
	return provider.DownloadState{Done: true}, nil
}
func (p *payloadProvider) Fetch(context.Context, string) (*provider.Payload, error) {
	return &provider.Payload{Dir: p.dir, Files: p.files}, nil
}
func (p *payloadProvider) Cleanup(context.Context, string) error { return nil }

func queueVolumes(t *testing.T, st *store.Store, seriesID, collectionID int64, hash string, numbers ...int64) int64 {
	t.Helper()
	slots := make([]store.ExpectedSlot, 0, len(numbers))
	for _, n := range numbers {
		targets, err := st.ListTargets(context.Background(), seriesID)
		if err != nil {
			t.Fatal(err)
		}
		works, err := st.ListWorks(context.Background(), collectionID)
		if err != nil {
			t.Fatal(err)
		}
		var targetID, workID int64
		for _, target := range targets {
			if target.CollectionID == collectionID && target.IsDefault {
				targetID = target.ID
			}
		}
		for _, work := range works {
			if work.SequenceNum == n {
				workID = work.ID
			}
		}
		slots = append(slots, store.ExpectedSlot{Ledger: "volume", CollectionID: collectionID, Number: n, TargetID: targetID, WorkID: workID})
	}
	raw, _ := json.Marshal(slots)
	id, err := st.AddQueueItem(context.Background(), store.QueueItem{
		SeriesID: seriesID, Protocol: provider.ProtocolTorrent, ExternalID: hash,
		Title: "ONE PIECE v01-v02", ExpectedSlots: raw,
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// The torrent path hardlinks payload files into the library. Grabbing the
// same release again (after a failed first import, or a manual re-grab)
// must converge: same library files, no duplicate rows, and — above all —
// the seeding payload left byte-for-byte intact.
func TestImportTorrentPayloadIsIdempotent(t *testing.T) {
	ctx := context.Background()
	lib, st := setup(t)
	seriesID, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377})
	if err != nil {
		t.Fatal(err)
	}
	cols, _ := st.ListCollections(ctx, seriesID)
	var standard int64
	for _, c := range cols {
		if c.EditionName == "Standard Edition" {
			standard = c.ID
		}
	}

	downloads, library := t.TempDir(), t.TempDir()
	files := []string{"ONE PIECE v01.cbz", "ONE PIECE v02.cbz", "cover.jpg"}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(downloads, name), []byte("payload "+name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	im := NewImporter(st, provider.NewRegistry(&payloadProvider{dir: downloads, files: files}), library, nil)

	for pass := 1; pass <= 2; pass++ {
		qid := queueVolumes(t, st, seriesID, standard, "abc123", 100, 200)
		n, err := im.ProcessQueue(ctx)
		if err != nil || n != 1 {
			t.Fatalf("pass %d: imported %d (err %v), want 1", pass, n, err)
		}
		items, _ := st.ListQueueItems(ctx, false)
		for _, it := range items {
			if it.ID == qid && it.Status != store.QueueImported {
				t.Fatalf("pass %d: queue item is %q (%s), want imported", pass, it.Status, it.Error)
			}
		}
		for _, name := range files[:2] {
			got, err := os.ReadFile(filepath.Join(downloads, name))
			if err != nil || string(got) != "payload "+name {
				t.Fatalf("pass %d: seeding payload %s = %q (err %v), want it untouched", pass, name, got, err)
			}
		}
	}

	libFiles, err := st.ListFiles(ctx, seriesID)
	if err != nil {
		t.Fatal(err)
	}
	if len(libFiles) != 2 {
		t.Fatalf("library has %d file rows, want 2: %+v", len(libFiles), libFiles)
	}
	for _, f := range libFiles {
		got, err := os.ReadFile(f.Path)
		if err != nil || len(got) == 0 {
			t.Fatalf("library file %s unreadable or empty (err %v)", f.Path, err)
		}
	}
	links, _ := st.SlotLinks(ctx, seriesID)
	for _, n := range []int64{100, 200} {
		if len(links[store.SlotKey("volume", standard, n)]) != 1 {
			t.Fatalf("volume %d has %d links, want 1", n/100, len(links[store.SlotKey("volume", standard, n)]))
		}
	}
}

func TestMapPath(t *testing.T) {
	im := NewImporter(nil, nil, "", []string{"/data=/mnt/seedbox", "/data/complete=/mnt/complete", "bogus"})
	for remote, want := range map[string]string{
		"/data/complete/sento":   "/mnt/complete/sento", // longest prefix wins
		"/data/incoming/x":       "/mnt/seedbox/incoming/x",
		"/elsewhere/unmapped":    "/elsewhere/unmapped",
		"/data/complete":         "/mnt/complete",
		"/database/not-a-prefix": "/database/not-a-prefix",
	} {
		if got := im.mapPath(remote); got != want {
			t.Errorf("mapPath(%q) = %q, want %q", remote, got, want)
		}
	}
}
