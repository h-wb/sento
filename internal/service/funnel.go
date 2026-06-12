package service

// The decision engine: raw provider candidates are parsed, gated by the
// hard/soft rejection rules and scored (decision-engine.md). Pure given
// the searchRun inputs — no I/O happens here.

import (
	"sort"
	"strings"

	"github.com/h-wb/sento/internal/parse"
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
			s.funnel(run.ledger, &rc, ranges, run.collections, run.wanted, run.chapterLang, run.target)
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
// parameterized by ledger.
func (s *SearchService) funnel(searchLedger string, c *ReleaseCandidate, ranges []parse.Range,
	collections []store.Collection, wanted map[int64]map[int64]bool, chapterLang string, target *TargetSlot,
) {
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

	// Torrent-only health checks.
	if c.Protocol == provider.ProtocolTorrent {
		if c.Size > 0 {
			perUnit := c.Size / int64(len(c.Covered))
			minSize, maxSize := int64(20<<20), int64(1500<<20)
			if searchLedger == LedgerChapter {
				minSize, maxSize = 2<<20, 200<<20
			}
			if perUnit < minSize || perUnit > maxSize {
				reject("rejected.size")
			}
		}
		if c.Seeders < 1 {
			reject("rejected.seeders")
		}
	}
	if len(c.Rejections) > 0 {
		return
	}

	// Scoring (decision-engine.md §4): protocol preference flips per ledger.
	score := len(c.Covered) * 1000
	switch {
	case searchLedger == LedgerVolume && c.Protocol == provider.ProtocolTorrent:
		score += 200
	case searchLedger == LedgerChapter && c.Protocol == provider.ProtocolSource:
		score += 200
	}
	if c.Parse.Quality.Digital {
		score += 100
	}
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
