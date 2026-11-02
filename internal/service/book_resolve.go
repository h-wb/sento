package service

import (
	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/store"
)

// Resolving a release or file name to books is one rule shared by search
// and scan: the number picks candidates, the kind narrows them, and two
// books left sharing a number are never guessed between.

// bookKindFits reports whether a name can stand for book b on its number
// alone: a plain "v01" is the main book, and only a name marked as an
// extra reaches extras and other non-main books.
func bookKindFits(b store.Book, extra bool) bool {
	return (b.CountType == "main") != extra
}

// inRanges reports whether b's number falls in one of the parsed ranges.
func inRanges(b store.Book, ranges []parse.Range) bool {
	if !b.NumberKnown {
		return false
	}
	for _, r := range ranges {
		if b.SequenceNum >= int64(r.From) && b.SequenceNum <= int64(r.To) {
			return true
		}
	}
	return false
}

// sharesNumber reports whether another book of the same kind in books has
// b's number, e.g. two revisions of volume 1. A number cannot tell those
// apart.
func sharesNumber(books []store.Book, b store.Book) bool {
	if !b.NumberKnown {
		return false
	}
	for _, other := range books {
		if other.ID != b.ID && other.NumberKnown && other.SequenceNum == b.SequenceNum && other.CountType == b.CountType {
			return true
		}
	}
	return false
}

// resolveBooks returns the books of one edition that the parsed ranges
// identify. ambiguous is set when any of them shares its number with
// another book of the same kind; the caller must then ask rather than
// link.
func resolveBooks(books []store.Book, ranges []parse.Range, extra bool) (matched []store.Book, ambiguous bool) {
	for _, b := range books {
		if !bookKindFits(b, extra) || !inRanges(b, ranges) {
			continue
		}
		matched = append(matched, b)
		ambiguous = ambiguous || sharesNumber(books, b)
	}
	return matched, ambiguous
}

// numberHolds says whether one bare number stands for a book: a single
// number is a book of the edition, and a range lies within the numbering
// its books run to.
func numberHolds(books []store.Book, r parse.Range) bool {
	if r.From == r.To {
		for _, b := range books {
			if b.NumberKnown && b.SequenceNum == int64(r.From) {
				return true
			}
		}
		return false
	}
	var low, high int64
	first := true
	for _, b := range books {
		if !b.NumberKnown {
			continue
		}
		if first || b.SequenceNum < low {
			low = b.SequenceNum
		}
		if first || b.SequenceNum > high {
			high = b.SequenceNum
		}
		first = false
	}
	return !first && int64(r.From) >= low && int64(r.To) <= high
}

// everyNumberHeld says the books resolve the whole name: each number it
// states identifies at least one of them, never a subset ("187 - Side
// Story 08" is not volume 8 alone).
func everyNumberHeld(books []store.Book, ranges []parse.Range) bool {
	for _, r := range ranges {
		held := false
		for _, b := range books {
			if b.SequenceNum >= int64(r.From) && b.SequenceNum <= int64(r.To) {
				held = true
				break
			}
		}
		if !held {
			return false
		}
	}
	return true
}
