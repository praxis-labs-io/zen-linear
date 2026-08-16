package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/zen-linear/zen-linear/internal/linearapi"
	"github.com/zen-linear/zen-linear/internal/logger"
)

var issueRelationTypeLabels = []PickerItem{
	{ID: "blocking", Label: "blocking"},
	{ID: "blocked by", Label: "blocked by"},
	{ID: "related", Label: "related"},
	{ID: "duplicate", Label: "duplicate"},
	{ID: "similar", Label: "similar"},
}

func validateLinearDate(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("date is required")
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil || parsed.Format("2006-01-02") != value {
		return fmt.Errorf("date must be YYYY-MM-DD")
	}
	return nil
}

func parseEstimateInput(value string) (float64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("estimate is required")
	}
	estimate, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("estimate must be numeric")
	}
	if estimate < 0 {
		return 0, fmt.Errorf("estimate must be non-negative")
	}
	return estimate, nil
}

// runIssueUpdate applies an update to the issue named in input.ID.
func (a *App) runIssueUpdate(input linearapi.UpdateIssueInput, successMessage string) {
	a.runIssueUpdateWithResult(input, successMessage, nil)
}

// runIssueUpdateWithResult is runIssueUpdate plus the outcome. An empty ID is
// refused, never resolved against a selection the caller may have outlived.
func (a *App) runIssueUpdateWithResult(input linearapi.UpdateIssueInput, successMessage string, onDone func(error)) {
	if input.ID == "" {
		logger.Error("tui.planning: issue update with no id, dropped")
		a.flashError("No issue to update")
		if onDone != nil {
			onDone(fmt.Errorf("issue update with no id"))
		}
		return
	}
	updateIssue := a.updateIssueFunc
	if updateIssue == nil {
		updateIssue = a.api.UpdateIssue
	}
	go func(issueID string) {
		updated, err := updateIssue(context.Background(), input)
		a.QueueUpdateDraw(func() {
			if err != nil {
				logger.ErrorWithErr(err, "tui.planning: issue update failed issue_id=%s", issueID)
				a.updateStatusBarWithError(err)
				if onDone != nil {
					onDone(err)
				}
				return
			}
			a.flashSuccess(successMessage)
			a.applyIssueUpdate(updated)
			if onDone != nil {
				onDone(nil)
			}
		})
	}(input.ID)
}

// runIssueDetailAction runs a background issue mutation whose only UI effect is a
// success flash and a details-pane refresh — the async twin of runIssueUpdate for
// calls that return no issue to splice into the list.
func (a *App) runIssueDetailAction(issueID string, action func(context.Context) error, successMessage string) {
	go func() {
		err := action(context.Background())
		a.QueueUpdateDraw(func() {
			if err != nil {
				a.updateStatusBarWithError(err)
				return
			}
			a.flashSuccess(successMessage)
			a.loadIssueDetailsByID(issueID)
		})
	}()
}

func (a *App) showSetDueDateModal() {
	issue := a.GetSelectedIssue()
	if issue == nil {
		a.flashStatus("No issue selected")
		return
	}
	initial := ""
	if issue.DueDate != nil {
		initial = *issue.DueDate
	}
	// The modal names this issue, so the write targets it even if a refresh
	// moves the selection while the modal is up.
	target := *issue
	a.textInputModal.ShowWithContext("Set Due Date", "YYYY-MM-DD: ", initial, a.issueContextLine(target), func(value string) {
		a.setDueDate(target, value)
	})
}

func (a *App) setDueDate(issue linearapi.Issue, value string) {
	save, err := issueFieldDueDateSave(issue, value)
	if err != nil {
		a.updateStatusBarWithError(err)
		return
	}
	a.saveIssueField(save)
}

func (a *App) clearDueDateForSelectedIssue() {
	issue := a.GetSelectedIssue()
	if issue == nil {
		a.flashStatus("No issue selected")
		return
	}
	a.saveIssueField(issueFieldDueDateClear(*issue))
}

