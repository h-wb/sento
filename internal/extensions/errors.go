package extensions

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// HTTPError retains the source failure behind the sidecar's gateway response.
type HTTPError struct {
	StatusCode int
	Message    string
	Retryable  bool
	RetryAfter time.Duration
}

func (e *HTTPError) Error() string {
	if e.Message != "" {
		return "host: " + e.Message
	}
	return fmt.Sprintf("host: HTTP %d", e.StatusCode)
}

func responseError(resp *http.Response) error {
	defer func() { _ = resp.Body.Close() }()
	var wire struct {
		Error          string `json:"error"`
		Retryable      *bool  `json:"retryable"`
		UpstreamStatus int    `json:"upstreamStatus"`
		RetryAfter     string `json:"retryAfter"`
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	_ = json.Unmarshal(raw, &wire)
	status := resp.StatusCode
	if wire.UpstreamStatus > 0 {
		status = wire.UpstreamStatus
	}
	retryable := status == 408 || status == 425 || status == 429 || status >= 500
	if wire.Retryable != nil {
		retryable = *wire.Retryable
	}
	retryAfter := resp.Header.Get("Retry-After")
	if wire.RetryAfter != "" {
		retryAfter = wire.RetryAfter
	}
	return &HTTPError{StatusCode: status, Message: wire.Error, Retryable: retryable,
		RetryAfter: parseRetryAfter(retryAfter, time.Now())}
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds <= 0 {
			return 0
		}
		const maxSeconds = int64((1<<63 - 1) / time.Second)
		return time.Duration(min(seconds, maxSeconds)) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil {
		return max(0, at.Sub(now))
	}
	return 0
}
