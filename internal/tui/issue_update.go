package tui

import (
	"context"
	"slices"

	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
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
		// A search result outside the loaded pages still owns the details
		// pane and its row; without this the mutation lands and nothing on
		// screen changes.
		a.applyDetachedIssueEdit(updated)
		// The edit may have brought the issue into scope. Ask about that one
		// issue rather than refetching every page to find out.
		a.confirmIssueInScope(updated, false)
		return
	}
	// The mutation selection carries no comments, relations, subscribers, or
	// attachments. Only the details pane's full fetch has them, and it lives
	// in selectedIssue, never in the list entry.
	existing := a.issues[index]
	detail := &existing
	if a.selectedIssue != nil && a.selectedIssue.ID == updated.ID {
		detail = a.selectedIssue
	}
	updated.Comments = detail.Comments
	updated.Relations = detail.Relations
	updated.Subscribers = detail.Subscribers
	updated.Attachments = detail.Attachments
	a.issues[index] = updated
	a.moveChildRef(existing.Parent, updated.Parent, updated)
	a.sortIssuesLocally()
	a.issuesMu.Unlock()

	a.renderIssueChange(updated.ID, false)
	a.updateSearchIssueRow(updated)
	// The edit itself can push the issue out of the active filter, which the
	// refetch this replaced used to handle by simply not returning it.
	a.confirmIssueInScope(updated, true)
}

// applyDetachedIssueEdit reflects an edit to an issue that is not in the main
// list: the details pane if it is the selection, and its search-tab row.
func (a *App) applyDetachedIssueEdit(updated linearapi.Issue) {
	a.issuesMu.Lock()
	if a.selectedIssue != nil && a.selectedIssue.ID == updated.ID {
		selected := updated
		selected.Comments = a.selectedIssue.Comments
		selected.Activity = a.selectedIssue.Activity
		selected.Relations = a.selectedIssue.Relations
		selected.Subscribers = a.selectedIssue.Subscribers
		selected.Attachments = a.selectedIssue.Attachments
		a.selectedIssue = &selected
		a.issuesMu.Unlock()
		a.updateDetailsView()
	} else {
		a.issuesMu.Unlock()
	}
	a.updateSearchIssueRow(updated)
}

// updateSearchIssueRow keeps an edited issue's search-tab row in step. Search
// results carry their own model, so the main-list render never touches them.
func (a *App) updateSearchIssueRow(updated linearapi.Issue) {
	for i := range a.searchIssues {
		if a.searchIssues[i].ID != updated.ID {
			continue
		}
		// searchIDToIssue points into this slice, so the write is visible to
		// the repaint.
		a.searchIssues[i] = updated
		a.repaintIssueRow(IssuesSectionSearch, updated.ID)
		return
	}
}

// applyIssueInsert adds a newly created issue to the local list. CreateIssue
// returns the same selection the list query uses, so the row renders without a
// fetch. Whether it belongs under the active filter is the server's to answer,
// so the row goes up immediately and confirmIssueInScope takes it back down if
// the filter excludes it.
func (a *App) applyIssueInsert(created linearapi.Issue) {
	if a.insertIssue(created) {
		a.confirmIssueInScope(created, true)
	}
}

// insertIssue splices a new issue into the list, reporting whether it landed.
// An id already present is an update instead.
func (a *App) insertIssue(created linearapi.Issue) bool {
	if created.ID == "" {
		return false
	}

	a.issuesMu.Lock()
	for i := range a.issues {
		if a.issues[i].ID == created.ID {
			a.issuesMu.Unlock()
			a.applyIssueUpdate(created)
			return false
		}
	}
	a.issues = append(a.issues, created)
	a.moveChildRef(nil, created.Parent, created)
	a.sortIssuesLocally()
	a.issuesMu.Unlock()

	a.renderIssueChange(created.ID, true)
	return true
}

// confirmIssueInScope asks whether one issue belongs in the list as currently
// filtered, and reconciles the row. inList says whether the row is showing:
// a row that does not belong comes out, one that does belong goes in.
//
// A failed check leaves the list alone. Guessing wrong in the direction of
// removing a row the user is looking at is worse than showing one row too many
// until the next refresh.
func (a *App) confirmIssueInScope(issue linearapi.Issue, inList bool) {
	if issue.ID == "" {
		return
	}
	params := a.currentFetchParams(string(a.sortFields[0]))
	if !issueScopeIsNarrowed(params) {
		// Nothing to fall out of.
		return
	}
	matches := a.issueMatchesScopeFunc
	if matches == nil {
		matches = a.api.IssueMatchesScope
	}
	generation := a.refreshGeneration.Load()

	go func() {
		inScope, err := matches(context.Background(), params, issue.ID)
		if err != nil {
			logger.ErrorWithErr(err, "tui.issue_update: scope check failed issue_id=%s", issue.ID)
			return
		}
		if inScope == inList {
			return
		}
		logger.Debug("tui.issue_update: scope check moved issue_id=%s in_scope=%v", issue.ID, inScope)
		a.QueueUpdateDraw(func() {
			// A refresh or navigation change re-scoped the list while the
			// answer was in flight; the verdict describes a list that is no
			// longer showing.
			if a.refreshGeneration.Load() != generation {
				return
			}
			if inScope {
				a.insertIssue(issue)
				return
			}
			a.applyIssueRemoval(issue.ID)
		})
	}()
}

