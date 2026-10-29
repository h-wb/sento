package service

import (
	"fmt"
	"github.com/h-wb/sento/internal/parse"
	"strings"
)

type ReleaseReason struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Override bool   `json:"override"`
}
type ScorePart struct {
	Label string `json:"label"`
	Value int    `json:"value"`
}
type ReleaseDecision struct {
	Status         string          `json:"status" tstype:"'ready' | 'review' | 'blocked'"`
	CanDownload    bool            `json:"canDownload"`
	CanForce       bool            `json:"canForce"`
	Kind           string          `json:"kind"`
	LanguageSource string          `json:"languageSource"`
	NumberSource   string          `json:"numberSource"`
	Reasons        []ReleaseReason `json:"reasons"`
	Warnings       []string        `json:"warnings"`
}

// One policy authority supplies both automatic acquisition and the interactive UI.
func (c *ReleaseCandidate) explain() {
	d := ReleaseDecision{Status: "ready", CanDownload: len(c.Rejections) == 0, Reasons: []ReleaseReason{}, Warnings: []string{}, Kind: "Unclassified", LanguageSource: "unknown", NumberSource: "unknown"}
	if c.Parse.Language != "" {
		d.LanguageSource = "release name"
		if c.cand.Source != nil && c.cand.Source.Lang == c.Parse.Language && !hasLanguageTrace(c.Parse) {
			d.LanguageSource = "source listing"
		}
		if c.languageSource != "" {
			d.LanguageSource = c.languageSource
		}
	}
	switch {
	case ledgerOf(c.Parse) == LedgerVolume:
		d.Kind = "Volume"
		d.NumberSource = "release name"
	case ledgerOf(c.Parse) == LedgerChapter:
		d.Kind = "Chapter"
		d.NumberSource = "release name"
	case c.cand.Source != nil && c.cand.Source.ChapterNumber > 0:
		d.Kind = "Chapter"
		d.NumberSource = "source listing"
	case c.Parse.Complete:
		d.Kind = "Complete pack"
	}
	if c.Parse.ContentType != parse.ContentManga {
		d.Kind = string(c.Parse.ContentType)
	}
	if c.Parse.Language == "" {
		d.Warnings = append(d.Warnings, "Language is not stated; this version allows unknown language only if configured.")
	}
	if len(c.Parse.Bare) > 0 {
		d.Warnings = append(d.Warnings, "Some numbers have no confirmed volume or chapter marker.")
	}
	if c.Parse.CompleteInferred {
		d.Warnings = append(d.Warnings, "A year range suggests a pack, but does not establish which books it contains.")
	}
	if !c.Parse.Quality.Digital && !c.Parse.Quality.Scan && !c.Parse.Quality.Web {
		d.Warnings = append(d.Warnings, "Release quality is unspecified.")
	}
	labels := map[string]string{
		"rejected.previous_download": "This is the download you are looking to replace. It is excluded from this search only.",
		"rejected.content_type":      "Classified as " + string(c.Parse.ContentType) + ", not manga.",
		"rejected.series_match":      "The release name does not identify this series unambiguously.",
		"rejected.edition_routing":   "The release states a different edition than this one.",
		"rejected.edition_ambiguous": "This release also fits another edition you collect. Confirm it belongs to this one.",
		"rejected.no_coverage":       "No books in this release need a download in the current selection.",
		"rejected.numbers_unknown":   "No confirmed volume or chapter numbers were found.",
		"rejected.pack_unverified":   "Nothing in the name says which books this pack holds. Confirm it is the whole edition; the import checks what is inside.",
		"rejected.blocklisted":       "This release is on your blocklist.",
		"rejected.language":          "The stated language does not match this edition.",
		"rejected.language_unknown":  "This version requires a confirmed language.",
		"rejected.book_ambiguous":    "Several books share this number. Search from the specific book to disambiguate.",
		"rejected.version_group":     "The release group does not match this version’s required group.",
		"rejected.no_upgrade":        "This release does not improve the existing copy enough.",
		"rejected.format_score":      "The release does not meet the profile’s minimum format score.",
		"rejected.pack_threshold":    fmt.Sprintf("Only %d of %d represented books are wanted; automatic packs require at least half.", len(c.Coverage), c.RepresentedBooks),
		"rejected.size":              "The estimated size per book is outside the profile’s limits.",
		"rejected.seeders":           "The torrent has fewer seeders than the profile requires.",
	}
	hard := false
	seen := map[string]bool{}
	for _, code := range c.Rejections {
		if seen[code] {
			continue
		}
		seen[code] = true
		message := labels[code]
		if code == "rejected.edition_routing" && c.EditionSnapshot != nil {
			if why, statesOwn := editionConflict(c.Parse, *c.EditionSnapshot); why != "" {
				message = why
				if statesOwn {
					message += " If MangaBaka does not list that edition, add it to the series with Add edition."
				}
			}
		}
		if message == "" {
			message = "Does not meet rule: " + strings.TrimPrefix(code, "rejected.")
		}
		override := !hardRejections[code]
		hard = hard || !override
		d.Reasons = append(d.Reasons, ReleaseReason{code, message, override})
	}
	if len(d.Reasons) > 0 {
		d.Status = "review"
		d.CanForce = !hard && len(c.Coverage) > 0
		if hard || !d.CanForce {
			d.Status = "blocked"
		}
	}
	c.Decision = d
	c.ParseTrace = c.Parse.Trace
}
func hasLanguageTrace(p parse.Result) bool {
	// Explicit language evidence is captured before source-language fallback.
	for _, s := range p.Trace {
		if s.Rule == "language.explicit" {
			return true
		}
	}
	return false
}
