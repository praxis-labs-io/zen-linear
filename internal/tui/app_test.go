package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/zen-linear/zen-linear/internal/config"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// stringPtr returns a string pointer for test helpers.
func stringPtr(value string) *string {
	return &value
}

// waitForCondition polls until a condition is true or times out.
func waitForCondition(t *testing.T, timeout time.Duration, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}

func installRefreshCompletionHook(app *App) <-chan struct{} {
	done := make(chan struct{}, 8)
	app.refreshCompleted = func() {
		select {
		case done <- struct{}{}:
		default:
		}
	}
	return done
}

func waitForRefreshCompletions(t *testing.T, done <-chan struct{}, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for refresh completion %d of %d", i+1, count)
		}
	}
}

func waitForRefreshCompletion(t *testing.T, done <-chan struct{}) {
	t.Helper()
	waitForRefreshCompletions(t, done, 1)
}

// TestRefreshIssues_LazyLoadsPages verifies first page renders before background pages.
func TestRefreshIssues_LazyLoadsPages(t *testing.T) {
	cfg := config.Config{
		PageSize: 2,
		CacheTTL: time.Minute,
	}
	app := NewApp(&linearapi.Client{}, cfg, nil)
	app.queueUpdateDraw = func(f func()) { f() }
	refreshDone := installRefreshCompletionHook(app)

	issue1 := linearapi.Issue{ID: "issue-1", Identifier: "ABC-1", Title: "First", State: "Todo"}
	issue2 := linearapi.Issue{ID: "issue-2", Identifier: "ABC-2", Title: "Second", State: "Todo"}

	issueByID := map[string]linearapi.Issue{
		issue1.ID: issue1,
		issue2.ID: issue2,
	}
	app.fetchIssueByID = func(ctx context.Context, id string) (linearapi.Issue, error) {
		return issueByID[id], nil
	}

	blockNext := make(chan struct{})
	app.fetchIssuesPage = func(ctx context.Context, params linearapi.FetchIssuesParams, after *string) (linearapi.IssuePage, error) {
		if after == nil {
			return linearapi.IssuePage{
				Issues:    []linearapi.Issue{issue1},
				HasNext:   true,
				EndCursor: stringPtr("cursor-1"),
			}, nil
		}
		<-blockNext
		return linearapi.IssuePage{
			Issues:  []linearapi.Issue{issue2},
			HasNext: false,
		}, nil
	}

	app.refreshIssues()

	waitForCondition(t, time.Second, func() bool {
		app.issuesMu.RLock()
		defer app.issuesMu.RUnlock()
		return len(app.issues) == 1
	})
	app.issuesMu.RLock()
	selectedIssue := app.selectedIssue
	app.issuesMu.RUnlock()
	if selectedIssue == nil || selectedIssue.ID != issue1.ID {
		t.Fatalf("selectedIssue = %#v, want %s", selectedIssue, issue1.ID)
	}

	close(blockNext)
	waitForCondition(t, time.Second, func() bool {
		app.issuesMu.RLock()
		defer app.issuesMu.RUnlock()
		return len(app.issues) == 2
	})
	waitForRefreshCompletion(t, refreshDone)
	app.issuesMu.RLock()
	selectedIssue = app.selectedIssue
	app.issuesMu.RUnlock()
	if selectedIssue == nil || selectedIssue.ID != issue1.ID {
		t.Fatalf("selectedIssue after append = %#v, want %s", selectedIssue, issue1.ID)
	}
}

// TestRefreshIssues_CancelsStaleLoad verifies stale background pages are ignored.
func TestRefreshIssues_CancelsStaleLoad(t *testing.T) {
	cfg := config.Config{
		PageSize: 2,
		CacheTTL: time.Minute,
	}
	app := NewApp(&linearapi.Client{}, cfg, nil)
	app.queueUpdateDraw = func(f func()) { f() }
	refreshDone := installRefreshCompletionHook(app)

	issue1 := linearapi.Issue{ID: "issue-1", Identifier: "ABC-1", Title: "First", State: "Todo"}
	issue2 := linearapi.Issue{ID: "issue-2", Identifier: "ABC-2", Title: "Second", State: "Todo"}
	issue3 := linearapi.Issue{ID: "issue-3", Identifier: "ABC-3", Title: "Third", State: "Todo"}

	issueByID := map[string]linearapi.Issue{
		issue1.ID: issue1,
		issue2.ID: issue2,
		issue3.ID: issue3,
	}
	app.fetchIssueByID = func(ctx context.Context, id string) (linearapi.Issue, error) {
		return issueByID[id], nil
	}

	var mode atomic.Int32
	blockNext := make(chan struct{})
	app.fetchIssuesPage = func(ctx context.Context, params linearapi.FetchIssuesParams, after *string) (linearapi.IssuePage, error) {
		if mode.Load() == 0 {
			if after == nil {
				return linearapi.IssuePage{
					Issues:    []linearapi.Issue{issue1},
					HasNext:   true,
					EndCursor: stringPtr("cursor-1"),
				}, nil
			}
			<-blockNext
			return linearapi.IssuePage{
				Issues:  []linearapi.Issue{issue2},
				HasNext: false,
			}, nil
		}

		if after == nil {
			return linearapi.IssuePage{
				Issues:  []linearapi.Issue{issue3},
				HasNext: false,
			}, nil
		}

		return linearapi.IssuePage{}, nil
	}

	app.refreshIssues()
	waitForCondition(t, time.Second, func() bool {
		app.issuesMu.RLock()
		defer app.issuesMu.RUnlock()
		return len(app.issues) == 1
	})

	mode.Store(1)
	app.refreshIssues()
	close(blockNext)

	waitForRefreshCompletions(t, refreshDone, 2)
	waitForCondition(t, time.Second, func() bool {
		app.issuesMu.RLock()
		defer app.issuesMu.RUnlock()
		return len(app.issues) == 1 && app.issues[0].ID == issue3.ID
	})
	app.issuesMu.RLock()
	issueID := app.issues[0].ID
	app.issuesMu.RUnlock()
	if issueID == issue2.ID {
		t.Fatalf("stale issue applied, got %s", issueID)
	}
}

