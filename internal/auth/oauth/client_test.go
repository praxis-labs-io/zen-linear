package oauth_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/roeyazroel/linear-tui/internal/auth/oauth"
)

func TestExchangeCode_RequestShape(t *testing.T) {
	t.Parallel()

	var gotContentType string
	var gotForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		gotForm, _ = url.ParseQuery(string(body))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"access_token":"access-1",
			"token_type":"Bearer",
			"expires_in":3600,
			"scope":"read,write",
			"refresh_token":"refresh-1"
		}`))
	}))
	defer server.Close()

	client := oauth.NewClient(oauth.ClientConfig{
		ClientID:   "client-1",
		HTTPClient: server.Client(),
		TokenURL:   server.URL,
	})

	token, err := client.ExchangeCode(context.Background(), "code-1", "http://127.0.0.1:53682/callback", "verifier-1")
	if err != nil {
		t.Fatalf("ExchangeCode() error: %v", err)
	}
	if token.AccessToken != "access-1" || token.RefreshToken != "refresh-1" {
		t.Fatalf("unexpected token: %+v", token)
	}
	if gotContentType != "application/x-www-form-urlencoded" {
		t.Fatalf("Content-Type = %q", gotContentType)
	}
	if gotForm.Get("grant_type") != "authorization_code" {
		t.Fatalf("grant_type = %q", gotForm.Get("grant_type"))
	}
	if gotForm.Get("code") != "code-1" {
		t.Fatalf("code = %q", gotForm.Get("code"))
	}
	if gotForm.Get("client_id") != "client-1" {
		t.Fatalf("client_id = %q", gotForm.Get("client_id"))
	}
	if gotForm.Get("code_verifier") != "verifier-1" {
		t.Fatalf("code_verifier = %q", gotForm.Get("code_verifier"))
	}
}

func TestRefresh_RequestShape(t *testing.T) {
	t.Parallel()

	var gotForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotForm, _ = url.ParseQuery(string(body))
		_, _ = w.Write([]byte(`{"access_token":"a2","token_type":"Bearer","expires_in":100,"scope":"read","refresh_token":"r2"}`))
	}))
	defer server.Close()

	client := oauth.NewClient(oauth.ClientConfig{
		ClientID:   "client-1",
		HTTPClient: server.Client(),
		TokenURL:   server.URL,
	})
	token, err := client.Refresh(context.Background(), "old-refresh")
	if err != nil {
		t.Fatalf("Refresh() error: %v", err)
	}
	if token.RefreshToken != "r2" {
		t.Fatalf("refresh_token = %q", token.RefreshToken)
	}
	if gotForm.Get("grant_type") != "refresh_token" {
		t.Fatalf("grant_type = %q", gotForm.Get("grant_type"))
	}
	if gotForm.Get("refresh_token") != "old-refresh" {
		t.Fatalf("refresh_token = %q", gotForm.Get("refresh_token"))
	}
}

func TestRevoke_ErrorStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`already revoked`))
	}))
	defer server.Close()

	client := oauth.NewClient(oauth.ClientConfig{
		ClientID:   "client-1",
		HTTPClient: server.Client(),
		RevokeURL:  server.URL,
	})
	err := client.Revoke(context.Background(), "token", "refresh_token")
	if err == nil {
		t.Fatal("expected revoke error")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Fatalf("error = %v", err)
	}
}

func TestDefaultClientID(t *testing.T) {
	t.Parallel()
	if oauth.DefaultClientID != "ea40a3da4d4511d43a97ce7691dc315d" {
		t.Fatalf("DefaultClientID = %q", oauth.DefaultClientID)
	}
}
