package service

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/h-wb/sento/internal/mediafile"
	"github.com/h-wb/sento/internal/store"
)

// placement works out, for one series under the current settings, where a
// book goes, what its file is called and what it says about itself. It is
// built once per pass (an organize run, a pack import): the catalog rows
// come from its cache, and the folder layout and each copy's reader names
// are worked out once, whatever the number of files.
type placement struct {
	root    string // library root
	ms      MediaSettings
	series  *store.Series
	cat     *catalogCache
	folders map[folderKey]string
	readers map[folderKey]map[string]string
}

func (im *Importer) newPlacement(ms MediaSettings, series *store.Series) *placement {
	return &placement{
		root: im.libraryRoot, ms: ms, series: series,
		cat:     newCatalogCache(im.store, series.ID),
		readers: map[folderKey]map[string]string{},
	}
}

// reader is the copy's presented fields, rendered once per copy.
func (p *placement) reader(e store.Edition, v *store.CollectedVersion) map[string]string {
	key := folderKey{EditionID: e.ID}
	if v != nil {
		key.VersionID = v.ID
	}
	values, ok := p.readers[key]
	if !ok {
		values = readerValues(p.ms, p.series, e, v)
		p.readers[key] = values
	}
	return values
}

// tokens are what the paths of a book's copy render with. A book whose
// edition is gone still renders its series and ids.
func (p *placement) tokens(ctx context.Context, sl store.BookRef) mediafile.NameTokens {
	t := seriesTokens(p.series)
	t.Type = "Volumes"
	if sl.Ledger == "chapter" {
		t.Type = "Chapters"
	}
	version, _ := p.cat.version(ctx, sl.VersionID)
	if e, err := p.cat.edition(ctx, sl.EditionID); err == nil {
		t = baseCopyTokens(p.series, *e, version)
		t.ReaderSeries = p.reader(*e, version)["series"]
	} else if version != nil && !version.IsDefault {
		t.Version = version.Name
	}
	t.EditionID, t.VersionID = sl.EditionID, sl.VersionID
	return t
}

// fileName is the templated name for a file holding the given books: a
// single number, or the range it spans ("c001-008") when it holds several.
// ext includes the dot.
func (p *placement) fileName(ctx context.Context, slots []store.BookRef, group, ext string) string {
	first, last := slotSpan(slots)
	tokens := p.tokens(ctx, first)
	tokens.Group = group
	tmpl := p.ms.VolumeFile
	if first.Ledger == "volume" {
		tokens.Volume, tokens.VolumeEnd = first.Number, last.Number
	} else {
		tokens.Chapter, tokens.ChapterEnd, tmpl = first.Number, last.Number, p.ms.ChapterFile
	}
	rendered := mediafile.Render(tmpl, tokens)
	return strings.TrimSuffix(rendered, filepath.Ext(rendered)) + ext
}

// dir is where a book of the given edition and version lives: in its
// copy's top folder (the series folder template rendered for it, seriesDir
// when the series has none of its own), then the configured book folder
// layout.
func (p *placement) dir(ctx context.Context, seriesDir string, sl store.BookRef) string {
	if p.folders == nil {
		p.folders = map[folderKey]string{}
		if editions, err := p.cat.editionsAll(ctx); err == nil {
			versions, _ := p.cat.versionsAll(ctx)
			p.folders = editionFolders(p.root, p.series, p.ms, editions, versions)
		}
	}
	if dir, ok := p.folders[folderKey{sl.EditionID, sl.VersionID}]; ok {
		seriesDir = dir
	} else if dir, ok := p.folders[folderKey{sl.EditionID, 0}]; ok {
		seriesDir = dir
	}
	return filepath.Join(append([]string{seriesDir}, mediafile.RenderFolder(p.ms.BookFolder, p.tokens(ctx, sl))...)...)
}
