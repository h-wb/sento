package service

import (
	"context"
	"maps"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/profile"
	"github.com/h-wb/sento/internal/store"
)

// FileScore evaluates stored file evidence under a collected version's current
// effective profile, including any confirmed edition context.
type FileScore struct {
	ProfileID   int64       `json:"profileId"`
	ProfileName string      `json:"profileName"`
	Score       int         `json:"score"`
	Formats     []FormatHit `json:"formats"`
	FactsKnown  bool        `json:"factsKnown"`
	Reason      string      `json:"reason"`
}

func judgeMappedFile(col store.Edition, prof *profile.Profile, formats []profile.Format, file store.File, mapping store.AssetCoverage) FileScore {
	facts := backedFacts(file.Facts, file.Claims)
	out := FileScore{FactsKnown: facts.FactsKnown, Formats: []FormatHit{}}
	if prof != nil {
		out.ProfileID, out.ProfileName = prof.ID, prof.Name
	}
	if mapping.Evidence != "manual" && facts.Language != "" && facts.Language != "multi" && profile.BaseLang(facts.Language) != profile.BaseLang(col.Language) {
		out.Reason = "The file on disk is in another language"
	}
	if mapping.Evidence == "manual" {
		facts.Language = col.Language
		// Confirming a color edition supplies evidence a bare filename lacks.
		if parse.ReadEdition(col.EditionName).Color {
			facts.Flags = maps.Clone(facts.Flags)
			if facts.Flags == nil {
				facts.Flags = map[string]bool{}
			}
			facts.Flags["colored"] = true
		}
	}
	judgement := assetJudgement(facts, prof, formats)
	out.Score = judgement.Score
	for _, f := range judgement.Matched {
		out.Formats = append(out.Formats, FormatHit{Name: f.Name, Score: prof.FormatScores[f.ID]})
	}
	if judgement.Broken != "" {
		out.Reason = "The file on disk " + judgement.Broken
	}
	if prof != nil && prof.MinFormatScore > profile.MinScoreOff && facts.FactsKnown && judgement.Score < prof.MinFormatScore {
		out.Reason = "The file on disk is below the profile’s minimum score"
	}
	return out
}

func (l *Library) FileScore(ctx context.Context, sid, vid, fid int64) (*FileScore, error) {
	version, err := l.store.GetVersion(ctx, vid)
	if err != nil {
		return nil, err
	}
	col, err := l.store.GetEdition(ctx, version.EditionID)
	if err != nil {
		return nil, err
	}
	if col.SeriesID != sid {
		return nil, apperr.NotFoundf("card not found")
	}
	files, err := l.store.ListFiles(ctx, sid)
	if err != nil {
		return nil, err
	}
	var file *store.File
	for i := range files {
		if files[i].ID == fid {
			file = &files[i]
			break
		}
	}
	if file == nil {
		return nil, apperr.NotFoundf("file not found")
	}
	profiles, err := NewProfiles(l.store).seriesProfiles(ctx, sid)
	if err != nil {
		return nil, err
	}
	coverage, err := l.store.Coverage(ctx, sid)
	if err != nil {
		return nil, err
	}
	// An unmapped file can be inspected for the version selected in the editor.
	mapping := store.AssetCoverage{}
	for _, a := range coverage {
		if a.VersionID == vid && a.FileID == fid {
			mapping = a
		}
	}
	out := judgeMappedFile(*col, profiles.For(*version), profiles.Formats, *file, mapping)
	return &out, nil
}

// backedFacts is what a file is judged on: its own facts, with the
// release's claims backing the fields the facts do not carry. Organizing
// and importing name a file after its books ("Volume 1.cbz"), which drops
// the digital/group/resolution evidence the release itself carried; the
// claims are that evidence, so a good copy is not forgotten. A field the
// facts do carry always wins over the offer, and a file without claims
// (a folder scan) keeps scoring from its own name alone.
func backedFacts(facts, claims store.AssetFacts) store.AssetFacts {
	if !claims.FactsKnown && claims.Title == "" {
		return facts
	}
	if facts.Group == "" && facts.Sources["group"] != "manual" {
		facts.Group = claims.Group
	}
	if facts.Language == "" && facts.Sources["language"] != "manual" {
		facts.Language = claims.Language
	}
	if facts.Site == "" {
		facts.Site = claims.Site
	}
	if facts.Protocol == "" {
		facts.Protocol = claims.Protocol
	}
	if facts.Resolution == 0 {
		facts.Resolution = claims.Resolution
	}
	// A flag a person turned off stays off, whatever the release claimed.
	if claims.Digital && !facts.Digital && !facts.ManualFlag("digital") {
		facts.Digital = true
	}
	var backed []string
	for flag, set := range claims.Flags {
		if set && !facts.Flags[flag] && !facts.ManualFlag(flag) {
			backed = append(backed, flag)
		}
	}
	if len(backed) > 0 {
		flags := maps.Clone(facts.Flags)
		if flags == nil {
			flags = map[string]bool{}
		}
		for _, flag := range backed {
			flags[flag] = true
		}
		facts.Flags = flags
	}
	facts.FactsKnown = facts.FactsKnown || claims.FactsKnown
	return facts
}

func assetJudgement(f store.AssetFacts, p *profile.Profile, formats []profile.Format) profile.Judgement {
	return p.Judge(profile.Input{Title: f.Title, Group: f.Group, Language: f.Language, Protocol: f.Protocol, Site: f.Site, Resolution: f.Resolution, Flags: f.Flags}, formats)
}
