package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/roeyazroel/linear-tui/internal/linearapi"
)

// Tree icons for expand/collapse indicators.
const (
	IconExpanded    = "▼"
	IconCollapsed   = "▶"
	IconChildPrefix = "└─"
)

// formatPriority renders a priority as an arrow glyph — up for high, equals
// for normal, down for low, triangle for urgent — all single-cell text
// presentation so rows stay aligned (no emoji-width variance).
// Linear priority: 0 = No priority, 1 = Urgent, 2 = High, 3 = Normal, 4 = Low.
func formatPriority(priority int, theme Theme) (string, tcell.Color) {
	switch priority {
	case 1:
		return "▲", theme.StatusCanceled // Red for urgent
	case 2:
		return "↑", theme.StatusInProgress // Yellow for high
	case 3:
		return "=", theme.Foreground // Default for normal
	case 4:
		return "↓", theme.SecondaryText // Gray for low
	default:
		return "-", theme.SecondaryText // No priority
	}
}

// formatGroupHeaderIcon returns the icon for a group header row based on its
// grouping dimension. Assignee and cycle groups have no icon.
func formatGroupHeaderIcon(row IssueRow, theme Theme) (string, tcell.Color) {
	switch row.HeaderDimension {
	case GroupByPriority:
		switch row.HeaderText {
		case "Urgent":
			return formatPriority(1, theme)
		case "High":
			return formatPriority(2, theme)
		case "Normal":
			return formatPriority(3, theme)
		case "Low":
			return formatPriority(4, theme)
		default:
			return formatPriority(0, theme)
		}
	case GroupByAssignee, GroupByCycle:
		return "", theme.SecondaryText
	default: // status
		return formatStateIcon(row.HeaderText, theme)
	}
}

// formatStateIcon renders a workflow state as a colored icon from a single
// circle family so every state occupies one cell at the same visual weight.
func formatStateIcon(state string, theme Theme) (string, tcell.Color) {
	lowerState := strings.ToLower(state)
	switch {
	case strings.Contains(lowerState, "done") || strings.Contains(lowerState, "complete"):
		return "●", theme.StatusDone
	case strings.Contains(lowerState, "review"):
		return "◉", theme.StatusReviewColor()
	case strings.Contains(lowerState, "progress"):
		return "◉", theme.StatusInProgress
	case strings.Contains(lowerState, "cancel") || strings.Contains(lowerState, "duplicate"):
		return "⊘", theme.StatusCanceled
	case strings.Contains(lowerState, "backlog"):
		return "◌", theme.SecondaryText
	default:
		return "○", theme.StatusTodo
	}
}

// formatUpdatedAt renders the last-updated timestamp like Linear's list view.
func formatUpdatedAt(updatedAt time.Time) string {
	if updatedAt.IsZero() {
		return "-"
	}
	if updatedAt.Year() != time.Now().Year() {
		return updatedAt.Format("Jan 2006")
	}
	return updatedAt.Format("Jan 2")
}

// formatLabels renders label names as a compact comma list.
func formatLabels(labels []linearapi.IssueLabel) string {
	if len(labels) == 0 {
		return "-"
	}
	names := make([]string, 0, len(labels))
	for _, label := range labels {
		names = append(names, label.Name)
	}
	return strings.Join(names, ", ")
}

// Issue list column identifiers, configurable via the columns setting.
const (
	ColumnPriority  = "priority"
	ColumnID        = "id"
	ColumnState     = "state"
	ColumnTitle     = "title"
	ColumnLabels    = "labels"
	ColumnAssignee  = "assignee"
	ColumnUpdated   = "updated"
	ColumnCycle     = "cycle"
	ColumnDue       = "due"
	ColumnEstimate  = "estimate"
	ColumnMilestone = "milestone"
)

// DefaultIssueColumns matches Linear's own list view.
var DefaultIssueColumns = []string{
	ColumnPriority, ColumnID, ColumnState, ColumnTitle, ColumnLabels, ColumnAssignee, ColumnUpdated,
}

// issueColumnSpec describes how one column renders.
type issueColumnSpec struct {
	header    string
	expansion int
	maxWidth  int
}

