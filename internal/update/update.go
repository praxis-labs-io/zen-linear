// Package update reports whether a newer release has been published. It only
// ever answers that question: nothing here downloads or replaces a binary,
// because re-running the installer already upgrades correctly and a self-update
// would fight go install, make install, and whatever else owns the file.
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
	// LatestReleaseURL is the release GitHub calls latest. It excludes
	// pre-releases by GitHub's own definition, which is what the release
	// workflow's --prerelease on a suffixed tag buys us: somebody on a stable
	// build is never nudged toward an rc.
	LatestReleaseURL = "https://api.github.com/repos/praxis-labs-io/zen-linear/releases/latest"

	// DefaultTTL keeps this to one request a day rather than one a launch,
	// which also holds it well clear of the 60/hour unauthenticated rate limit.
	DefaultTTL = 24 * time.Hour

	// requestTimeout bounds the call. It runs off the UI thread, so this is
	// about not holding a goroutine and a connection open, not about latency
	// anybody waits on.
	requestTimeout = 5 * time.Second

	// devVersion is what a build that was not stamped by the release workflow
	// reports. A working tree is not behind anything.
	devVersion = "dev"

	// maxBodyBytes caps what is read off the wire. The release payload is a
	// few kilobytes and only one field of it is wanted.
	maxBodyBytes = 1 << 20
)

// Result is what a check found. Latest is empty when there was no answer, so a
// caller has nothing to print rather than a zero version to reason about.
type Result struct {
	Latest    string
	Available bool
}

// Options is what a check needs. Only Current is required; the rest have
// defaults, and Endpoint and Client exist so tests do not reach GitHub.
type Options struct {
	// Current is the running build's version, as main.Version reports it.
	Current string
	// CachePath is where the last answer is kept. An empty path skips the
	// cache in both directions and asks every time.
	CachePath string
	// TTL is how long a cached answer stands. Zero means DefaultTTL.
	TTL time.Duration
	// Endpoint overrides the release URL. Zero means LatestReleaseURL.
	Endpoint string
	// Client overrides the HTTP client. Zero means one bounded by
	// requestTimeout.
	Client *http.Client
	// Now overrides the clock, for testing the TTL without waiting it out.
	Now func() time.Time
}

// Check reports whether a release newer than Current has been published,
// answering from the cache while it is fresh and asking GitHub when it is not.
//
// An error means the check did not complete, never that the user should be
// told something. Every caller logs it and moves on: an update nudge that
// reports its own failure is worse than one that stays quiet.
func Check(ctx context.Context, opts Options) (Result, error) {
	// A build the release workflow never stamped is a working tree, which is
	// what make install and go build produce by design. It is not behind, and
	// asking on its behalf spends a request to be told so.
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

	// A tag that cannot be ranked is not written: caching it would hold a
	// useless answer for a day, where re-asking costs one request.
	if _, ok := parseVersion(tag); !ok {
		return Result{}, fmt.Errorf("unrecognized release tag %q", tag)
	}

	if opts.CachePath != "" {
		if err := recordCache(opts.CachePath, tag, now()); err != nil {
			// The answer is good even when it could not be kept. Report it so
			// the caller can log the path problem, and hand back the result.
			return resultFor(opts.Current, tag), err
		}
	}

	return resultFor(opts.Current, tag), nil
}

// resultFor pairs a tag with whether the running build is behind it.
func resultFor(current, tag string) Result {
	return Result{Latest: tag, Available: isNewer(current, tag)}
}

// fetchLatestTag reads the latest release's tag. GitHub answers 403 without a
// User-Agent, so the running version goes in one: it is the only thing this
// sends, and no token, workspace, or identifier goes with it.
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
