package auth

import "time"

type TokenSource string

const (
	TokenSourceAPIKey TokenSource = "api_key"
	TokenSourceOAuth  TokenSource = "oauth"
)

type Credentials struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	Scope        string    `json:"scope"`
	ExpiresAt    time.Time `json:"expires_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ResolvedAuth struct {
	Token     string
	Source    TokenSource
	ExpiresAt *time.Time
}
