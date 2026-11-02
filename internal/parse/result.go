package parse

import "slices"

// Channel biases interpretation of ambiguous names (parser.md §1).
type Channel string

const (
	ChannelTorrent Channel = "torrent"
	ChannelSource  Channel = "source"
	ChannelFile    Channel = "file"
)

type ContentType string

const (
	ContentManga     ContentType = "manga"
	ContentNovel     ContentType = "novel"
	ContentAudiobook ContentType = "audiobook"
	ContentArtbook   ContentType = "artbook"
	ContentAnime     ContentType = "anime"
	// Not reading material at all: a general tracker answers a title
	// search with films, episodes and games of the same name.
	ContentVideo    ContentType = "video"
	ContentSoftware ContentType = "software"
)

type EditionHints struct {
	Omnibus  bool     `json:"omnibus,omitempty"`
	Deluxe   bool     `json:"deluxe,omitempty"`
	BoxSet   bool     `json:"boxset,omitempty"`
	Colored  bool     `json:"colored,omitempty"`
	Official bool     `json:"official,omitempty"`
	Raw      bool     `json:"raw,omitempty"`
	Names    []string `json:"names,omitempty"`
}

type QualityHints struct {
	Digital    bool `json:"digital,omitempty"`
	Scan       bool `json:"scan,omitempty"`
	Web        bool `json:"web,omitempty"`
	Premium    bool `json:"premium,omitempty"`
	Resolution int  `json:"resolution,omitempty"`
}

// Step records one rule firing, for explainability (parser.md §1).
type Step struct {
	Rule string `json:"rule"`
	Text string `json:"text,omitempty"`
}

type Result struct {
	Title     string   `json:"title,omitempty"`
	AltTitles []string `json:"altTitles,omitempty"`
	Volumes   []Range  `json:"volumes,omitempty"`
	Chapters  []Range  `json:"chapters,omitempty"`
	Bare      []Range  `json:"bare,omitempty"` // axis deferred to resolver
	// BarePadded are the unresolved bare numbers written zero-padded to
	// three or more digits ("001"), the chapter-number convention; the
	// padding is a fact for whoever resolves the axis, not one.
	BarePadded  []Range      `json:"barePadded,omitempty"`
	Season      *int         `json:"season,omitempty"`
	Year        *int         `json:"year,omitempty"`
	YearRange   *[2]int      `json:"yearRange,omitempty"`
	Group       string       `json:"group,omitempty"`
	Languages   []string     `json:"languages,omitempty"`
	Language    string       `json:"language,omitempty"`
	ContentType ContentType  `json:"contentType"`
	Edition     EditionHints `json:"edition"`
	// Claim is what the name says about the numbering it belongs to.
	Claim    EditionClaim `json:"claim"`
	Quality  QualityHints `json:"quality"`
	Anchored bool         `json:"anchored,omitempty"` // title matched a known candidate
	// TitleLeftover holds unexplained words around an anchored title
	// ("WANTED! Eiichiro Oda Before" around "One Piece") — a strong signal
	// the release is a different work that merely contains the title.
	TitleLeftover    string  `json:"titleLeftover,omitempty"`
	CompleteInferred bool    `json:"completeInferred,omitempty"`
	Complete         bool    `json:"complete,omitempty"`
	Ongoing          bool    `json:"ongoing,omitempty"`
	Extra            bool    `json:"extra,omitempty"`
	Version          int     `json:"version,omitempty"`
	Confidence       float64 `json:"confidence"`
	Trace            []Step  `json:"-"`
	// name is the cleaned, lowercased input, kept for Mentions.
	name string
}

func (r *Result) step(rule, text string) {
	r.Trace = append(r.Trace, Step{Rule: rule, Text: text})
}

// Inherit fills what a release's own name leaves unsaid from the name of
// the listing it sits in ("Vol. 001 (Digital)" in "One Piece - Digital
// Colored Comics"): the edition it claims, colour, a box, the group, the
// language, the publisher and how it was made. What the release says of
// itself always wins.
func (r *Result) Inherit(listing Result) {
	if !r.Claim.Named() && listing.Claim.Named() {
		r.Claim.Regroup, r.Claim.Own, r.Claim.ColorEdition, r.Claim.Written =
			listing.Claim.Regroup, listing.Claim.Own, listing.Claim.ColorEdition, listing.Claim.Written
	}
	r.Claim.Color = r.Claim.Color || listing.Claim.Color
	r.Claim.Box = r.Claim.Box || listing.Claim.Box
	e, l := &r.Edition, listing.Edition
	e.Omnibus, e.Deluxe, e.BoxSet = e.Omnibus || l.Omnibus, e.Deluxe || l.Deluxe, e.BoxSet || l.BoxSet
	e.Colored, e.Official, e.Raw = e.Colored || l.Colored, e.Official || l.Official, e.Raw || l.Raw
	for _, n := range l.Names {
		if !slices.Contains(e.Names, n) {
			e.Names = append(e.Names, n)
		}
	}
	q, lq := &r.Quality, listing.Quality
	q.Digital, q.Scan, q.Web, q.Premium = q.Digital || lq.Digital, q.Scan || lq.Scan, q.Web || lq.Web, q.Premium || lq.Premium
	q.Resolution = max(q.Resolution, lq.Resolution)
	if r.Group == "" {
		r.Group = listing.Group
	}
	if r.Language == "" {
		r.Language, r.Languages = listing.Language, listing.Languages
	}
	r.step("listing.inherited", "")
}
