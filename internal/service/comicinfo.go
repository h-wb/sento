package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/h-wb/sento/internal/mediafile"
	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/store"
)

// maxTags bounds the Tags field: MangaBaka can carry well over a hundred
// community tags per series, most of them noise in a reader's filter list.
const maxTags = 30

// tagSource is what a file came as: the release, its group, how and where
// it was found, and the release's flags. Any of it may be empty.
type tagSource struct {
	Release, Group, Via, Site string
	Flags                     map[string]bool
}

// releaseSource is the tagSource of a queued download.
func releaseSource(item store.QueueItem) tagSource {
	var snap ReleaseCandidate
	_ = json.Unmarshal(item.Snapshot, &snap)
	group := snap.Parse.Group
	if group == "" {
		group = snap.Scanlator
	}
	return tagSource{Release: item.Title, Group: group, Via: item.Protocol, Site: snap.Site, Flags: candidateFlags(snap)}
}

// fileInfo builds the ComicInfo for a file holding the given books of one
// edition: everything a reader needs to file and describe it without
// Sento, and Sento's Notes line to adopt it back by (docs/domain/metadata.md).
// The catalog rows come from cat, so a whole-series pass loads them once.
func (im *Importer) fileInfo(ctx context.Context, cat *catalogCache, ms MediaSettings, series *store.Series, slots []store.BookRef, src tagSource) mediafile.ComicInfo {
	first, last := slotSpan(slots)
	info := mediafile.ComicInfo{
		Series:          series.Title,
		Summary:         series.Description,
		Web:             fmt.Sprintf("https://mangabaka.org/%d", series.MangaBakaID),
		ScanInformation: src.Group,
	}
	if series.Year > 0 {
		info.Year = strconv.Itoa(series.Year)
	}
	if series.Type == "manga" {
		info.Manga = "YesAndRightToLeft"
	}
	prov := mediafile.Provenance{MangaBaka: series.MangaBakaID, Ledger: first.Ledger, Release: src.Release, Via: src.Via, Site: src.Site}
	for flag, on := range src.Flags {
		if on {
			prov.Flags = append(prov.Flags, flag)
		}
	}
	slices.Sort(prov.Flags)

	if col, _ := cat.edition(ctx, first.EditionID); col != nil {
		info.LanguageISO = col.Language
		info.Format = editionFormat(col.EditionName)
		// A scanlator's chapter is a fan translation, not the publisher's
		// book, unless the release names a publisher or platform.
		if src.Via == "source" && !src.Flags["official"] {
			info.Translator = src.Group
		} else {
			info.Publisher = col.Publisher
		}
		prov.Edition = col.EditionName
		if !col.Custom {
			prov.CatalogEdition = col.ProviderEditionID
		}
		// How readers file and group this copy: the user's templates.
		version, _ := cat.version(ctx, first.VersionID)
		presentCopy(&info, ms, series, *col, version)
	}
	books := map[int64]*store.Book{}
	for _, sl := range slots {
		prov.Books = append(prov.Books, mediafile.RenderPlainNum(sl.Number))
		if w, err := cat.book(ctx, sl.BookID); err == nil {
			books[sl.BookID] = w
			if w.ProviderID != "" {
				prov.Works = append(prov.Works, w.ProviderID)
			}
		}
	}
	info.Notes = prov.Notes()

	// Several books: series-level facts only, since no single number,
	// date or ISBN describes the file.
	if len(slots) > 1 {
		seriesRaw, _, _ := cat.payloads(ctx, 0)
		applyCatalog(&info, seriesRaw, "")
		info.Summary = series.Description
		kind := "Volumes"
		if first.Ledger == "chapter" {
			kind = "Chapters"
		}
		info.Title = kind + " " + mediafile.RenderPlainNum(first.Number) + "–" + mediafile.RenderPlainNum(last.Number)
		return info
	}

	number := mediafile.RenderPlainNum(first.Number)
	completed := series.Status == "completed"
	if first.Ledger == "volume" {
		// Volume only: a Number beside it makes readers that file by
		// ComicInfo (Kavita) list the volume twice, once as their volume
		// chapter and once as the numbered chapter. Merge clears whatever
		// Number the source guessed.
		info.Volume = number
		if completed && series.FinalVolume > 0 {
			info.Count = strconv.Itoa(series.FinalVolume)
		}
	} else {
		info.Number = number
		if completed && series.TotalChapters > 0 {
			info.Count = strconv.Itoa(series.TotalChapters)
		}
	}
	if w := books[first.BookID]; w != nil {
		info.GTIN = w.ISBN
		if y, m, d, ok := splitDate(w.ReleaseDate); ok {
			info.Year, info.Month, info.Day = y, m, d
		}
		if !w.NumberKnown {
			info.Number, info.Volume = "", ""
			info.Title = w.SequenceStr
		}
	}
	seriesRaw, workRaw, _ := cat.payloads(ctx, first.BookID)
	applyCatalog(&info, seriesRaw, workRaw)
	return info
}

