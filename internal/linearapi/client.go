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
	DefaultEndpoint = "https://api.linear.app/graphql"
)

type ClientConfig struct {
	Token string
	// UseBearer prefixes the token with "Bearer ". Personal API keys leave it false.
	UseBearer bool
	// OnUnauthorized refreshes the token after a 401. The request is retried once.
	OnUnauthorized func(ctx context.Context) (string, error)
	// Endpoint empty means DefaultEndpoint.
	Endpoint   string
	HTTPClient *http.Client
	// Timeout zero means 30s.
	Timeout time.Duration
}

type Client struct {
	httpClient *http.Client
	endpoint   string
	token      string
	client     *gqlClient
	limits     *rateLimitTracker
}

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
		httpClient = cfg.HTTPClient
		if httpClient.Transport == nil {
			httpClient.Transport = http.DefaultTransport
		}
		transport.Base = httpClient.Transport
		httpClient.Transport = retry
	} else {
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
		limits:     &retry.limits,
	}
}

// Queries are marked replayable and mutations are not, so no write is resent.
type gqlClient struct {
	inner *graphql.Client
}

func (g *gqlClient) query(ctx context.Context, q interface{}, variables map[string]interface{}) error {
	return g.inner.Query(withReplayable(ctx), q, variables)
}

func (g *gqlClient) mutate(ctx context.Context, m interface{}, variables map[string]interface{}) error {
	return g.inner.Mutate(ctx, m, variables)
}

func NewClientWithToken(token string) *Client {
	return NewClient(ClientConfig{Token: token})
}

// The refresh ignores the request's cancellation: abandoning a token rotation logs the user out.
const refreshTimeout = 30 * time.Second

type authTransport struct {
	mu             sync.Mutex
	Token          string
	UseBearer      bool
	OnUnauthorized func(ctx context.Context) (string, error)
	Base           http.RoundTripper
}

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

// RateLimit returns what the last answered request said about the budgets.
func (c *Client) RateLimit() RateLimitSnapshot {
	if c.limits == nil {
		return RateLimitSnapshot{}
	}
	return c.limits.snapshot()
}

func (c *Client) Endpoint() string {
	return c.endpoint
}
