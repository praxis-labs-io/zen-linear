package linearapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const teamsOK = `{"data":{"teams":{"nodes":[]}}}`

const commentOK = `{"data":{"commentCreate":{"success":true,"comment":{"id":"comment-1","body":"hi"}}}}`

// callRecorder counts handler calls and what each one carried. The handler runs
// on the server's goroutine, so the mutex is not optional.
type callRecorder struct {
	mu      sync.Mutex
	calls   int
	headers []string
	times   []time.Time
	remotes []string
}

func (r *callRecorder) record(req *http.Request) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.headers = append(r.headers, req.Header.Get("Authorization"))
	r.times = append(r.times, time.Now())
	r.remotes = append(r.remotes, req.RemoteAddr)
	return r.calls
}

func (r *callRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func (r *callRecorder) authHeaders() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.headers...)
}

func (r *callRecorder) gap() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.times) < 2 {
		return 0
	}
	return r.times[1].Sub(r.times[0])
}

func (r *callRecorder) distinctRemotes() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	seen := make(map[string]struct{}, len(r.remotes))
	for _, remote := range r.remotes {
		seen[remote] = struct{}{}
	}
	return len(seen)
}

// newFastRetryClient builds the production transport chain with the backoff
// shrunk so a retry test finishes in milliseconds.
func newFastRetryClient(t *testing.T, cfg ClientConfig) *Client {
	t.Helper()
	client := NewClient(cfg)
	retry, ok := client.httpClient.Transport.(*retryTransport)
	if !ok {
		t.Fatalf("transport = %T, want *retryTransport", client.httpClient.Transport)
	}
	retry.baseDelay = time.Millisecond
	retry.maxDelay = 5 * time.Millisecond
	return client
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		value string
		want  time.Duration
		ok    bool
	}{
		{name: "delta seconds", value: "1", want: time.Second, ok: true},
		{name: "zero means now", value: "0", want: 0, ok: true},
		{name: "padded", value: " 2 ", want: 2 * time.Second, ok: true},
		{name: "negative", value: "-5"},
		{name: "empty", value: ""},
		{name: "garbage", value: "soon"},
		{
			name:  "rfc1123 ahead",
			value: now.Add(3 * time.Second).Format(http.TimeFormat),
			want:  3 * time.Second,
			ok:    true,
		},
		{name: "date in the past", value: now.Add(-time.Minute).Format(http.TimeFormat)},
		{
			name:  "rfc850",
			value: now.Add(time.Minute).Format("Monday, 02-Jan-06 15:04:05 MST"),
			want:  time.Minute,
			ok:    true,
		},
		{
			name:  "ansic",
			value: now.Add(time.Minute).Format(time.ANSIC),
			want:  time.Minute,
			ok:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseRetryAfter(tt.value, now)
			if ok != tt.ok {
				t.Fatalf("parseRetryAfter(%q) ok = %v, want %v", tt.value, ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Fatalf("parseRetryAfter(%q) = %s, want %s", tt.value, got, tt.want)
			}
		})
	}
}

func TestRetryTransportBackoff(t *testing.T) {
	transport := &retryTransport{baseDelay: 100 * time.Millisecond, maxDelay: 400 * time.Millisecond}
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 0, want: 100 * time.Millisecond},
		{attempt: 1, want: 200 * time.Millisecond},
		{attempt: 2, want: 400 * time.Millisecond},
		{attempt: 3, want: 400 * time.Millisecond},
		{attempt: 40, want: 400 * time.Millisecond}, // the shift overflows
	}

	for _, tt := range tests {
		got := transport.nextBackoff(tt.attempt)
		if got < tt.want/2 || got >= tt.want {
			t.Fatalf("nextBackoff(%d) = %s, want [%s, %s)", tt.attempt, got, tt.want/2, tt.want)
		}
	}
}

func TestRetryTransportRetriesOn5xxThenSucceeds(t *testing.T) {
	var recorder callRecorder
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if recorder.record(r) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(teamsOK))
	}))
	defer server.Close()

	client := newFastRetryClient(t, ClientConfig{Endpoint: server.URL})
	if _, err := client.ListTeams(context.Background()); err != nil {
		t.Fatalf("ListTeams() error: %v", err)
	}
	if got := recorder.count(); got != 3 {
		t.Fatalf("handler calls = %d, want 3", got)
	}
}

