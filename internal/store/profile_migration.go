package store

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"slices"

	"github.com/h-wb/sento/internal/profile"
)

// MigrateCopyRules moves legacy copy restrictions into reusable profiles.
// All writes commit together; clearing the old fields makes reruns a no-op.
// Run before serving requests or starting acquisition workers.
func (s *Store) MigrateCopyRules(ctx context.Context) error {
	versions, err := list[CollectedVersion](ctx, s, `SELECT `+versionColumns+` FROM versions WHERE required_group<>'' OR allow_unknown=FALSE ORDER BY id`)
	if err != nil || len(versions) == 0 {
		return err
	}
	profiles, err := s.ListProfiles(ctx)
	if err != nil {
		return err
	}
	formats, err := s.ListFormats(ctx)
	if err != nil {
		return err
	}
	var defaultID int64
	if raw, e := s.GetSetting(ctx, "default_profile_id"); e == nil {
		_ = json.Unmarshal(raw, &defaultID)
	}
	if defaultID == 0 && len(profiles) > 0 {
		defaultID = profiles[0].ID
	}
	// Resolve inheritance before opening the transaction (SQLite has one connection).
	bases := map[int64]int64{}
	for _, v := range versions {
		edition, e := s.GetEdition(ctx, v.EditionID)
		if e != nil {
			return e
		}
		series, e := s.GetSeries(ctx, edition.SeriesID)
		if e != nil {
			return e
		}
		id := v.ProfileID
		if id == 0 {
			id = series.ProfileID
		}
		if id == 0 {
			id = defaultID
		}
		bases[v.ID] = id
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	ensureFormat := func(name string, conditions ...profile.Condition) (int64, error) {
		for _, f := range formats {
			if reflect.DeepEqual(f.Conditions, conditions) {
				return f.ID, nil
			}
		}
		base := name
		for n := 2; ; n++ {
			used := false
			for _, f := range formats {
				if f.Name == name {
					used = true
					break
				}
			}
			if !used {
				break
			}
			name = fmt.Sprintf("%s (%d)", base, n)
		}
		body, e := json.Marshal(conditions)
		if e != nil {
			return 0, e
		}
		var id int64
		e = tx.QueryRowContext(ctx, s.q(`INSERT INTO custom_formats(name,conditions_json) VALUES(?,?) RETURNING id`), name, string(body)).Scan(&id)
		if e == nil {
			formats = append(formats, profile.Format{ID: id, Name: name, Conditions: conditions})
		}
		return id, e
	}
	for _, v := range versions {
		p := profile.Profile{Name: "Standard", MinFormatScore: profile.MinScoreOff}
		p.Volume, p.Chapter = profile.DefaultRules()
		for _, ex := range profiles {
			if ex.ID == bases[v.ID] {
				p = ex
				break
			}
		}
		p.Required = slices.Clone(p.Required)
		name := p.Name
		if v.RequiredGroup != "" {
			conditions := []profile.Condition{{Type: profile.CondGroup, Value: "(?i)" + v.RequiredGroup}}
			re, e := regexp.Compile("(?i)" + v.RequiredGroup)
			if e != nil {
				return fmt.Errorf("copy %d group rule: %w", v.ID, e)
			}
			// Legacy regexes could also accept an unstated group. Preserve that
			// alternative without changing normal group-format semantics.
			if re.MatchString("") {
				conditions = append(conditions, profile.Condition{Type: profile.CondGroup, Value: ".*", Negate: true})
			}
			id, e := ensureFormat("Group: "+v.RequiredGroup, conditions...)
			if e != nil {
				return e
			}
			if !slices.Contains(p.Required, id) {
				p.Required = append(p.Required, id)
			}
			name += " · " + v.RequiredGroup
		}
		if !v.AllowUnknown {
			id, e := ensureFormat("Stated language", profile.Condition{Type: profile.CondLanguage, Value: profile.AnyLanguage})
			if e != nil {
				return e
			}
			if !slices.Contains(p.Required, id) {
				p.Required = append(p.Required, id)
			}
			name += " · stated language"
		}
		body, e := encodeProfileBody(p)
		if e != nil {
			return e
		}
		var id int64
		for _, ex := range profiles {
			existing, e := encodeProfileBody(ex)
			if e != nil {
				return e
			}
			if existing == body {
				id = ex.ID
				break
			}
		}
		if id == 0 {
			base := name
			for n := 2; ; n++ {
				used := false
				for _, ex := range profiles {
					if ex.Name == name {
						used = true
						break
					}
				}
				if !used {
					break
				}
				name = fmt.Sprintf("%s (%d)", base, n)
			}
			e = tx.QueryRowContext(ctx, s.q(`INSERT INTO profiles(name,body_json) VALUES(?,?) RETURNING id`), name, body).Scan(&id)
			if e != nil {
				return e
			}
			p.ID, p.Name = id, name
			profiles = append(profiles, p)
		}
		if _, e = tx.ExecContext(ctx, s.q(`UPDATE versions SET profile_id=?,required_group='',allow_unknown=TRUE WHERE id=?`), id, v.ID); e != nil {
			return e
		}
	}
	return tx.Commit()
}
