package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// Tree icons for expand/collapse indicators.
const (
	IconExpanded    = "▼"
	IconCollapsed   = "▶"
	IconChildPrefix = "└─"
)

// priorityLabels names each Linear priority. The index is the priority value,
// so pickers can map a selection straight onto the API field.
var priorityLabels = []string{"No priority", "Urgent", "High", "Normal", "Low"}

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
		return "⊙", theme.StatusInProgress
	case strings.Contains(lowerState, "triage"):
		return "◎", theme.StatusTriageColor()
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

// formatAssigneeInitials condenses a full name to the first letters of its
// first and last words, so the column costs two cells instead of a name's
// worth. A one-word name gives one letter. An empty name gives an empty
// string, which callers render as unassigned.
func formatAssigneeInitials(name string) string {
	words := strings.Fields(name)
	if len(words) == 0 {
		return ""
	}
	initials := initialLetter(words[0])
	if len(words) > 1 {
		initials += initialLetter(words[len(words)-1])
	}
	return initials
}

// initialLetter returns a word's first rune, uppercased. strings.Fields never
// yields an empty word, so there is always a rune to read.
func initialLetter(word string) string {
	first, _ := utf8.DecodeRuneInString(word)
	return string(unicode.ToUpper(first))
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
	ColumnProject   = "project"
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
	ColumnAssignee:  {header: " ", expansion: 0},
	ColumnUpdated:   {header: "Updated", expansion: 0},
	ColumnCycle:     {header: "Cycle", expansion: 1, maxWidth: 15},
	ColumnDue:       {header: "Due", expansion: 0},
	ColumnEstimate:  {header: "Est", expansion: 0},
	ColumnProject:   {header: "Project", expansion: 1, maxWidth: 18},
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
		table.SetCell(0, column, tview.NewTableCell(headerText(spec.header, name, column)).
			SetStyle(headerStyle).
			SetAlign(tview.AlignLeft).
			SetSelectable(false).
			SetExpansion(spec.expansion))
	}
}

// headerText indents a column header to match the lead space its cells carry:
// the ID column reserves one for the tree icon wherever it sits, and whatever
// column lands first gets one from setIssueRowCells.
func headerText(header, name string, column int) string {
	if name == ColumnID || column == 0 {
		return " " + header
	}
	return header
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
		initials := formatAssigneeInitials(issue.Assignee)
		if initials == "" {
			return "-", theme.SecondaryText
		}
		return initials, theme.AssigneeTextColor()
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
	case ColumnProject:
		if issue.ProjectName == "" {
			return "-", theme.SecondaryText
		}
		return issue.ProjectName, theme.Foreground
	case ColumnMilestone:
		milestone := formatMilestoneName(issue.ProjectMilestone)
		if milestone == "-" {
			return milestone, theme.SecondaryText
		}
		return milestone, theme.Foreground
	}
	return "", theme.SecondaryText
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

// All is the zero value so a freshly built App opens on it.
const (
	IssuesSectionAll IssuesSection = iota
	IssuesSectionMy
	IssuesSectionSearch
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

	// Handle selection (Enter toggles the details pane; Space toggles expand)
	table.SetSelectedFunc(func(row, _ int) {
		if rows := a.rowsForSection(section); row >= 1 && row <= len(rows) && rows[row-1].IsHeader {
			a.toggleGroupCollapse(section, rows[row-1])
			return
		}
		issue := a.getIssueFromRowForSection(row, section)
		if issue != nil {
			a.selectIssueNow(*issue)
		}
		a.toggleDetailsPane()
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
			// Enter toggles a header's group, or the details pane while
			// staying in the list; Space toggles expand/collapse on parents.
			row, _ := table.GetSelection()
			if rows := a.rowsForSection(section); row >= 1 && row <= len(rows) && rows[row-1].IsHeader {
				a.toggleGroupCollapse(section, rows[row-1])
				return nil
			}
			if issue := a.getIssueFromRowForSection(row, section); issue != nil {
				a.selectIssueNow(*issue)
				a.activeIssuesSection = section
			}
			a.toggleDetailsPane()
			return nil
		case tcell.KeyDown:
			row, _ := table.GetSelection()
			if next := nextSelectableRow(a.rowsForSection(section), row, 1); next > 0 {
				table.Select(next, 0)
				if issue := a.getIssueFromRowForSection(next, section); issue != nil {
					a.onIssueSelected(*issue)
					a.activeIssuesSection = section
				}
			}
			return nil
		case tcell.KeyUp:
			row, _ := table.GetSelection()
			if previous := nextSelectableRow(a.rowsForSection(section), row, -1); previous > 0 {
				selectIssueRow(table, a.rowsForSection(section), previous)
				if issue := a.getIssueFromRowForSection(previous, section); issue != nil {
					a.onIssueSelected(*issue)
					a.activeIssuesSection = section
				}
			} else if section == IssuesSectionSearch {
				// At top of search results - return to the search input
				a.focusSearchInput()
			}
			return nil
		}
		return event
	})
}

