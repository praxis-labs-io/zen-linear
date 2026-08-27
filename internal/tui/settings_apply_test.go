package tui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/praxis-labs-io/zen-linear/internal/config"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
)

// TestApplySettingsPreservesOAuthBearer guards the bug where an in-app settings
// save rebuilt the API client from config alone, dropping the OAuth bearer
// scheme and downgrading the session to raw-token auth Linear then rejects.
func TestApplySettingsPreservesOAuthBearer(t *testing.T) {
	var mu sync.Mutex
	var projectsAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		var request struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		query := strings.ToLower(request.Query)

		var data any
		switch {
		case strings.Contains(query, "viewer"):
			data = map[string]any{"viewer": map[string]any{
				"id": "user-1", "name": "Test User", "displayName": "Test User", "email": "test@example.com",
			}}
		case strings.Contains(query, "teams"):
			data = map[string]any{"teams": map[string]any{"nodes": []any{
				map[string]any{"id": "team-2", "key": "NEX", "name": "Nexa"},
			}}}
		case strings.Contains(query, "favorites"):
			data = map[string]any{"favorites": map[string]any{
				"nodes":    []any{},
				"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""},
			}}
		case strings.Contains(query, "projects"):
			mu.Lock()
			projectsAuth = auth
			mu.Unlock()
			data = map[string]any{"team": map[string]any{"projects": map[string]any{"nodes": []any{
				map[string]any{"id": "proj-1", "name": "Website"},
			}}}}
		case strings.Contains(query, "states"):
			data = map[string]any{"team": map[string]any{"states": map[string]any{"nodes": []any{}}}}
		case strings.Contains(query, "cycles"):
			data = map[string]any{"team": map[string]any{"cycles": map[string]any{
				"nodes":    []any{},
				"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""},
			}}}
		case strings.Contains(query, "issues"):
			data = map[string]any{"issues": map[string]any{
				"nodes":    []any{},
				"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""},
			}}
		default:
			data = map[string]any{}
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"data": data}); err != nil {
			t.Errorf("encode GraphQL response: %v", err)
		}
	}))
	defer server.Close()

	cfg := config.Config{
		APIEndpoint:  server.URL,
		LinearAPIKey: "oauth-token",
		CacheTTL:     time.Minute,
		PageSize:     10,
	}
	app := NewApp(linearapi.ClientConfig{
		Token:     "oauth-token",
		Endpoint:  server.URL,
		UseBearer: true,
	}, cfg, nil)
	startReviewTestApplication(t, app)
	refreshDone := installRefreshCompletionHook(app)

	app.applySettings(cfg)

	if _, err := app.fetchProjectsFunc(context.Background(), "team-2"); err != nil {
		t.Fatalf("fetchProjectsFunc() error: %v", err)
	}

	mu.Lock()
	got := projectsAuth
	mu.Unlock()
	if got != "Bearer oauth-token" {
		t.Fatalf("Authorization after settings save = %q, want %q", got, "Bearer oauth-token")
	}
	waitForRefreshCompletion(t, refreshDone)
}

// A log path the app cannot open used to abort applySettings: logger.Reinit
// returned an error, the handler reported it and returned early, and everything
// after it — the rebuilt API client included — never ran. Saving a bad log path
// took the rest of the settings with it.
func TestApplySettingsSurvivesAnUnwritableLogPath(t *testing.T) {
	app := newUXTestApp(t)
	logger.Close()

	tmpDir := t.TempDir()
	// A regular file where the refused path wants a directory.
	blocker := filepath.Join(tmpDir, "blocker")
	if err := os.WriteFile(blocker, nil, 0644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	refused := filepath.Join(blocker, "nested", "app.log")

	// The API client is rebuilt after the logger line, so the old pointer
	// surviving is what an early return there looks like from outside.
	before := app.api

	cfg := app.config
	cfg.APIEndpoint = "https://example.invalid/graphql"
	cfg.LogFile = refused
	app.applySettings(cfg)

	if app.api == before {
		t.Error("API client not rebuilt: applySettings stopped at the refused log path")
	}

	// The config names where logs actually go, not the path that was refused.
	if app.config.LogFile == refused {
		t.Errorf("config.LogFile = %q, want the path actually opened", app.config.LogFile)
	}
	if !strings.Contains(app.statusMessage, refused) {
		t.Errorf("status %q does not name the refused path %q", app.statusMessage, refused)
	}
}
