package tui

import (
	"strings"
	"testing"

	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// contextTestApp puts a project of a known team on screen, so the title has a
// team key to prefix and a scope to name.
func contextTestApp(t *testing.T) *App {
	t.Helper()
	app := newUXTestApp(t)
	app.rebuildNavigationTree([]linearapi.Team{{ID: "team-1", Key: "ZNL", Name: "Zen Linear"}}, nil)
	app.selectedNavigation = &NavigationNode{
		ID:        "project-1",
		Text:      "Polish & Bugs",
		TeamID:    "team-1",
		IsProject: true,
	}
	return app
}

func TestIssuesTitleNamesTheListAndItsCount(t *testing.T) {
	app := contextTestApp(t)
	app.listIssueRows = []IssueRow{{IssueID: "issue-1"}, {IssueID: "issue-2"}}

	if got := app.issuesTitleLabel(); got != "ZNL · Polish & Bugs (2)" {
		t.Errorf("title = %q, want the list named with its count", got)
	}
}

// The team's own row already says the team, so the key would be the same fact
// twice.
func TestIssuesTitleLeavesATeamRowUnprefixed(t *testing.T) {
	app := contextTestApp(t)
	app.selectedNavigation = &NavigationNode{ID: "team-1", Text: "Zen Linear", TeamID: "team-1", IsTeam: true}

	if got := app.issuesTitleLabel(); strings.Contains(got, "ZNL ·") {
		t.Errorf("title = %q, want no key in front of the team's own name", got)
	}
}

func TestIssuesTitleNamesSearchWhileAQueryIsLive(t *testing.T) {
	app := contextTestApp(t)
	app.activeIssuesSection = IssuesSectionSearch

	if got := app.issuesTitleLabel(); got != "Search" {
		t.Errorf("title with no results = %q, want %q", got, "Search")
	}

	app.searchIssueRows = []IssueRow{{IssueID: "issue-1"}}
	if got := app.issuesTitleLabel(); got != "Search (1)" {
		t.Errorf("title = %q, want the result count", got)
	}
}

func TestIssuesContextNamesSortAndFilters(t *testing.T) {
	app := contextTestApp(t)
	app.richFilters = IssueFilters{AssigneeID: "user-1", AssigneeName: "drew"}

	text := stripTags(app.issuesContextText(120))

	for _, want := range []string{"Sort: ", "Filters: "} {
		if !strings.Contains(text, want) {
			t.Errorf("context = %q, want it to carry %q", text, want)
		}
	}
}

// A narrow pane gives up the sort first: the rows themselves show the order,
// and the filters cannot be read anywhere else.
func TestIssuesContextDropsSortBeforeFilters(t *testing.T) {
	app := contextTestApp(t)
	app.richFilters = IssueFilters{AssigneeID: "user-1", AssigneeName: "drew"}

	text := stripTags(app.issuesContextText(25))

	if strings.Contains(text, "Sort:") {
		t.Errorf("context = %q at width 25, want the sort dropped", text)
	}
	if !strings.Contains(text, "Filters: ") {
		t.Errorf("context = %q at width 25, want the filters kept", text)
	}
}

// A search takes neither the tree's scope, the filters, nor the sort chain, so
// the context line would be false about its results.
func TestSearchResultsCarryNoContextLine(t *testing.T) {
	app := contextTestApp(t)
	app.richFilters = IssueFilters{AssigneeID: "user-1", AssigneeName: "drew"}
	app.activeIssuesSection = IssuesSectionSearch

	if got := app.issuesContextText(120); got != "" {
		t.Errorf("search context = %q, want nothing", got)
	}
}

// TestIssuesContextDrawsOnTheTopBorder covers the line landing in the border
// itself rather than costing the list a row, and staying clear of the title
// sharing that row.
func TestIssuesContextDrawsOnTheTopBorder(t *testing.T) {
	app := contextTestApp(t)
	app.updateAllPaneTitles()

	lines := drawPrimitive(t, app.listIssuesTable, 80)

	top := lines[0]
	if !strings.Contains(top, "ZNL · Polish & Bugs") {
		t.Errorf("top border = %q, want the list named in it", top)
	}
	if !strings.Contains(top, "Sort: ") {
		t.Errorf("top border = %q, want the context line drawn in it", top)
	}
	if strings.Index(top, "Sort: ") < strings.Index(top, "Polish & Bugs") {
		t.Errorf("top border = %q, want the context right of the title", top)
	}
	for _, line := range lines[1:] {
		if strings.Contains(line, "Sort: ") {
			t.Errorf("context line also drawn inside the pane: %q", line)
		}
	}
}

// A long title takes the row it needs; the context yields rather than
// overwriting the name of the list.
func TestALongTitleCrowdsOutTheContextLine(t *testing.T) {
	app := contextTestApp(t)
	app.selectedNavigation = &NavigationNode{
		ID:        "project-1",
		Text:      "A project with a name long enough to fill the whole border",
		TeamID:    "team-1",
		IsProject: true,
	}
	app.updateAllPaneTitles()

	for _, line := range drawPrimitive(t, app.listIssuesTable, 60) {
		if strings.Contains(line, "Sort: ") {
			t.Errorf("the context line drew over a title that already filled the row: %q", line)
		}
	}
}

// Project names are Linear's and the title is built from color tags, so a
// bracketed name would be read as one instead of printed.
func TestTheIssuesTitleKeepsABracketedName(t *testing.T) {
	app := contextTestApp(t)
	app.selectedNavigation = &NavigationNode{ID: "p", Text: "[red] sprint", TeamID: "team-1", IsProject: true}

	if got := stripTags(app.issuesPaneTitle(true)); !strings.Contains(got, "[red]") {
		t.Errorf("title = %q, want the bracketed name kept", got)
	}
}
