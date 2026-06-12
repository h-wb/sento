package service

// The decision engine: raw provider candidates are parsed, gated by the
// hard/soft rejection rules and scored (decision-engine.md). Pure given
// the searchRun inputs — no I/O happens here.

import (
	"sort"
	"strings"

	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/profile"
	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

// process parses, funnels and scores one provider's raw candidates.
func (s *SearchService) process(run *searchRun, cands []provider.Candidate) (out []ReleaseCandidate, dropped int) {
	for _, cand := range cands {
		rc := ReleaseCandidate{
			GUID: cand.GUID(), Provider: cand.Provider, Protocol: cand.Protocol,
			Site: cand.Site, SiteID: cand.SiteID, Title: cand.Title, cand: cand,
		}
		channel := parse.ChannelTorrent
		if cand.Protocol == provider.ProtocolSource {
			channel = parse.ChannelSource
		}
		if cand.Torrent != nil {
			rc.Size = cand.Torrent.Size
			rc.Seeders = cand.Torrent.Seeders
			rc.URL = cand.Torrent.InfoURL
		}
		if cand.Source != nil {
			rc.Scanlator = cand.Source.Scanlator
			rc.URL = cand.Source.WebURL
		}
		rc.Parse = parse.ParseAnchored(cand.Title, channel, run.titles)

		ranges := s.coverageRanges(run.ledger, &rc)
		// Keep number-less candidates only as potential whole-series packs
		// (volume ledger).
		isPack := run.ledger == LedgerVolume && rc.Parse.Complete
		if len(ranges) == 0 && !isPack {
			dropped++
			continue
		}
		if run.blocked[cand.Protocol+"/"+cand.ExternalKey()] {
			rc.Rejections = append(rc.Rejections, "rejected.blocklisted") // hard
		} else {
			s.funnel(run, &rc, ranges)
		}
		rc.Score -= run.priorities[[2]string{rc.Provider, rc.SiteID}]
		out = append(out, rc)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out, dropped
}

// coverageRanges extracts the ledger-relevant number ranges of a candidate.
func (s *SearchService) coverageRanges(searchLedger string, rc *ReleaseCandidate) []parse.Range {
	if searchLedger == LedgerVolume {
		return rc.Parse.Volumes
	}
	ranges := rc.Parse.Chapters
	// Source corroboration for terse names ("1184"): the extension's own
	// recognition.
	if len(ranges) == 0 && rc.cand.Source != nil && rc.cand.Source.ChapterNumber > 0 {
		n := parse.Num(rc.cand.Source.ChapterNumber*100 + 0.5)
		ranges = []parse.Range{{From: n, To: n}}
	}
	return ranges
}

// funnel applies hard rejections then scoring (decision-engine.md §3–4),
// parameterized by ledger and the series' release profile.
func (s *SearchService) funnel(run *searchRun, c *ReleaseCandidate, ranges []parse.Range) {
	searchLedger, target := run.ledger, run.target
	collections, wanted, chapterLang := run.collections, run.wanted, run.chapterLang
	rules := run.ledgerRules()
	reject := func(code string) { c.Rejections = append(c.Rejections, code) }

	// R1 content-type gate.
	if c.Parse.ContentType != parse.ContentManga {
		reject("rejected.content_type")
		return
	}
	// Series match (source candidates are binding-scoped; anchor waived).
	if c.Protocol != provider.ProtocolSource {
		if !c.Parse.Anchored || c.Parse.TitleLeftover != "" {
			reject("rejected.series_match")
			return
		}
	}

	// Routing: volumes route to a collection (R6); chapters use ledger 0.
	colID := int64(0)
	wantLang := chapterLang
	if searchLedger == LedgerVolume {
		if target != nil {
			colID = target.CollectionID
		} else {
			var ok bool
			colID, ok = routeEdition(c.Parse, collections, wanted)
			if !ok {
				reject("rejected.edition_routing")
				return
			}
		}
		for _, col := range collections {
			if col.ID == colID {
				wantLang = col.Language
			}
		}
	}
	c.CollectionID = colID

	// R2 language gate (untagged passes). Soft: record and keep going so
	// Covered is still computed — a force-grab needs the slot list.
	if c.Parse.Language != "" && c.Parse.Language != "multi" && wantLang != "" &&
		baseLang(c.Parse.Language) != baseLang(wantLang) {
		reject("rejected.language")
	}

	// R5 slot coverage. A whole-series pack covers everything wanted
	// (volume ledger only).
	if len(ranges) == 0 && searchLedger == LedgerVolume && c.Parse.Complete {
		for n := range wanted[colID] {
			c.Covered = append(c.Covered, n)
		}
	} else {
		totalCovered := 0
		for _, r := range ranges {
			for n := (int64(r.From) + 99) / 100 * 100; n <= int64(r.To); n += 100 {
				totalCovered++
				if wanted[colID][n] {
					c.Covered = append(c.Covered, n)
				}
			}
		}
		// D12 pack rule: ≥50% of covered slots must be wanted to auto-grab.
		if totalCovered > 0 && len(c.Covered)*2 < totalCovered && len(c.Covered) > 0 {
			reject("rejected.pack_threshold")
		}
	}
	if len(c.Covered) == 0 {
		reject("rejected.no_coverage")
		return
	}
	sort.Slice(c.Covered, func(i, j int) bool { return c.Covered[i] < c.Covered[j] })

	// Custom formats: matched on every routable candidate (rejected ones
	// keep their chips), floor-gated softly so force-grab can override.
	c.evalFormats(run)
	if run.profile != nil && run.profile.MinFormatScore > profile.MinScoreOff &&
		c.FormatScore < run.profile.MinFormatScore {
		reject("rejected.format_score")
	}

	// Torrent-only health checks (profile rules).
	if c.Protocol == provider.ProtocolTorrent {
		if c.Size > 0 && rules.SizeMaxMB > 0 {
			perUnit := c.Size / int64(len(c.Covered))
			if perUnit < int64(rules.SizeMinMB)<<20 || perUnit > int64(rules.SizeMaxMB)<<20 {
				reject("rejected.size")
			}
		}
		if c.Seeders < rules.MinSeeders {
			reject("rejected.seeders")
		}
	}
	if len(c.Rejections) > 0 {
		return
	}

	// Scoring (decision-engine.md §4): the protocol preference comes from
	// the profile's ledger rules; term scores from the custom formats.
	score := len(c.Covered) * 1000
	if c.Protocol == rules.PreferredProtocol {
		score += 200
	}
	score += c.FormatScore
	switch {
	case c.Seeders > 20:
		score += 50
	case c.Seeders > 5:
		score += 25
	}
	if c.Parse.Confidence < 0.8 {
		score -= 100
	}
	c.Score = score
}

// ledgerRules resolves the funnel knobs for the run's ledger, falling
// back to the built-in defaults when no profile is configured.
func (run *searchRun) ledgerRules() profile.LedgerRules {
	vol, ch := profile.DefaultRules()
	if run.profile != nil {
		vol, ch = run.profile.Volume, run.profile.Chapter
	}
	if run.ledger == LedgerChapter {
		return ch
	}
	return vol
}

// evalFormats matches the active profile's custom formats against the
// candidate and sums their scores.
func (c *ReleaseCandidate) evalFormats(run *searchRun) {
	if run.profile == nil {
		return
	}
	group := c.Parse.Group
	if group == "" {
		group = c.Scanlator
	}
	in := profile.Input{
		Title: c.Title, Group: group, Language: c.Parse.Language,
		Protocol: c.Protocol, Site: c.Site, Resolution: c.Parse.Quality.Resolution,
		Flags: map[string]bool{
			"digital": c.Parse.Quality.Digital, "scan": c.Parse.Quality.Scan,
			"web": c.Parse.Quality.Web, "premium": c.Parse.Quality.Premium,
			"colored": c.Parse.Edition.Colored, "omnibus": c.Parse.Edition.Omnibus,
			"deluxe": c.Parse.Edition.Deluxe, "boxset": c.Parse.Edition.BoxSet,
			"official": c.Parse.Edition.Official, "raw": c.Parse.Edition.Raw,
			"complete": c.Parse.Complete,
		},
	}
	for _, f := range run.formats {
		if !f.Matches(in) {
			continue
		}
		score := run.profile.FormatScores[f.ID]
		c.Formats = append(c.Formats, FormatHit{Name: f.Name, Score: score})
		c.FormatScore += score
	}
}

func baseLang(lang string) string {
	base, _, _ := strings.Cut(lang, "-")
	return strings.ToLower(base)
}

// routeEdition maps edition hints to a monitored collection (R6). A
// language-tagged release prefers collections of that language (custom
// fr variants receive FR releases; en/untagged keep the original) —
// falling back to all monitored when no language matches.
func routeEdition(p parse.Result, collections []store.Collection,
	wanted map[int64]map[int64]bool,
) (int64, bool) {
	monitored := make([]store.Collection, 0, len(collections))
	for _, col := range collections {
		if wanted[col.ID] != nil {
			monitored = append(monitored, col)
		}
	}
	if len(monitored) == 0 {
		return 0, false
	}
	if p.Language != "" && p.Language != "multi" {
		var sameLang []store.Collection
		for _, col := range monitored {
			if baseLang(col.Language) == baseLang(p.Language) {
				sameLang = append(sameLang, col)
			}
		}
		if len(sameLang) > 0 {
			monitored = sameLang
		}
	}
	matchName := func(substr string) (int64, bool) {
		for _, col := range monitored {
			if strings.Contains(strings.ToLower(col.EditionName), substr) {
				return col.ID, true
			}
		}
		return 0, false
	}
	switch {
	case p.Edition.Omnibus:
		return matchName("omnibus")
	case p.Edition.BoxSet:
		return matchName("box")
	case p.Edition.Colored:
		if id, ok := matchName("color"); ok {
			return id, true
		}
	case p.Edition.Deluxe:
		if id, ok := matchName("deluxe"); ok {
			return id, true
		}
	}
	if id, ok := matchName("standard"); ok {
		return id, true
	}
	return monitored[0].ID, true
}

// Rejections that survive a force-grab (decision-engine.md §7).
var hardRejections = map[string]bool{
	"rejected.content_type":    true,
	"rejected.series_match":    true,
	"rejected.edition_routing": true,
	"rejected.no_coverage":     true,
	"rejected.blocklisted":     true,
}
