package tui

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/config"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// detailsFixture is an issue whose metadata is long enough to wrap in a narrow
// pane: a branch name with no spaces to break on, and sub-issue titles.
func detailsFixture() *linearapi.Issue {
	return &linearapi.Issue{
		ID:         "issue-1",
		Identifier: "ZNO-5",
		Title:      "M3: comment infrastructure",
		State:      "In Progress",
		BranchName: "feature/zno-5-m3-comment-infrastructure",
		Children: []linearapi.IssueChildRef{
			{ID: "c1", Identifier: "ZNO-28", Title: "M3: comment identity and permissions in the read path", State: "In Progress"},
			{ID: "c2", Identifier: "ZNO-20", Title: "M3: a focus ring in both detail panes", State: "Todo"},
		},
		Description: "A description long enough that it has to wrap across several lines inside the details pane rather than being cut off at the border.",
	}
}

// drawDetails renders the details pane at a width and returns its lines.
func drawDetails(t *testing.T, app *App, width int) []string {
	t.Helper()
	return drawTextView(t, app.detailsView, width)
}

// descriptionEnd is the row the description stops on: the second rule down the
// page, the first closing the metadata and this one closing the description.
// A page with only one rule has no comments under it and runs to the end.
func descriptionEnd(lines []string) int {
	rules := 0
	for i, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" && strings.Trim(trimmed, "─") == "" {
			if rules++; rules == 2 {
				return i
			}
		}
	}
	return len(lines)
}

// drawTextView renders a bordered primitive at a width and returns what landed
// on the screen, one string per row, with the border columns taken off.
func drawTextView(t *testing.T, view tview.Primitive, width int) []string {
	t.Helper()
	lines := drawPrimitive(t, view, width)
	// Columns 0 and width-1 are the pane border, which would otherwise sit on
	// the end of every line and hide what the content does there.
	for i, line := range lines {
		runes := []rune(line)
		if len(runes) < 2 {
			lines[i] = ""
			continue
		}
		lines[i] = strings.TrimRight(string(runes[1:len(runes)-1]), " ")
	}
	return lines
}

// drawPrimitive renders any primitive at a width and returns every column of
// what landed on the screen, one string per row.
func drawPrimitive(t *testing.T, primitive tview.Primitive, width int) []string {
	t.Helper()
	return drawPrimitiveAt(t, primitive, width, 40)
}

// drawPrimitiveAt renders at an explicit height, for a page too long to fit the
// default: the details pane draws the issue, its description and its comments
// as one scroll, so the cards start well down it.
func drawPrimitiveAt(t *testing.T, primitive tview.Primitive, width, height int) []string {
	t.Helper()

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	t.Cleanup(screen.Fini)

	screen.SetSize(width, height)
	primitive.SetRect(0, 0, width, height)
	primitive.Draw(screen)
	screen.Show()

	cells, screenWidth, screenHeight := screen.GetContents()
	lines := make([]string, 0, screenHeight)
	for y := 0; y < screenHeight; y++ {
		row := make([]rune, 0, screenWidth)
		for x := 0; x < screenWidth; x++ {
			runes := cells[y*screenWidth+x].Runes
			if len(runes) == 0 || runes[0] == 0 {
				row = append(row, ' ')
				continue
			}
			row = append(row, runes[0])
		}
		lines = append(lines, strings.TrimRight(string(row), " "))
	}
	return lines
}

func newDetailsTestApp(t *testing.T) *App {
	t.Helper()

	app := NewApp(linearapi.ClientConfig{}, config.Config{CacheTTL: time.Minute}, nil)
	stopBackgroundWorkOnCleanup(t, app)
	app.queueUpdateDraw = func(f func()) { f() }

	app.selectedIssue = detailsFixture()
	app.updateDetailsView()
	return app
}

// TestDetailsHeaderTruncatesInsteadOfWrapping covers the metadata block
// spilling onto a second line in a narrow pane. Each field is one row now, cut
// with an ellipsis.
func TestDetailsHeaderTruncatesInsteadOfWrapping(t *testing.T) {
	app := newDetailsTestApp(t)
	lines := drawDetails(t, app, 40)

	branch := findLine(t, lines, "Branch:")
	if !strings.HasSuffix(branch, "…") {
		t.Errorf("branch line = %q, want it cut with an ellipsis", branch)
	}
	if next := lines[indexOfLine(lines, branch)+1]; strings.Contains(next, "infrastructure") {
		t.Errorf("branch wrapped onto the next line: %q", next)
	}

	child := findLine(t, lines, "ZNO-28")
	if !strings.HasSuffix(child, "…") {
		t.Errorf("sub-issue line = %q, want it cut with an ellipsis", child)
	}
}