func (a *App) showEditEstimateModal() {
	issue := a.GetSelectedIssue()
	if issue == nil {
		a.flashStatus("No issue selected")
		return
	}
	initial := ""
	if issue.Estimate != nil {
		initial = formatEstimate(issue.Estimate)
	}
	// The modal names this issue, so the write targets it even if a refresh
	// moves the selection while the modal is up.
	target := *issue
	a.textInputModal.ShowWithContext("Edit Estimate", "Points: ", initial, a.issueContextLine(target), func(value string) {
		a.setEstimate(target, value)
	})
}

func (a *App) setEstimate(issue linearapi.Issue, value string) {
	save, err := issueFieldEstimateSave(issue, value)
	if err != nil {
		a.updateStatusBarWithError(err)
		return
	}
	a.saveIssueField(save)
}

func (a *App) clearEstimateForSelectedIssue() {
	issue := a.GetSelectedIssue()
	if issue == nil {
		a.flashStatus("No issue selected")
		return
	}
	a.saveIssueField(issueFieldEstimateClear(*issue))
}

func (a *App) showSetPriorityPicker() {
	issue := a.GetSelectedIssue()
	if issue == nil {
		a.flashStatus("No issue selected")
		return
	}
	// The picker names this issue, so the write targets it even if a refresh
	// moves the selection while the picker is open.
	target := *issue
	a.ShowFieldPicker(issueFieldPriority, a.issueOptionScope(target), a.issueContextLine(target), func(item PickerItem) {
		priority, err := strconv.Atoi(item.ID)
		if err != nil {
			a.updateStatusBarWithError(err)
			return
		}
		a.saveIssueField(issueFieldPrioritySave(target, priority))
	})
}

func (a *App) showSetProjectPicker() {
	issue := a.GetSelectedIssue()
	if issue == nil {
		a.flashStatus("No issue selected")
		return
	}
	// The picker names this issue, so the write targets it even if a refresh
	// moves the selection while the picker is open.
	target := *issue
	a.ShowFieldPicker(issueFieldProject, a.issueOptionScope(target), a.issueContextLine(target), func(item PickerItem) {
		if item.ID == target.ProjectID {
			a.flashStatus("Already in that project")
			return
		}
		a.saveIssueField(issueFieldProjectSave(target, item.ID, item.name()))
	})
}

func (a *App) showChangeTeamPicker() {
	issue := a.GetSelectedIssue()
	if issue == nil {
		a.flashStatus("No issue selected")
		return
	}
	// The picker names this issue, so the write targets it even if a refresh
	// moves the selection while the picker is open.
	target := *issue
	a.ShowTeamPicker(a.issueContextLine(target), func(item PickerItem) {
		if item.ID == target.TeamID {
			a.flashStatus("Already in that team")
			return
		}
		name := ""
		if team := findTeamByID(a.navTeams, item.ID); team != nil {
			name = team.Name
		}
		a.saveIssueField(issueFieldTeamSave(target, item.ID, name))
	})
}

func (a *App) clearProjectForSelectedIssue() {
	issue := a.GetSelectedIssue()
	if issue == nil {
		a.flashStatus("No issue selected")
		return
	}
	if strings.TrimSpace(issue.ProjectID) == "" {
		a.flashStatus("Issue has no project")
		return
	}
	a.saveIssueField(issueFieldProjectClear(*issue))
}

// clearMilestoneOnProjectChange nulls the milestone alongside a project change.
// A milestone belongs to one project, so leaving it set would orphan it against
// the new project.
func clearMilestoneOnProjectChange(input *linearapi.UpdateIssueInput, issue linearapi.Issue) {
	if issue.ProjectMilestone == nil {
		return
	}
	empty := ""
	input.ProjectMilestoneID = &empty
}

// The issue is captured before the fetch, not read again in the callback: the
// id would otherwise be read a round trip and a navigated picker later.
func (a *App) showProjectMilestonePicker(onSelect func(linearapi.Issue, PickerItem)) {
	issue := a.GetSelectedIssue()
	if issue == nil {
		a.flashStatus("No issue selected")
		return
	}
	target := *issue
	a.ShowFieldPicker(issueFieldMilestone, a.issueOptionScope(target), a.issueContextLine(target), func(item PickerItem) {
		onSelect(target, item)
	})
}

