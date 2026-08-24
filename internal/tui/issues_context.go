package tui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

const (
	// issuesContextInset is the gap the context line keeps from the corner.
	issuesContextInset = 2
	contextSeparator   = " | "
)

// attachIssuesContext writes how the list is ordered and filtered along the top
// border of a pane standing in for the issues list, right of the title. It
// reads live state on every draw, so nothing has to remember to refresh it.
func (a *App) attachIssuesContext(box *tview.Box) {
	box.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
		a.drawIssuesContext(screen, x, y, width, height)
		return box.GetInnerRect()
	})
}

// drawIssuesContext paints the context line over the top border. A draw func
// runs after the border, which is what lets the line sit in it.
func (a *App) drawIssuesContext(screen tcell.Screen, x, y, width, height int) {
	if height < 2 {
		return
	}
	// The title holds the left of this same row, so the line gets what is left
	// of it. Anything wider would overwrite the list's name.
	maxWidth := width - issuesContextInset - paneTitleWidth(a.issuesTitleLabel())
	text := a.issuesContextText(maxWidth)
	if text == "" {
		return
	}
	tview.Print(screen, text, x+width-issuesContextInset-maxWidth, y, maxWidth, tview.AlignRight, a.theme.SecondaryText)
}

// issuesContextText renders how the list is ordered and what it is filtered by.
// Sort is dropped first when the pair will not fit, being the part a reader can
// also see in the rows; below that the line falls back to whichever single fact
// still fits rather than going blank. Search takes neither, so it gets no line.
func (a *App) issuesContextText(maxWidth int) string {
	if a.activeIssuesSection == IssuesSectionSearch || maxWidth <= 4 {
		return ""
	}

	sorted := "Sort: " + sortChainLabel(a.effectiveSortFields())
	filtered := ""
	if !a.richFilters.Empty() {
		filtered = "Filters: " + a.richFilters.Summary()
	}

	for _, line := range [][]string{
		{sorted, filtered},
		{filtered},
		{sorted},
	} {
		line = withoutEmptySegments(line)
		if len(line) > 0 && contextWidth(line) <= maxWidth {
			return a.renderContext(line)
		}
	}

	return ""
}

// issuesScopeLabel names the list on screen: the team's key and what was picked
// in the tree. A team's own row is left unprefixed, where the key would say the
// same thing twice.
func (a *App) issuesScopeLabel() string {
	node := a.selectedNavigation
	if node == nil {
		return ""
	}
	label := node.Text
	switch {
	case node.IsStatus && node.StateName != "":
		label = "Status: " + node.StateName
	case node.IsCycle && node.CycleName != "":
		label = "Cycle: " + node.CycleName
	}
	if key := a.teamKey(node.TeamID); key != "" && !node.IsTeam {
		return key + ": " + label
	}
	return label
}

// teamKey returns a team's identifier, or nothing when the tree has not been
// built from a teams fetch yet.
func (a *App) teamKey(teamID string) string {
	for _, team := range a.navTeams {
		if team.ID == teamID {
			return team.Key
		}
	}
	return ""
}

// teamName is the team's own name, empty while the tree has not painted.
func (a *App) teamName(teamID string) string {
	for _, team := range a.navTeams {
		if team.ID == teamID {
			return team.Name
		}
	}
	return ""
}

func withoutEmptySegments(line []string) []string {
	kept := make([]string, 0, len(line))
	for _, segment := range line {
		if segment != "" {
			kept = append(kept, segment)
		}
	}
	return kept
}

// contextWidth measures a line as drawn, including the separators and the space
// that keeps each end off the border.
func contextWidth(line []string) int {
	width := 2
	for index, segment := range line {
		if index > 0 {
			width += runewidth.StringWidth(contextSeparator)
		}
		width += runewidth.StringWidth(segment)
	}
	return width
}

// renderContext says the whole line in the secondary color. It is context, not
// a warning: the border it sits in already sets it apart from the rows.
func (a *App) renderContext(line []string) string {
	parts := make([]string, 0, len(line))
	for _, segment := range line {
		// Project, cycle and label names are Linear's, and this line is built
		// from color tags: a project called [red] would be read as one instead
		// of printed. The width is measured off the text as it stands, since
		// the escape is not drawn.
		parts = append(parts, a.themeTags.SecondaryText+tview.Escape(segment)+"[-]")
	}
	return " " + strings.Join(parts, a.themeTags.Border+contextSeparator+"[-]") + " "
}
