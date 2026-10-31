package service

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/match"
	"github.com/h-wb/sento/internal/mediafile"
	"github.com/h-wb/sento/internal/metadata"
)

// ImportProposal is one library subfolder matched against MangaBaka
// (D14/§11.4: bulk import proposes, the user confirms).
type ImportProposal struct {
	Dir        string               `json:"dir"`  // absolute path
	Name       string               `json:"name"` // cleaned folder name used for lookup
	Candidates []metadata.SeriesRef `json:"candidates,omitempty"`
	BestID     int64                `json:"bestId,omitempty"`
	Confidence float64              `json:"confidence"`
	ExistingID int64                `json:"existingSeriesId,omitempty"` // already in the library
}

var (
	reMangaBakaHint = regexp.MustCompile(`\[mangabaka-(\d+)\]`)
	reBracketGroup  = regexp.MustCompile(`\[[^\]]*\]|\([^)]*\)|\{[^}]*\}`)
)

// ProposeImports scans root's subfolders and matches each against
// MangaBaka. Read-only: nothing is added until the user confirms.
func (l *Library) ProposeImports(ctx context.Context, root string) ([]ImportProposal, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, apperr.Invalidf("reading import root: %w", err)
	}
	known := map[string]int64{} // series path → id
	if series, err := l.store.ListSeries(ctx); err == nil {
		for _, s := range series {
			if s.Path != "" {
				known[filepath.Clean(s.Path)] = s.ID
			}
		}
	}

	proposals := []ImportProposal{}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		dir := filepath.Join(root, e.Name())
		p := ImportProposal{Dir: dir, Name: cleanFolderName(e.Name())}
		if id, ok := known[filepath.Clean(dir)]; ok {
			p.ExistingID = id
			proposals = append(proposals, p)
			continue
		}

		// Files Sento tagged name their series: re-adoption after a
		// reinstall is exact (§11.4, step 1).
		if id, title := taggedSeries(dir); id > 0 {
			p.BestID, p.Confidence = id, 1
			p.Candidates = []metadata.SeriesRef{{MangaBakaID: id, Title: title}}
			proposals = append(proposals, p)
			continue
		}

		// Our own naming embeds the id — definitive match, no lookup.
		if m := reMangaBakaHint.FindStringSubmatch(e.Name()); m != nil {
			id, _ := strconv.ParseInt(m[1], 10, 64)
			p.BestID, p.Confidence = id, 1
			proposals = append(proposals, p)
			continue
		}

		refs, err := l.provider.Search(ctx, p.Name)
		if err != nil {
			proposals = append(proposals, p) // unmatched, confidence 0
			continue
		}
		if len(refs) > 5 {
			refs = refs[:5]
		}
		p.Candidates = refs
		for _, ref := range refs {
			if sc := match.Similarity(match.Normalize(p.Name), match.Normalize(ref.Title)); sc > p.Confidence {
				p.Confidence, p.BestID = sc, ref.MangaBakaID
			}
		}
		proposals = append(proposals, p)
	}
	return proposals, nil
}

// taggedSeries is the MangaBaka series the archives Sento tagged in dir
// name, and its title as they write it. It reads a handful of archives;
// 0 when none is tagged, or when they disagree (a folder holding two
// series is the user's to sort out).
func taggedSeries(dir string) (id int64, title string) {
	const enough, budget = 3, 20
	agree, read := 0, 0
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.EqualFold(filepath.Ext(path), ".cbz") {
			return nil
		}
		if read++; read > budget {
			return fs.SkipAll
		}
		ci := mediafile.ReadComicInfo(path)
		if ci == nil {
			return nil
		}
		p, ok := mediafile.ReadProvenance(ci.Notes)
		switch {
		case !ok || p.MangaBaka <= 0:
			return nil
		case id != 0 && p.MangaBaka != id:
			id, title = 0, ""
			return fs.SkipAll
		}
		id, title = p.MangaBaka, ci.Series
		if agree++; agree >= enough {
			return fs.SkipAll
		}
		return nil
	})
	return id, title
}

// cleanFolderName strips release-style noise from a folder name before
// the metadata lookup: bracketed groups, separators, trailing junk.
func cleanFolderName(name string) string {
	name = reBracketGroup.ReplaceAllString(name, " ")
	name = strings.NewReplacer("_", " ", ".", " ").Replace(name)
	return strings.Join(strings.Fields(name), " ")
}

// ImportFolder adds a series bound to an existing folder: metadata in,
// path set, files linked in place (non-destructive). A series already in
// the library keeps its own folder and settings; the folder's files are
// linked to it in place, as a library is built up over time. existing
// says which happened.
func (l *Library) ImportFolder(ctx context.Context, scanner *Scanner, dir string, mangabakaID int64) (id int64, result *ScanResult, existing bool, err error) {
	all, err := l.store.ListSeries(ctx)
	if err != nil {
		return 0, nil, false, err
	}
	for _, s := range all {
		if s.MangaBakaID == mangabakaID {
			result, err = scanner.LinkFolder(ctx, s.ID, dir)
			return s.ID, result, true, err
		}
	}
	id, err = l.AddSeries(ctx, AddSeriesRequest{MangaBakaID: mangabakaID, Monitor: "all"})
	if err != nil {
		return 0, nil, false, err
	}
	if err := l.store.SetSeriesPath(ctx, id, dir); err != nil {
		return id, nil, false, err
	}
	if result, err = scanner.ScanSeriesFolder(ctx, id, dir); err != nil {
		return id, nil, false, err
	}
	return id, result, false, nil
}
