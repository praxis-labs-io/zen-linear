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
	a.moveChildRef(existing.Parent, updated.Parent, updated)
	a.sortIssuesLocally()
	a.issuesMu.Unlock()

	a.renderIssueChange(updated.ID, false)
}

// applyIssueInsert adds a newly created issue to the local list. CreateIssue
// returns the same selection the list query uses, so the row can be rendered
// without a fetch. An issue the active filter would have excluded stays
// visible until the next refresh, which is the cost of not asking the server.
func (a *App) applyIssueInsert(created linearapi.Issue) {
	if created.ID == "" {
		return
	}

	a.issuesMu.Lock()
	for i := range a.issues {
		if a.issues[i].ID == created.ID {
			a.issuesMu.Unlock()
			a.applyIssueUpdate(created)
			return
		}
	}
	a.issues = append(a.issues, created)
	a.moveChildRef(nil, created.Parent, created)
	a.sortIssuesLocally()
	a.issuesMu.Unlock()

	a.renderIssueChange(created.ID, true)
}

// applyIssueRemoval drops an archived issue from the local list, landing the
// cursor on the row that followed it. Sub-issues left behind render as
// top-level rows, which is how indexIssues already treats an orphan.
func (a *App) applyIssueRemoval(issueID string) {
	if issueID == "" {
		return
	}
	successor := a.issueRowAfter(a.effectiveIssuesSection(), issueID)

	a.issuesMu.Lock()
	index := -1
	for i := range a.issues {
		if a.issues[i].ID == issueID {
			index = i
			break
		}
	}
	if index < 0 {
		a.issuesMu.Unlock()
		return
	}
	removed := a.issues[index]
	a.issues = append(a.issues[:index], a.issues[index+1:]...)
	a.moveChildRef(removed.Parent, nil, removed)
	if a.selectedIssue != nil && a.selectedIssue.ID == issueID {
		a.selectedIssue = nil
	}
	a.issuesMu.Unlock()

	a.renderIssueChange(successor, true)
}

// issueRowAfter returns the id of the issue row following the given one, so a
// removal can keep the cursor where it was instead of jumping to the top.
func (a *App) issueRowAfter(section IssuesSection, issueID string) string {
	rows := a.rowsForSection(section)
	for i, row := range rows {
		if row.IssueID != issueID {
			continue
		}
		if next := nextIssueRow(rows, i+1, 1); next > 0 {
			return rows[next-1].IssueID
		}
		if previous := nextIssueRow(rows, i+1, -1); previous > 0 {
			return rows[previous-1].IssueID
		}
		return ""
	}
	return ""
}

// moveChildRef keeps the Children slices of the old and new parent in step
// with a child's own Parent field. Nothing else reports it: the mutation
// answers for the child alone, and a parent whose Children still lists a
// departed child renders an expand arrow over nothing.
// Callers must hold issuesMu.
func (a *App) moveChildRef(oldParent, newParent *linearapi.IssueRef, child linearapi.Issue) {
	oldID, newID := "", ""
	if oldParent != nil {
		oldID = oldParent.ID
	}
	if newParent != nil {
		newID = newParent.ID
	}
	if oldID == newID {
		return
	}

	for i := range a.issues {
		switch a.issues[i].ID {
		case oldID:
			for j := range a.issues[i].Children {
				if a.issues[i].Children[j].ID == child.ID {
					a.issues[i].Children = append(a.issues[i].Children[:j], a.issues[i].Children[j+1:]...)
					break
				}
			}
		case newID:
			a.issues[i].Children = append(a.issues[i].Children, linearapi.IssueChildRef{
				ID:         child.ID,
				Identifier: child.Identifier,
				Title:      child.Title,
				State:      child.State,
				StateID:    child.StateID,
			})
		}
	}
}

// renderIssueChange repaints the list after the issue slice changed, painting
// the narrowest thing that reflects it. selectTarget moves the details pane to
// targetIssueID; an edit leaves it false so changing a row the user is not
// looking at does not pull the pane away from the row they are.
func (a *App) renderIssueChange(targetIssueID string, selectTarget bool) {
	previousRows := map[IssuesSection][]IssueRow{
		IssuesSectionMy:    a.myIssueRows,
		IssuesSectionOther: a.otherIssueRows,
		IssuesSectionAll:   a.issueRows,
	}
	previousSection := a.activeIssuesSection

	a.rebuildIssueRowModels()
	selections := a.sectionSelectionsFor(targetIssueID)

	deferred := make(map[IssuesSection]string, len(selections))
	for section, selectedIssueID := range selections {
		if issueRowsEqual(previousRows[section], a.rowsForSection(section)) {
			// Nothing moved, so only this issue's own cells can have changed.
			// Repainting the row keeps the table's scroll, selection, and
			// column offset, which a full render resets.
			a.repaintIssueRow(section, targetIssueID)
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
	if selectTarget || (a.selectedIssue != nil && a.selectedIssue.ID == targetIssueID) {
		if issue := a.idToIssue[targetIssueID]; issue != nil {
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
