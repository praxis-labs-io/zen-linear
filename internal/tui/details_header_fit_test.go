package tui

import (
	"strings"
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
