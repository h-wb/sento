package extensions

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSourceHTTPError(t *testing.T) {
	for _, tc := range []struct {
		name, body, header string
		status             int
		retry              bool
		delay              time.Duration
	}{
		{"throttled", `{"error":"HTTP error 429","upstreamStatus":429,"retryable":true,"retryAfter":"120"}`, "", 429, true, 120 * time.Second},
		{"unauthorized", `{"error":"HTTP error 401","upstreamStatus":401,"retryable":false}`, "", 401, false, 0},
		{"unknown permanent", `{"error":"bad archive","retryable":false}`, "", 502, false, 0},
		{"invalid archive range", `{"error":"Not a local file header","retryable":true,"unreadable":false}`, "", 502, true, 0},
		{"legacy gateway", `{"error":"unavailable"}`, "60", 502, true, time.Minute},
		{"proxy gateway", "bad gateway", "", 502, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", tc.header)
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			client := NewClient(srv.URL)
			// Metadata and streaming page requests must carry the same error.
			_, metaErr := client.Pages(context.Background(), "src", "chapter")
			_, pageErr := client.Page(context.Background(), "src", "ref")
			for _, err := range []error{metaErr, pageErr} {
				var source *HTTPError
				if !errors.As(err, &source) || source.StatusCode != tc.status || source.Retryable != tc.retry || source.RetryAfter != tc.delay {
					t.Fatalf("source error: %#v (%v)", source, err)
				}
			}
		})
	}
}

func TestRetryAfter(t *testing.T) {
	now := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		value string
		want  time.Duration
	}{
		{" 30 ", 30 * time.Second}, {"-1", 0}, {"invalid", 0},
		{now.Add(time.Hour).Format(http.TimeFormat), time.Hour},
		{now.Add(-time.Hour).Format(http.TimeFormat), 0},
		{"9223372036854775807", time.Duration((1<<63-1)/int64(time.Second)) * time.Second},
	} {
		if got := parseRetryAfter(tc.value, now); got != tc.want {
			t.Errorf("%q: got %s, want %s", tc.value, got, tc.want)
		}
	}
}
