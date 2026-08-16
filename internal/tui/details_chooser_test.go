package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

const chooserTeamID = "team-1"

// seedChooserOptions gives the details fixture a team, the values a chooser
// opens on, and the warm caches that let one open without a fetch.
func seedChooserOptions(app *App) {
	issue := app.selectedIssue
	issue.TeamID = chooserTeamID
	issue.StateID = "state-1"
	issue.AssigneeID = "user-1"
	issue.Assignee = "Ada Lovelace"
	issue.ProjectID = "project-1"
	issue.ProjectName = "Alpha"

	app.metadataTeamID = chooserTeamID
	app.workflowStates = []linearapi.WorkflowState{
		{ID: "state-1", Name: "In Progress"},
		{ID: "state-2", Name: "In Review"},
		{ID: "state-3", Name: "Done"},
	}
	app.teamUsers = []linearapi.User{
		{ID: "user-1", Name: "Ada Lovelace"},
		{ID: "user-2", Name: "Grace Hopper"},
	}
	app.teamProjects = []linearapi.Project{
		{ID: "project-1", Name: "Alpha"},
		{ID: "project-2", Name: "Beta"},
	}
	app.teamCycles = []linearapi.Cycle{{ID: "cycle-1", Name: "Launch", Number: 12, IsActive: true}}
}

// chooserFixture opens the details fixture in edit mode with the cursor on one
// field. Writes land on the channel and UI updates wait on the queue.
func chooserFixture(t *testing.T, field issueField) (*App, <-chan linearapi.UpdateIssueInput, <-chan func()) {
	t.Helper()

	app := newDetailsTestApp(t)
	seedChooserOptions(app)
	app.updateDetailsView()
	app.enterDetailsEdit()
	if !app.detailsEdit.on {
		t.Fatal("the pane did not enter edit mode")
	}
	drawDetails(t, app, 90)
	cursorTo(t, app, field)

	writes := make(chan linearapi.UpdateIssueInput, 1)
	app.updateIssueFunc = func(_ context.Context, input linearapi.UpdateIssueInput) (linearapi.Issue, error) {
		writes <- input
		return linearapi.Issue{ID: input.ID}, nil
	}
	// Queued rather than run where they are raised, so the write goroutine
	// cannot re-render the page the test is reading.
	pending := make(chan func(), 64)
	app.queueUpdateDraw = func(f func()) { pending <- f }
	return app, writes, pending
}

// cursorTo walks the field cursor onto a field with the key a reader would use.
func cursorTo(t *testing.T, app *App, field issueField) {
	t.Helper()
	for range len(app.detailsFieldSpans) {
		if app.detailsEdit.cursor == field {
			return
		}
		pressField(app, 'j')
	}
	if app.detailsEdit.cursor != field {
		t.Fatalf("field cursor = %q, want it on %q", app.detailsEdit.cursor, field)
	}
}

// openChooser presses Enter and draws, which is what puts the options on the
// page and records where they landed.
func openChooser(t *testing.T, app *App) []string {
	t.Helper()
	pressFieldKey(app, tcell.KeyEnter)
	if app.detailsEdit.open == "" {
		t.Fatal("Enter opened no chooser")
	}
	return drawDetails(t, app, 90)
}

// litOption is the option wearing the cursor line. The drawn screen has lost
// its colors, so this reads the page text the chooser was written into.
func litOption(t *testing.T, app *App) string {
	t.Helper()
	page := app.detailsPageView.GetText(false)
	for _, line := range strings.Split(page, "\n") {
		if strings.Contains(line, app.themeTags.Selection) {
			return strings.TrimSpace(strings.ReplaceAll(stripTags(line), "│", ""))
		}
	}
	t.Fatalf("no option wears the cursor line in:\n%s", stripTags(page))
	return ""
}

