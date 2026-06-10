package parse

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
	Title       string       `json:"title,omitempty"`
	AltTitles   []string     `json:"altTitles,omitempty"`
	Volumes     []Range      `json:"volumes,omitempty"`
	Chapters    []Range      `json:"chapters,omitempty"`
	Bare        []Range      `json:"bare,omitempty"` // axis deferred to resolver
	Season      *int         `json:"season,omitempty"`
	Year        *int         `json:"year,omitempty"`
	YearRange   *[2]int      `json:"yearRange,omitempty"`
	Group       string       `json:"group,omitempty"`
	Language    string       `json:"language,omitempty"`
	ContentType ContentType  `json:"contentType"`
	Edition     EditionHints `json:"edition"`
	Quality     QualityHints `json:"quality"`
	Anchored    bool         `json:"anchored,omitempty"` // title matched a known candidate
	// TitleLeftover holds unexplained words around an anchored title
	// ("WANTED! Eiichiro Oda Before" around "One Piece") — a strong signal
	// the release is a different work that merely contains the title.
	TitleLeftover string  `json:"titleLeftover,omitempty"`
	Complete      bool    `json:"complete,omitempty"`
	Ongoing       bool    `json:"ongoing,omitempty"`
	Extra         bool    `json:"extra,omitempty"`
	Version       int     `json:"version,omitempty"`
	Confidence    float64 `json:"confidence"`
	Trace         []Step  `json:"-"`
}

func (r *Result) step(rule, text string) {
	r.Trace = append(r.Trace, Step{Rule: rule, Text: text})
}
