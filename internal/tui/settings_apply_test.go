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

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/praxis-labs-io/zen-linear/internal/config"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
)

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

	saved := cfg
	saved.Timeout = 45 * time.Second
	app.applySettings(saved)

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

func isolateLogging(t *testing.T) {
	t.Helper()
	setHomeDir(t, t.TempDir())
	t.Cleanup(func() {
		if _, warning := logger.Restart("", "", logger.LevelWarning); warning != "" {
			t.Errorf("restoring the logger: %s", warning)
		}
	})
}

func TestApplySettingsSurvivesAnUnwritableLogPath(t *testing.T) {
	isolateLogging(t)

	app := newUXTestApp(t)

	tmpDir := t.TempDir()
	blocker := filepath.Join(tmpDir, "blocker")
	if err := os.WriteFile(blocker, nil, 0644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	refused := filepath.Join(blocker, "nested", "app.log")

	before := app.api

	cfg := app.config
	cfg.APIEndpoint = "https://example.invalid/graphql"
	cfg.LogFile = refused
	app.applySettings(cfg)

	if app.api == before {
		t.Error("API client not rebuilt: applySettings stopped at the refused log path")
	}

	if app.config.LogFile == refused {
		t.Errorf("config.LogFile = %q, want the path actually opened", app.config.LogFile)
	}
	if !strings.Contains(app.pendingWarning, refused) {
		t.Errorf("held warning %q does not name the refused path %q", app.pendingWarning, refused)
	}

	app.reportPendingWarning()
	if status := app.statusBar.GetText(true); !strings.Contains(status, refused) {
		t.Errorf("status %q does not name the refused path %q", status, refused)
	}
	if app.statusMessage != "" {
		t.Errorf("toast corner = %q, want the warning on the hint line", app.statusMessage)
	}
}

func TestApplySettingsDoesNotAdoptLoggingOffAsASetting(t *testing.T) {
	isolateLogging(t)

	app := newUXTestApp(t)

	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, nil, 0644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	refused := filepath.Join(blocker, "nested", "app.log")

	home := filepath.Join(t.TempDir(), "home-is-a-file")
	if err := os.WriteFile(home, nil, 0644); err != nil {
		t.Fatalf("write home: %v", err)
	}
	setHomeDir(t, home)

	cfg := app.config
	cfg.LogFile = refused
	app.applySettings(cfg)

	if app.config.LogFile == "" {
		t.Error("config.LogFile = \"\", which saves as a deliberate logging-off")
	}
	if got := config.SettingsFromConfig(app.config).LogFile; got != nil && *got == "" {
		t.Error("settings would write log_file: \"\" after a failed open")
	}
}

func seedPlace(app *App) {
	app.selectedNavigation = &NavigationNode{ID: "team-1", TeamID: "team-1", IsTeam: true, Text: "Engineering"}
	app.issues = []linearapi.Issue{{ID: "issue-1", Identifier: "ZNL-1", Title: "On screen"}}
	app.listIssueRows = []IssueRow{{IssueID: "issue-1"}}
	app.richFilters = IssueFilters{AssigneeID: "user-1", AssigneeName: "Test User"}
	app.collapsedGroups = map[string]bool{"Todo": true}
	app.metadataTeamID = "team-1"
	app.teamProjects = []linearapi.Project{{ID: "proj-1", Name: "Website"}}
	app.groupingOverridden = true
}

func TestSavingAThemeKeepsWhatIsOnScreen(t *testing.T) {
	app := newUXTestApp(t)
	seedPlace(app)

	before := app.api
	generation := app.resetGeneration.Load()

	cfg := app.config
	cfg.Theme = config.ThemeLinear
	app.applySettings(cfg)

	if app.theme != ResolveTheme(config.ThemeLinear) {
		t.Fatal("theme not applied: the save did not take")
	}

	if app.api != before {
		t.Error("API client rebuilt for a theme change")
	}
	if got := app.resetGeneration.Load(); got != generation {
		t.Errorf("resetGeneration = %d, want %d: the cached state was reset", got, generation)
	}
	if app.selectedNavigation == nil {
		t.Error("navigation selection dropped")
	}
	if len(app.listIssueRows) == 0 {
		t.Error("issue rows dropped")
	}
	if app.richFilters.AssigneeID != "user-1" {
		t.Errorf("richFilters.AssigneeID = %q, want the filter the user set", app.richFilters.AssigneeID)
	}
	if !app.collapsedGroups["Todo"] {
		t.Error("collapsed groups dropped")
	}
	if app.metadataTeamID != "team-1" || len(app.teamProjects) == 0 {
		t.Error("team metadata dropped, costing a refetch the save did not need")
	}
	if !app.groupingOverridden {
		t.Error("grouping override dropped, so a custom view's preference would outrank the user's choice")
	}
}

func TestSavingAPageSizeRebuildsNoModals(t *testing.T) {
	app := newUXTestApp(t)

	before := app.settingsModal

	cfg := app.config
	cfg.PageSize = app.config.PageSize + 10
	app.applySettings(cfg)

	if app.settingsModal != before {
		t.Error("modals rebuilt for a page size change")
	}
	if app.config.PageSize != cfg.PageSize {
		t.Errorf("config.PageSize = %d, want %d", app.config.PageSize, cfg.PageSize)
	}
}

func TestSavingAThemeDoesNotRestartLogging(t *testing.T) {
	isolateLogging(t)

	app := newUXTestApp(t)

	logPath := filepath.Join(t.TempDir(), "app.log")
	cfg := app.config
	cfg.LogFile = logPath
	cfg.LogLevel = "debug"
	app.applySettings(cfg)

	themed := app.config
	themed.Theme = config.ThemeLinear
	app.applySettings(themed)

	contents, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if got := strings.Count(string(contents), "=== Session started ==="); got != 1 {
		t.Errorf("session markers = %d, want 1: the theme save reopened the log", got)
	}
}

func TestSavingANewEndpointReloads(t *testing.T) {
	app := newUXTestApp(t)
	seedPlace(app)

	before := app.api
	generation := app.resetGeneration.Load()

	cfg := app.config
	cfg.APIEndpoint = "https://example.invalid/graphql"
	app.applySettings(cfg)

	if app.api == before {
		t.Error("API client not rebuilt for a new endpoint")
	}
	if got := app.resetGeneration.Load(); got == generation {
		t.Error("cached state not reset for a new endpoint")
	}
	if len(app.listIssueRows) != 0 {
		t.Error("issue rows survived a connection change, so the pane shows another workspace's issues")
	}
}

func TestSavingANewConnectionPutsTheUserBackWhereTheyWere(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
				map[string]any{"id": "team-1", "key": "ENG", "name": "Engineering"},
				map[string]any{"id": "team-2", "key": "NEX", "name": "Nexa"},
			}}}
		case strings.Contains(query, "favorites"):
			data = map[string]any{"favorites": map[string]any{
				"nodes":    []any{},
				"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""},
			}}
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
		LinearAPIKey: "token",
		CacheTTL:     time.Minute,
		PageSize:     10,
		DefaultTeam:  "NEX",
	}
	app := NewApp(linearapi.ClientConfig{Token: "token", Endpoint: server.URL}, cfg, nil)
	startReviewTestApplication(t, app)
	navDone := installNavSettledHook(app)
	refreshDone := installRefreshCompletionHook(app)

	saved := cfg
	saved.Timeout = 45 * time.Second
	app.app.QueueUpdate(func() {
		app.selectedNavigation = &NavigationNode{ID: "team-1", TeamID: "team-1", IsTeam: true, Text: "Engineering"}
		app.applySettings(saved)
	})

	waitForNavSettled(t, navDone)
	waitForRefreshCompletion(t, refreshDone)

	var selected *NavigationNode
	app.app.QueueUpdate(func() { selected = app.selectedNavigation })
	if selected == nil {
		t.Fatal("navigation selection empty after the reload")
	}
	if got := selected.TeamID; got != "team-1" {
		t.Errorf("selected team = %q, want %q: the reload moved the user to the configured default", got, "team-1")
	}
}

