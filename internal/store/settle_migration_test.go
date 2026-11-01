package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/h-wb/sento/internal/db/dbtest"
)

// Migration 0027 settles source jobs that waited for review on a release
// the source cannot give: they fail and the release is blocklisted; any
// other job waiting for review is left alone.
func TestSettleDeadSourceJobsMigration(t *testing.T) {
	conn, dialect := dbtest.Open(t)
	s := New(conn, dialect)
	ctx := context.Background()
	var sid int64
	if err := s.db.QueryRowContext(ctx, s.q(`INSERT INTO series (mangabaka_id, title, sort_title, raw_json) VALUES (999998, 'Settle Test', 'settle test', '{}') RETURNING id`)).Scan(&sid); err != nil {
		t.Fatal(err)
	}
	job := func(protocol, external, key, errText string) int64 {
		t.Helper()
		var id int64
		if err := s.db.QueryRowContext(ctx, s.q(`INSERT INTO queue_items (series_id, protocol, external_id, release_key, title, status, books_json, error) VALUES (?,?,?,?,'x','needs_review','[]',?) RETURNING id`),
			sid, protocol, external, key, errText).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	gone := job("source", "ext-gone", "rel-gone", "listing pages: host: HTTP error 404")
	damaged := job("source", "ext-dmg", "", "source download still failing after 6 attempts: page 3: host: java.lang.IllegalArgumentException: Not a local file header")
	review := job("source", "ext-files", "rel-files", "import needs review: partial import: verified 0 of 8 expected books")
	torrent := job("torrent", "hash", "guid", "listing pages: host: HTTP error 404")
	sql, err := os.ReadFile(filepath.Join("..", "db", "migrations", dialect, "0027_settle_dead_source_jobs.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.ExecContext(ctx, string(sql)); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[int64]string{gone: "failed", damaged: "failed", review: "needs_review", torrent: "needs_review"} {
		q, err := s.GetQueueItem(ctx, id)
		if err != nil || q.Status != want {
			t.Errorf("job %d: %v %v, want %s", id, q, err, want)
		}
	}
	blocked, err := s.BlockedKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"source/ext-gone", "source/rel-gone", "source/ext-dmg"} {
		if !blocked[key] {
			t.Errorf("%s not blocklisted: %v", key, blocked)
		}
	}
	if blocked["source/ext-files"] || blocked["torrent/hash"] {
		t.Errorf("blocklisted a job it should leave alone: %v", blocked)
	}
}
