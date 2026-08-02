package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/roeyazroel/linear-tui/internal/linearapi"
	"github.com/roeyazroel/linear-tui/internal/logger"
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

// runIssueUpdate applies an update to the issue named in input.ID, falling
// back to the current selection. Callers that open a modal first should set
// the ID when they open it: a background refresh can move the selection while
// the modal is up, and the write must land on the issue the modal named.
func (a *App) runIssueUpdate(input linearapi.UpdateIssueInput, successMessage string) {
	if input.ID == "" {
		issue := a.GetSelectedIssue()
		if issue == nil {
			a.flashStatus("No issue selected")
			return
		}
		input.ID = issue.ID
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
				return
			}
			a.flashStatus(successMessage)
			a.applyIssueUpdate(updated)
		})
	}(input.ID)
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
	a.textInputModal.ShowWithContext("Set Due Date", "YYYY-MM-DD: ", initial, a.issueContextLine(*issue), func(value string) {
		if err := validateLinearDate(value); err != nil {
			a.updateStatusBarWithError(err)
			return
		}
		a.setDueDateForSelectedIssue(value)
	})
}

func (a *App) setDueDateForSelectedIssue(value string) {
	value = strings.TrimSpace(value)
	if err := validateLinearDate(value); err != nil {
		a.updateStatusBarWithError(err)
		return
	}
	a.runIssueUpdate(linearapi.UpdateIssueInput{DueDate: &value}, "Updated due date")
}

func (a *App) clearDueDateForSelectedIssue() {
	empty := ""
	a.runIssueUpdate(linearapi.UpdateIssueInput{DueDate: &empty}, "Cleared due date")
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
	a.textInputModal.ShowWithContext("Edit Estimate", "Points: ", initial, a.issueContextLine(*issue), func(value string) {
		a.editEstimateForSelectedIssue(value)
	})
}

func (a *App) editEstimateForSelectedIssue(value string) {
	estimate, err := parseEstimateInput(value)
	if err != nil {
		a.updateStatusBarWithError(err)
		return
	}
	a.runIssueUpdate(linearapi.UpdateIssueInput{Estimate: &estimate}, "Updated estimate")
}

func (a *App) clearEstimateForSelectedIssue() {
	a.runIssueUpdate(linearapi.UpdateIssueInput{ClearEstimate: true}, "Cleared estimate")
}

