package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/h-wb/sento/internal/mediafile"
	"github.com/h-wb/sento/internal/store"
)

// maxTags bounds the Tags field: MangaBaka can carry well over a hundred
// community tags per series, most of them noise in a reader's filter list.
const maxTags = 30

// bookInfo builds the ComicInfo for one book of an edition: everything a
// reader needs to file and describe the file without Sento. release is
// the name of the download it came from and group its release group; both
// may be empty.
func (im *Importer) bookInfo(ctx context.Context, series *store.Series, sl store.BookRef, release, group string) mediafile.ComicInfo {
	info := mediafile.ComicInfo{
		Series:          series.Title,
		Summary:         series.Description,
		Web:             fmt.Sprintf("https://mangabaka.org/%d", series.MangaBakaID),
		ScanInformation: group,
	}
	if series.Year > 0 {
		info.Year = strconv.Itoa(series.Year)
	}
	if series.Type == "manga" {
		info.Manga = "YesAndRightToLeft"
	}
	notes := []string{"sento:v1", fmt.Sprintf("mangabaka:%d", series.MangaBakaID)}

	col, _ := im.store.GetCollection(ctx, sl.CollectionID)
	if col != nil {
		info.LanguageISO = col.Language
		info.Publisher = col.Publisher
		var traits []string
		for _, t := range col.EditionTraits {
			if t != "standard" {
				traits = append(traits, titleCase(t))
			}
		}
		if col.ContentVariant == "colored" {
			traits = append(traits, "Colored")
		}
		info.Format = strings.Join(traits, ", ")
		notes = append(notes, "edition:"+strconv.Quote(col.EditionName))
	}

	number := mediafile.RenderPlainNum(sl.Number)
	completed := series.Status == "completed"
	if sl.Ledger == "volume" {
		// Number too: a merge would otherwise keep whatever the source's
		// own ComicInfo guessed (often a year mis-parse).
		info.Volume, info.Number = number, number
		if completed && series.FinalVolume > 0 {
			info.Count = strconv.Itoa(series.FinalVolume)
		}
	} else {
		info.Number = number
		if completed && series.TotalChapters > 0 {
			info.Count = strconv.Itoa(series.TotalChapters)
		}
	}

	if w, err := im.store.GetBook(ctx, sl.BookID); err == nil {
		info.GTIN = w.ISBN
		if y, m, d, ok := splitDate(w.ReleaseDate); ok {
			info.Year, info.Month, info.Day = y, m, d
		}
		if w.ProviderID != "" {
			notes = append(notes, "work:"+w.ProviderID)
		}
		if !w.NumberKnown {
			info.Number, info.Volume = "", ""
			info.Title = w.SequenceStr
		}
	}

	seriesRaw, workRaw, _ := im.store.CatalogPayloads(ctx, series.ID, sl.BookID)
	applyCatalog(&info, seriesRaw, workRaw)

	if release != "" {
		notes = append(notes, "release:"+strconv.Quote(release))
	}
	info.Notes = strings.Join(notes, " ")
	return info
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

// packInfo is bookInfo for an archive holding several books: series-level
// facts only, since no single number, date or ISBN describes it.
func (im *Importer) packInfo(ctx context.Context, series *store.Series, slots []store.BookRef, release, group string) mediafile.ComicInfo {
	info := im.bookInfo(ctx, series, store.BookRef{Ledger: slots[0].Ledger, CollectionID: slots[0].CollectionID}, release, group)
	info.Number, info.Volume, info.Count = "", "", ""
	info.Summary = series.Description
	first, last := slotSpan(slots)
	kind := "Volumes"
	if first.Ledger == "chapter" {
		kind = "Chapters"
	}
	info.Title = kind + " " + mediafile.RenderPlainNum(first.Number) + "–" + mediafile.RenderPlainNum(last.Number)
	return info
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
func (im *Importer) bookFileName(ctx context.Context, ms MediaSettings, series *store.Series, slots []store.BookRef, group, ext string) string {
	first, last := slotSpan(slots)
	tokens := mediafile.NameTokens{
		SeriesTitle: series.Title, SeriesID: series.MangaBakaID, Group: group,
		Volume: -1, Chapter: -1, Year: series.Year,
	}
	if col, err := im.store.GetCollection(ctx, first.CollectionID); err == nil {
		tokens.Edition, tokens.Lang = col.EditionName, col.Language
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

// releaseGroup is the release group recorded with a queued download.
func releaseGroup(item store.QueueItem) string {
	var snap ReleaseCandidate
	_ = json.Unmarshal(item.Snapshot, &snap)
	if snap.Parse.Group != "" {
		return snap.Parse.Group
	}
	return snap.Scanlator
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

// bookDir is where a book of the given edition and version lives inside
// the series folder, per the configured layout.
func (im *Importer) bookDir(ctx context.Context, ms MediaSettings, seriesDir string, series *store.Series, sl store.BookRef) string {
	tokens := mediafile.NameTokens{
		SeriesTitle: series.Title, SeriesID: series.MangaBakaID, Volume: -1, Chapter: -1,
		Year: series.Year, CollectionID: sl.CollectionID, VersionID: sl.TargetID,
		Type: "Volumes",
	}
	if sl.Ledger == "chapter" {
		tokens.Type = "Chapters"
	}
	if col, err := im.store.GetCollection(ctx, sl.CollectionID); err == nil {
		tokens.Edition, tokens.Lang, tokens.Publisher, tokens.Medium = col.EditionName, col.Language, col.Publisher, col.Medium
	}
	if t, err := im.store.GetTarget(ctx, sl.TargetID); err == nil && !t.IsDefault {
		tokens.Version = t.Name
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