// issueColumnSpecs maps column identifiers to their rendering properties.
var issueColumnSpecs = map[string]issueColumnSpec{
	ColumnPriority:  {header: " ", expansion: 0},
	ColumnID:        {header: "ID", expansion: 0},
	ColumnState:     {header: " ", expansion: 0},
	ColumnTitle:     {header: "Title", expansion: 4, maxWidth: 45},
	ColumnLabels:    {header: "Labels", expansion: 1, maxWidth: 18},
	ColumnAssignee:  {header: "Assignee", expansion: 1},
	ColumnUpdated:   {header: "Updated", expansion: 0},
	ColumnCycle:     {header: "Cycle", expansion: 1, maxWidth: 15},
	ColumnDue:       {header: "Due", expansion: 0},
	ColumnEstimate:  {header: "Est", expansion: 0},
	ColumnMilestone: {header: "Milestone", expansion: 1, maxWidth: 18},
}

// columnIndex returns the position of a column in the layout, or fallback.
func columnIndex(columns []string, name string, fallback int) int {
	for index, column := range columns {
		if column == name {
			return index
		}
	}
	return fallback
}

// setIssuesTableHeaders writes the list header row for the configured
// columns. The default order and visibility match Linear's own list view.
func setIssuesTableHeaders(table *tview.Table, theme Theme, columns []string) {
	headerStyle := tcell.StyleDefault.
		Foreground(theme.HeaderText).
		Background(theme.HeaderBg).
		Bold(true)

	for column, name := range columns {
		spec := issueColumnSpecs[name]
		table.SetCell(0, column, tview.NewTableCell(spec.header).
			SetStyle(headerStyle).
			SetAlign(tview.AlignLeft).
			SetSelectable(false).
			SetExpansion(spec.expansion))
	}
}

// issueColumnCell renders one issue cell for a column identifier.
func issueColumnCell(name string, issue *linearapi.Issue, identifierPrefix string, theme Theme) (string, tcell.Color) {
	switch name {
	case ColumnPriority:
		return formatPriority(issue.Priority, theme)
	case ColumnID:
		return identifierPrefix + issue.Identifier, theme.SecondaryText
	case ColumnState:
		return formatStateIcon(issue.State, theme)
	case ColumnTitle:
		return issue.Title, theme.Foreground
	case ColumnLabels:
		labels := formatLabels(issue.Labels)
		if labels == "-" {
			return labels, theme.SecondaryText
		}
		return labels, theme.HeaderText
	case ColumnAssignee:
		if issue.Assignee == "" {
			return "-", theme.SecondaryText
		}
		assignee := issue.Assignee
		if len(assignee) > 14 {
			assignee = assignee[:14]
		}
		return assignee, theme.Foreground
	case ColumnUpdated:
		return formatUpdatedAt(issue.UpdatedAt), theme.SecondaryText
	case ColumnCycle:
		if issue.Cycle == nil {
			return "-", theme.SecondaryText
		}
		return issue.Cycle.DisplayName(), theme.Foreground
	case ColumnDue:
		due := formatDueDate(issue.DueDate)
		if due == "-" {
			return due, theme.SecondaryText
		}
		return due, theme.Foreground
	case ColumnEstimate:
		estimate := formatEstimate(issue.Estimate)
		if estimate == "-" {
			return estimate, theme.SecondaryText
		}
		return estimate, theme.Foreground
	case ColumnMilestone:
		milestone := formatMilestoneName(issue.ProjectMilestone)
		if milestone == "-" {
			return milestone, theme.SecondaryText
		}
		return milestone, theme.Foreground
	}
	return "", theme.SecondaryText
}

// getIssueFromRow returns the issue for a given table row (accounting for header).
// Returns nil if the row is invalid.
// This is a convenience wrapper that uses the current app's issueRows and idToIssue.
func (a *App) getIssueFromRow(row int) *linearapi.Issue {
	return getIssueFromRowModel(row, a.issueRows, a.idToIssue)
}

// getRowForIssue returns the table row for a given issue ID.
// Returns -1 if not found.
// This is a convenience wrapper that uses the current app's issueRows.
func (a *App) getRowForIssue(issueID string) int {
	return getRowForIssueModel(issueID, a.issueRows)
}

// getIssueFromRowModel returns the issue for a given table row using the provided model.
// Returns nil if the row is invalid.
func getIssueFromRowModel(row int, rows []IssueRow, idToIssue map[string]*linearapi.Issue) *linearapi.Issue {
	rowIndex := row - 1 // Account for header row
	if rowIndex < 0 || rowIndex >= len(rows) {
		return nil
	}
	issueID := rows[rowIndex].IssueID
	if issue, ok := idToIssue[issueID]; ok {
		return issue
	}
	return nil
}

