package mangabaka

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New(srv.URL)
	c.backoff = time.Millisecond
	return c
}

// One 429 in the middle of a paged works listing used to fail the whole
// add/refresh (and the nightly contract run).
func TestGetWorksRetriesThrottledPages(t *testing.T) {
	var calls, throttled atomic.Int32
	var c *Client
	c = testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		page := r.URL.Query().Get("page")
		if page == "2" && throttled.Add(1) <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		next := "null"
		if page == "" {
			next = fmt.Sprintf("%q", c.baseURL+"/series/377/works?page=2")
		}
		seq := "1"
		if page == "2" {
			seq = "2"
		}
		_, _ = fmt.Fprintf(w, `{"status":200,"data":[{"id":"w%s","sequence_numeric":%s,
			"count_type":"main","collections":[{"id":"col"}]}],"pagination":{"next":%s}}`, seq, seq, next)
	})

	works, err := c.GetWorks(context.Background(), 377)
	if err != nil {
		t.Fatal(err)
	}
	if len(works["col"]) != 2 {
		t.Fatalf("works = %+v, want both pages", works)
	}
	if calls.Load() != 4 {
		t.Fatalf("calls = %d, want 4 (page 1, two throttled tries, page 2)", calls.Load())
	}
}

func TestGetGivesUpAndReportsTheStatus(t *testing.T) {
	var calls atomic.Int32
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	_, err := c.Search(context.Background(), "frieren")
	if err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("err = %v, want the 503 reported", err)
	}
	if calls.Load() != maxAttempts {
		t.Fatalf("calls = %d, want %d", calls.Load(), maxAttempts)
	}
}

func TestGetDoesNotRetryClientErrors(t *testing.T) {
	var calls atomic.Int32
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	})
	if _, err := c.GetSeries(context.Background(), 1); err == nil {
		t.Fatal("want an error for a 404")
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1 (a 404 will not get better)", calls.Load())
	}
}

func TestRetryWaitHonoursRetryAfterWithinTheCap(t *testing.T) {
	c := New("")
	if got := c.retryWait(1, &statusError{status: 429, retryAfter: 7 * time.Second}); got != 7*time.Second {
		t.Errorf("Retry-After 7s: wait = %v", got)
	}
	if got := c.retryWait(1, &statusError{status: 429, retryAfter: time.Hour}); got != maxRetryWait {
		t.Errorf("Retry-After 1h: wait = %v, want the %v cap", got, maxRetryWait)
	}
	if got := c.retryWait(3, &statusError{status: 503}); got != 4*time.Second {
		t.Errorf("third retry without Retry-After: wait = %v, want 4s", got)
	}
}

func TestRetryStopsWhenTheContextEnds(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := c.Search(ctx, "x"); err == nil {
		t.Fatal("want the context error")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("retry wait ignored the context deadline")
	}
}

func TestGetSeriesFollowsMergesButNotInCircles(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/series/1": // merged into 2
			_, _ = w.Write([]byte(`{"data":{"id":1,"merged_with":2}}`))
		case "/series/2":
			_, _ = w.Write([]byte(`{"data":{"id":2,"title":"Frieren","total_chapters":"140"}}`))
		case "/series/10": // 10 and 11 point at each other
			_, _ = w.Write([]byte(`{"data":{"id":10,"merged_with":11}}`))
		case "/series/11":
			_, _ = w.Write([]byte(`{"data":{"id":11,"merged_with":10}}`))
		}
	})
	s, err := c.GetSeries(context.Background(), 1)
	if err != nil || s.MangaBakaID != 2 || s.TotalChapters != 140 {
		t.Fatalf("merged lookup = %+v (err %v), want series 2", s, err)
	}
	if _, err := c.GetSeries(context.Background(), 10); err == nil {
		t.Fatal("want an error for a merge cycle")
	}
}
