package oauth

import (
	"fmt"
	"time"
)

// Linear OAuth endpoints and public-client defaults for linear-tui.
const (
	AuthorizeURL    = "https://linear.app/oauth/authorize"
	TokenURL        = "https://api.linear.app/oauth/token"
	RevokeURL       = "https://api.linear.app/oauth/revoke"
	DefaultClientID = "ea40a3da4d4511d43a97ce7691dc315d"
	RedirectHost    = "127.0.0.1"
	RedirectPort    = 53682
	RedirectPath    = "/callback"
	DefaultScopes   = "read,write"
	LoginTimeout    = 5 * time.Minute
	RefreshSkew     = 5 * time.Minute
)

// RedirectURI returns the fixed loopback callback URL registered with Linear.
func RedirectURI() string {
	return fmt.Sprintf("http://%s:%d%s", RedirectHost, RedirectPort, RedirectPath)
}

// ListenAddr returns the TCP address for the login callback server.
func ListenAddr() string {
	return fmt.Sprintf("%s:%d", RedirectHost, RedirectPort)
}