// TestDetailsDescriptionStillWraps covers the half that must keep wrapping:
// the prose below the metadata.
func TestDetailsDescriptionStillWraps(t *testing.T) {
	app := newDetailsTestApp(t)
	lines := drawDetails(t, app, 40)

	body := strings.Join(strings.Fields(strings.Join(lines, " ")), " ")
	if !strings.Contains(body, "rather than being cut off at the border.") {
		t.Errorf("description did not wrap into view:\n%s", strings.Join(lines, "\n"))
	}
}

// TestDetailsHeaderRefitsOnResize covers a widened pane: the header has to be
// re-cut against the new width instead of keeping the narrow truncation.
func TestDetailsHeaderRefitsOnResize(t *testing.T) {
	app := newDetailsTestApp(t)

	drawDetails(t, app, 40)
	narrow := findLine(t, drawDetails(t, app, 40), "Branch:")

	wide := findLine(t, drawDetails(t, app, 90), "Branch:")
	if strings.HasSuffix(wide, "…") {
		t.Errorf("branch line = %q, want the full name at 90 cells", wide)
	}
	if !strings.Contains(wide, "feature/zno-5-m3-comment-infrastructure") {
		t.Errorf("branch line = %q, want the full branch name", wide)
	}
	if len(wide) <= len(narrow) {
		t.Errorf("branch line did not grow with the pane: %q then %q", narrow, wide)
	}
}

// headerTexts is the metadata header as the rows it was built from, for a test
// that asserts what a row says rather than where it landed.
func headerTexts(app *App) []string {
	texts := make([]string, 0, len(app.detailsHeaderRows))
	for _, row := range app.detailsHeaderRows {
		texts = append(texts, row.text)
	}
	return texts
}

func findLine(t *testing.T, lines []string, substring string) string {
	t.Helper()
	for _, line := range lines {
		if strings.Contains(line, substring) {
			return line
		}
	}
	t.Fatalf("no line containing %q in:\n%s", substring, strings.Join(lines, "\n"))
	return ""
}

func indexOfLine(lines []string, want string) int {
	for i, line := range lines {
		if line == want {
			return i
		}
	}
	return -1
}

// TestDetailsPaneContentSitsInsideItsBorder pins the inner rect the draw func
// returns. Fitting the header means the draw func has to hand back the content
// area tview would have computed itself, and getting it wrong shifts every
// line without failing anything else.
func TestDetailsPaneContentSitsInsideItsBorder(t *testing.T) {
	densities := []struct {
		name       string
		id         string
		wantIndent int
	}{
		{"comfortable", config.DensityComfortable, 2},
		{"compact", config.DensityCompact, 1},
	}

	for _, density := range densities {
		t.Run(density.name, func(t *testing.T) {
			app := newDetailsTestApp(t)
			app.density = ResolveDensity(density.id)
			padding := app.density.DetailsPadding
			app.applyDensityToComponents()
			app.updateDetailsView()

			const width = 60
			lines := drawDetails(t, app, width)

			// drawDetails already drops the border columns, so the remaining
			// indent is the padding and nothing else.
			state := findLine(t, lines, "State:")
			if indent := len(state) - len(strings.TrimLeft(state, " ")); indent != density.wantIndent {
				t.Errorf("content indent = %d, want %d for %s padding", indent, density.wantIndent, density.name)
			}
			if got := app.detailsFittedWidth; got != width-2-padding.Left-padding.Right {
				t.Errorf("fitted width = %d, want %d", got, width-2-padding.Left-padding.Right)
			}
		})
	}
}