// issueScopeIsNarrowed reports whether the params exclude anything. On the All
// Issues view they do not, so there is nothing to check and no reason to spend
// a round trip on every edit.
func issueScopeIsNarrowed(params linearapi.FetchIssuesParams) bool {
	return params.CustomViewID != "" ||
		params.TeamID != "" ||
		params.ProjectID != "" ||
		params.StateID != "" ||
		params.StateType != "" ||
		params.CycleID != "" ||
		params.AssigneeID != "" ||
		params.ProjectMilestoneID != "" ||
		len(params.LabelIDs) > 0 ||
		!params.DueDate.Empty() ||
		!params.Estimate.Empty()
}

// applyIssueRemoval drops an archived issue from the local list, landing the
// cursor on the row that followed it. Sub-issues left behind render as
// top-level rows, which is how indexIssues already treats an orphan.
func (a *App) applyIssueRemoval(issueID string) {
	if issueID == "" {
		return
	}
	successor := a.issueRowAfter(a.activeIssuesSection, issueID)

	a.issuesMu.Lock()
	// Read the selection before the splice: selectedIssue can alias the
	// backing array, and the shift below changes what that pointer reads.
	selectedID := ""
	if a.selectedIssue != nil {
		selectedID = a.selectedIssue.ID
	}
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
	wasSelected := selectedID == issueID
	if wasSelected {
		a.selectedIssue = nil
	}
	a.issuesMu.Unlock()

	if wasSelected {
		a.renderIssueChange(successor, true)
		return
	}
	// The user is on another row, possibly moved there while a scope check
	// was in flight; a removal elsewhere has no claim on their cursor.
	a.renderIssueChange(selectedID, false)
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
		IssuesSectionList: a.listIssueRows,
	}
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

	// What is on screen cannot change here, so only the title needs redrawing:
	// its count moves even when the mounted table does not.
	a.updateAllPaneTitles()

	a.repointSelection(targetIssueID, selectTarget)
}

// repointSelection re-resolves the details pane after the row models changed.
// The id maps point into a snapshot of the list that the next rebuild replaces,
// so the selection always keeps its own copy.
func (a *App) repointSelection(targetIssueID string, selectTarget bool) {
	a.issuesMu.RLock()
	previousSelected := a.selectedIssue
	a.issuesMu.RUnlock()
	selectedID := ""
	if previousSelected != nil {
		selectedID = previousSelected.ID
	}

	target := a.listIDToIssue[targetIssueID]
	switch {
	case target != nil && selectedID == targetIssueID:
		// Same issue: take the fresh list fields, keep the detail data only
		// the pane's full fetch carries.
		a.setSelectedIssue(*target)
		a.updateDetailsView()
	case selectTarget && target != nil:
		// The selection moves to a different issue; render and fetch its
		// details. The list moved under the user, not the cursor, so there is
		// nothing to debounce.
		a.selectIssueNow(*target)
	case selectTarget:
		a.clearSelectedIssue()
	default:
		a.updateDetailsView()
	}
}

// rebuildIssueRowModels rebuilds the list's rows from the issue slice. Search
// results are not in here: they come from their own query and their own model.
func (a *App) rebuildIssueRowModels() {
	a.issuesMu.RLock()
	// The id map below points into whatever slice it indexes, and pagination
	// re-sorts a.issues in place between paints. Index a snapshot, or a lookup
	// inside that window returns whichever issue now sits in the slot.
	issues := slices.Clone(a.issues)
	a.issuesMu.RUnlock()

	// Build hierarchical tree rows, grouped when enabled.
	a.listIssueRows, a.listIDToIssue = a.buildIssueRowsFor(issues)
}

// sectionSelectionsFor maps each section to the row it should land on. The list
// holds every fetched issue, so the target is always in it. What is on screen
// is the user's choice and stays put either way.
func (a *App) sectionSelectionsFor(targetIssueID string) map[IssuesSection]string {
	return map[IssuesSection]string{IssuesSectionList: targetIssueID}
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
