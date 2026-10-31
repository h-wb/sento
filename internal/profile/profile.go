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

import (
	"fmt"
	"slices"
)

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
// format id → score; unlisted formats score 0, so one profile can rank a
// group high and another low. Candidates whose summed format score is
// below MinFormatScore are soft-rejected (force-grab still works);
// MinScoreOff disables the floor.
//
// Required lists formats a release must match, every one (Sonarr's "must
// contain"); Refused, formats it must match none of ("must not contain").
// Both are hard: a release that breaks one is not downloaded for this
// profile, and a file on disk that breaks one does not count toward it.
type Profile struct {
	Required          []int64       `json:"required"`
	Refused           []int64       `json:"refused"`
	UpgradesEnabled   bool          `json:"upgradesEnabled"`
	UpgradeUntilScore int           `json:"upgradeUntilScore"`
	MinUpgradeScore   int           `json:"minUpgradeScore"`
	ID                int64         `json:"id"`
	Name              string        `json:"name"`
	MinFormatScore    int           `json:"minFormatScore"`
	FormatScores      map[int64]int `json:"formatScores"`
	Volume            LedgerRules   `json:"volume"`
	Chapter           LedgerRules   `json:"chapter"`
	// ApartFrom lists the profiles no release can satisfy together with
	// this one (Apart), filled when profiles are listed; never stored.
	ApartFrom []int64 `json:"apartFrom,omitempty"`
}

// Apart reports whether no release can satisfy both profiles: one refuses
// a format the other requires. Otherwise a release both accept fills an
// edition's card for each, and a file both accept counts for both.
func Apart(a, b Profile) bool {
	refuses := func(p, q Profile) bool {
		for _, f := range q.Required {
			if slices.Contains(p.Refused, f) {
				return true
			}
		}
		return false
	}
	return refuses(a, b) || refuses(b, a)
}

// DefaultRules returns the funnel constants that applied before profiles
// existed — the rules of a fresh profile.
func DefaultRules() (volume, chapter LedgerRules) {
	return LedgerRules{SizeMinMB: 20, SizeMaxMB: 1500, MinSeeders: 1, PreferredProtocol: "torrent"},
		LedgerRules{SizeMinMB: 2, SizeMaxMB: 200, MinSeeders: 1, PreferredProtocol: "source"}
}

// Judgement is what a profile says about one release or file: its summed
// format score, the formats it matched (in list order), and the first
// hard rule it breaks, in words ("" when it breaks none).
type Judgement struct {
	Score   int
	Matched []Format
	Broken  string
}

// Judge applies the profile to a release or file. The one place a
// profile's scores and rules meet a candidate: search, the ledger and the
// importer all ask here.
func (p *Profile) Judge(in Input, formats []Format) Judgement {
	var j Judgement
	if p == nil {
		return j
	}
	matched := map[int64]bool{}
	for _, f := range formats {
		if f.Matches(in) {
			matched[f.ID] = true
			j.Matched = append(j.Matched, f)
			j.Score += p.FormatScores[f.ID]
		}
	}
	name := func(id int64) string {
		for _, f := range formats {
			if f.ID == id {
				return f.Name
			}
		}
		return fmt.Sprintf("format %d", id)
	}
	for _, id := range p.Required {
		if !matched[id] {
			j.Broken = fmt.Sprintf("does not match “%s”, which %s requires", name(id), p.Name)
			return j
		}
	}
	for _, id := range p.Refused {
		if matched[id] {
			j.Broken = fmt.Sprintf("matches “%s”, which %s refuses", name(id), p.Name)
			return j
		}
	}
	return j
}