func TestRetryTransportStopsAtMaxAttempts(t *testing.T) {
	var recorder callRecorder
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder.record(r)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := newFastRetryClient(t, ClientConfig{Endpoint: server.URL})
	_, err := client.ListTeams(context.Background())
	if err == nil {
		t.Fatal("ListTeams() succeeded against a server that only returns 503")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Fatalf("error = %v, want the 503 to survive to the caller", err)
	}
	if got := recorder.count(); got != defaultMaxAttempts {
		t.Fatalf("handler calls = %d, want %d", got, defaultMaxAttempts)
	}
}

func TestRetryTransportRetriesOnNetworkError(t *testing.T) {
	var recorder callRecorder
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if recorder.record(r) == 1 {
			// net/http does not replay a POST on its own, so any second call
			// here is ours.
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Error("test server does not support hijacking")
				return
			}
			conn, _, err := hijacker.Hijack()
			if err != nil {
				t.Errorf("hijack: %v", err)
				return
			}
			_ = conn.Close()
			return
		}
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

// TestRetryTransportDoesNotRetryMutationOn5xx is the duplicate-comment guard. A
// 5xx may mean the write landed and only the reply was lost.
func TestRetryTransportDoesNotRetryMutationOn5xx(t *testing.T) {
	var recorder callRecorder
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder.record(r)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := newFastRetryClient(t, ClientConfig{Endpoint: server.URL})
	_, err := client.CreateComment(context.Background(), CreateCommentInput{IssueID: "issue-1", Body: "hi"})
	if err == nil {
		t.Fatal("CreateComment() succeeded against a server that only returns 503")
	}
	if got := recorder.count(); got != 1 {
		t.Fatalf("handler calls = %d, want 1: a mutation must not be resent after a 5xx", got)
	}
}

func TestRetryTransportRetriesMutationOn429(t *testing.T) {
	var recorder callRecorder
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if recorder.record(r) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(commentOK))
	}))
	defer server.Close()

	client := newFastRetryClient(t, ClientConfig{Endpoint: server.URL})
	// A rate limiter rejects before the resolver runs, so the write never
	// happened and replaying it is safe.
	if _, err := client.CreateComment(context.Background(), CreateCommentInput{IssueID: "issue-1", Body: "hi"}); err != nil {
		t.Fatalf("CreateComment() error: %v", err)
	}
	if got := recorder.count(); got != 2 {
		t.Fatalf("handler calls = %d, want 2", got)
	}
}

func TestRetryTransportDoesNotRetryClientErrors(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var recorder callRecorder
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				recorder.record(r)
				w.WriteHeader(status)
			}))
			defer server.Close()

			client := newFastRetryClient(t, ClientConfig{Endpoint: server.URL})
			if _, err := client.ListTeams(context.Background()); err == nil {
				t.Fatalf("ListTeams() succeeded against a %d", status)
			}
			if got := recorder.count(); got != 1 {
				t.Fatalf("handler calls = %d, want 1", got)
			}
		})
	}
}

func TestRetryTransportHonorsRetryAfterOverBackoff(t *testing.T) {
	var recorder callRecorder
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if recorder.record(r) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(teamsOK))
	}))
	defer server.Close()

	client := newFastRetryClient(t, ClientConfig{Endpoint: server.URL})
	if _, err := client.ListTeams(context.Background()); err != nil {
		t.Fatalf("ListTeams() error: %v", err)
	}
	if gap := recorder.gap(); gap < time.Second {
		t.Fatalf("gap between attempts = %s, want at least the 1s the server asked for", gap)
	}
}

func TestRetryTransportAbandonsLongRetryAfter(t *testing.T) {
	var recorder callRecorder
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder.record(r)
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := newFastRetryClient(t, ClientConfig{Endpoint: server.URL})
	start := time.Now()
	if _, err := client.ListTeams(context.Background()); err == nil {
		t.Fatal("ListTeams() succeeded against a server that only returns 429")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("took %s: a minute-long Retry-After must be abandoned, not waited out", elapsed)
	}
	if got := recorder.count(); got != 1 {
		t.Fatalf("handler calls = %d, want 1", got)
	}
}

func TestRetryTransportStopsWhenBackoffExceedsDeadline(t *testing.T) {
	var recorder callRecorder
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder.record(r)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	// The default 250ms backoff plus the reserve does not fit in a 200ms
	// budget, so the 503 reaches the caller instead of a deadline error.
	client := NewClient(ClientConfig{Endpoint: server.URL, Timeout: 200 * time.Millisecond})
	start := time.Now()
	_, err := client.ListTeams(context.Background())
	if err == nil {
		t.Fatal("ListTeams() succeeded against a server that only returns 503")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Fatalf("error = %v, want the 503 rather than a deadline error", err)
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("took %s, want a return before the 200ms budget ran out", elapsed)
	}
	if got := recorder.count(); got != 1 {
		t.Fatalf("handler calls = %d, want 1", got)
	}
}

