package store

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/h-wb/sento/internal/db/dbtest"
)

func TestUnblockReleaseAliases(t *testing.T) {
	for _, tc := range []struct {
		name        string
		byID, prune bool
	}{{"id", true, false}, {"keys", false, false}, {"id-after-pruning", true, true}, {"keys-after-pruning", false, true}} {
		t.Run(tc.name, func(t *testing.T) {
			conn, dialect := dbtest.Open(t)
			s := New(conn, dialect)
			ctx := context.Background()
			var sid int64
			if err := conn.QueryRowContext(ctx, `INSERT INTO series (mangabaka_id,title,sort_title,raw_json) VALUES (999996,'Blocks','blocks','{}') RETURNING id`).Scan(&sid); err != nil {
				t.Fatal(err)
			}
			var failed int64
			// The GUID is only in the snapshot, and a second historical
			// attempt connects a different listing to the same download.
			for i, listing := range []string{"listing", "other-listing"} {
				id, err := s.AddQueueItem(ctx, QueueItem{
					SeriesID: sid, Protocol: "torrent", ExternalID: "hash",
					Title: "Release title", Status: QueueFailed, Books: JSONText(`[]`),
				})
				if err != nil {
					t.Fatal(err)
				}
				if _, err = conn.ExecContext(ctx, s.q(`UPDATE queue_items SET release_key=?, snapshot_json=?, error='old download failure' WHERE id=?`), listing, `{"guid":"feed-guid"}`, id); err != nil {
					t.Fatal(err)
				}
				if i == 0 {
					failed = id
				}
				if err := s.AddReleaseBlocklist(ctx, "torrent", []string{"hash", listing, "feed-guid"}, "Release title", "blocked manually"); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.AddBlocklist(ctx, "torrent", "unrelated", "blocked manually"); err != nil {
				t.Fatal(err)
			}
			if err := s.AddBlocklist(ctx, "source", "hash", "HTTP 404"); err != nil {
				t.Fatal(err)
			}
			if tc.prune {
				if n, err := s.PruneFinishedQueue(ctx, 0); err != nil || n != 2 {
					t.Fatalf("pruned %d jobs, %v", n, err)
				}
				s = New(conn, dialect)
			}
			releases, err := s.ListBlockedReleases(ctx)
			if err != nil || len(releases) != 3 {
				t.Fatalf("grouped releases = %v, %v", releases, err)
			}
			var releaseID int64
			for _, release := range releases {
				if release.Title == "Release title" {
					releaseID = release.ID
				}
			}
			if releaseID == 0 {
				t.Fatal("missing grouped release title")
			}
			// A stale selection fails without clearing any other blocks.
			if err = s.UnblockReleases(ctx, []int64{releaseID, 999999}, "", nil); !errors.Is(err, ErrNotFound) {
				t.Fatalf("stale selection = %v", err)
			}
			before, _ := s.ListBlocklist(ctx)
			if len(before) != 6 {
				t.Fatal("stale selection partially unblocked releases")
			}
			if tc.byID {
				err = s.DeleteBlocklist(ctx, releaseID)
			} else {
				err = s.UnblockReleases(ctx, nil, "torrent", []string{"feed-guid"})
			}
			if err != nil {
				t.Fatal(err)
			}
			blocked, err := s.BlockedKeys(ctx)
			if err != nil || len(blocked) != 2 || !blocked["torrent/unrelated"] || !blocked["source/hash"] {
				t.Fatalf("remaining blocks = %v, %v", blocked, err)
			}
			job, err := s.GetQueueItem(ctx, failed)
			if tc.prune {
				if !errors.Is(err, ErrNotFound) {
					t.Fatalf("pruned attempt reappeared: %v, %v", job, err)
				}
			} else if err != nil || job.Status != QueueFailed || job.Error != "old download failure" {
				t.Fatalf("changed historical attempt: %v, %v", job, err)
			}
		})
	}
}

func TestBulkUnblockBeyond500(t *testing.T) {
	conn, dialect := dbtest.Open(t)
	s := New(conn, dialect)
	ctx := context.Background()
	for i := range 505 {
		if err := s.AddBlocklist(ctx, "source", fmt.Sprint(i), "blocked manually"); err != nil {
			t.Fatal(err)
		}
	}
	releases, err := s.ListBlockedReleases(ctx)
	if err != nil || len(releases) != 505 {
		t.Fatalf("list = %d releases, %v", len(releases), err)
	}
	ids := make([]int64, 502)
	for i := range ids {
		ids[i] = releases[i].ID
	}
	if err := s.UnblockReleases(ctx, ids, "", nil); err != nil {
		t.Fatal(err)
	}
	remaining, err := s.BlockedKeys(ctx)
	if err != nil || len(remaining) != 3 {
		t.Fatalf("selected unblock left %d blocks, %v", len(remaining), err)
	}
	// Refill to prove ClearBlocklist clears beyond a displayed page too.
	for i := range 505 {
		if err := s.AddBlocklist(ctx, "source", fmt.Sprint(i), "HTTP 404"); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := s.ClearBlocklist(ctx); err != nil {
			t.Fatal(err)
		}
	}
	remaining, err = s.BlockedKeys(ctx)
	if err != nil || len(remaining) != 0 {
		t.Fatalf("clear left %v, %v", remaining, err)
	}
}