// TestTheTopPaddingScrollsWithThePage covers the gap above the issue: written
// into the text like the bottom, so it goes once the reader scrolls past it.
func TestTheTopPaddingScrollsWithThePage(t *testing.T) {
	app := newDetailsTestApp(t)
	if app.density.DetailsPadding.Top == 0 {
		t.Fatal("this density has no top padding to scroll")
	}

	// Row 0 is the pane's own border, so row 1 is the first row of the page.
	lines := drawDetails(t, app, 90)
	if strings.TrimSpace(lines[1]) != "" {
		t.Errorf("first page row = %q, want the padding", lines[1])
	}
	if !strings.Contains(lines[2], "ZNO-5") {
		t.Fatalf("second page row = %q, want the identifier", lines[2])
	}

	app.detailsPageView.ScrollTo(1, 0)
	scrolled := drawDetails(t, app, 90)
	if !strings.Contains(scrolled[1], "ZNO-5") {
		t.Errorf("one row down the top row = %q, want the padding gone", scrolled[1])
	}
}

// TestDetailsFieldOrder pins the metadata grid: what the issue is and who has
// it, then where it sits in the plan, then its dates.
func TestDetailsFieldOrder(t *testing.T) {
	app := newDetailsTestApp(t)
	lines := drawDetails(t, app, 90)

	want := []string{
		"State:", "Assignee:", "Priority:", "Labels:",
		"Project:", "Milestone:", "Cycle:",
		"Due date:", "Estimate:", "Branch:",
	}
	at := make([]int, len(want))
	for i, field := range want {
		at[i] = indexOfLine(lines, findLine(t, lines, field))
	}
	for i := 1; i < len(at); i++ {
		if at[i] <= at[i-1] {
			t.Errorf("%s at line %d, want it below %s at line %d", want[i], at[i], want[i-1], at[i-1])
		}
	}
}

// TestStateAndPriorityReadInWords covers the two fields that used to be a bare
// string and a bare number. Each carries the list's own glyph, so one issue
// reads the same in both places, and the color the list gives it.
func TestStateAndPriorityReadInWords(t *testing.T) {
	app := newDetailsTestApp(t)
	app.selectedIssue.State = "In Progress"
	app.selectedIssue.Priority = 1
	app.updateDetailsView()

	stateIcon, stateColor := formatStateIcon("In Progress", app.theme)
	priorityIcon, priorityColor := formatPriority(1, app.theme)

	for _, want := range []struct {
		field string
		line  string
	}{
		{"State", fmt.Sprintf("State:[-]      %s%s In Progress[-]", colorTag(stateColor), stateIcon)},
		{"Priority", fmt.Sprintf("Priority:[-]   %s%s Urgent[-]", colorTag(priorityColor), priorityIcon)},
	} {
		texts := headerTexts(app)
		if !slices.ContainsFunc(texts, func(line string) bool {
			return strings.Contains(line, want.line)
		}) {
			t.Errorf("no %s line reads %q, header:\n%s", want.field, want.line, strings.Join(texts, "\n"))
		}
	}

	// Drawn as well as built: a tag the renderer swallowed would leave the
	// glyph and the word uncolored with nothing to show for it.
	drawn := findLine(t, drawDetails(t, app, 90), "Priority:")
	if !strings.Contains(drawn, priorityIcon+" Urgent") {
		t.Errorf("the drawn priority line = %q, want the glyph and the word", drawn)
	}
}

// TestGriddedRowsLineUpAtTheGutter measures the drawn header. Every label on
// the grid is padded to the same column, so the values read as a column.
func TestGriddedRowsLineUpAtTheGutter(t *testing.T) {
	app := newDetailsTestApp(t)
	lines := drawDetails(t, app, 90)

	for _, label := range []string{
		"State:", "Assignee:", "Priority:", "Labels:", "Project:", "Milestone:",
		"Cycle:", "Due date:", "Estimate:", "Branch:", "Sub-issues:",
	} {
		line := findLine(t, lines, label)
		// Off the pane's own left padding, so column 0 is the start of the label.
		row := []rune(strings.TrimLeft(line, " "))
		if len(row) <= detailsLabelGutter {
			t.Errorf("%s row = %q, want a value at column %d", label, line, detailsLabelGutter)
			continue
		}
		if row[detailsLabelGutter-1] != ' ' || row[detailsLabelGutter] == ' ' {
			t.Errorf("%s row = %q, want its value to start at column %d", label, string(row), detailsLabelGutter)
		}
	}
}

