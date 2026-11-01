package service

// The decision engine: raw provider candidates are parsed, gated by the
// hard/soft rejection rules and scored (decision-engine.md). Pure given
// the searchRun inputs — no I/O happens here.

import (
	"fmt"
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
		rc, ranges, keep := readRelease(run, cand)
		switch {
		case !keep:
			dropped++
		case len(rc.Rejections) > 0:
			rc.IndexerPriority = run.priorities[[2]string{rc.Provider, rc.SiteID}]
			out = append(out, rc)
		default:
			out = append(out, s.route(run, rc, ranges)...)
		}
	}
	for i := range out {
		out[i].Key = candidateKey(out[i])
		out[i].ContentKey = out[i].Provider + "/" + out[i].cand.ExternalKey()
		out[i].replaces = run.replaceQueueID
		if run.excludeRelease != "" && out[i].ContentKey == run.excludeRelease {
			out[i].Rejections = append(out[i].Rejections, "rejected.previous_download")
		}
		out[i].explain()
	}
	sort.SliceStable(out, func(i, j int) bool { return betterRelease(out[i], out[j]) })
	return out, dropped
}

// readRelease is what a release is, before any card is asked: its parse,
// the language its surroundings vouch for, and the books its name
// covers. keep is false for a release whose numbers fall outside the
// ledger; one that cannot be read at all, or is blocklisted, comes back
// with its rejection.
func readRelease(run *searchRun, cand provider.Candidate) (rc ReleaseCandidate, ranges []parse.Range, keep bool) {
	rc = ReleaseCandidate{
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
	// A source release named by its number alone ("Vol. 001 (Digital)")
	// is described by the listing it sits in: colour, the group, an
	// edition.
	if cand.Source != nil && cand.Source.Listing != "" {
		rc.Parse.Inherit(parse.ParseAnchored(cand.Source.Listing, channel, run.titles))
	}
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

	ranges = coverageRanges(run.ledger, &rc)
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
		switch {
		case len(rc.Parse.Volumes)+len(rc.Parse.Chapters) > 0:
			return rc, nil, false // numbered, but for the other ledger
		case rc.Parse.ContentType != parse.ContentManga:
			rc.Rejections = []string{"rejected.content_type"}
		case cand.Protocol != provider.ProtocolSource && (!rc.Parse.Anchored || rc.Parse.TitleLeftover != ""):
			rc.Rejections = []string{"rejected.series_match"}
		default:
			rc.Rejections = []string{"rejected.numbers_unknown"}
		}
		return rc, ranges, true
	}
	if run.blocked[cand.Protocol+"/"+cand.ExternalKey()] {
		rc.Rejections = append(rc.Rejections, "rejected.blocklisted") // hard
	}
	return rc, ranges, true
}