func (a *App) showSetPriorityPicker() {
	issue := a.GetSelectedIssue()
	if issue == nil {
		a.flashStatus("No issue selected")
		return
	}
	items := make([]PickerItem, 0, len(priorityLabels))
	for value, label := range priorityLabels {
		items = append(items, PickerItem{ID: strconv.Itoa(value), Label: label})
	}
	// The picker names this issue, so the write targets it even if a refresh
	// moves the selection while the picker is open.
	issueID := issue.ID
	a.pickerActive = true
	a.pickerModal.ShowWithContext("Set Priority", a.issueContextLine(*issue), items, func(item PickerItem) {
		a.pickerActive = false
		priority, err := strconv.Atoi(item.ID)
		if err != nil {
			a.updateStatusBarWithError(err)
			return
		}
		a.runIssueUpdate(linearapi.UpdateIssueInput{ID: issueID, Priority: &priority}, "Set priority: "+item.Label)
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
	a.ShowProjectPicker(a.issueContextLine(target), func(projectID string) {
		if projectID == target.ProjectID {
			a.flashStatus("Already in that project")
			return
		}
		input := linearapi.UpdateIssueInput{ID: target.ID, ProjectID: &projectID}
		clearMilestoneOnProjectChange(&input, target)
		a.runIssueUpdate(input, "Set project: "+projectNameByID(a.teamProjects, projectID))
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
	empty := ""
	input := linearapi.UpdateIssueInput{ID: issue.ID, ProjectID: &empty}
	clearMilestoneOnProjectChange(&input, *issue)
	a.runIssueUpdate(input, "Cleared project")
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

func (a *App) selectedIssueProjectID() (string, bool) {
	issue := a.GetSelectedIssue()
	if issue == nil {
		a.flashStatus("No issue selected")
		return "", false
	}
	if strings.TrimSpace(issue.ProjectID) == "" {
		a.updateStatusBarWithError(fmt.Errorf("issue must have a project"))
		return "", false
	}
	return issue.ProjectID, true
}

func (a *App) showProjectMilestonePicker(title string, onSelect func(linearapi.ProjectMilestone)) {
	projectID, ok := a.selectedIssueProjectID()
	if !ok {
		return
	}
	contextLine := ""
	if issue := a.GetSelectedIssue(); issue != nil {
		contextLine = a.issueContextLine(*issue)
	}
	go func() {
		milestones, err := a.cache.GetProjectMilestones(context.Background(), projectID)
		a.QueueUpdateDraw(func() {
			if err != nil {
				a.updateStatusBarWithError(err)
				return
			}
			if len(milestones) == 0 {
				a.flashStatus("No project milestones available")
				return
			}
			items := make([]PickerItem, 0, len(milestones))
			byID := make(map[string]linearapi.ProjectMilestone, len(milestones))
			for _, milestone := range milestones {
				label := milestone.Name
				if milestone.TargetDate != nil && *milestone.TargetDate != "" {
					label += " (" + *milestone.TargetDate + ")"
				}
				items = append(items, PickerItem{ID: milestone.ID, Label: label})
				byID[milestone.ID] = milestone
			}
			a.pickerActive = true
			a.pickerModal.ShowWithContext(title, contextLine, items, func(item PickerItem) {
				a.pickerActive = false
				if onSelect != nil {
					onSelect(byID[item.ID])
				}
			})
		})
	}()
}

func (a *App) listProjectMilestonesForSelectedIssue() {
	a.showProjectMilestonePicker("Project Milestones", func(milestone linearapi.ProjectMilestone) {
		a.flashStatus(fmt.Sprintf("Milestone: %s", milestone.Name))
	})
}

func (a *App) showSetMilestonePicker() {
	a.showProjectMilestonePicker("Set Milestone", func(milestone linearapi.ProjectMilestone) {
		milestoneID := milestone.ID
		a.runIssueUpdate(linearapi.UpdateIssueInput{ProjectMilestoneID: &milestoneID}, "Set milestone")
	})
}

func (a *App) clearMilestoneForSelectedIssue() {
	empty := ""
	a.runIssueUpdate(linearapi.UpdateIssueInput{ProjectMilestoneID: &empty}, "Cleared milestone")
}

func (a *App) applyFiltersAndRefresh(message string) {
	a.flashStatus(message)
	go a.refreshIssues()
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
	a.pickerActive = true
	a.pickerModal.Show("Filter Issues", items, func(item PickerItem) {
		a.pickerActive = false
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
	a.ShowUserPicker("", func(userID string) {
		a.richFilters.AssigneeID = userID
		a.richFilters.AssigneeName = userDisplayNameByID(a.teamUsers, userID)
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
	a.ShowStatusPicker("", func(stateID string) {
		a.richFilters.StateID = stateID
		a.richFilters.StateName = workflowStateNameByID(a.workflowStates, stateID)
		a.applyFiltersAndRefresh("Applied status filter")
	})
}

func (a *App) showProjectFilter() {
	// ShowProjectPicker only logs on a missing team; the filter's failure has
	// to be visible.
	if a.GetSelectedTeamID() == "" {
		a.updateStatusBarWithError(fmt.Errorf("team context is required"))
		return
	}
	a.ShowProjectPicker("", func(projectID string) {
		a.richFilters.ProjectID = projectID
		a.richFilters.ProjectName = projectNameByID(a.teamProjects, projectID)
		a.applyFiltersAndRefresh("Applied project filter")
	})
}

func (a *App) showCycleFilter() {
	a.ShowCyclePicker("", func(cycleID string) {
		a.richFilters.CycleID = cycleID
		a.richFilters.CycleName = cycleNameByID(a.teamCycles, cycleID)
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

func userDisplayNameByID(users []linearapi.User, id string) string {
	for _, user := range users {
		if user.ID == id {
			return formatUserDisplayName(user)
		}
	}
	return id
}

func workflowStateNameByID(states []linearapi.WorkflowState, id string) string {
	for _, state := range states {
		if state.ID == id {
			return state.Name
		}
	}
	return id
}

func projectNameByID(projects []linearapi.Project, id string) string {
	for _, project := range projects {
		if project.ID == id {
			return project.Name
		}
	}
	return id
}

func cycleNameByID(cycles []linearapi.Cycle, id string) string {
	for _, cycle := range cycles {
		if cycle.ID == id {
			return cycle.DisplayName()
		}
	}
	return id
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
	a.pickerActive = true
	contextLine := a.issueContextLine(*issue)
	a.pickerModal.ShowWithContext("Relation Type", contextLine, issueRelationTypeLabels, func(item PickerItem) {
		a.pickerActive = false
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
	go func(issueID string) {
		_, err := createRelation(context.Background(), input)
		a.QueueUpdateDraw(func() {
			if err != nil {
				a.updateStatusBarWithError(err)
				return
			}
			a.flashStatus("Added issue relation")
			a.refreshIssueDetails(issueID)
		})
	}(issue.ID)
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
	a.pickerActive = true
	a.pickerModal.ShowWithContext("Remove Relation", a.issueContextLine(*issue), items, func(item PickerItem) {
		a.pickerActive = false
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
	go func(issueID string) {
		err := deleteRelation(context.Background(), relationID)
		a.QueueUpdateDraw(func() {
			if err != nil {
				a.updateStatusBarWithError(err)
				return
			}
			a.flashStatus("Removed issue relation")
			a.refreshIssueDetails(issueID)
		})
	}(issue.ID)
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
	go func(issueID string) {
		_, err := subscribe(context.Background(), issueID)
		a.QueueUpdateDraw(func() {
			if err != nil {
				a.updateStatusBarWithError(err)
				return
			}
			a.flashStatus("Subscribed to issue")
			a.refreshIssueDetails(issueID)
		})
	}(issue.ID)
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
	go func(issueID string) {
		_, err := unsubscribe(context.Background(), issueID)
		a.QueueUpdateDraw(func() {
			if err != nil {
				a.updateStatusBarWithError(err)
				return
			}
			a.flashStatus("Unsubscribed from issue")
			a.refreshIssueDetails(issueID)
		})
	}(issue.ID)
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
		a.flashStatus("Opened attachment")
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
		a.flashStatus("Copied attachment URL")
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
	a.pickerActive = true
	a.pickerModal.ShowWithContext(title, a.issueContextLine(*issue), items, func(item PickerItem) {
		a.pickerActive = false
		action(byID[item.ID])
	})
}
