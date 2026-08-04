package linearapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/shurcooL/graphql"
)

const (
	// DefaultEndpoint is the default Linear API GraphQL endpoint.
	DefaultEndpoint = "https://api.linear.app/graphql"
)

// ClientConfig contains configuration for creating a new Linear API client.
type ClientConfig struct {
	// Token is the Linear API key or OAuth access token for authentication.
	Token string
	// UseBearer prefixes the Authorization header with "Bearer " (OAuth tokens).
	// Personal API keys must leave this false.
	UseBearer bool
	// OnUnauthorized optionally refreshes credentials after a 401 and retries once.
	OnUnauthorized func(ctx context.Context) (string, error)
	// Endpoint is the GraphQL API endpoint (defaults to Linear's production endpoint).
	Endpoint string
	// HTTPClient is an optional custom HTTP client (useful for testing).
	HTTPClient *http.Client
	// Timeout is the HTTP request timeout (defaults to 30s).
	Timeout time.Duration
}

// Client is a client for interacting with the Linear GraphQL API.
type Client struct {
	httpClient *http.Client
	endpoint   string
	token      string
	client     *gqlClient
}

// NewClient creates a new Linear API client with the provided configuration.
func NewClient(cfg ClientConfig) *Client {
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	transport := &authTransport{
		Token:          cfg.Token,
		UseBearer:      cfg.UseBearer,
		OnUnauthorized: cfg.OnUnauthorized,
	}
	retry := newRetryTransport(transport)

	var httpClient *http.Client
	if cfg.HTTPClient != nil {
		// Use provided HTTP client but wrap its transport with auth
		httpClient = cfg.HTTPClient
		if httpClient.Transport == nil {
			httpClient.Transport = http.DefaultTransport
		}
		transport.Base = httpClient.Transport
		httpClient.Transport = retry
	} else {
		// Create a new HTTP client
		transport.Base = http.DefaultTransport
		httpClient = &http.Client{
			Timeout:   timeout,
			Transport: retry,
		}
	}

	return &Client{
		httpClient: httpClient,
		endpoint:   endpoint,
		token:      cfg.Token,
		client:     &gqlClient{inner: graphql.NewClient(endpoint, httpClient)},
	}
}

// gqlClient is the package's only door to shurcooL/graphql. query marks the
// context replayable and mutate does not, so no mutation is ever resent after a
// 5xx or a dropped connection.
type gqlClient struct {
	inner *graphql.Client
}

func (g *gqlClient) query(ctx context.Context, q interface{}, variables map[string]interface{}) error {
	return g.inner.Query(withReplayable(ctx), q, variables)
}

func (g *gqlClient) mutate(ctx context.Context, m interface{}, variables map[string]interface{}) error {
	return g.inner.Mutate(ctx, m, variables)
}

// NewClientWithToken creates a new Linear API client with just a token (convenience method).
func NewClientWithToken(token string) *Client {
	return NewClient(ClientConfig{Token: token})
}

// refreshTimeout bounds the post-401 token refresh. It runs detached from the
// triggering request's cancellation, so it needs a deadline of its own.
const refreshTimeout = 30 * time.Second

// authTransport adds the Authorization header to requests and optionally
// refreshes OAuth credentials once after a 401 response.
type authTransport struct {
	mu             sync.Mutex
	Token          string
	UseBearer      bool
	OnUnauthorized func(ctx context.Context) (string, error)
	Base           http.RoundTripper
}

// RoundTrip implements http.RoundTripper.
func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}

	authReq, err := cloneRequestForRetry(req)
	if err != nil {
		return nil, err
	}
	t.setAuthHeader(authReq)

	resp, err := base.RoundTrip(authReq)
	if err != nil || resp == nil || resp.StatusCode != http.StatusUnauthorized || t.OnUnauthorized == nil {
		return resp, err
	}

	_ = resp.Body.Close()

	// A refresh rotates the token server-side and writes the new one to disk.
	// Canceling it part-way leaves the stored credential dead and logs the
	// user out, so it must not inherit the cancellation of whichever request
	// happened to hit the 401. It gets its own deadline instead.
	refreshCtx, cancelRefresh := context.WithTimeout(context.WithoutCancel(req.Context()), refreshTimeout)
	newToken, refreshErr := t.OnUnauthorized(refreshCtx)
	cancelRefresh()
	if refreshErr != nil {
		return nil, fmt.Errorf("%w: %w", errAuthRefresh, refreshErr)
	}

	t.mu.Lock()
	t.Token = newToken
	t.mu.Unlock()

	retryReq, err := cloneRequestForRetry(req)
	if err != nil {
		return nil, err
	}
	t.setAuthHeader(retryReq)
	return base.RoundTrip(retryReq)
}

// setAuthHeader applies the current token to the request Authorization header.
func (t *authTransport) setAuthHeader(req *http.Request) {
	t.mu.Lock()
	token := t.Token
	useBearer := t.UseBearer
	t.mu.Unlock()

	if useBearer {
		req.Header.Set("Authorization", "Bearer "+token)
		return
	}
	req.Header.Set("Authorization", token)
}

// cloneRequestForRetry duplicates req so the body can be resent after a 401 refresh.
func cloneRequestForRetry(req *http.Request) (*http.Request, error) {
	clone := req.Clone(req.Context())
	if req.Body == nil || req.Body == http.NoBody {
		return clone, nil
	}
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, fmt.Errorf("rewind request body: %w", err)
		}
		clone.Body = body
		return clone, nil
	}
	// Fall back to buffering when GetBody is unavailable.
	data, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}
	req.Body = io.NopCloser(strings.NewReader(string(data)))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(string(data))), nil
	}
	clone.Body = io.NopCloser(strings.NewReader(string(data)))
	clone.GetBody = req.GetBody
	clone.ContentLength = int64(len(data))
	return clone, nil
}

// Endpoint returns the GraphQL endpoint being used.
func (c *Client) Endpoint() string {
	return c.endpoint
}
