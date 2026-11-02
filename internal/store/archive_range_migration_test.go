package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/h-wb/sento/internal/db/dbtest"
)

func TestArchiveRangeBlocklistMigration(t *testing.T) {
	conn, dialect := dbtest.Open(t)
	s := New(conn, dialect)
	ctx := context.Background()
	var sid int64
	if err := conn.QueryRowContext(ctx, s.q(`INSERT INTO series (mangabaka_id,title,sort_title,raw_json) VALUES (999997,'Range Test','range test','{}') RETURNING id`)).Scan(&sid); err != nil {
		t.Fatal(err)
	}
	const rangeError = "page 5: host: java.lang.IllegalArgumentException: Not a local file header"
	job := func(external, key, message string) int64 {
		t.Helper()
		var id int64
		if err := conn.QueryRowContext(ctx, s.q(`INSERT INTO queue_items (series_id,protocol,external_id,release_key,title,status,books_json,error) VALUES (?,'source',?,?,'x','failed','[]',?) RETURNING id`), sid, external, key, message).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	failed := job("range-external", "range-release", rangeError)
	job("gone-external", "gone-release", "listing pages: host: HTTP error 404")
	job("gone-410", "gone-410-release", "listing pages: host: HTTP error 410")
	// A later 404 wins over an earlier range failure for the same release.
	job("gone-external", "gone-release", rangeError)
	for _, b := range []struct{ protocol, key, reason string }{
		{"source", "range-external", rangeError},
		{"source", "range-release", "the source cannot give this release"},
		{"source", "range-feed-alias", rangeError},
		{"source", "manual", "blocked manually"},
		{"source", "gone-external", rangeError},
		{"source", "gone-release", "the source cannot give this release"},
		{"source", "gone-410", "the source cannot give this release"},
		{"source", "zip-format", "invalid entry size"},
		{"torrent", "range-external", rangeError},
	} {
		if err := s.AddBlocklist(ctx, b.protocol, b.key, b.reason); err != nil {
			t.Fatal(err)
		}
	}
	job("manual", "", rangeError)
	query, err := os.ReadFile(filepath.Join("..", "db", "migrations", dialect, "0028_retry_archive_range_reads.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := conn.ExecContext(ctx, string(query)); err != nil {
			t.Fatal(err)
		}
	}
	blocked, err := s.BlockedKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"source/range-external", "source/range-release", "source/range-feed-alias"} {
		if blocked[key] {
			t.Errorf("retained automatic range block %s", key)
		}
	}
	for _, key := range []string{"source/manual", "source/gone-external", "source/gone-release", "source/gone-410", "source/zip-format", "torrent/range-external"} {
		if !blocked[key] {
			t.Errorf("removed unrelated block %s", key)
		}
	}
	item, err := s.GetQueueItem(ctx, failed)
	if err != nil || item.Status != QueueFailed {
		t.Fatalf("revived historical job: %v, %v", item, err)
	}
}
