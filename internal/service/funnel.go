package service

// The decision engine: raw provider candidates are parsed, gated by the
// hard/soft rejection rules and scored (decision-engine.md). Pure given
// the searchRun inputs — no I/O happens here.

import (
	"regexp"
	"slices"
	"sort"
	"strconv"
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
		// Chapter names rarely tag a language; the source's own language is
		// the release language (drives the R2 gate and edition routing).
		if cand.Source != nil && rc.Parse.Language == "" {
			rc.Parse.Language = cand.Source.Lang
		}
		// A torrent name that states no language still has evidence around
		// it, strongest first: the indexer section it is filed under, an
		// English-language publisher's tag, the indexer's own language.
		if cand.Torrent != nil && rc.Parse.Language == "" {
			switch {
			case cand.Torrent.CategoryLanguage != "":
				rc.Parse.Language, rc.languageSource = cand.Torrent.CategoryLanguage, "indexer category"
			case englishPublisher(rc.Parse.Edition.Names):
				rc.Parse.Language, rc.languageSource = "en", "publisher"
			case cand.Torrent.DefaultLanguage != "":
				rc.Parse.Language, rc.languageSource = cand.Torrent.DefaultLanguage, "indexer default"
			}
		}

		ranges := s.coverageRanges(run.ledger, &rc)
		// Keep number-less candidates only as potential whole-series packs
		// (volume ledger).
		isPack := run.ledger == LedgerVolume && rc.Parse.Complete
		// A torrent named for the series alone, with no number anywhere,
		// may well be the whole edition ("Title - Deluxe Edition (2012)").
		// The name cannot confirm it, so it is treated as an inferred pack:
		// offered for review, never grabbed automatically, and the import
		// maps what is actually inside.
		if run.ledger == LedgerVolume && !isPack && cand.Protocol == provider.ProtocolTorrent &&
			len(rc.Parse.Volumes)+len(rc.Parse.Chapters)+len(rc.Parse.Bare) == 0 &&
			rc.Parse.Anchored && rc.Parse.TitleLeftover == "" && rc.Parse.ContentType == parse.ContentManga {
			rc.Parse.Complete, rc.Parse.CompleteInferred = true, true
			isPack = true
		}

		if len(ranges) == 0 && !isPack {
			if len(rc.Parse.Volumes)+len(rc.Parse.Chapters) > 0 {
				dropped++
				continue
			}
			if rc.Parse.ContentType != parse.ContentManga {
				rc.Rejections = []string{"rejected.content_type"}
			} else if cand.Protocol != provider.ProtocolSource && (!rc.Parse.Anchored || rc.Parse.TitleLeftover != "") {
				rc.Rejections = []string{"rejected.series_match"}
			} else {
				rc.Rejections = []string{"rejected.numbers_unknown"}
			}
			out = append(out, rc)
			continue
		}
		if run.blocked[cand.Protocol+"/"+cand.ExternalKey()] {
			rc.Rejections = append(rc.Rejections, "rejected.blocklisted") // hard
		} else {
			{
				evidence := int64(0)
				if cand.Source != nil {
					evidence = run.sourceEditions[cand.Source.BindingID]
				}
				compatible := 0
				for _, t := range run.versions {
					col := editionByID(run.editions, t.EditionID)
					if run.wantedBooks[t.ID] != nil && col != nil && editionCompatibleWithEvidence(rc.Parse, *col, evidence) {
						compatible++
					}
				}
				rejectedShown := false
				for _, t := range run.versions {
					if run.wantedBooks[t.ID] == nil {
						continue
					}
					col := editionByID(run.editions, t.EditionID)
					if col == nil {
						continue
					}
					if !editionCompatibleWithEvidence(rc.Parse, *col, evidence) && run.scope == nil {
						if compatible > 0 || rejectedShown {
							continue
						}
						rejectedShown = true
					}
					scoped := *run
					scoped.scope = &SearchScope{VersionID: t.ID}
					if run.scope != nil {
						scoped.scope.BookID = run.scope.BookID
						scoped.scope.All = run.scope.All
						scoped.scope.AllBooks = run.scope.AllBooks
					}
					cp := rc
					cp.VersionID = t.ID
					cp.EditionID = col.ID
					cp.EditionSnapshot = col
					cp.PolicySnapshot = run.versionProfiles[t.ID]
					cp.VersionSnapshot = &t
					cp.FormatSnapshot = run.formats
					cp.Rejections = nil
					cp.Coverage = nil
					if (evidence != 0 && evidence != col.ID) || !editionCompatibleWithEvidence(cp.Parse, *col, evidence) {
						if !languageCompatible(cp.Parse, col.Language) {
							cp.Rejections = append(cp.Rejections, "rejected.language")
						} else {
							cp.Rejections = append(cp.Rejections, "rejected.edition_routing")
						}
					} else {
						s.funnel(&scoped, &cp, ranges)
						// The release fits this edition but not only this one.
						// That is a question for the user, not a contradiction:
						// never grabbed automatically, downloadable after review.
						if evidence == 0 && run.ambiguousEdition(cp.Parse, *col) {
							cp.Rejections = append(cp.Rejections, "rejected.edition_ambiguous")
						}
					}
					if t.RequiredGroup != "" {
						group := cp.Parse.Group
						if group == "" {
							group = cp.Scanlator
						}
						if !matchGroup(t.RequiredGroup, group) {
							cp.Rejections = append(cp.Rejections, "rejected.version_group")
						}
					}
					if !t.AllowUnknown && cp.Parse.Language == "" {
						cp.Rejections = append(cp.Rejections, "rejected.language_unknown")
					}
					if !t.IsDefault || compatible > 1 {
						cp.GUID = rc.GUID + "@" + strconv.FormatInt(t.ID, 10)
					}
					cp.applySitePriority(run.priorities[[2]string{cp.Provider, cp.SiteID}])
					out = append(out, cp)
				}
				continue
			}
		}
		rc.applySitePriority(run.priorities[[2]string{rc.Provider, rc.SiteID}])
		out = append(out, rc)
	}
	for i := range out {
		out[i].explain()
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out, dropped
}

