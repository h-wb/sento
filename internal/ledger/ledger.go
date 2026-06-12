// Package ledger derives volume/chapter slots from stored metadata and file
// links (spec.md §4.2). Slots are computed, never stored.
package ledger

import (
	"time"

	"github.com/h-wb/sento/internal/store"
)

type VolumeSlot struct {
	Number      int64   `json:"number"` // hundredths
	SequenceStr string  `json:"sequenceStr"`
	CountType   string  `json:"countType"`
	ReleaseDate string  `json:"releaseDate,omitempty"`
	Monitored   bool    `json:"monitored"`
	Released    bool    `json:"released"`
	FileIDs     []int64 `json:"fileIds,omitempty"`
	Wanted      bool    `json:"wanted"`
}

type ChapterSlot struct {
	Number    int64   `json:"number"` // hundredths
	Monitored bool    `json:"monitored"`
	FileIDs   []int64 `json:"fileIds,omitempty"`
	Wanted    bool    `json:"wanted"`
}

// BuildVolumeSlots derives slots for one collection. Default monitoring:
// every count_type=main work iff the collection is monitored; extras are
// unmonitored by default (D11). Overrides are applied on top when present.
func BuildVolumeSlots(col store.Collection, works []store.Work, links map[string][]int64, overrides map[int64]bool, now time.Time) []VolumeSlot {
	today := now.Format("2006-01-02")
	anyDate := false
	var maxDated int64
	for _, w := range works {
		if w.ReleaseDate != "" {
			anyDate = true
			if w.SequenceNum > maxDated {
				maxDated = w.SequenceNum
			}
		}
	}

	slots := make([]VolumeSlot, 0, len(works))
	for _, w := range works {
		monitored := col.Monitored && w.CountType == "main"
		if ov, ok := overrides[w.SequenceNum]; ok {
			monitored = ov
		}

		// Released: dated and in the past; undated works are considered
		// released iff they sit at/below the highest dated sequence, or the
		// collection has no dates at all (spec.md §4.2 [default]).
		released := false
		switch {
		case w.ReleaseDate != "":
			released = w.ReleaseDate <= today
		case !anyDate:
			released = true
		default:
			released = w.SequenceNum <= maxDated
		}

		files := links[store.SlotKey("volume", col.ID, w.SequenceNum)]
		slots = append(slots, VolumeSlot{
			Number:      w.SequenceNum,
			SequenceStr: w.SequenceStr,
			CountType:   w.CountType,
			ReleaseDate: w.ReleaseDate,
			Monitored:   monitored,
			Released:    released,
			FileIDs:     files,
			Wanted:      monitored && released && len(files) == 0,
		})
	}
	return slots
}

// BuildChapterSlots derives integer chapter slots 1..totalChapters for one
// chapter-type collection, from MangaBaka metadata only (D7-rev/D15:
// sources never feed the ledger). A slot is monitored iff the collection
// is monitored and its number is above the cutoff (D6: "monitor from N
// onward", cutoff itself excluded).
func BuildChapterSlots(col store.Collection, totalChapters int, links map[string][]int64, overrides map[int64]bool) []ChapterSlot {
	upper := int64(totalChapters) * 100
	if upper <= 0 {
		return nil
	}

	count := upper / 100
	slots := make([]ChapterSlot, 0, count)
	for n := int64(1); n <= count; n++ {
		num := n * 100
		monitored := col.Monitored && num > col.CutoffNum
		if ov, ok := overrides[num]; ok {
			monitored = ov
		}
		files := links[store.SlotKey("chapter", col.ID, num)]
		slots = append(slots, ChapterSlot{
			Number:    num,
			Monitored: monitored,
			FileIDs:   files,
			Wanted:    monitored && len(files) == 0,
		})
	}
	return slots
}
