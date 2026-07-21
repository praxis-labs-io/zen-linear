package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/roeyazroel/linear-tui/internal/auth/oauth"
)

// LogoutOptions configures credential revocation and local deletion.
type LogoutOptions struct {
	StorePath   string
	OAuthClient *oauth.Client
}

// Logout revokes stored tokens (best effort) and deletes the credentials file.
func Logout(ctx context.Context, opts LogoutOptions) error {
	if opts.StorePath == "" {
		return fmt.Errorf("credentials store path is empty")
	}

	creds, err := LoadCredentials(opts.StorePath)
	if err != nil {
		if errors.Is(err, ErrCredentialsNotFound) {
			return nil
		}
		return err
	}

	if opts.OAuthClient != nil {
		// Prefer revoking the refresh token so the grant is fully invalidated.
		token := creds.RefreshToken
		hint := "refresh_token"
		if token == "" {
			token = creds.AccessToken
			hint = "access_token"
		}
		_ = opts.OAuthClient.Revoke(ctx, token, hint)
	}

	return DeleteCredentials(opts.StorePath)
}
