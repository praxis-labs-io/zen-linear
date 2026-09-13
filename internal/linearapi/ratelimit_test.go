package linearapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

const rateLimitedJSON = `{"errors":[{"message":"Rate limit exceeded",` +
	`"extensions":{"code":"RATELIMITED","type":"ratelimited"}}]}`

func epochMillis(at time.Time) string {
	return strconv.FormatInt(at.UnixMilli(), 10)
}

func writeRateLimitHeaders(w http.ResponseWriter, remaining int, reset time.Time) {
	w.Header().Set("X-RateLimit-Requests-Limit", "2500")
	w.Header().Set("X-RateLimit-Requests-Remaining", strconv.Itoa(remaining))
	w.Header().Set("X-RateLimit-Requests-Reset", epochMillis(reset))
	w.Header().Set("X-Complexity", "42")
}

func TestParseRateLimitSnapshot(t *testing.T) {
	reset := time.UnixMilli(1_800_000_000_000).UTC()
	tests := []struct {
		name    string
		header  http.Header
		named   bool
		hasCost bool
		want    RateLimitSnapshot
	}{
		{
			name: "every budget",
			header: http.Header{
				"X-Ratelimit-Requests-Limit":       {"2500"},
				"X-Ratelimit-Requests-Remaining":   {"2499"},
				"X-Ratelimit-Requests-Reset":       {epochMillis(reset)},
				"X-Ratelimit-Complexity-Limit":     {"3000000"},
				"X-Ratelimit-Complexity-Remaining": {"2999000"},
				"X-Ratelimit-Complexity-Reset":     {epochMillis(reset)},
				"X-Complexity":                     {"1000"},
			},
			named:   true,
			hasCost: true,
			want: RateLimitSnapshot{
				Requests:   RateLimit{Limit: 2500, Remaining: 2499, Reset: reset, remainingSaid: true},
				Complexity: RateLimit{Limit: 3000000, Remaining: 2999000, Reset: reset, remainingSaid: true},
				Cost:       1000,
			},
		},
		{
			name: "endpoint budget only arrives when it is hit",
			header: http.Header{
				"X-Ratelimit-Endpoint-Requests-Limit":     {"10"},
				"X-Ratelimit-Endpoint-Requests-Remaining": {"0"},
				"X-Ratelimit-Endpoint-Requests-Reset":     {epochMillis(reset)},
			},
			named: true,
			want:  RateLimitSnapshot{Endpoint: RateLimit{Limit: 10, Reset: reset, remainingSaid: true}},
		},
		{
			name:    "the query cost alone is not a budget",
			header:  http.Header{"X-Complexity": {"1"}},
			hasCost: true,
			want:    RateLimitSnapshot{Cost: 1},
		},
		{
			name:   "a response that named none of them",
			header: http.Header{"Content-Type": {"application/json"}},
		},
		{
			name:   "garbage is not a budget",
			header: http.Header{"X-Ratelimit-Requests-Limit": {"lots"}},
		},
		{name: "no headers at all"},
	}

	at := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, named, hasCost := parseRateLimitSnapshot(tt.header, at)
			if named != tt.named || hasCost != tt.hasCost {
				t.Fatalf("parseRateLimitSnapshot() named=%v hasCost=%v, want named=%v hasCost=%v",
					named, hasCost, tt.named, tt.hasCost)
			}
			if !named && !hasCost {
				return
			}
			tt.want.At = at
			if got != tt.want {
				t.Fatalf("parseRateLimitSnapshot() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRateLimitSnapshotWait(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	soon, later := now.Add(time.Minute), now.Add(time.Hour)

	tests := []struct {
		name string
		snap RateLimitSnapshot
		want time.Duration
		ok   bool
	}{
		{
			name: "the spent budget's own window",
			snap: RateLimitSnapshot{Requests: RateLimit{Limit: 2500, Reset: soon, remainingSaid: true}},
			want: time.Minute,
			ok:   true,
		},
		{
			name: "two spent budgets wait for the later one",
			snap: RateLimitSnapshot{
				Requests:   RateLimit{Limit: 2500, Reset: soon, remainingSaid: true},
				Complexity: RateLimit{Limit: 3000000, Reset: later, remainingSaid: true},
			},
			want: time.Hour,
			ok:   true,
		},
		{
			name: "a budget with room left is not what we are waiting on",
			snap: RateLimitSnapshot{
				Requests:   RateLimit{Limit: 2500, Remaining: 2000, Reset: soon, remainingSaid: true},
				Complexity: RateLimit{Limit: 3000000, Reset: later, remainingSaid: true},
			},
			want: time.Hour,
			ok:   true,
		},
		{
			name: "a window that already closed is no wait at all",
			snap: RateLimitSnapshot{Requests: RateLimit{Limit: 2500, Reset: now.Add(-time.Minute), remainingSaid: true}},
			ok:   true,
		},
		{
			name: "nothing is spent",
			snap: RateLimitSnapshot{Requests: RateLimit{Limit: 2500, Remaining: 2500, Reset: soon, remainingSaid: true}},
		},
		{
			name: "a budget whose remaining count was never sent",
			snap: RateLimitSnapshot{Requests: RateLimit{Limit: 2500, Reset: soon}},
		},
		{name: "the headers explained nothing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.snap.wait(now)
			if ok != tt.ok {
				t.Fatalf("wait() ok = %v, want %v", ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Fatalf("wait() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestNamesRateLimit(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{name: "the documented shape", body: rateLimitedJSON, want: true},
		{
			name: "code alone",
			body: `{"errors":[{"extensions":{"code":"RATELIMITED"}}]}`,
			want: true,
		},
		{
			name: "type alone",
			body: `{"errors":[{"extensions":{"type":"ratelimited"}}]}`,
			want: true,
		},
		{
			name: "a second error carries it",
			body: `{"errors":[{"extensions":{"code":"INVALID_INPUT"}},` +
				`{"extensions":{"code":"RATELIMITED"}}]}`,
			want: true,
		},
		{
			name: "another code entirely",
			body: `{"errors":[{"extensions":{"code":"AUTHENTICATION_ERROR"}}]}`,
		},
		{
			name: "the word echoed back out of a rejected write",
			body: `{"errors":[{"message":"title RATELIMITED is not valid",` +
				`"extensions":{"code":"INVALID_INPUT"}}]}`,
		},
		{name: "not a GraphQL document", body: "Bad Request"},
		{name: "empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := namesRateLimit([]byte(tt.body)); got != tt.want {
				t.Fatalf("namesRateLimit(%q) = %v, want %v", tt.body, got, tt.want)
			}
		})
	}
}

var hugeRateLimitedJSON = `{"errors":[{"message":"` + strings.Repeat("x", maxErrorPeekBytes+8<<10) +
	`","extensions":{"code":"RATELIMITED"}}]}`

type closeRecorder struct {
	io.Reader
	closed bool
}

func (c *closeRecorder) Close() error {
	c.closed = true
	return nil
}

func TestIsRateLimitedLeavesTheBodyReadable(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		detected bool
	}{
		{name: "a rate limit fits in the peek", body: rateLimitedJSON, detected: true},
		{
			name: "a body larger than the peek limit",
			body: hugeRateLimitedJSON,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := &closeRecorder{Reader: strings.NewReader(tt.body)}
			resp := &http.Response{StatusCode: http.StatusBadRequest, Body: body}

			if got := isRateLimited(resp); got != tt.detected {
				t.Fatalf("isRateLimited() = %v, want %v", got, tt.detected)
			}
			got, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("reading the body back: %v", err)
			}
			if len(got) != len(tt.body) || string(got) != tt.body {
				t.Fatalf("body came back %d bytes, want %d", len(got), len(tt.body))
			}
			if err := resp.Body.Close(); err != nil {
				t.Fatalf("Close() error: %v", err)
			}
			if !body.closed {
				t.Fatal("Close did not reach the original body: the connection leaks")
			}
		})
	}
}

func TestRetryTransportReusesConnectionThroughARateLimit(t *testing.T) {
	var recorder callRecorder
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if recorder.record(r) == 1 {
			writeRateLimitHeaders(w, 0, time.Now())
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(rateLimitedJSON))
			return
		}
		writeRateLimitHeaders(w, 2499, time.Now().Add(time.Hour))
		_, _ = w.Write([]byte(teamsOK))
	}))
	defer server.Close()

	client := newFastRetryClient(t, ClientConfig{Endpoint: server.URL})
	if _, err := client.ListTeams(context.Background()); err != nil {
		t.Fatalf("ListTeams() error: %v", err)
	}
	if got := recorder.distinctRemotes(); got != 1 {
		t.Fatalf("distinct client ports = %d, want 1: a retry must reuse the connection", got)
	}
}