// coverageRanges extracts the ledger-relevant number ranges of a candidate.
func (s *SearchService) coverageRanges(searchLedger string, rc *ReleaseCandidate) []parse.Range {
	if searchLedger == LedgerVolume {
		return bookRanges(rc.Parse, LedgerVolume)
	}
	ranges := bookRanges(rc.Parse, LedgerChapter)
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
	searchLedger, scope := run.ledger, run.scope
	collections := run.editions
	reject := func(code string) { c.Rejections = append(c.Rejections, code) }

	// An inferred pack runs the whole funnel so its books are known, then
	// carries a soft rejection: a person has to confirm it.
	if c.Parse.CompleteInferred && len(ranges) == 0 {
		defer reject("rejected.pack_unverified")
	}
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

	// The release is judged against one collected version at a time: the
	// caller scoped the run to it.
	colID := c.EditionID
	wantLang := ""
	for _, col := range collections {
		if col.ID == colID {
			wantLang = col.Language
		}
	}
	c.EditionID = colID
	// The routed edition's profile decides scoring and health rules
	// (edition override > series > instance default).
	prof := run.profileFor(colID)
	if p, ok := run.versionProfiles[c.VersionID]; ok {
		prof = p
	}
	if prof != nil {
		c.ProfileID = prof.ID
	}
	c.PolicySnapshot = prof
	c.EditionSnapshot = editionByID(collections, colID)
	rules := ledgerRules(prof, searchLedger)

	// R2 language gate (untagged passes). Soft: record and keep going so
	// Covered is still computed — a force-grab needs the book list.
	if !languageCompatible(c.Parse, wantLang) {
		reject("rejected.language")
	}

	total := 0
	for _, r := range ranges {
		total += int((int64(r.To)-int64(r.From))/100) + 1
	}
	for _, w := range run.books[colID] {
		if w.CountType != "main" && !c.Parse.Extra && (scope == nil || scope.BookID != w.ID) {
			continue
		}
		if w.CountType == "main" && c.Parse.Extra && (scope == nil || scope.BookID != w.ID) {
			continue
		}
		hit := len(ranges) == 0 && searchLedger == LedgerVolume && c.Parse.Complete
		for _, r := range ranges {
			if w.NumberKnown && w.SequenceNum >= int64(r.From) && w.SequenceNum <= int64(r.To) {
				hit = true
			}
		}
		if !hit {
			continue
		}
		if run.wantedBooks[c.VersionID][w.ID] {
			if scope == nil || scope.BookID == 0 {
				for _, other := range run.books[colID] {
					if other.ID != w.ID && other.NumberKnown && w.NumberKnown && other.SequenceNum == w.SequenceNum && other.CountType == w.CountType {
						reject("rejected.book_ambiguous")
						return
					}
				}
			}
			c.Coverage = append(c.Coverage, store.BookRef{Ledger: searchLedger, EditionID: colID, VersionID: c.VersionID, BookID: w.ID, Number: w.SequenceNum})
		}
	}
	if total == 0 {
		total = len(c.Coverage)
	}
	c.RepresentedBooks = total
	if total > 0 && len(c.Coverage) > 0 && len(c.Coverage)*2 < total {
		reject("rejected.pack_threshold")
	}
	if len(c.Coverage) == 0 {
		reject("rejected.no_coverage")
		return
	}

	// Custom formats: matched on every routable candidate (rejected ones
	// keep their chips), floor-gated softly so force-grab can override.
	c.evalFormats(prof, run.formats)
	if prof != nil && prof.UpgradesEnabled && c.VersionID > 0 && (run.scope == nil || run.scope.BookID == 0) {
		filtered := c.Coverage[:0]
		for _, sl := range c.Coverage {
			if old, ok := run.ownedScores[c.VersionID][sl.BookID]; ok {
				min := prof.MinUpgradeScore
				if min < 1 {
					min = 1
				}
				if c.FormatScore-old < min {
					continue
				}
			}
			filtered = append(filtered, sl)
		}
		c.Coverage = filtered
		if len(filtered) == 0 {
			reject("rejected.no_upgrade")
			return
		}
	}

	if prof != nil && prof.MinFormatScore > profile.MinScoreOff &&
		c.FormatScore < prof.MinFormatScore {
		reject("rejected.format_score")
	}

	// Torrent-only health checks (profile rules).
	if c.Protocol == provider.ProtocolTorrent {
		if c.Size > 0 && rules.SizeMaxMB > 0 {
			count := c.RepresentedBooks
			if count <= 0 {
				count = len(c.Coverage)
			}
			perBook := c.Size / int64(count)
			if perBook < int64(rules.SizeMinMB)<<20 || perBook > int64(rules.SizeMaxMB)<<20 {
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
	score := len(c.Coverage) * 1000
	c.ScoreParts = []ScorePart{{Label: "Wanted books", Value: score}}
	if c.Protocol == rules.PreferredProtocol {
		score += 200
		c.ScoreParts = append(c.ScoreParts, ScorePart{"Preferred protocol", 200})
	}
	score += c.FormatScore
	c.ScoreParts = append(c.ScoreParts, ScorePart{"Custom formats", c.FormatScore})
	switch {
	case c.Seeders > 20:
		score += 50
		c.ScoreParts = append(c.ScoreParts, ScorePart{"Torrent availability", 50})
	case c.Seeders > 5:
		score += 25
		c.ScoreParts = append(c.ScoreParts, ScorePart{"Torrent availability", 25})
	}
	if c.Parse.Confidence < 0.8 {
		score -= 100
		c.ScoreParts = append(c.ScoreParts, ScorePart{"Uncertain name", -100})
	}
	c.Score = score
}

// ledgerRules resolves a profile's funnel knobs for one ledger, falling
// back to the built-in defaults when no profile is configured.
func ledgerRules(prof *profile.Profile, searchLedger string) profile.LedgerRules {
	vol, ch := profile.DefaultRules()
	if prof != nil {
		vol, ch = prof.Volume, prof.Chapter
	}
	if searchLedger == LedgerChapter {
		return ch
	}
	return vol
}

// evalFormats matches the profile's custom formats against the candidate
// and sums their scores.
func (c *ReleaseCandidate) evalFormats(prof *profile.Profile, formats []profile.Format) {
	if prof == nil {
		return
	}
	group := c.Parse.Group
	if group == "" {
		group = c.Scanlator
	}
	in := profile.Input{
		Title: c.Title, Group: group, Language: c.Parse.Language,
		Protocol: c.Protocol, Site: c.Site, Resolution: c.Parse.Quality.Resolution,
		Flags: candidateFlags(*c),
	}
	for _, f := range formats {
		if !f.Matches(in) {
			continue
		}
		score := prof.FormatScores[f.ID]
		c.Formats = append(c.Formats, FormatHit{Name: f.Name, Score: score})
		c.FormatScore += score
	}
}

func baseLang(lang string) string {
	base, _, _ := strings.Cut(lang, "-")
	return strings.ToLower(base)
}

// Rejections that survive a force-grab (decision-engine.md §7).
var hardRejections = map[string]bool{
	"rejected.numbers_unknown":  true,
	"rejected.language":         true,
	"rejected.book_ambiguous":   true,
	"rejected.language_unknown": true,
	"rejected.no_upgrade":       true,
	"rejected.version_group":    true,
	"rejected.content_type":     true,
	"rejected.series_match":     true,
	"rejected.edition_routing":  true,
	"rejected.no_coverage":      true,
	"rejected.blocklisted":      true,
}

func editionByID(cols []store.Edition, id int64) *store.Edition {
	for i := range cols {
		if cols[i].ID == id {
			return &cols[i]
		}
	}
	return nil
}
func matchGroup(expr, value string) bool {
	r, err := regexp.Compile("(?i)" + expr)
	return err == nil && r.MatchString(value)
}

// Publishers that only release in English: their rip tag is language
// evidence. Publishers that also print in Japanese (Kodansha, Shonen Jump,
// Square Enix) and multilingual platforms are left out.
var englishPublishers = []string{"viz", "seven seas", "seven seas siren", "yen press", "dark horse", "j-novel club", "vertical", "ize press", "udon"}

func englishPublisher(names []string) bool {
	for _, n := range names {
		if slices.Contains(englishPublishers, n) {
			return true
		}
	}
	return false
}

func candidateFlags(c ReleaseCandidate) map[string]bool {
	return map[string]bool{"digital": c.Parse.Quality.Digital, "scan": c.Parse.Quality.Scan, "web": c.Parse.Quality.Web, "premium": c.Parse.Quality.Premium, "colored": c.Parse.Edition.Colored, "omnibus": c.Parse.Edition.Omnibus, "deluxe": c.Parse.Edition.Deluxe, "boxset": c.Parse.Edition.BoxSet, "official": c.Parse.Edition.Official, "raw": c.Parse.Edition.Raw, "complete": c.Parse.Complete}
}

func (c *ReleaseCandidate) applySitePriority(priority int) {
	if len(c.Rejections) > 0 {
		return
	}
	c.Score -= priority
	if priority != 0 {
		c.ScoreParts = append(c.ScoreParts, ScorePart{"Indexer priority", -priority})
	}
}

func languageCompatible(p parse.Result, want string) bool {
	if want == "" {
		return true
	}
	if len(p.Languages) > 0 {
		for _, lang := range p.Languages {
			if baseLang(lang) == baseLang(want) {
				return true
			}
		}
		return false
	}
	return p.Language == "" || p.Language == "multi" || baseLang(p.Language) == baseLang(want)
}
