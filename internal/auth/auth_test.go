package auth_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/roeyazroel/linear-tui/internal/auth"
	"github.com/roeyazroel/linear-tui/internal/auth/oauth"
)

func TestGeneratePKCE(t *testing.T) {
	t.Parallel()

	verifier, challenge, err := auth.GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE() error: %v", err)
	}
	if verifier == "" || challenge == "" {
		t.Fatal("expected non-empty verifier and challenge")
	}
	sum := sha256.Sum256([]byte(verifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	if challenge != want {
		t.Fatalf("challenge = %q, want %q", challenge, want)
	}

	v2, c2, err := auth.GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE() second error: %v", err)
	}
	if verifier == v2 || challenge == c2 {
		t.Fatal("expected unique pkce values")
	}
}

func TestGenerateState(t *testing.T) {
	t.Parallel()

	s1, err := auth.GenerateState()
	if err != nil {
		t.Fatalf("GenerateState() error: %v", err)
	}
	s2, err := auth.GenerateState()
	if err != nil {
		t.Fatalf("GenerateState() second error: %v", err)
	}
	if s1 == "" || s1 == s2 {
		t.Fatalf("unexpected states %q %q", s1, s2)
	}
}

func TestCredentialsStoreRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.json")
	creds := auth.Credentials{
		AccessToken:  "access",
		RefreshToken: "refresh",
		TokenType:    "Bearer",
		Scope:        "read,write",
		ExpiresAt:    time.Now().UTC().Add(time.Hour).Truncate(time.Second),
		UpdatedAt:    time.Now().UTC().Truncate(time.Second),
	}
	if err := auth.SaveCredentials(path, creds); err != nil {
		t.Fatalf("SaveCredentials() error: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions = %o, want 0600", info.Mode().Perm())
	}

	loaded, err := auth.LoadCredentials(path)
	if err != nil {
		t.Fatalf("LoadCredentials() error: %v", err)
	}
	if loaded.AccessToken != creds.AccessToken || loaded.RefreshToken != creds.RefreshToken {
		t.Fatalf("loaded = %+v", loaded)
	}

	if err := auth.DeleteCredentials(path); err != nil {
		t.Fatalf("DeleteCredentials() error: %v", err)
	}
	if err := auth.DeleteCredentials(path); err != nil {
		t.Fatalf("DeleteCredentials() idempotent error: %v", err)
	}
	if _, err := auth.LoadCredentials(path); !errors.Is(err, auth.ErrCredentialsNotFound) {
		t.Fatalf("LoadCredentials() after delete = %v", err)
	}
}

func TestLoadCredentialsCorruptJSON(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.LoadCredentials(path); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestResolveAPIKeyWins(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "credentials.json")
	_ = auth.SaveCredentials(path, auth.Credentials{
		AccessToken:  "oauth-access",
		RefreshToken: "oauth-refresh",
		ExpiresAt:    time.Now().Add(time.Hour),
	})
	resolved, err := auth.Resolve(context.Background(), "env-key", path, oauth.NewClient(oauth.ClientConfig{ClientID: "x"}))
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if resolved.Source != auth.TokenSourceAPIKey || resolved.Token != "env-key" {
		t.Fatalf("resolved = %+v", resolved)
	}
}

func TestResolveMissingBoth(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "missing.json")
	_, err := auth.Resolve(context.Background(), "", path, oauth.NewClient(oauth.ClientConfig{ClientID: "x"}))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "auth login") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveRefreshesNearExpiry(t *testing.T) {
	t.Parallel()

	var gotRefresh string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		values, _ := url.ParseQuery(string(body))
		gotRefresh = values.Get("refresh_token")
		_, _ = w.Write([]byte(`{"access_token":"new-access","token_type":"Bearer","expires_in":3600,"scope":"read,write","refresh_token":"new-refresh"}`))
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "credentials.json")
	old := auth.Credentials{
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
		ExpiresAt:    time.Now().Add(time.Minute),
		UpdatedAt:    time.Now().Add(-time.Hour),
	}
	if err := auth.SaveCredentials(path, old); err != nil {
		t.Fatal(err)
	}

	client := oauth.NewClient(oauth.ClientConfig{
		ClientID:   "client",
		HTTPClient: server.Client(),
		TokenURL:   server.URL,
	})
	resolved, err := auth.Resolve(context.Background(), "", path, client)
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if resolved.Token != "new-access" || resolved.Source != auth.TokenSourceOAuth {
		t.Fatalf("resolved = %+v", resolved)
	}
	if gotRefresh != "old-refresh" {
		t.Fatalf("refresh_token sent = %q", gotRefresh)
	}
	loaded, err := auth.LoadCredentials(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RefreshToken != "new-refresh" {
		t.Fatalf("persisted refresh = %q", loaded.RefreshToken)
	}
}

