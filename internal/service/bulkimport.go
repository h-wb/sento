package service

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/h-wb/sento/internal/match"
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
		return nil, err
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

// cleanFolderName strips release-style noise from a folder name before
// the metadata lookup: bracketed groups, separators, trailing junk.
func cleanFolderName(name string) string {
	name = reBracketGroup.ReplaceAllString(name, " ")
	name = strings.NewReplacer("_", " ", ".", " ").Replace(name)
	return strings.Join(strings.Fields(name), " ")
}

// ImportFolder adds a series bound to an existing folder: metadata in,
// path set, files linked in place (non-destructive).
func (l *Library) ImportFolder(ctx context.Context, scanner *Scanner, dir string, mangabakaID int64) (int64, *ScanResult, error) {
	id, err := l.AddSeries(ctx, AddSeriesRequest{MangaBakaID: mangabakaID, MonitorAll: true})
	if err != nil {
		return 0, nil, err
	}
	if err := l.store.SetSeriesPath(ctx, id, dir); err != nil {
		return id, nil, err
	}
	result, err := scanner.ScanSeriesFolder(ctx, id, dir)
	if err != nil {
		return id, nil, err
	}
	return id, result, nil
}