func TestRefreshIssues_PreservesNavigationFocus(t *testing.T) {
	cfg := config.Config{
		PageSize: 1,
		CacheTTL: time.Minute,
	}
	app := NewApp(&linearapi.Client{}, cfg, nil)
	app.queueUpdateDraw = func(f func()) { f() }
	refreshDone := installRefreshCompletionHook(app)

	issue := linearapi.Issue{ID: "issue-1", Identifier: "ABC-1", Title: "First", State: "Todo"}
	app.fetchIssueByID = func(ctx context.Context, id string) (linearapi.Issue, error) {
		return issue, nil
	}
	app.fetchIssuesPage = func(ctx context.Context, params linearapi.FetchIssuesParams, after *string) (linearapi.IssuePage, error) {
		return linearapi.IssuePage{
			Issues:  []linearapi.Issue{issue},
			HasNext: false,
		}, nil
	}

	app.focusedPane = FocusNavigation
	app.refreshIssuesWithFocusChange(false)

	waitForCondition(t, time.Second, func() bool {
		app.issuesMu.RLock()
		defer app.issuesMu.RUnlock()
		return len(app.issues) == 1
	})
	waitForRefreshCompletion(t, refreshDone)

	if app.focusedPane != FocusNavigation {
		t.Fatalf("focusedPane = %v, want %v", app.focusedPane, FocusNavigation)
	}
}

