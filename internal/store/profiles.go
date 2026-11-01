package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/h-wb/sento/internal/profile"
)

// --- custom formats ---

type formatRow struct {
	ID         int64
	Name       string
	Conditions string `db:"conditions_json"`
}

func (r formatRow) decode() (profile.Format, error) {
	f := profile.Format{ID: r.ID, Name: r.Name}
	if err := json.Unmarshal([]byte(r.Conditions), &f.Conditions); err != nil {
		return f, fmt.Errorf("format %d: %w", r.ID, err)
	}
	return f, nil
}

func (s *Store) ListFormats(ctx context.Context) ([]profile.Format, error) {
	rows, err := list[formatRow](ctx, s,
		`SELECT id, name, conditions_json FROM custom_formats ORDER BY name`)
	if err != nil {
		return nil, err
	}
	out := make([]profile.Format, 0, len(rows))
	for _, r := range rows {
		f, err := r.decode()
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

func (s *Store) AddFormat(ctx context.Context, f profile.Format) (int64, error) {
	conds, err := json.Marshal(f.Conditions)
	if err != nil {
		return 0, err
	}
	id, err := s.insert(ctx,
		`INSERT INTO custom_formats (name, conditions_json) VALUES (?,?) RETURNING id`,
		f.Name, string(conds))
	return id, conflict(err, "a custom format with this name already exists")
}

func (s *Store) UpdateFormat(ctx context.Context, f profile.Format) error {
	conds, err := json.Marshal(f.Conditions)
	if err != nil {
		return err
	}
	return conflict(s.touch(ctx,
		`UPDATE custom_formats SET name = ?, conditions_json = ? WHERE id = ?`,
		f.Name, string(conds), f.ID), "a custom format with this name already exists")
}

func (s *Store) DeleteFormat(ctx context.Context, id int64) error {
	return s.touch(ctx, `DELETE FROM custom_formats WHERE id = ?`, id)
}

// --- profiles ---

// profileBody is everything but id/name, stored as the row's body_json.
type profileBody struct {
	UpgradesEnabled   bool                `json:"upgradesEnabled"`
	UpgradeUntilScore int                 `json:"upgradeUntilScore"`
	MinUpgradeScore   int                 `json:"minUpgradeScore"`
	MinFormatScore    int                 `json:"minFormatScore"`
	FormatScores      map[int64]int       `json:"formatScores"`
	Required          []int64             `json:"required,omitempty"`
	Refused           []int64             `json:"refused,omitempty"`
	Volume            profile.LedgerRules `json:"volume"`
	Chapter           profile.LedgerRules `json:"chapter"`
}

type profileRow struct {
	ID   int64
	Name string
	Body string `db:"body_json"`
}

func (r profileRow) decode() (profile.Profile, error) {
	var body profileBody
	if err := json.Unmarshal([]byte(r.Body), &body); err != nil {
		return profile.Profile{}, fmt.Errorf("profile %d: %w", r.ID, err)
	}
	if body.FormatScores == nil {
		body.FormatScores = map[int64]int{}
	}
	return profile.Profile{
		ID: r.ID, Name: r.Name,
		UpgradesEnabled: body.UpgradesEnabled, UpgradeUntilScore: body.UpgradeUntilScore, MinUpgradeScore: body.MinUpgradeScore,
		MinFormatScore: body.MinFormatScore, FormatScores: body.FormatScores,
		Required: body.Required, Refused: body.Refused,
		Volume: body.Volume, Chapter: body.Chapter,
	}, nil
}

func encodeProfileBody(p profile.Profile) (string, error) {
	body, err := json.Marshal(profileBody{
		UpgradesEnabled: p.UpgradesEnabled, UpgradeUntilScore: p.UpgradeUntilScore, MinUpgradeScore: p.MinUpgradeScore,
		MinFormatScore: p.MinFormatScore, FormatScores: p.FormatScores,
		Required: p.Required, Refused: p.Refused,
		Volume: p.Volume, Chapter: p.Chapter,
	})
	return string(body), err
}

func (s *Store) ListProfiles(ctx context.Context) ([]profile.Profile, error) {
	rows, err := list[profileRow](ctx, s, `SELECT id, name, body_json FROM profiles ORDER BY name`)
	if err != nil {
		return nil, err
	}
	out := make([]profile.Profile, 0, len(rows))
	for _, r := range rows {
		p, err := r.decode()
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *Store) GetProfile(ctx context.Context, id int64) (*profile.Profile, error) {
	r, err := get[profileRow](ctx, s, `SELECT id, name, body_json FROM profiles WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	p, err := r.decode()
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) AddProfile(ctx context.Context, p profile.Profile) (int64, error) {
	body, err := encodeProfileBody(p)
	if err != nil {
		return 0, err
	}
	id, err := s.insert(ctx,
		`INSERT INTO profiles (name, body_json) VALUES (?,?) RETURNING id`, p.Name, body)
	return id, conflict(err, "a profile with this name already exists")
}

func (s *Store) UpdateProfile(ctx context.Context, p profile.Profile) error {
	body, err := encodeProfileBody(p)
	if err != nil {
		return err
	}
	return conflict(
		s.touch(ctx, `UPDATE profiles SET name = ?, body_json = ? WHERE id = ?`, p.Name, body, p.ID),
		"a profile with this name already exists")
}

func (s *Store) DeleteProfile(ctx context.Context, id int64) error {
	return s.touch(ctx, `DELETE FROM profiles WHERE id = ?`, id)
}

// CountProfileUses guards profile deletion: the series and cards that
// name it (editions' own column is legacy, cleared by migration 24).
func (s *Store) CountProfileUses(ctx context.Context, id int64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, s.q(`
		SELECT (SELECT COUNT(*) FROM series WHERE profile_id = ?)
		     + (SELECT COUNT(*) FROM editions WHERE profile_id = ?) + (SELECT COUNT(*) FROM versions WHERE profile_id=?)`), id, id, id).Scan(&n)
	return n, err
}

// SetSeriesProfile assigns the series' release profile (0 = back to the
// instance default, stored NULL).
func (s *Store) SetSeriesProfile(ctx context.Context, seriesID, profileID int64) error {
	var v any
	if profileID > 0 {
		v = profileID
	}
	return s.touch(ctx, `UPDATE series SET profile_id = ? WHERE id = ?`, v, seriesID)
}
