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

	"github.com/h-wb/sento/internal/metadata"
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
		// the release language (drives the R2 gate and collection routing).
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
			if len(run.targets) > 0 {
				evidence := int64(0)
				if cand.Source != nil {
					evidence = run.sourceEditions[cand.Source.BindingID]
				}
				compatible := 0
				for _, t := range run.targets {
					col := collectionByID(run.collections, t.CollectionID)
					if run.targetWanted[t.ID] != nil && col != nil && editionCompatibleWithEvidence(rc.Parse, *col, evidence) {
						compatible++
					}
				}
				rejectedShown := false
				for _, t := range run.targets {
					if run.targetWanted[t.ID] == nil {
						continue
					}
					col := collectionByID(run.collections, t.CollectionID)
					if col == nil {
						continue
					}
					if !editionCompatibleWithEvidence(rc.Parse, *col, evidence) && run.target == nil {
						if compatible > 0 || rejectedShown {
							continue
						}
						rejectedShown = true
					}
					scoped := *run
					scoped.target = &TargetSlot{CollectionID: t.CollectionID, TargetID: t.ID}
					if run.target != nil {
						scoped.target.WorkID = run.target.WorkID
						scoped.target.Number = run.target.Number
						scoped.target.All = run.target.All
						scoped.target.AllBooks = run.target.AllBooks
					}
					scoped.wanted = map[int64]map[int64]bool{t.CollectionID: {}}
					for _, w := range run.units[t.CollectionID] {
						if run.targetWanted[t.ID][w.ID] {
							scoped.wanted[t.CollectionID][w.SequenceNum] = true
						}
					}
					cp := rc
					cp.TargetID = t.ID
					cp.CollectionID = col.ID
					cp.EditionSnapshot = col
					cp.PolicySnapshot = run.targetProfiles[t.ID]
					cp.TargetSnapshot = &t
					cp.FormatSnapshot = run.formats
					cp.Rejections = nil
					cp.Covered = nil
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
							cp.Rejections = append(cp.Rejections, "rejected.target_group")
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
			s.funnel(run, &rc, ranges)
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
	collections, wanted := run.collections, run.wanted
	reject := func(code string) { c.Rejections = append(c.Rejections, code) }

	if c.Parse.CompleteInferred && len(ranges) == 0 {
		reject("rejected.pack_unverified")
		return
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

	// Routing: both ledgers route to a collection of their ctype (R6) —
	// the wanted map only carries collections of the searched ledger.
	colID := int64(0)
	if target != nil {
		colID = target.CollectionID
	} else {
		var ok bool
		colID, ok = routeCollection(c.Parse, collections, wanted)
		if !ok {
			reject("rejected.edition_routing")
			return
		}
	}
	wantLang := ""
	for _, col := range collections {
		if col.ID == colID {
			wantLang = col.Language
		}
	}
	c.CollectionID = colID
	// The routed collection's profile decides scoring and health rules
	// (collection override > series > instance default).
	prof := run.profileFor(colID)
	if p, ok := run.targetProfiles[c.TargetID]; ok {
		prof = p
	}
	if prof != nil {
		c.ProfileID = prof.ID
	}
	c.PolicySnapshot = prof
	c.EditionSnapshot = collectionByID(collections, colID)
	rules := ledgerRules(prof, searchLedger)

	// R2 language gate (untagged passes). Soft: record and keep going so
	// Covered is still computed — a force-grab needs the slot list.
	if !languageCompatible(c.Parse, wantLang) {
		reject("rejected.language")
	}

	if c.TargetID > 0 {
		total := 0
		for _, r := range ranges {
			total += int((int64(r.To)-int64(r.From))/100) + 1
		}
		for _, w := range run.units[colID] {
			if w.CountType != "main" && !c.Parse.Extra && (target == nil || target.WorkID != w.ID) {
				continue
			}
			if w.CountType == "main" && c.Parse.Extra && (target == nil || target.WorkID != w.ID) {
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
			if run.targetWanted[c.TargetID][w.ID] {
				if target == nil || target.WorkID == 0 {
					for _, other := range run.units[colID] {
						if other.ID != w.ID && other.NumberKnown && w.NumberKnown && other.SequenceNum == w.SequenceNum && other.CountType == w.CountType {
							reject("rejected.unit_ambiguous")
							return
						}
					}
				}
				c.Covered = append(c.Covered, w.SequenceNum)
				c.Coverage = append(c.Coverage, store.ExpectedSlot{Ledger: searchLedger, CollectionID: colID, TargetID: c.TargetID, WorkID: w.ID, Number: w.SequenceNum})
			}
		}
		if total == 0 {
			total = len(c.Coverage)
		}
		c.RepresentedUnits = total
		if total > 0 && len(c.Coverage) > 0 && len(c.Coverage)*2 < total {
			reject("rejected.pack_threshold")
		}
	} else {
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

	}
	if len(c.Covered) == 0 {
		reject("rejected.no_coverage")
		return
	}
	sort.Slice(c.Covered, func(i, j int) bool { return c.Covered[i] < c.Covered[j] })

	// Custom formats: matched on every routable candidate (rejected ones
	// keep their chips), floor-gated softly so force-grab can override.
	c.evalFormats(prof, run.formats)
	if prof != nil && prof.UpgradesEnabled && c.TargetID > 0 && (run.target == nil || run.target.WorkID == 0) {
		filtered := c.Coverage[:0]
		numbers := []int64{}
		for _, sl := range c.Coverage {
			if old, ok := run.ownedScores[c.TargetID][sl.WorkID]; ok {
				min := prof.MinUpgradeScore
				if min < 1 {
					min = 1
				}
				if c.FormatScore-old < min {
					continue
				}
			}
			filtered = append(filtered, sl)
			numbers = append(numbers, sl.Number)
		}
		c.Coverage = filtered
		c.Covered = numbers
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
			count := c.RepresentedUnits
			if count <= 0 {
				count = len(c.Covered)
			}
			perUnit := c.Size / int64(count)
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

// routeCollection maps edition hints to a monitored collection (R6) —
// volume editions and chapter collections alike (the wanted map scopes
// the candidates to one ctype). A language-tagged release prefers
// collections of that language (custom fr variants receive FR releases;
// en/untagged keep the original) — falling back to all monitored when no
// language matches.
func routeCollection(p parse.Result, collections []store.Collection,
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
	// Untagged release, no name hint: the oldest collection wins — stable,
	// and the original (pre-customs) collection has the lowest id.
	best := monitored[0]
	for _, col := range monitored[1:] {
		if col.ID < best.ID {
			best = col
		}
	}
	return best.ID, true
}

// Rejections that survive a force-grab (decision-engine.md §7).
var hardRejections = map[string]bool{
	"rejected.numbers_unknown":  true,
	"rejected.pack_unverified":  true,
	"rejected.language":         true,
	"rejected.unit_ambiguous":   true,
	"rejected.language_unknown": true,
	"rejected.no_upgrade":       true,
	"rejected.target_group":     true,
	"rejected.content_type":     true,
	"rejected.series_match":     true,
	"rejected.edition_routing":  true,
	"rejected.no_coverage":      true,
	"rejected.blocklisted":      true,
}

func collectionByID(cols []store.Collection, id int64) *store.Collection {
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

// Absence of evidence is never proof of a special edition. Source quality
// and publication medium are different, but explicit digital/scan tags help
// disambiguate the standard publisher runs.
func editionCompatible(p parse.Result, c store.Collection) bool {
	if !languageCompatible(p, c.Language) {
		return false
	}
	traits := c.EditionTraits
	if traits == nil {
		traits = metadata.EditionTraits(c.EditionName)
	}
	variant := c.ContentVariant
	if variant == "" {
		variant = metadata.ContentVariant(c.EditionName)
	}
	if p.Edition.Colored != (variant == "colored") {
		return false
	}
	for trait, present := range map[string]bool{"omnibus": p.Edition.Omnibus, "boxset": p.Edition.BoxSet, "deluxe": p.Edition.Deluxe} {
		if present != slices.Contains(traits, trait) {
			return false
		}
	}
	// Medium is deliberately not checked: "(Digital)" says how a file was
	// produced, not which book it is. It only breaks ties (preferByMedium).
	return true
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

// preferByMedium narrows editions a release fits by how the file was
// produced: a digital rip belongs to the digital edition and a scan to the
// printed one, when both are in play. With no such evidence, or no edition
// of that medium, every edition stays. Medium never chooses between
// languages: a digital rip in an unstated language is not thereby English.
func preferByMedium(p parse.Result, cols []store.Collection) []store.Collection {
	for _, c := range cols {
		if baseLang(c.Language) != baseLang(cols[0].Language) {
			return cols
		}
	}
	var fits func(store.Collection) bool
	switch {
	case p.Quality.Digital:
		fits = func(c store.Collection) bool { return c.Medium == "digital" }
	case p.Quality.Scan:
		fits = func(c store.Collection) bool { return c.Medium != "" && c.Medium != "digital" }
	default:
		return cols
	}
	var out []store.Collection
	for _, c := range cols {
		if fits(c) {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return cols
	}
	return out
}

// ambiguousEdition reports whether a release that fits col fits another
// edition the user collects at least as well. Catalog editions nobody
// collects do not compete: the user already chose against them.
func (run *searchRun) ambiguousEdition(p parse.Result, col store.Collection) bool {
	fits := []store.Collection{col}
	otherLanguage := false
	for _, c := range run.collections {
		if c.ID == col.ID || c.Type != col.Type || c.SyncState == "stale" || c.SyncState == "superseded" {
			continue
		}
		collected := false
		for _, t := range run.targets {
			collected = collected || (t.CollectionID == c.ID && t.Monitored)
		}
		if !collected || !editionCompatible(p, c) {
			continue
		}
		if baseLang(c.Language) != baseLang(col.Language) {
			otherLanguage = true
			continue
		}
		fits = append(fits, c)
	}
	best := preferByMedium(p, fits)
	return otherLanguage || len(best) != 1 || best[0].ID != col.ID
}

func candidateFlags(c ReleaseCandidate) map[string]bool {
	return map[string]bool{"digital": c.Parse.Quality.Digital, "scan": c.Parse.Quality.Scan, "web": c.Parse.Quality.Web, "premium": c.Parse.Quality.Premium, "colored": c.Parse.Edition.Colored, "omnibus": c.Parse.Edition.Omnibus, "deluxe": c.Parse.Edition.Deluxe, "boxset": c.Parse.Edition.BoxSet, "official": c.Parse.Edition.Official, "raw": c.Parse.Edition.Raw, "complete": c.Parse.Complete}
}

func editionCompatibleWithEvidence(p parse.Result, c store.Collection, evidence int64) bool {
	if evidence == 0 {
		return editionCompatible(p, c)
	}
	if evidence != c.ID {
		return false
	}
	// A verified listing establishes publication identity. It does not
	// excuse explicit contradictory language or edition flags.
	if !languageCompatible(p, c.Language) {
		return false
	}
	traits := c.EditionTraits
	if traits == nil {
		traits = metadata.EditionTraits(c.EditionName)
	}
	variant := c.ContentVariant
	if variant == "" {
		variant = metadata.ContentVariant(c.EditionName)
	}
	return (!p.Edition.Omnibus || (slices.Contains(traits, "omnibus") || len(traits) == 0)) && (!p.Edition.BoxSet || (slices.Contains(traits, "boxset") || len(traits) == 0)) && (!p.Edition.Deluxe || (slices.Contains(traits, "deluxe") || len(traits) == 0)) && (!p.Edition.Colored || (variant == "colored" || variant == "unknown"))
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
