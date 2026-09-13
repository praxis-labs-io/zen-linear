package update

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type releaseServer struct {
	*httptest.Server
	calls     int
	userAgent string
}

func newReleaseServer(t *testing.T, status int, body string) *releaseServer {
	t.Helper()
	rs := &releaseServer{}
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rs.calls++
		rs.userAgent = r.Header.Get("User-Agent")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(rs.Close)
	return rs
}

func tagBody(tag string) string {
	encoded, err := json.Marshal(map[string]string{"tag_name": tag})
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func TestCheckReportsANewerRelease(t *testing.T) {
	server := newReleaseServer(t, http.StatusOK, tagBody("v0.4.0"))

	got, err := Check(context.Background(), Options{
		Current:   "0.3.0",
		CachePath: filepath.Join(t.TempDir(), cacheFileName),
		Endpoint:  server.URL,
	})
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if !got.Available {
		t.Error("0.3.0 against a published v0.4.0 reported nothing available")
	}
	if got.Latest != "v0.4.0" {
		t.Errorf("Latest = %q, want v0.4.0", got.Latest)
	}
	if server.userAgent != "zen-linear/0.3.0" {
		t.Errorf("User-Agent = %q, want zen-linear/0.3.0", server.userAgent)
	}
}

func TestCheckSaysNothingWhenCurrent(t *testing.T) {
	server := newReleaseServer(t, http.StatusOK, tagBody("v0.3.0"))

	got, err := Check(context.Background(), Options{
		Current:   "0.3.0",
		CachePath: filepath.Join(t.TempDir(), cacheFileName),
		Endpoint:  server.URL,
	})
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if got.Available {
		t.Error("the running version reported an update against itself")
	}
}

func TestCheckNeverAsksForAnUnstampedBuild(t *testing.T) {
	for _, current := range []string{"dev", ""} {
		t.Run(current, func(t *testing.T) {
			server := newReleaseServer(t, http.StatusOK, tagBody("v9.9.9"))

			got, err := Check(context.Background(), Options{
				Current:   current,
				CachePath: filepath.Join(t.TempDir(), cacheFileName),
				Endpoint:  server.URL,
			})
			if err != nil {
				t.Fatalf("Check() error: %v", err)
			}
			if got.Available || got.Latest != "" {
				t.Errorf("an unstamped build got %+v, want nothing", got)
			}
			if server.calls != 0 {
				t.Errorf("server saw %d calls, want none", server.calls)
			}
		})
	}
}

func TestCheckReportsNothingToShowOnAFailure(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "a rate limit", status: http.StatusForbidden, body: `{"message":"rate limit"}`},
		{name: "not found", status: http.StatusNotFound, body: `{"message":"Not Found"}`},
		{name: "a server error", status: http.StatusInternalServerError, body: ""},
		{name: "a malformed body", status: http.StatusOK, body: "not json at all"},
		{name: "no tag in the payload", status: http.StatusOK, body: `{}`},
		{name: "a tag that will not rank", status: http.StatusOK, body: tagBody("nightly")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newReleaseServer(t, tt.status, tt.body)
			cachePath := filepath.Join(t.TempDir(), cacheFileName)

			got, err := Check(context.Background(), Options{
				Current:   "0.3.0",
				CachePath: cachePath,
				Endpoint:  server.URL,
			})
			if err == nil {
				t.Fatal("a failed lookup returned no error")
			}
			if got.Available || got.Latest != "" {
				t.Errorf("got %+v, want nothing to show", got)
			}
			if _, statErr := os.Stat(cachePath); !os.IsNotExist(statErr) {
				t.Error("a failed lookup wrote a cache file")
			}
		})
	}
}

