package service

import "github.com/h-wb/sento/internal/store"

// BookState is where one book stands for one card.
type BookState string

// bookState derives these states from ownership, reservation phase and policy.
const (
	// StatePending: a download or import already holds the book.
	StatePending BookState = "pending"
	// StateNeedsReview: a file holds the book but its mapping is not
	// confirmed, and no confirmed file does.
	StateNeedsReview BookState = "needs_review"
	// StateOwned: a confirmed file the card's profile accepts holds it.
	StateOwned BookState = "owned"
	// StateUpgradeEligible: owned, and the profile wants a better file.
	StateUpgradeEligible BookState = "upgrade_eligible"
	// StateStorageUnavailable: the library folder cannot be checked.
	StateStorageUnavailable BookState = "storage_unavailable"
	// StateUnmonitored: the card does not collect this book.
	StateUnmonitored BookState = "unmonitored"
	// StateNotReleased: published in the future.
	StateNotReleased BookState = "not_released"
	// StateUnknown: no publication date, and undated books are not searched.
	StateUnknown BookState = "unknown"
	// StateUnsatisfactory: a file on disk holds it, but not one this card
	// accepts; wanted.
	StateUnsatisfactory BookState = "unsatisfactory"
	// StateMissing: wanted, nothing holds it.
	StateMissing BookState = "missing"
)

type BookStatus struct {
	OnDiskFileIDs []int64 `json:"onDiskFileIds"`
	Reason        string  `json:"reason"`
	CurrentScore  int     `json:"currentScore"`

	Book         store.Book `json:"book"`
	VersionID    int64      `json:"versionId"`
	Monitored    bool       `json:"monitored"`
	Availability string     `json:"availability"`
	State        BookState  `json:"state"`
	FileIDs      []int64    `json:"fileIds"`
	Wanted       bool       `json:"wanted"`
}

// bookFacts is everything bookState weighs for one book of one card.
type bookFacts struct {
	// Collected: the card collects the book. Why says why not.
	Collected bool
	Why       string
	Owned     bool // a confirmed, accepted file holds it
	OnDisk    bool // some present file holds it
	Review    bool // a file's mapping to it awaits confirmation
	Reserved  bool // a download or import holds it
	Reviewing bool // its reservation is held by a download awaiting review
	// Availability is released, upcoming or unknown.
	Availability   string
	AcquireUnknown bool
	StorageDown    bool
	// Upgrade: the profile wants a better file than the owned one.
	Upgrade bool
	// Refused says why the file on disk does not count, when it does not.
	Refused string
}

// bookState is where a book stands, why, and whether it is wanted. The
// cases are checked in order; the first that holds decides.
func bookState(f bookFacts) (state BookState, reason string, wanted bool) {
	switch {
	// A download in flight says what is happening to the book; the file
	// awaiting review still shows in Review, and a confirmed download
	// makes the book owned either way.
	case f.Reserved && f.Reviewing && f.Owned:
		return StateOwned, "A verified file is available; the download is awaiting review", false
	case f.Reserved:
		return StatePending, "A download or import already holds this book", false
	case f.Review && !f.Owned:
		return StateNeedsReview, "Existing file mappings need review", false
	case f.Owned && f.StorageDown:
		return StateOwned, "Library storage is unavailable or has not been checked", false
	case f.Owned && !f.Collected:
		return StateOwned, f.Why, false
	case f.Owned && f.Upgrade:
		return StateUpgradeEligible, "The file on disk is below the upgrade cutoff", true
	case f.Owned:
		return StateOwned, "", false
	case f.StorageDown:
		return StateStorageUnavailable, "Library storage is unavailable or has not been checked", false
	case !f.Collected:
		return StateUnmonitored, f.Why, false
	case f.Availability == "upcoming":
		return StateNotReleased, "Publication date is in the future", false
	case f.Availability == "unknown" && !f.AcquireUnknown:
		return StateUnknown, "Publication date is unknown and searching undated books is disabled", false
	case f.OnDisk:
		reason := f.Refused
		if reason == "" {
			reason = "A file of this book is on disk, but this edition does not use it"
		}
		return StateUnsatisfactory, reason, true
	default:
		return StateMissing, "", true
	}
}

// availability is released, upcoming or unknown, as of today.
func availability(releaseDate, today string) string {
	switch {
	case releaseDate == "":
		return "unknown"
	case releaseDate > today:
		return "upcoming"
	default:
		return "released"
	}
}
