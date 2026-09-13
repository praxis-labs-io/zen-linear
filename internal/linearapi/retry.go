package linearapi

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/praxis-labs-io/zen-linear/internal/logger"
)

const (
	defaultMaxAttempts = 3
	defaultBaseDelay   = 250 * time.Millisecond
	defaultMaxDelay    = 4 * time.Second
	maxRetryAfterWait  = 8 * time.Second
	retryAfterJitter   = 250 * time.Millisecond
	minRetryBudget     = 500 * time.Millisecond
	maxDrainBytes      = 64 << 10
)

var errAuthRefresh = errors.New("refresh auth after 401")

// A mutation that drew a 5xx may already have applied, so only queries set this.
type replayableKey struct{}

func withReplayable(ctx context.Context) context.Context {
	return context.WithValue(ctx, replayableKey{}, true)
}

func isReplayable(ctx context.Context) bool {
	replayable, _ := ctx.Value(replayableKey{}).(bool)
	return replayable
}

// Wraps authTransport so every attempt re-stamps a token a refresh may have rotated.
type retryTransport struct {
	base        http.RoundTripper
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration
	limits      rateLimitTracker
}

func newRetryTransport(base http.RoundTripper) *retryTransport {
	return &retryTransport{
		base:        base,
		maxAttempts: defaultMaxAttempts,
		baseDelay:   defaultBaseDelay,
		maxDelay:    defaultMaxDelay,
	}
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	replayable := isReplayable(ctx)

	if _, err := cloneRequestForRetry(req); err != nil {
		return nil, err
	}

	for attempt := 0; ; attempt++ {
		attemptReq, err := cloneRequestForRetry(req)
		if err != nil {
			return nil, err
		}

		resp, err := t.base.RoundTrip(attemptReq)
		snap, named, limited := t.observe(resp)
		if attempt == t.maxAttempts-1 || !shouldRetry(resp, err, replayable, limited) {
			return resp, err
		}
		delay, ok := t.retryDelay(attempt, resp, snap, limited && named)
		if !ok || !fitsInDeadline(ctx, delay) {
			return resp, err
		}

		logger.Debug("linearapi.retry: retrying attempt=%d/%d status=%s wait=%s",
			attempt+1, t.maxAttempts, statusOf(resp), delay)
		drainAndClose(resp)
		if waitErr := sleepBeforeRetry(ctx, delay); waitErr != nil {
			return nil, waitErr
		}
	}
}

func (t *retryTransport) observe(resp *http.Response) (snap RateLimitSnapshot, named, limited bool) {
	if resp == nil {
		return RateLimitSnapshot{}, false, false
	}
	snap, named = t.limits.record(resp.Header, time.Now())
	if !isRateLimited(resp) {
		return snap, named, false
	}
	refill, _ := snap.wait(time.Now())
	logger.Warning("linearapi.retry: rate limited status=%s requests=%d/%d complexity=%d/%d refill=%s",
		resp.Status, snap.Requests.Remaining, snap.Requests.Limit,
		snap.Complexity.Remaining, snap.Complexity.Limit, refill)
	return snap, named, true
}

func shouldRetry(resp *http.Response, err error, replayable, limited bool) bool {
	if !replayable {
		return false
	}
	if err != nil {
		return isTransient(err)
	}
	if resp == nil {
		return false
	}
	return limited || resp.StatusCode >= 500
}

func isTransient(err error) bool {
	if errors.Is(err, errAuthRefresh) || errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return false
	}
	var certErr *tls.CertificateVerificationError
	return !errors.As(err, &certErr)
}

func (t *retryTransport) retryDelay(attempt int, resp *http.Response, snap RateLimitSnapshot, limited bool) (time.Duration, bool) {
	if resp != nil {
		if wait, ok := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()); ok {
			if wait > maxRetryAfterWait {
				return 0, false
			}
			return wait + rand.N(retryAfterJitter), true
		}
		if limited {
			if wait, ok := snap.wait(time.Now()); ok {
				if wait > maxRetryAfterWait {
					return 0, false
				}
				return wait + rand.N(retryAfterJitter), true
			}
		}
	}
	return t.nextBackoff(attempt), true
}

func fitsInDeadline(ctx context.Context, delay time.Duration) bool {
	deadline, ok := ctx.Deadline()
	if !ok {
		return true
	}
	return !time.Now().Add(delay + minRetryBudget).After(deadline)
}

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

func sleepBeforeRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
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