func TestEnterDrawsTheOptionsUnderTheField(t *testing.T) {
	app, _, _ := chooserFixture(t, issueFieldState)

	lines := openChooser(t, app)

	state := indexOfLine(lines, findLine(t, lines, "State:"))
	review := indexOfLine(lines, findLine(t, lines, "In Review"))
	if review <= state {
		t.Fatalf("In Review is at row %d and State: at %d, want the options under the field", review, state)
	}
	// A span's column is a column of the row, and the drawn line carries the
	// pane's own padding in front of it.
	span := app.detailsFieldSpans[app.fieldSpanIndex(issueFieldState)]
	padding := runeColumn(lines[state], "State:") - detailsCursorGutter
	if got, want := runeColumn(lines[state+1], "╭"), padding+span.valueColumn; got != want {
		t.Fatalf("the frame hangs at column %d, want the value column %d", got, want)
	}
	if !strings.HasSuffix(strings.TrimRight(lines[review], " "), "│") {
		t.Fatalf("option row = %q, want it inside the frame", lines[review])
	}
}

// runeColumn is where a substring starts in cells. Bytes would count the cursor
// glyph as three.
func runeColumn(line, substring string) int {
	at := strings.Index(line, substring)
	if at < 0 {
		return -1
	}
	return len([]rune(line[:at]))
}

func TestTheChooserIsAsWideAsWhatIsInIt(t *testing.T) {
	app, _, _ := chooserFixture(t, issueFieldState)

	top := findLine(t, openChooser(t, app), "╭")

	// The longest option plus the frame's own cells. Drawn to the measure, a
	// three-line list would span the pane.
	want := len("In Progress") + commentCardChrome
	if got := runeColumn(top, "╮") - runeColumn(top, "╭") + 1; got != want {
		t.Fatalf("the frame is %d cells wide, want %d, the width of what is in it", got, want)
	}
}

// The screen a draw returns has lost its colors, so this reads the page text
// the chooser was written into, tags and all.
func TestTheOpenChooserWearsTheAccentBorder(t *testing.T) {
	app, _, _ := chooserFixture(t, issueFieldState)
	openChooser(t, app)

	page := strings.Split(app.detailsPageView.GetText(false), "\n")
	if top := findLine(t, page, "╭"); !strings.Contains(top, app.themeTags.BorderFocus) {
		t.Fatalf("chooser frame = %q, want the accent a frame holding the keyboard takes", top)
	}
}

func TestTheChooserOpensOnTheValueTheFieldHolds(t *testing.T) {
	app, _, _ := chooserFixture(t, issueFieldState)

	openChooser(t, app)
	if lit := litOption(t, app); lit != "In Progress" {
		t.Fatalf("lit option = %q, want the state the issue already has", lit)
	}
}

func TestSteppingAnOptionLeavesTheFieldCursorWhereItWas(t *testing.T) {
	app, _, _ := chooserFixture(t, issueFieldState)
	openChooser(t, app)

	pressField(app, 'j')

	if lit := litOption(t, app); lit != "In Review" {
		t.Fatalf("lit option = %q, want the next one down", lit)
	}
	if app.detailsEdit.cursor != issueFieldState {
		t.Fatalf("field cursor = %q, want it still on the state row", app.detailsEdit.cursor)
	}
}

func TestChoosingAValueSavesThatFieldAlone(t *testing.T) {
	app, writes, _ := chooserFixture(t, issueFieldState)
	openChooser(t, app)
	before, _ := app.detailsPageView.GetScrollOffset()

	pressField(app, 'j')
	pressFieldKey(app, tcell.KeyEnter)

	input := awaitWrite(t, writes)
	if input.ID != "issue-1" {
		t.Fatalf("wrote to issue %q, want the one the chooser opened on", input.ID)
	}
	if input.StateID == nil || *input.StateID != "state-2" {
		t.Fatalf("StateID = %v, want the option picked", input.StateID)
	}
	if input.AssigneeID != nil || input.Priority != nil || input.ProjectID != nil {
		t.Fatal("the write carried a field the chooser was not on")
	}
	if app.detailsEdit.open != "" {
		t.Fatal("the chooser stayed open after committing")
	}
	if !app.detailsEdit.on {
		t.Fatal("committing left edit mode")
	}
	if after, _ := app.detailsPageView.GetScrollOffset(); after != before {
		t.Fatalf("scroll went from %d to %d, want the page held", before, after)
	}
}

func TestChoosingTheValueAlreadySetSendsNothing(t *testing.T) {
	app, writes, _ := chooserFixture(t, issueFieldState)
	openChooser(t, app)

	pressFieldKey(app, tcell.KeyEnter)
	if app.detailsEdit.open != "" {
		t.Fatal("the chooser stayed open")
	}
	// Then pick one that did change. Reading the first write is the assertion:
	// a channel checked on the spot only races the goroutine that fills it.
	openChooser(t, app)
	pressField(app, 'j')
	pressFieldKey(app, tcell.KeyEnter)

	if input := awaitWrite(t, writes); input.StateID == nil || *input.StateID != "state-2" {
		t.Fatalf("first write set state %v, want only the value that changed", input.StateID)
	}
}