// getRowForIssueModel returns the table row for a given issue ID using the provided model.
// Returns -1 if not found.
func getRowForIssueModel(issueID string, rows []IssueRow) int {
	for i, row := range rows {
		if row.IssueID == issueID {
			return i + 1 // +1 for header row
		}
	}
	return -1
}

// IssuesSection represents which issues section is active.
type IssuesSection int

const (
	IssuesSectionMy IssuesSection = iota
	IssuesSectionOther
)

// buildIssuesTable creates and configures an issues table widget with the given title.
// The table will use the provided getIssue and getRow functions for lookups.
func (a *App) buildIssuesTable(title string, section IssuesSection) *tview.Table {
	table := tview.NewTable()
	table.SetBorders(false). // Remove cell borders for cleaner look
					SetSelectable(true, false).
					SetBorder(true).
					SetTitle(title).
					SetTitleColor(a.theme.Foreground).
					SetBorderColor(a.theme.Border).
					SetBackgroundColor(a.theme.Background)

	table.SetSelectedStyle(tcell.StyleDefault.
		Foreground(a.theme.SelectionText).
		Background(a.theme.SelectionBg).
		Bold(true))

	setIssuesTableHeaders(table, a.theme, a.issueColumns())

	// Set fixed column widths
	table.SetFixed(1, 0)

	// Handle selection (Enter to open details or toggle expand)
	table.SetSelectedFunc(func(row, _ int) {
		issue := a.getIssueFromRowForSection(row, section)
		if issue == nil {
			return
		}

		// If issue has children, toggle expand/collapse
		if len(issue.Children) > 0 {
			a.toggleIssueExpanded(issue.ID)
			return
		}

		// Otherwise, focus on details
		a.onIssueSelected(*issue)
		a.focusedPane = FocusDetails
		a.updateFocus()
	})

	// Set up keyboard navigation with cross-section support
	a.setupIssuesTableNavigation(table, section)

	return table
}

// setupIssuesTableNavigation sets up keyboard navigation for an issues table with cross-section support.
func (a *App) setupIssuesTableNavigation(table *tview.Table, section IssuesSection) {
	table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyRune:
			return a.handleIssuesTableRune(table, section, event)
		case tcell.KeyEnter:
			row, _ := table.GetSelection()
			issue := a.getIssueFromRowForSection(row, section)
			if issue == nil {
				return nil
			}

			// If issue has children, toggle expand/collapse
			if len(issue.Children) > 0 {
				a.toggleIssueExpanded(issue.ID)
				a.activeIssuesSection = section
				return nil
			}

			// Otherwise, focus on details
			a.onIssueSelected(*issue)
			a.focusedPane = FocusDetails
			a.updateFocus()
			return nil
		case tcell.KeyDown:
			row, _ := table.GetSelection()
			if next := nextIssueRow(a.rowsForSection(section), row, 1); next > 0 {
				table.Select(next, 0)
				if issue := a.getIssueFromRowForSection(next, section); issue != nil {
					a.onIssueSelected(*issue)
					a.activeIssuesSection = section
				}
			} else if section == IssuesSectionMy && len(a.otherIssueRows) > 0 {
				// At bottom - move to the Other Issues table
				a.activeIssuesSection = IssuesSectionOther
				a.updateIssuesColumnLayout()
				if first := nextIssueRow(a.otherIssueRows, 0, 1); first > 0 {
					selectIssueRow(a.otherIssuesTable, a.otherIssueRows, first)
					if issue := a.getIssueFromRowForSection(first, IssuesSectionOther); issue != nil {
						a.onIssueSelected(*issue)
					}
				}
				a.updateFocus()
			}
			return nil
		case tcell.KeyUp:
			row, _ := table.GetSelection()
			if previous := nextIssueRow(a.rowsForSection(section), row, -1); previous > 0 {
				selectIssueRow(table, a.rowsForSection(section), previous)
				if issue := a.getIssueFromRowForSection(previous, section); issue != nil {
					a.onIssueSelected(*issue)
					a.activeIssuesSection = section
				}
			} else if section == IssuesSectionOther && len(a.myIssueRows) > 0 {
				// At top - move to the My Issues table
				a.activeIssuesSection = IssuesSectionMy
				a.updateIssuesColumnLayout()
				if last := nextIssueRow(a.myIssueRows, len(a.myIssueRows)+1, -1); last > 0 {
					a.myIssuesTable.Select(last, 0)
					if issue := a.getIssueFromRowForSection(last, IssuesSectionMy); issue != nil {
						a.onIssueSelected(*issue)
					}
				}
				a.updateFocus()
			}
			return nil
		}
		return event
	})
}