// TestTheHeaderReportsWhereItsFieldsLanded covers the map the field cursor
// moves by: one span per editable field, in read order, on the row it is drawn.
func TestTheHeaderReportsWhereItsFieldsLanded(t *testing.T) {
	app := newDetailsTestApp(t)
	lines, spans, _ := app.detailsHeaderBlock(app.detailsMeasureWidth())

	want := []struct {
		field       issueField
		text        string
		valueColumn int
	}{
		{issueFieldTitle, "M3: comment infrastructure", 0},
		{issueFieldState, "State:", detailsLabelGutter},
		{issueFieldAssignee, "Assignee:", detailsLabelGutter},
		{issueFieldPriority, "Priority:", detailsLabelGutter},
		{issueFieldLabels, "Labels:", detailsLabelGutter},
		{issueFieldProject, "Project:", detailsLabelGutter},
		{issueFieldMilestone, "Milestone:", detailsLabelGutter},
		{issueFieldCycle, "Cycle:", detailsLabelGutter},
		{issueFieldDueDate, "Due date:", detailsLabelGutter},
		{issueFieldEstimate, "Estimate:", detailsLabelGutter},
	}
	if len(spans) != len(want) {
		t.Fatalf("%d field spans, want %d: %v", len(spans), len(want), spans)
	}
	for i, span := range spans {
		if span.field != want[i].field {
			t.Errorf("span %d is %q, want %q", i, span.field, want[i].field)
			continue
		}
		if !strings.Contains(lines[span.row], want[i].text) {
			t.Errorf("%s is on row %d, which reads %q", span.field, span.row, lines[span.row])
		}
		if span.valueColumn != want[i].valueColumn {
			t.Errorf("%s value column = %d, want %d", span.field, span.valueColumn, want[i].valueColumn)
		}
	}
}

// TestDetailsSectionOrder pins the order of the metadata sections below the
// fields: sub-issues, then subscribers, then relations, then attachments.
func TestDetailsSectionOrder(t *testing.T) {
	app := newDetailsTestApp(t)
	app.selectedIssue.Subscribers = []linearapi.User{{ID: "u1", DisplayName: "Ada Lovelace"}}
	app.selectedIssue.Relations = []linearapi.IssueRelation{{
		ID:           "r1",
		Type:         string(linearapi.IssueRelationBlocks),
		RelatedIssue: linearapi.IssueRef{ID: "i2", Identifier: "ZNO-7", Title: "Dependency"},
	}}
	app.selectedIssue.Attachments = []linearapi.Attachment{{
		ID: "a1", Title: "Pull request", SourceType: "github", URL: "https://example.com/pr/1",
	}}
	app.updateDetailsView()

	lines := drawDetails(t, app, 90)
	want := []string{"Sub-issues:", "Subscribers:", "Relations:", "Attachments:"}
	at := make([]int, len(want))
	for i, section := range want {
		at[i] = indexOfLine(lines, findLine(t, lines, section))
	}
	for i := 1; i < len(at); i++ {
		if at[i] <= at[i-1] {
			t.Errorf("%s at line %d, want it below %s at line %d", want[i], at[i], want[i-1], at[i-1])
		}
	}
}

// TestDetailsSectionLabelsKeepTheirOwnLine covers the compact density, where a
// rule carries no gap line either side and the label under it would otherwise
// be appended to it. This is the whole of the coverage for compact: the density
// setting itself does not currently apply, so nobody can check it by eye.
func TestDetailsSectionLabelsKeepTheirOwnLine(t *testing.T) {
	app := newDetailsTestApp(t)
	app.selectedIssue.Comments = threadedComments()
	app.density = ResolveDensity(config.DensityCompact)
	app.updateDetailsView()

	lines := drawComments(t, app, 60)
	for _, label := range []string{"Description:", "Activity"} {
		for _, line := range lines {
			if strings.Contains(line, "────") && strings.Contains(line, label) {
				t.Errorf("the rule and %q share a line: %q", label, line)
			}
		}
		findLine(t, lines, label)
	}
}