func TestTheClearRowClearsTheField(t *testing.T) {
	app, writes, _ := chooserFixture(t, issueFieldAssignee)
	lines := openChooser(t, app)

	if !strings.Contains(strings.Join(lines, "\n"), "Unassigned") {
		t.Fatalf("no clear row in:\n%s", strings.Join(lines, "\n"))
	}
	// The clear row heads the list and the highlight opened on the assignee.
	pressField(app, 'k')
	pressFieldKey(app, tcell.KeyEnter)

	input := awaitWrite(t, writes)
	if input.AssigneeID == nil || *input.AssigneeID != "" {
		t.Fatalf("AssigneeID = %v, want the explicit null", input.AssigneeID)
	}
}

func TestAFieldAlreadyEmptyOffersNoClearRow(t *testing.T) {
	app, _, _ := chooserFixture(t, issueFieldCycle)

	lines := openChooser(t, app)

	// Once for the field's own row, which is read mode saying it holds none.
	if count := strings.Count(strings.Join(lines, "\n"), "No cycle"); count != 1 {
		t.Fatalf("No cycle appears %d times, want only the field's own row", count)
	}
}

func TestEscapeClosesTheChooserAndKeepsTheMode(t *testing.T) {
	app, _, _ := chooserFixture(t, issueFieldState)
	openChooser(t, app)

	sendKey(app, tcell.KeyEscape)
	if app.detailsEdit.open != "" {
		t.Fatal("Escape left the chooser open")
	}
	if !app.detailsEdit.on {
		t.Fatal("Escape left edit mode as well as the chooser")
	}
	if body := strings.Join(drawDetails(t, app, 90), "\n"); strings.Contains(body, "In Review") {
		t.Fatalf("the options are still drawn:\n%s", body)
	}

	sendKey(app, tcell.KeyEscape)
	if app.detailsEdit.on {
		t.Fatal("the second Escape did not leave edit mode")
	}
}

func TestClosingTheChooserBringsTheFieldBack(t *testing.T) {
	app, _, _ := chooserFixture(t, issueFieldState)
	openChooser(t, app)
	// The wheel goes straight through the pane, so the page can be scrolled off
	// the chooser while it is open.
	app.detailsPageView.ScrollTo(40, 0)

	sendKey(app, tcell.KeyEscape)

	row := app.detailsFieldSpans[app.fieldSpanIndex(issueFieldState)].row
	offset, _ := app.detailsPageView.GetScrollOffset()
	height := viewHeight(app.detailsPageView)
	if row < offset || row >= offset+height {
		t.Fatalf("the state row is at %d and the page shows %d..%d, want it back in view", row, offset, offset+height)
	}
}

func TestTheChooserSwallowsTheKeysThatWouldLeaveIt(t *testing.T) {
	for _, key := range []rune{'q', ':', '1', '/', 'e'} {
		t.Run(string(key), func(t *testing.T) {
			app, _, _ := chooserFixture(t, issueFieldState)
			openChooser(t, app)

			if left := pressField(app, key); left != nil {
				t.Fatalf("%q was handed on, want the chooser to swallow it", key)
			}
			if app.detailsEdit.open == "" {
				t.Fatalf("%q closed the chooser", key)
			}
		})
	}
}

func TestALongListDrawsTheCapAndCountsTheRest(t *testing.T) {
	app, _, _ := chooserFixture(t, issueFieldState)
	states := make([]linearapi.WorkflowState, 0, 16)
	for i := range 16 {
		states = append(states, linearapi.WorkflowState{ID: fmt.Sprintf("state-%d", i+1), Name: fmt.Sprintf("Stage %02d", i+1)})
	}
	app.workflowStates = states

	lines := openChooser(t, app)

	drawn := 0
	for _, line := range lines {
		if strings.Contains(line, "Stage ") {
			drawn++
		}
	}
	if drawn > detailsChooserMaxRows {
		t.Fatalf("drew %d options, want at most %d", drawn, detailsChooserMaxRows)
	}
	if want := fmt.Sprintf("+%d more", len(states)-drawn); !strings.Contains(strings.Join(lines, "\n"), want) {
		t.Fatalf("no %q row in:\n%s", want, strings.Join(lines, "\n"))
	}
}

