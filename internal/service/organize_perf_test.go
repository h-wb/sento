package service

// Profiling harness for https://github.com/h-wb/sento/issues/56 — not part
// of the standard suite: run with
//
//	go test -run TestOrganizePerfProfile -v ./internal/service
//
// It builds a 270-file series (ONE PIECE-shaped) in a real SQLite store with
// real CBZ files on disk, wraps the SQL driver to count queries, and times
// each phase of organize separately (plan / rename / retag / fingerprint).

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	sqlitedrv "modernc.org/sqlite"

	"github.com/h-wb/sento/internal/db"
	"github.com/h-wb/sento/internal/mediafile"
	"github.com/h-wb/sento/internal/store"
)

// --- query-counting SQL driver -------------------------------------------

type queryStats struct {
	mu    sync.Mutex
	total int
	byOp  map[string]int
}

var reTable = regexp.MustCompile(`(?i)\b(from|into|update|join)\s+([a-z_]+)`)

func (s *queryStats) record(query string) {
	table := "?"
	if m := reTable.FindStringSubmatch(query); m != nil {
		table = strings.ToLower(m[2])
	}
	verb := "?"
	if fields := strings.Fields(strings.TrimSpace(query)); len(fields) > 0 {
		verb = strings.ToUpper(fields[0])
	}
	s.mu.Lock()
	s.total++
	s.byOp[verb+" "+table]++
	s.mu.Unlock()
}

func (s *queryStats) reset() {
	s.mu.Lock()
	s.total = 0
	s.byOp = map[string]int{}
	s.mu.Unlock()
}

func (s *queryStats) report() (int, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]string, 0, len(s.byOp))
	for k := range s.byOp {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return s.byOp[keys[i]] > s.byOp[keys[j]] })
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, fmt.Sprintf("    %-24s × %d", k, s.byOp[k]))
	}
	return s.total, out
}

var profStats = &queryStats{byOp: map[string]int{}}

type countDriver struct {
	base driver.Driver
	st   *queryStats
}

func (d *countDriver) Open(name string) (driver.Conn, error) {
	c, err := d.base.Open(name)
	if err != nil {
		return nil, err
	}
	return &countConn{Conn: c, st: d.st}, nil
}

type countConn struct {
	driver.Conn
	st *queryStats
}

func (c *countConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.st.record(query)
	if q, ok := c.Conn.(driver.QueryerContext); ok {
		return q.QueryContext(ctx, query, args)
	}
	stmt, err := c.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = stmt.Close() }()
	return stmt.(driver.StmtQueryContext).QueryContext(ctx, args)
}

func (c *countConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.st.record(query)
	if e, ok := c.Conn.(driver.ExecerContext); ok {
		return e.ExecContext(ctx, query, args)
	}
	stmt, err := c.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = stmt.Close() }()
	return stmt.(driver.StmtExecContext).ExecContext(ctx, args)
}

func (c *countConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	c.st.record(query)
	if p, ok := c.Conn.(driver.ConnPrepareContext); ok {
		return p.PrepareContext(ctx, query)
	}
	return c.Prepare(query)
}

func init() {
	sql.Register("sqlite-prof", &countDriver{base: &sqlitedrv.Driver{}, st: profStats})
}

// openCountedDB migrates a throwaway SQLite file with the plain driver,
// then reopens it through the counting one.
func openCountedDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "perf.db")
	raw, dialect, err := db.Open("", path)
	if err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)", path)
	conn, err := sql.Open("sqlite-prof", dsn)
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	return conn, dialect
}

// --- fixture ---------------------------------------------------------------