func TestRetryTransportAbortsOnContextCancel(t *testing.T) {
	var recorder callRecorder
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder.record(r)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewClient(ClientConfig{Endpoint: server.URL})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	if _, err := client.ListTeams(ctx); err == nil {
		t.Fatal("ListTeams() succeeded after its context was canceled")
	}
	if got := recorder.count(); got != 1 {
		t.Fatalf("handler calls = %d, want 1: the backoff must abort on cancellation", got)
	}
}

// TestRetryTransportRefreshesAuthHeaderPerAttempt fails if the retry loop ever
// moves inside authTransport, which would replay a credential the refresh
// rotated.
func TestRetryTransportRefreshesAuthHeaderPerAttempt(t *testing.T) {
	var recorder callRecorder
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch recorder.record(r) {
		case 1:
			w.WriteHeader(http.StatusServiceUnavailable)
		case 2:
			w.WriteHeader(http.StatusUnauthorized)
		default:
			_, _ = w.Write([]byte(teamsOK))
		}
	}))
	defer server.Close()

	client := newFastRetryClient(t, ClientConfig{
		Token:     "tok-a",
		UseBearer: true,
		Endpoint:  server.URL,
		OnUnauthorized: func(context.Context) (string, error) {
			return "tok-b", nil
		},
	})
	if _, err := client.ListTeams(context.Background()); err != nil {
		t.Fatalf("ListTeams() error: %v", err)
	}

	want := []string{"Bearer tok-a", "Bearer tok-a", "Bearer tok-b"}
	got := recorder.authHeaders()
	if len(got) != len(want) {
		t.Fatalf("authorization headers = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("authorization headers = %v, want %v", got, want)
		}
	}
}

func TestRetryTransportDoesNotRetryFailedRefresh(t *testing.T) {
	var recorder callRecorder
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder.record(r)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	var refreshMu sync.Mutex
	refreshes := 0
	client := newFastRetryClient(t, ClientConfig{
		Token:     "tok-a",
		UseBearer: true,
		Endpoint:  server.URL,
		OnUnauthorized: func(context.Context) (string, error) {
			refreshMu.Lock()
			refreshes++
			refreshMu.Unlock()
			return "", errors.New("refresh endpoint down")
		},
	})
	if _, err := client.ListTeams(context.Background()); err == nil {
		t.Fatal("ListTeams() succeeded with a dead refresh endpoint")
	}

	refreshMu.Lock()
	defer refreshMu.Unlock()
	if refreshes != 1 {
		t.Fatalf("refresh attempts = %d, want 1: a failed refresh fails the same way every time", refreshes)
	}
}

// TestRetryTransportReusesConnectionAcrossAttempts is how a drained body shows
// up in behavior: an abandoned one pins the connection and the retry opens a
// fresh socket.
func TestRetryTransportReusesConnectionAcrossAttempts(t *testing.T) {
	var recorder callRecorder
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if recorder.record(r) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"errors":[{"message":"upstream unavailable"}]}`))
			return
		}
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

// TestFetchIssueByIDReportsCancellation pins the error a canceled detail fetch
// returns. The details pane cancels one on every superseded selection, so the
// caller has to be able to tell that apart from a real failure.
func TestFetchIssueByIDReportsCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("a canceled fetch still reached the server")
	}))
	defer server.Close()

	client := NewClient(ClientConfig{Endpoint: server.URL})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.FetchIssueByID(ctx, "issue-1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("FetchIssueByID() error = %v, want a wrapped context.Canceled", err)
	}
}

func TestNewClientCustomHTTPClientRetries(t *testing.T) {
	var recorder callRecorder
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if recorder.record(r) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(teamsOK))
	}))
	defer server.Close()

	client := newFastRetryClient(t, ClientConfig{
		Endpoint:   server.URL,
		HTTPClient: &http.Client{},
	})
	if _, err := client.ListTeams(context.Background()); err != nil {
		t.Fatalf("ListTeams() error: %v", err)
	}
	if got := recorder.count(); got != 2 {
		t.Fatalf("handler calls = %d, want 2: a caller-supplied client still gets the retry chain", got)
	}
}
