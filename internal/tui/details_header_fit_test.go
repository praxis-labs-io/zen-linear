package tui

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/config"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/rivo/tview"
)

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

func drawDetails(t *testing.T, app *App, width int) []string {
	t.Helper()
	return drawTextView(t, app.detailsView, width)
}

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

func drawTextView(t *testing.T, view tview.Primitive, width int) []string {
	t.Helper()
	lines := drawPrimitive(t, view, width)
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

func drawPrimitive(t *testing.T, primitive tview.Primitive, width int) []string {
	t.Helper()
	return drawPrimitiveAt(t, primitive, width, 40)
}

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

func TestDetailsDescriptionStillWraps(t *testing.T) {
	app := newDetailsTestApp(t)
	lines := drawDetails(t, app, 40)

	body := strings.Join(strings.Fields(strings.Join(lines, " ")), " ")
	if !strings.Contains(body, "rather than being cut off at the border.") {
		t.Errorf("description did not wrap into view:\n%s", strings.Join(lines, "\n"))
	}
}

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

			state := findLine(t, lines, "Status:")
			if indent := len(state) - len(strings.TrimLeft(state, " ")); indent != density.wantIndent {
				t.Errorf("content indent = %d, want %d for %s padding", indent, density.wantIndent, density.name)
			}
			if got := app.detailsFittedWidth; got != width-2-padding.Left-padding.Right {
				t.Errorf("fitted width = %d, want %d", got, width-2-padding.Left-padding.Right)
			}
		})
	}
}

func TestTheTopPaddingScrollsWithThePage(t *testing.T) {
	app := newDetailsTestApp(t)
	if app.density.DetailsPadding.Top == 0 {
		t.Fatal("this density has no top padding to scroll")
	}

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

func TestDetailsFieldOrder(t *testing.T) {
	app := newDetailsTestApp(t)
	lines := drawDetails(t, app, 90)

	want := []string{
		"Status:", "Assignee:", "Priority:", "Labels:",
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
		{"Status", fmt.Sprintf("Status:[-]     %s%s In Progress[-]", colorTag(stateColor), stateIcon)},
		{"Priority", fmt.Sprintf("Priority:[-]   %s%s Urgent[-]", colorTag(priorityColor), priorityIcon)},
	} {
		texts := headerTexts(app)
		if !slices.ContainsFunc(texts, func(line string) bool {
			return strings.Contains(line, want.line)
		}) {
			t.Errorf("no %s line reads %q, header:\n%s", want.field, want.line, strings.Join(texts, "\n"))
		}
	}

	drawn := findLine(t, drawDetails(t, app, 90), "Priority:")
	if !strings.Contains(drawn, priorityIcon+" Urgent") {
		t.Errorf("the drawn priority line = %q, want the glyph and the word", drawn)
	}
}

func TestGriddedRowsLineUpAtTheGutter(t *testing.T) {
	app := newDetailsTestApp(t)
	lines := drawDetails(t, app, 90)

	for _, label := range []string{
		"Status:", "Assignee:", "Priority:", "Labels:", "Project:", "Milestone:",
		"Cycle:", "Due date:", "Estimate:", "Branch:", "Sub-issues:",
	} {
		line := findLine(t, lines, label)
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

func TestTheHeaderReportsWhereItsFieldsLanded(t *testing.T) {
	app := newDetailsTestApp(t)
	header := app.detailsHeaderBlock(app.detailsMeasureWidth())
	lines, spans := header.lines, header.fields

	want := []struct {
		field       issueField
		text        string
		valueColumn int
	}{
		{issueFieldTitle, "M3: comment infrastructure", 0},
		{issueFieldState, "Status:", detailsLabelGutter},
		{issueFieldAssignee, "Assignee:", detailsLabelGutter},
		{issueFieldPriority, "Priority:", detailsLabelGutter},
		{issueFieldLabels, "Labels:", detailsLabelGutter},
		{issueFieldTeam, "Team:", detailsLabelGutter},
		{issueFieldProject, "Project:", detailsLabelGutter},
		{issueFieldMilestone, "Milestone:", detailsLabelGutter},
		{issueFieldCycle, "Cycle:", detailsLabelGutter},
		{issueFieldDueDate, "Due date:", detailsLabelGutter},
		{issueFieldEstimate, "Estimate:", detailsLabelGutter},
		{issueFieldDescription, "Description:", 0},
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

func TestDetailsPaneSurvivesAPaneNarrowerThanItsBorder(t *testing.T) {
	app := newDetailsTestApp(t)

	for _, width := range []int{0, 1, 3, 5} {
		lines := drawDetails(t, app, width)
		if len(lines) == 0 {
			t.Fatalf("width %d drew nothing", width)
		}
		_, _, innerWidth, innerHeight := app.detailsView.GetInnerRect()
		if innerWidth < 0 || innerHeight < 0 {
			t.Errorf("width %d gave a content rect of %dx%d", width, innerWidth, innerHeight)
		}
	}
}

func TestDetailsCapsTheReadingMeasure(t *testing.T) {
	const width = 180
	app := newDetailsTestApp(t)
	lines := drawDetails(t, app, width)

	title := findLine(t, lines, "M3: comment infrastructure")
	left := len(title) - len(strings.TrimLeft(title, " "))
	if wantGutter := (width - 2 - detailsMeasure) / 2; left != wantGutter {
		t.Errorf("content starts at column %d, want %d to center the measure", left, wantGutter)
	}

	for i, line := range lines[1 : len(lines)-1] {
		if got := len([]rune(line)); got > left+detailsMeasure {
			t.Errorf("line %d runs to %d cells, past the %d measure: %q", i+1, got, detailsMeasure, line)
		}
	}
}

func TestDetailsBelowTheMeasureUsesTheWholePane(t *testing.T) {
	app := newDetailsTestApp(t)
	lines := drawDetails(t, app, 60)

	title := findLine(t, lines, "M3: comment infrastructure")
	if left := len(title) - len(strings.TrimLeft(title, " ")); left != app.density.DetailsPadding.Left {
		t.Errorf("content starts at column %d, want the padding alone at %d", left, app.density.DetailsPadding.Left)
	}
}

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

func TestDescriptionTablesFitTheReadingMeasure(t *testing.T) {
	for _, width := range []int{180, 70} {
		app := newDetailsTestApp(t)
		app.selectedIssue = tableFixture()
		app.updateDetailsView()
		lines := drawDetails(t, app, width)

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

func TestTheHeaderDividerFollowsTheWidth(t *testing.T) {
	app := newDetailsTestApp(t)

	dividerWidth := func(width int) int {
		widest := 0
		lines := drawDetails(t, app, width)
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

func TestTheReadingGutterMatchesThePaneBackground(t *testing.T) {
	themes := []struct {
		name    string
		id      string
		theme   Theme
		reapply bool
	}{
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
			const row = 10
			backgrounds := map[tcell.Color]int{}
			for x := 1; x < screenWidth-1; x++ {
				_, bg, _ := cells[row*screenWidth+x].Style.Decompose()
				backgrounds[bg]++
			}
			if len(backgrounds) > 1 {
				t.Errorf("row %d paints %d different backgrounds across the pane, want one: %v", row, len(backgrounds), backgrounds)
			}
			if _, ok := backgrounds[tc.theme.Background]; !ok {
				t.Errorf("pane paints %v, want the theme background %v", backgrounds, tc.theme.Background)
			}
		})
	}
}