// TestDetailsPaneSurvivesAPaneNarrowerThanItsBorder covers the draw func
// handing tview a content rect: a pane with no room for its own border and
// padding must not produce a negative one.
func TestDetailsPaneSurvivesAPaneNarrowerThanItsBorder(t *testing.T) {
	app := newDetailsTestApp(t)

	for _, width := range []int{0, 1, 3, 5} {
		lines := drawDetails(t, app, width)
		if len(lines) == 0 {
			t.Fatalf("width %d drew nothing", width)
		}
		// The draw func's return value is what GetInnerRect reports back.
		_, _, innerWidth, innerHeight := app.detailsView.GetInnerRect()
		if innerWidth < 0 || innerHeight < 0 {
			t.Errorf("width %d gave a content rect of %dx%d", width, innerWidth, innerHeight)
		}
	}
}

// TestDetailsCapsTheReadingMeasure covers a details pane wider than a line of
// prose should be: the text holds its measure and centers, and the border still
// spans the pane.
func TestDetailsCapsTheReadingMeasure(t *testing.T) {
	const width = 180
	app := newDetailsTestApp(t)
	lines := drawDetails(t, app, width)

	title := findLine(t, lines, "M3: comment infrastructure")
	left := len(title) - len(strings.TrimLeft(title, " "))
	if wantGutter := (width - 2 - detailsMeasure) / 2; left != wantGutter {
		t.Errorf("content starts at column %d, want %d to center the measure", left, wantGutter)
	}

	// The first and last rows are the pane's own border, which is meant to span
	// the full width; the cap is on what is set inside it.
	for i, line := range lines[1 : len(lines)-1] {
		if got := len([]rune(line)); got > left+detailsMeasure {
			t.Errorf("line %d runs to %d cells, past the %d measure: %q", i+1, got, detailsMeasure, line)
		}
	}
}

// TestDetailsBelowTheMeasureUsesTheWholePane guards against the cap stealing
// width a narrow pane does not have to give.
func TestDetailsBelowTheMeasureUsesTheWholePane(t *testing.T) {
	app := newDetailsTestApp(t)
	lines := drawDetails(t, app, 60)

	title := findLine(t, lines, "M3: comment infrastructure")
	if left := len(title) - len(strings.TrimLeft(title, " ")); left != app.density.DetailsPadding.Left {
		t.Errorf("content starts at column %d, want the padding alone at %d", left, app.density.DetailsPadding.Left)
	}
}

// tableFixture is an issue whose description holds a markdown table wide enough
// that glamour has to size its columns to something.
func tableFixture() *linearapi.Issue {
	return &linearapi.Issue{
		ID: "issue-2", Identifier: "ZNO-6", Title: "Table", State: "Todo",
		Description: "Intro paragraph.\n\n" +
			"| Object | Paths to create |\n" +
			"| --- | --- |\n" +
			"| Workspace | the picker's add affordance (WindowController.swift:923), and Settings (:1121) |\n" +
			"| Tool | Settings only (SettingsToolsSection then onEditFloat then ToolFloatFormOverlay) |\n",
	}
}

// Glamour sizes table columns to its wrap width. Given none it laid the table
// out at its natural width and the text view re-wrapped it, which broke every
// row onto a continuation line with no column gutter.
func TestDescriptionTablesFitTheReadingMeasure(t *testing.T) {
	for _, width := range []int{180, 70} {
		app := newDetailsTestApp(t)
		app.selectedIssue = tableFixture()
		app.updateDetailsView()
		lines := drawDetails(t, app, width)

		// The table spans from its first gutter line to its last. A cell that
		// spilled past the width lands in that span as a line with no gutter
		// at all, which is exactly what the broken render looks like.
		//
		// The scan stops at the rule closing the description: the comment cards
		// below it are drawn from the same gutter rune and would otherwise read
		// as the far end of the table.
		first, last := -1, -1
		for i, line := range lines[:descriptionEnd(lines)] {
			if strings.Contains(line, "│") || strings.Contains(line, "┼") {
				if first < 0 {
					first = i
				}
				last = i
			}
		}
		if first < 0 {
			t.Fatalf("at %d: no table drawn at all", width)
		}
		for i := first; i <= last; i++ {
			line := strings.TrimSpace(lines[i])
			if line == "" {
				continue
			}
			if !strings.Contains(line, "│") && !strings.Contains(line, "┼") {
				t.Errorf("at %d: line %d sits inside the table but carries no column gutter: %q", width, i, lines[i])
			}
		}
	}
}

