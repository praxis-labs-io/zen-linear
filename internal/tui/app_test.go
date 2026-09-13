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
	"github.com/praxis-labs-io/zen-linear/internal/config"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
)

func stringPtr(value string) *string {
	return &value
}

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
		case <-time.After(30 * time.Second):
			t.Fatalf("timed out waiting for refresh completion %d of %d", i+1, count)
		}
	}
}

func waitForRefreshCompletion(t *testing.T, done <-chan struct{}) {
	t.Helper()
	waitForRefreshCompletions(t, done, 1)
}

func TestRefreshIssues_LazyLoadsPages(t *testing.T) {
	cfg := config.Config{
		PageSize: 2,
		CacheTTL: time.Minute,
	}
	app := NewApp(linearapi.ClientConfig{}, cfg, nil)
	stopBackgroundWorkOnCleanup(t, app)
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
		return len(app.issues) == 1 && app.selectedIssue != nil
	})
	app.issuesMu.RLock()
	selectedIssue := app.selectedIssue
	app.issuesMu.RUnlock()
	if selectedIssue.ID != issue1.ID {
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

func TestRefreshIssues_CancelsStaleLoad(t *testing.T) {
	cfg := config.Config{
		PageSize: 2,
		CacheTTL: time.Minute,
	}
	app := NewApp(linearapi.ClientConfig{}, cfg, nil)
	stopBackgroundWorkOnCleanup(t, app)
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
	app := NewApp(linearapi.ClientConfig{}, cfg, nil)
	stopBackgroundWorkOnCleanup(t, app)
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
	app := NewApp(linearapi.ClientConfig{}, cfg, nil)
	stopBackgroundWorkOnCleanup(t, app)
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
	app := NewApp(linearapi.ClientConfig{}, cfg, nil)
	stopBackgroundWorkOnCleanup(t, app)
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
	app := NewApp(linearapi.ClientConfig{}, cfg, nil)
	stopBackgroundWorkOnCleanup(t, app)
	app.queueUpdateDraw = func(f func()) { f() }
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

	app.focusNavSearch()
	app.navSearchInput.SetText("a")
	app.navSearchInput.SetText("ab")

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
	if !app.navSearchFocused {
		t.Fatal("search input lost focus during live search")
	}

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
	app := NewApp(linearapi.ClientConfig{}, cfg, nil)
	stopBackgroundWorkOnCleanup(t, app)
	app.queueUpdateDraw = func(f func()) { f() }

	issue := linearapi.Issue{ID: "issue-1", Identifier: "ABC-1", Title: "Search hit", State: "Todo"}
	app.fetchIssuesPage = func(ctx context.Context, params linearapi.FetchIssuesParams, after *string) (linearapi.IssuePage, error) {
		return linearapi.IssuePage{Issues: []linearapi.Issue{issue}}, nil
	}
	releaseDetails := make(chan struct{})
	defer close(releaseDetails)
	app.fetchIssueByID = func(ctx context.Context, id string) (linearapi.Issue, error) {
		<-releaseDetails
		return issue, nil
	}

	app.focusNavSearch()
	app.navSearchInput.SetText("hit")
	waitForSearchRows(t, app, 1)

	app.handleNavSearchKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))

	if app.navSearchFocused {
		t.Fatal("navSearchFocused = true after Enter, want focus on results")
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
	app := NewApp(linearapi.ClientConfig{}, cfg, nil)
	stopBackgroundWorkOnCleanup(t, app)
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

	app.focusNavSearch()
	app.performIssueSearch("stale")
	<-firstStarted
	app.performIssueSearch("fresh")
	waitForSearchRows(t, app, 1)
	close(releaseFirst)

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
	app := NewApp(linearapi.ClientConfig{}, cfg, nil)
	stopBackgroundWorkOnCleanup(t, app)
	app.queueUpdateDraw = func(f func()) { f() }

	var fetches atomic.Int64
	issue := linearapi.Issue{ID: "issue-1", Identifier: "ABC-1", Title: "Hit", State: "Todo"}
	app.fetchIssuesPage = func(ctx context.Context, params linearapi.FetchIssuesParams, after *string) (linearapi.IssuePage, error) {
		fetches.Add(1)
		return linearapi.IssuePage{Issues: []linearapi.Issue{issue}}, nil
	}

	app.focusNavSearch()
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
		SearchDebounce: time.Hour,
	}
	app := NewApp(linearapi.ClientConfig{}, cfg, nil)
	stopBackgroundWorkOnCleanup(t, app)
	app.queueUpdateDraw = func(f func()) { f() }

	app.focusNavSearch()
	if !app.navSearchActive() {
		t.Fatal("navSearchActive() = false after focusNavSearch")
	}

	event := tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone)
	if got := app.handleNavSearchKey(event); got != event {
		t.Fatalf("handleNavSearchKey(q) = %v, want the event passed through", got)
	}
}

