package service

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/h-wb/sento/internal/metadata"
	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

func TestActivityGroupsFileQuestionsWithTheirImport(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	lib, st, sid, edition, books, version := modelFixture(t)
	reserve := func(key string, book int, status string) int64 {
		t.Helper()
		id, _, err := st.ReserveQueue(ctx, store.QueueItem{SeriesID: sid, Protocol: provider.ProtocolSource, ReleaseKey: key, Title: key}, []store.BookRef{modelRef(edition, books[book], version)})
		if err != nil {
			t.Fatal(err)
		}
		if err = st.UpdateQueueItem(ctx, id, status, ""); err != nil {
			t.Fatal(err)
		}
		return id
	}
	retry := reserve("temporary", 0, store.QueueRetryWait)
	pack := reserve("pack", 1, store.QueueNeedsReview)
	path := t.TempDir() + "/Volume 2.cbz"
	if err := os.WriteFile(path, []byte("archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddFile(ctx, store.File{SeriesID: sid, Path: path, NeedsReview: true, Unrequested: true, Facts: store.AssetFacts{Title: "Volume 2.cbz", ReleaseKey: "pack"}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	for _, scope := range []int64{0, sid} {
		data, err := lib.Activity(ctx, scope)
		if err != nil {
			t.Fatal(err)
		}
		if len(data.Downloads) != 2 {
			t.Fatalf("downloads=%+v", data.Downloads)
		}
		for _, d := range data.Downloads {
			if d.Job.SeriesTitle == "" {
				t.Fatal("missing series title")
			}
			switch d.Job.ID {
			case retry:
				if d.Attention || d.History {
					t.Fatalf("automatic retry became a question: %+v", d)
				}
			case pack:
				if !d.Attention || len(d.Files) != 1 || d.Job.CanTryAnother {
					t.Fatalf("import file question missing or unsafe recovery: %+v", d)
				}
			}
		}
		for _, r := range data.Reviews {
			if r.Kind == "file" || r.Kind == "download" {
				t.Fatalf("import is duplicated in attention: %+v", r)
			}
		}
	}
	otherProvider := testProvider()
	otherProvider.series = &metadata.Series{MangaBakaID: 999, Title: "Another series"}
	otherProvider.collections = nil
	otherProvider.works = nil
	other, err := NewLibrary(st, otherProvider).AddSeries(ctx, AddSeriesRequest{MangaBakaID: 999, Monitor: "none"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.AddQueueItem(ctx, store.QueueItem{SeriesID: other, Protocol: "torrent", Status: store.QueueNeedsReview, Books: []byte(`[]`)}); err != nil {
		t.Fatal(err)
	}
	scoped, err := lib.Activity(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	all, err := lib.Activity(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(scoped.Downloads) != 2 || len(all.Downloads) != 3 {
		t.Fatalf("scope leaked: series=%d all=%d", len(scoped.Downloads), len(all.Downloads))
	}

}

func TestActivityRetiresFailedAttemptsOnlyWhenAllBooksAreCovered(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	lib, st, sid, edition, books, version := modelFixture(t)
	old, _, err := st.ReserveQueue(ctx, store.QueueItem{SeriesID: sid, Protocol: "torrent", ReleaseKey: "old-pack"}, []store.BookRef{modelRef(edition, books[0], version), modelRef(edition, books[1], version)})
	if err != nil {
		t.Fatal(err)
	}
	if err = st.UpdateQueueItem(ctx, old, store.QueueFailed, "download dead"); err != nil {
		t.Fatal(err)
	}
	reserve := func(key string, book int) int64 {
		t.Helper()
		id, _, err := st.ReserveQueue(ctx, store.QueueItem{SeriesID: sid, Protocol: "torrent", ReleaseKey: key}, []store.BookRef{modelRef(edition, books[book], version)})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	first := reserve("replacement-1", 0)
	check := func(history bool) {
		t.Helper()
		data, err := lib.Activity(ctx, sid)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range data.Downloads {
			if d.Job.ID == old {
				if d.History != history || d.Attention == history {
					t.Fatalf("failed attempt: %+v", d)
				}
				return
			}
		}
		t.Fatal("failed attempt disappeared from history")
	}
	check(false) // One replacement cannot clear a partly uncovered pack.
	reserve("replacement-2", 1)
	check(true)
	targets, err := NewImporter(st, provider.NewRegistry(), t.TempDir(), nil).TryAnother(ctx, []int64{old})
	if err != nil || len(targets) != 0 {
		t.Fatalf("resolved failure was recovered again: %v %v", targets, err)
	}
	if job, _ := st.GetQueueItem(ctx, old); job.Status != store.QueueFailed {
		t.Fatal("reading Activity changed persisted job state")
	}
	if err = st.CancelQueue(ctx, first); err != nil {
		t.Fatal(err)
	}
	check(false)
}

func TestActivityOwnedBooksClearAFailureButKeepUnmatchedFilesVisible(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	lib, st, sid, edition, books, version := modelFixture(t)
	qid, _, err := st.ReserveQueue(ctx, store.QueueItem{SeriesID: sid, Protocol: "torrent", ReleaseKey: "failed"}, []store.BookRef{modelRef(edition, books[0], version)})
	if err != nil {
		t.Fatal(err)
	}
	if err = st.UpdateQueueItem(ctx, qid, store.QueueFailed, "failed"); err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/book.cbz"
	if err = os.WriteFile(path, []byte("archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	fid, err := st.AddFile(ctx, store.File{SeriesID: sid, Path: path, Facts: store.AssetFacts{ReleaseKey: "working"}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.MapFile(ctx, sid, fid, []store.BookRef{modelRef(edition, books[0], version)}); err != nil {
		t.Fatal(err)
	}
	data, err := lib.Activity(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	if !data.Downloads[0].History || data.Downloads[0].Attention {
		t.Fatalf("owned book still asks for a replacement: %+v", data.Downloads[0])
	}
	// Files still waiting on a person remain one import question, even if
	// another file happens to satisfy all the original books already.
	if _, err = st.AddFile(ctx, store.File{SeriesID: sid, Path: t.TempDir() + "/extra.cbz", NeedsReview: true, Facts: store.AssetFacts{ReleaseKey: "failed"}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	data, err = lib.Activity(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	d := data.Downloads[0]
	if d.History || !d.Attention || len(d.Files) != 1 {
		t.Fatalf("unresolved file was hidden: %+v", d)
	}
	raw, _ := json.Marshal(data)
	if !json.Valid(raw) {
		t.Fatal("activity cannot be serialized")
	}
}

func TestActivityExplainsTheCauseWithoutMistakingURLNumbersForStatuses(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ error, issue string }{
		{"listing pages: host: HTTP error 404", "gone"},
		{"host: HTTP 403", "access"},
		{"page 3: host: Not a local file header", "archive_read"},
		{"chapter has no pages", "empty"},
		{"host: unreadable archive", "damaged"},
		{"host: HTTP 429", "rate_limit"},
		{"host: HTTP 503", "connection"},
		{"open file: permission denied", "storage"},
		{"failed reading https://example/404/file.cbz", "download"},
	} {
		issue, _ := downloadIssue(store.QueueItem{Status: store.QueueFailed, Error: tc.error})
		if issue != tc.issue {
			t.Errorf("%q: %s, want %s", tc.error, issue, tc.issue)
		}
	}
}

// An unfinished pack must not hide the books that already imported, nor
// silently abandon its unresolved books when additional files need review.
func TestPartialImportShowsVerifiedBooksAndScopesReplacementToUnmatchedBooks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	lib, st, sid, edition, books, version := modelFixture(t)
	refs := []store.BookRef{modelRef(edition, books[0], version), modelRef(edition, books[1], version)}
	qid, _, err := st.ReserveQueue(ctx, store.QueueItem{SeriesID: sid, Protocol: "torrent", ReleaseKey: "partial", Title: "Example pack"}, refs)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.UpdateQueueItem(ctx, qid, store.QueueNeedsReview, "partial import"); err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/confirmed.cbz"
	if err = os.WriteFile(path, []byte("verified pages"), 0600); err != nil {
		t.Fatal(err)
	}
	fid, err := st.AddFile(ctx, store.File{SeriesID: sid, Path: path, Presence: "present", Facts: store.AssetFacts{ReleaseKey: "partial"}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.MapFile(ctx, sid, fid, refs[:1]); err != nil {
		t.Fatal(err)
	}
	extraPath := t.TempDir() + "/extra.cbz"
	if err = os.WriteFile(extraPath, []byte("extra pages"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = st.AddFile(ctx, store.File{SeriesID: sid, Path: extraPath, NeedsReview: true, Unrequested: true, Facts: store.AssetFacts{ReleaseKey: "partial"}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	for _, scope := range []int64{0, sid} {
		data, err := lib.Activity(ctx, scope)
		if err != nil {
			t.Fatal(err)
		}
		d := data.Downloads[0]
		if d.ImportedBooks != 1 || len(d.UnresolvedBooks) != 1 || d.UnresolvedBooks[0] != refs[1] || !d.Attention || d.History || d.Job.CanTryAnother {
			t.Fatalf("incorrect partial import: %+v", d)
		}
	}
	view, err := lib.BooksByVersion(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	if view[version.ID][0].State != StateOwned || view[version.ID][1].State != StatePending {
		t.Fatalf("partial ownership: %+v", view[version.ID])
	}
	svc := NewSearchService(st, provider.NewRegistry())
	run := &searchRun{seriesID: sid, ledger: LedgerVolume, scope: &SearchScope{VersionID: version.ID, ReplaceQueueID: qid, All: true}, replacing: map[string]bool{refKey(refs[0]): true, refKey(refs[1]): true}}
	if err = svc.prepareVersions(ctx, run); err != nil {
		t.Fatal(err)
	}
	if run.wantedBooks[version.ID][books[0].ID] || !run.wantedBooks[version.ID][books[1].ID] {
		t.Fatalf("replacement would redownload confirmed books: %+v", run.wantedBooks)
	}
	pending, err := st.PendingBooks(ctx, sid)
	if err != nil || !pending[store.BookKey(version.ID, books[0].ID)] || !pending[store.BookKey(version.ID, books[1].ID)] {
		t.Fatalf("review reservations were lost: %v %v", pending, err)
	}
	// Transfers still take precedence: an upgrade is visibly in progress.
	if err = st.UpdateQueueItem(ctx, qid, store.QueueDownloading, ""); err != nil {
		t.Fatal(err)
	}
	view, err = lib.BooksByVersion(ctx, sid)
	if err != nil || view[version.ID][0].State != StatePending {
		t.Fatalf("active transfer hidden: %+v %v", view, err)
	}
}

func TestActivityArchivesGoneSourcesWithoutHidingFileQuestions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	lib, st, sid, e, books, v := modelFixture(t)
	refs := []store.BookRef{modelRef(e, books[0], v)}
	qid, _, err := st.ReserveQueue(ctx, store.QueueItem{SeriesID: sid, Protocol: "source", ReleaseKey: "gone-release", Title: "Gone volume"}, refs)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.UpdateQueueItem(ctx, qid, store.QueueFailed, "listing pages: host: HTTP error 404"); err != nil {
		t.Fatal(err)
	}
	if err = st.AddBlocklist(ctx, "source", "gone-release", "HTTP 404"); err != nil {
		t.Fatal(err)
	}
	for _, scope := range []int64{0, sid} {
		data, err := lib.Activity(ctx, scope)
		if err != nil {
			t.Fatal(err)
		}
		d := data.Downloads[0]
		if !d.History || d.Attention || d.Job.CanTryAnother || d.Resolution == "" {
			t.Fatalf("dead download remains a task: %+v", d)
		}
	}
	byVersion, err := lib.BooksByVersion(ctx, sid)
	if err != nil || !byVersion[v.ID][0].Wanted {
		t.Fatal("archiving changed wanted intent", byVersion, err)
	}
	job, err := st.GetQueueItem(ctx, qid)
	if err != nil || job.Status != store.QueueFailed {
		t.Fatal("page read rewrote the job", job, err)
	}
	path := t.TempDir() + "/partial.cbz"
	if err = os.WriteFile(path, []byte("pages"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = st.AddFile(ctx, store.File{SeriesID: sid, Path: path, NeedsReview: true, Facts: store.AssetFacts{ReleaseKey: "gone-release"}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	data, err := lib.Activity(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	if d := data.Downloads[0]; !d.Attention || d.History || len(d.Files) != 1 {
		t.Fatalf("404 hid an unfinished import: %+v", d)
	}
}