func TestSavingAThemeRestylesTheNavigationTree(t *testing.T) {
	app := newUXTestApp(t)
	app.config.Theme = config.ThemeLinear
	app.applyThemeAndDensity()
	app.rebuildNavigationTree([]linearapi.Team{
		{ID: "team-1", Key: "ENG", Name: "Engineering"},
	}, nil)

	opaque := ResolveTheme(config.ThemeLinear).Background
	if got := navNodeBackgrounds(app); len(got) != 1 || got[opaque] == 0 {
		t.Fatalf("nav node backgrounds before the save = %v, want every node on %v", got, opaque)
	}

	cfg := app.config
	cfg.Theme = config.ThemeRosePineMoon
	app.applySettings(cfg)

	want := ResolveTheme(config.ThemeRosePineMoon).Background
	got := navNodeBackgrounds(app)
	if len(got) != 1 || got[want] == 0 {
		t.Errorf("nav node backgrounds after the save = %v, want every node on %v", got, want)
	}
}

func navNodeBackgrounds(app *App) map[tcell.Color]int {
	counts := map[tcell.Color]int{}
	var walk func(*tview.TreeNode)
	walk = func(node *tview.TreeNode) {
		if node == nil {
			return
		}
		_, background, _ := node.GetTextStyle().Decompose()
		counts[background]++
		for _, child := range node.GetChildren() {
			walk(child)
		}
	}
	walk(app.navigationTree.GetRoot())
	return counts
}

func TestSavingSettingsRebuildsTheImageStoreOnlyForImages(t *testing.T) {
	app := newUXTestApp(t)
	app.imageCache = map[string]*loadedImage{"https://example.test/a.png": {}}

	themed := app.config
	themed.Theme = config.ThemeLinear
	app.applySettings(themed)

	if len(app.imageCache) != 1 {
		t.Errorf("imageCache = %v after a theme save, want the decoded pictures kept", app.imageCache)
	}

	off := app.config
	off.Images = config.ImagesOff
	app.applySettings(off)

	if app.imageCache != nil {
		t.Errorf("imageCache = %v after turning images off, want it dropped with the store", app.imageCache)
	}
	if app.imageStore != nil {
		t.Error("image store still built with images off")
	}
}
