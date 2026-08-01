package tui

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/roeyazroel/linear-tui/internal/config"
	"github.com/roeyazroel/linear-tui/internal/linearapi"
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
	app.otherIssueRows, app.otherIDToIssue = buildFlatSearchRows([]linearapi.Issue{issue})
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
	app.otherIssueRows, app.otherIDToIssue = buildFlatSearchRows([]linearapi.Issue{issue})
	app.activeIssuesSection = IssuesSectionOther

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
