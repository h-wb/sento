package db

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
)

func TestEditionMigrationResetsLibrary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func(original *sql.DB) { _ = original.Close() }(conn)
	conn.SetMaxOpenConns(1)
	if _, err = conn.Exec(`PRAGMA foreign_keys=ON; CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,applied_at TEXT NOT NULL DEFAULT '')`); err != nil {
		t.Fatal(err)
	}
	entries, err := migrationsFS.ReadDir("migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		var version int
		_, err = fmt.Sscanf(entry.Name(), "%d_", &version)
		if err != nil {
			t.Fatal(err)
		}
		if version >= 14 {
			continue
		}
		body, e := migrationsFS.ReadFile("migrations/sqlite/" + entry.Name())
		if e != nil {
			t.Fatal(e)
		}
		if _, e = conn.Exec(string(body)); e != nil {
			t.Fatal(entry.Name(), e)
		}
		if _, e = conn.Exec(`INSERT INTO schema_migrations(version) VALUES(?)`, version); e != nil {
			t.Fatal(e)
		}
	}
	fixture := `INSERT INTO profiles(id,name,body_json) VALUES(43,'Keep this profile','{}');
 INSERT INTO indexers(id,name,protocol,priority) VALUES(43,'Kept source','source',7);
 INSERT INTO series(id,mangabaka_id,title,sort_title,raw_json,total_chapters) VALUES(42,377,'One Piece','One Piece','{}',3);
 INSERT INTO collections(id,series_id,mb_collection_id,edition_name,language,monitored,custom,cutoff_num) VALUES(81,42,'catalog-edition','Standard','en',TRUE,FALSE,0);
 INSERT INTO collections(id,series_id,edition_name,language,ctype,monitored,custom,cutoff_num) VALUES(82,42,'Chapters','en','chapter',TRUE,TRUE,100);
 INSERT INTO works(id,collection_id,mb_work_id,sequence_num,sequence_string,count_type) VALUES(91,81,'volume-one',100,'1','main'),(92,81,'volume-two',200,'2','main'),(93,81,'extra-two',200,'2','extra');
 INSERT INTO files(id,series_id,path,size) VALUES(101,42,'/existing/book.cbz',1),(102,42,'/existing/ambiguous.cbz',2),(103,42,'/existing/chapter.cbz',3);
 INSERT INTO file_slot_links(file_id,ledger,collection_id,number) VALUES(101,'volume',81,100),(102,'volume',81,200),(103,'chapter',82,200);
 INSERT INTO slot_overrides(series_id,ledger,collection_id,number,monitored) VALUES(42,'volume',81,100,FALSE);
 INSERT INTO queue_items(id,series_id,protocol,external_id,title,status,expected_slots_json) VALUES(201,42,'torrent','existing-download','Book','downloading','[{"ledger":"volume","collectionId":81,"number":100}]');`
	if _, err = conn.Exec(fixture); err != nil {
		t.Fatal(err)
	}
	if err = conn.Close(); err != nil {
		t.Fatal(err)
	}
	conn, err = openSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	for _, table := range []string{"series", "collections", "works", "files", "queue_items", "asset_coverage", "download_targets"} {
		var count int
		if err := conn.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s was not reset: %d %v", table, count, err)
		}
	}
	for _, table := range []string{"file_slot_links", "slot_overrides", "chapter_ledgers"} {
		var count int
		if err := conn.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("obsolete table retained: %s %v", table, err)
		}
	}
	var kept int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM profiles WHERE id=43 AND name='Keep this profile'`).Scan(&kept); err != nil || kept != 1 {
		t.Fatalf("profile lost: %d %v", kept, err)
	}
	if err := conn.QueryRow(`SELECT priority FROM indexers WHERE id=43`).Scan(&kept); err != nil || kept != 7 {
		t.Fatalf("connection settings lost: %d %v", kept, err)
	}
	rows, err := conn.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		t.Fatal("migration left dangling foreign keys")
	}
}
