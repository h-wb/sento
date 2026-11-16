package store

import (
	"cmp"
	"slices"
	"strconv"

	"github.com/h-wb/sento/internal/metadata"
)

// mediumOrder is which copy of a book speaks for it when several exist:
// its ISBN, page count and the edition's publisher. Release quality is the
// profile's business, not this order's.
var mediumOrder = []string{"digital", "paperback", "hardcover"}

func mediumRank(medium string) int {
	if i := slices.Index(mediumOrder, medium); i >= 0 {
		return i
	}
	return len(mediumOrder)
}

// preferred orders catalog rows: present in the catalog first, then by medium.
func preferred(aState, aMedium string, aID int64, bState, bMedium string, bID int64) int {
	return cmp.Or(
		cmp.Compare(b2i(aState != "linked"), b2i(bState != "linked")),
		cmp.Compare(mediumRank(aMedium), mediumRank(bMedium)),
		cmp.Compare(aID, bID),
	)
}

// numberKnown says whether a catalog work has a number at all: a numeric
// sequence, or a label that reads as one (volume 0 is a number).
func numberKnown(w metadata.Work) bool {
	if w.NumberKnown || w.SequenceNum != 0 {
		return true
	}
	_, err := strconv.ParseFloat(w.SequenceStr, 64)
	return err == nil
}

// pairBooks matches each book of one edition with the first unclaimed
// book of another that carries the same number.
func pairBooks(from, into []Book) map[int64]int64 {
	out := map[int64]int64{}
	used := map[int64]bool{}
	for _, a := range from {
		for _, b := range into {
			if !used[b.ID] && sameNumber(a, b) {
				out[a.ID], used[b.ID] = b.ID, true
				break
			}
		}
	}
	return out
}

// sameLabel says whether two editions of a series can be the same
// publication: one type, one language, and one MangaBaka edition when
// both name one.
func sameLabel(a, b *Edition) bool {
	if a.Type != b.Type || a.Language != b.Language {
		return false
	}
	return a.ProviderEditionID == "" || b.ProviderEditionID == "" || a.ProviderEditionID == b.ProviderEditionID
}

// sameBook says whether two books of different editions are the same book:
// the same number, and no ISBN that says otherwise.
func sameBook(a, b Book) bool {
	return sameNumber(a, b) && (a.ISBN == "" || b.ISBN == "" || a.ISBN == b.ISBN)
}

// sameNumber compares what a book is called: the same number when both
// have one (labels vary: "1", "01"), otherwise the same label; and never
// across kinds or revisions.
func sameNumber(a, b Book) bool {
	if a.CountType != b.CountType || a.Revision != b.Revision || a.NumberKnown != b.NumberKnown {
		return false
	}
	if a.NumberKnown {
		return a.SequenceNum == b.SequenceNum
	}
	return a.SequenceStr == b.SequenceStr
}
