package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// wantedNumbers extracts the volume numbers (sequence hundredths) of a
// series' wanted volume rows.
func wantedNumbers(rows []WantedRow, sid int64) []int64 {
	var out []int64
	for _, r := range rows {
		if r.SeriesID == sid && r.Ledger == LedgerVolume && r.Book.CountType == "main" {
			out = append(out, r.Book.SequenceNum)
		}
	}
	return out
}

func containsInt(xs []int64, want int64) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// TestWantedListsMissingAndRescanReopens locks the contract the
// library-scan loop rests on: wanted is what the ledger says, a scanned
// file closes its book, and a vanished file reopens it once presence is
// re-checked.
func TestWantedListsMissingAndRescanReopens(t *testing.T) {
	t.Parallel()
	lib, st := setup(t)
	ctx := context.Background()
	sid, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377})
	if err != nil {
		t.Fatal(err)
	}

	// Default monitor: the standard edition's volumes are wanted, chapter
	// books are not (cutoff at latest).
	rows, err := lib.Wanted(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("a fresh series should have wanted books")
	}
	for _, r := range rows {
		if r.SeriesID != sid {
			t.Errorf("wanted row for series %d, want %d", r.SeriesID, sid)
		}
		if r.State != "missing" || r.Reason != "" {
			t.Errorf("row %d: state %q reason %q, want a plain missing row", r.Book.SequenceNum, r.State, r.Reason)
		}
	}
	if !containsInt(wantedNumbers(rows, sid), 100) {
		t.Errorf("volume 1 should be wanted, got %v", wantedNumbers(rows, sid))
	}

	// Drop one archive into the series folder and scan: that book stops
	// being wanted.
	dir := t.TempDir()
	file := filepath.Join(dir, "ONE PIECE v01 (Digital).cbz")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	sc := NewScanner(st, dir)
	if _, err := sc.ScanSeriesFolder(ctx, sid, dir); err != nil {
		t.Fatal(err)
	}
	rows, err = lib.Wanted(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if containsInt(wantedNumbers(rows, sid), 100) {
		t.Error("volume 1 should not be wanted while its file is on disk")
	}

	// A file that vanishes reopens its book: delete the archive, re-check
	// presence the way the library-scan loop does, and the book is wanted
	// again.
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := st.CheckFilePresence(ctx, sid); err != nil {
		t.Fatal(err)
	}
	rows, err = lib.Wanted(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !containsInt(wantedNumbers(rows, sid), 100) {
		t.Errorf("volume 1 should be wanted again after its file vanished, got %v", wantedNumbers(rows, sid))
	}

	// Unmonitored series never contribute rows.
	if err := st.SetSeriesMonitored(ctx, sid, false); err != nil {
		t.Fatal(err)
	}
	rows, err = lib.Wanted(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(wantedNumbers(rows, sid)) != 0 {
		t.Error("an unmonitored series should have no wanted rows")
	}
}
