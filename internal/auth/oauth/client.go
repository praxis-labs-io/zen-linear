package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TokenResponse is the successful JSON body from Linear's token endpoint.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	RefreshToken string `json:"refresh_token"`
}

// Client exchanges, refreshes, and revokes Linear OAuth tokens.
type Client struct {
	httpClient *http.Client
	clientID   string
	tokenURL   string
	revokeURL  string
}

// ClientConfig configures an OAuth HTTP client.
type ClientConfig struct {
	ClientID   string
	HTTPClient *http.Client
	TokenURL   string
	RevokeURL  string
}

// NewClient creates an OAuth client for Linear token endpoints.
func NewClient(cfg ClientConfig) *Client {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	tokenURL := cfg.TokenURL
	if tokenURL == "" {
		tokenURL = TokenURL
	}
	revokeURL := cfg.RevokeURL
	if revokeURL == "" {
		revokeURL = RevokeURL
	}
	return &Client{
		httpClient: httpClient,
		clientID:   cfg.ClientID,
		tokenURL:   tokenURL,
		revokeURL:  revokeURL,
	}
}

// ExchangeCode trades an authorization code for access and refresh tokens (PKCE).
func (c *Client) ExchangeCode(ctx context.Context, code, redirectURI, codeVerifier string) (TokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", c.clientID)
	form.Set("code_verifier", codeVerifier)
	return c.postToken(ctx, form)
}

// Refresh exchanges a refresh token for a new access (and refresh) token.
func (c *Client) Refresh(ctx context.Context, refreshToken string) (TokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", c.clientID)
	return c.postToken(ctx, form)
}

// Revoke revokes an access or refresh token. hint may be "access_token" or "refresh_token".
func (c *Client) Revoke(ctx context.Context, token, hint string) error {
	form := url.Values{}
	form.Set("token", token)
	if hint != "" {
		form.Set("token_type_hint", hint)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.revokeURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("create revoke request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("revoke token: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// postToken performs a form-encoded token request and decodes the JSON response.
func (c *Client) postToken(ctx context.Context, form url.Values) (TokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return TokenResponse{}, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return TokenResponse{}, fmt.Errorf("read token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return TokenResponse{}, fmt.Errorf("token request: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var token TokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return TokenResponse{}, fmt.Errorf("decode token response: %w", err)
	}
	if token.AccessToken == "" {
		return TokenResponse{}, fmt.Errorf("token response missing access_token")
	}
	return token, nil
}
