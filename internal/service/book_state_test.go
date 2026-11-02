package service

import "testing"

// Each row is a case where two facts compete; the first state in
// bookState's order wins.
func TestBookStatePriority(t *testing.T) {
	t.Parallel()
	collected := bookFacts{Collected: true, Availability: "released"}
	with := func(edit func(*bookFacts)) bookFacts {
		f := collected
		edit(&f)
		return f
	}
	for _, tc := range []struct {
		name   string
		facts  bookFacts
		state  BookState
		wanted bool
	}{
		{"nothing holds it", collected, StateMissing, true},
		{"a download in flight outranks an unconfirmed file", with(func(f *bookFacts) { f.Review, f.Reserved = true, true }), StatePending, false},
		{"an unconfirmed file outranks wanting the book", with(func(f *bookFacts) { f.Review = true }), StateNeedsReview, false},
		{"a confirmed file settles review", with(func(f *bookFacts) { f.Review, f.Owned = true, true }), StateOwned, false},
		{"verified files survive another file’s review", with(func(f *bookFacts) { f.Reserved, f.Reviewing, f.Owned = true, true, true }), StateOwned, false},
		{"review still reserves an unfulfilled book", with(func(f *bookFacts) { f.Reserved, f.Reviewing = true, true }), StatePending, false},
		{"a download in flight outranks ownership", with(func(f *bookFacts) { f.Reserved, f.Owned = true, true }), StatePending, false},
		{"a download in flight outranks storage", with(func(f *bookFacts) { f.Reserved, f.StorageDown = true, true }), StatePending, false},
		{"owned stays owned when not collected", with(func(f *bookFacts) { f.Owned, f.Collected, f.Why = true, false, "paused" }), StateOwned, false},
		{"no upgrade while storage is down", with(func(f *bookFacts) { f.Owned, f.Upgrade, f.StorageDown = true, true, true }), StateOwned, false},
		{"no upgrade when not collected", with(func(f *bookFacts) { f.Owned, f.Upgrade, f.Collected = true, true, false }), StateOwned, false},
		{"an upgrade is wanted", with(func(f *bookFacts) { f.Owned, f.Upgrade = true, true }), StateUpgradeEligible, true},
		{"storage outranks collecting", with(func(f *bookFacts) { f.StorageDown, f.Collected = true, false }), StateStorageUnavailable, false},
		{"not collected outranks a future date", with(func(f *bookFacts) { f.Collected, f.Availability = false, "upcoming" }), StateUnmonitored, false},
		{"a future date outranks a file on disk", with(func(f *bookFacts) { f.Availability, f.OnDisk = "upcoming", true }), StateNotReleased, false},
		{"undated, not searched", with(func(f *bookFacts) { f.Availability = "unknown" }), StateUnknown, false},
		{"undated, searched", with(func(f *bookFacts) { f.Availability, f.AcquireUnknown = "unknown", true }), StateMissing, true},
		{"a file this card refuses leaves the book wanted", with(func(f *bookFacts) { f.OnDisk, f.Refused = true, "wrong group" }), StateUnsatisfactory, true},
	} {
		state, _, wanted := bookState(tc.facts)
		if state != tc.state || wanted != tc.wanted {
			t.Errorf("%s: %s wanted=%v, want %s wanted=%v", tc.name, state, wanted, tc.state, tc.wanted)
		}
	}
}