func TestRetryTransportRetriesQueryOnRateLimited400(t *testing.T) {
	var recorder callRecorder
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if recorder.record(r) == 1 {
			writeRateLimitHeaders(w, 0, time.Now())
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(rateLimitedJSON))
			return
		}
		writeRateLimitHeaders(w, 2499, time.Now().Add(time.Hour))
		_, _ = w.Write([]byte(teamsOK))
	}))
	defer server.Close()

	client := newFastRetryClient(t, ClientConfig{Endpoint: server.URL})
	if _, err := client.ListTeams(context.Background()); err != nil {
		t.Fatalf("ListTeams() error: %v", err)
	}
	if got := recorder.count(); got != 2 {
		t.Fatalf("handler calls = %d, want 2", got)
	}
}

func TestRetryTransportDoesNotRetryMutationOnRateLimited400(t *testing.T) {
	var recorder callRecorder
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder.record(r)
		writeRateLimitHeaders(w, 0, time.Now())
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(rateLimitedJSON))
	}))
	defer server.Close()

	client := newFastRetryClient(t, ClientConfig{Endpoint: server.URL})
	_, err := client.CreateComment(context.Background(), CreateCommentInput{IssueID: "issue-1", Body: "hi"})
	if err == nil {
		t.Fatal("CreateComment() succeeded against a server that only rate-limits")
	}
	if got := recorder.count(); got != 1 {
		t.Fatalf("handler calls = %d, want 1: a mutation must not be resent after a rate limit", got)
	}
	snap := client.RateLimit()
	if snap.Requests.Limit != 2500 || snap.Requests.Remaining != 0 {
		t.Fatalf("requests budget = %+v, want 0/2500 recorded off the refused mutation", snap.Requests)
	}
	if snap.At.IsZero() {
		t.Fatal("At is zero: the mutation's response was never observed")
	}
}

