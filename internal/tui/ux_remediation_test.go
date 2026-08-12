package tui

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/zen-linear/zen-linear/internal/config"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

func newUXTestApp(t testing.TB) *App {
	t.Helper()
	app := NewApp(linearapi.ClientConfig{}, config.Config{
		PageSize: 1,
		CacheTTL: time.Minute,
	}, nil)
	app.queueUpdateDraw = func(f func()) { f() }
	app.teamUsers = []linearapi.User{{ID: "user-1", Name: "Test User"}}
	app.teamCycles = []linearapi.Cycle{{ID: "cycle-1", Name: "Test Cycle", Number: 1}}
	stopBackgroundWorkOnCleanup(t, app)
	return app
}

// stopBackgroundWorkOnCleanup keeps a debounce timer armed by a selection from
// outliving the test and repainting into the next one's app. Taking uiUpdateMu
// waits out a callback already inside QueueUpdateDraw; the generation bump stops
// every later one. Tests that build an App directly need this too.
func stopBackgroundWorkOnCleanup(t testing.TB, app *App) {
	t.Helper()
	t.Cleanup(func() {
		app.cancelDetailDebounce()
		app.cancelStatusFlash()
		// A test App never reaches Run, so the frame loop has no other way to
		// stop. Left running with queueUpdateDraw stubbed inline, it keeps
		// writing App fields while the next test reads them.
		if app.loading != nil {
			app.loading.stop()
		}
		app.uiUpdateMu.Lock()
		app.detailFetchGeneration.Add(1)
		// An issues refresh in flight guards on refreshGeneration, not the
		// detail one. Left current, it paints tables from its own goroutine
		// after the test returns, racing the next test's NewApp over tview's
		// package-level Styles.
		app.refreshGeneration.Add(1)
		app.searchFetchGeneration.Add(1)
		app.uiUpdateMu.Unlock()
	})
}

func TestPaletteController_FilterCommandsMatchesAllQueryTokens(t *testing.T) {
	commands := []Command{
		{ID: "sort_updated", Title: "Sort by updated", Keywords: []string{"sort", "updated", "recent"}},
		{ID: "sort_priority", Title: "Sort by priority", Keywords: []string{"sort", "priority", "urgent"}},
	}
	pc := NewPaletteController(commands)

	pc.SetQuery("sort priority")

	filtered := pc.Filtered()
	if len(filtered) != 1 {
		t.Fatalf("Filtered() length = %d, want 1", len(filtered))
	}
	if filtered[0].ID != "sort_priority" {
		t.Fatalf("Filtered()[0].ID = %q, want sort_priority", filtered[0].ID)
	}
}

func TestOpenSearchTabFocusesInput(t *testing.T) {
	app := newUXTestApp(t)

	app.openSearchTab()

	if app.activeIssuesSection != IssuesSectionSearch {
		t.Fatalf("activeIssuesSection = %v, want IssuesSectionSearch", app.activeIssuesSection)
	}
	if app.focusedPane != FocusIssues {
		t.Fatalf("focusedPane = %v, want FocusIssues", app.focusedPane)
	}
	if !app.searchInputFocused {
		t.Fatal("searchInputFocused = false, want the input focused")
	}
	if got := app.searchInput.GetLabel(); got != "/ " {
		t.Fatalf("search input label = %q, want %q", got, "/ ")
	}
	if !strings.Contains(app.issuesTabsTitle(true), "Search") {
		t.Fatalf("issues tab strip %q missing the Search tab", app.issuesTabsTitle(true))
	}
}

func TestDetailsCommentsTabAlwaysVisibleWithCount(t *testing.T) {
	app := newUXTestApp(t)

	issue := linearapi.Issue{ID: "issue-1", Identifier: "ABC-1", Title: "First", State: "Todo"}
	app.issuesMu.Lock()
	app.selectedIssue = &issue
	app.issuesMu.Unlock()
	app.updateDetailsView()

	if !app.detailsCommentsVisible {
		t.Fatal("comments tab hidden for an issue without comments")
	}
	if title := app.detailsTabsTitle(false); !strings.Contains(title, "Comments (0)") {
		t.Fatalf("details tab strip = %q, want a Comments (0) segment", title)
	}

	issue.Comments = []linearapi.Comment{{ID: "c-1", Body: "hi"}, {ID: "c-2", Body: "again"}}
	app.updateDetailsView()
	if title := app.detailsTabsTitle(false); !strings.Contains(title, "Comments (2)") {
		t.Fatalf("details tab strip = %q, want a Comments (2) segment", title)
	}

	app.issuesMu.Lock()
	app.selectedIssue = nil
	app.issuesMu.Unlock()
	app.updateDetailsView()
	if app.detailsCommentsVisible {
		t.Fatal("comments tab shown with no issue selected")
	}
}