// editionFormat is what an edition's name says it is, for ComicInfo's
// Format: "Deluxe", "Box Set", "Full Color"; empty for a standard edition.
func editionFormat(name string) string {
	c := parse.ReadEdition(name)
	if c.Written != "" {
		w := strings.TrimSpace(c.Written)
		if strings.HasSuffix(strings.ToLower(w), " edition") {
			w = strings.TrimSpace(w[:len(w)-len(" edition")])
		}
		return w
	}
	if c.Box {
		return "Box Set"
	}
	return ""
}

// applyCatalog fills what only the raw MangaBaka documents carry: credits,
// genres, tags and rating from the series; subtitle and summary from the
// book. Unreadable or empty documents leave info as it is.
func applyCatalog(info *mediafile.ComicInfo, seriesRaw, workRaw string) {
	var sr struct {
		Authors       []string `json:"authors"`
		Artists       []string `json:"artists"`
		Genres        []string `json:"genres"`
		Tags          []string `json:"tags"`
		ContentRating string   `json:"content_rating"`
	}
	if json.Unmarshal([]byte(seriesRaw), &sr) == nil {
		info.Writer = strings.Join(sr.Authors, ", ")
		info.Penciller = strings.Join(sr.Artists, ", ")
		genres := make([]string, 0, len(sr.Genres))
		for _, g := range sr.Genres {
			genres = append(genres, titleCase(g))
		}
		info.Genre = strings.Join(genres, ", ")
		if len(sr.Tags) > maxTags {
			sr.Tags = sr.Tags[:maxTags]
		}
		info.Tags = strings.Join(sr.Tags, ", ")
		info.AgeRating = map[string]string{
			"suggestive": "Teen", "erotica": "Mature 17+", "pornographic": "Adults Only 18+",
		}[sr.ContentRating]
	}
	var wr struct {
		SubTitle    string `json:"sub_title"`
		Description struct {
			Desc string `json:"desc"`
		} `json:"description"`
	}
	if json.Unmarshal([]byte(workRaw), &wr) == nil {
		if wr.SubTitle != "" {
			info.Title = wr.SubTitle
		}
		if d := strings.TrimSpace(wr.Description.Desc); d != "" {
			info.Summary = d
		}
	}
}

// slotSpan returns the lowest- and highest-numbered of the books a file
// holds.
func slotSpan(slots []store.BookRef) (first, last store.BookRef) {
	first, last = slots[0], slots[0]
	for _, sl := range slots[1:] {
		if sl.Number < first.Number {
			first = sl
		}
		if sl.Number > last.Number {
			last = sl
		}
	}
	return first, last
}

// bookFileName is the templated name for a file holding the given books:
// a single number, or the range it spans ("c001-008") when it holds
// several. ext includes the dot.
func (im *Importer) bookFileName(ctx context.Context, cat *catalogCache, ms MediaSettings, series *store.Series, slots []store.BookRef, group, ext string) string {
	first, last := slotSpan(slots)
	tokens := mediafile.NameTokens{
		SeriesTitle: series.Title, SeriesID: series.MangaBakaID, Group: group,
		Volume: -1, Chapter: -1, Year: series.Year, VersionID: first.VersionID,
	}
	var ver *store.CollectedVersion
	if t, err := cat.version(ctx, first.VersionID); err == nil {
		ver = t
		if !t.IsDefault {
			tokens.Version = t.Name
		}
	}
	if col, err := cat.edition(ctx, first.EditionID); err == nil {
		tokens.Edition, tokens.Lang = col.EditionName, col.Language
		tokens.ReaderSeries = readerValues(ms, series, *col, ver)["series"]
	}
	tmpl := ms.VolumeFile
	if first.Ledger == "volume" {
		tokens.Volume, tokens.VolumeEnd = first.Number, last.Number
	} else {
		tokens.Chapter, tokens.ChapterEnd, tmpl = first.Number, last.Number, ms.ChapterFile
	}
	rendered := mediafile.Render(tmpl, tokens)
	return strings.TrimSuffix(rendered, filepath.Ext(rendered)) + ext
}

