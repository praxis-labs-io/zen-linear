package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

var errNotReachable = errors.New("linear unreachable")

// titleColumn is the index of the title cell in DefaultIssueColumns.
const titleColumn = 3

// newIssueUpdateTestApp returns an app seeded with issues and a channel that
// fires after each queued draw completes. A selection move fetches details on
// a background goroutine; a test that triggers one must waitForDraw before
// finishing, or that goroutine's table work races the next test's NewApp over
// tview's package-level styles.
func newIssueUpdateTestApp(t *testing.T, issues []linearapi.Issue) (*App, <-chan struct{}) {
	t.Helper()
	app := newUXTestApp(t)
	drawn := make(chan struct{}, 8)
	app.queueUpdateDraw = func(f func()) {
		f()
		select {
		case drawn <- struct{}{}:
		default:
		}
	}
	app.fetchIssuesPage = func(ctx context.Context, params linearapi.FetchIssuesParams, after *string) (linearapi.IssuePage, error) {
		t.Error("a single-issue update refetched the whole list")
		return linearapi.IssuePage{}, nil
	}
	app.fetchIssueByID = func(ctx context.Context, issueID string) (linearapi.Issue, error) {
		return linearapi.Issue{ID: issueID, Identifier: "FETCHED"}, nil
	}
	app.issuesMu.Lock()
	app.issues = issues
	app.selectedIssue = &app.issues[0]
	app.issuesMu.Unlock()
	app.rebuildIssuesTables(issues[0].ID)
	return app, drawn
}

// scopedTestApp narrows the list to a team, so the scope check has something
// to check. On All Issues it is skipped.
func scopedTestApp(t *testing.T, issues []linearapi.Issue) (*App, <-chan struct{}) {
	t.Helper()
	app, drawn := newIssueUpdateTestApp(t, issues)
	app.selectedNavigation = &NavigationNode{IsTeam: true, TeamID: "team-1"}
	return app, drawn
}

func waitForDraw(t *testing.T, drawn <-chan struct{}) {
	t.Helper()
	select {
	case <-drawn:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a queued draw")
	}
}

func assertIssueCount(t *testing.T, app *App, want int) {
	t.Helper()
	app.issuesMu.RLock()
	got := len(app.issues)
	app.issuesMu.RUnlock()
	if got != want {
		t.Fatalf("issue count = %d, want %d", got, want)
	}
}

// assertSelectionNotAliased fails when selectedIssue points into the a.issues
// backing array, which in-place sorts and splices silently repoint.
func assertSelectionNotAliased(t *testing.T, app *App) {
	t.Helper()
	app.issuesMu.RLock()
	defer app.issuesMu.RUnlock()
	for i := range app.issues {
		if app.selectedIssue == &app.issues[i] {
			t.Fatal("selectedIssue aliases the issues backing array")
		}
	}
}

func TestApplyIssueUpdate_RepaintsOneRowWhenNothingMoves(t *testing.T) {
	app, _ := newIssueUpdateTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"},
		{ID: "issue-2", Identifier: "LIN-2", Title: "Beta"},
	})

	// Park the cursor and the horizontal scroll somewhere a full re-render
	// would reset.
	table := app.tableForSection(IssuesSectionList)
	table.Select(2, 0)
	table.SetOffset(0, 2)

	updated := linearapi.Issue{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha renamed"}
	app.applyIssueUpdate(updated)

	if got := table.GetCell(1, titleColumn).Text; got != "Alpha renamed" {
		t.Fatalf("row 1 title = %q, want %q", got, "Alpha renamed")
	}
	if row, _ := table.GetSelection(); row != 2 {
		t.Fatalf("selected row = %d, want 2", row)
	}
	if _, columnOffset := table.GetOffset(); columnOffset != 2 {
		t.Fatalf("column offset = %d, want 2", columnOffset)
	}
	app.issuesMu.RLock()
	defer app.issuesMu.RUnlock()
	if app.issues[0].Title != "Alpha renamed" {
		t.Fatalf("local issue title = %q, want %q", app.issues[0].Title, "Alpha renamed")
	}
}