func TestResetCachedStateClearsSearch(t *testing.T) {
	cfg := config.Config{
		PageSize:       5,
		CacheTTL:       time.Minute,
		SearchDebounce: 10 * time.Millisecond,
	}
	app := NewApp(linearapi.ClientConfig{}, cfg, nil)
	stopBackgroundWorkOnCleanup(t, app)
	app.queueUpdateDraw = func(f func()) { f() }

	issue := linearapi.Issue{ID: "issue-1", Identifier: "ABC-1", Title: "Hit", State: "Todo"}
	app.fetchIssuesPage = func(ctx context.Context, params linearapi.FetchIssuesParams, after *string) (linearapi.IssuePage, error) {
		return linearapi.IssuePage{Issues: []linearapi.Issue{issue}}, nil
	}

	app.focusNavSearch()
	app.performIssueSearch("hit")
	waitForSearchRows(t, app, 1)

	app.resetCachedState()
	if len(app.searchIssueRows) != 0 || len(app.searchIssues) != 0 {
		t.Fatalf("search state not cleared: rows=%d issues=%d", len(app.searchIssueRows), len(app.searchIssues))
	}
	if app.navSearchInput.GetText() != "" {
		t.Fatalf("search input text = %q, want empty", app.navSearchInput.GetText())
	}
}

func TestResetCachedStateEmptiesTheDetailsPane(t *testing.T) {
	app := newUXTestApp(t)
	app.detailsHidden = false
	app.selectedIssue = &linearapi.Issue{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"}
	app.updateDetailsView()
	if !strings.Contains(app.detailsPageView.GetText(true), "Alpha") {
		t.Fatal("the details pane never showed the issue, so this test proves nothing")
	}

	app.resetCachedState()

	if got := app.detailsPageView.GetText(true); strings.Contains(got, "Alpha") {
		t.Fatalf("details pane still shows the old workspace's issue: %q", got)
	}
	if app.GetSelectedIssue() != nil {
		t.Error("GetSelectedIssue() returned an issue after a reset")
	}
}

func TestResetCachedStateClearsTheOffScreenListTable(t *testing.T) {
	app, _ := newIssueUpdateTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"},
	})
	app.rebuildIssueRowModels()
	holdDetailFetches(t, app)

	app.renderIssueSections(map[IssuesSection]string{IssuesSectionList: "issue-1"})
	app.activeIssuesSection = IssuesSectionSearch
	if len(renderedTitles(app, IssuesSectionList)) == 0 {
		t.Fatal("the list was never painted, so this test proves nothing")
	}

	app.resetCachedState()

	if got := renderedTitles(app, IssuesSectionList); len(got) != 0 {
		t.Fatalf("the list still shows %v after a reset, want no rows", got)
	}
	if len(app.pendingSectionRenders) != 0 {
		t.Fatalf("pendingSectionRenders = %v, want empty", app.pendingSectionRenders)
	}
}

