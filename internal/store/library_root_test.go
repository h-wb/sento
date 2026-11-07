package store

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"github.com/h-wb/sento/internal/db/dbtest"
)

func TestLibraryRootRelocation(t *testing.T) {
	for _, bootstrap := range []bool{false, true} {
		t.Run(fmt.Sprint("bootstrap=", bootstrap), func(t *testing.T) {
			conn, dialect := dbtest.Open(t)
			s := New(conn, dialect)
			ctx := context.Background()
			// Moving to a parent is the production layout change; the
			// metacharacters must be treated as literal path characters.
			root := filepath.Join(t.TempDir(), "manga_%")
			old := filepath.Join(root, "Sento")
			var sid int64
			if err := conn.QueryRowContext(ctx, s.q(`INSERT INTO series (mangabaka_id,title,sort_title,raw_json,path,chapter_path) VALUES (1,'Test','test','{}',?,?) RETURNING id`), old+"/Volumes/Test", old+"/Chapters/Test").Scan(&sid); err != nil {
				t.Fatal(err)
			}
			insert := func(path, presence string) int64 {
				t.Helper()
				var id int64
				if err := conn.QueryRowContext(ctx, s.q(`INSERT INTO files (series_id,path,size,provenance,presence) VALUES (?,?,12,'scan',?) RETURNING id`), sid, path, presence).Scan(&id); err != nil {
					t.Fatal(err)
				}
				return id
			}
			live := insert(old+"/Volumes/Test/v01.cbz", "present")
			gone := insert(old+"/Chapters/Test/c01.cbz", "missing")
			trashed := insert(old+"/Volumes/Test/review/v02.cbz", "missing")
			outside := insert(old+"-other/v03.cbz", "present")
			trashPath := old + "/Volumes/Test/.sento-trash/v02.cbz"
			if _, err := conn.ExecContext(ctx, s.q(`INSERT INTO file_trash (file_id,trash_path,digest,trashed_at,purge_after) VALUES (?,?,'hash','2026-01-01','2027-01-01')`), trashed, trashPath); err != nil {
				t.Fatal(err)
			}
			if _, err := conn.ExecContext(ctx, s.q(`INSERT INTO root_folders (path) VALUES (?)`), old); err != nil {
				t.Fatal(err)
			}
			check := func(string) error { return nil }
			previous := old
			if !bootstrap {
				if err := s.ReconcileLibraryRoot(ctx, old, "", check); err != nil {
					t.Fatal(err)
				}
				previous = ""
			}
			// Missing destination: neither the marker nor any row changes.
			if err := s.ReconcileLibraryRoot(ctx, root, previous, func(string) error { return fmt.Errorf("unmounted") }); err == nil {
				t.Fatal("accepted an unavailable destination")
			}
			series, _ := s.GetSeries(ctx, sid)
			if series.Path != old+"/Volumes/Test" {
				t.Fatal("failed validation changed the series")
			}
			var checked []string
			if err := s.ReconcileLibraryRoot(ctx, root, previous, func(p string) error { checked = append(checked, p); return nil }); err != nil {
				t.Fatal(err)
			}
			slices.Sort(checked)
			wantChecked := []string{root + "/Volumes/Test/.sento-trash/v02.cbz", root + "/Volumes/Test/v01.cbz"}
			if !slices.Equal(checked, wantChecked) {
				t.Fatalf("validated %v, want %v", checked, wantChecked)
			}
			for id, want := range map[int64]string{live: root + "/Volumes/Test/v01.cbz", gone: root + "/Chapters/Test/c01.cbz", trashed: root + "/Volumes/Test/review/v02.cbz", outside: old + "-other/v03.cbz"} {
				var got string
				if err := conn.QueryRowContext(ctx, s.q(`SELECT path FROM files WHERE id=?`), id).Scan(&got); err != nil || got != want {
					t.Fatalf("file %d: %q, %v; want %q", id, got, err, want)
				}
			}
			series, _ = s.GetSeries(ctx, sid)
			trash, _ := s.GetFileTrash(ctx, trashed)
			var folder string
			if err := conn.QueryRowContext(ctx, `SELECT path FROM root_folders`).Scan(&folder); err != nil {
				t.Fatal(err)
			}
			if series.Path != root+"/Volumes/Test" || series.ChapterPath != root+"/Chapters/Test" || trash.TrashPath != root+"/Volumes/Test/.sento-trash/v02.cbz" || folder != root {
				t.Fatalf("incomplete relocation: %+v %+v %s", series, trash, folder)
			}
			// A retained bootstrap variable must never rewrite twice, including
			// when moving into a descendant of the old root.
			for _, dest := range []string{root + "/Nested", root + "/Nested"} {
				if err := s.ReconcileLibraryRoot(ctx, dest, old, check); err != nil {
					t.Fatal(err)
				}
			}
			series, _ = s.GetSeries(ctx, sid)
			if series.Path != root+"/Nested/Volumes/Test" {
				t.Fatalf("repeated relocation: %q", series.Path)
			}
		})
	}
}

func TestLibraryRootCollisionRollsBack(t *testing.T) {
	conn, dialect := dbtest.Open(t)
	s := New(conn, dialect)
	ctx := context.Background()
	old, root := "/old", "/new"
	var sid int64
	if err := conn.QueryRowContext(ctx, s.q(`INSERT INTO series (mangabaka_id,title,sort_title,raw_json,path) VALUES (1,'Test','test','{}',?) RETURNING id`), old+"/Test").Scan(&sid); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{old + "/Test/v01.cbz", root + "/Test/v01.cbz"} {
		if _, err := conn.ExecContext(ctx, s.q(`INSERT INTO files (series_id,path,size,provenance) VALUES (?,?,1,'scan')`), sid, path); err != nil {
			t.Fatal(err)
		}
	}
	check := func(string) error { return nil }
	if err := s.ReconcileLibraryRoot(ctx, old, "", check); err != nil {
		t.Fatal(err)
	}
	if err := s.ReconcileLibraryRoot(ctx, root, "", check); err == nil {
		t.Fatal("accepted a path collision")
	}
	series, _ := s.GetSeries(ctx, sid)
	raw, _ := s.GetSetting(ctx, libraryRootKey)
	if series.Path != old+"/Test" || string(raw) != `"/old"` {
		t.Fatalf("partial relocation: %s %s", series.Path, raw)
	}
}
