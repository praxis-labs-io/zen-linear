package tui

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// markedRow is one chooser row as its box and its label, for a test that cares
// which are toggled on.
func markedRow(t *testing.T, app *App, label string) string {
	t.Helper()
	page := strings.Split(app.detailsPageView.GetText(false), "\n")
	for _, line := range page {
		plain := stripTags(line)
		// The glyph is what tells a chooser row from the Labels row above it,
		// which says the same words.
		if !strings.Contains(plain, label) || !strings.ContainsAny(plain, "◼◻") {
			continue
		}
		return strings.TrimSpace(strings.ReplaceAll(plain, "│", ""))
	}
	t.Fatalf("no row says %q in:\n%s", label, stripTags(app.detailsPageView.GetText(false)))
	return ""
}

func TestTheLabelChooserMarksWhatTheIssueHolds(t *testing.T) {
	app, _, _ := chooserFixture(t, issueFieldLabels)

	openChooser(t, app)

	if got := markedRow(t, app, "Bug"); got != "◼ Bug" {
		t.Fatalf("the Bug row = %q, want it filled, the issue holds it", got)
	}
	if got := markedRow(t, app, "Chore"); got != "◻ Chore" {
		t.Fatalf("the Chore row = %q, want it hollow", got)
	}
}

func TestTheLabelChooserOpensOnTheFirstRow(t *testing.T) {
	app, _, _ := chooserFixture(t, issueFieldLabels)
	app.teamLabels = []linearapi.IssueLabel{
		{ID: "label-2", Name: "Chore"},
		{ID: "label-1", Name: "Bug"},
	}

	openChooser(t, app)

	// The marks say what is set, so the cursor has nothing left to mark and
	// starts at the top even though the issue's own label is below it.
	if lit := litOption(t, app); lit != "◻ Chore" {
		t.Fatalf("lit option = %q, want the first row", lit)
	}
}

func TestSpaceFlipsTheMarkAndLeavesTheCursor(t *testing.T) {
	app, _, _ := chooserFixture(t, issueFieldLabels)
	openChooser(t, app)

	pressField(app, ' ')

	if got := markedRow(t, app, "Bug"); got != "◻ Bug" {
		t.Fatalf("the Bug row = %q, want it emptied by the toggle", got)
	}
	if lit := litOption(t, app); lit != "◻ Bug" {
		t.Fatalf("lit option = %q, want the cursor still on the row it toggled", lit)
	}
}

func TestTheLitLabelRowIsPaintedEndToEnd(t *testing.T) {
	app, _, _ := chooserFixture(t, issueFieldLabels)
	openChooser(t, app)

	var lit string
	for _, line := range strings.Split(app.detailsPageView.GetText(false), "\n") {
		if at := strings.Index(line, app.themeTags.Selection); at >= 0 {
			lit, _, _ = strings.Cut(line[at+len(app.themeTags.Selection):], "[-:-:-]")
		}
	}
	// A colored mark closes with its own reset, which would drop the label
	// after it to the terminal default over the cursor line's background.
	if lit == "" || strings.Contains(lit, "[-]") {
		t.Fatalf("lit row = %q, want one unbroken run of the cursor line", lit)
	}
}

func TestApplyingLabelsWritesTheWholeSet(t *testing.T) {
	app, writes, _ := chooserFixture(t, issueFieldLabels)
	openChooser(t, app)

	// Chore on, Bug left alone: one write carrying both, not one per toggle.
	pressField(app, 'j')
	pressField(app, ' ')
	pressFieldKey(app, tcell.KeyEnter)

	input := awaitWrite(t, writes)
	if input.ID != "issue-1" {
		t.Fatalf("wrote to issue %q, want the one the chooser opened on", input.ID)
	}
	if input.LabelIDs == nil || !reflect.DeepEqual(*input.LabelIDs, []string{"label-1", "label-2"}) {
		t.Fatalf("wrote labels %v, want both", input.LabelIDs)
	}
	if input.StateID != nil || input.AssigneeID != nil || input.Priority != nil {
		t.Fatalf("the write carried another field: %+v", input)
	}
}

func TestApplyingLabelsUnchangedSendsNothing(t *testing.T) {
	app, writes, _ := chooserFixture(t, issueFieldLabels)
	// Held in the order Linear returned them, which is not the order the two
	// sets are compared in.
	app.selectedIssue.Labels = []linearapi.IssueLabel{{ID: "label-2", Name: "Chore"}, {ID: "label-1", Name: "Bug"}}
	app.updateDetailsView()
	drawDetails(t, app, 90)
	openChooser(t, app)

	pressFieldKey(app, tcell.KeyEnter)
	if app.detailsEdit.open != "" {
		t.Fatal("the chooser stayed open")
	}
	// Then a set that did change. Reading the first write is the assertion: a
	// channel checked on the spot only races the goroutine that fills it.
	openChooser(t, app)
	pressField(app, ' ')
	pressFieldKey(app, tcell.KeyEnter)

	if input := awaitWrite(t, writes); input.LabelIDs == nil || !reflect.DeepEqual(*input.LabelIDs, []string{"label-2"}) {
		t.Fatalf("first write set labels %v, want only the set that changed", input.LabelIDs)
	}
}