func TestIssueContextLineShownInModals(t *testing.T) {
	app := newUXTestApp(t)
	issue := linearapi.Issue{ID: "issue-1", Identifier: "ZEN-9", Title: "A very important thing"}
	line := app.issueContextLine(issue)
	if !strings.Contains(line, "ZEN-9") || !strings.Contains(line, "A very important thing") {
		t.Fatalf("issueContextLine = %q, want identifier and title", line)
	}

	longTitle := strings.Repeat("long ", 20)
	if truncated := app.issueContextLine(linearapi.Issue{Identifier: "ZEN-10", Title: longTitle}); !strings.Contains(truncated, "…") {
		t.Fatalf("issueContextLine did not truncate a long title: %q", truncated)
	}

	// Issue-scoped shows carry the line; generic shows clear it.
	app.textInputModal.ShowWithContext("Set Due Date", "YYYY-MM-DD: ", "", line, func(string) {})
	if got := app.textInputModal.fm.contextText; got != line {
		t.Fatalf("text input context = %q, want %q", got, line)
	}
	app.textInputModal.Hide()
	app.textInputModal.Show("Filter Due Date", "YYYY-MM-DD: ", "", func(string) {})
	if got := app.textInputModal.fm.contextText; got != "" {
		t.Fatalf("filter context = %q, want empty", got)
	}
	app.textInputModal.Hide()

	app.pickerModal.ShowWithContext("Select Status", line, []PickerItem{{ID: "s", Label: "Todo"}}, func(PickerItem) {})
	if got := app.pickerModal.contextView.GetText(true); !strings.Contains(got, "ZEN-9") {
		t.Fatalf("picker context = %q, want the issue line", got)
	}
	app.pickerModal.Hide()
}

func TestSettingsModalShowsAndBuildsSearchDebounceSetting(t *testing.T) {
	app := newUXTestApp(t)
	app.config.SearchDebounce = 450 * time.Millisecond
	modal := app.settingsModal

	modal.Show()
	defer modal.Hide()

	if got := modal.searchDebounceField.GetText(); got != "450ms" {
		t.Fatalf("search debounce field = %q, want 450ms", got)
	}

	modal.searchDebounceField.SetText("600ms")
	settings, err := modal.settingsFromForm()
	if err != nil {
		t.Fatalf("settingsFromForm() error: %v", err)
	}
	if settings.SearchDebounce != "600ms" {
		t.Fatalf("SearchDebounce = %q, want 600ms", settings.SearchDebounce)
	}
}

func TestSettingsModalShowsAndBuildsDefaultNavigationSettings(t *testing.T) {
	app := newUXTestApp(t)
	app.config.DefaultTeam = "NEX"
	app.config.DefaultProject = "Website"
	modal := app.settingsModal

	modal.Show()
	defer modal.Hide()

	if got := modal.defaultTeamField.GetText(); got != "NEX" {
		t.Fatalf("default team field = %q, want NEX", got)
	}
	if got := modal.defaultProjectField.GetText(); got != "Website" {
		t.Fatalf("default project field = %q, want Website", got)
	}

	modal.defaultTeamField.SetText("ENG")
	modal.defaultProjectField.SetText("Mobile App")
	settings, err := modal.settingsFromForm()
	if err != nil {
		t.Fatalf("settingsFromForm() error: %v", err)
	}
	if settings.DefaultTeam != "ENG" {
		t.Fatalf("DefaultTeam = %q, want ENG", settings.DefaultTeam)
	}
	if settings.DefaultProject != "Mobile App" {
		t.Fatalf("DefaultProject = %q, want Mobile App", settings.DefaultProject)
	}
}

func TestEditLabelsModalShowsFocusAndTogglesWithSpaceAndT(t *testing.T) {
	app := newUXTestApp(t)
	modal := app.editLabelsModal
	var saved []string
	labels := []linearapi.IssueLabel{
		{ID: "bug", Name: "Bug"},
		{ID: "feature", Name: "Feature"},
	}
	modal.Show("issue-1", nil, labels, "ZEN-1 · Test issue", func(_ string, labelIDs []string) {
		saved = append([]string(nil), labelIDs...)
	})

	first, _ := modal.list.GetItemText(0)
	if first != "> ( ) Bug" {
		t.Fatalf("initial first row = %q, want focused unchecked row", first)
	}

	modal.HandleKey(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone))
	first, _ = modal.list.GetItemText(0)
	if first != "> (x) Bug" {
		t.Fatalf("after space first row = %q, want focused checked row", first)
	}

	modal.HandleKey(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	first, _ = modal.list.GetItemText(0)
	second, _ := modal.list.GetItemText(1)
	if first != "  (x) Bug" || second != "> ( ) Feature" {
		t.Fatalf("rows after moving focus = %q / %q, want focus marker on second row", first, second)
	}

	modal.HandleKey(tcell.NewEventKey(tcell.KeyRune, 't', tcell.ModNone))
	second, _ = modal.list.GetItemText(1)
	if second != "> (x) Feature" {
		t.Fatalf("after t second row = %q, want focused checked row", second)
	}

	modal.HandleKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	sort.Strings(saved)
	if !reflect.DeepEqual(saved, []string{"bug", "feature"}) {
		t.Fatalf("saved label IDs = %#v, want bug and feature", saved)
	}
}

