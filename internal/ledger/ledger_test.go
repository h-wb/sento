package ledger

import (
	"testing"
	"time"

	"github.com/h-wb/sento/internal/store"
)

var now = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

func work(seq int64, countType, date string) store.Work {
	return store.Work{SequenceNum: seq, SequenceStr: "s", CountType: countType, ReleaseDate: date}
}

func TestVolumeSlots(t *testing.T) {
	col := store.Collection{ID: 7, Monitored: true}
	works := []store.Work{
		work(100, "main", "2020-01-01"), // released, owned
		work(200, "main", "2020-02-01"), // released, missing → wanted
		work(300, "main", "2099-01-01"), // future → not wanted
		work(400, "extra", "2020-01-01"), // extras unmonitored by default (D11)
	}
	links := map[string][]int64{store.SlotKey("volume", 7, 100): {42}}

	slots := BuildVolumeSlots(col, works, links, nil, now)
	if len(slots) != 4 {
		t.Fatalf("want 4 slots, got %d", len(slots))
	}
	for i, want := range []struct {
		released, monitored, wanted bool
		files                       int
	}{
		{true, true, false, 1},  // owned
		{true, true, true, 0},   // wanted
		{false, true, false, 0}, // future
		{true, false, false, 0}, // extra
	} {
		s := slots[i]
		if s.Released != want.released || s.Monitored != want.monitored ||
			s.Wanted != want.wanted || len(s.FileIDs) != want.files {
			t.Errorf("slot %d: got %+v, want %+v", i, s, want)
		}
	}
}

func TestVolumeSlotsUnmonitoredCollection(t *testing.T) {
	col := store.Collection{ID: 7, Monitored: false}
	slots := BuildVolumeSlots(col, []store.Work{work(100, "main", "2020-01-01")}, nil, nil, now)
	if slots[0].Monitored || slots[0].Wanted {
		t.Errorf("unmonitored collection must not want anything: %+v", slots[0])
	}
}

func TestVolumeSlotsOverrides(t *testing.T) {
	col := store.Collection{ID: 7, Monitored: true}
	works := []store.Work{
		work(100, "main", "2020-01-01"),
		work(400, "extra", "2020-01-01"),
	}
	overrides := map[int64]bool{100: false, 400: true}
	slots := BuildVolumeSlots(col, works, nil, overrides, now)
	if slots[0].Monitored {
		t.Error("main slot overridden off must be unmonitored")
	}
	if !slots[1].Monitored || !slots[1].Wanted {
		t.Errorf("extra overridden on must be monitored+wanted: %+v", slots[1])
	}
}

// Undated works are released iff they sit at/below the highest dated
// sequence; with no dates at all, everything counts as released.
func TestVolumeSlotsUndatedReleases(t *testing.T) {
	col := store.Collection{ID: 7, Monitored: true}
	mixed := BuildVolumeSlots(col, []store.Work{
		work(100, "main", ""),           // below max dated → released
		work(200, "main", "2020-01-01"), // dated
		work(300, "main", ""),           // above max dated → unreleased
	}, nil, nil, now)
	if !mixed[0].Released || !mixed[1].Released || mixed[2].Released {
		t.Errorf("mixed dating: got released=%v,%v,%v want true,true,false",
			mixed[0].Released, mixed[1].Released, mixed[2].Released)
	}

	undated := BuildVolumeSlots(col, []store.Work{work(100, "main", ""), work(200, "main", "")}, nil, nil, now)
	for i, s := range undated {
		if !s.Released {
			t.Errorf("fully undated collection: slot %d should be released", i)
		}
	}
}

func TestChapterSlots(t *testing.T) {
	l := store.ChapterLedger{Monitored: true, CutoffNum: 300} // monitor from c4 (cutoff excluded)
	links := map[string][]int64{store.SlotKey("chapter", 0, 500): {9}}

	slots := BuildChapterSlots(l, 6, links, nil)
	if len(slots) != 6 {
		t.Fatalf("want 6 slots, got %d", len(slots))
	}
	for i, want := range []struct{ monitored, wanted bool }{
		{false, false}, // c1
		{false, false}, // c2
		{false, false}, // c3 = cutoff, excluded
		{true, true},   // c4
		{true, false},  // c5 owned
		{true, true},   // c6
	} {
		s := slots[i]
		if s.Monitored != want.monitored || s.Wanted != want.wanted {
			t.Errorf("c%d: got monitored=%v wanted=%v, want %+v", i+1, s.Monitored, s.Wanted, want)
		}
	}
}

func TestChapterSlotsOverridesAndEmpty(t *testing.T) {
	l := store.ChapterLedger{Monitored: true, CutoffNum: 0}
	slots := BuildChapterSlots(l, 2, nil, map[int64]bool{100: false})
	if slots[0].Monitored || !slots[1].Monitored {
		t.Errorf("override: got %v,%v want false,true", slots[0].Monitored, slots[1].Monitored)
	}

	if got := BuildChapterSlots(l, 0, nil, nil); got != nil {
		t.Errorf("zero chapters: want nil, got %d slots", len(got))
	}

	paused := store.ChapterLedger{Monitored: false}
	for _, s := range BuildChapterSlots(paused, 3, nil, nil) {
		if s.Monitored || s.Wanted {
			t.Errorf("paused ledger must not monitor/want: %+v", s)
		}
	}
}