// writeCBZPages writes a CBZ of pages pages of pageSize pseudo-random bytes
// each (stored, not deflated — like real JPEG pages).
func writeCBZPages(t *testing.T, path string, seed uint64, pages, pageSize int) int64 {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	rng := rand.New(rand.NewPCG(seed, 0))
	for i := 0; i < pages; i++ {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: fmt.Sprintf("Book (Digital)/%03d.jpg", i+1), Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		page := make([]byte, pageSize)
		for j := range page {
			page[j] = byte(rng.Uint32())
		}
		if _, err = w.Write(page); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}

// buildBigSeries records a series with n volumes, each a real CBZ on disk
// under <libraryRoot>/incoming, with the fingerprint stored as import/scan
// would have stored it.
func buildBigSeries(t *testing.T, st *store.Store, lib *Library, libraryRoot string, n int) (seriesID, editionID, versionID int64) {
	t.Helper()
	ctx := context.Background()
	var err error
	seriesID, err = lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377})
	if err != nil {
		t.Fatal(err)
	}
	cols, err := st.ListEditions(ctx, seriesID)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cols {
		if c.EditionName == "Standard Edition" {
			editionID = c.ID
		}
	}
	versions, err := st.ListVersions(ctx, seriesID)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range versions {
		if v.EditionID == editionID && v.IsDefault {
			versionID = v.ID
		}
	}
	incoming := filepath.Join(libraryRoot, "incoming")
	if err := os.MkdirAll(incoming, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= n; i++ {
		num := int64(i) * 100
		bid, err := st.SaveBook(ctx, seriesID, store.Book{EditionID: editionID, SequenceNum: num, SequenceStr: strconv.Itoa(i), CountType: "main"})
		if err != nil {
			t.Fatal(err)
		}
		name := fmt.Sprintf("ONE PIECE v%03d.cbz", i)
		path := filepath.Join(incoming, name)
		size := writeCBZPages(t, path, uint64(i), 20, 50*1024) // ~1 MB per book
		// The fingerprint import/scan stores once.
		hash, err := mediafile.Fingerprint(path)
		if err != nil {
			t.Fatal(err)
		}
		facts := store.AssetFacts{Title: name, Group: "ScanGroup", FactsKnown: true, Sources: map[string]string{"language": "filename", "group": "filename", "quality": "filename"}}
		if _, err = st.AddFile(ctx, store.File{
			SeriesID: seriesID, Path: path, Size: size, Provenance: "torrent",
			Facts: facts, Claims: facts, Hash: hash,
			ContentBooks: []store.BookRef{{Ledger: "volume", EditionID: editionID, Number: num, BookID: bid}},
		}, nil, []store.BookLink{{VersionID: versionID, BookID: bid, EditionID: editionID, Ledger: "volume", Number: num}}); err != nil {
			t.Fatal(err)
		}
	}
	return seriesID, editionID, versionID
}

func logQueries(t *testing.T, label string) {
	total, byOp := profStats.report()
	t.Logf("%s: %d queries", label, total)
	for _, line := range byOp {
		t.Log(line)
	}
	profStats.reset()
}

// TestOrganizePlanningQueryCount guards the issue-#56 fix: planning a series
// costs a fixed number of queries, not several per file.
func TestOrganizePlanningQueryCount(t *testing.T) {
	ctx := context.Background()
	conn, dialect := openCountedDB(t)
	st := store.New(conn, dialect)
	lib := NewLibrary(st, testProvider())
	library := t.TempDir()
	if err := SaveMediaSettings(ctx, st, DefaultMediaSettings()); err != nil {
		t.Fatal(err)
	}
	const N = 120
	seriesID, _, _ := buildBigSeries(t, st, lib, library, N)
	im := NewImporter(st, nil, library, nil)

	profStats.reset()
	plans, err := im.PlanOrganize(ctx, seriesID)
	if err != nil {
		t.Fatal(err)
	}
	planQueries, _ := profStats.report()
	profStats.reset()
	t.Logf("plan of %d files: %d queries, %d changes", N, planQueries, len(plans))
	if planQueries > 50 {
		t.Fatalf("planning %d files took %d queries: per-file lookups are back", N, planQueries)
	}

	if _, err := im.Organize(ctx, seriesID, true, false); err != nil {
		t.Fatal(err)
	}
	total, _ := profStats.report()
	profStats.reset()
	t.Logf("rename-only organize of %d files: %d queries", N, total)
	// Apply phase is 2 queries per moved file (path check + location update)
	// plus the constant planning cost.
	if total > 3*N+50 {
		t.Fatalf("organize of %d files took %d queries", N, total)
	}

	// One confirmed file (review confirm) plans without bulk-loading the
	// whole series: a bulk review runs this per file.
	files, err := st.ListFiles(ctx, seriesID)
	if err != nil || len(files) == 0 {
		t.Fatalf("files: %v %v", files, err)
	}
	profStats.reset()
	if _, _, _, err := im.organizePlans(ctx, seriesID, files[0].ID); err != nil {
		t.Fatal(err)
	}
	single, _ := profStats.report()
	profStats.reset()
	t.Logf("plan of one file in a %d-file series: %d queries", N, single)
	if single > 40 {
		t.Fatalf("single-file plan took %d queries: the bulk preload leaked into the per-file path", single)
	}
}

