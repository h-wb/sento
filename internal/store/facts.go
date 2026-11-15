package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/profile"
)

type AssetFacts struct {
	Sources     map[string]string `json:"sources,omitempty"`
	Title       string            `json:"title,omitempty"`
	Site        string            `json:"site,omitempty"`
	Protocol    string            `json:"protocol,omitempty"`
	Flags       map[string]bool   `json:"flags,omitempty"`
	FactsKnown  bool              `json:"factsKnown"`
	Language    string            `json:"language,omitempty"`
	Group       string            `json:"group,omitempty"`
	Format      string            `json:"format,omitempty"`
	FormatScore int               `json:"formatScore"`
	Resolution  int               `json:"resolution"`
	Revision    int               `json:"revision"`
	Digital     bool              `json:"digital"`
	ProfileID   int64             `json:"profileId,omitempty"`
	ReleaseKey  string            `json:"releaseKey,omitempty"`
}

func (s *Store) SetAssetFacts(ctx context.Context, id int64, f AssetFacts, hash string) error {
	raw, err := json.Marshal(f)
	if err != nil {
		return err
	}
	// A different fingerprint was not read from the archive the signature describes.
	return s.touch(ctx, `UPDATE files SET facts_json=?,hash_stat=CASE WHEN COALESCE(hash,'')=? THEN hash_stat ELSE '' END,hash=? WHERE id=?`, string(raw), hash, hash, id)
}

func (f *AssetFacts) Scan(src any) error {
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), f)
	case []byte:
		return json.Unmarshal(v, f)
	case nil:
		*f = AssetFacts{}
		return nil
	default:
		return fmt.Errorf("cannot scan asset facts from %T", src)
	}
}

// FactFlags are the flags a file's facts carry, as profiles' custom formats
// test them and a person can set them.
var FactFlags = profile.Flags

// manualSource marks a fact a person set: no re-read, re-scan or release
// claim replaces it.
const manualSource = "manual"

func flagSource(flag string) string { return "flag:" + flag }

// ManualFlag reports whether a person set the flag, either way.
func (f AssetFacts) ManualFlag(flag string) bool {
	return f.Sources[flagSource(flag)] == manualSource
}

// AssetFactPatch is a person's correction of what a file is. A flag named
// in Flags is set to that value by hand; the others are left as read.
type AssetFactPatch struct {
	Language *string         `json:"language,omitempty"`
	Group    *string         `json:"group,omitempty"`
	Flags    map[string]bool `json:"flags,omitempty"`
}

func (p AssetFactPatch) Validate() error {
	for flag := range p.Flags {
		if !slices.Contains(FactFlags, flag) {
			return apperr.Invalidf("unknown flag %q", flag)
		}
	}
	return nil
}

// Corrected applies a person's corrections and marks each as theirs.
func (f AssetFacts) Corrected(p AssetFactPatch) AssetFacts {
	f.Sources, f.Flags = maps.Clone(f.Sources), maps.Clone(f.Flags)
	if f.Sources == nil {
		f.Sources = map[string]string{}
	}
	if p.Language != nil {
		f.Language = strings.TrimSpace(*p.Language)
		f.Sources["language"] = manualSource
	}
	if p.Group != nil {
		f.Group = strings.TrimSpace(*p.Group)
		f.Sources["group"] = manualSource
	}
	for flag, on := range p.Flags {
		f.setFlag(flag, on)
		f.Sources[flagSource(flag)] = manualSource
	}
	return f
}

func (f *AssetFacts) setFlag(flag string, on bool) {
	if f.Flags == nil {
		f.Flags = map[string]bool{}
	}
	f.Flags[flag] = on
	if flag == "digital" {
		f.Digital = on
	}
}

// KeepingManual carries over, onto facts read anew, everything a person had
// set by hand in the previous ones.
func (f AssetFacts) KeepingManual(previous AssetFacts) AssetFacts {
	f.Sources, f.Flags = maps.Clone(f.Sources), maps.Clone(f.Flags)
	if f.Sources == nil {
		f.Sources = map[string]string{}
	}
	if previous.Sources["language"] == manualSource {
		f.Language = previous.Language
		f.Sources["language"] = manualSource
	}
	if previous.Sources["group"] == manualSource {
		f.Group = previous.Group
		f.Sources["group"] = manualSource
	}
	for _, flag := range FactFlags {
		if previous.ManualFlag(flag) {
			f.setFlag(flag, previous.Flags[flag])
			f.Sources[flagSource(flag)] = manualSource
		}
	}
	return f
}

// CorrectFileFacts records a person's correction of one file's facts.
func (s *Store) CorrectFileFacts(ctx context.Context, sid, fid int64, p AssetFactPatch) error {
	return s.CorrectFilesFacts(ctx, sid, []int64{fid}, p)
}

// CorrectFilesFacts records the same correction on several files of a
// series, all or none.
func (s *Store) CorrectFilesFacts(ctx context.Context, sid int64, ids []int64, p AssetFactPatch) error {
	if err := p.Validate(); err != nil {
		return err
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		for _, fid := range ids {
			var seriesID int64
			var raw string
			if err := tx.QueryRowContext(ctx, s.q(`SELECT series_id,facts_json FROM files WHERE id=?`), fid).Scan(&seriesID, &raw); errors.Is(err, sql.ErrNoRows) || (err == nil && seriesID != sid) {
				return ErrNotFound
			} else if err != nil {
				return err
			}
			var facts AssetFacts
			if err := json.Unmarshal([]byte(raw), &facts); err != nil {
				return err
			}
			corrected, err := json.Marshal(facts.Corrected(p))
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, s.q(`UPDATE files SET facts_json=?,modified_at=? WHERE id=?`), string(corrected), utcNow(), fid); err != nil {
				return err
			}
		}
		return nil
	})
}