func TestAssignMe_DuringCurrentUserFetch(t *testing.T) {
	app := NewApp(linearapi.ClientConfig{}, config.Config{PageSize: 1, CacheTTL: time.Minute}, nil)
	stopBackgroundWorkOnCleanup(t, app)
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

	hammered := make(chan struct{})
	go func() {
		defer close(hammered)
		for i := 0; i < 50; i++ {
			dispatch()
		}
	}()
	loaded := make(chan struct{})
	fetchUser, generation := app.fetchCurrentUserFunc, app.resetGeneration.Load()
	go func() {
		defer close(loaded)
		app.loadCurrentUser(context.Background(), fetchUser, generation)
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
	app := NewApp(linearapi.ClientConfig{}, cfg, nil)
	stopBackgroundWorkOnCleanup(t, app)
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
	text := app.detailsPageView.GetText(true)
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

func TestRefreshIssues_PaintsOncePerRefreshNotOncePerPage(t *testing.T) {
	cfg := config.Config{PageSize: 1, CacheTTL: time.Minute}
	app := NewApp(linearapi.ClientConfig{}, cfg, nil)
	stopBackgroundWorkOnCleanup(t, app)
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

	waitForCondition(t, time.Second, func() bool {
		app.issuesMu.RLock()
		defer app.issuesMu.RUnlock()
		return len(app.issues) == 2
	})
	if got := renderedTitles(app, IssuesSectionList); !slices.Equal(got, []string{"First"}) {
		t.Fatalf("rendered titles mid-pagination = %v, want [First]", got)
	}

	close(blockLast)
	waitForRefreshCompletion(t, refreshDone)

	got := renderedTitles(app, IssuesSectionList)
	want := []string{"First", "Second", "Third"}
	if !slices.Equal(got, want) {
		t.Fatalf("rendered titles after pagination = %v, want %v", got, want)
	}
}

func TestRefreshIssues_KeepsSelectionAcrossPagination(t *testing.T) {
	cfg := config.Config{PageSize: 1, CacheTTL: time.Minute}
	app := NewApp(linearapi.ClientConfig{}, cfg, nil)
	stopBackgroundWorkOnCleanup(t, app)
	app.queueUpdateDraw = func(f func()) { f() }
	refreshDone := installRefreshCompletionHook(app)
	app.fetchIssueByID = func(_ context.Context, id string) (linearapi.Issue, error) {
		return linearapi.Issue{ID: id}, nil
	}

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

	got := renderedTitles(app, IssuesSectionList)
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

func TestAccumulateIssues_ReconcilesAfterAnOutsideSplice(t *testing.T) {
	app := newUXTestApp(t)
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

	app.issuesMu.Lock()
	app.issues = append(app.issues, spliced)
	app.issuesMu.Unlock()

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

func TestAccumulateIssues_KeepsTheListSorted(t *testing.T) {
	app := newUXTestApp(t)
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

func TestRenderAccumulatedIssues_KeepsTheHydratedSelection(t *testing.T) {
	app := newUXTestApp(t)
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

func TestRenderAccumulatedIssues_LeavesTheSearchTabAlone(t *testing.T) {
	app := newUXTestApp(t)
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

func TestRefreshIssues_PaintsDuringPagination(t *testing.T) {
	cfg := config.Config{PageSize: 1, CacheTTL: time.Minute}
	app := NewApp(linearapi.ClientConfig{}, cfg, nil)
	stopBackgroundWorkOnCleanup(t, app)
	app.fetchIssueByID = func(_ context.Context, id string) (linearapi.Issue, error) {
		return linearapi.Issue{ID: id}, nil
	}
	var paintedMu sync.Mutex
	var painted []string
	app.queueUpdateDraw = func(f func()) {
		f()
		paintedMu.Lock()
		painted = renderedTitles(app, IssuesSectionList)
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
			time.Sleep(issuesRepaintInterval + 50*time.Millisecond)
		}
		return linearapi.IssuePage{
			Issues:    []linearapi.Issue{pages[index]},
			HasNext:   index < len(pages)-1,
			EndCursor: stringPtr(fmt.Sprintf("cursor-%d", index)),
		}, nil
	}

	app.refreshIssues()

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

func TestRenderedTitles_SkipsGroupHeaders(t *testing.T) {
	cfg := config.Config{PageSize: 10, CacheTTL: time.Minute, GroupBy: GroupByStatus}
	app := NewApp(linearapi.ClientConfig{}, cfg, nil)
	stopBackgroundWorkOnCleanup(t, app)
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
	rows := app.rowsForSection(IssuesSectionList)
	headers := 0
	for _, row := range rows {
		if row.IsHeader {
			headers++
		}
	}
	if headers == 0 {
		t.Fatal("no group headers rendered, so this test proves nothing")
	}

	got := renderedTitles(app, IssuesSectionList)
	want := []string{"Second", "First"}
	if !slices.Equal(got, want) {
		t.Fatalf("renderedTitles = %v, want %v", got, want)
	}
}