func TestRefreshIssues_IncludesStateID(t *testing.T) {
	cfg := config.Config{
		PageSize: 1,
		CacheTTL: time.Minute,
	}
	app := NewApp(&linearapi.Client{}, cfg, nil)
	app.queueUpdateDraw = func(f func()) { f() }
	refreshDone := installRefreshCompletionHook(app)

	called := make(chan linearapi.FetchIssuesParams, 1)
	app.fetchIssuesPage = func(ctx context.Context, params linearapi.FetchIssuesParams, after *string) (linearapi.IssuePage, error) {
		select {
		case called <- params:
		default:
		}
		return linearapi.IssuePage{Issues: []linearapi.Issue{}, HasNext: false}, nil
	}

	app.selectedNavigation = &NavigationNode{
		ID:        "state-123",
		Text:      "In Progress",
		TeamID:    "team-1",
		IsStatus:  true,
		StateID:   "state-123",
		StateName: "In Progress",
	}

	app.refreshIssues()

	select {
	case params := <-called:
		if params.StateID != "state-123" {
			t.Fatalf("StateID = %q, want %q", params.StateID, "state-123")
		}
		if params.TeamID != "team-1" {
			t.Fatalf("TeamID = %q, want %q", params.TeamID, "team-1")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for fetchIssuesPage")
	}
	waitForRefreshCompletion(t, refreshDone)
}

func TestRefreshIssues_IncludesCycleID(t *testing.T) {
	cfg := config.Config{
		PageSize: 1,
		CacheTTL: time.Minute,
	}
	app := NewApp(&linearapi.Client{}, cfg, nil)
	app.queueUpdateDraw = func(f func()) { f() }
	refreshDone := installRefreshCompletionHook(app)

	called := make(chan linearapi.FetchIssuesParams, 1)
	app.fetchIssuesPage = func(ctx context.Context, params linearapi.FetchIssuesParams, after *string) (linearapi.IssuePage, error) {
		select {
		case called <- params:
		default:
		}
		return linearapi.IssuePage{Issues: []linearapi.Issue{}, HasNext: false}, nil
	}

	app.selectedNavigation = &NavigationNode{
		ID:        "cycle-123",
		Text:      "Cycle 12",
		TeamID:    "team-1",
		IsCycle:   true,
		CycleID:   "cycle-123",
		CycleName: "Cycle 12",
	}

	app.refreshIssues()

	select {
	case params := <-called:
		if params.CycleID != "cycle-123" {
			t.Fatalf("CycleID = %q, want %q", params.CycleID, "cycle-123")
		}
		if params.TeamID != "team-1" {
			t.Fatalf("TeamID = %q, want %q", params.TeamID, "team-1")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for fetchIssuesPage")
	}
	waitForRefreshCompletion(t, refreshDone)
}

// waitForSearchRows waits until the Search tab holds the given number of
// result rows. Reads go through uiUpdateMu, the lock the immediate
// queueUpdateDraw stub applies around search-result updates.
func waitForSearchRows(t *testing.T, app *App, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		app.uiUpdateMu.Lock()
		got := len(app.searchIssueRows)
		app.uiUpdateMu.Unlock()
		if got == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d search result rows", want)
}

func TestSearchTabTypingDebouncesLatestQuery(t *testing.T) {
	cfg := config.Config{
		PageSize:       1,
		CacheTTL:       time.Minute,
		SearchDebounce: 80 * time.Millisecond,
	}
	app := NewApp(&linearapi.Client{}, cfg, nil)
	app.queueUpdateDraw = func(f func()) { f() }
	// A selected team must not scope the search: it is workspace-wide.
	app.selectedNavigation = &NavigationNode{ID: "team-1", TeamID: "team-1", IsTeam: true}

	called := make(chan linearapi.FetchIssuesParams, 4)
	app.fetchIssuesPage = func(ctx context.Context, params linearapi.FetchIssuesParams, after *string) (linearapi.IssuePage, error) {
		if after != nil {
			t.Errorf("search fetched a follow-up page; want first page only")
		}
		select {
		case called <- params:
		default:
		}
		return linearapi.IssuePage{Issues: []linearapi.Issue{}, HasNext: true}, nil
	}

	app.openSearchTab()
	app.searchInput.SetText("a")
	app.searchInput.SetText("ab")

	select {
	case params := <-called:
		t.Fatalf("fetch fired before debounce elapsed with search %q", params.Search)
	case <-time.After(25 * time.Millisecond):
	}

	var params linearapi.FetchIssuesParams
	select {
	case params = <-called:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for debounced search fetch")
	}
	if params.Search != "ab" {
		t.Fatalf("Search = %q, want latest query %q", params.Search, "ab")
	}
	if params.TeamID != "" {
		t.Fatalf("TeamID = %q, want empty for workspace-wide search", params.TeamID)
	}
	if params.First != cfg.PageSize {
		t.Fatalf("First = %d, want %d", params.First, cfg.PageSize)
	}

	select {
	case params := <-called:
		t.Fatalf("unexpected extra fetch after debounce fired with search %q", params.Search)
	case <-time.After(120 * time.Millisecond):
	}

	if app.activeIssuesSection != IssuesSectionSearch {
		t.Fatalf("activeIssuesSection = %v, want IssuesSectionSearch", app.activeIssuesSection)
	}
	if !app.searchInputFocused {
		t.Fatal("search input lost focus during live search")
	}

	// Wait for the result callback to finish before the test returns: it
	// renders through tview globals the next test's NewApp rewrites.
	waitForCondition(t, time.Second, func() bool {
		app.uiUpdateMu.Lock()
		defer app.uiUpdateMu.Unlock()
		return !app.searchLoading
	})
}

func TestSearchTabEnterMovesFocusToResults(t *testing.T) {
	cfg := config.Config{
		PageSize:       1,
		CacheTTL:       time.Minute,
		SearchDebounce: 20 * time.Millisecond,
	}
	app := NewApp(&linearapi.Client{}, cfg, nil)
	app.queueUpdateDraw = func(f func()) { f() }

	issue := linearapi.Issue{ID: "issue-1", Identifier: "ABC-1", Title: "Search hit", State: "Todo"}
	app.fetchIssuesPage = func(ctx context.Context, params linearapi.FetchIssuesParams, after *string) (linearapi.IssuePage, error) {
		return linearapi.IssuePage{Issues: []linearapi.Issue{issue}}, nil
	}
	// Hold the detail fetch until assertions are done so its immediate
	// queueUpdateDraw stub cannot run concurrently with them.
	releaseDetails := make(chan struct{})
	defer close(releaseDetails)
	app.fetchIssueByID = func(ctx context.Context, id string) (linearapi.Issue, error) {
		<-releaseDetails
		return issue, nil
	}

	app.openSearchTab()
	app.searchInput.SetText("hit")
	waitForSearchRows(t, app, 1)

	app.handleSearchInputKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))

	if app.searchInputFocused {
		t.Fatal("searchInputFocused = true after Enter, want focus on results")
	}
	app.issuesMu.RLock()
	selected := app.selectedIssue
	app.issuesMu.RUnlock()
	if selected == nil || selected.ID != "issue-1" {
		t.Fatalf("selectedIssue = %+v, want issue-1", selected)
	}
}

func TestSearchStaleResultsDropped(t *testing.T) {
	cfg := config.Config{
		PageSize:       5,
		CacheTTL:       time.Minute,
		SearchDebounce: 10 * time.Millisecond,
	}
	app := NewApp(&linearapi.Client{}, cfg, nil)
	app.queueUpdateDraw = func(f func()) { f() }

	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	issueA := linearapi.Issue{ID: "issue-a", Identifier: "ABC-1", Title: "Stale", State: "Todo"}
	issueB := linearapi.Issue{ID: "issue-b", Identifier: "ABC-2", Title: "Fresh", State: "Todo"}
	app.fetchIssuesPage = func(ctx context.Context, params linearapi.FetchIssuesParams, after *string) (linearapi.IssuePage, error) {
		if params.Search == "stale" {
			close(firstStarted)
			<-releaseFirst
			return linearapi.IssuePage{Issues: []linearapi.Issue{issueA}}, nil
		}
		return linearapi.IssuePage{Issues: []linearapi.Issue{issueB}}, nil
	}

	app.openSearchTab()
	app.performIssueSearch("stale")
	<-firstStarted
	app.performIssueSearch("fresh")
	waitForSearchRows(t, app, 1)
	close(releaseFirst)

	// Give the stale response a chance to land (it must be discarded).
	time.Sleep(50 * time.Millisecond)
	app.uiUpdateMu.Lock()
	defer app.uiUpdateMu.Unlock()
	if len(app.searchIssueRows) != 1 || app.searchIssueRows[0].IssueID != "issue-b" {
		t.Fatalf("searchIssueRows = %+v, want the fresh result only", app.searchIssueRows)
	}
}

