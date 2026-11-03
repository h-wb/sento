package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/georgysavva/scany/v2/sqlscan"
)

// A release can be known by its download ID, listing key and feed GUID.
// Queue snapshots retain those relationships even after a failed attempt.
type releaseIdentity struct{ protocol, key string }
type releaseAliases map[releaseIdentity]releaseIdentity

func (a releaseAliases) root(id releaseIdentity) releaseIdentity {
	parent, ok := a[id]
	if !ok {
		a[id] = id
		return id
	}
	if parent != id {
		a[id] = a.root(parent)
	}
	return a[id]
}

func (s *Store) loadReleaseBlocks(ctx context.Context, tx *sql.Tx) ([]BlocklistEntry, releaseAliases, map[releaseIdentity]string, error) {
	var entries []BlocklistEntry
	if err := sqlscan.Select(ctx, tx, &entries, `SELECT id, protocol, key, COALESCE(reason,'') AS reason, created_at FROM blocklist ORDER BY id DESC`); err != nil {
		return nil, nil, nil, err
	}
	aliases := releaseAliases{}
	titles := map[releaseIdentity]string{}
	if len(entries) == 0 {
		return entries, aliases, titles, nil
	}
	var jobs []struct {
		Protocol   string
		ExternalID string
		ReleaseKey string
		Snapshot   JSONText `db:"snapshot_json"`
		Title      string
	}
	if err := sqlscan.Select(ctx, tx, &jobs, `SELECT protocol, external_id, release_key, snapshot_json, title FROM queue_items ORDER BY id DESC`); err != nil {
		return nil, nil, nil, err
	}
	for _, job := range jobs {
		var snapshot struct {
			GUID string `json:"guid"`
		}
		_ = json.Unmarshal(job.Snapshot, &snapshot)
		var first releaseIdentity
		for _, key := range []string{job.ExternalID, job.ReleaseKey, snapshot.GUID} {
			if key == "" {
				continue
			}
			id := releaseIdentity{job.Protocol, key}
			if first.key == "" {
				first = id
			}
			aliases[aliases.root(id)] = aliases.root(first)
			if _, ok := titles[id]; !ok {
				titles[id] = job.Title
			}
		}
	}
	return entries, aliases, titles, nil
}

// ListBlockedReleases groups aliases into one row per release. It returns
// the complete list so older blocks can be selected or cleared too.
func (s *Store) ListBlockedReleases(ctx context.Context) ([]BlocklistEntry, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	entries, aliases, titles, err := s.loadReleaseBlocks(ctx, tx)
	if err != nil {
		return nil, err
	}
	seen := map[releaseIdentity]bool{}
	var releases []BlocklistEntry
	for _, entry := range entries {
		id := releaseIdentity{entry.Protocol, entry.Key}
		root := aliases.root(id)
		if seen[root] {
			continue
		}
		seen[root] = true
		entry.Title = titles[root]
		releases = append(releases, entry)
	}
	return releases, tx.Commit()
}

// UnblockReleases removes every known alias of selected entries or release
// keys in one transaction. It never changes historical jobs or reservations.
func (s *Store) UnblockReleases(ctx context.Context, ids []int64, protocol string, keys []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	entries, aliases, _, err := s.loadReleaseBlocks(ctx, tx)
	if err != nil {
		return err
	}
	byID := map[int64]releaseIdentity{}
	for _, entry := range entries {
		byID[entry.ID] = releaseIdentity{entry.Protocol, entry.Key}
	}
	selected := map[releaseIdentity]bool{}
	for _, id := range ids {
		identity, ok := byID[id]
		if !ok {
			return ErrNotFound
		}
		selected[aliases.root(identity)] = true
	}
	for _, key := range keys {
		selected[aliases.root(releaseIdentity{protocol, key})] = true
	}
	var remove []any
	for _, entry := range entries {
		if selected[aliases.root(releaseIdentity{entry.Protocol, entry.Key})] {
			remove = append(remove, entry.ID)
		}
	}
	// Keep each statement within both databases' parameter limits.
	for len(remove) > 0 {
		n := min(len(remove), 400)
		if _, err := tx.ExecContext(ctx, s.q(`DELETE FROM blocklist WHERE id IN (`+strings.TrimSuffix(strings.Repeat("?,", n), ",")+`)`), remove[:n]...); err != nil {
			return err
		}
		remove = remove[n:]
	}
	return tx.Commit()
}

// ClearBlocklist clears all blocks, including entries outside a displayed page.
func (s *Store) ClearBlocklist(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM blocklist`)
	return err
}
