package tui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

const (
	issuesContextInset = 2
	contextSeparator   = " | "
)

func (a *App) attachIssuesContext(box *tview.Box) {
	box.SetDrawFunc(func(screen tcell.Screen, x, y, width, height int) (int, int, int, int) {
		a.drawIssuesContext(screen, x, y, width, height)
		return box.GetInnerRect()
	})
}

func (a *App) drawIssuesContext(screen tcell.Screen, x, y, width, height int) {
	if height < 2 {
		return
	}
	maxWidth := width - issuesContextInset - paneTitleWidth(a.issuesTitleLabel())
	text := a.issuesContextText(maxWidth)
	if text == "" {
		return
	}
	tview.Print(screen, text, x+width-issuesContextInset-maxWidth, y, maxWidth, tview.AlignRight, a.theme.SecondaryText)
}

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

func (a *App) teamKey(teamID string) string {
	for _, team := range a.navTeams {
		if team.ID == teamID {
			return team.Key
		}
	}
	return ""
}

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

func (a *App) renderContext(line []string) string {
	parts := make([]string, 0, len(line))
	for _, segment := range line {
		parts = append(parts, a.themeTags.SecondaryText+tview.Escape(segment)+"[-]")
	}
	return " " + strings.Join(parts, a.themeTags.Border+contextSeparator+"[-]") + " "
}
