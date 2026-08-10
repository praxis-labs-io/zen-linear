package tui

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
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

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	t.Cleanup(screen.Fini)

	const height = 40
	screen.SetSize(width, height)
	app.detailsDescriptionView.SetRect(0, 0, width, height)
	app.detailsDescriptionView.Draw(screen)
	screen.Show()

	// Columns 0 and width-1 are the pane border, which would otherwise sit on
	// the end of every line and hide what the content does there.
	cells, screenWidth, screenHeight := screen.GetContents()
	lines := make([]string, 0, screenHeight)
	for y := 0; y < screenHeight; y++ {
		row := make([]rune, 0, screenWidth)
		for x := 1; x < screenWidth-1; x++ {
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
			app.detailsDescriptionView.SetBorderPadding(padding.Top, padding.Bottom, padding.Left, padding.Right)
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

// TestDetailsDescriptionKeepsItsOwnLine covers the compact density, where the
// header ends on the divider with no trailing gap line and the description
// would otherwise be appended to it.
func TestDetailsDescriptionKeepsItsOwnLine(t *testing.T) {
	app := newDetailsTestApp(t)
	app.density = ResolveDensity(config.DensityCompact)
	app.updateDetailsView()

	lines := drawDetails(t, app, 60)
	for _, line := range lines {
		if strings.Contains(line, "────") && strings.Contains(line, "Description:") {
			t.Errorf("divider and description share a line: %q", line)
		}
	}
	findLine(t, lines, "Description:")
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
		_, _, innerWidth, innerHeight := app.detailsDescriptionView.GetInnerRect()
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
		first, last := -1, -1
		for i, line := range lines {
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
			app.detailsDescriptionView.SetRect(0, 0, width, 40)
			app.detailsDescriptionView.Draw(screen)
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
