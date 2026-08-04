package linearapi

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/zen-linear/zen-linear/internal/logger"
)

const (
	defaultMaxAttempts = 3
	defaultBaseDelay   = 250 * time.Millisecond
	defaultMaxDelay    = 4 * time.Second
	// maxRetryAfterWait abandons a retry the server wants delayed past what
	// anyone will sit in front of. Clamping the server's number down instead
	// would retry earlier than we were told, which is worse than not retrying.
	maxRetryAfterWait = 8 * time.Second
	// minRetryBudget keeps a backoff from eating the whole remaining deadline
	// and leaving no room for the request it is waiting to make.
	minRetryBudget = 500 * time.Millisecond
	maxDrainBytes  = 64 << 10
)

// errAuthRefresh marks a token exchange that failed. Replaying the request just
// fails the same way, so the retry layer treats it as terminal.
var errAuthRefresh = errors.New("refresh auth after 401")

// replayableKey marks a context as carrying an operation that is safe to send
// again. Only queries set it. A mutation that draws a 5xx or a dropped
// connection may have been applied already, so a replay would double-apply it.
type replayableKey struct{}

func withReplayable(ctx context.Context) context.Context {
	return context.WithValue(ctx, replayableKey{}, true)
}

func isReplayable(ctx context.Context) bool {
	replayable, _ := ctx.Value(replayableKey{}).(bool)
	return replayable
}

// retryTransport resends a request after a 429 or a transient server or network
// failure. It wraps authTransport rather than living inside it, so every attempt
// re-stamps the token a concurrent refresh may have rotated.
type retryTransport struct {
	base        http.RoundTripper
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration
}

func newRetryTransport(base http.RoundTripper) *retryTransport {
	return &retryTransport{
		base:        base,
		maxAttempts: defaultMaxAttempts,
		baseDelay:   defaultBaseDelay,
		maxDelay:    defaultMaxDelay,
	}
}

// RoundTrip implements http.RoundTripper.
func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	replayable := isReplayable(ctx)

	// Clone once up front: on a request without GetBody this is what installs
	// it, and that is what makes every later attempt possible.
	if _, err := cloneRequestForRetry(req); err != nil {
		return nil, err
	}

	for attempt := 0; ; attempt++ {
		attemptReq, err := cloneRequestForRetry(req)
		if err != nil {
			return nil, err
		}

		resp, err := t.base.RoundTrip(attemptReq)
		if attempt == t.maxAttempts-1 || !shouldRetry(resp, err, replayable) {
			return resp, err
		}
		delay, ok := t.retryDelay(attempt, resp)
		if !ok {
			return resp, err
		}
		if waitErr := sleepBeforeRetry(ctx, delay); waitErr != nil {
			return resp, err
		}

		logger.Debug("linearapi.retry: retrying attempt=%d/%d status=%s wait=%s",
			attempt+1, t.maxAttempts, statusOf(resp), delay)
		drainAndClose(resp)
	}
}

// shouldRetry classifies one attempt. A 429 is safe for anything: the rate
// limiter rejects before the resolver runs, so nothing was applied.
func shouldRetry(resp *http.Response, err error, replayable bool) bool {
	if err != nil {
		if errors.Is(err, errAuthRefresh) || errors.Is(err, context.Canceled) ||
			errors.Is(err, context.DeadlineExceeded) {
			return false
		}
		return replayable
	}
	if resp == nil {
		return false
	}
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return true
	case resp.StatusCode >= 500:
		return replayable
	default:
		return false
	}
}

// retryDelay picks how long to wait, reporting false when the wait is long
// enough that giving up beats holding the request.
func (t *retryTransport) retryDelay(attempt int, resp *http.Response) (time.Duration, bool) {
	if resp != nil {
		if wait, ok := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()); ok {
			if wait > maxRetryAfterWait {
				return 0, false
			}
			return wait, true
		}
	}
	return t.nextBackoff(attempt), true
}

// nextBackoff returns the delay before the attempt after this one. Equal jitter
// keeps a burst of requests from re-synchronizing on every retry without
// letting the delay collapse to zero.
func (t *retryTransport) nextBackoff(attempt int) time.Duration {
	delay := t.baseDelay << attempt
	if delay > t.maxDelay || delay <= 0 {
		delay = t.maxDelay
	}
	half := delay / 2
	if half <= 0 {
		return delay
	}
	return half + rand.N(half)
}

// parseRetryAfter reads both forms RFC 9110 allows: delta-seconds and an
// HTTP-date. A value of 0 is legal and means retry now.
func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds < 0 {
			return 0, false
		}
		return time.Duration(seconds) * time.Second, true
	}
	when, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	if wait := when.Sub(now); wait >= 0 {
		return wait, true
	}
	return 0, false
}

// sleepBeforeRetry waits out the backoff, reporting an error when the wait is
// not worth taking.
func sleepBeforeRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	// http.Client.Timeout lands on this context as a deadline, so a retry
	// spends the same budget as the request. Sleeping past it would turn a
	// readable 503 into a context-deadline error.
	if deadline, ok := ctx.Deadline(); ok && time.Now().Add(delay+minRetryBudget).After(deadline) {
		return errors.New("backoff would exceed the request deadline")
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// drainAndClose frees the connection for reuse; an unread body pins it and the
// next attempt opens a fresh one.
func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDrainBytes))
	_ = resp.Body.Close()
}

func statusOf(resp *http.Response) string {
	if resp == nil {
		return "network error"
	}
	return resp.Status
}