func (a *App) listProjectMilestonesForSelectedIssue() {
	a.showProjectMilestonePicker(func(_ linearapi.Issue, milestone PickerItem) {
		a.flashStatus(fmt.Sprintf("Milestone: %s", milestone.name()))
	})
}

func (a *App) showSetMilestonePicker() {
	a.showProjectMilestonePicker(func(issue linearapi.Issue, milestone PickerItem) {
		a.saveIssueField(issueFieldMilestoneSave(issue, milestone.ID, milestone.name()))
	})
}

func (a *App) clearMilestoneForSelectedIssue() {
	issue := a.GetSelectedIssue()
	if issue == nil {
		a.flashStatus("No issue selected")
		return
	}
	a.saveIssueField(issueFieldMilestoneClear(*issue))
}

func (a *App) applyFiltersAndRefresh(message string) {
	a.flashStatus(message)
	a.refreshIssues()
}

func (a *App) clearFilters() {
	a.richFilters = IssueFilters{}
	a.applyFiltersAndRefresh("Cleared filters")
}

func (a *App) showFilterIssuesPicker() {
	items := []PickerItem{
		{ID: "assignee", Label: "Assignee"},
		{ID: "labels", Label: "Labels"},
		{ID: "status", Label: "Status"},
		{ID: "project", Label: "Project"},
		{ID: "cycle", Label: "Cycle"},
		{ID: "due", Label: "Due date"},
		{ID: "estimate", Label: "Estimate"},
		{ID: "clear", Label: "Clear filters"},
	}
	a.pickerModal.Show("Filter Issues", items, func(item PickerItem) {
		switch item.ID {
		case "assignee":
			a.showAssigneeFilter()
		case "labels":
			a.showLabelFilter()
		case "status":
			a.showStatusFilter()
		case "project":
			a.showProjectFilter()
		case "cycle":
			a.showCycleFilter()
		case "due":
			a.showDueDateFilter()
		case "estimate":
			a.showEstimateFilter()
		case "clear":
			a.clearFilters()
		}
	})
}

func (a *App) showAssigneeFilter() {
	a.ShowFieldPicker(issueFieldAssignee, a.navOptionScope(), "", func(item PickerItem) {
		a.richFilters.AssigneeID = item.ID
		a.richFilters.AssigneeName = item.name()
		a.applyFiltersAndRefresh("Applied assignee filter")
	})
}

func (a *App) showLabelFilter() {
	teamID := a.GetSelectedTeamID()
	if teamID == "" {
		a.updateStatusBarWithError(fmt.Errorf("team context is required"))
		return
	}
	go func() {
		labels, err := a.cache.GetIssueLabels(context.Background(), teamID)
		a.QueueUpdateDraw(func() {
			if err != nil {
				a.updateStatusBarWithError(err)
				return
			}
			items := make([]MultiSelectItem, 0, len(labels))
			labelByID := make(map[string]string, len(labels))
			for _, label := range labels {
				items = append(items, MultiSelectItem{ID: label.ID, Label: label.Name})
				labelByID[label.ID] = label.Name
			}
			a.multiSelectModal.Show("Filter Labels", items, a.richFilters.LabelIDs, func(ids []string) {
				a.richFilters.LabelIDs = ids
				a.richFilters.LabelNames = namesForIDs(ids, labelByID)
				a.applyFiltersAndRefresh("Applied label filters")
			})
		})
	}()
}

func (a *App) showStatusFilter() {
	a.ShowFieldPicker(issueFieldState, a.navOptionScope(), "", func(item PickerItem) {
		a.richFilters.StateID = item.ID
		a.richFilters.StateName = item.name()
		a.applyFiltersAndRefresh("Applied status filter")
	})
}

func (a *App) showProjectFilter() {
	a.ShowFieldPicker(issueFieldProject, a.navOptionScope(), "", func(item PickerItem) {
		a.richFilters.ProjectID = item.ID
		a.richFilters.ProjectName = item.name()
		a.applyFiltersAndRefresh("Applied project filter")
	})
}

func (a *App) showCycleFilter() {
	a.ShowFieldPicker(issueFieldCycle, a.navOptionScope(), "", func(item PickerItem) {
		a.richFilters.CycleID = item.ID
		a.richFilters.CycleName = item.name()
		a.applyFiltersAndRefresh("Applied cycle filter")
	})
}

