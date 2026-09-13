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

// Linear sends this in the body of a 400, not as a 429.
const rateLimitedCode = "RATELIMITED"

const maxErrorPeekBytes = 32 << 10

const (
	headerRequestsPrefix   = "X-RateLimit-Requests"
	headerComplexityPrefix = "X-RateLimit-Complexity"
	headerEndpointPrefix   = "X-RateLimit-Endpoint-Requests"
	headerQueryComplexity  = "X-Complexity"
)

type RateLimit struct {
	Limit     int
	Remaining int
	Reset     time.Time

	remainingSaid bool
}

func (b RateLimit) Known() bool {
	return b.Limit > 0 || !b.Reset.IsZero()
}

// Exhausted reports whether the server counted this budget down to zero. A
// budget with no remaining count is unknown, not spent.
func (b RateLimit) Exhausted() bool {
	return b.Known() && b.remainingSaid && b.Remaining <= 0
}

type RateLimitSnapshot struct {
	Requests   RateLimit
	Complexity RateLimit
	Endpoint   RateLimit
	Cost       int
	At         time.Time
}

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

type rateLimitTracker struct {
	mu   sync.Mutex
	snap RateLimitSnapshot
}

func (t *rateLimitTracker) record(header http.Header, at time.Time) (RateLimitSnapshot, bool) {
	fresh, named, hasCost := parseRateLimitSnapshot(header, at)

	t.mu.Lock()
	defer t.mu.Unlock()
	if !named && !hasCost {
		return t.snap, false
	}
	t.snap.Requests = pickBudget(t.snap.Requests, fresh.Requests)
	t.snap.Complexity = pickBudget(t.snap.Complexity, fresh.Complexity)
	t.snap.Endpoint = pickBudget(t.snap.Endpoint, fresh.Endpoint)
	if hasCost {
		t.snap.Cost = fresh.Cost
	}
	t.snap.At = at
	return t.snap, named
}

func pickBudget(standing, fresh RateLimit) RateLimit {
	if fresh.Known() {
		return fresh
	}
	return standing
}

func (t *rateLimitTracker) snapshot() RateLimitSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.snap
}

func parseRateLimitSnapshot(header http.Header, at time.Time) (snap RateLimitSnapshot, named, hasCost bool) {
	if header == nil {
		return RateLimitSnapshot{}, false, false
	}
	snap = RateLimitSnapshot{
		Requests:   parseRateLimit(header, headerRequestsPrefix),
		Complexity: parseRateLimit(header, headerComplexityPrefix),
		Endpoint:   parseRateLimit(header, headerEndpointPrefix),
		At:         at,
	}
	snap.Cost, hasCost = parseHeaderInt(header, headerQueryComplexity)
	named = snap.Requests.Known() || snap.Complexity.Known() || snap.Endpoint.Known()
	return snap, named, hasCost
}

func parseRateLimit(header http.Header, prefix string) RateLimit {
	var budget RateLimit
	budget.Limit, _ = parseHeaderInt(header, prefix+"-Limit")
	budget.Remaining, budget.remainingSaid = parseHeaderInt(header, prefix+"-Remaining")
	if millis, ok := parseHeaderInt64(header, prefix+"-Reset"); ok && millis > 0 {
		budget.Reset = time.UnixMilli(millis).UTC()
	}
	return budget
}

func parseHeaderInt(header http.Header, name string) (int, bool) {
	raw := strings.TrimSpace(header.Get(name))
	if raw == "" {
		return 0, false
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return value, true
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

type peekedBody struct {
	io.Reader
	io.Closer
}

// Decodes the envelope because a refused mutation can echo its own content back.
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