// rowsForSection returns the row model for the specified section.
func (a *App) rowsForSection(section IssuesSection) []IssueRow {
	switch section {
	case IssuesSectionAll:
		return a.allIssueRows
	case IssuesSectionMy:
		return a.myIssueRows
	case IssuesSectionSearch:
		return a.searchIssueRows
	}
	return nil
}

// issueMapForSection returns the id lookup backing a section's rows.
func (a *App) issueMapForSection(section IssuesSection) map[string]*linearapi.Issue {
	switch section {
	case IssuesSectionAll:
		return a.allIDToIssue
	case IssuesSectionMy:
		return a.myIDToIssue
	case IssuesSectionSearch:
		return a.searchIDToIssue
	}
	return nil
}

// renderIssueSections paints the mounted section and defers the rest. Only one
// tab is ever on screen, so rendering all three costs three tables' worth of
// cell allocations to show one, and resets the off-screen tabs' selection to
// the first row every time. Deferred sections paint in
// updateIssuesColumnLayout, which every tab switch already routes through.
func (a *App) renderIssueSections(selected map[IssuesSection]string) {
	if a.pendingSectionRenders == nil {
		a.pendingSectionRenders = make(map[IssuesSection]string, len(selected))
	}
	active := a.activeIssuesSection
	for section, issueID := range selected {
		if section == active {
			delete(a.pendingSectionRenders, section)
			a.renderIssueSection(section, issueID)
			continue
		}
		a.pendingSectionRenders[section] = issueID
	}
}

func (a *App) renderIssueSection(section IssuesSection, selectedIssueID string) {
	table := a.tableForSection(section)
	if table == nil {
		return
	}
	renderIssuesTableModel(table, a.rowsForSection(section), a.issueMapForSection(section), selectedIssueID, a.theme, a.issueColumns())
}

// flushPendingSectionRender paints a section whose render was deferred while it
// was off screen.
func (a *App) flushPendingSectionRender(section IssuesSection) {
	selectedIssueID, pending := a.pendingSectionRenders[section]
	if !pending {
		return
	}
	delete(a.pendingSectionRenders, section)
	a.renderIssueSection(section, selectedIssueID)
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
	if row <= nextIssueRow(rows, 0, 1) {
		table.SetOffset(0, 0)
	}
}

// nextIssueRow returns the next table row holding an issue in the given
// direction, skipping group headers and spacer rows. Returns 0 when none
// remains.
func nextIssueRow(rows []IssueRow, from int, delta int) int {
	for row := from + delta; row >= 1 && row <= len(rows); row += delta {
		if !rows[row-1].IsHeader && !rows[row-1].IsSpacer {
			return row
		}
	}
	return 0
}