func TestLoginHappyPath(t *testing.T) {
	t.Parallel()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"login-access","token_type":"Bearer","expires_in":3600,"scope":"read,write","refresh_token":"login-refresh"}`))
	}))
	defer tokenServer.Close()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listenAddr := ln.Addr().String()
	_ = ln.Close()

	redirectURI := "http://" + listenAddr + "/callback"
	storePath := filepath.Join(t.TempDir(), "credentials.json")
	_ = auth.SaveCredentials(storePath, auth.Credentials{AccessToken: "old", RefreshToken: "old", ExpiresAt: time.Now().Add(time.Hour)})

	err = auth.Login(context.Background(), auth.LoginOptions{
		ClientID:     "client",
		StorePath:    storePath,
		RedirectURI:  redirectURI,
		ListenAddr:   listenAddr,
		AuthorizeURL: "https://example.test/oauth/authorize",
		Timeout:      5 * time.Second,
		OAuthClient: oauth.NewClient(oauth.ClientConfig{
			ClientID:   "client",
			HTTPClient: tokenServer.Client(),
			TokenURL:   tokenServer.URL,
		}),
		OpenBrowser: func(authURL string) error {
			u, err := url.Parse(authURL)
			if err != nil {
				return err
			}
			state := u.Query().Get("state")
			go func() {
				time.Sleep(20 * time.Millisecond)
				resp, err := http.Get(redirectURI + "?code=abc&state=" + url.QueryEscape(state))
				if err == nil {
					_ = resp.Body.Close()
				}
			}()
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Login() error: %v", err)
	}
	loaded, err := auth.LoadCredentials(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AccessToken != "login-access" || loaded.RefreshToken != "login-refresh" {
		t.Fatalf("loaded = %+v", loaded)
	}
}

func TestLoginStateMismatch(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listenAddr := ln.Addr().String()
	_ = ln.Close()
	redirectURI := "http://" + listenAddr + "/callback"
	storePath := filepath.Join(t.TempDir(), "credentials.json")
	prior := auth.Credentials{AccessToken: "keep", RefreshToken: "keep", ExpiresAt: time.Now().Add(time.Hour)}
	if err := auth.SaveCredentials(storePath, prior); err != nil {
		t.Fatal(err)
	}

	err = auth.Login(context.Background(), auth.LoginOptions{
		ClientID:     "client",
		StorePath:    storePath,
		RedirectURI:  redirectURI,
		ListenAddr:   listenAddr,
		AuthorizeURL: "https://example.test/oauth/authorize",
		Timeout:      2 * time.Second,
		OAuthClient:  oauth.NewClient(oauth.ClientConfig{ClientID: "client"}),
		OpenBrowser: func(string) error {
			go func() {
				time.Sleep(20 * time.Millisecond)
				resp, err := http.Get(redirectURI + "?code=abc&state=wrong")
				if err == nil {
					_ = resp.Body.Close()
				}
			}()
			return nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "state mismatch") {
		t.Fatalf("expected state mismatch, got %v", err)
	}
	loaded, err := auth.LoadCredentials(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AccessToken != "keep" {
		t.Fatalf("credentials changed on failure: %+v", loaded)
	}
}

func TestLoginTimeout(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listenAddr := ln.Addr().String()
	_ = ln.Close()

	err = auth.Login(context.Background(), auth.LoginOptions{
		ClientID:     "client",
		StorePath:    filepath.Join(t.TempDir(), "credentials.json"),
		RedirectURI:  "http://" + listenAddr + "/callback",
		ListenAddr:   listenAddr,
		AuthorizeURL: "https://example.test/oauth/authorize",
		Timeout:      50 * time.Millisecond,
		OAuthClient:  oauth.NewClient(oauth.ClientConfig{ClientID: "client"}),
		OpenBrowser:  func(string) error { return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout, got %v", err)
	}
}

func TestLogoutDeletesEvenWhenRevokeFails(t *testing.T) {
	t.Parallel()

	revokeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("nope"))
	}))
	defer revokeServer.Close()

	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := auth.SaveCredentials(path, auth.Credentials{
		AccessToken:  "a",
		RefreshToken: "r",
		ExpiresAt:    time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	err := auth.Logout(context.Background(), auth.LogoutOptions{
		StorePath: path,
		OAuthClient: oauth.NewClient(oauth.ClientConfig{
			ClientID:   "client",
			HTTPClient: revokeServer.Client(),
			RevokeURL:  revokeServer.URL,
		}),
	})
	if err != nil {
		t.Fatalf("Logout() error: %v", err)
	}
	if _, err := auth.LoadCredentials(path); !errors.Is(err, auth.ErrCredentialsNotFound) {
		t.Fatalf("expected credentials removed, got %v", err)
	}
}

func TestEnsureAccessTokenNoRefreshWhenFresh(t *testing.T) {
	t.Parallel()
	creds := auth.Credentials{
		AccessToken:  "a",
		RefreshToken: "r",
		ExpiresAt:    time.Now().Add(time.Hour),
	}
	updated, refreshed, err := auth.EnsureAccessToken(
		context.Background(),
		creds,
		oauth.NewClient(oauth.ClientConfig{ClientID: "c"}),
		time.Now(),
		5*time.Minute,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed {
		t.Fatal("expected no refresh")
	}
	if updated.AccessToken != "a" {
		t.Fatalf("updated = %+v", updated)
	}
}