func TestApplyIssueUpdate_KeepsDetailsThePaneLoaded(t *testing.T) {
	app, _ := newIssueUpdateTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"},
	})
	// The pane's full fetch lives only in selectedIssue; the list entry never
	// carries comments or subscribers.
	app.issuesMu.Lock()
	app.selectedIssue = &linearapi.Issue{
		ID: "issue-1", Identifier: "LIN-1", Title: "Alpha",
		Comments:    []linearapi.Comment{{ID: "comment-1", Body: "keep me"}},
		Subscribers: []linearapi.User{{ID: "user-9"}},
	}
	app.issuesMu.Unlock()

	app.applyIssueUpdate(linearapi.Issue{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha renamed"})

	app.issuesMu.RLock()
	selected := app.selectedIssue
	app.issuesMu.RUnlock()
	if selected == nil || selected.Title != "Alpha renamed" {
		t.Fatalf("selected issue = %#v, want the renamed issue", selected)
	}
	if len(selected.Comments) != 1 {
		t.Fatalf("comments = %#v, want the loaded comment kept", selected.Comments)
	}
	if len(selected.Subscribers) != 1 {
		t.Fatalf("subscribers = %#v, want the loaded subscriber kept", selected.Subscribers)
	}
	assertSelectionNotAliased(t, app)
}

func TestApplyIssueUpdate_ConsecutiveEditsFollowTheEditedIssue(t *testing.T) {
	base := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	app, _ := newIssueUpdateTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha", UpdatedAt: base.Add(3 * time.Hour)},
		{ID: "issue-2", Identifier: "LIN-2", Title: "Beta", UpdatedAt: base.Add(2 * time.Hour)},
		{ID: "issue-3", Identifier: "LIN-3", Title: "Gamma", UpdatedAt: base.Add(time.Hour)},
	})
	app.issuesMu.Lock()
	app.selectedIssue = &linearapi.Issue{ID: "issue-3", Identifier: "LIN-3", Title: "Gamma"}
	app.issuesMu.Unlock()

	// The first edit re-sorts the slice under the selection; the second must
	// still land on the same issue.
	app.applyIssueUpdate(linearapi.Issue{ID: "issue-3", Identifier: "LIN-3", Title: "Gamma", UpdatedAt: base.Add(4 * time.Hour)})
	app.applyIssueUpdate(linearapi.Issue{ID: "issue-3", Identifier: "LIN-3", Title: "Gamma edited", UpdatedAt: base.Add(5 * time.Hour)})

	app.issuesMu.RLock()
	selected := app.selectedIssue
	app.issuesMu.RUnlock()
	if selected == nil || selected.ID != "issue-3" {
		t.Fatalf("selected issue = %#v, want issue-3", selected)
	}
	if selected.Title != "Gamma edited" {
		t.Fatalf("selected title = %q, want %q", selected.Title, "Gamma edited")
	}
	assertSelectionNotAliased(t, app)
}

func TestApplyIssueUpdate_ReflectsEditToAnIssueOutsideTheList(t *testing.T) {
	app, _ := newIssueUpdateTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"},
	})
	// A search result outside the loaded pages: it owns the pane and a search
	// row, but is absent from a.issues.
	app.searchIssues = []linearapi.Issue{{ID: "search-1", Identifier: "LIN-9", Title: "Old title"}}
	app.searchIssueRows, app.searchIDToIssue = BuildIssueRows(app.searchIssues, app.expandedState)
	app.issuesMu.Lock()
	app.selectedIssue = &linearapi.Issue{
		ID: "search-1", Identifier: "LIN-9", Title: "Old title",
		Comments: []linearapi.Comment{{ID: "comment-1", Body: "keep"}},
	}
	app.issuesMu.Unlock()

	app.applyIssueUpdate(linearapi.Issue{ID: "search-1", Identifier: "LIN-9", Title: "New title"})

	app.issuesMu.RLock()
	selected := app.selectedIssue
	app.issuesMu.RUnlock()
	if selected == nil || selected.Title != "New title" {
		t.Fatalf("selected issue = %#v, want the edited title", selected)
	}
	if len(selected.Comments) != 1 {
		t.Fatalf("comments = %#v, want kept", selected.Comments)
	}
	if app.searchIssues[0].Title != "New title" {
		t.Fatalf("search model title = %q, want %q", app.searchIssues[0].Title, "New title")
	}
	if got := app.searchResultsTable.GetCell(1, titleColumn).Text; got != "New title" {
		t.Fatalf("search row title = %q, want %q", got, "New title")
	}
}

