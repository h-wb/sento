package provider

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"time"

	"github.com/h-wb/sento/internal/extensions"
)

// DownloadError is an ended source attempt, distinct from a missing or
// dead remote download. The queue can back off or retain it for review.
type DownloadError struct {
	Err        error
	Retryable  bool
	RetryAfter time.Duration
}

func (e *DownloadError) Error() string { return e.Err.Error() }
func (e *DownloadError) Unwrap() error { return e.Err }

func sourceRetry(err error) (bool, time.Duration) {
	if errors.Is(err, context.Canceled) {
		return false, 0
	}
	var httpErr *extensions.HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.Retryable, httpErr.RetryAfter
	}
	var network net.Error
	return errors.As(err, &network) || errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, io.ErrUnexpectedEOF), 0
}

// Every attempt, including retries, passes through the same source-wide
// cooldown and page rate limiter. A throttled page slows its siblings too.
func (p *SourceProvider) waitPage(ctx context.Context, sourceID string) error {
	limiter := p.limiterFor(sourceID)
	for {
		p.mu.Lock()
		delay := time.Until(p.cooldowns[sourceID])
		p.mu.Unlock()
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			continue // a sibling may have extended the cooldown
		}
		if limiter != nil {
			if err := limiter.Wait(ctx); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				// Wait can reject a reservation before ctx expires when
				// the next token would exceed its deadline. That is still
				// a temporary timeout, not a permanent source failure.
				if _, ok := ctx.Deadline(); ok {
					return context.DeadlineExceeded
				}
				return err
			}
		}
		p.mu.Lock()
		cooling := time.Now().Before(p.cooldowns[sourceID])
		p.mu.Unlock()
		if !cooling {
			return ctx.Err()
		}
	}
}

func (p *SourceProvider) retryPage(ctx context.Context, ref, sourceID string) (fetchedPage, error) {
	const attempts = 4
	for attempt := range attempts {
		if err := p.waitPage(ctx, sourceID); err != nil {
			return fetchedPage{}, err
		}
		page, err := p.fetchPage(ctx, ref, sourceID)
		if err == nil {
			return page, nil
		}
		retryable, retryAfter := sourceRetry(err)
		if !retryable {
			return fetchedPage{}, err
		}
		// Positive jitter never shortens Retry-After. Keep the cooldown on
		// exhaustion so other chapters respect it while this one backs off.
		base := time.Second << attempt
		delay := max(retryAfter, base+time.Duration(rand.Int64N(int64(base/4))))
		p.mu.Lock()
		until := time.Now().Add(delay)
		if until.After(p.cooldowns[sourceID]) {
			p.cooldowns[sourceID] = until
		}
		p.mu.Unlock()
		if attempt == attempts-1 {
			return fetchedPage{}, err
		}
	}
	panic("unreachable")
}
