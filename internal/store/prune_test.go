package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/h-wb/sento/internal/db/dbtest"
)

// Pruning keeps the newest terminal rows (and all active ones) and drops
// the rest, in both the queue and the history.
func TestPruneFinishedQueueAndDecisions(t *testing.T) {
	conn, dialect := dbtest.Open(t)
	s := New(conn, dialect)
	ctx := context.Background()
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	if _, err := s.db.ExecContext(ctx, s.q(`INSERT INTO series (id, mangabaka_id, title, sort_title, raw_json) VALUES (1, 999999, 'Prune Test', 'prune test', '{}')`)); err != nil {
		t.Fatal(err)
	}
	// 5 terminal jobs, oldest first, plus one active job.
	for i := 0; i < 5; i++ {
		status := "imported"
		switch i {
		case 2:
			status = "cancelled"
		case 4:
			status = "failed"
		}
		created := time.Now().Add(time.Duration(-i-1) * time.Hour).UTC().Format("2006-01-02 15:04:05")
		if _, err := s.db.ExecContext(ctx, s.q(`INSERT INTO queue_items (series_id, protocol, external_id, title, status, books_json, snapshot_json, created_at, updated_at) VALUES (1,'torrent',?,'old',?,?,?,?,?)`),
			fmt.Sprintf("ext-%d", i), status, "[]", "{}", created, now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.ExecContext(ctx, s.q(`INSERT INTO queue_items (series_id, protocol, external_id, title, status, books_json, snapshot_json, created_at, updated_at) VALUES (1,'torrent','active-ext','active','queued','[]','{}',?,?)`), now, now); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		created := time.Now().Add(time.Duration(-i) * time.Hour).UTC().Format("2006-01-02 15:04:05")
		if _, err := s.db.ExecContext(ctx, s.q(`INSERT INTO decisions (series_id, body_json, created_at) VALUES (1,'{}',?)`), created); err != nil {
			t.Fatal(err)
		}
	}

	// A generous cap (what the housekeeping task uses) removes nothing.
	if n, err := s.PruneFinishedQueue(ctx, 200); err != nil || n != 0 {
		t.Fatalf("generous prune removed %d (err %v), want 0", n, err)
	}
	// A small cap keeps the newest 2 terminal jobs and drops 3; the active
	// job is never touched.
	if n, err := s.PruneFinishedQueue(ctx, 2); err != nil || n != 3 {
		t.Fatalf("prune removed %d (err %v), want 3", n, err)
	}
	var queueN, activeN int
	if err := s.db.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM queue_items`)).Scan(&queueN); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM queue_items WHERE status='queued'`)).Scan(&activeN); err != nil {
		t.Fatal(err)
	}
	if queueN != 3 || activeN != 1 {
		t.Fatalf("after prune: %d rows (%d active), want 3 rows / 1 active", queueN, activeN)
	}
	if n, err := s.PruneDecisions(ctx, 2); err != nil || n != 3 {
		t.Fatalf("decision prune removed %d (err %v), want 3", n, err)
	}
	var decN int
	if err := s.db.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM decisions`)).Scan(&decN); err != nil {
		t.Fatal(err)
	}
	if decN != 2 {
		t.Fatalf("decisions has %d rows after prune, want 2", decN)
	}
}