func TestRetryTransportWaitsForTheResetWindow(t *testing.T) {
	const window = 120 * time.Millisecond
	var recorder callRecorder
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if recorder.record(r) == 1 {
			writeRateLimitHeaders(w, 0, time.Now().Add(window))
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(rateLimitedJSON))
			return
		}
		writeRateLimitHeaders(w, 2499, time.Now().Add(time.Hour))
		_, _ = w.Write([]byte(teamsOK))
	}))
	defer server.Close()

	client := newFastRetryClient(t, ClientConfig{Endpoint: server.URL})
	if _, err := client.ListTeams(context.Background()); err != nil {
		t.Fatalf("ListTeams() error: %v", err)
	}
	if gap := recorder.gap(); gap < window/2 {
		t.Fatalf("gap between attempts = %s, want at least %s", gap, window/2)
	}
}

func TestRetryTransportAbandonsALongResetWindow(t *testing.T) {
	var recorder callRecorder
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder.record(r)
		writeRateLimitHeaders(w, 0, time.Now().Add(time.Hour))
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(rateLimitedJSON))
	}))
	defer server.Close()

	client := newFastRetryClient(t, ClientConfig{Endpoint: server.URL})
	start := time.Now()
	if _, err := client.ListTeams(context.Background()); err == nil {
		t.Fatal("ListTeams() succeeded against a server that only rate-limits")
	}
	if got := recorder.count(); got != 1 {
		t.Fatalf("handler calls = %d, want 1", got)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("took %s: a window an hour out must not be waited on", elapsed)
	}
}