// route asks every card with wanted books whether the release is one of
// its books, and judges a copy of it for each card that says yes or
// cannot say. A release no card takes is shown once, for the first card,
// with why not; a run scoped to one card always shows it.
func (s *SearchService) route(run *searchRun, rc ReleaseCandidate, ranges []parse.Range) []ReleaseCandidate {
	var out []ReleaseCandidate
	evidence := int64(0)
	if rc.cand.Source != nil {
		evidence = run.sourceEditions[rc.cand.Source.BindingID]
	}
	compatible := 0
	for _, t := range run.versions {
		col := editionByID(run.editions, t.EditionID)
		if run.wantedBooks[t.ID] != nil && col != nil {
			if v, _ := run.shelf.judge(rc.Parse, *col, evidence); v == fitYes {
				compatible++
			}
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
		verdict, why := run.shelf.judge(rc.Parse, *col, evidence)
		if verdict != fitYes && run.scope == nil {
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
		cp.PolicySnapshot = run.cardProfile(t.ID)
		cp.VersionSnapshot = &t
		cp.FormatSnapshot = run.profiles.Formats
		cp.Rejections = nil
		cp.Coverage = nil
		cp.Contents = nil
		switch {
		case verdict == fitNo && !languageCompatible(cp.Parse, col.Language):
			cp.Rejections = append(cp.Rejections, "rejected.language")
		case verdict == fitNo:
			cp.Rejections = append(cp.Rejections, "rejected.edition_routing")
			cp.editionWhy, cp.OtherEdition = why, run.shelf.offer(cp.Parse, *col)
		default:
			s.funnel(&scoped, &cp, ranges)
			// Words alone leave the numbering open, or the release
			// fits another collected edition as well: a question for
			// the user, not a contradiction — never grabbed
			// automatically, downloadable after review.
			if verdict == fitUnsure {
				cp.Rejections = append(cp.Rejections, "rejected.edition_ambiguous")
				cp.editionWhy, cp.OtherEdition = why, run.shelf.offer(cp.Parse, *col)
			} else if other := run.ambiguousEdition(cp.Parse, *col); evidence == 0 && other != "" {
				cp.Rejections = append(cp.Rejections, "rejected.edition_ambiguous")
				cp.editionWhy = fmt.Sprintf("This release also fits %s, which you collect. Confirm it belongs to this one.", other)
			}
		}
		if !t.IsDefault || compatible > 1 {
			cp.GUID = rc.GUID + "@" + strconv.FormatInt(t.ID, 10)
		}
		cp.IndexerPriority = run.priorities[[2]string{cp.Provider, cp.SiteID}]
		out = append(out, cp)
	}
	return out
}

// coverageRanges extracts the ledger-relevant number ranges of a candidate.
func coverageRanges(searchLedger string, rc *ReleaseCandidate) []parse.Range {
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

// funnel judges a release routed to one card (the run is scoped to it):
// first whether it holds that card's wanted books, then what the card's
// profile says, then, when nothing stands against it, its score
// (decision-engine.md §3–4).
func (s *SearchService) funnel(run *searchRun, c *ReleaseCandidate, ranges []parse.Range) {
	// An inferred pack runs the whole funnel so its books are known, then
	// carries a soft rejection: a person has to confirm it.
	if c.Parse.CompleteInferred && len(ranges) == 0 {
		defer c.reject("rejected.pack_unverified")
	}
	prof := c.PolicySnapshot
	if prof != nil {
		c.ProfileID = prof.ID
	}
	if !fitBooks(run, c, ranges) || !judgeByProfile(run, c, prof) || len(c.Rejections) > 0 {
		return
	}
	c.score(ledgerRules(prof, run.ledger))
}

func (c *ReleaseCandidate) reject(code string) { c.Rejections = append(c.Rejections, code) }

// fitBooks is the book's side: is the release manga of this series, in
// the edition's language, and which of the card's books it holds and
// covers. false stops the funnel: nothing more is worth judging.
func fitBooks(run *searchRun, c *ReleaseCandidate, ranges []parse.Range) bool {
	searchLedger, scope := run.ledger, run.scope
	// R1 content-type gate.
	if c.Parse.ContentType != parse.ContentManga {
		c.reject("rejected.content_type")
		return false
	}
	// Series match (source candidates are binding-scoped; anchor waived).
	if c.Protocol != provider.ProtocolSource && (!c.Parse.Anchored || c.Parse.TitleLeftover != "") {
		c.reject("rejected.series_match")
		return false
	}
	// R2 language gate (untagged passes). Soft: record and keep going so
	// Covered is still computed — a force-grab needs the book list.
	if c.EditionSnapshot != nil && !languageCompatible(c.Parse, c.EditionSnapshot.Language) {
		c.reject("rejected.language")
	}

	colID := c.EditionID
	total := 0
	for _, r := range ranges {
		total += int((int64(r.To)-int64(r.From))/100) + 1
	}
	for _, w := range run.books[colID] {
		// Opening a search from a specific book names it outright, so the
		// kind rule only applies to books reached by number.
		if !bookKindFits(w, c.Parse.Extra) && (scope == nil || scope.BookID != w.ID) {
			continue
		}
		hit := (len(ranges) == 0 && searchLedger == LedgerVolume && c.Parse.Complete) || inRanges(w, ranges)
		if !hit {
			continue
		}
		c.Contents = append(c.Contents, store.BookRef{Ledger: searchLedger, EditionID: colID, VersionID: c.VersionID, BookID: w.ID, Number: w.SequenceNum})
		if run.wantedBooks[c.VersionID][w.ID] {
			if scope == nil || scope.BookID == 0 {
				if sharesNumber(run.books[colID], w) {
					c.reject("rejected.book_ambiguous")
					return false
				}
			}
			c.Coverage = append(c.Coverage, store.BookRef{Ledger: searchLedger, EditionID: colID, VersionID: c.VersionID, BookID: w.ID, Number: w.SequenceNum})
		}
	}
	if total == 0 {
		total = len(c.Coverage)
	}
	c.RepresentedBooks = total
	if len(c.Coverage) > 0 && belowPackThreshold(len(c.Coverage), total) {
		c.reject("rejected.pack_threshold")
	}
	if len(c.Coverage) == 0 {
		c.reject("rejected.no_coverage")
		return false
	}
	return true
}

// judgeByProfile is the profile's side: its formats and hard rules, the
// upgrade it asks for, its score floor and torrent health rules. false
// stops the funnel.
func judgeByProfile(run *searchRun, c *ReleaseCandidate, prof *profile.Profile) bool {
	// Custom formats: matched on every routable candidate (rejected ones
	// keep their chips), floor-gated softly so force-grab can override.
	c.evalFormats(prof, run.profiles.Formats)
	if prof != nil && prof.UpgradesEnabled && c.VersionID > 0 && (run.scope == nil || run.scope.BookID == 0) {
		filtered := c.Coverage[:0]
		for _, sl := range c.Coverage {
			if old, ok := run.ownedScores[c.VersionID][sl.BookID]; ok && c.FormatScore-old < max(prof.MinUpgradeScore, 1) {
				continue
			}
			filtered = append(filtered, sl)
		}
		c.Coverage = filtered
		if len(filtered) == 0 {
			c.reject("rejected.no_upgrade")
			return false
		}
	}
	if prof != nil && prof.MinFormatScore > profile.MinScoreOff && c.FormatScore < prof.MinFormatScore {
		c.reject("rejected.format_score")
	}
	// Torrent-only health checks.
	if c.Protocol == provider.ProtocolTorrent {
		rules := ledgerRules(prof, run.ledger)
		if c.Size > 0 && rules.SizeMaxMB > 0 {
			count := c.RepresentedBooks
			if count <= 0 {
				count = len(c.Coverage)
			}
			perBook := c.Size / int64(count)
			if perBook < int64(rules.SizeMinMB)<<20 || perBook > int64(rules.SizeMaxMB)<<20 {
				c.reject("rejected.size")
			}
		}
		if c.Seeders < rules.MinSeeders {
			c.reject("rejected.seeders")
		}
	}
	return true
}

// score ranks a release nothing stands against (decision-engine.md §4):
// wanted books first, the profile's preferred protocol, its custom
// formats, torrent health, and a doubt for an uncertain name.
func (c *ReleaseCandidate) score(rules profile.LedgerRules) {
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
	// A source release's listing is part of its name for formats: a
	// folder named "upscaled" makes every file in it so.
	title := c.Title
	if c.cand.Source != nil && c.cand.Source.Listing != "" {
		title = c.cand.Source.Listing + " " + title
	}
	in := profile.Input{
		Title: title, Group: group, Language: c.Parse.Language,
		Protocol: c.Protocol, Site: c.Site, Resolution: c.Parse.Quality.Resolution,
		Flags: candidateFlags(*c),
	}
	j := prof.Judge(in, formats)
	c.FormatScore = j.Score
	for _, f := range j.Matched {
		c.Formats = append(c.Formats, FormatHit{Name: f.Name, Score: prof.FormatScores[f.ID]})
	}
	if j.Broken != "" {
		c.Rejections = append(c.Rejections, "rejected.profile_rule")
		c.profileWhy = j.Broken
	}
}

func baseLang(lang string) string {
	base, _, _ := strings.Cut(lang, "-")
	return strings.ToLower(base)
}

// Rejections that survive a force-grab (decision-engine.md §7).
var hardRejections = map[string]bool{
	"rejected.profile_rule":      true,
	"rejected.previous_download": true,
	"rejected.numbers_unknown":   true,
	"rejected.language":          true,
	"rejected.book_ambiguous":    true,
	"rejected.no_upgrade":        true,
	"rejected.content_type":      true,
	"rejected.series_match":      true,
	"rejected.edition_routing":   true,
	"rejected.no_coverage":       true,
	"rejected.blocklisted":       true,
}

func editionByID(cols []store.Edition, id int64) *store.Edition {
	for i := range cols {
		if cols[i].ID == id {
			return &cols[i]
		}
	}
	return nil
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

// betterRelease orders releases: the profile's score decides; between
// equal scores, the indexer with the higher priority (the lower number)
// wins. Priority is a property of the indexer, never part of the score.
func betterRelease(a, b ReleaseCandidate) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	return a.IndexerPriority < b.IndexerPriority
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
