package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/georgysavva/scany/v2/sqlscan"

	"github.com/h-wb/sento/internal/apperr"
)

// A release can be known by its download ID, listing key and feed GUID.
// Active blocks retain those relationships even after queue history is pruned.
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
	var rows []struct {
		BlocklistEntry
		Aliases JSONText `db:"aliases_json"`
	}
	if err := sqlscan.Select(ctx, tx, &rows, `SELECT id, protocol, key, COALESCE(reason,'') AS reason, created_at, title, aliases_json FROM blocklist ORDER BY id DESC`); err != nil {
		return nil, nil, nil, err
	}
	aliases := releaseAliases{}
	titles := map[releaseIdentity]string{}
	var entries []BlocklistEntry
	for _, row := range rows {
		entries = append(entries, row.BlocklistEntry)
		var keys []string
		if err := json.Unmarshal(row.Aliases, &keys); err != nil {
			return nil, nil, nil, fmt.Errorf("reading aliases for block %d: %w", row.ID, err)
		}
		first := releaseIdentity{row.Protocol, row.Key}
		for _, key := range append(keys, row.Key) {
			if key == "" {
				continue
			}
			id := releaseIdentity{row.Protocol, key}
			aliases[aliases.root(id)] = aliases.root(first)
		}
	}
	for _, row := range rows {
		root := aliases.root(releaseIdentity{row.Protocol, row.Key})
		if titles[root] == "" {
			titles[root] = row.Title
		}
	}
	return entries, aliases, titles, nil
}

// AddReleaseBlocklist stores all known names of a release atomically. Its
// aliases and title live with the blocks, not with disposable queue history.
func (s *Store) AddReleaseBlocklist(ctx context.Context, protocol string, keys []string, title, reason string) error {
	keys = slices.DeleteFunc(slices.Clone(keys), func(key string) bool { return key == "" })
	if len(keys) == 0 {
		return apperr.Invalidf("this release has no identity to block")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, aliases, titles, err := s.loadReleaseBlocks(ctx, tx)
	if err != nil {
		return err
	}
	selected := map[releaseIdentity]bool{}
	for _, key := range keys {
		root := aliases.root(releaseIdentity{protocol, key})
		selected[root] = true
		if title == "" {
			title = titles[root]
		}
	}
	// Preserve aliases learned during earlier attempts, including keys
	// connected through an identifier shared with the current attempt.
	keys = nil
	for id := range aliases {
		if selected[aliases.root(id)] {
			keys = append(keys, id.key)
		}
	}
	slices.Sort(keys)
	raw, err := json.Marshal(keys)
	if err != nil {
		return err
	}
	for _, key := range keys {
		if _, err := tx.ExecContext(ctx, s.q(`
			INSERT INTO blocklist (protocol, key, reason, title, aliases_json) VALUES (?,?,?,?,?)
			ON CONFLICT(protocol, key) DO UPDATE SET reason=excluded.reason, title=excluded.title, aliases_json=excluded.aliases_json`),
			protocol, key, reason, title, string(raw)); err != nil {
			return err
		}
	}
	return tx.Commit()
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
