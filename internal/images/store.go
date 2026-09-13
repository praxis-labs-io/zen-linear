// Package images fetches and caches the pictures an issue description links to.
package images

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	// Registers the one format keptFormat allows.
	_ "image/png"

	"github.com/praxis-labs-io/zen-linear/internal/config"
)

const (
	requestTimeout = 20 * time.Second
	maxBytes       = 10 << 20
	defaultMaxAge  = 30 * 24 * time.Hour
	dirName        = "images"
)

// The only host the token is sent to; a description can link anywhere.
const uploadHost = "uploads.linear.app"

// ErrUnsupportedHost is returned for a URL outside Linear's upload host.
var ErrUnsupportedHost = errors.New("images: not a Linear upload")

// Kitty's only encoded format (f=100). Anything else would reserve rows for a
// picture the terminal refuses silently.
const keptFormat = "png"

type Image struct {
	Path   string
	Width  int
	Height int
}

type Options struct {
	Token     string
	UseBearer bool
	Host      string
	CacheDir  string
	Client    *http.Client
	MaxAge    time.Duration
	Now       func() time.Time
}

type Store struct {
	token     string
	useBearer bool
	host      string
	dir       string
	client    *http.Client
}

// NewStore creates the cache directory and returns a Store. Only Token is required.
func NewStore(opts Options) (*Store, error) {
	dir := opts.CacheDir
	if dir == "" {
		base, err := config.Dir()
		if err != nil {
			return nil, fmt.Errorf("resolve image cache directory: %w", err)
		}
		dir = filepath.Join(base, dirName)
	}

	if err := os.MkdirAll(dir, config.DirMode); err != nil {
		return nil, fmt.Errorf("create image cache directory: %w", err)
	}
	_ = os.Chmod(dir, config.DirMode)

	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: requestTimeout}
	}

	host := opts.Host
	if host == "" {
		host = uploadHost
	}

	store := &Store{
		token:     opts.Token,
		useBearer: opts.UseBearer,
		host:      host,
		dir:       dir,
		client:    client,
	}

	maxAge := opts.MaxAge
	if maxAge == 0 {
		maxAge = defaultMaxAge
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	store.prune(now(), maxAge)

	return store, nil
}

func (s *Store) Fetch(ctx context.Context, raw string) (Image, error) {
	if err := s.allowed(raw); err != nil {
		return Image{}, err
	}

	path := filepath.Join(s.dir, cacheName(raw))
	if img, ok := s.cached(path); ok {
		return img, nil
	}

	data, err := s.download(ctx, raw)
	if err != nil {
		return Image{}, err
	}

	header, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Image{}, fmt.Errorf("images: read %s: %w", raw, err)
	}
	if format != keptFormat {
		return Image{}, fmt.Errorf("images: %s is %s, and only %s can be drawn", raw, format, keptFormat)
	}

	if err := writeCache(path, data); err != nil {
		return Image{}, err
	}

	return Image{Path: path, Width: header.Width, Height: header.Height}, nil
}

func (s *Store) allowed(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrUnsupportedHost, raw)
	}
	if parsed.Scheme != "https" || parsed.Hostname() != s.host {
		return fmt.Errorf("%w: %s", ErrUnsupportedHost, raw)
	}
	return nil
}

func (s *Store) download(ctx context.Context, raw string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, fmt.Errorf("images: request %s: %w", raw, err)
	}
	if s.token != "" {
		token := s.token
		if s.useBearer {
			token = "Bearer " + token
		}
		req.Header.Set("Authorization", token)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("images: fetch %s: %w", raw, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("images: fetch %s: %s", raw, resp.Status)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("images: read %s: %w", raw, err)
	}
	if len(data) > maxBytes {
		return nil, fmt.Errorf("images: %s is larger than %d bytes", raw, maxBytes)
	}

	return data, nil
}

func (s *Store) cached(path string) (Image, bool) {
	file, err := os.Open(path)
	if err != nil {
		return Image{}, false
	}
	defer func() { _ = file.Close() }()

	header, format, err := image.DecodeConfig(file)
	if err != nil || format != keptFormat {
		return Image{}, false
	}
	return Image{Path: path, Width: header.Width, Height: header.Height}, true
}

func (s *Store) prune(now time.Time, maxAge time.Duration) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > maxAge {
			_ = os.Remove(filepath.Join(s.dir, entry.Name()))
		}
	}
}

// Hashed because an upload URL carries workspace and issue ids.
func cacheName(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func writeCache(path string, data []byte) error {
	if err := config.WriteFileAtomic(path, data, 0o600); err != nil {
		return fmt.Errorf("images: cache %s: %w", path, err)
	}
	return nil
}
