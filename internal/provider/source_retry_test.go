package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/h-wb/sento/internal/extensions"
)

type retryHost struct {
	fakeHost
	mu    sync.Mutex
	calls map[string]int
	fail  func(string, int) (io.ReadCloser, error)
}

func (h *retryHost) Page(_ context.Context, _, ref string) (io.ReadCloser, error) {
	h.mu.Lock()
	h.calls[ref]++
	n := h.calls[ref]
	h.mu.Unlock()
	return h.fail(ref, n)
}

type truncatedPage struct{ io.Reader }

func (truncatedPage) Close() error { return nil }
func (p truncatedPage) Read(b []byte) (int, error) {
	n, _ := p.Reader.Read(b)
	return n, io.ErrUnexpectedEOF
}

func TestSourcePageRetries(t *testing.T) {
	for _, kind := range []string{"transient", "truncated", "permanent", "exhausted"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			host := &retryHost{calls: map[string]int{}}
			host.fail = func(ref string, n int) (io.ReadCloser, error) {
				if ref == "bad" && (n == 1 || kind == "exhausted") {
					if kind == "truncated" {
						return truncatedPage{strings.NewReader(string(pngHeader))}, nil
					}
					return nil, &extensions.HTTPError{StatusCode: 503, Retryable: kind != "permanent"}
				}
				return io.NopCloser(strings.NewReader(string(pngHeader))), nil
			}
			dir := t.TempDir()
			p := NewSource(host, dir, DownloadConfig{PageConcurrency: 1}, testEntries).(*SourceProvider)
			job := &sourceJob{}
			pages, err := p.fetchPages(context.Background(), "src", []string{"good", "bad"}, job)
			want := 2
			if kind == "permanent" {
				want = 1
			}
			if kind == "exhausted" {
				want = 4
			}
			if host.calls["good"] != 1 || host.calls["bad"] != want {
				t.Fatalf("fetch counts: %v", host.calls)
			}
			if kind == "permanent" || kind == "exhausted" {
				if err == nil {
					t.Fatal("expected failure")
				}
				entries, _ := os.ReadDir(dir)
				if len(entries) != 0 {
					t.Fatalf("failed attempt leaked files: %v", entries)
				}
			} else {
				if err != nil || len(pages) != 2 {
					t.Fatalf("retry failed: %v", err)
				}
				for _, page := range pages {
					_ = os.Remove(page.path)
				}
			}
		})
	}
}

func TestSourceCooldownSharedAndCancellable(t *testing.T) {
	host := &retryHost{calls: map[string]int{}}
	failed := make(chan struct{})
	host.fail = func(ref string, n int) (io.ReadCloser, error) {
		if ref == "throttled" && n == 1 {
			close(failed)
			return nil, &extensions.HTTPError{StatusCode: 429, Retryable: true, RetryAfter: time.Hour}
		}
		return io.NopCloser(strings.NewReader(string(pngHeader))), nil
	}
	p := NewSource(host, t.TempDir(), DownloadConfig{}, testEntries).(*SourceProvider)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := p.retryPage(ctx, "throttled", "src"); done <- err }()
	<-failed
	deadline := time.Now().Add(time.Second)
	for {
		p.mu.Lock()
		until := p.cooldowns["src"]
		p.mu.Unlock()
		if until.After(time.Now().Add(59 * time.Minute)) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Retry-After not shared")
		}
		time.Sleep(time.Millisecond)
	}
	otherCtx, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	_, err := p.retryPage(otherCtx, "sibling", "src")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cooldown: %v", err)
	}
	host.mu.Lock()
	count := host.calls["sibling"]
	host.mu.Unlock()
	if count != 0 {
		t.Fatal("sibling bypassed source cooldown")
	}
	// Other sources remain independent.
	page, err := p.retryPage(context.Background(), "other", "different-source")
	if err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(page.path)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel did not interrupt cooldown")
	}
}

func TestSourceWaitingForSlotDoesNotStall(t *testing.T) {
	p := NewSource(&fakeHost{}, t.TempDir(), DownloadConfig{}, testEntries).(*SourceProvider)
	p.sem <- struct{}{}
	p.sem <- struct{}{}
	id := "src|chapter"
	p.startJob(id)
	state, err := p.State(context.Background(), id)
	if err != nil || !state.InFlight || state.StallEligible || state.Detail != "waiting for download slot" {
		t.Fatalf("waiting slot: %+v, %v", state, err)
	}
	<-p.sem
	<-p.sem
	if !waitDone(t, p, id).Done {
		t.Fatal("slot did not release")
	}
}

func TestSourceFailurePreservesOutstandingCooldown(t *testing.T) {
	p := NewSource(&fakeHost{}, t.TempDir(), DownloadConfig{}, testEntries).(*SourceProvider)
	id := "src|chapter"
	until := time.Now().Add(time.Hour)
	p.jobs[id] = &sourceJob{err: context.DeadlineExceeded}
	p.jobs["unrelated"] = &sourceJob{err: errors.New("another failure")}
	p.cooldowns["src"] = until
	_, err := p.State(context.Background(), id)
	var failure *DownloadError
	if !errors.As(err, &failure) || !failure.Retryable || failure.RetryAfter < 59*time.Minute {
		t.Fatalf("cooldown lost after chapter timeout: %v", err)
	}
	if p.jobs["unrelated"] == nil {
		t.Fatal("reset another chapter's failure")
	}
}

func TestSourceRateLimiterDeadlineIsRetryable(t *testing.T) {
	p := NewSource(&fakeHost{}, t.TempDir(), DownloadConfig{PageRatePerSecond: 1}, testEntries).(*SourceProvider)
	if err := p.waitPage(context.Background(), "src"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := p.waitPage(ctx, "src")
	retryable, _ := sourceRetry(err)
	if !errors.Is(err, context.DeadlineExceeded) || !retryable {
		t.Fatalf("limiter timeout became permanent: %v", err)
	}
}

func TestSourceCooldownDoesNotAppearStalled(t *testing.T) {
	p := NewSource(&fakeHost{}, t.TempDir(), DownloadConfig{}, testEntries).(*SourceProvider)
	id := "src|chapter"
	p.jobs[id] = &sourceJob{running: true, active: true, total: 10, done: 3}
	p.cooldowns["src"] = time.Now().Add(time.Hour)
	state, err := p.State(context.Background(), id)
	if err != nil || !state.InFlight || state.StallEligible || state.Detail != "waiting for source cooldown" || state.Progress != 0.3 {
		t.Fatalf("deliberate cooldown appeared stalled: %+v, %v", state, err)
	}
	// Resume ordinary progress tracking after the cooldown.
	delete(p.cooldowns, "src")
	state, err = p.State(context.Background(), id)
	if err != nil || !state.StallEligible || state.Detail != "downloading" {
		t.Fatalf("cooldown did not expire: %+v, %v", state, err)
	}
}

func TestSourceGone(t *testing.T) {
	for _, tc := range []struct {
		err  error
		gone bool
	}{
		{fmt.Errorf("listing pages: %w", &extensions.HTTPError{StatusCode: 404}), true},
		{&extensions.HTTPError{StatusCode: 410}, true},
		{&extensions.HTTPError{StatusCode: 503, Retryable: true}, false},
		{errors.New("connection reset"), false},
	} {
		if got := sourceGone(tc.err); got != tc.gone {
			t.Errorf("%v: gone = %v", tc.err, got)
		}
	}
}