func TestSearchEmptyQueryClearsWithoutFetch(t *testing.T) {
	cfg := config.Config{
		PageSize:       5,
		CacheTTL:       time.Minute,
		SearchDebounce: 10 * time.Millisecond,
	}
	app := NewApp(&linearapi.Client{}, cfg, nil)
	app.queueUpdateDraw = func(f func()) { f() }

	var fetches atomic.Int64
	issue := linearapi.Issue{ID: "issue-1", Identifier: "ABC-1", Title: "Hit", State: "Todo"}
	app.fetchIssuesPage = func(ctx context.Context, params linearapi.FetchIssuesParams, after *string) (linearapi.IssuePage, error) {
		fetches.Add(1)
		return linearapi.IssuePage{Issues: []linearapi.Issue{issue}}, nil
	}

	app.openSearchTab()
	app.performIssueSearch("hit")
	waitForSearchRows(t, app, 1)

	app.performIssueSearch("")
	waitForSearchRows(t, app, 0)
	time.Sleep(30 * time.Millisecond)
	if got := fetches.Load(); got != 1 {
		t.Fatalf("fetch count = %d, want 1 (empty query must not hit the API)", got)
	}
}

func TestSearchTabTypedLettersReachInput(t *testing.T) {
	cfg := config.Config{
		PageSize:       1,
		CacheTTL:       time.Minute,
		SearchDebounce: time.Hour, // keep the debounce from firing mid-test
	}
	app := NewApp(&linearapi.Client{}, cfg, nil)
	app.queueUpdateDraw = func(f func()) { f() }

	app.openSearchTab()
	if !app.searchInputActive() {
		t.Fatal("searchInputActive() = false after openSearchTab")
	}

	// The quit shortcut must pass through to the input instead of stopping
	// the app.
	event := tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone)
	if got := app.handleSearchInputKey(event); got != event {
		t.Fatalf("handleSearchInputKey(q) = %v, want the event passed through", got)
	}
}

func TestSearchTabRemappedTabKeysCycleOutOfInput(t *testing.T) {
	cfg := config.Config{
		PageSize:    1,
		CacheTTL:    time.Minute,
		Keybindings: map[string]string{"tab_next": "]", "tab_prev": "["},
	}
	app := NewApp(&linearapi.Client{}, cfg, nil)
	app.queueUpdateDraw = func(f func()) { f() }

	issue := linearapi.Issue{ID: "issue-1", Identifier: "ABC-1", Title: "First", State: "Todo"}
	app.allIssueRows, app.allIDToIssue = buildFlatSearchRows([]linearapi.Issue{issue})
	// Hold the detail fetch until assertions are done so its immediate
	// queueUpdateDraw stub cannot run concurrently with them.
	releaseDetails := make(chan struct{})
	defer close(releaseDetails)
	app.fetchIssueByID = func(ctx context.Context, id string) (linearapi.Issue, error) {
		<-releaseDetails
		return issue, nil
	}

	app.openSearchTab()
	if got := app.handleSearchInputKey(tcell.NewEventKey(tcell.KeyRune, ']', tcell.ModNone)); got != nil {
		t.Fatal("tab_next rune leaked through to the search input")
	}
	if app.activeIssuesSection == IssuesSectionSearch {
		t.Fatal("tab_next did not cycle out of the Search tab")
	}
	if app.searchInput.GetText() != "" {
		t.Fatalf("search input text = %q, want empty (rune must not be typed)", app.searchInput.GetText())
	}
}