// installProcessed writes src to dest as a clean, tagged CBZ. A book with
// this fingerprint already at dest is left as it is.
func installProcessed(src, dest, hash string, info mediafile.ComicInfo) error {
	if _, err := os.Stat(dest); err == nil {
		return verifyDestination(dest, hash)
	}
	// Staged next to dest, then installed exclusively: a path that became
	// occupied in the meantime is never replaced.
	stage := filepath.Join(filepath.Dir(dest), ".sento-stage-"+filepath.Base(dest))
	defer func() { _ = os.Remove(stage) }()
	if err := mediafile.Process(src, stage, &info); err != nil {
		return err
	}
	return installAsset(stage, dest, hash, false)
}

func splitDate(date string) (y, m, d string, ok bool) {
	parts := strings.Split(date, "-")
	if len(parts) != 3 || len(parts[0]) != 4 {
		return "", "", "", false
	}
	month, err1 := strconv.Atoi(parts[1])
	day, err2 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil {
		return "", "", "", false
	}
	return parts[0], strconv.Itoa(month), strconv.Itoa(day), true
}

func titleCase(s string) string {
	words := strings.Fields(strings.ReplaceAll(s, "_", " "))
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// bookDir is where a book of the given edition and version lives: in its
// copy's top folder (the series folder template rendered for it), then
// the configured book folder layout.
func (im *Importer) bookDir(ctx context.Context, cat *catalogCache, ms MediaSettings, seriesDir string, series *store.Series, sl store.BookRef) string {
	if editions, err := cat.editionsAll(ctx); err == nil {
		versions, _ := cat.versionsAll(ctx)
		folders := editionFolders(im.libraryRoot, series, ms, editions, versions)
		if dir, ok := folders[folderKey{sl.EditionID, sl.VersionID}]; ok {
			seriesDir = dir
		} else if dir, ok := folders[folderKey{sl.EditionID, 0}]; ok {
			seriesDir = dir
		}
	}
	tokens := mediafile.NameTokens{
		SeriesTitle: series.Title, SeriesID: series.MangaBakaID, Volume: -1, Chapter: -1,
		Year: series.Year, EditionID: sl.EditionID, VersionID: sl.VersionID,
		Type: "Volumes",
	}
	if sl.Ledger == "chapter" {
		tokens.Type = "Chapters"
	}
	var ver *store.CollectedVersion
	if t, err := cat.version(ctx, sl.VersionID); err == nil {
		ver = t
		if !t.IsDefault {
			tokens.Version = t.Name
		}
	}
	if col, err := cat.edition(ctx, sl.EditionID); err == nil {
		tokens.Edition, tokens.Lang, tokens.Publisher, tokens.Medium = col.EditionName, col.Language, col.Publisher, col.Medium
		tokens.ReaderSeries = readerValues(ms, series, *col, ver)["series"]
	}
	return filepath.Join(append([]string{seriesDir}, mediafile.RenderFolder(ms.BookFolder, tokens)...)...)
}

// placeAsset picks the path for a book with the given fingerprint. The
// fingerprint enters the name when configured, or when a different book
// already holds the plain path: an occupied path is never overwritten.
func placeAsset(dir, name, hash string, hashInName bool) (string, error) {
	if !hashInName {
		plain := assetPath(dir, name, "")
		if verifyDestination(plain, hash) == nil {
			return plain, nil
		}
	}
	dest := assetPath(dir, name, hash)
	return dest, verifyDestination(dest, hash)
}