func TestApplyIssueUpdate_AddsAnIssueTheEditBroughtIntoScope(t *testing.T) {
	app, drawn := scopedTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"},
	})
	checked := make(chan string, 1)
	app.issueMatchesScopeFunc = func(ctx context.Context, params linearapi.FetchIssuesParams, issueID string) (bool, error) {
		checked <- issueID
		return true, nil
	}

	app.applyIssueUpdate(linearapi.Issue{ID: "issue-2", Identifier: "LIN-2", Title: "Beta"})

	select {
	case issueID := <-checked:
		if issueID != "issue-2" {
			t.Fatalf("checked issue = %q, want issue-2", issueID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the scope check")
	}
	// One draw applies the verdict, a second lands the selection's detail
	// fetch.
	waitForDraw(t, drawn)
	waitForDraw(t, drawn)
	assertIssueCount(t, app, 2)
}

func TestApplyIssueUpdate_DropsAnIssueTheEditPushedOutOfScope(t *testing.T) {
	app, drawn := scopedTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"},
		{ID: "issue-2", Identifier: "LIN-2", Title: "Beta"},
	})
	app.issueMatchesScopeFunc = func(ctx context.Context, params linearapi.FetchIssuesParams, issueID string) (bool, error) {
		return false, nil
	}

	app.applyIssueUpdate(linearapi.Issue{ID: "issue-2", Identifier: "LIN-2", Title: "Beta moved"})

	waitForDraw(t, drawn)
	assertIssueCount(t, app, 1)
}

func TestConfirmIssueInScope_DiscardsVerdictAfterRefresh(t *testing.T) {
	app, drawn := scopedTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"},
		{ID: "issue-2", Identifier: "LIN-2", Title: "Beta"},
	})
	release := make(chan struct{})
	app.issueMatchesScopeFunc = func(ctx context.Context, params linearapi.FetchIssuesParams, issueID string) (bool, error) {
		<-release
		return false, nil
	}

	app.applyIssueUpdate(linearapi.Issue{ID: "issue-2", Identifier: "LIN-2", Title: "Beta edited"})
	// A refresh re-scopes the list while the answer is in flight; the stale
	// verdict must not touch it.
	app.refreshGeneration.Add(1)
	close(release)

	waitForDraw(t, drawn)
	assertIssueCount(t, app, 2)
}

func TestApplyIssueInsert_KeepsTheRowWhenTheScopeCheckFails(t *testing.T) {
	app, drawn := scopedTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"},
	})
	checked := make(chan struct{}, 1)
	app.issueMatchesScopeFunc = func(ctx context.Context, params linearapi.FetchIssuesParams, issueID string) (bool, error) {
		checked <- struct{}{}
		return false, errNotReachable
	}

	app.applyIssueInsert(linearapi.Issue{ID: "issue-2", Identifier: "LIN-2", Title: "Beta"})

	<-checked
	// The insert moved the selection, which fetches details in background.
	waitForDraw(t, drawn)
	// A failed check must not take away a row the user is looking at.
	assertIssueCount(t, app, 2)
}

func TestConfirmIssueInScope_SkippedOnTheUnscopedList(t *testing.T) {
	app, _ := newIssueUpdateTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"},
	})
	app.issueMatchesScopeFunc = func(ctx context.Context, params linearapi.FetchIssuesParams, issueID string) (bool, error) {
		t.Error("scope check ran with no filter to check against")
		return true, nil
	}

	app.applyIssueUpdate(linearapi.Issue{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha renamed"})
	time.Sleep(100 * time.Millisecond)
}

func TestApplyIssueRemoval_KeepsCursorWhenAnotherRowIsRemoved(t *testing.T) {
	app, _ := newIssueUpdateTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"},
		{ID: "issue-2", Identifier: "LIN-2", Title: "Beta"},
		{ID: "issue-3", Identifier: "LIN-3", Title: "Gamma"},
	})

	app.applyIssueRemoval("issue-2")

	app.issuesMu.RLock()
	remaining := make([]string, 0, len(app.issues))
	for _, issue := range app.issues {
		remaining = append(remaining, issue.ID)
	}
	selected := app.selectedIssue
	app.issuesMu.RUnlock()

	if len(remaining) != 2 || remaining[0] != "issue-1" || remaining[1] != "issue-3" {
		t.Fatalf("remaining issues = %v, want [issue-1 issue-3]", remaining)
	}
	// The removed row was not the selected one; the cursor stays put.
	if selected == nil || selected.ID != "issue-1" {
		t.Fatalf("selected issue = %#v, want issue-1", selected)
	}
	table := app.tableForSection(IssuesSectionList)
	if got := table.GetCell(2, titleColumn).Text; got != "Gamma" {
		t.Fatalf("row 2 title = %q, want Gamma", got)
	}
}

