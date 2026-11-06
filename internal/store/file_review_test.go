package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/h-wb/sento/internal/db/dbtest"
	"github.com/h-wb/sento/internal/profile"
)

func TestCommitFileReviewsSerializesSeriesProfileUpdates(t *testing.T) {
	t.Parallel()
	conn, dialect := dbtest.Open(t)
	s := New(conn, dialect)
	ctx := context.Background()
	var sid int64
	if err := conn.QueryRowContext(ctx, `INSERT INTO series (mangabaka_id,title,sort_title,raw_json) VALUES (999995,'Review','review','{}') RETURNING id`).Scan(&sid); err != nil {
		t.Fatal(err)
	}
	pid, err := s.AddProfile(ctx, profile.Profile{Name: "New inherited profile", MinFormatScore: profile.MinScoreOff})
	if err != nil {
		t.Fatal(err)
	}
	revision, err := s.ReviewRevision(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	// This callback runs after the revision check while the review commits.
	// A direct series-profile writer must wait until the review releases its
	// row lock. A short-lived writer makes that ordering deterministic.
	validated := false
	err = s.CommitFileReviews(ctx, sid, revision, nil, func() error {
		validated = true
		writerCtx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
		defer cancel()
		if e := s.SetSeriesProfile(writerCtx, sid, pid); !errors.Is(e, context.DeadlineExceeded) {
			t.Errorf("series policy changed during review commit: %v", e)
		}
		return nil
	})
	if err != nil || !validated {
		t.Fatalf("review commit: validated=%v err=%v", validated, err)
	}
	if err = s.SetSeriesProfile(ctx, sid, pid); err != nil {
		t.Fatalf("series remained locked after review: %v", err)
	}
	fresh, err := s.ReviewRevision(ctx, sid)
	if err != nil || fresh == revision {
		t.Fatalf("profile update did not invalidate the preview: %v", err)
	}
	if err = s.CommitFileReviews(ctx, sid, revision, nil, nil); err == nil {
		t.Fatal("old preview remained applicable after profile update")
	}
}