// The rule closing the metadata header spans the measure. Baked into the header
// lines at render time it stayed at whatever width the pane first drew at, so
// zooming left a short stub under a full-width header.
func TestTheHeaderDividerFollowsTheWidth(t *testing.T) {
	app := newDetailsTestApp(t)

	dividerWidth := func(width int) int {
		widest := 0
		lines := drawDetails(t, app, width)
		// The first and last rows are the pane's own border, which is drawn
		// from the same rune and spans the full width by design.
		for _, line := range lines[1 : len(lines)-1] {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.Trim(trimmed, "─") != "" {
				continue
			}
			if n := len([]rune(trimmed)); n > widest {
				widest = n
			}
		}
		return widest
	}

	narrow := dividerWidth(50)
	wide := dividerWidth(180)

	if narrow == 0 || wide == 0 {
		t.Fatalf("no divider drawn: narrow %d, wide %d", narrow, wide)
	}
	if wide <= narrow {
		t.Errorf("divider is %d cells at width 180 and %d at width 50, want it to follow the measure", wide, narrow)
	}
	if wide != detailsMeasure {
		t.Errorf("divider is %d cells at width 180, want the %d measure", wide, detailsMeasure)
	}
}

// The agent output modal renders unwrapped from its own goroutine while the
// details pane renders at its measure on the event loop. A single cached
// renderer both raced and handed each caller the other's wrap width.
func TestMarkdownRenderersAreSafeAcrossGoroutines(t *testing.T) {
	initMarkdownRenderer(LinearTheme)
	const doc = "| a | b |\n| --- | --- |\n| one | two |\n"

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			width := 0
			if i%2 == 0 {
				width = detailsMeasure
			}
			for range 20 {
				if out := renderMarkdownAt(doc, width); out == "" {
					t.Errorf("empty render at width %d", width)
					return
				}
			}
		}(i)
	}
	wg.Wait()
}

// The centered measure leaves gutters either side of the text. tview fills the
// inner rect with the text style whenever it disagrees with the box
// background, which painted those gutters a different color from the pane.
func TestTheReadingGutterMatchesThePaneBackground(t *testing.T) {
	themes := []struct {
		name    string
		id      string
		theme   Theme
		reapply bool
	}{
		// Both paths set the background, and they used to disagree: at build
		// it was hardcoded transparent, on a theme change it came from the
		// theme. A fresh launch takes the first, so it needs its own case.
		{"transparent at build", config.ThemeRosePineMoon, RosePineMoonTheme, false},
		{"opaque at build", config.ThemeLinear, LinearTheme, false},
		{"transparent after a theme change", config.ThemeRosePineMoon, RosePineMoonTheme, true},
		{"opaque after a theme change", config.ThemeLinear, LinearTheme, true},
	}

	for _, tc := range themes {
		t.Run(tc.name, func(t *testing.T) {
			app := NewApp(linearapi.ClientConfig{}, config.Config{CacheTTL: time.Minute, Theme: tc.id}, nil)
			stopBackgroundWorkOnCleanup(t, app)
			app.queueUpdateDraw = func(f func()) { f() }
			if tc.reapply {
				app.applyThemeToComponents()
			}
			app.selectedIssue = detailsFixture()
			app.updateDetailsView()

			const width = 180
			screen := tcell.NewSimulationScreen("UTF-8")
			if err := screen.Init(); err != nil {
				t.Fatalf("init simulation screen: %v", err)
			}
			t.Cleanup(screen.Fini)
			screen.SetSize(width, 40)
			app.detailsView.SetRect(0, 0, width, 40)
			app.detailsView.Draw(screen)
			screen.Show()

			cells, screenWidth, _ := screen.GetContents()
			// Row 10 is inside the content, past the title row.
			const row = 10
			backgrounds := map[tcell.Color]int{}
			for x := 1; x < screenWidth-1; x++ {
				_, bg, _ := cells[row*screenWidth+x].Style.Decompose()
				backgrounds[bg]++
			}
			if len(backgrounds) > 1 {
				t.Errorf("row %d paints %d different backgrounds across the pane, want one: %v", row, len(backgrounds), backgrounds)
			}
			// One color is not enough: agreeing on the terminal default would
			// also be uniform, and wrong for a theme that paints its own.
			if _, ok := backgrounds[tc.theme.Background]; !ok {
				t.Errorf("pane paints %v, want the theme background %v", backgrounds, tc.theme.Background)
			}
		})
	}
}