func TestApplyIssueRemoval_SelectedRowLandsOnSuccessor(t *testing.T) {
	app, drawn := newIssueUpdateTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"},
		{ID: "issue-2", Identifier: "LIN-2", Title: "Beta"},
		{ID: "issue-3", Identifier: "LIN-3", Title: "Gamma"},
	})
	app.issuesMu.Lock()
	app.selectedIssue = &linearapi.Issue{ID: "issue-2", Identifier: "LIN-2", Title: "Beta"}
	app.issuesMu.Unlock()

	app.applyIssueRemoval("issue-2")

	// The successor selection fetches details in background.
	waitForDraw(t, drawn)
	app.issuesMu.RLock()
	selected := app.selectedIssue
	app.issuesMu.RUnlock()
	if selected == nil || selected.ID != "issue-3" {
		t.Fatalf("selected issue = %#v, want the successor issue-3", selected)
	}
}

// Removing the last row leaves the list on screen saying it is empty, rather
// than bouncing the user somewhere else, and drops the selection so status,
// assign and archive stop acting on a row that is gone.
func TestApplyIssueRemoval_LeavesTheEmptiedListOnScreen(t *testing.T) {
	app, _ := newIssueUpdateTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Only"},
	})
	app.rebuildIssueRowModels()
	app.updateIssuesColumnLayout()
	app.issuesMu.Lock()
	app.selectedIssue = &linearapi.Issue{ID: "issue-1", Identifier: "LIN-1", Title: "Only"}
	app.issuesMu.Unlock()

	app.applyIssueRemoval("issue-1")

	if app.activeIssuesSection != IssuesSectionList {
		t.Fatalf("emptying the list moved the pane to %v", app.activeIssuesSection)
	}
	if got := app.listIssuesTable.GetCell(1, titleColumn).Text; !strings.Contains(got, "No issues") {
		t.Fatalf("the emptied list renders %q, want its empty row", got)
	}
	app.issuesMu.RLock()
	defer app.issuesMu.RUnlock()
	if app.selectedIssue != nil {
		t.Fatalf("selected issue = %#v, want nil with nothing left in the section", app.selectedIssue)
	}
}

