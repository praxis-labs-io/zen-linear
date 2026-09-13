// Package update checks for a newer release and runs the published installer.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	LatestReleaseURL = "https://api.github.com/repos/praxis-labs-io/zen-linear/releases/latest"

	DefaultTTL = 24 * time.Hour

	requestTimeout = 5 * time.Second

	devVersion = "dev"

	maxBodyBytes = 1 << 20
)

// Result is what a check found. Latest is empty when there was no answer.
type Result struct {
	Latest    string
	Available bool
}

type Options struct {
	Current string
	// CachePath empty skips the cache in both directions.
	CachePath string
	// TTL zero means DefaultTTL.
	TTL time.Duration
	// Endpoint empty means LatestReleaseURL.
	Endpoint string
	// Client nil means one bounded by requestTimeout.
	Client *http.Client
	Now    func() time.Time
}

// Check reports whether a release newer than Current is published, answering
// from the cache while it is fresh. An empty or dev Current is never checked. A
// failed cache write returns the good result alongside its error.
func Check(ctx context.Context, opts Options) (Result, error) {
	if opts.Current == "" || opts.Current == devVersion {
		return Result{}, nil
	}

	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = DefaultTTL
	}

	if cached := loadCache(opts.CachePath); cached.fresh(now(), ttl) {
		return resultFor(opts.Current, cached.LatestTag), nil
	}

	tag, err := fetchLatestTag(ctx, opts)
	if err != nil {
		return Result{}, err
	}

	if _, ok := parseVersion(tag); !ok {
		return Result{}, fmt.Errorf("unrecognized release tag %q", tag)
	}

	if opts.CachePath != "" {
		if err := recordCache(opts.CachePath, tag, now()); err != nil {
			return resultFor(opts.Current, tag), err
		}
	}

	return resultFor(opts.Current, tag), nil
}

func resultFor(current, tag string) Result {
	return Result{Latest: tag, Available: isNewer(current, tag)}
}

func fetchLatestTag(ctx context.Context, opts Options) (string, error) {
	endpoint := opts.Endpoint
	if endpoint == "" {
		endpoint = LatestReleaseURL
	}
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: requestTimeout}
	}

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("build update request: %w", err)
	}
	req.Header.Set("User-Agent", "zen-linear/"+opts.Current)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request the latest release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the latest release lookup answered %s", resp.Status)
	}

	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(&payload); err != nil {
		return "", fmt.Errorf("parse the latest release: %w", err)
	}
	if payload.TagName == "" {
		return "", fmt.Errorf("the latest release named no tag")
	}

	return payload.TagName, nil
}