// rowsForSection returns the row model for the specified section.
func (a *App) rowsForSection(section IssuesSection) []IssueRow {
	switch section {
	case IssuesSectionMy:
		return a.myIssueRows
	case IssuesSectionOther:
		return a.otherIssueRows
	}
	return nil
}

// scrollIssueColumns scrolls the issue table horizontally: H left, L right.
func scrollIssueColumns(table *tview.Table, key rune) {
	rowOffset, columnOffset := table.GetOffset()
	switch {
	case key == 'H' && columnOffset > 0:
		table.SetOffset(rowOffset, columnOffset-1)
	case key == 'L' && columnOffset < table.GetColumnCount()-1:
		table.SetOffset(rowOffset, columnOffset+1)
	}
}

// selectIssueRow selects a table row and, when the selection reaches the
// first issue row, resets the scroll offset so leading group headers stay
// reachable (they are not selectable, so scrolling alone never reveals them).
func selectIssueRow(table *tview.Table, rows []IssueRow, row int) {
	table.Select(row, 0)
	if row == nextIssueRow(rows, 0, 1) {
		table.SetOffset(0, 0)
	}
}

// nextIssueRow returns the next table row holding an issue in the given
// direction, skipping status group headers. Returns 0 when none remains.
func nextIssueRow(rows []IssueRow, from int, delta int) int {
	for row := from + delta; row >= 1 && row <= len(rows); row += delta {
		if !rows[row-1].IsHeader {
			return row
		}
	}
	return 0
}

