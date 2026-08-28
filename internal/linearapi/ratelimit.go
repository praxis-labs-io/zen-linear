package linearapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// rateLimitedCode is the GraphQL error code Linear returns when a budget is
// spent. It rides on an HTTP 400, so the status alone cannot tell it from a
// malformed query.
const rateLimitedCode = "RATELIMITED"

// maxErrorPeekBytes bounds how much of an error body is read to look for that
// code. A GraphQL error document is small; anything larger is not one.
const maxErrorPeekBytes = 32 << 10

// Linear stamps these on every response. The endpoint-scoped set arrives only
// when a per-endpoint limit is what was hit.
const (
	headerRequestsPrefix   = "X-RateLimit-Requests"
	headerComplexityPrefix = "X-RateLimit-Complexity"
	headerEndpointPrefix   = "X-RateLimit-Endpoint-Requests"
	headerQueryComplexity  = "X-Complexity"
)

// RateLimit is one budget: what it allows, what is left of it, and when the
// window it is counted in ends.
type RateLimit struct {
	Limit     int
	Remaining int
	Reset     time.Time
}

// Known reports whether the server said anything about this budget. A budget it
// never mentioned reads as zero, which is indistinguishable from a spent one.
func (b RateLimit) Known() bool {
	return b.Limit > 0 || !b.Reset.IsZero()
}

// Exhausted is a budget the server named and that has nothing left in it.
func (b RateLimit) Exhausted() bool {
	return b.Known() && b.Remaining <= 0
}

// RateLimitSnapshot is what the last response said about the budgets, plus what
// that one query cost. At is when it was read, so a stale snapshot is visible
// as one rather than read as current.
type RateLimitSnapshot struct {
	Requests   RateLimit
	Complexity RateLimit
	Endpoint   RateLimit
	Cost       int
	At         time.Time
}

// refillsAt is when every spent budget has refilled, which is the latest of
// their resets rather than the soonest: retrying while a second one is still
// empty spends a request to be refused again.
func (s RateLimitSnapshot) refillsAt() (time.Time, bool) {
	var latest time.Time
	for _, budget := range []RateLimit{s.Requests, s.Complexity, s.Endpoint} {
		if !budget.Exhausted() || budget.Reset.IsZero() {
			continue
		}
		if budget.Reset.After(latest) {
			latest = budget.Reset
		}
	}
	return latest, !latest.IsZero()
}

// wait is how long until this snapshot's spent budgets refill. It reports false
// where nothing is spent, which is a rate limit the headers did not explain.
func (s RateLimitSnapshot) wait(now time.Time) (time.Duration, bool) {
	at, ok := s.refillsAt()
	if !ok {
		return 0, false
	}
	if delay := at.Sub(now); delay > 0 {
		return delay, true
	}
	return 0, true
}

// rateLimitTracker holds the last snapshot for a reader that has not been built
// yet. Responses land on whichever goroutine made the request, so the mutex is
// not optional.
type rateLimitTracker struct {
	mu   sync.Mutex
	snap RateLimitSnapshot
}

// record adopts a response's headers. A response carrying none leaves the last
// snapshot standing rather than zeroing it: no news is not an empty budget.
func (t *rateLimitTracker) record(header http.Header, at time.Time) RateLimitSnapshot {
	snap, ok := parseRateLimitSnapshot(header, at)
	if !ok {
		return t.snapshot()
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.snap = snap
	return snap
}

func (t *rateLimitTracker) snapshot() RateLimitSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.snap
}

// parseRateLimitSnapshot reads the budgets off a response, reporting false when
// the response named none of them.
func parseRateLimitSnapshot(header http.Header, at time.Time) (RateLimitSnapshot, bool) {
	if header == nil {
		return RateLimitSnapshot{}, false
	}
	snap := RateLimitSnapshot{
		Requests:   parseRateLimit(header, headerRequestsPrefix),
		Complexity: parseRateLimit(header, headerComplexityPrefix),
		Endpoint:   parseRateLimit(header, headerEndpointPrefix),
		At:         at,
	}
	cost, hasCost := parseHeaderInt(header, headerQueryComplexity)
	snap.Cost = cost
	if !snap.Requests.Known() && !snap.Complexity.Known() && !snap.Endpoint.Known() && !hasCost {
		return RateLimitSnapshot{}, false
	}
	return snap, true
}

func parseRateLimit(header http.Header, prefix string) RateLimit {
	var budget RateLimit
	budget.Limit, _ = parseHeaderInt(header, prefix+"-Limit")
	budget.Remaining, _ = parseHeaderInt(header, prefix+"-Remaining")
	if millis, ok := parseHeaderInt64(header, prefix+"-Reset"); ok && millis > 0 {
		budget.Reset = time.UnixMilli(millis).UTC()
	}
	return budget
}

func parseHeaderInt(header http.Header, name string) (int, bool) {
	value, ok := parseHeaderInt64(header, name)
	return int(value), ok
}

func parseHeaderInt64(header http.Header, name string) (int64, bool) {
	raw := strings.TrimSpace(header.Get(name))
	if raw == "" {
		return 0, false
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

// isRateLimited reports whether the server refused this request for spending.
// A 429 says so in the status; Linear says so in the body of a 400, so that
// case reads the body and puts it back for whoever gets the response.
func isRateLimited(resp *http.Response) bool {
	if resp == nil {
		return false
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if resp.StatusCode != http.StatusBadRequest {
		return false
	}
	return bodyNamesRateLimit(resp)
}

// bodyNamesRateLimit peeks at an error document for the RATELIMITED code. The
// bytes it consumed are pushed back in front of the rest, so the response is
// still the one the caller would have read.
func bodyNamesRateLimit(resp *http.Response) bool {
	if resp.Body == nil {
		return false
	}
	head, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorPeekBytes))
	if err != nil {
		resp.Body = peekedBody{Reader: bytes.NewReader(head), Closer: resp.Body}
		return false
	}
	resp.Body = peekedBody{
		Reader: io.MultiReader(bytes.NewReader(head), resp.Body),
		Closer: resp.Body,
	}
	return namesRateLimit(head)
}

// peekedBody is the body a peek already read, followed by whatever is left of
// it. Close still reaches the connection underneath.
type peekedBody struct {
	io.Reader
	io.Closer
}

// namesRateLimit decodes the GraphQL error envelope rather than matching the
// code anywhere in the bytes: a mutation refused for its own content can echo
// that content back in the message.
func namesRateLimit(body []byte) bool {
	var envelope struct {
		Errors []struct {
			Extensions struct {
				Code string `json:"code"`
				Type string `json:"type"`
			} `json:"extensions"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return false
	}
	for _, e := range envelope.Errors {
		if strings.EqualFold(e.Extensions.Code, rateLimitedCode) ||
			strings.EqualFold(e.Extensions.Type, rateLimitedCode) {
			return true
		}
	}
	return false
}