func (a *App) showDueDateFilter() {
	initial := formatDateFilterSummary(a.richFilters.DueDate)
	a.textInputModal.Show("Filter Due Date", "YYYY-MM-DD: ", initial, func(value string) {
		if err := validateLinearDate(value); err != nil {
			a.updateStatusBarWithError(err)
			return
		}
		a.richFilters.DueDate = linearapi.DateFilter{Eq: value}
		a.applyFiltersAndRefresh("Applied due date filter")
	})
}

func (a *App) showEstimateFilter() {
	initial := formatNumberFilterSummary(a.richFilters.Estimate)
	a.textInputModal.Show("Filter Estimate", "Points: ", initial, func(value string) {
		estimate, err := parseEstimateInput(value)
		if err != nil {
			a.updateStatusBarWithError(err)
			return
		}
		a.richFilters.Estimate = linearapi.NumberFilter{Eq: &estimate}
		a.applyFiltersAndRefresh("Applied estimate filter")
	})
}

func namesForIDs(ids []string, names map[string]string) []string {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if name := names[id]; name != "" {
			result = append(result, name)
			continue
		}
		result = append(result, id)
	}
	return result
}

func relationInputForIssue(issueID, label, targetIssueID string) (linearapi.CreateIssueRelationInput, error) {
	label = strings.ToLower(strings.TrimSpace(label))
	targetIssueID = strings.TrimSpace(targetIssueID)
	if issueID == "" {
		return linearapi.CreateIssueRelationInput{}, fmt.Errorf("no issue selected")
	}
	if targetIssueID == "" {
		return linearapi.CreateIssueRelationInput{}, fmt.Errorf("related issue ID is required")
	}
	switch label {
	case "blocking":
		return linearapi.CreateIssueRelationInput{IssueID: issueID, RelatedIssueID: targetIssueID, Type: linearapi.IssueRelationBlocks}, nil
	case "blocked by":
		return linearapi.CreateIssueRelationInput{IssueID: targetIssueID, RelatedIssueID: issueID, Type: linearapi.IssueRelationBlocks}, nil
	case "related":
		return linearapi.CreateIssueRelationInput{IssueID: issueID, RelatedIssueID: targetIssueID, Type: linearapi.IssueRelationRelated}, nil
	case "duplicate":
		return linearapi.CreateIssueRelationInput{IssueID: issueID, RelatedIssueID: targetIssueID, Type: linearapi.IssueRelationDuplicate}, nil
	case "similar":
		return linearapi.CreateIssueRelationInput{IssueID: issueID, RelatedIssueID: targetIssueID, Type: linearapi.IssueRelationSimilar}, nil
	default:
		return linearapi.CreateIssueRelationInput{}, fmt.Errorf("unsupported relation type %q", label)
	}
}

func (a *App) showAddIssueRelationPicker() {
	issue := a.GetSelectedIssue()
	if issue == nil {
		a.flashStatus("No issue selected")
		return
	}
	contextLine := a.issueContextLine(*issue)
	a.pickerModal.ShowWithContext("Relation Type", contextLine, issueRelationTypeLabels, func(item PickerItem) {
		a.textInputModal.ShowWithContext("Related Issue", "Issue ID: ", "", contextLine, func(targetIssueID string) {
			a.createIssueRelationForSelectedIssue(item.ID, targetIssueID)
		})
	})
}

func (a *App) createIssueRelationForSelectedIssue(label, targetIssueID string) {
	issue := a.GetSelectedIssue()
	if issue == nil {
		a.flashStatus("No issue selected")
		return
	}
	input, err := relationInputForIssue(issue.ID, label, targetIssueID)
	if err != nil {
		a.updateStatusBarWithError(err)
		return
	}
	createRelation := a.createIssueRelationFunc
	if createRelation == nil {
		createRelation = a.api.CreateIssueRelation
	}
	a.runIssueDetailAction(issue.ID, func(ctx context.Context) error {
		_, err := createRelation(ctx, input)
		return err
	}, "Added issue relation")
}

