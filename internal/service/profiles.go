package service

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/profile"
	"github.com/h-wb/sento/internal/store"
)

// ProfileService owns release profiles and custom formats: validation,
// the instance default, seeding and the portable JSON form.
type ProfileService struct {
	store *store.Store
}

func NewProfiles(st *store.Store) *ProfileService {
	return &ProfileService{store: st}
}

const defaultProfileKey = "default_profile_id"

func validateFormat(f profile.Format) error {
	if strings.TrimSpace(f.Name) == "" {
		return apperr.Invalidf("format needs a name")
	}
	if len(f.Conditions) == 0 {
		return apperr.Invalidf("format needs at least one condition")
	}
	for _, c := range f.Conditions {
		if err := c.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (s *ProfileService) ListFormats(ctx context.Context) ([]profile.Format, error) {
	return s.store.ListFormats(ctx)
}

func (s *ProfileService) CreateFormat(ctx context.Context, f profile.Format) (*profile.Format, error) {
	if err := validateFormat(f); err != nil {
		return nil, err
	}
	id, err := s.store.AddFormat(ctx, f)
	if err != nil {
		return nil, err
	}
	f.ID = id
	return &f, nil
}

func (s *ProfileService) UpdateFormat(ctx context.Context, f profile.Format) error {
	if err := validateFormat(f); err != nil {
		return err
	}
	return s.store.UpdateFormat(ctx, f)
}

// DeleteFormat also drops the format's scores from every profile.
func (s *ProfileService) DeleteFormat(ctx context.Context, id int64) error {
	profiles, err := s.store.ListProfiles(ctx)
	if err != nil {
		return err
	}
	for _, p := range profiles {
		_, scored := p.FormatScores[id]
		required, refused := slices.Contains(p.Required, id), slices.Contains(p.Refused, id)
		if !scored && !required && !refused {
			continue
		}
		delete(p.FormatScores, id)
		p.Required = slices.DeleteFunc(p.Required, func(x int64) bool { return x == id })
		p.Refused = slices.DeleteFunc(p.Refused, func(x int64) bool { return x == id })
		if err := s.store.UpdateProfile(ctx, p); err != nil {
			return err
		}
	}
	return s.store.DeleteFormat(ctx, id)
}

func (s *ProfileService) validateProfile(ctx context.Context, p profile.Profile) error {
	if p.MinUpgradeScore < 0 {
		return apperr.Invalidf("minimum upgrade improvement cannot be negative")
	}
	if strings.TrimSpace(p.Name) == "" {
		return apperr.Invalidf("profile needs a name")
	}
	formats, err := s.store.ListFormats(ctx)
	if err != nil {
		return err
	}
	known := map[int64]bool{}
	for _, f := range formats {
		known[f.ID] = true
	}
	for id := range p.FormatScores {
		if !known[id] {
			return apperr.Invalidf("profile scores unknown format %d", id)
		}
	}
	for _, id := range append(slices.Clone(p.Required), p.Refused...) {
		if !known[id] {
			return apperr.Invalidf("profile requires or refuses unknown format %d", id)
		}
	}
	for _, id := range p.Required {
		if slices.Contains(p.Refused, id) {
			return apperr.Invalidf("a format cannot be both required and refused")
		}
	}
	return nil
}

func (s *ProfileService) ListProfiles(ctx context.Context) ([]profile.Profile, error) {
	profiles, err := s.store.ListProfiles(ctx)
	for i := range profiles {
		for _, o := range profiles {
			if profile.Apart(profiles[i], o) {
				profiles[i].ApartFrom = append(profiles[i].ApartFrom, o.ID)
			}
		}
	}
	return profiles, err
}

func (s *ProfileService) CreateProfile(ctx context.Context, p profile.Profile) (*profile.Profile, error) {
	if err := s.validateProfile(ctx, p); err != nil {
		return nil, err
	}
	id, err := s.store.AddProfile(ctx, p)
	if err != nil {
		return nil, err
	}
	p.ID = id
	return &p, nil
}

func (s *ProfileService) UpdateProfile(ctx context.Context, p profile.Profile) error {
	if err := s.validateProfile(ctx, p); err != nil {
		return err
	}
	return s.store.UpdateProfile(ctx, p)
}

// DeleteProfile refuses while series or editions use the profile or it
// is the default.
func (s *ProfileService) DeleteProfile(ctx context.Context, id int64) error {
	if did, _ := s.DefaultProfileID(ctx); did == id {
		return apperr.Conflictf("cannot delete the default profile")
	}
	n, err := s.store.CountProfileUses(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return apperr.Conflictf("this profile judges %d series or cards; give them another first", n)
	}
	return s.store.DeleteProfile(ctx, id)
}

// DefaultProfileID resolves the instance default profile.
func (s *ProfileService) DefaultProfileID(ctx context.Context) (int64, error) {
	raw, err := s.store.GetSetting(ctx, defaultProfileKey)
	if err == nil {
		var id int64
		if json.Unmarshal(raw, &id) == nil && id > 0 {
			return id, nil
		}
	}
	profiles, err := s.store.ListProfiles(ctx)
	if err != nil || len(profiles) == 0 {
		return 0, err
	}
	return profiles[0].ID, nil
}

func (s *ProfileService) SetDefaultProfile(ctx context.Context, id int64) error {
	if _, err := s.store.GetProfile(ctx, id); err != nil {
		return err
	}
	raw, _ := json.Marshal(id)
	return s.store.PutSetting(ctx, defaultProfileKey, raw)
}

// CardProfiles says which release profile judges for each card of a
// series: the card's own, else the series', else the instance default.
// With no profile configured everything is nil, and built-in rules apply.
type CardProfiles struct {
	Series  *profile.Profile
	ByID    map[int64]*profile.Profile
	Formats []profile.Format
}

// For is the profile that judges releases and files for one card.
func (p CardProfiles) For(v store.CollectedVersion) *profile.Profile {
	if own := p.ByID[v.ProfileID]; own != nil {
		return own
	}
	return p.Series
}

// SeriesProfiles loads what CardProfiles needs for one series.
func (s *ProfileService) SeriesProfiles(ctx context.Context, seriesID int64) (CardProfiles, error) {
	series, err := s.store.GetSeries(ctx, seriesID)
	if err != nil {
		return CardProfiles{}, err
	}
	all, err := s.store.ListProfiles(ctx)
	if err != nil || len(all) == 0 {
		return CardProfiles{}, err
	}
	out := CardProfiles{ByID: map[int64]*profile.Profile{}}
	for i := range all {
		out.ByID[all[i].ID] = &all[i]
	}
	out.Series = out.ByID[series.ProfileID]
	if out.Series == nil { // unset or stale assignment → instance default
		did, err := s.DefaultProfileID(ctx)
		if err != nil {
			return CardProfiles{}, err
		}
		out.Series = out.ByID[did]
	}
	out.Formats, err = s.store.ListFormats(ctx)
	return out, err
}

// --- portable JSON form (recyclarr-style) ---

// ProfileDoc is the self-contained export of one profile: format
// definitions travel with their scores, so a doc imports anywhere.
type ProfileDoc struct {
	UpgradesEnabled   bool                `json:"upgradesEnabled"`
	UpgradeUntilScore int                 `json:"upgradeUntilScore"`
	MinUpgradeScore   int                 `json:"minUpgradeScore"`
	Name              string              `json:"name"`
	MinFormatScore    int                 `json:"minFormatScore"`
	Volume            profile.LedgerRules `json:"volume"`
	Chapter           profile.LedgerRules `json:"chapter"`
	Formats           []FormatDoc         `json:"formats"`
}

type FormatDoc struct {
	Name       string              `json:"name"`
	Score      int                 `json:"score"`
	Conditions []profile.Condition `json:"conditions"`
	// Rule is "required" or "refused" when the profile holds releases to it.
	Rule string `json:"rule,omitempty"`
}

func (s *ProfileService) ExportProfile(ctx context.Context, id int64) (*ProfileDoc, error) {
	p, err := s.store.GetProfile(ctx, id)
	if err != nil {
		return nil, err
	}
	formats, err := s.store.ListFormats(ctx)
	if err != nil {
		return nil, err
	}
	doc := &ProfileDoc{
		UpgradesEnabled: p.UpgradesEnabled, UpgradeUntilScore: p.UpgradeUntilScore, MinUpgradeScore: p.MinUpgradeScore,
		Name: p.Name, MinFormatScore: p.MinFormatScore,
		Volume: p.Volume, Chapter: p.Chapter, Formats: []FormatDoc{},
	}
	for _, f := range formats {
		score, scored := p.FormatScores[f.ID]
		rule := ""
		switch {
		case slices.Contains(p.Required, f.ID):
			rule = "required"
		case slices.Contains(p.Refused, f.ID):
			rule = "refused"
		}
		if !scored && rule == "" {
			continue // only the formats the profile cares about travel
		}
		doc.Formats = append(doc.Formats, FormatDoc{Name: f.Name, Score: score, Conditions: f.Conditions, Rule: rule})
	}
	return doc, nil
}

// ImportProfile upserts the doc's formats by name, then creates or
// replaces the profile by name.
func (s *ProfileService) ImportProfile(ctx context.Context, doc ProfileDoc) (*profile.Profile, error) {
	if strings.TrimSpace(doc.Name) == "" {
		return nil, apperr.Invalidf("profile doc needs a name")
	}
	for _, fd := range doc.Formats {
		if fd.Rule != "" && fd.Rule != "required" && fd.Rule != "refused" {
			return nil, apperr.Invalidf("unknown format rule %q", fd.Rule)
		}
		if err := validateFormat(profile.Format{Name: fd.Name, Conditions: fd.Conditions}); err != nil {
			return nil, apperr.Invalidf("format %q: %w", fd.Name, err)
		}
	}
	existing, err := s.store.ListFormats(ctx)
	if err != nil {
		return nil, err
	}
	byName := map[string]profile.Format{}
	for _, f := range existing {
		byName[strings.ToLower(f.Name)] = f
	}
	scores := map[int64]int{}
	var required, refused []int64
	for _, fd := range doc.Formats {
		f, ok := byName[strings.ToLower(fd.Name)]
		if ok {
			f.Conditions = fd.Conditions
			if err := s.store.UpdateFormat(ctx, f); err != nil {
				return nil, err
			}
		} else {
			f = profile.Format{Name: fd.Name, Conditions: fd.Conditions}
			if f.ID, err = s.store.AddFormat(ctx, f); err != nil {
				return nil, err
			}
		}
		scores[f.ID] = fd.Score
		switch fd.Rule {
		case "required":
			required = append(required, f.ID)
		case "refused":
			refused = append(refused, f.ID)
		}
	}

	p := profile.Profile{
		UpgradesEnabled: doc.UpgradesEnabled, UpgradeUntilScore: doc.UpgradeUntilScore, MinUpgradeScore: doc.MinUpgradeScore,
		Name: doc.Name, MinFormatScore: doc.MinFormatScore,
		FormatScores: scores, Required: required, Refused: refused, Volume: doc.Volume, Chapter: doc.Chapter,
	}
	if p.Volume == (profile.LedgerRules{}) && p.Chapter == (profile.LedgerRules{}) {
		p.Volume, p.Chapter = profile.DefaultRules()
	}
	profiles, err := s.store.ListProfiles(ctx)
	if err != nil {
		return nil, err
	}
	for _, ex := range profiles {
		if strings.EqualFold(ex.Name, doc.Name) {
			p.ID = ex.ID
			return &p, s.store.UpdateProfile(ctx, p)
		}
	}
	id, err := s.store.AddProfile(ctx, p)
	if err != nil {
		return nil, err
	}
	p.ID = id
	return &p, nil
}

// --- shipped defaults ---

// EnsureDefaults seeds the shipped formats and the Standard profile on a
// fresh instance (both tables empty). Like recyclarr's templates: a good
// starting point the user owns from then on.
func (s *ProfileService) EnsureDefaults(ctx context.Context) error {
	profiles, err := s.store.ListProfiles(ctx)
	if err != nil {
		return err
	}
	formats, err := s.store.ListFormats(ctx)
	if err != nil {
		return err
	}
	if len(profiles) > 0 || len(formats) > 0 {
		return nil
	}
	vol, ch := profile.DefaultRules()
	doc := ProfileDoc{
		Name:           "Standard",
		MinFormatScore: profile.MinScoreOff,
		Volume:         vol,
		Chapter:        ch,
		Formats:        defaultFormats,
	}
	p, err := s.ImportProfile(ctx, doc)
	if err != nil {
		return err
	}
	return s.SetDefaultProfile(ctx, p.ID)
}

// defaultFormats are the shipped scored terms (decision-engine.md §4) plus
// the manga equivalents of the TRaSH-guide unwanted formats.
var defaultFormats = []FormatDoc{
	{Name: "Digital", Score: 100, Conditions: []profile.Condition{{Type: profile.CondFlag, Value: "digital"}}},
	{Name: "Premium", Score: 50, Conditions: []profile.Condition{{Type: profile.CondFlag, Value: "premium"}}},
	{Name: "Official", Score: 50, Conditions: []profile.Condition{{Type: profile.CondFlag, Value: "official"}}},
	{Name: "High Resolution", Score: 25, Conditions: []profile.Condition{{Type: profile.CondResolution, Value: "1600"}}},
	{Name: "Colored", Score: 0, Conditions: []profile.Condition{{Type: profile.CondFlag, Value: "colored"}}},
	{Name: "Low Quality", Score: -100, Conditions: []profile.Condition{{Type: profile.CondTitle, Value: `\b(lq|low[ ._-]?quality)\b`}}},
	{Name: "Compressed", Score: -50, Conditions: []profile.Condition{{Type: profile.CondTitle, Value: `\b(re)?compress(ed)?\b`}}},
	{Name: "Watermarked", Score: -50, Conditions: []profile.Condition{{Type: profile.CondTitle, Value: `watermark`}}},
	{Name: "PDF", Score: -50, Conditions: []profile.Condition{{Type: profile.CondTitle, Value: `\bpdfs?\b`}}},
	{Name: "EPUB", Score: -25, Conditions: []profile.Condition{{Type: profile.CondTitle, Value: `\bepubs?\b`}}},
	{Name: "Upscaled", Score: -25, Conditions: []profile.Condition{{Type: profile.CondTitle, Value: `\bupscaled?\b`}}},
	{Name: "Raw (untranslated)", Score: -1000, Conditions: []profile.Condition{{Type: profile.CondFlag, Value: "raw"}}},
}