// handleIssuesTableRune handles single-rune keys for an issues table.
func (a *App) handleIssuesTableRune(table *tview.Table, section IssuesSection, event *tcell.EventKey) *tcell.EventKey {
	switch event.Rune() {
	case 'j':
		row, _ := table.GetSelection()
		if next := nextIssueRow(a.rowsForSection(section), row, 1); next > 0 {
			table.Select(next, 0)
			if issue := a.getIssueFromRowForSection(next, section); issue != nil {
				a.onIssueSelected(*issue)
				a.activeIssuesSection = section
			}
		} else if section == IssuesSectionMy && len(a.otherIssueRows) > 0 {
			// At bottom of this section - move to the Other Issues table
			a.activeIssuesSection = IssuesSectionOther
			a.updateIssuesColumnLayout()
			if first := nextIssueRow(a.otherIssueRows, 0, 1); first > 0 {
				selectIssueRow(a.otherIssuesTable, a.otherIssueRows, first)
				if issue := a.getIssueFromRowForSection(first, IssuesSectionOther); issue != nil {
					a.onIssueSelected(*issue)
				}
			}
			a.updateFocus()
		}
		return nil
	case 'k':
		row, _ := table.GetSelection()
		if previous := nextIssueRow(a.rowsForSection(section), row, -1); previous > 0 {
			selectIssueRow(table, a.rowsForSection(section), previous)
			if issue := a.getIssueFromRowForSection(previous, section); issue != nil {
				a.onIssueSelected(*issue)
				a.activeIssuesSection = section
			}
		} else if section == IssuesSectionOther && len(a.myIssueRows) > 0 {
			// At top of this section - move to the My Issues table
			a.activeIssuesSection = IssuesSectionMy
			a.updateIssuesColumnLayout()
			if last := nextIssueRow(a.myIssueRows, len(a.myIssueRows)+1, -1); last > 0 {
				a.myIssuesTable.Select(last, 0)
				if issue := a.getIssueFromRowForSection(last, IssuesSectionMy); issue != nil {
					a.onIssueSelected(*issue)
				}
			}
			a.updateFocus()
		}
		return nil
	case 'g':
		// Go to top of current section
		if first := nextIssueRow(a.rowsForSection(section), 0, 1); first > 0 {
			selectIssueRow(table, a.rowsForSection(section), first)
			if issue := a.getIssueFromRowForSection(first, section); issue != nil {
				a.onIssueSelected(*issue)
				a.activeIssuesSection = section
			}
		}
		return nil
	case 'G':
		// Go to bottom of current section
		rows := a.rowsForSection(section)
		if last := nextIssueRow(rows, len(rows)+1, -1); last > 0 {
			table.Select(last, 0)
			if issue := a.getIssueFromRowForSection(last, section); issue != nil {
				a.onIssueSelected(*issue)
				a.activeIssuesSection = section
			}
		}
		return nil
	case 'l':
		// Expand current parent issue
		row, _ := table.GetSelection()
		if issue := a.getIssueFromRowForSection(row, section); issue != nil {
			if len(issue.Children) > 0 && !a.expandedState[issue.ID] {
				a.toggleIssueExpanded(issue.ID)
				a.activeIssuesSection = section
			}
		}
		return nil
	case 'h':
		// Collapse current parent issue, or go to parent if on child
		row, _ := table.GetSelection()
		if issue := a.getIssueFromRowForSection(row, section); issue != nil {
			if len(issue.Children) > 0 && a.expandedState[issue.ID] {
				// Collapse this parent
				a.toggleIssueExpanded(issue.ID)
				a.activeIssuesSection = section
			} else if issue.Parent != nil {
				// Navigate to parent - may be in different section
				parentRow := a.getRowForIssueInSection(issue.Parent.ID, IssuesSectionMy)
				if parentRow > 0 {
					a.activeIssuesSection = IssuesSectionMy
					a.myIssuesTable.Select(parentRow, 0)
					if parent := a.getIssueFromRowForSection(parentRow, IssuesSectionMy); parent != nil {
						a.onIssueSelected(*parent)
					}
					a.updateFocus()
				} else {
					parentRow = a.getRowForIssueInSection(issue.Parent.ID, IssuesSectionOther)
					if parentRow > 0 {
						a.activeIssuesSection = IssuesSectionOther
						a.otherIssuesTable.Select(parentRow, 0)
						if parent := a.getIssueFromRowForSection(parentRow, IssuesSectionOther); parent != nil {
							a.onIssueSelected(*parent)
						}
						a.updateFocus()
					}
				}
			}
		}
		return nil
	case 'H', 'L':
		scrollIssueColumns(table, event.Rune())
		return nil
	case ' ':
		// Space toggles expand/collapse
		row, _ := table.GetSelection()
		if issue := a.getIssueFromRowForSection(row, section); issue != nil {
			if len(issue.Children) > 0 {
				a.toggleIssueExpanded(issue.ID)
				a.activeIssuesSection = section
			}
		}
		return nil
	}
	return event
}

// getIssueFromRowForSection returns the issue for a given table row in the specified section.
func (a *App) getIssueFromRowForSection(row int, section IssuesSection) *linearapi.Issue {
	var rows []IssueRow
	var idToIssue map[string]*linearapi.Issue
	switch section {
	case IssuesSectionMy:
		rows = a.myIssueRows
		idToIssue = a.myIDToIssue
	case IssuesSectionOther:
		rows = a.otherIssueRows
		idToIssue = a.otherIDToIssue
	}
	return getIssueFromRowModel(row, rows, idToIssue)
}

// getRowForIssueInSection returns the table row for a given issue ID in the specified section.
func (a *App) getRowForIssueInSection(issueID string, section IssuesSection) int {
	var rows []IssueRow
	switch section {
	case IssuesSectionMy:
		rows = a.myIssueRows
	case IssuesSectionOther:
		rows = a.otherIssueRows
	}
	return getRowForIssueModel(issueID, rows)
}

