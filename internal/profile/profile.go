// Package profile is the pure domain of release profiles and custom
// formats (decision-engine.md §5, Sonarr's model): a custom format is a
// named list of conditions matched against a release candidate; a profile
// assigns scores to formats and owns the tunable funnel knobs. No I/O —
// the store persists, the service validates, the funnel applies.
//
// Two entities, two files: format.go is the custom-format matcher
// (Condition/Format/Matches); profile.go (here) is the profile that scores
// formats and parameterizes the funnel.
package profile

// MinScoreOff disables the profile's format-score floor.
const MinScoreOff = -10000

// LedgerRules are the per-ledger funnel knobs a profile owns
// (decision-engine.md §3.5–6, §4 protocol preference).
type LedgerRules struct {
	SizeMinMB         int    `json:"sizeMinMB"`
	SizeMaxMB         int    `json:"sizeMaxMB"`
	MinSeeders        int    `json:"minSeeders"`
	PreferredProtocol string `json:"preferredProtocol"` // +200 in scoring
}

// Profile scores formats and parameterizes the funnel. FormatScores maps
// format id → score; unlisted formats score 0. Candidates whose summed
// format score is below MinFormatScore are soft-rejected (force-grab
// still works); MinScoreOff disables the floor.
type Profile struct {
	ID             int64         `json:"id"`
	Name           string        `json:"name"`
	MinFormatScore int           `json:"minFormatScore"`
	FormatScores   map[int64]int `json:"formatScores"`
	Volume         LedgerRules   `json:"volume"`
	Chapter        LedgerRules   `json:"chapter"`
}

// DefaultRules returns the funnel constants that applied before profiles
// existed — the rules of a fresh profile.
func DefaultRules() (volume, chapter LedgerRules) {
	return LedgerRules{SizeMinMB: 20, SizeMaxMB: 1500, MinSeeders: 1, PreferredProtocol: "torrent"},
		LedgerRules{SizeMinMB: 2, SizeMaxMB: 200, MinSeeders: 1, PreferredProtocol: "source"}
}