func TestShowParentIssuePickerExcludesSelectedIssueAndDescendants(t *testing.T) {
	app := newUXTestApp(t)
	selected := linearapi.Issue{
		ID:         "selected",
		Identifier: "LTUI-1",
		Title:      "Selected",
		Children: []linearapi.IssueChildRef{
			{ID: "child", Identifier: "LTUI-2", Title: "Child"},
		},
	}
	app.issuesMu.Lock()
	app.selectedIssue = &selected
	app.issues = []linearapi.Issue{
		selected,
		{ID: "child", Identifier: "LTUI-2", Title: "Child"},
		{ID: "sibling", Identifier: "LTUI-3", Title: "Sibling"},
	}
	app.issuesMu.Unlock()

	app.ShowParentIssuePicker("", func(parentID string) {})

	if len(app.pickerModal.items) != 1 {
		t.Fatalf("picker item count = %d, want 1", len(app.pickerModal.items))
	}
	if got := app.pickerModal.items[0].ID; got != "sibling" {
		t.Fatalf("picker item ID = %q, want sibling", got)
	}
}

func TestDestructiveCommandsOpenConfirmationBeforeMutation(t *testing.T) {
	app := newUXTestApp(t)
	parent := &linearapi.IssueRef{ID: "parent-1", Identifier: "LTUI-1", Title: "Parent"}
	issue := linearapi.Issue{ID: "issue-1", Identifier: "LTUI-2", Title: "Child", Parent: parent}
	app.issuesMu.Lock()
	app.selectedIssue = &issue
	app.issuesMu.Unlock()

	archive := findCommandByID(DefaultCommands(app), "archive")
	if archive == nil {
		t.Fatal("archive command not found")
	}
	archive.Run(app)
	if !app.pages.HasPage("confirmation") {
		t.Fatal("archive command did not open confirmation modal")
	}

	app.confirmationModal.Hide()
	removeParent := findCommandByID(DefaultCommands(app), "remove_parent")
	if removeParent == nil {
		t.Fatal("remove_parent command not found")
	}
	removeParent.Run(app)
	if !app.pages.HasPage("confirmation") {
		t.Fatal("remove parent command did not open confirmation modal")
	}
}

func TestNoOpCommandsShowStatusFeedback(t *testing.T) {
	app := newUXTestApp(t)

	openBrowser := findCommandByID(DefaultCommands(app), "open_browser")
	if openBrowser == nil {
		t.Fatal("open_browser command not found")
	}
	openBrowser.Run(app)
	if got := statusText(app); !strings.Contains(got, "No issue selected") {
		t.Fatalf("status after open_browser without issue = %q, want no issue feedback", got)
	}

	app.issuesMu.Lock()
	app.selectedIssue = &linearapi.Issue{ID: "issue-1", Identifier: "LTUI-1", Title: "No parent"}
	app.issuesMu.Unlock()
	viewParent := findCommandByID(DefaultCommands(app), "view_parent")
	if viewParent == nil {
		t.Fatal("view_parent command not found")
	}
	viewParent.Run(app)
	if got := statusText(app); !strings.Contains(got, "No parent issue") {
		t.Fatalf("status after view_parent without parent = %q, want no parent feedback", got)
	}
}

func TestAgentOutputModalFailureSetsErrorStatusAndFinalSummary(t *testing.T) {
	app := newUXTestApp(t)
	modal := app.agentOutputModal
	modal.Show(" Cursor Output ", func() {})

	modal.FailRun(fmt.Errorf("agent exited: exit status 1: invalid model"))

	if got := modal.statusView.GetText(true); !strings.Contains(got, "Status: Error") {
		t.Fatalf("status = %q, want error status", got)
	}
	final := modal.finalView.GetText(true)
	if !strings.Contains(final, "Agent run failed") {
		t.Fatalf("final output = %q, want failure summary", final)
	}
	if !strings.Contains(final, "Check agent provider/model settings") {
		t.Fatalf("final output = %q, want model/provider guidance", final)
	}
}
