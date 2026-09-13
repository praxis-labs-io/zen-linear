package images

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testPNG(t *testing.T, width, height int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	return buf.Bytes()
}

func newTestStore(t *testing.T, handler http.HandlerFunc, opts Options) (*Store, string) {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	endpoint, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}

	if opts.CacheDir == "" {
		opts.CacheDir = t.TempDir()
	}
	opts.Host = endpoint.Hostname()
	opts.Client = &http.Client{Transport: rewriteTo{host: endpoint.Host}}

	store, err := NewStore(opts)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return store, "https://" + endpoint.Host + "/shot.png"
}

type rewriteTo struct {
	host string
}

func (target rewriteTo) RoundTrip(req *http.Request) (*http.Response, error) {
	routed := req.Clone(req.Context())
	routed.URL.Scheme = "http"
	routed.URL.Host = target.host
	return http.DefaultTransport.RoundTrip(routed)
}

func TestFetchStoresTheImageAndItsSize(t *testing.T) {
	var authorization string
	store, endpoint := newTestStore(t, func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(testPNG(t, 320, 180))
	}, Options{Token: "lin_api_test"})

	img, err := store.Fetch(context.Background(), endpoint)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if authorization != "lin_api_test" {
		t.Errorf("Authorization = %q, want the bare token", authorization)
	}
	if img.Width != 320 || img.Height != 180 {
		t.Errorf("size = %dx%d, want 320x180", img.Width, img.Height)
	}
	if _, err := os.Stat(img.Path); err != nil {
		t.Errorf("cached file: %v", err)
	}
}

func TestFetchSendsBearerForOAuth(t *testing.T) {
	var authorization string
	store, endpoint := newTestStore(t, func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		_, _ = w.Write(testPNG(t, 8, 8))
	}, Options{Token: "oauth-token", UseBearer: true})

	if _, err := store.Fetch(context.Background(), endpoint); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if authorization != "Bearer oauth-token" {
		t.Errorf("Authorization = %q, want the bearer form", authorization)
	}
}

func TestFetchRefusesAURLOffTheUploadHost(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
	}))
	t.Cleanup(server.Close)

	endpoint, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	store, err := NewStore(Options{
		Token:    "lin_api_test",
		CacheDir: t.TempDir(),
		Client:   &http.Client{Transport: rewriteTo{host: endpoint.Host}},
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if store.host != uploadHost {
		t.Fatalf("store host = %q, want the real upload host", store.host)
	}

	tests := []struct {
		name string
		url  string
	}{
		{name: "another host", url: "https://example.com/shot.png"},
		{name: "the host as a prefix", url: "https://uploads.linear.app.example.com/shot.png"},
		{name: "the host as a suffix", url: "https://evil-uploads.linear.app/shot.png"},
		{name: "the host in the userinfo", url: "https://uploads.linear.app@example.com/shot.png"},
		{name: "plain http", url: "http://uploads.linear.app/shot.png"},
		{name: "not a URL", url: "https://%zz"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := store.Fetch(context.Background(), test.url)
			if !errors.Is(err, ErrUnsupportedHost) {
				t.Fatalf("Fetch(%q) error = %v, want ErrUnsupportedHost", test.url, err)
			}
		})
	}
	if requests != 0 {
		t.Errorf("%d requests reached the server, want none", requests)
	}
}

func TestFetchAnswersFromTheCacheWithoutAsking(t *testing.T) {
	requests := 0
	store, endpoint := newTestStore(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write(testPNG(t, 40, 20))
	}, Options{Token: "lin_api_test"})

	first, err := store.Fetch(context.Background(), endpoint)
	if err != nil {
		t.Fatalf("first Fetch: %v", err)
	}
	second, err := store.Fetch(context.Background(), endpoint)
	if err != nil {
		t.Fatalf("second Fetch: %v", err)
	}

	if requests != 1 {
		t.Errorf("%d requests, want 1: the second fetch did not use the cache", requests)
	}
	if first != second {
		t.Errorf("cached image = %+v, want the same as the fetched %+v", second, first)
	}
}

func TestFetchRefusesWhatItCannotMeasure(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{
			name: "an error status",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
		},
		{
			name: "something that is not an image",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("<html>not a picture</html>"))
			},
		},
		{
			name: "a JPEG",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "image/jpeg")
				img := image.NewRGBA(image.Rect(0, 0, 16, 16))
				_ = jpeg.Encode(w, img, nil)
			},
		},
		{
			name: "a body past the size cap",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(bytes.Repeat([]byte("x"), maxBytes+1))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, endpoint := newTestStore(t, test.handler, Options{Token: "lin_api_test"})
			if _, err := store.Fetch(context.Background(), endpoint); err == nil {
				t.Fatal("Fetch succeeded, want an error")
			}
			entries, err := os.ReadDir(store.dir)
			if err != nil {
				t.Fatalf("read cache dir: %v", err)
			}
			if len(entries) != 0 {
				t.Errorf("%d files cached, want none", len(entries))
			}
		})
	}
}

func TestNewStorePrunesWhatHasAgedOut(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "stale")
	fresh := filepath.Join(dir, "fresh")
	for _, path := range []string{stale, fresh} {
		if err := os.WriteFile(path, testPNG(t, 4, 4), 0o600); err != nil {
			t.Fatalf("seed %s: %v", path, err)
		}
	}

	now := time.Now()
	if err := os.Chtimes(stale, now.Add(-48*time.Hour), now.Add(-48*time.Hour)); err != nil {
		t.Fatalf("age the stale entry: %v", err)
	}

	if _, err := NewStore(Options{
		CacheDir: dir,
		MaxAge:   24 * time.Hour,
		Now:      func() time.Time { return now },
	}); err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("the stale entry survived: %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("the fresh entry was pruned: %v", err)
	}
}

func TestCacheNameHidesTheURL(t *testing.T) {
	raw := "https://uploads.linear.app/9e76ca0c/229fe13d/1b54e5df"
	name := cacheName(raw)

	if strings.Contains(name, "9e76ca0c") || strings.Contains(name, "linear") {
		t.Errorf("cacheName(%q) = %q, want nothing of the URL in it", raw, name)
	}
	if name == cacheName(raw+"x") {
		t.Error("two URLs share a cache name")
	}
}