func TestTheWindowFollowsTheHighlightPastTheCap(t *testing.T) {
	app, _, _ := chooserFixture(t, issueFieldState)
	states := make([]linearapi.WorkflowState, 0, 16)
	for i := range 16 {
		states = append(states, linearapi.WorkflowState{ID: fmt.Sprintf("state-%d", i+1), Name: fmt.Sprintf("Stage %02d", i+1)})
	}
	app.workflowStates = states
	openChooser(t, app)

	visible := app.chooserVisibleRows()
	for range visible + 1 {
		pressField(app, 'j')
	}
	lines := drawDetails(t, app, 90)

	want := fmt.Sprintf("Stage %02d", visible+2)
	if lit := litOption(t, app); lit != want {
		t.Fatalf("lit option = %q, want %q", lit, want)
	}
	if !strings.Contains(strings.Join(lines, "\n"), want) {
		t.Fatalf("the highlight is off the drawn window:\n%s", strings.Join(lines, "\n"))
	}
}

func TestOptionsLoadAgainstTheIssuesOwnTeam(t *testing.T) {
	app, _, pending := chooserFixture(t, issueFieldState)
	// The tree is on another team, which is what a search result looks like.
	app.metadataTeamID = "team-elsewhere"
	asked := make(chan string, 1)
	app.fetchWorkflowStatesFunc = func(_ context.Context, teamID string) ([]linearapi.WorkflowState, error) {
		asked <- teamID
		return []linearapi.WorkflowState{{ID: "state-9", Name: "Shipped"}}, nil
	}

	pressFieldKey(app, tcell.KeyEnter)

	select {
	case teamID := <-asked:
		if teamID != chooserTeamID {
			t.Fatalf("loaded options for team %q, want the issue's own %q", teamID, chooserTeamID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the option load")
	}
	runQueuedUpdate(t, pending)
	if got := app.workflowStates[0].ID; got != "state-1" {
		t.Fatalf("the navigation team's cache now holds %q, want it untouched", got)
	}
	if body := strings.Join(drawDetails(t, app, 90), "\n"); !strings.Contains(body, "Shipped") {
		t.Fatalf("the fetched options are not on the page:\n%s", body)
	}
}

func TestALoadThatFailsClosesTheChooser(t *testing.T) {
	app, _, pending := chooserFixture(t, issueFieldState)
	app.metadataTeamID = "team-elsewhere"
	app.fetchWorkflowStatesFunc = func(context.Context, string) ([]linearapi.WorkflowState, error) {
		return nil, fmt.Errorf("linear said no")
	}

	pressFieldKey(app, tcell.KeyEnter)
	runQueuedUpdate(t, pending)

	if app.detailsEdit.open != "" {
		t.Fatal("a failed load left the chooser on its loading row")
	}
	if !app.detailsEdit.on {
		t.Fatal("a failed load dropped edit mode")
	}
	if text := statusText(app); !strings.Contains(text, "linear said no") {
		t.Fatalf("status bar = %q, want the failure named", text)
	}
}

func TestALateLoadForAClosedChooserIsDropped(t *testing.T) {
	app, _, pending := chooserFixture(t, issueFieldState)
	app.metadataTeamID = "team-elsewhere"
	app.fetchWorkflowStatesFunc = func(context.Context, string) ([]linearapi.WorkflowState, error) {
		return []linearapi.WorkflowState{{ID: "state-9", Name: "Stale"}}, nil
	}

	pressFieldKey(app, tcell.KeyEnter)
	sendKey(app, tcell.KeyEscape)
	pressFieldKey(app, tcell.KeyEnter)
	runQueuedUpdate(t, pending)

	if app.detailsEdit.gen == 0 {
		t.Fatal("the second opening took no generation of its own")
	}
	if !app.detailsEdit.loading {
		t.Fatal("the first load filled the second chooser")
	}
}

func TestMilestoneWithNoProjectDoesNotOpen(t *testing.T) {
	app, _, _ := chooserFixture(t, issueFieldMilestone)
	app.issuesMu.Lock()
	app.selectedIssue.ProjectID = ""
	app.issuesMu.Unlock()

	pressFieldKey(app, tcell.KeyEnter)

	if app.detailsEdit.open != "" {
		t.Fatal("a milestone chooser opened on an issue with no project")
	}
	if text := statusText(app); !strings.Contains(text, "must have a project") {
		t.Fatalf("status bar = %q, want the refusal named", text)
	}
}

func TestTheCommitNamesTheValueWithoutItsDecoration(t *testing.T) {
	app, writes, pending := chooserFixture(t, issueFieldCycle)
	openChooser(t, app)

	pressFieldKey(app, tcell.KeyEnter)
	awaitWrite(t, writes)
	// The corner is written where the answer lands, which is a queued update.
	runQueuedUpdate(t, pending)

	if text := statusText(app); !strings.Contains(text, "Set cycle: Launch") || strings.Contains(text, "(active)") {
		t.Fatalf("status bar = %q, want the cycle named without the row's decoration", text)
	}
}

func TestTheCountRowNamesWhatIsBelowIt(t *testing.T) {
	app, _, _ := chooserFixture(t, issueFieldState)
	app.workflowStates = manyStates(16)
	openChooser(t, app)

	// Stepped to the last option, nothing is below the window, so a count at
	// the foot of the list would be pointing at the rows above it.
	for range 20 {
		pressField(app, 'j')
	}
	if body := strings.Join(drawDetails(t, app, 90), "\n"); strings.Contains(body, "more") {
		t.Fatalf("the foot of the list still counts something:\n%s", body)
	}
}

func TestAValueTheListDoesNotCarryLightsNothing(t *testing.T) {
	app, writes, _ := chooserFixture(t, issueFieldState)
	// A state Linear has since retired, or a priority it has since added.
	app.issuesMu.Lock()
	app.selectedIssue.StateID = "state-gone"
	app.issuesMu.Unlock()
	app.updateDetailsView()
	cursorTo(t, app, issueFieldState)
	openChooser(t, app)

	if app.detailsEdit.choice != -1 {
		t.Fatalf("choice = %d, want nothing lit for a value the list has not got", app.detailsEdit.choice)
	}
	pressFieldKey(app, tcell.KeyEnter)
	select {
	case input := <-writes:
		t.Fatalf("Enter wrote %+v, want nothing while no option is lit", input)
	default:
	}
	// j lands on the first option rather than refusing the key.
	pressField(app, 'j')
	if lit := litOption(t, app); lit != "In Progress" {
		t.Fatalf("lit option = %q, want the first one", lit)
	}
}

func TestAnIssueThatMovedTeamDoesNotCommitTheOldTeamsOption(t *testing.T) {
	app, writes, _ := chooserFixture(t, issueFieldState)
	openChooser(t, app)
	pressField(app, 'j')
	// The background refresh that lands while the list is open.
	app.issuesMu.Lock()
	app.selectedIssue.TeamID = "team-elsewhere"
	app.issuesMu.Unlock()

	pressFieldKey(app, tcell.KeyEnter)

	select {
	case input := <-writes:
		t.Fatalf("wrote %+v, want nothing for an option from the team it left", input)
	default:
	}
	if text := statusText(app); !strings.Contains(text, "team changed") {
		t.Fatalf("status bar = %q, want the move named", text)
	}
}

func TestANarrowPaneKeepsTheChooserOnScreen(t *testing.T) {
	app, _, _ := chooserFixture(t, issueFieldState)
	pressFieldKey(app, tcell.KeyEnter)

	// Narrower than the metadata gutter the chooser hangs off. Drawn twice: the
	// refit runs inside a draw, so it lands a frame behind the resize.
	drawTextView(t, app.detailsView, 22)
	lines := drawTextView(t, app.detailsView, 22)

	below := lines[indexOfLine(lines, findLine(t, lines, "State:"))+1]
	if !strings.Contains(below, "╭") {
		t.Fatalf("the row under the field is %q, want the chooser drawn on it", below)
	}
	if runeColumn(below, "╮") < 0 {
		t.Fatalf("the chooser runs off the drawn line: %q", below)
	}
}

func TestAShorterPaneRecapsAnOpenChooser(t *testing.T) {
	app, _, _ := chooserFixture(t, issueFieldState)
	app.workflowStates = manyStates(16)
	pressFieldKey(app, tcell.KeyEnter)
	drawPrimitiveAt(t, app.detailsView, 90, 40)
	tall := app.chooserVisibleRows()

	// A height-only resize, which the width guard alone would not refit.
	drawPrimitiveAt(t, app.detailsView, 90, 12)

	if short := app.chooserVisibleRows(); short >= tall {
		t.Fatalf("a %d-row pane still draws %d options, want fewer than the %d a tall one did", 12, short, tall)
	}
}

func TestAnEmptyOptionListSaysSoRatherThanOpening(t *testing.T) {
	app := newUXTestApp(t)
	pending := make(chan func(), 8)
	app.queueUpdateDraw = func(f func()) { pending <- f }
	issue := linearapi.Issue{ID: "issue-1", Identifier: "ZNO-5", TeamID: chooserTeamID}
	app.issuesMu.Lock()
	app.selectedIssue = &issue
	app.issuesMu.Unlock()
	app.metadataTeamID = chooserTeamID
	app.workflowStates = nil
	app.fetchWorkflowStatesFunc = func(context.Context, string) ([]linearapi.WorkflowState, error) {
		return nil, nil
	}

	app.ShowFieldPicker(issueFieldState, app.issueOptionScope(issue), "", func(PickerItem) {})
	runQueuedUpdate(t, pending)

	if app.pages.HasPage("picker") {
		t.Fatal("an overlay opened with nothing in it to pick")
	}
	if text := statusText(app); !strings.Contains(text, "No status available") {
		t.Fatalf("status bar = %q, want the empty list named", text)
	}
}

func manyStates(n int) []linearapi.WorkflowState {
	states := make([]linearapi.WorkflowState, 0, n)
	states = append(states, linearapi.WorkflowState{ID: "state-1", Name: "In Progress"})
	for i := len(states); i < n; i++ {
		states = append(states, linearapi.WorkflowState{ID: fmt.Sprintf("state-%d", i+1), Name: fmt.Sprintf("Stage %02d", i+1)})
	}
	return states
}

func TestTheHintNamesEnterOnlyWhereItOpensSomething(t *testing.T) {
	app, _, _ := chooserFixture(t, issueFieldState)

	if text := statusText(app); !strings.Contains(text, "⏎ open") {
		t.Fatalf("status bar = %q, want Enter named on a field with a list", text)
	}

	cursorTo(t, app, issueFieldLabels)
	if text := statusText(app); strings.Contains(text, "⏎ open") {
		t.Fatalf("status bar = %q, want Enter unnamed on a field it does nothing on", text)
	}
}

func TestTheAssigneeSaveNamesTheDisplayName(t *testing.T) {
	app, writes, pending := chooserFixture(t, issueFieldAssignee)
	app.teamUsers = []linearapi.User{
		{ID: "user-1", Name: "Ada Lovelace"},
		{ID: "user-3", DisplayName: "grace"},
	}
	openChooser(t, app)

	// One down from the current assignee, onto the user whose only name is a
	// display name.
	pressField(app, 'j')
	pressFieldKey(app, tcell.KeyEnter)
	awaitWrite(t, writes)
	runQueuedUpdate(t, pending)

	if text := statusText(app); !strings.Contains(text, "Set assignee: grace") {
		t.Fatalf("status bar = %q, want the display name, not an empty one", text)
	}
}

func TestTheChooserKeepsItsFootOnScreen(t *testing.T) {
	app, _, _ := chooserFixture(t, issueFieldState)
	app.workflowStates = manyStates(16)
	// Short enough that reaching the last option has to scroll the page.
	drawPrimitiveAt(t, app.detailsView, 90, 10)
	pressFieldKey(app, tcell.KeyEnter)
	drawPrimitiveAt(t, app.detailsView, 90, 10)

	for range 20 {
		pressField(app, 'j')
	}
	drawPrimitiveAt(t, app.detailsView, 90, 10)

	top, _ := app.detailsPageView.GetScrollOffset()
	height := viewHeight(app.detailsPageView)
	if end := app.detailsChooserSpan.end; end >= top+height {
		t.Fatalf("the chooser ends at row %d and the page shows %d..%d, want its foot on screen", end, top, top+height-1)
	}
}
