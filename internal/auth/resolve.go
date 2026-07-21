package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/roeyazroel/linear-tui/internal/auth/oauth"
	"github.com/roeyazroel/linear-tui/internal/config"
)

// Resolve selects an API token: LINEAR_API_KEY overrides stored OAuth credentials.
func Resolve(ctx context.Context, apiKey string, storePath string, oauthClient *oauth.Client) (ResolvedAuth, error) {
	if apiKey != "" {
		return ResolvedAuth{Token: apiKey, Source: TokenSourceAPIKey}, nil
	}

	creds, err := LoadCredentials(storePath)
	if err != nil {
		if errors.Is(err, ErrCredentialsNotFound) {
			return ResolvedAuth{}, fmt.Errorf("not authenticated: run `linear-tui auth login` or set %s", config.LinearAPIKeyEnv)
		}
		return ResolvedAuth{}, err
	}

	updated, refreshed, err := EnsureAccessToken(ctx, creds, oauthClient, time.Now(), oauth.RefreshSkew, false)
	if err != nil {
		return ResolvedAuth{}, fmt.Errorf("refresh oauth credentials: %w (re-run `linear-tui auth login`)", err)
	}
	if refreshed {
		if err := SaveCredentials(storePath, updated); err != nil {
			return ResolvedAuth{}, fmt.Errorf("save refreshed credentials: %w", err)
		}
	}

	expiresAt := updated.ExpiresAt
	return ResolvedAuth{
		Token:     updated.AccessToken,
		Source:    TokenSourceOAuth,
		ExpiresAt: &expiresAt,
	}, nil
}

// EnsureAccessToken refreshes credentials when force is set or the access token
// expires within skew of now. Returns the (possibly updated) credentials and
// whether a refresh occurred.
func EnsureAccessToken(
	ctx context.Context,
	creds Credentials,
	oauthClient *oauth.Client,
	now time.Time,
	skew time.Duration,
	force bool,
) (Credentials, bool, error) {
	if oauthClient == nil {
		return Credentials{}, false, fmt.Errorf("oauth client is nil")
	}
	if creds.RefreshToken == "" {
		return Credentials{}, false, fmt.Errorf("credentials missing refresh_token")
	}

	needsRefresh := force || !now.Add(skew).Before(creds.ExpiresAt)
	if !needsRefresh {
		return creds, false, nil
	}

	token, err := oauthClient.Refresh(ctx, creds.RefreshToken)
	if err != nil {
		return Credentials{}, false, err
	}
	updated := CredentialsFromTokenResponse(token, now)
	return updated, true, nil
}

// CredentialsFromTokenResponse maps a token endpoint response to stored credentials.
func CredentialsFromTokenResponse(token oauth.TokenResponse, now time.Time) Credentials {
	expiresIn := token.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 86400
	}
	return Credentials{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		Scope:        token.Scope,
		ExpiresAt:    now.Add(time.Duration(expiresIn) * time.Second).UTC(),
		UpdatedAt:    now.UTC(),
	}
}

// NewRefreshFunc returns a callback that force-refreshes stored OAuth credentials.
// Suitable for linearapi unauthorized retry wiring.
func NewRefreshFunc(storePath string, oauthClient *oauth.Client) func(ctx context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		creds, err := LoadCredentials(storePath)
		if err != nil {
			return "", err
		}
		updated, _, err := EnsureAccessToken(ctx, creds, oauthClient, time.Now(), 0, true)
		if err != nil {
			return "", err
		}
		if err := SaveCredentials(storePath, updated); err != nil {
			return "", err
		}
		return updated.AccessToken, nil
	}
}
