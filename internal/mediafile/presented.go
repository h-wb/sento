package mediafile

// PresentedField is a ComicInfo field that shows a copy (an edition, as
// one of its versions) to readers the way the user chose: rendered for
// each copy from a template in the media settings, or the edition's own,
// instead of being a fact Sento reads back (docs/domain/metadata.md).
// Adding one is adding an entry here.
type PresentedField struct {
	// Key names the field in settings and edition overrides.
	Key string
	// Label names it in messages: "the series name in readers".
	Label string
	// Title and Help are how the settings page offers it.
	Title string
	Help  string
	// Default is the template used until the user sets another.
	Default string
	field   func(*ComicInfo) *string
}

// PresentedFields are the fields readers file and group a copy by.
var PresentedFields = []PresentedField{
	// The series a reader files the copy under: an additional version
	// names itself, so a reader keeps it apart from the default one.
	{Key: "series", Label: "series name", Default: "{Series Title} {version}",
		Title: "Series name in readers",
		Help:  "Written into each book, so Kavita, Komga and other readers file it under this series. {version} is empty for an edition’s first card. An edition can set its own name in Edit edition. Also usable as {Reader Series} in folder and file names.",
		field: func(c *ComicInfo) *string { return &c.Series }},
	// What the copies have in common, for readers that collect series.
	{Key: "seriesGroup", Label: "series group", Default: "{Series Title}",
		Title: "Series group",
		Help:  "What the copies of a series share, for readers that build collections from it.",
		field: func(c *ComicInfo) *string { return &c.SeriesGroup }},
}

// Set writes the field's value into a ComicInfo.
func (f PresentedField) Set(c *ComicInfo, value string) { *f.field(c) = value }

// Get reads the field's value from a ComicInfo.
func (f PresentedField) Get(c *ComicInfo) string { return *f.field(c) }