func (a *App) showRemoveIssueRelationPicker() {
	issue := a.GetSelectedIssue()
	if issue == nil {
		a.flashStatus("No issue selected")
		return
	}
	if len(issue.Relations) == 0 {
		a.flashStatus("No issue relations")
		return
	}
	items := make([]PickerItem, 0, len(issue.Relations))
	for _, relation := range issue.Relations {
		ref := relation.RelatedIssue
		if relation.Inverse {
			ref = relation.Issue
		}
		items = append(items, PickerItem{
			ID:    relation.ID,
			Label: relation.DisplayType() + " " + formatIssueReference(ref),
		})
	}
	a.pickerModal.ShowWithContext("Remove Relation", a.issueContextLine(*issue), items, func(item PickerItem) {
		a.deleteIssueRelationForSelectedIssue(item.ID)
	})
}

func (a *App) deleteIssueRelationForSelectedIssue(relationID string) {
	issue := a.GetSelectedIssue()
	if issue == nil {
		a.flashStatus("No issue selected")
		return
	}
	deleteRelation := a.deleteIssueRelationFunc
	if deleteRelation == nil {
		deleteRelation = a.api.DeleteIssueRelation
	}
	a.runIssueDetailAction(issue.ID, func(ctx context.Context) error {
		return deleteRelation(ctx, relationID)
	}, "Removed issue relation")
}

func (a *App) subscribeSelectedIssue() {
	issue := a.GetSelectedIssue()
	if issue == nil {
		a.flashStatus("No issue selected")
		return
	}
	subscribe := a.subscribeIssueFunc
	if subscribe == nil {
		subscribe = a.api.SubscribeToIssue
	}
	a.runIssueDetailAction(issue.ID, func(ctx context.Context) error {
		_, err := subscribe(ctx, issue.ID)
		return err
	}, "Subscribed to issue")
}

func (a *App) unsubscribeSelectedIssue() {
	issue := a.GetSelectedIssue()
	if issue == nil {
		a.flashStatus("No issue selected")
		return
	}
	unsubscribe := a.unsubscribeIssueFunc
	if unsubscribe == nil {
		unsubscribe = a.api.UnsubscribeFromIssue
	}
	a.runIssueDetailAction(issue.ID, func(ctx context.Context) error {
		_, err := unsubscribe(ctx, issue.ID)
		return err
	}, "Unsubscribed from issue")
}

func (a *App) openSelectedAttachment() {
	a.runAttachmentAction("Open Attachment", func(attachment linearapi.Attachment) {
		openFn := a.openURLFunc
		if openFn == nil {
			openFn = openURL
		}
		if err := openFn(attachment.URL); err != nil {
			a.updateStatusBarWithError(err)
			return
		}
		a.flashSuccess("Opened attachment")
	})
}

func (a *App) copySelectedAttachmentURL() {
	a.runAttachmentAction("Copy Attachment URL", func(attachment linearapi.Attachment) {
		copyFn := a.copyToClipboardFunc
		if copyFn == nil {
			copyFn = copyToClipboard
		}
		if err := copyFn(attachment.URL); err != nil {
			a.updateStatusBarWithError(err)
			return
		}
		a.flashSuccess("Copied attachment URL")
	})
}

func (a *App) runAttachmentAction(title string, action func(linearapi.Attachment)) {
	issue := a.GetSelectedIssue()
	if issue == nil {
		a.flashStatus("No issue selected")
		return
	}
	attachments := issue.Attachments
	if len(attachments) == 0 {
		a.flashStatus("No attachments")
		return
	}
	if len(attachments) == 1 {
		action(attachments[0])
		return
	}
	byID := make(map[string]linearapi.Attachment, len(attachments))
	items := make([]PickerItem, 0, len(attachments))
	for _, attachment := range attachments {
		label := attachment.Title
		if label == "" {
			label = attachment.URL
		}
		if attachment.SourceType != "" {
			label += " (" + attachment.SourceType + ")"
		}
		byID[attachment.ID] = attachment
		items = append(items, PickerItem{ID: attachment.ID, Label: label})
	}
	a.pickerModal.ShowWithContext(title, a.issueContextLine(*issue), items, func(item PickerItem) {
		action(byID[item.ID])
	})
}