// renderIssuesTableModel renders a table with the given rows and issue lookup map.
func renderIssuesTableModel(table *tview.Table, rows []IssueRow, idToIssue map[string]*linearapi.Issue, selectedIssueID string, theme Theme, columns []string) {
	if len(columns) == 0 {
		columns = DefaultIssueColumns
	}
	table.Clear()

	setIssuesTableHeaders(table, theme, columns)

	// Add issue rows using the hierarchical structure
	for i, issueRow := range rows {
		row := i + 1

		if issueRow.IsHeader {
			for column := range columns {
				table.SetCell(row, column, tview.NewTableCell("").SetSelectable(false))
			}
			labelIndex := columnIndex(columns, ColumnTitle, 0)
			if stateIndex := columnIndex(columns, ColumnState, -1); stateIndex >= 0 && stateIndex != labelIndex {
				icon, iconColor := formatGroupHeaderIcon(issueRow, theme)
				if icon != "" {
					table.SetCell(row, stateIndex, tview.NewTableCell(icon).
						SetTextColor(iconColor).
						SetSelectable(false))
				}
			}
			indent := strings.Repeat("  ", issueRow.HeaderLevel)
			// Group labels read distinctly from issue titles: accent for the
			// main level, subtle header color for subgroups.
			labelColor := theme.Accent
			if issueRow.HeaderLevel > 0 {
				labelColor = theme.HeaderText
			}
			table.SetCell(row, labelIndex, tview.NewTableCell(fmt.Sprintf("%s%s (%d)", indent, issueRow.HeaderText, issueRow.HeaderCount)).
				SetTextColor(labelColor).
				SetAttributes(tcell.AttrBold).
				SetSelectable(false))
			continue
		}

		issue, ok := idToIssue[issueRow.IssueID]
		if !ok || issue == nil {
			continue
		}

		// Build identifier with hierarchy indicator
		identifierPrefix := " "

		if issueRow.Level > 0 {
			// Child issue - show indent prefix
			identifierPrefix = " " + IconChildPrefix + " "
		} else if issueRow.HasChildren {
			// Parent issue - show expand/collapse indicator
			if issueRow.IsExpanded {
				identifierPrefix = " " + IconExpanded + " "
			} else {
				identifierPrefix = " " + IconCollapsed + " "
			}
		}

		for column, name := range columns {
			cellText, cellColor := issueColumnCell(name, issue, identifierPrefix, theme)
			if column == 0 && name != ColumnID {
				cellText = " " + cellText
			}
			cell := tview.NewTableCell(cellText).
				SetTextColor(cellColor).
				SetAlign(tview.AlignLeft)
			if spec := issueColumnSpecs[name]; spec.maxWidth > 0 {
				cell.SetMaxWidth(spec.maxWidth)
			}
			table.SetCell(row, column, cell)
		}
	}

	// Select the specified issue or first issue row (skipping group headers)
	if len(rows) > 0 {
		selectedRow := nextIssueRow(rows, 0, 1)
		if selectedIssueID != "" {
			// Find the row with matching issue ID
			for i, row := range rows {
				if row.IssueID == selectedIssueID {
					selectedRow = i + 1 // +1 because row 0 is header
					break
				}
			}
		}
		if selectedRow < 1 {
			selectedRow = 1
		}
		selectIssueRow(table, rows, selectedRow)
	} else {
		// Show empty state message
		for column := range columns {
			table.SetCell(1, column, tview.NewTableCell("").SetSelectable(false))
		}
		table.SetCell(1, columnIndex(columns, ColumnTitle, 0), tview.NewTableCell("No issues").
			SetTextColor(theme.SecondaryText).
			SetAlign(tview.AlignCenter).
			SetSelectable(false))
	}
}

func formatDueDate(dueDate *string) string {
	if dueDate == nil || strings.TrimSpace(*dueDate) == "" {
		return "-"
	}
	return strings.TrimSpace(*dueDate)
}

func formatEstimate(estimate *float64) string {
	if estimate == nil {
		return "-"
	}
	return strconv.FormatFloat(*estimate, 'f', -1, 64)
}

func formatMilestoneName(milestone *linearapi.ProjectMilestoneRef) string {
	if milestone == nil || strings.TrimSpace(milestone.Name) == "" {
		return "-"
	}
	return milestone.Name
}

// renderIssueRow formats an issue for display in the table, in column order:
// priority, id, state, title, labels, assignee, updated.
// This is a helper function that can be used for testing.
func renderIssueRow(issue linearapi.Issue) []string {
	identifier := issue.Identifier
	if len(identifier) > 10 {
		identifier = identifier[:10]
	}

	priorityText, _ := formatPriority(issue.Priority, LinearTheme)
	stateIcon, _ := formatStateIcon(issue.State, LinearTheme)

	assignee := issue.Assignee
	if assignee == "" {
		assignee = "-"
	}
	if len(assignee) > 14 {
		assignee = assignee[:14]
	}

	return []string{priorityText, identifier, stateIcon, issue.Title, formatLabels(issue.Labels), assignee, formatUpdatedAt(issue.UpdatedAt)}
}