// TestOrganizePerfProfile profiles a 270-file series organize (issue #56).
// Heavy: run explicitly with SENTO_ORGANIZE_PROFILE=1.
func TestOrganizePerfProfile(t *testing.T) {
	if os.Getenv("SENTO_ORGANIZE_PROFILE") == "" {
		t.Skip("set SENTO_ORGANIZE_PROFILE=1 to run the heavy profiling pass")
	}
	ctx := context.Background()
	conn, dialect := openCountedDB(t)
	st := store.New(conn, dialect)
	lib := NewLibrary(st, testProvider())
	library := t.TempDir()
	if err := SaveMediaSettings(ctx, st, DefaultMediaSettings()); err != nil {
		t.Fatal(err)
	}
	const N = 270
	seriesID, _, _ := buildBigSeries(t, st, lib, library, N)
	im := NewImporter(st, nil, library, nil)

	// Phase 1: planning (what GET /series/{id}/files/organize does).
	profStats.reset()
	start := time.Now()
	plans, err := im.PlanOrganize(ctx, seriesID)
	planWall := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	renaming := 0
	for _, p := range plans {
		if p.Rename {
			renaming++
		}
	}
	t.Logf("PLAN: files=%d changes=%d renames=%d wall=%s (%.1f ms/file)", N, len(plans), renaming, planWall, float64(planWall.Milliseconds())/N)
	logQueries(t, "PLAN queries")

	// Phase 2: rename-only organize (hardlink move + DB update per file).
	profStats.reset()
	start = time.Now()
	res, err := im.Organize(ctx, seriesID, true, false)
	renameWall := time.Since(start)
	if err != nil {
		t.Fatalf("organize rename: %v (after %s)", err, renameWall)
	}
	t.Logf("RENAME-ONLY: %+v wall=%s (%.1f ms/file)", res, renameWall, float64(renameWall.Milliseconds())/N)
	logQueries(t, "RENAME-ONLY queries")

	// Phase 3: retag-only organize (full CBZ repack per file, names unchanged).
	profStats.reset()
	start = time.Now()
	res2, err := im.Organize(ctx, seriesID, false, true)
	retagWall := time.Since(start)
	if err != nil {
		t.Fatalf("organize retag: %v (after %s)", err, retagWall)
	}
	t.Logf("RETAG-ONLY: %+v wall=%s (%.1f ms/file)", res2, retagWall, float64(retagWall.Milliseconds())/N)
	logQueries(t, "RETAG-ONLY queries")

	// Phase 4: what a re-fingerprint of every file would cost (pre-#55
	// behavior): clear the stored hashes, change layout so files move
	// again, and organize. organizeFile then hashes each file from disk.
	if _, err := conn.Exec(`UPDATE files SET hash=''`); err != nil {
		t.Fatal(err)
	}
	ms := DefaultMediaSettings()
	ms.BookFolder = BookFolderByID
	if err := SaveMediaSettings(ctx, st, ms); err != nil {
		t.Fatal(err)
	}
	profStats.reset()
	start = time.Now()
	res3, err := im.Organize(ctx, seriesID, true, false)
	refpWall := time.Since(start)
	if err != nil {
		t.Fatalf("organize with empty hashes: %v (after %s)", err, refpWall)
	}
	t.Logf("RENAME with files.hash='' (re-fingerprint): %+v wall=%s (%.1f ms/file)", res3, refpWall, float64(refpWall.Milliseconds())/N)
	logQueries(t, "RENAME+REFINGERPRINT queries")

	// Phase 5: raw fingerprint vs repack cost on one realistic book
	// (~200 pages × 1 MB ≈ 200 MB, the size of a real volume CBZ).
	big := filepath.Join(t.TempDir(), "big.cbz")
	writeCBZPages(t, big, 42, 200, 1024*1024)
	start = time.Now()
	if _, err := mediafile.Fingerprint(big); err != nil {
		t.Fatal(err)
	}
	fpWall := time.Since(start)
	t.Logf("FINGERPRINT one ~200 MB cbz: %s (×%d files = %s)", fpWall, N, fpWall*time.Duration(N))
	start = time.Now()
	if err := mediafile.Process(big, filepath.Join(filepath.Dir(big), "out.cbz"), nil); err != nil {
		t.Fatal(err)
	}
	procWall := time.Since(start)
	t.Logf("PROCESS (repack) one ~200 MB cbz: %s (×%d files = %s)", procWall, N, procWall*time.Duration(N))
}
