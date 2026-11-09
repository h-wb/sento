package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/h-wb/sento/internal/db/dbtest"
)

// A series whose books live in edition folders — siblings of its own,
// which may be pruned once empty — still has its files checked against
// the folders they are in, not against the missing series folder.
func TestCheckFilePresenceEditionFolders(t *testing.T) {
	conn, dialect := dbtest.Open(t)
	s := New(conn, dialect)
	ctx := context.Background()
	root := t.TempDir()
	seriesDir := filepath.Join(root, "HUNTER [mangabaka-677]") // pruned: nothing lives here
	chapterDir := filepath.Join(root, "Chapters", "HUNTER [mangabaka-677]")
	editionDir := filepath.Join(root, "HUNTER [mangabaka-677] [Standard Edition][en]")
	if err := os.MkdirAll(editionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	vol := filepath.Join(editionDir, "HUNTER v01.cbz")
	if err := os.WriteFile(vol, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	var sid int64
	if err := conn.QueryRowContext(ctx, s.q(`INSERT INTO series (mangabaka_id,title,sort_title,raw_json,path,chapter_path) VALUES (1,'Test','test','{}',?,?) RETURNING id`), seriesDir, chapterDir).Scan(&sid); err != nil {
		t.Fatal(err)
	}
	insert := func(path string) {
		t.Helper()
		if _, err := conn.ExecContext(ctx, s.q(`INSERT INTO files (series_id,path,size,provenance,presence) VALUES (?,?,12,'scan','present')`), sid, path); err != nil {
			t.Fatal(err)
		}
	}
	insert(vol)
	insert(filepath.Join(editionDir, "HUNTER v02.cbz"))  // not on disk
	insert(filepath.Join(chapterDir, "HUNTER c001.cbz")) // chapter root unreadable
	nested := filepath.Join(editionDir, "(Colored)", "HUNTER v03.cbz")
	insert(nested) // its book folder is gone, the edition folder is not
	offline := filepath.Join(root, "HUNTER [mangabaka-677] [Deluxe][en]", "(Colored)", "HUNTER v01.cbz")
	insert(offline) // the whole edition folder is gone
	if err := s.CheckFilePresence(ctx, sid); err != nil {
		t.Fatal(err)
	}
	rows, err := conn.QueryContext(ctx, s.q(`SELECT path,presence FROM files WHERE series_id=?`), sid)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	got := map[string]string{}
	for rows.Next() {
		var p, state string
		if err := rows.Scan(&p, &state); err != nil {
			t.Fatal(err)
		}
		got[p] = state
	}
	if got[vol] != "present" {
		t.Errorf("book in the edition folder: %q, want present", got[vol])
	}
	if got[filepath.Join(editionDir, "HUNTER v02.cbz")] != "missing" {
		t.Errorf("absent book in the edition folder: %q, want missing", got[filepath.Join(editionDir, "HUNTER v02.cbz")])
	}
	if got[nested] != "missing" {
		t.Errorf("book under a removed book folder of a healthy edition folder: %q, want missing", got[nested])
	}
	if got[offline] != "unavailable" {
		t.Errorf("book under a removed edition folder: %q, want unavailable", got[offline])
	}
	if got[filepath.Join(chapterDir, "HUNTER c001.cbz")] != "unavailable" {
		t.Errorf("book under the unreadable chapter folder: %q, want unavailable", got[filepath.Join(chapterDir, "HUNTER c001.cbz")])
	}
}
