package tui

import (
	"strings"
	"testing"

	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// footerTestApp puts a project of a known team on screen, so the context line
// has a team key to prefix and a scope to name.
func footerTestApp(t *testing.T) *App {
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

func TestIssuesFooterNamesScopeAndSort(t *testing.T) {
	app := footerTestApp(t)

	text := stripTags(app.issuesFooterText(80))

	for _, want := range []string{"ZNL · Polish & Bugs", "Sort: "} {
		if !strings.Contains(text, want) {
			t.Errorf("footer = %q, want it to carry %q", text, want)
		}
	}
}

// The team's own row already says the team, so the key would be the same fact
// twice.
func TestIssuesFooterLeavesATeamRowUnprefixed(t *testing.T) {
	app := footerTestApp(t)
	app.selectedNavigation = &NavigationNode{ID: "team-1", Text: "Zen Linear", TeamID: "team-1", IsTeam: true}

	if got := stripTags(app.issuesFooterText(80)); strings.Contains(got, "ZNL ·") {
		t.Errorf("footer = %q, want no key in front of the team's own name", got)
	}
}

func TestIssuesFooterCarriesFilters(t *testing.T) {
	app := footerTestApp(t)
	app.richFilters = IssueFilters{AssigneeID: "user-1", AssigneeName: "drew"}

	if got := stripTags(app.issuesFooterText(120)); !strings.Contains(got, "Filters: ") {
		t.Errorf("footer = %q, want the filters named", got)
	}
}

// A narrow pane gives up the sort first: the rows themselves show the order,
// and the scope and filters cannot be read anywhere else.
func TestIssuesFooterDropsSortBeforeAnythingElse(t *testing.T) {
	app := footerTestApp(t)
	app.richFilters = IssueFilters{AssigneeID: "user-1", AssigneeName: "drew"}

	text := stripTags(app.issuesFooterText(50))

	if strings.Contains(text, "Sort:") {
		t.Errorf("footer = %q at width 50, want the sort dropped", text)
	}
	for _, want := range []string{"ZNL · Polish & Bugs", "Filters: "} {
		if !strings.Contains(text, want) {
			t.Errorf("footer = %q at width 50, want it to keep %q", text, want)
		}
	}
}

func TestIssuesFooterIsEmptyWithNothingSelected(t *testing.T) {
	app := newUXTestApp(t)

	if got := app.issuesFooterText(80); got != "" {
		t.Errorf("footer = %q with no navigation selection, want nothing", got)
	}
}

// TestIssuesFooterDrawsOnTheBottomBorder covers the line landing in the border
// itself rather than costing the list a row.
func TestIssuesFooterDrawsOnTheBottomBorder(t *testing.T) {
	app := footerTestApp(t)

	lines := drawPrimitive(t, app.allIssuesTable, 80)

	bottom := lines[len(lines)-1]
	if !strings.Contains(bottom, "ZNL · Polish & Bugs") {
		t.Errorf("bottom border = %q, want the context line drawn in it", bottom)
	}
	for _, line := range lines[:len(lines)-1] {
		if strings.Contains(line, "ZNL · Polish & Bugs") {
			t.Errorf("context line also drawn inside the pane: %q", line)
		}
	}
}
