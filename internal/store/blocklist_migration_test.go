package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/h-wb/sento/internal/db/dbtest"
)

func TestDurableBlocklistMigration(t *testing.T) {
	conn, dialect := dbtest.Open(t)
	s := New(conn, dialect)
	ctx := context.Background()
	// Recreate the previous schema, then exercise the upgrade with real
	// legacy blocks. dbtest runs this fixture on both database dialects.
	for _, column := range []string{"aliases_json", "title"} {
		if _, err := conn.ExecContext(ctx, `ALTER TABLE blocklist DROP COLUMN `+column); err != nil {
			t.Fatal(err)
		}
	}
	var sid int64
	if err := conn.QueryRowContext(ctx, `INSERT INTO series (mangabaka_id,title,sort_title,raw_json) VALUES (999995,'Legacy blocks','legacy blocks','{}') RETURNING id`).Scan(&sid); err != nil {
		t.Fatal(err)
	}
	for _, job := range []struct{ protocol, external, listing, snapshot string }{
		{"torrent", "hash", "listing", `{"guid":"feed-guid"}`},
		{"torrent", "bridge", "hash", `{}`},
		{"torrent", "other-hash", "bridge", `{"guid":"other-feed"}`},
		{"source", "hash", "source-listing", `not-json`},
		{"source", "numeric-guid", "", `{"guid":42}`},
	} {
		if _, err := conn.ExecContext(ctx, s.q(`INSERT INTO queue_items
			(series_id,protocol,external_id,release_key,snapshot_json,title,status,books_json)
			VALUES (?,?,?,?,?,'Release title','failed','[]')`), sid, job.protocol, job.external, job.listing, job.snapshot); err != nil {
			t.Fatal(err)
		}
	}
	for _, block := range []struct{ protocol, key string }{
		{"torrent", "listing"}, {"torrent", "other-feed"}, {"source", "hash"}, {"source", "orphan"},
	} {
		if err := s.AddBlocklist(ctx, block.protocol, block.key, "blocked manually"); err != nil {
			t.Fatal(err)
		}
	}
	query, err := os.ReadFile(filepath.Join("..", "db", "migrations", dialect, "0030_durable_release_blocks.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = conn.ExecContext(ctx, string(query)); err != nil {
		t.Fatal(err)
	}
	// Backfill includes the transitive bridge despite neither of its keys
	// being blocked. It must survive pruning before the first list call.
	if n, err := s.PruneFinishedQueue(ctx, 0); err != nil || n != 5 {
		t.Fatalf("pruned %d jobs, %v", n, err)
	}
	releases, err := s.ListBlockedReleases(ctx)
	if err != nil || len(releases) != 3 {
		t.Fatalf("migrated releases = %v, %v", releases, err)
	}
	for _, release := range releases {
		if release.Reason != "blocked manually" || release.CreatedAt == "" {
			t.Errorf("lost block details: %+v", release)
		}
		if release.Protocol == "torrent" && release.Title != "Release title" {
			t.Errorf("lost release title: %+v", release)
		}
	}
	// hash has no block row of its own in the torrent protocol, but it
	// still identifies both migrated blocks. The source's hash is distinct.
	if err := s.UnblockReleases(ctx, nil, "torrent", []string{"hash"}); err != nil {
		t.Fatal(err)
	}
	blocked, err := s.BlockedKeys(ctx)
	if err != nil || len(blocked) != 2 || !blocked["source/hash"] || !blocked["source/orphan"] {
		t.Fatalf("remaining blocks = %v, %v", blocked, err)
	}
}