func TestSearchEscOnEmptyInputReturnsToPreviousTab(t *testing.T) {
	cfg := config.Config{
		PageSize:       1,
		CacheTTL:       time.Minute,
		SearchDebounce: 10 * time.Millisecond,
	}
	app := NewApp(&linearapi.Client{}, cfg, nil)
	app.queueUpdateDraw = func(f func()) { f() }
	app.activeIssuesSection = IssuesSectionAll

	app.openSearchTab()
	if app.searchReturnSection != IssuesSectionAll {
		t.Fatalf("searchReturnSection = %v, want IssuesSectionAll", app.searchReturnSection)
	}

	app.handleSearchInputKey(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	if app.activeIssuesSection != IssuesSectionAll {
		t.Fatalf("activeIssuesSection = %v, want IssuesSectionAll after Esc", app.activeIssuesSection)
	}
	if app.searchInputFocused {
		t.Fatal("searchInputFocused = true after leaving the tab")
	}
}

func TestCycleIssuesSectionReachesEmptySearchTab(t *testing.T) {
	cfg := config.Config{
		PageSize: 1,
		CacheTTL: time.Minute,
	}
	app := NewApp(&linearapi.Client{}, cfg, nil)
	app.queueUpdateDraw = func(f func()) { f() }

	issue := linearapi.Issue{ID: "issue-1", Identifier: "ABC-1", Title: "First", State: "Todo"}
	app.allIssueRows, app.allIDToIssue = buildFlatSearchRows([]linearapi.Issue{issue})
	app.activeIssuesSection = IssuesSectionAll

	app.cycleIssuesSection(1)
	if app.activeIssuesSection != IssuesSectionMy {
		t.Fatalf("activeIssuesSection = %v, want IssuesSectionMy (an empty tab is still a tab)", app.activeIssuesSection)
	}
	app.cycleIssuesSection(1)
	if app.activeIssuesSection != IssuesSectionSearch {
		t.Fatalf("activeIssuesSection = %v, want IssuesSectionSearch (empty Search tab must stay reachable)", app.activeIssuesSection)
	}
	if !app.searchInputFocused {
		t.Fatal("entering the Search tab must focus its input")
	}
}

func TestResetCachedStateClearsSearch(t *testing.T) {
	cfg := config.Config{
		PageSize:       5,
		CacheTTL:       time.Minute,
		SearchDebounce: 10 * time.Millisecond,
	}
	app := NewApp(&linearapi.Client{}, cfg, nil)
	app.queueUpdateDraw = func(f func()) { f() }

	issue := linearapi.Issue{ID: "issue-1", Identifier: "ABC-1", Title: "Hit", State: "Todo"}
	app.fetchIssuesPage = func(ctx context.Context, params linearapi.FetchIssuesParams, after *string) (linearapi.IssuePage, error) {
		return linearapi.IssuePage{Issues: []linearapi.Issue{issue}}, nil
	}

	app.openSearchTab()
	app.performIssueSearch("hit")
	waitForSearchRows(t, app, 1)

	app.resetCachedState()
	if len(app.searchIssueRows) != 0 || len(app.searchIssues) != 0 {
		t.Fatalf("search state not cleared: rows=%d issues=%d", len(app.searchIssueRows), len(app.searchIssues))
	}
	if app.searchInput.GetText() != "" {
		t.Fatalf("search input text = %q, want empty", app.searchInput.GetText())
	}
}

func TestResetCachedStateClearsPendingSectionRenders(t *testing.T) {
	app := NewApp(&linearapi.Client{}, config.Config{PageSize: 1, CacheTTL: time.Minute}, nil)
	app.queueUpdateDraw = func(f func()) { f() }
	app.pendingSectionRenders = map[IssuesSection]string{IssuesSectionMy: "issue-1"}

	app.resetCachedState()

	if len(app.pendingSectionRenders) != 0 {
		t.Fatalf("pendingSectionRenders = %v, want empty", app.pendingSectionRenders)
	}
}

// TestAssignMe_DuringCurrentUserFetch drives assign_me through the real command
// handler while the current-user fetch is still in flight, on both sides of the
// queued write that installs the user.
func TestAssignMe_DuringCurrentUserFetch(t *testing.T) {
	app := NewApp(&linearapi.Client{}, config.Config{PageSize: 1, CacheTTL: time.Minute}, nil)
	app.queueUpdateDraw = func(f func()) { f() }

	release := make(chan struct{})
	user := linearapi.User{ID: "user-1", Name: "Test User", DisplayName: "Tester"}
	app.fetchCurrentUserFunc = func(context.Context) (linearapi.User, error) {
		<-release
		return user, nil
	}

	issue := linearapi.Issue{ID: "issue-1", Identifier: "ZNL-1", Title: "First", State: "Todo"}
	app.issuesMu.Lock()
	app.selectedIssue = &issue
	app.issuesMu.Unlock()

	var assignedMu sync.Mutex
	var assigned []string
	// The empty response stops applyIssueUpdate at its first check. What the
	// command sent is the subject here, and a repaint would outlive the test on
	// the update goroutines the dispatch loop below leaves in flight.
	app.updateIssueFunc = func(_ context.Context, input linearapi.UpdateIssueInput) (linearapi.Issue, error) {
		assignedMu.Lock()
		if input.AssigneeID != nil {
			assigned = append(assigned, *input.AssigneeID)
		}
		assignedMu.Unlock()
		return linearapi.Issue{}, nil
	}
	assignedIDs := func() []string {
		assignedMu.Lock()
		defer assignedMu.Unlock()
		return slices.Clone(assigned)
	}

	assignMe := findCommandByID(DefaultCommands(app), "assign_me")
	if assignMe == nil {
		t.Fatal("assign_me command not found")
	}
	// Every dispatch goes through the same queue the fetch writes through, the
	// contract the command relies on to see a whole user or none.
	dispatch := func() { app.QueueUpdateDraw(func() { assignMe.Run(app) }) }

	dispatch()
	if got := assignedIDs(); len(got) != 0 {
		t.Fatalf("assigned before the user loaded = %v, want none", got)
	}
	var status string
	app.QueueUpdateDraw(func() { status = app.statusMessage })
	if status != "No issue or current user selected" {
		t.Fatalf("status message = %q, want the no-user message", status)
	}

	// Dispatch across the window where the fetch lands, so the command handler
	// and the queued write overlap.
	hammered := make(chan struct{})
	go func() {
		defer close(hammered)
		for i := 0; i < 50; i++ {
			dispatch()
		}
	}()
	loaded := make(chan struct{})
	go func() {
		defer close(loaded)
		app.loadCurrentUser(context.Background())
	}()
	close(release)
	<-loaded
	<-hammered

	dispatch()
	waitForCondition(t, time.Second, func() bool { return len(assignedIDs()) > 0 })
	for _, id := range assignedIDs() {
		if id != user.ID {
			t.Fatalf("assigned ID = %q, want %q", id, user.ID)
		}
	}
}

func TestUpdateDetailsView_IncludesCycle(t *testing.T) {
	cfg := config.Config{
		PageSize: 1,
		CacheTTL: time.Minute,
	}
	app := NewApp(&linearapi.Client{}, cfg, nil)
	app.queueUpdateDraw = func(f func()) { f() }

	app.issuesMu.Lock()
	app.selectedIssue = &linearapi.Issue{
		ID:         "issue-1",
		Identifier: "ABC-1",
		Title:      "Issue with cycle",
		State:      "Todo",
		Cycle:      &linearapi.CycleRef{ID: "cycle-1", Name: "Launch", Number: 12},
	}
	app.issuesMu.Unlock()

	app.updateDetailsView()
	text := app.detailsDescriptionView.GetText(true)
	if !strings.Contains(text, "Cycle:") || !strings.Contains(text, "Launch") {
		t.Fatalf("details text = %q, want Cycle: Launch", text)
	}
}

func TestDefaultCommands_IncludesCycleCommands(t *testing.T) {
	commands := DefaultCommands(nil)
	ids := make(map[string]bool, len(commands))
	for _, command := range commands {
		ids[command.ID] = true
	}

	for _, id := range []string{"set_cycle", "clear_cycle"} {
		if !ids[id] {
			t.Fatalf("command %q missing from DefaultCommands", id)
		}
	}
}

// renderedTitles returns the title cell of every issue row in a section. Group
// headers render their label into the title column, so they have to be skipped
// by row kind, not by an empty-cell check.
func renderedTitles(app *App, section IssuesSection) []string {
	table := app.tableForSection(section)
	rows := app.rowsForSection(section)
	var titles []string
	for row := 1; row < table.GetRowCount(); row++ {
		if row <= len(rows) && (rows[row-1].IsHeader || rows[row-1].IsSpacer) {
			continue
		}
		cell := table.GetCell(row, titleColumn)
		if cell == nil || cell.Text == "" {
			continue
		}
		titles = append(titles, cell.Text)
	}
	return titles
}

// TestRefreshIssues_PaintsOncePerRefreshNotOncePerPage verifies pages after the
// first accumulate without repainting. Repainting per page regroups and
// re-renders the whole table for an end state only the last page settles.
func TestRefreshIssues_PaintsOncePerRefreshNotOncePerPage(t *testing.T) {
	cfg := config.Config{PageSize: 1, CacheTTL: time.Minute}
	app := NewApp(&linearapi.Client{}, cfg, nil)
	app.queueUpdateDraw = func(f func()) { f() }
	refreshDone := installRefreshCompletionHook(app)
	app.fetchIssueByID = func(_ context.Context, id string) (linearapi.Issue, error) {
		return linearapi.Issue{ID: id}, nil
	}

	pages := []linearapi.Issue{
		{ID: "issue-1", Identifier: "ABC-1", Title: "First", State: "Todo"},
		{ID: "issue-2", Identifier: "ABC-2", Title: "Second", State: "Todo"},
		{ID: "issue-3", Identifier: "ABC-3", Title: "Third", State: "Todo"},
	}

	blockLast := make(chan struct{})
	var fetched atomic.Int32
	app.fetchIssuesPage = func(_ context.Context, _ linearapi.FetchIssuesParams, after *string) (linearapi.IssuePage, error) {
		index := int(fetched.Add(1)) - 1
		if index == len(pages)-1 {
			<-blockLast
		}
		return linearapi.IssuePage{
			Issues:    []linearapi.Issue{pages[index]},
			HasNext:   index < len(pages)-1,
			EndCursor: stringPtr(fmt.Sprintf("cursor-%d", index)),
		}, nil
	}

	app.refreshIssues()

	// Page 2 has merged but must not have reached the table yet.
	waitForCondition(t, time.Second, func() bool {
		app.issuesMu.RLock()
		defer app.issuesMu.RUnlock()
		return len(app.issues) == 2
	})
	if got := renderedTitles(app, IssuesSectionAll); !slices.Equal(got, []string{"First"}) {
		t.Fatalf("rendered titles mid-pagination = %v, want [First]", got)
	}

	close(blockLast)
	waitForRefreshCompletion(t, refreshDone)

	got := renderedTitles(app, IssuesSectionAll)
	want := []string{"First", "Second", "Third"}
	if !slices.Equal(got, want) {
		t.Fatalf("rendered titles after pagination = %v, want %v", got, want)
	}
}

// TestRefreshIssues_KeepsSelectionAcrossPagination guards the one thing the
// deferred paint could break: the cursor moving because a later page reordered
// the list under it.
func TestRefreshIssues_KeepsSelectionAcrossPagination(t *testing.T) {
	cfg := config.Config{PageSize: 1, CacheTTL: time.Minute}
	app := NewApp(&linearapi.Client{}, cfg, nil)
	app.queueUpdateDraw = func(f func()) { f() }
	refreshDone := installRefreshCompletionHook(app)
	app.fetchIssueByID = func(_ context.Context, id string) (linearapi.Issue, error) {
		return linearapi.Issue{ID: id}, nil
	}

	// Page 2 sorts ahead of page 1, so the selected row moves.
	pages := []linearapi.Issue{
		{ID: "issue-b", Identifier: "ABC-2", Title: "Beta", State: "Todo", Priority: 3},
		{ID: "issue-a", Identifier: "ABC-1", Title: "Alpha", State: "Todo", Priority: 1},
	}
	app.sortFields = []SortField{SortByPriority}

	var fetched atomic.Int32
	app.fetchIssuesPage = func(_ context.Context, _ linearapi.FetchIssuesParams, after *string) (linearapi.IssuePage, error) {
		index := int(fetched.Add(1)) - 1
		return linearapi.IssuePage{
			Issues:    []linearapi.Issue{pages[index]},
			HasNext:   index < len(pages)-1,
			EndCursor: stringPtr(fmt.Sprintf("cursor-%d", index)),
		}, nil
	}

	app.refreshIssues()
	waitForRefreshCompletion(t, refreshDone)

	got := renderedTitles(app, IssuesSectionAll)
	if !slices.Equal(got, []string{"Alpha", "Beta"}) {
		t.Fatalf("rendered titles = %v, want [Alpha Beta]", got)
	}

	app.issuesMu.RLock()
	selected := app.selectedIssue
	app.issuesMu.RUnlock()
	if selected == nil || selected.ID != "issue-b" {
		t.Fatalf("selectedIssue = %#v, want issue-b to survive the reorder", selected)
	}
}

// TestAccumulateIssues_ReconcilesAfterAnOutsideSplice guards the dedup set
// against another path appending to a.issues mid-refresh: insertIssue does
// exactly that when an edit brings an issue into scope, and a page carrying
// the same issue would otherwise add it twice.
func TestAccumulateIssues_ReconcilesAfterAnOutsideSplice(t *testing.T) {
	app := newUXTestApp()
	app.fetchIssueByID = func(_ context.Context, id string) (linearapi.Issue, error) {
		return linearapi.Issue{ID: id}, nil
	}
	page1 := []linearapi.Issue{{ID: "issue-1", Identifier: "ZNL-1", Title: "First"}}
	spliced := linearapi.Issue{ID: "issue-2", Identifier: "ZNL-2", Title: "Spliced"}

	app.updateIssuesData(page1)
	merge := &pageMerge{seen: make(map[string]bool)}
	app.issuesMu.RLock()
	merge.reset(app.issues)
	app.issuesMu.RUnlock()

	// Something else adds issue-2 while pagination is still running.
	app.issuesMu.Lock()
	app.issues = append(app.issues, spliced)
	app.issuesMu.Unlock()

	// The next server page carries it too.
	app.accumulateIssues([]linearapi.Issue{spliced, {ID: "issue-3", Identifier: "ZNL-3"}}, merge)

	app.issuesMu.RLock()
	defer app.issuesMu.RUnlock()
	counts := map[string]int{}
	for i := range app.issues {
		counts[app.issues[i].ID]++
	}
	if counts["issue-2"] != 1 {
		t.Fatalf("issue-2 appears %d times, want 1", counts["issue-2"])
	}
	if len(app.issues) != 3 {
		t.Fatalf("issue count = %d, want 3", len(app.issues))
	}
}

// TestAccumulateIssues_KeepsTheListSorted guards the repaint paths that read
// a.issues directly mid-pagination and do not sort first.
func TestAccumulateIssues_KeepsTheListSorted(t *testing.T) {
	app := newUXTestApp()
	app.fetchIssueByID = func(_ context.Context, id string) (linearapi.Issue, error) {
		return linearapi.Issue{ID: id}, nil
	}
	app.sortFields = []SortField{SortByPriority}

	app.updateIssuesData([]linearapi.Issue{{ID: "issue-b", Identifier: "ZNL-2", Priority: 3}})
	merge := &pageMerge{seen: make(map[string]bool)}
	app.issuesMu.RLock()
	merge.reset(app.issues)
	app.issuesMu.RUnlock()

	app.accumulateIssues([]linearapi.Issue{{ID: "issue-a", Identifier: "ZNL-1", Priority: 1}}, merge)

	app.issuesMu.RLock()
	defer app.issuesMu.RUnlock()
	if app.issues[0].ID != "issue-a" {
		t.Fatalf("issues[0] = %q, want issue-a: the merged page left the list unsorted", app.issues[0].ID)
	}
}

// TestRenderAccumulatedIssues_KeepsTheHydratedSelection guards the details
// pane: the list model carries no comments or attachments, so overwriting a
// surviving selection with it silently strips them.
func TestRenderAccumulatedIssues_KeepsTheHydratedSelection(t *testing.T) {
	app := newUXTestApp()
	// Set the state directly: updateIssuesData fires an async detail fetch
	// that would race the selection this test installs.
	hydrated := linearapi.Issue{
		ID: "issue-1", Identifier: "ZNL-1", Title: "First",
		Comments: []linearapi.Comment{{ID: "comment-1", Body: "still here"}},
	}
	app.issuesMu.Lock()
	app.issues = []linearapi.Issue{
		{ID: "issue-1", Identifier: "ZNL-1", Title: "First"},
		{ID: "issue-2", Identifier: "ZNL-2", Title: "Second"},
	}
	app.selectedIssue = &hydrated
	app.issuesMu.Unlock()

	app.renderAccumulatedIssues()

	app.issuesMu.RLock()
	defer app.issuesMu.RUnlock()
	if got := len(app.selectedIssue.Comments); got != 1 {
		t.Fatalf("selected issue has %d comments, want 1: the repaint replaced it with the list copy", got)
	}
}

// TestRenderAccumulatedIssues_LeavesTheSearchTabAlone mirrors the guard
// updateIssuesData carries: a background refresh must not clear the selection
// the user is browsing in search results.
func TestRenderAccumulatedIssues_LeavesTheSearchTabAlone(t *testing.T) {
	app := newUXTestApp()
	// A search hit that is not in the My/Other models. State is set directly
	// so no async detail fetch races the selection.
	offList := linearapi.Issue{ID: "issue-99", Identifier: "ZNL-99", Title: "Elsewhere"}
	app.issuesMu.Lock()
	app.issues = []linearapi.Issue{{ID: "issue-1", Identifier: "ZNL-1", Title: "First"}}
	app.selectedIssue = &offList
	app.issuesMu.Unlock()
	app.activeIssuesSection = IssuesSectionSearch

	app.renderAccumulatedIssues()

	app.issuesMu.RLock()
	defer app.issuesMu.RUnlock()
	if app.selectedIssue == nil || app.selectedIssue.ID != "issue-99" {
		t.Fatalf("selectedIssue = %#v, want the search hit to survive", app.selectedIssue)
	}
}

// TestRefreshIssues_PaintsDuringPagination guards the budget: pages fetched
// early in a slow load must become reachable without waiting for the last one.
func TestRefreshIssues_PaintsDuringPagination(t *testing.T) {
	cfg := config.Config{PageSize: 1, CacheTTL: time.Minute}
	app := NewApp(&linearapi.Client{}, cfg, nil)
	app.fetchIssueByID = func(_ context.Context, id string) (linearapi.Issue, error) {
		return linearapi.Issue{ID: id}, nil
	}
	// The table belongs to the draw goroutine, so snapshot it there rather
	// than reading it from the test.
	var paintedMu sync.Mutex
	var painted []string
	app.queueUpdateDraw = func(f func()) {
		f()
		paintedMu.Lock()
		painted = renderedTitles(app, IssuesSectionAll)
		paintedMu.Unlock()
	}
	lastPainted := func() []string {
		paintedMu.Lock()
		defer paintedMu.Unlock()
		return slices.Clone(painted)
	}
	refreshDone := installRefreshCompletionHook(app)

	pages := []linearapi.Issue{
		{ID: "issue-1", Identifier: "ZNL-1", Title: "First", State: "Todo"},
		{ID: "issue-2", Identifier: "ZNL-2", Title: "Second", State: "Todo"},
		{ID: "issue-3", Identifier: "ZNL-3", Title: "Third", State: "Todo"},
	}

	blockLast := make(chan struct{})
	var fetched atomic.Int32
	app.fetchIssuesPage = func(_ context.Context, _ linearapi.FetchIssuesParams, _ *string) (linearapi.IssuePage, error) {
		index := int(fetched.Add(1)) - 1
		if index == len(pages)-1 {
			<-blockLast
		}
		if index > 0 {
			// Push page 2 past the repaint budget so it must paint on its own.
			time.Sleep(issuesRepaintInterval + 50*time.Millisecond)
		}
		return linearapi.IssuePage{
			Issues:    []linearapi.Issue{pages[index]},
			HasNext:   index < len(pages)-1,
			EndCursor: stringPtr(fmt.Sprintf("cursor-%d", index)),
		}, nil
	}

	app.refreshIssues()

	// Page 2 must reach the table while page 3 is still blocked.
	waitForCondition(t, 3*time.Second, func() bool {
		return slices.Equal(lastPainted(), []string{"First", "Second"})
	})

	close(blockLast)
	waitForRefreshCompletion(t, refreshDone)

	got := lastPainted()
	want := []string{"First", "Second", "Third"}
	if !slices.Equal(got, want) {
		t.Fatalf("rendered titles after pagination = %v, want %v", got, want)
	}
}

// TestRenderedTitles_SkipsGroupHeaders guards the test helper itself: group
// headers write their label into the title column, so an empty-cell check does
// not exclude them and every assertion built on it would silently compare
// against header text once grouping is on.
func TestRenderedTitles_SkipsGroupHeaders(t *testing.T) {
	cfg := config.Config{PageSize: 10, CacheTTL: time.Minute, GroupBy: GroupByStatus}
	app := NewApp(&linearapi.Client{}, cfg, nil)
	app.queueUpdateDraw = func(f func()) { f() }
	app.fetchIssueByID = func(_ context.Context, id string) (linearapi.Issue, error) {
		return linearapi.Issue{ID: id}, nil
	}

	app.issuesMu.Lock()
	app.issues = []linearapi.Issue{
		{ID: "issue-1", Identifier: "ZNL-1", Title: "First", State: "Todo"},
		{ID: "issue-2", Identifier: "ZNL-2", Title: "Second", State: "In Progress"},
	}
	app.issuesMu.Unlock()
	app.rebuildIssuesTables("")

	if app.effectiveGroupBy() != GroupByStatus {
		t.Fatalf("effectiveGroupBy = %q, want %q", app.effectiveGroupBy(), GroupByStatus)
	}
	rows := app.rowsForSection(IssuesSectionAll)
	headers := 0
	for _, row := range rows {
		if row.IsHeader {
			headers++
		}
	}
	if headers == 0 {
		t.Fatal("no group headers rendered, so this test proves nothing")
	}

	got := renderedTitles(app, IssuesSectionAll)
	want := []string{"Second", "First"}
	if !slices.Equal(got, want) {
		t.Fatalf("renderedTitles = %v, want %v", got, want)
	}
}