func TestApplyIssueRemoval_DetachesTheRowFromItsParent(t *testing.T) {
	parentRef := &linearapi.IssueRef{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"}
	app, _ := newIssueUpdateTestApp(t, []linearapi.Issue{
		{
			ID: "issue-1", Identifier: "LIN-1", Title: "Alpha",
			Children: []linearapi.IssueChildRef{{ID: "issue-2", Identifier: "LIN-2", Title: "Beta"}},
		},
		{ID: "issue-2", Identifier: "LIN-2", Title: "Beta", Parent: parentRef},
	})

	app.applyIssueRemoval("issue-2")

	app.issuesMu.RLock()
	defer app.issuesMu.RUnlock()
	if len(app.issues[0].Children) != 0 {
		t.Fatalf("parent children = %#v, want the archived child dropped", app.issues[0].Children)
	}
}

func TestApplyIssueUpdate_MovesTheChildRefBetweenParents(t *testing.T) {
	oldParent := &linearapi.IssueRef{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"}
	app, _ := newIssueUpdateTestApp(t, []linearapi.Issue{
		{
			ID: "issue-1", Identifier: "LIN-1", Title: "Alpha",
			Children: []linearapi.IssueChildRef{{ID: "issue-3", Identifier: "LIN-3", Title: "Gamma"}},
		},
		{ID: "issue-2", Identifier: "LIN-2", Title: "Beta"},
		{ID: "issue-3", Identifier: "LIN-3", Title: "Gamma", Parent: oldParent},
	})

	newParent := &linearapi.IssueRef{ID: "issue-2", Identifier: "LIN-2", Title: "Beta"}
	app.applyIssueUpdate(linearapi.Issue{
		ID: "issue-3", Identifier: "LIN-3", Title: "Gamma", Parent: newParent,
	})

	app.issuesMu.RLock()
	defer app.issuesMu.RUnlock()
	byID := make(map[string]linearapi.Issue, len(app.issues))
	for _, issue := range app.issues {
		byID[issue.ID] = issue
	}
	if len(byID["issue-1"].Children) != 0 {
		t.Fatalf("old parent children = %#v, want empty", byID["issue-1"].Children)
	}
	if children := byID["issue-2"].Children; len(children) != 1 || children[0].ID != "issue-3" {
		t.Fatalf("new parent children = %#v, want issue-3", children)
	}
}

func TestApplyIssueUpdate_DropsTheChildRefWhenTheParentIsRemoved(t *testing.T) {
	parentRef := &linearapi.IssueRef{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"}
	app, _ := newIssueUpdateTestApp(t, []linearapi.Issue{
		{
			ID: "issue-1", Identifier: "LIN-1", Title: "Alpha",
			Children: []linearapi.IssueChildRef{{ID: "issue-2", Identifier: "LIN-2", Title: "Beta"}},
		},
		{ID: "issue-2", Identifier: "LIN-2", Title: "Beta", Parent: parentRef},
	})

	app.applyIssueUpdate(linearapi.Issue{ID: "issue-2", Identifier: "LIN-2", Title: "Beta"})

	app.issuesMu.RLock()
	defer app.issuesMu.RUnlock()
	if len(app.issues[0].Children) != 0 {
		t.Fatalf("parent children = %#v, want empty after the child left", app.issues[0].Children)
	}
}

func TestApplyIssueInsert_AddsTheCreatedRowAndSelectsIt(t *testing.T) {
	app, drawn := newIssueUpdateTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"},
	})

	app.applyIssueInsert(linearapi.Issue{ID: "issue-2", Identifier: "LIN-2", Title: "Beta"})

	// The insert selects the new issue, which fetches details in background.
	waitForDraw(t, drawn)
	assertIssueCount(t, app, 2)
	app.issuesMu.RLock()
	selected := app.selectedIssue
	app.issuesMu.RUnlock()
	if selected == nil || selected.ID != "issue-2" {
		t.Fatalf("selected issue = %#v, want issue-2", selected)
	}
	if _, ok := app.listIDToIssue["issue-2"]; !ok {
		t.Fatal("created issue missing from the row model")
	}
}

func TestApplyIssueInsert_LinksASubIssueToItsParent(t *testing.T) {
	app, drawn := newIssueUpdateTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"},
	})

	app.applyIssueInsert(linearapi.Issue{
		ID: "issue-2", Identifier: "LIN-2", Title: "Beta",
		Parent: &linearapi.IssueRef{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"},
	})

	waitForDraw(t, drawn)
	app.issuesMu.RLock()
	defer app.issuesMu.RUnlock()
	for _, issue := range app.issues {
		if issue.ID != "issue-1" {
			continue
		}
		if len(issue.Children) != 1 || issue.Children[0].ID != "issue-2" {
			t.Fatalf("parent children = %#v, want issue-2", issue.Children)
		}
		return
	}
	t.Fatal("parent issue missing from the list")
}

func TestExpandAllKeepsGroupingAndCoversAllTab(t *testing.T) {
	app, _ := newIssueUpdateTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha", State: "Todo", AssigneeID: "user-1", Assignee: "Me"},
		{ID: "issue-2", Identifier: "LIN-2", Title: "Beta", State: "Done"},
	})
	app.config.GroupBy = "status"
	app.currentUser = &linearapi.User{ID: "user-1", Name: "Me"}

	command := findCommandByID(DefaultCommands(app), "expand_all")
	if command == nil {
		t.Fatal("expand_all command not found")
	}
	command.Run(app)

	hasHeader := func(rows []IssueRow) bool {
		for _, row := range rows {
			if row.IsHeader {
				return true
			}
		}
		return false
	}
	if !hasHeader(app.listIssueRows) {
		t.Fatal("expand_all dropped grouping from the list rows")
	}
	if _, pending := app.pendingSectionRenders[IssuesSectionList]; !pending && app.activeIssuesSection != IssuesSectionList {
		t.Fatal("expand_all left the list neither painted nor deferred")
	}
}

