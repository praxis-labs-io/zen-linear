package tui

import (
	"context"

	"github.com/roeyazroel/linear-tui/internal/linearapi"
	"github.com/roeyazroel/linear-tui/internal/logger"
)

// applyIssueUpdate folds a mutation result into the local list instead of
// refetching. UpdateIssue returns the issue it wrote with everything the list
// query selects, so a refresh would spend every page of the workspace plus a
// full rebuild of every table to change one row.
func (a *App) applyIssueUpdate(updated linearapi.Issue) {
	if updated.ID == "" {
		return
	}

	a.issuesMu.Lock()
	index := -1
	for i := range a.issues {
		if a.issues[i].ID == updated.ID {
			index = i
			break
		}
	}
	if index < 0 {
		a.issuesMu.Unlock()
		// The issue is outside the current filter or list; only a fetch can say
		// where it belongs now.
		go a.refreshIssues(updated.ID)
		return
	}
	// The mutation selection carries no comments, relations, subscribers, or
	// attachments. The details pane loaded those separately and a straight
	// replace would drop them.
	existing := a.issues[index]
	updated.Comments = existing.Comments
	updated.Relations = existing.Relations
	updated.Subscribers = existing.Subscribers
	updated.Attachments = existing.Attachments
	a.issues[index] = updated
	a.sortIssuesLocally()
	a.issuesMu.Unlock()

	previousRows := map[IssuesSection][]IssueRow{
		IssuesSectionMy:    a.myIssueRows,
		IssuesSectionOther: a.otherIssueRows,
		IssuesSectionAll:   a.issueRows,
	}
	previousSection := a.activeIssuesSection

	a.rebuildIssueRowModels()
	selections := a.sectionSelectionsFor(updated.ID)

	deferred := make(map[IssuesSection]string, len(selections))
	for section, selectedIssueID := range selections {
		if issueRowsEqual(previousRows[section], a.rowsForSection(section)) {
			// Nothing moved, so only this issue's own cells can have changed.
			// Repainting the row keeps the table's scroll, selection, and
			// column offset, which a full render resets.
			a.repaintIssueRow(section, updated.ID)
			continue
		}
		deferred[section] = selectedIssueID
	}
	if len(deferred) > 0 {
		a.renderIssueSections(deferred)
	}

	if a.activeIssuesSection != previousSection {
		a.updateIssuesColumnLayout()
	} else {
		a.updateAllPaneTitles()
	}

	a.issuesMu.Lock()
	if a.selectedIssue != nil && a.selectedIssue.ID == updated.ID {
		if issue := a.idToIssue[updated.ID]; issue != nil {
			a.selectedIssue = issue
		}
	}
	a.issuesMu.Unlock()
	a.updateDetailsView()
}

// refreshIssueDetails refetches one issue so the details pane picks up data the
// list query never carries: comments, relations, subscribers, attachments.
// Nothing the list renders changes, so the list is left alone.
func (a *App) refreshIssueDetails(issueID string) {
	fetchIssue := a.fetchIssueByID
	if fetchIssue == nil {
		fetchIssue = a.api.FetchIssueByID
	}
	a.fetchingIssueID = issueID
	go func() {
		fullIssue, err := fetchIssue(context.Background(), issueID)
		a.QueueUpdateDraw(func() {
			// A newer selection owns the details pane.
			if a.fetchingIssueID != issueID {
				return
			}
			if err != nil {
				logger.ErrorWithErr(err, "tui.issue_update: failed to refresh issue details issue_id=%s", issueID)
				return
			}
			a.issuesMu.Lock()
			a.selectedIssue = &fullIssue
			a.issuesMu.Unlock()
			a.updateDetailsView()
		})
	}()
}

// rebuildIssueRowModels rebuilds every section's rows from the issue list.
func (a *App) rebuildIssueRowModels() {
	a.issuesMu.RLock()
	issues := a.issues
	a.issuesMu.RUnlock()

	currentUserID := ""
	if a.currentUser != nil {
		currentUserID = a.currentUser.ID
	}
	myIssues, otherIssues := splitIssuesByAssignee(issues, currentUserID)

	// Build hierarchical tree rows for each section, grouped when enabled.
	a.myIssueRows, a.myIDToIssue = a.buildIssueRowsFor(myIssues)
	a.otherIssueRows, a.otherIDToIssue = a.buildIssueRowsFor(otherIssues)

	// The All Issues tab renders the full list.
	a.issueRows, a.idToIssue = a.buildIssueRowsFor(issues)
}

// sectionSelectionsFor maps each section to the row it should land on, and
// makes the section holding the target issue active. The All and Search tabs
// show their own lists, so they stay active when selected.
func (a *App) sectionSelectionsFor(targetIssueID string) map[IssuesSection]string {
	selections := map[IssuesSection]string{
		IssuesSectionMy:    "",
		IssuesSectionOther: "",
		IssuesSectionAll:   "",
	}
	if targetIssueID == "" {
		return selections
	}
	selections[IssuesSectionAll] = targetIssueID

	sectionPinned := a.activeIssuesSection == IssuesSectionAll || a.activeIssuesSection == IssuesSectionSearch
	if _, ok := a.myIDToIssue[targetIssueID]; ok {
		selections[IssuesSectionMy] = targetIssueID
		if !sectionPinned {
			a.activeIssuesSection = IssuesSectionMy
		}
	} else if _, ok := a.otherIDToIssue[targetIssueID]; ok {
		selections[IssuesSectionOther] = targetIssueID
		if !sectionPinned {
			a.activeIssuesSection = IssuesSectionOther
		}
	}
	return selections
}

// repaintIssueRow rewrites one issue's cells in a section's table.
func (a *App) repaintIssueRow(section IssuesSection, issueID string) {
	if _, stale := a.pendingSectionRenders[section]; stale {
		// The whole table is waiting on a render; one fresh row among stale
		// ones would be worse than leaving it.
		return
	}
	table := a.tableForSection(section)
	if table == nil {
		return
	}
	issue := a.issueMapForSection(section)[issueID]
	if issue == nil {
		return
	}
	rows := a.rowsForSection(section)
	for i, row := range rows {
		if row.IssueID != issueID {
			continue
		}
		setIssueRowCells(table, i+1, row, issue, a.theme, a.issueColumns())
		return
	}
}

// issueRowsEqual reports whether two row models are identical. IssueRow holds
// only comparable fields, so this catches a moved row, a changed group, and a
// changed header count alike.
func issueRowsEqual(previous, current []IssueRow) bool {
	if len(previous) != len(current) {
		return false
	}
	for i := range previous {
		if previous[i] != current[i] {
			return false
		}
	}
	return true
}
