package parse

import (
	"regexp"
	"strconv"
	"strings"
)

// Tag tables (parser.md §5). Segment classification is token-based: a
// bracket/paren/brace segment is scanned for known tokens/phrases; every
// hit folds hints into the Result. A segment with no hits and no numbers
// is a group candidate.

var languageTags = map[string]string{
	"eng": "en", "english": "en", "en": "en",
	"fr": "fr", "vf": "fr", "french": "fr", "fre": "fr",
	"jp": "ja", "ja": "ja", "jap": "ja", "japanese": "ja",
	"es": "es", "esp": "es", "spanish": "es",
	"pt-br": "pt-BR", "pt": "pt",
	"kr": "ko", "ko": "ko", "korean": "ko",
	"zh": "zh", "chinese": "zh",
}

var contentTypeTags = map[string]ContentType{
	"novel": ContentNovel, "ln": ContentNovel, "light novel": ContentNovel,
	"audiobook": ContentAudiobook,
	"artbook":   ContentArtbook, "fanbook": ContentArtbook,
	"anthology": ContentArtbook, "guidebook": ContentArtbook,
	"anime": ContentAnime,
}

// Publisher/platform rip tags → Edition.Official (parser.md §5.4).
var publisherTags = []string{
	"viz", "seven seas", "seven seas siren", "yen press", "kobo", "comixology",
	"shonen jump", "kodansha", "dark horse", "square enix", "j-novel club",
	"vertical", "tapas", "webtoon", "manga plus", "ize press", "udon",
}

// Unambiguous language words also found in free text (scene naming:
// "One.piece.T01.FRENCH.HYBRiD"). Two-letter codes stay segment-only.
var freeTextLangWords = map[string]string{
	"french": "fr", "francais": "fr", "vf": "fr", "vostfr": "fr",
	"english": "en", "japanese": "ja", "spanish": "es",
	"korean": "ko", "chinese": "zh", "german": "de", "italian": "it",
	"portuguese": "pt", "russian": "ru", "polish": "pl", "multi": "multi",
}

// Bare two-letter codes are ambiguous in prose but conventional in scene
// names ("One.Piece.T112.2026.FR.[CBR]") — accepted only as standalone
// UPPERCASE words, and only codes that don't collide with common words
// in shouting-case titles (EN/DE/ES/IT stay out).
var sceneLangCodes = map[string]string{
	"FR": "fr", "JP": "ja", "JA": "ja", "KR": "ko",
	"CN": "zh", "RU": "ru", "PL": "pl",
}

type segmentHints struct {
	language    string
	contentType ContentType
	edition     EditionHints
	quality     QualityHints
	year        *int
	yearRange   *[2]int
	complete    bool
	ongoing     bool
	version     int
	recognized  bool // at least one token classified
}

var (
	// reYear matches a whole bracketed segment that is just a 4-digit year.
	// Known limit (parser.md §9): a bare bracketed chapter number in this
	// range (e.g. "[1960]") reads as a year, not a chapter. Harmless in
	// practice — no series has reached chapter 1800+, and marked chapters
	// ("Chapter 1960") never hit this path.
	reYear      = regexp.MustCompile(`^(1[89]\d\d|20\d\d|2100)$`)
	reYearRange = regexp.MustCompile(`^(1[89]\d\d|20\d\d)\s*[-–~]\s*(1[89]\d\d|20\d\d|2100)$`)
	reVersion   = regexp.MustCompile(`^v(\d)$`)
	reResPx     = regexp.MustCompile(`^x?(\d{3,4})px?$|^(\d{3,4})px$`)
)

// classifySegment inspects one delimited segment's text.
func classifySegment(text string) segmentHints {
	var h segmentHints
	lower := strings.ToLower(strings.TrimSpace(text))

	if m := reYearRange.FindStringSubmatch(lower); m != nil {
		from, _ := strconv.Atoi(m[1])
		to, _ := strconv.Atoi(m[2])
		h.yearRange = &[2]int{from, to}
		h.recognized = true
		return h
	}
	if reYear.MatchString(lower) {
		y, _ := strconv.Atoi(lower)
		h.year = &y
		h.recognized = true
		return h
	}
	if m := reVersion.FindStringSubmatch(lower); m != nil {
		h.version, _ = strconv.Atoi(m[1])
		h.recognized = true
		return h
	}
	if lower == "f" { // (F) = fixed re-release
		h.version = 2
		h.recognized = true
		return h
	}

	// Whole-segment matches first (multi-word tags).
	if lang, ok := languageTags[lower]; ok {
		h.language = lang
		h.recognized = true
		return h
	}
	if ct, ok := contentTypeTags[lower]; ok {
		h.contentType = ct
		h.recognized = true
		return h
	}

	// Compound segment ("FULL COLOR Digital Colored Comics"): the shared
	// vocabulary scanner, same one the free-text pass uses.
	return scanTags(lower)
}