// A refresh while search results are showing rebuilds the list off screen.
// Painting it there costs a table's worth of cells nobody can see and resets
// its selection, so the paint waits for the section to come back.
func TestRenderIssueSections_DefersTheOffScreenListUntilShown(t *testing.T) {
	app, _ := newIssueUpdateTestApp(t, []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha"},
		{ID: "issue-2", Identifier: "LIN-2", Title: "Beta"},
	})
	app.rebuildIssueRowModels()
	app.listIssuesTable.Clear()

	app.activeIssuesSection = IssuesSectionSearch
	app.renderIssueSections(map[IssuesSection]string{IssuesSectionList: "issue-2"})

	if _, pending := app.pendingSectionRenders[IssuesSectionList]; !pending {
		t.Fatal("the list rendered while off screen")
	}
	if got := app.listIssuesTable.GetCell(2, titleColumn).Text; got != "" {
		t.Fatalf("the off-screen list already shows %q", got)
	}

	app.activeIssuesSection = IssuesSectionList
	app.updateIssuesColumnLayout()

	if _, pending := app.pendingSectionRenders[IssuesSectionList]; pending {
		t.Fatal("the list is still pending after being shown")
	}
	if got := app.listIssuesTable.GetCell(2, titleColumn).Text; got != "Beta" {
		t.Fatalf("list row 2 title = %q, want Beta", got)
	}
}

// Pagination sorts a.issues in place between paints, and the id maps used to
// point into it. Skimming inside that window resolved a row to whichever issue
// had sorted into the slot, so the details pane and every command reading the
// selection landed on the wrong issue.
func TestSkimmingDuringPagination_SelectsTheIssueTheRowNames(t *testing.T) {
	app := newUXTestApp(t)
	holdDetailFetches(t, app)
	app.sortFields = []SortField{SortByPriority}

	// Spare capacity so the merged page cannot reallocate. The sort then
	// permutes the same array the maps index, which is the failing case.
	firstPage := make([]linearapi.Issue, 0, 8)
	firstPage = append(firstPage,
		linearapi.Issue{ID: "issue-c", Identifier: "ZNL-3", Title: "Gamma", Priority: 3},
		linearapi.Issue{ID: "issue-d", Identifier: "ZNL-4", Title: "Delta", Priority: 4},
	)
	app.updateIssuesData(firstPage)

	merge := &pageMerge{seen: make(map[string]bool)}
	app.issuesMu.RLock()
	merge.reset(app.issues)
	app.issuesMu.RUnlock()

	// A page that sorts ahead of everything on screen, inside the repaint
	// budget: accumulated, not yet painted.
	app.accumulateIssues([]linearapi.Issue{
		{ID: "issue-a", Identifier: "ZNL-1", Title: "Alpha", Priority: 1},
		{ID: "issue-b", Identifier: "ZNL-2", Title: "Beta", Priority: 2},
	}, merge)

	pressInIssuesTable(app, tcell.KeyRune, 'j')

	row, _ := app.tableForSection(IssuesSectionList).GetSelection()
	want := app.listIssueRows[row-1].IssueID
	selected := app.GetSelectedIssue()
	if selected == nil || selected.ID != want {
		t.Fatalf("selected issue = %#v, want the %s the cursor is on", selected, want)
	}
}

// The maps hold pointers, so indexing the live list lets a later in-place sort
// repoint every entry. They index a snapshot instead.
func TestRebuildIssueRowModels_IndexesASnapshotNotTheLiveList(t *testing.T) {
	app := newUXTestApp(t)
	app.issuesMu.Lock()
	app.issues = []linearapi.Issue{
		{ID: "issue-1", Identifier: "ZNL-1", Title: "Alpha"},
		{ID: "issue-2", Identifier: "ZNL-2", Title: "Beta"},
	}
	app.issuesMu.Unlock()

	app.rebuildIssueRowModels()

	app.issuesMu.RLock()
	defer app.issuesMu.RUnlock()
	for id, issue := range app.listIDToIssue {
		for i := range app.issues {
			if issue == &app.issues[i] {
				t.Fatalf("listIDToIssue[%q] aliases the live issues array", id)
			}
		}
	}
}
