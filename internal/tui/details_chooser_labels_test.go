package tui

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
)

func markedRow(t *testing.T, app *App, label string) string {
	t.Helper()
	page := strings.Split(app.detailsPageView.GetText(false), "\n")
	for _, line := range page {
		plain := stripTags(line)
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
	if lit == "" || strings.Contains(lit, "[-]") {
		t.Fatalf("lit row = %q, want one unbroken run of the cursor line", lit)
	}
}

func TestApplyingLabelsWritesTheWholeSet(t *testing.T) {
	app, writes, _ := chooserFixture(t, issueFieldLabels)
	openChooser(t, app)

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
	app.selectedIssue.Labels = []linearapi.IssueLabel{{ID: "label-2", Name: "Chore"}, {ID: "label-1", Name: "Bug"}}
	app.updateDetailsView()
	drawDetails(t, app, 90)
	openChooser(t, app)

	pressFieldKey(app, tcell.KeyEnter)
	if app.detailsEdit.open != "" {
		t.Fatal("the chooser stayed open")
	}
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

func refreshIssueLabels(app *App, labels []linearapi.IssueLabel) {
	app.issuesMu.Lock()
	refreshed := *app.selectedIssue
	refreshed.Labels = labels
	app.selectedIssue = &refreshed
	app.issuesMu.Unlock()
}

func TestALabelAddedWhileTheChooserIsOpenSurvivesTheApply(t *testing.T) {
	app, writes, _ := chooserFixture(t, issueFieldLabels)
	openChooser(t, app)

	refreshIssueLabels(app, []linearapi.IssueLabel{{ID: "label-1", Name: "Bug"}, {ID: "label-9", Name: "Hidden"}})

	pressField(app, 'j')
	pressField(app, ' ')
	pressFieldKey(app, tcell.KeyEnter)

	input := awaitWrite(t, writes)
	if input.LabelIDs == nil || !reflect.DeepEqual(*input.LabelIDs, []string{"label-1", "label-2", "label-9"}) {
		t.Fatalf("wrote labels %v, want the one added under the chooser kept", input.LabelIDs)
	}
}

func TestApplyingWithNoToggleSendsNothingAfterARefresh(t *testing.T) {
	app, writes, _ := chooserFixture(t, issueFieldLabels)
	openChooser(t, app)

	refreshIssueLabels(app, []linearapi.IssueLabel{{ID: "label-1", Name: "Bug"}, {ID: "label-2", Name: "Chore"}})
	pressFieldKey(app, tcell.KeyEnter)

	openChooser(t, app)
	pressField(app, ' ')
	pressFieldKey(app, tcell.KeyEnter)

	if input := awaitWrite(t, writes); input.LabelIDs == nil || !reflect.DeepEqual(*input.LabelIDs, []string{"label-2"}) {
		t.Fatalf("first write set labels %v, want only the set the reader changed", input.LabelIDs)
	}
}

func TestALabelTakenOffWhileTheChooserIsOpenIsNotPutBack(t *testing.T) {
	app, writes, _ := chooserFixture(t, issueFieldLabels)
	app.selectedIssue.Labels = append(app.selectedIssue.Labels, linearapi.IssueLabel{ID: "label-9", Name: "Hidden"})
	app.updateDetailsView()
	drawDetails(t, app, 90)
	openChooser(t, app)

	refreshIssueLabels(app, []linearapi.IssueLabel{{ID: "label-1", Name: "Bug"}})

	pressField(app, 'j')
	pressField(app, ' ')
	pressFieldKey(app, tcell.KeyEnter)

	input := awaitWrite(t, writes)
	if input.LabelIDs == nil || !reflect.DeepEqual(*input.LabelIDs, []string{"label-1", "label-2"}) {
		t.Fatalf("wrote labels %v, want the one taken off left off", input.LabelIDs)
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

func TestTheLabelsOverlayLoadsTheIssuesOwnTeam(t *testing.T) {
	app := newDetailsTestApp(t)
	app.selectedIssue.TeamID = "team-other"
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