func TestCheckAnswersFromAFreshCache(t *testing.T) {
	server := newReleaseServer(t, http.StatusOK, tagBody("v0.4.0"))
	cachePath := filepath.Join(t.TempDir(), cacheFileName)
	now := time.Now()

	if err := recordCache(cachePath, "v0.5.0", now.Add(-time.Hour)); err != nil {
		t.Fatalf("recordCache() error: %v", err)
	}

	got, err := Check(context.Background(), Options{
		Current:   "0.3.0",
		CachePath: cachePath,
		Endpoint:  server.URL,
		Now:       func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if got.Latest != "v0.5.0" {
		t.Errorf("Latest = %q, want the cached v0.5.0", got.Latest)
	}
	if server.calls != 0 {
		t.Errorf("server saw %d calls, want the cache to have answered", server.calls)
	}
}

func TestCheckAsksAgainOnceTheCacheIsStale(t *testing.T) {
	server := newReleaseServer(t, http.StatusOK, tagBody("v0.4.0"))
	cachePath := filepath.Join(t.TempDir(), cacheFileName)
	now := time.Now()

	if err := recordCache(cachePath, "v0.5.0", now.Add(-25*time.Hour)); err != nil {
		t.Fatalf("recordCache() error: %v", err)
	}

	got, err := Check(context.Background(), Options{
		Current:   "0.3.0",
		CachePath: cachePath,
		Endpoint:  server.URL,
		Now:       func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if got.Latest != "v0.4.0" {
		t.Errorf("Latest = %q, want the served v0.4.0", got.Latest)
	}
	if server.calls != 1 {
		t.Errorf("server saw %d calls, want 1", server.calls)
	}

	if cached := loadCache(cachePath); cached.LatestTag != "v0.4.0" {
		t.Errorf("cached tag = %q, want the served v0.4.0", cached.LatestTag)
	}
}

func TestAnUpgradeSilencesAStillFreshCache(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), cacheFileName)
	now := time.Now()
	if err := recordCache(cachePath, "v0.4.0", now.Add(-time.Hour)); err != nil {
		t.Fatalf("recordCache() error: %v", err)
	}

	got, err := Check(context.Background(), Options{
		Current:   "0.4.0",
		CachePath: cachePath,
		Now:       func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if got.Available {
		t.Error("a cache naming the version now running still reported an update")
	}
}

func TestCheckWithNoCachePathAsksEveryTime(t *testing.T) {
	server := newReleaseServer(t, http.StatusOK, tagBody("v0.4.0"))

	for i := range 2 {
		if _, err := Check(context.Background(), Options{
			Current:  "0.3.0",
			Endpoint: server.URL,
		}); err != nil {
			t.Fatalf("Check() %d error: %v", i, err)
		}
	}
	if server.calls != 2 {
		t.Errorf("server saw %d calls, want 2", server.calls)
	}
}

func TestAStaleSchemaIsIgnored(t *testing.T) {
	server := newReleaseServer(t, http.StatusOK, tagBody("v0.4.0"))
	cachePath := filepath.Join(t.TempDir(), cacheFileName)

	encoded, err := json.Marshal(map[string]any{
		"version":    cacheVersion + 1,
		"checked_at": time.Now(),
		"latest_tag": "v9.9.9",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(cachePath, encoded, 0o600); err != nil {
		t.Fatalf("write cache: %v", err)
	}

	got, err := Check(context.Background(), Options{
		Current:   "0.3.0",
		CachePath: cachePath,
		Endpoint:  server.URL,
	})
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if got.Latest != "v0.4.0" {
		t.Errorf("Latest = %q, want the served v0.4.0 rather than the stale schema's", got.Latest)
	}
}

func TestAnUnreadableCacheIsIgnored(t *testing.T) {
	server := newReleaseServer(t, http.StatusOK, tagBody("v0.4.0"))
	cachePath := filepath.Join(t.TempDir(), cacheFileName)
	if err := os.WriteFile(cachePath, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write cache: %v", err)
	}

	got, err := Check(context.Background(), Options{
		Current:   "0.3.0",
		CachePath: cachePath,
		Endpoint:  server.URL,
	})
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if got.Latest != "v0.4.0" {
		t.Errorf("Latest = %q, want the served v0.4.0", got.Latest)
	}
}

func TestACacheStampedInTheFutureIsNotFresh(t *testing.T) {
	now := time.Now()
	file := cacheFile{Version: cacheVersion, CheckedAt: now.Add(time.Hour), LatestTag: "v0.4.0"}
	if file.fresh(now, DefaultTTL) {
		t.Error("a record stamped in the future reported fresh")
	}
}

func TestAnEmptyRecordIsNotFresh(t *testing.T) {
	now := time.Now()
	if (cacheFile{Version: cacheVersion, CheckedAt: now}).fresh(now, DefaultTTL) {
		t.Error("a record naming no tag reported fresh")
	}
	if (cacheFile{Version: cacheVersion, LatestTag: "v0.4.0"}).fresh(now, DefaultTTL) {
		t.Error("a record with no timestamp reported fresh")
	}
}

func TestCheckHonorsACancelledContext(t *testing.T) {
	server := newReleaseServer(t, http.StatusOK, tagBody("v0.4.0"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := Check(ctx, Options{
		Current:   "0.3.0",
		CachePath: filepath.Join(t.TempDir(), cacheFileName),
		Endpoint:  server.URL,
	})
	if err == nil {
		t.Fatal("a canceled context returned no error")
	}
	if got.Available || got.Latest != "" {
		t.Errorf("got %+v, want nothing to show", got)
	}
}
