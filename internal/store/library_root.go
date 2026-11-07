package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

const libraryRootKey = "library_root"

// ReconcileLibraryRoot records the initial root, or rewrites library paths
// when it changes. Call only at startup, before other database users start.
// previous is used only to bootstrap a database without a recorded root.
// check validates the physical destinations before any references are changed.
func (s *Store) ReconcileLibraryRoot(ctx context.Context, root, previous string, check func(string) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var raw []byte
	err = tx.QueryRowContext(ctx, s.q(`SELECT value_json FROM settings WHERE key=?`), libraryRootKey).Scan(&raw)
	old := previous
	switch {
	case err == nil:
		if err := json.Unmarshal(raw, &old); err != nil || old == "" {
			return fmt.Errorf("stored library root is invalid")
		}
		if old == root {
			return nil
		}
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return err
	}
	if old == "" {
		old = root
	}
	// Use path components rather than LIKE: '_' and '%' are literal
	// folder characters, and /books-old is not inside /books.
	mapped := func(path string) (string, bool) {
		if path == "" {
			return path, false
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return path, false
		}
		rel, err := filepath.Rel(old, absolute)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return path, false
		}
		dest := filepath.Join(root, rel)
		return dest, dest != path
	}
	type change struct {
		table, column string
		id            int64
		path          string
	}
	var changes []change
	// file_trash holds the physical path; files.path remains its restore
	// destination. Already-missing files keep their missing state.
	for _, field := range []struct{ table, column, query string }{
		{"series", "path", `SELECT id,COALESCE(path,''),FALSE FROM series`},
		{"series", "chapter_path", `SELECT id,COALESCE(chapter_path,''),FALSE FROM series`},
		{"root_folders", "path", `SELECT id,path,FALSE FROM root_folders`},
		{"files", "path", `SELECT f.id,f.path,(f.presence <> 'missing' AND t.file_id IS NULL) FROM files f LEFT JOIN file_trash t ON t.file_id=f.id`},
		{"file_trash", "trash_path", `SELECT file_id,trash_path,TRUE FROM file_trash`},
	} {
		rows, err := tx.QueryContext(ctx, field.query)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id int64
			var path string
			var physical bool
			if err := rows.Scan(&id, &path, &physical); err != nil {
				_ = rows.Close()
				return err
			}
			dest, moved := mapped(path)
			if !moved {
				continue
			}
			if physical {
				if err := check(dest); err != nil {
					_ = rows.Close()
					return err
				}
			}
			changes = append(changes, change{field.table, field.column, id, dest})
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
	}
	for _, c := range changes {
		idColumn := "id"
		if c.table == "file_trash" {
			idColumn = "file_id"
		}
		// Identifiers are fixed above, never supplied by a caller.
		if _, err := tx.ExecContext(ctx, s.q(`UPDATE `+c.table+` SET `+c.column+`=? WHERE `+idColumn+`=?`), c.path, c.id); err != nil {
			return fmt.Errorf("relocate %s.%s: %w", c.table, c.column, err)
		}
	}
	value, err := json.Marshal(root)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, s.q(`INSERT INTO settings (key,value_json,updated_at) VALUES (?,?,?)
		ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json,updated_at=excluded.updated_at`), libraryRootKey, value, utcNow()); err != nil {
		return err
	}
	return tx.Commit()
}
