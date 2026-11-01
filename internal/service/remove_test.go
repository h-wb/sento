package service

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

// removingProvider is a download client that records what it was asked
// to take back.
type removingProvider struct {
	payloadProvider
	removed []string
}

func (p *removingProvider) Remove(_ context.Context, externalID string) error {
	p.removed = append(p.removed, externalID)
	return nil
}

// Removing a series deletes, when asked, its files and the folders that
// leaves empty (never the library), and the torrents only it uses.
func TestRemoveSeriesTakesItsFilesAndOwnTorrents(t *testing.T) {
	_, st, sid, c, ws, version := modelFixture(t)
	ctx := context.Background()
	other := testProvider()
	other.series.MangaBakaID, other.series.Title = 900, "The Enigma of Amigara Fault"
	other.collections = nil
	oid, err := NewLibrary(st, other).AddSeries(ctx, AddSeriesRequest{MangaBakaID: 900})
	if err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	dir := filepath.Join(root, "Volumes", "ONE PIECE [mangabaka-377]", "Standard Edition [en]")
	if err = os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "ONE PIECE v01.cbz")
	if err = os.WriteFile(path, []byte("pages"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err = st.AddFile(ctx, store.File{SeriesID: sid, Path: path, Size: 5}, nil, []store.BookLink{{Ledger: c.Type, EditionID: c.ID, Number: ws[0].SequenceNum, VersionID: version.ID, BookID: ws[0].ID}}); err != nil {
		t.Fatal(err)
	}
	for _, q := range []store.QueueItem{
		{SeriesID: sid, Protocol: provider.ProtocolTorrent, ExternalID: "own", Title: "ONE PIECE v01"},
		{SeriesID: sid, Protocol: provider.ProtocolTorrent, ExternalID: "pack", Title: "Shonen pack"},
		{SeriesID: oid, Protocol: provider.ProtocolTorrent, ExternalID: "pack", Title: "Shonen pack"},
	} {
		if _, err = st.AddQueueItem(ctx, q); err != nil {
			t.Fatal(err)
		}
	}

	client := &removingProvider{}
	im := NewImporter(st, provider.NewRegistry(client), root, nil)
	got, err := im.RemoveSeries(ctx, sid, RemoveOptions{DeleteFiles: true, RemoveDownloads: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.FilesDeleted != 1 || got.DownloadsRemoved != 1 || len(got.Kept) != 1 {
		t.Fatalf("removed %+v", got)
	}
	if !slices.Equal(client.removed, []string{"own"}) {
		t.Fatalf("torrents removed: %v (the pack is the other series' too)", client.removed)
	}
	if _, err = os.Stat(filepath.Join(root, "Volumes")); !os.IsNotExist(err) {
		t.Fatalf("emptied folders left behind: %v", err)
	}
	if _, err = os.Stat(root); err != nil {
		t.Fatalf("the library itself was removed: %v", err)
	}
	if _, err = st.GetSeries(ctx, sid); err == nil {
		t.Fatal("the series is still there")
	}
}

// Without the options, a removal is what it always was: records only.
func TestRemoveSeriesKeepsFilesAndTorrentsByDefault(t *testing.T) {
	_, st, sid, c, ws, version := modelFixture(t)
	ctx := context.Background()
	ownedModelFile(t, st, sid, c, ws[0], version)
	files, err := st.ListFiles(ctx, sid)
	if err != nil || len(files) != 1 {
		t.Fatal(files, err)
	}
	if _, err = st.AddQueueItem(ctx, store.QueueItem{SeriesID: sid, Protocol: provider.ProtocolTorrent, ExternalID: "own", Title: "x"}); err != nil {
		t.Fatal(err)
	}
	client := &removingProvider{}
	if _, err = NewImporter(st, provider.NewRegistry(client), t.TempDir(), nil).RemoveSeries(ctx, sid, RemoveOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(files[0].Path); err != nil || len(client.removed) != 0 {
		t.Fatalf("a plain removal touched disk or the client: %v %v", err, client.removed)
	}
}

// Deleting a file a card pinned is refused; otherwise the file goes from
// disk, with the folder it leaves empty.
func TestDeleteFileRespectsPins(t *testing.T) {
	_, st, sid, c, ws, version := modelFixture(t)
	ctx := context.Background()
	root := t.TempDir()
	dir := filepath.Join(root, "ONE PIECE")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "v01.cbz")
	if err := os.WriteFile(path, []byte("pages"), 0o644); err != nil {
		t.Fatal(err)
	}
	fid, err := st.AddFile(ctx, store.File{SeriesID: sid, Path: path, Size: 5}, nil, []store.BookLink{{Ledger: c.Type, EditionID: c.ID, Number: ws[0].SequenceNum, VersionID: version.ID, BookID: ws[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	im := NewImporter(st, provider.NewRegistry(), root, nil)
	if err = st.PinCoverage(ctx, sid, store.AssetCoverage{FileID: fid, VersionID: version.ID, BookID: ws[0].ID, Pinned: true}); err != nil {
		t.Fatal(err)
	}
	if err = im.DeleteFile(ctx, fid); err == nil {
		t.Fatal("a pinned file was deleted")
	}
	if err = st.PinCoverage(ctx, sid, store.AssetCoverage{FileID: fid, VersionID: version.ID, BookID: ws[0].ID}); err != nil {
		t.Fatal(err)
	}
	if err = im.DeleteFile(ctx, fid); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("file or its folder left behind: %v", err)
	}
}

// A removed series takes its history with it.
func TestRemoveSeriesForgetsItsHistory(t *testing.T) {
	_, st, sid, _, _, _ := modelFixture(t)
	ctx := context.Background()
	if err := st.AddDecision(ctx, sid, store.DecisionBody{Event: "grabbed", Title: "ONE PIECE v01"}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewImporter(st, provider.NewRegistry(), t.TempDir(), nil).RemoveSeries(ctx, sid, RemoveOptions{}); err != nil {
		t.Fatal(err)
	}
	if left, _ := st.ListDecisions(ctx, 0, 10); len(left) != 0 {
		t.Fatalf("history outlived its series: %+v", left)
	}
}