// scanTags detects edition / quality / completeness / content vocabulary in
// already-lowercased text. Matching is word-boundary aware (so "raw" never
// fires inside "drawback", "color" not inside "discolored"). It is the one
// place this vocabulary lives: classifySegment runs it on a delimited
// segment, and parseInto runs it on free text, so a tag means the same
// thing whether or not it sits in brackets.
func scanTags(lower string) segmentHints {
	var h segmentHints
	tokens := strings.FieldsFunc(lower, func(r rune) bool {
		return r == ' ' || r == ',' || r == '/' || r == ';'
	})
	scanned := strings.Join(tokens, " ")

	phrase := func(p string) bool { return indexWord(scanned, p) >= 0 }

	if phrase("digital colored comics") || phrase("full color") || phrase("colored") || phrase("coloured") {
		h.edition.Colored = true
		h.recognized = true
	}
	if phrase("omnibus") || phrase("2-in-1") || phrase("3-in-1") || phrase("2in1") || phrase("3in1") {
		h.edition.Omnibus = true
		h.recognized = true
	}
	if phrase("deluxe") {
		h.edition.Deluxe = true
		h.recognized = true
	}
	if phrase("box set") || phrase("boxset") {
		h.edition.BoxSet = true
		h.recognized = true
	}
	if phrase("raw") {
		h.edition.Raw = true
		if h.language == "" {
			h.language = "ja"
		}
		h.recognized = true
	}
	for _, pub := range publisherTags {
		if scanned == pub || phrase(pub) {
			h.edition.Official = true
			h.edition.Names = append(h.edition.Names, pub)
			if pub == "webtoon" || pub == "tapas" {
				h.quality.Web = true
			}
			h.recognized = true
		}
	}

	for _, tok := range tokens {
		switch tok {
		case "digital":
			h.quality.Digital = true
			h.recognized = true
		case "scan", "scans", "scanlated", "scanlation", "c2c":
			h.quality.Scan = true
			h.recognized = true
		case "premium":
			h.quality.Premium = true
			h.recognized = true
		case "compressed", "jpeg", "jpg", "png", "hq", "lq", "batch":
			h.recognized = true // noise tags: recognized, no hint
		case "complete", "end", "completed":
			h.complete = true
			h.recognized = true
		case "full":
			// "full" means a complete pack only on its own — inside
			// "full color" it qualifies the edition, not completeness.
			if !phrase("full color") && !phrase("full colour") {
				h.complete = true
			}
			h.recognized = true
		case "ongoing":
			h.ongoing = true
			h.recognized = true
		case "oneshot", "one-shot":
			h.recognized = true
		default:
			if m := reResPx.FindStringSubmatch(tok); m != nil {
				v := m[1]
				if v == "" {
					v = m[2]
				}
				h.quality.Resolution, _ = strconv.Atoi(v)
				h.recognized = true
			}
		}
	}

	if ct, ok := contentTypeTags[scanned]; ok {
		h.contentType = ct
		h.recognized = true
	}

	return h
}

// fold merges segment hints into the result.
func (r *Result) fold(h segmentHints, text string) {
	if h.language != "" && r.Language == "" {
		r.Language = h.language
	}
	if h.contentType != "" {
		r.ContentType = h.contentType
	}
	if h.year != nil && r.Year == nil {
		r.Year = h.year
	}
	if h.yearRange != nil && r.YearRange == nil {
		r.YearRange = h.yearRange
	}
	if h.version > r.Version {
		r.Version = h.version
	}
	if h.complete {
		r.Complete = true
	}
	if h.ongoing {
		r.Ongoing = true
		r.Complete = false
	}
	e, q := &r.Edition, &r.Quality
	e.Omnibus = e.Omnibus || h.edition.Omnibus
	e.Deluxe = e.Deluxe || h.edition.Deluxe
	e.BoxSet = e.BoxSet || h.edition.BoxSet
	e.Colored = e.Colored || h.edition.Colored
	e.Official = e.Official || h.edition.Official
	e.Raw = e.Raw || h.edition.Raw
	e.Names = append(e.Names, h.edition.Names...)
	q.Digital = q.Digital || h.quality.Digital
	q.Scan = q.Scan || h.quality.Scan
	q.Web = q.Web || h.quality.Web
	q.Premium = q.Premium || h.quality.Premium
	if h.quality.Resolution > q.Resolution {
		q.Resolution = h.quality.Resolution
	}
	if h.recognized {
		r.step("segment.classified", text)
	}
}