func TestClientRateLimitReportsTheLastAnswer(t *testing.T) {
	reset := time.Now().Add(time.Hour).Truncate(time.Millisecond)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeRateLimitHeaders(w, 2499, reset)
		_, _ = w.Write([]byte(teamsOK))
	}))
	defer server.Close()

	client := newFastRetryClient(t, ClientConfig{Endpoint: server.URL})
	if got := client.RateLimit(); !got.At.IsZero() {
		t.Fatalf("RateLimit() before any request = %+v, want the zero value", got)
	}
	if _, err := client.ListTeams(context.Background()); err != nil {
		t.Fatalf("ListTeams() error: %v", err)
	}

	got := client.RateLimit()
	if got.Requests.Limit != 2500 || got.Requests.Remaining != 2499 {
		t.Fatalf("requests budget = %+v, want 2499/2500", got.Requests)
	}
	if !got.Requests.Reset.Equal(reset) {
		t.Fatalf("reset = %s, want %s", got.Requests.Reset, reset)
	}
	if got.Cost != 42 {
		t.Fatalf("cost = %d, want 42", got.Cost)
	}
	if got.At.IsZero() {
		t.Fatal("At is zero on a snapshot that came off a real response")
	}
}

func TestRateLimitTrackerKeepsTheLastKnownBudget(t *testing.T) {
	var tracker rateLimitTracker
	at := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	tracker.record(http.Header{
		"X-Ratelimit-Requests-Limit":     {"2500"},
		"X-Ratelimit-Requests-Remaining": {"2499"},
	}, at)

	got, named := tracker.record(http.Header{"Content-Type": {"application/json"}}, at.Add(time.Second))
	if named {
		t.Fatal("named = true on a response that mentioned no budget")
	}
	if got.Requests.Remaining != 2499 {
		t.Fatalf("remaining = %d, want the 2499 the last answer named", got.Requests.Remaining)
	}
}

func TestRateLimitTrackerKeepsABudgetTheResponseWasSilentAbout(t *testing.T) {
	var tracker rateLimitTracker
	at := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	reset := at.Add(time.Hour)
	tracker.record(http.Header{
		"X-Ratelimit-Requests-Limit":       {"2500"},
		"X-Ratelimit-Requests-Remaining":   {"2499"},
		"X-Ratelimit-Requests-Reset":       {epochMillis(reset)},
		"X-Ratelimit-Complexity-Limit":     {"3000000"},
		"X-Ratelimit-Complexity-Remaining": {"2999000"},
	}, at)

	got, named := tracker.record(http.Header{
		"X-Ratelimit-Requests-Limit":     {"2500"},
		"X-Ratelimit-Requests-Remaining": {"0"},
		"X-Ratelimit-Requests-Reset":     {epochMillis(reset)},
	}, at.Add(time.Second))

	if !named {
		t.Fatal("named = false on a response that carried the requests budget")
	}
	if got.Requests.Remaining != 0 || !got.Requests.Exhausted() {
		t.Fatalf("requests = %+v, want the spent budget this response named", got.Requests)
	}
	if got.Complexity.Remaining != 2999000 {
		t.Fatalf("complexity = %+v, want the 2999000 the earlier answer named", got.Complexity)
	}
	if got.Complexity.Exhausted() {
		t.Fatal("a budget carried over from an earlier answer reads as spent")
	}
}

func TestParseHeaderIntRefusesAValuePastThePlatformInt(t *testing.T) {
	header := http.Header{"X-Ratelimit-Requests-Remaining": {"9223372036854775808"}}
	if got, ok := parseHeaderInt(header, "X-RateLimit-Requests-Remaining"); ok {
		t.Fatalf("parseHeaderInt() = %d, true; want a refusal rather than a wrap", got)
	}
}

func TestARateLimitNamingNoBudgetBacksOffRatherThanAbandoning(t *testing.T) {
	var recorder callRecorder
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch recorder.record(r) {
		case 1:
			writeRateLimitHeaders(w, 0, time.Now().Add(time.Hour))
			_, _ = w.Write([]byte(teamsOK))
		case 2:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(rateLimitedJSON))
		default:
			_, _ = w.Write([]byte(teamsOK))
		}
	}))
	defer server.Close()

	client := newFastRetryClient(t, ClientConfig{Endpoint: server.URL})
	if _, err := client.ListTeams(context.Background()); err != nil {
		t.Fatalf("seeding call error: %v", err)
	}
	if _, err := client.ListTeams(context.Background()); err != nil {
		t.Fatalf("ListTeams() error: %v", err)
	}
	if got := recorder.count(); got != 3 {
		t.Fatalf("handler calls = %d, want 3: the retry was abandoned against a window "+
			"this response never named", got)
	}
}