func TestUncheckingEveryLabelWritesAnEmptySet(t *testing.T) {
	app, writes, _ := chooserFixture(t, issueFieldLabels)
	openChooser(t, app)

	pressField(app, ' ')
	pressFieldKey(app, tcell.KeyEnter)

	input := awaitWrite(t, writes)
	if input.LabelIDs == nil {
		t.Fatal("clearing the last label wrote nothing, want an empty list")
	}
	if len(*input.LabelIDs) != 0 {
		t.Fatalf("wrote labels %v, want none", *input.LabelIDs)
	}
}

func TestEscapeDropsTheLabelToggles(t *testing.T) {
	app, writes, _ := chooserFixture(t, issueFieldLabels)
	openChooser(t, app)

	pressField(app, ' ')
	pressFieldKey(app, tcell.KeyEscape)
	if !app.detailsEdit.on || app.detailsEdit.open != "" {
		t.Fatalf("edit mode = %v with %q open, want the chooser closed and the mode kept", app.detailsEdit.on, app.detailsEdit.open)
	}

	// Reopening is the assertion the toggle went nowhere: it seeds from the
	// issue, so a mark that survived would be a mark the write never made.
	openChooser(t, app)
	if got := markedRow(t, app, "Bug"); got != "◼ Bug" {
		t.Fatalf("the Bug row = %q, want the issue's own labels back", got)
	}
	select {
	case input := <-writes:
		t.Fatalf("Escape wrote %+v, want nothing", input)
	default:
	}
}

func TestALabelTheListDoesNotCarrySurvivesTheApply(t *testing.T) {
	app, writes, _ := chooserFixture(t, issueFieldLabels)
	// A label on the issue that the team's own list never offers, which is how
	// a workspace label reaches a cross-team issue.
	app.selectedIssue.Labels = append(app.selectedIssue.Labels, linearapi.IssueLabel{ID: "label-9", Name: "Hidden"})
	app.updateDetailsView()
	drawDetails(t, app, 90)
	openChooser(t, app)

	pressField(app, 'j')
	pressField(app, ' ')
	pressFieldKey(app, tcell.KeyEnter)

	input := awaitWrite(t, writes)
	if input.LabelIDs == nil || !reflect.DeepEqual(*input.LabelIDs, []string{"label-1", "label-2", "label-9"}) {
		t.Fatalf("wrote labels %v, want the unlisted one kept", input.LabelIDs)
	}
}

func TestSpaceDoesNothingOnASingleSelectChooser(t *testing.T) {
	app, _, _ := chooserFixture(t, issueFieldState)
	openChooser(t, app)

	pressField(app, ' ')

	if lit := litOption(t, app); lit != "In Progress" {
		t.Fatalf("lit option = %q, want the state row unmarked and unmoved", lit)
	}
}

func TestTheLabelHintNamesSpaceAndApply(t *testing.T) {
	app, _, _ := chooserFixture(t, issueFieldLabels)
	openChooser(t, app)

	text := statusText(app)
	if !strings.Contains(text, "space toggle") || !strings.Contains(text, "⏎ apply") {
		t.Fatalf("status bar = %q, want the two keys a set needs", text)
	}
}

// The overlay and the chooser share one loader, so the team it asks for is the
// issue's own rather than whatever the navigation tree is on.
func TestTheLabelsOverlayLoadsTheIssuesOwnTeam(t *testing.T) {
	app := newDetailsTestApp(t)
	app.selectedIssue.TeamID = "team-other"
	// The tree is parked on another team, which is what the loader used to
	// follow: a search result belongs to whichever team owns it.
	app.selectedNavigation = &NavigationNode{ID: chooserTeamID, TeamID: chooserTeamID, IsTeam: true}
	app.metadataTeamID = chooserTeamID
	navLabels := []linearapi.IssueLabel{{ID: "label-1", Name: "Bug"}}
	app.teamLabels = navLabels

	asked := make(chan string, 1)
	app.fetchIssueLabelsFunc = func(_ context.Context, teamID string) ([]linearapi.IssueLabel, error) {
		asked <- teamID
		return []linearapi.IssueLabel{{ID: "label-7", Name: "Regression"}}, nil
	}
	pending := make(chan func(), 4)
	app.queueUpdateDraw = func(f func()) { pending <- f }

	app.ShowEditLabelsModal()

	select {
	case teamID := <-asked:
		if teamID != "team-other" {
			t.Fatalf("loaded labels for team %q, want the issue's own", teamID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the label load")
	}
	runQueuedUpdate(t, pending)

	if !reflect.DeepEqual(app.teamLabels, navLabels) {
		t.Fatalf("the navigation team's cache = %v, want another team's load kept out of it", app.teamLabels)
	}
	if first, _ := app.multiSelectModal.list.GetItemText(0); !strings.Contains(first, "Regression") {
		t.Fatalf("first option = %q, want the issue's own team's label", first)
	}
}