// nextSelectableRow returns the next selectable table row (issue or header)
// in the given direction, skipping spacer rows. Returns 0 when none remains.
func nextSelectableRow(rows []IssueRow, from int, delta int) int {
	for row := from + delta; row >= 1 && row <= len(rows); row += delta {
		if !rows[row-1].IsSpacer {
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
		if next := nextSelectableRow(a.rowsForSection(section), row, 1); next > 0 {
			table.Select(next, 0)
			if issue := a.getIssueFromRowForSection(next, section); issue != nil {
				a.onIssueSelected(*issue)
				a.activeIssuesSection = section
			}
		}
		return nil
	case 'k':
		row, _ := table.GetSelection()
		if previous := nextSelectableRow(a.rowsForSection(section), row, -1); previous > 0 {
			selectIssueRow(table, a.rowsForSection(section), previous)
			if issue := a.getIssueFromRowForSection(previous, section); issue != nil {
				a.onIssueSelected(*issue)
				a.activeIssuesSection = section
			}
		} else if section == IssuesSectionSearch {
			// At top of search results - return to the search input
			a.focusSearchInput()
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
	// h and l never arrive here: handleIssuesKey claims them for pane movement,
	// which is what the README documents. Space expands, p jumps to the parent.
	// g only arrives when no command shortcut owns it, and edit_labels holds it
	// by default, so go-to-top above is dead until ZNL-32 reworks the defaults.
	case a.actionKey("columns_left", 'H'):
		scrollIssueColumns(table, 'H')
		return nil
	case a.actionKey("columns_right", 'L'):
		scrollIssueColumns(table, 'L')
		return nil
	case ' ':
		if section == IssuesSectionSearch {
			return nil // search results are a flat list
		}
		// Space toggles expand/collapse
		row, _ := table.GetSelection()
		if rows := a.rowsForSection(section); row >= 1 && row <= len(rows) && rows[row-1].IsHeader {
			a.toggleGroupCollapse(section, rows[row-1])
			return nil
		}
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
	return getIssueFromRowModel(row, a.rowsForSection(section), a.issueMapForSection(section))
}

// getRowForIssueInSection returns the table row for a given issue ID in the specified section.
func (a *App) getRowForIssueInSection(issueID string, section IssuesSection) int {
	return getRowForIssueModel(issueID, a.rowsForSection(section))
}

// buildFlatSearchRows maps search results 1:1 to rows, preserving the API's
// relevance order: no grouping, no parent/child nesting.
func buildFlatSearchRows(issues []linearapi.Issue) ([]IssueRow, map[string]*linearapi.Issue) {
	rows := make([]IssueRow, 0, len(issues))
	idToIssue := make(map[string]*linearapi.Issue, len(issues))
	for i := range issues {
		issue := &issues[i]
		rows = append(rows, IssueRow{IssueID: issue.ID})
		idToIssue[issue.ID] = issue
	}
	return rows, idToIssue
}

// setIssueRowCells writes one issue's cells into a table row. Splitting this
// out of the render loop lets a single-issue update repaint its own row instead
// of clearing and reallocating the whole table.
func setIssueRowCells(table *tview.Table, row int, issueRow IssueRow, issue *linearapi.Issue, theme Theme, columns []string) {
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

		if issueRow.IsSpacer {
			for column := range columns {
				table.SetCell(row, column, tview.NewTableCell("").SetSelectable(false))
			}
			continue
		}

		if issueRow.IsHeader {
			for column := range columns {
				table.SetCell(row, column, tview.NewTableCell("").SetSelectable(true))
			}
			labelIndex := columnIndex(columns, ColumnTitle, 0)
			if stateIndex := columnIndex(columns, ColumnState, -1); stateIndex >= 0 && stateIndex != labelIndex {
				icon, iconColor := formatGroupHeaderIcon(issueRow, theme)
				if icon != "" {
					table.SetCell(row, stateIndex, tview.NewTableCell(icon).
						SetTextColor(iconColor).
						SetSelectable(true))
				}
			}
			indicator := "▾ "
			if issueRow.HeaderCollapsed {
				indicator = "▸ "
			}
			indent := strings.Repeat("  ", issueRow.HeaderLevel)
			// Group labels read distinctly from issue titles: accent for the
			// main level, subtle header color for subgroups.
			labelColor := theme.Accent
			if issueRow.HeaderLevel > 0 {
				labelColor = theme.HeaderText
			}
			table.SetCell(row, labelIndex, tview.NewTableCell(fmt.Sprintf("%s%s%s (%d)", indent, indicator, issueRow.HeaderText, issueRow.HeaderCount)).
				SetTextColor(labelColor).
				SetAttributes(tcell.AttrBold).
				SetSelectable(true))
			continue
		}

		issue, ok := idToIssue[issueRow.IssueID]
		if !ok || issue == nil {
			continue
		}

		setIssueRowCells(table, row, issueRow, issue, theme, columns)
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

	assignee := formatAssigneeInitials(issue.Assignee)
	if assignee == "" {
		assignee = "-"
	}

	return []string{priorityText, identifier, stateIcon, issue.Title, formatLabels(issue.Labels), assignee, formatUpdatedAt(issue.UpdatedAt)}
}
