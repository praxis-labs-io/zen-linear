package tui

import (
	"context"
	"slices"

	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
)

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
		a.applyDetachedIssueEdit(updated)
		a.confirmIssueInScope(updated, false)
		return
	}
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
	a.confirmIssueInScope(updated, true)
}

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

func (a *App) updateSearchIssueRow(updated linearapi.Issue) {
	for i := range a.searchIssues {
		if a.searchIssues[i].ID != updated.ID {
			continue
		}
		a.searchIssues[i] = updated
		a.repaintIssueRow(IssuesSectionSearch, updated.ID)
		return
	}
}

func (a *App) applyIssueInsert(created linearapi.Issue) {
	if a.insertIssue(created) {
		a.confirmIssueInScope(created, true)
	}
}

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

func (a *App) confirmIssueInScope(issue linearapi.Issue, inList bool) {
	if issue.ID == "" {
		return
	}
	params := a.currentFetchParams(string(a.sortFields[0]))
	if !issueScopeIsNarrowed(params) {
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

func (a *App) applyIssueRemoval(issueID string) {
	if issueID == "" {
		return
	}
	successor := a.issueRowAfter(a.activeIssuesSection, issueID)

	a.issuesMu.Lock()
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
	a.renderIssueChange(selectedID, false)
}

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

func (a *App) renderIssueChange(targetIssueID string, selectTarget bool) {
	previousRows := map[IssuesSection][]IssueRow{
		IssuesSectionList: a.listIssueRows,
	}
	a.rebuildIssueRowModels()
	selections := a.sectionSelectionsFor(targetIssueID)

	deferred := make(map[IssuesSection]string, len(selections))
	for section, selectedIssueID := range selections {
		if issueRowsEqual(previousRows[section], a.rowsForSection(section)) {
			a.repaintIssueRow(section, targetIssueID)
			continue
		}
		deferred[section] = selectedIssueID
	}
	if len(deferred) > 0 {
		a.renderIssueSections(deferred)
	}

	a.updateAllPaneTitles()

	a.repointSelection(targetIssueID, selectTarget)
}

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
		a.setSelectedIssue(*target)
		a.updateDetailsView()
	case selectTarget && target != nil:
		a.selectIssueNow(*target)
	case selectTarget:
		a.clearSelectedIssue()
	default:
		a.updateDetailsView()
	}
}

// Indexes a clone: pagination re-sorts a.issues in place, and the id map would point at whatever moved into a slot.
func (a *App) rebuildIssueRowModels() {
	a.issuesMu.RLock()
	issues := slices.Clone(a.issues)
	a.issuesMu.RUnlock()

	a.listIssueRows, a.listIDToIssue = a.buildIssueRowsFor(issues)
}

func (a *App) sectionSelectionsFor(targetIssueID string) map[IssuesSection]string {
	return map[IssuesSection]string{IssuesSectionList: targetIssueID}
}

func (a *App) repaintIssueRow(section IssuesSection, issueID string) {
	if _, stale := a.pendingSectionRenders[section]; stale {
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
