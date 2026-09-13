package tui

import (
	"fmt"
	"strings"

	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
)

type issueField string

const (
	issueFieldTitle       issueField = "title"
	issueFieldDescription issueField = "description"
	issueFieldState       issueField = "state"
	issueFieldAssignee    issueField = "assignee"
	issueFieldPriority    issueField = "priority"
	issueFieldLabels      issueField = "labels"
	issueFieldProject     issueField = "project"
	issueFieldMilestone   issueField = "milestone"
	issueFieldCycle       issueField = "cycle"
	issueFieldDueDate     issueField = "dueDate"
	issueFieldEstimate    issueField = "estimate"
	issueFieldTeam        issueField = "team"
	issueFieldParent      issueField = "parent"
)

var issueFieldNames = map[issueField]string{
	issueFieldTitle:       "title",
	issueFieldDescription: "description",
	issueFieldState:       "status",
	issueFieldAssignee:    "assignee",
	issueFieldPriority:    "priority",
	issueFieldLabels:      "labels",
	issueFieldProject:     "project",
	issueFieldMilestone:   "milestone",
	issueFieldCycle:       "cycle",
	issueFieldDueDate:     "due date",
	issueFieldEstimate:    "estimate",
	issueFieldTeam:        "team",
	issueFieldParent:      "parent",
}

type issueFieldSave struct {
	issueID string
	message string
	apply   func(*linearapi.UpdateIssueInput)
}

func (a *App) saveIssueField(save issueFieldSave) {
	a.saveIssueFieldWithResult(save, nil)
}

func (a *App) saveIssueFieldWithResult(save issueFieldSave, onDone func(error)) {
	if save.issueID == "" {
		a.flashStatus("No issue selected")
		if onDone != nil {
			onDone(fmt.Errorf("no issue selected"))
		}
		return
	}
	input := linearapi.UpdateIssueInput{ID: save.issueID}
	save.apply(&input)
	a.runIssueUpdateWithResult(input, save.message, onDone)
}

func fieldSetMessage(field issueField, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fieldUpdateMessage(field)
	}
	return fmt.Sprintf("Set %s: %s", issueFieldNames[field], value)
}

func fieldClearMessage(field issueField) string {
	return fmt.Sprintf("Cleared %s", issueFieldNames[field])
}

func fieldUpdateMessage(field issueField) string {
	return fmt.Sprintf("Updated %s", issueFieldNames[field])
}

func issueFieldTitleSave(issue linearapi.Issue, text string) (issueFieldSave, error) {
	title := strings.TrimSpace(text)
	if title == "" {
		return issueFieldSave{}, fmt.Errorf("title is required")
	}
	return issueFieldSave{
		issueID: issue.ID,
		message: fieldUpdateMessage(issueFieldTitle),
		apply:   func(input *linearapi.UpdateIssueInput) { input.Title = &title },
	}, nil
}

func issueFieldDescriptionSave(issue linearapi.Issue, body string) issueFieldSave {
	return issueFieldSave{
		issueID: issue.ID,
		message: fieldUpdateMessage(issueFieldDescription),
		apply:   func(input *linearapi.UpdateIssueInput) { input.Description = &body },
	}
}

func issueFieldStateSave(issue linearapi.Issue, stateID, stateName string) issueFieldSave {
	return issueFieldSave{
		issueID: issue.ID,
		message: fieldSetMessage(issueFieldState, stateName),
		apply:   func(input *linearapi.UpdateIssueInput) { input.StateID = &stateID },
	}
}

func issueFieldAssigneeSave(issue linearapi.Issue, userID, userName string) issueFieldSave {
	return issueFieldSave{
		issueID: issue.ID,
		message: fieldSetMessage(issueFieldAssignee, userName),
		apply:   func(input *linearapi.UpdateIssueInput) { input.AssigneeID = &userID },
	}
}

func issueFieldAssigneeClear(issue linearapi.Issue) issueFieldSave {
	return issueFieldSave{
		issueID: issue.ID,
		message: fieldClearMessage(issueFieldAssignee),
		apply:   func(input *linearapi.UpdateIssueInput) { input.AssigneeID = clearedID() },
	}
}

func issueFieldPrioritySave(issue linearapi.Issue, priority int) issueFieldSave {
	return issueFieldSave{
		issueID: issue.ID,
		message: fieldSetMessage(issueFieldPriority, priorityLabel(priority)),
		apply:   func(input *linearapi.UpdateIssueInput) { input.Priority = &priority },
	}
}

func issueFieldLabelsSave(issue linearapi.Issue, labelIDs []string) issueFieldSave {
	return issueFieldSave{
		issueID: issue.ID,
		message: fieldUpdateMessage(issueFieldLabels),
		apply:   func(input *linearapi.UpdateIssueInput) { input.LabelIDs = &labelIDs },
	}
}

func issueFieldProjectSave(issue linearapi.Issue, projectID, projectName string) issueFieldSave {
	return issueFieldSave{
		issueID: issue.ID,
		message: fieldSetMessage(issueFieldProject, projectName),
		apply: func(input *linearapi.UpdateIssueInput) {
			input.ProjectID = &projectID
			clearMilestoneOnProjectChange(input, issue)
		},
	}
}

func issueFieldProjectClear(issue linearapi.Issue) issueFieldSave {
	return issueFieldSave{
		issueID: issue.ID,
		message: fieldClearMessage(issueFieldProject),
		apply: func(input *linearapi.UpdateIssueInput) {
			input.ProjectID = clearedID()
			clearMilestoneOnProjectChange(input, issue)
		},
	}
}

func issueFieldMilestoneSave(issue linearapi.Issue, milestoneID, milestoneName string) issueFieldSave {
	return issueFieldSave{
		issueID: issue.ID,
		message: fieldSetMessage(issueFieldMilestone, milestoneName),
		apply:   func(input *linearapi.UpdateIssueInput) { input.ProjectMilestoneID = &milestoneID },
	}
}

func issueFieldMilestoneClear(issue linearapi.Issue) issueFieldSave {
	return issueFieldSave{
		issueID: issue.ID,
		message: fieldClearMessage(issueFieldMilestone),
		apply:   func(input *linearapi.UpdateIssueInput) { input.ProjectMilestoneID = clearedID() },
	}
}

func issueFieldCycleSave(issue linearapi.Issue, cycleID, cycleName string) issueFieldSave {
	return issueFieldSave{
		issueID: issue.ID,
		message: fieldSetMessage(issueFieldCycle, cycleName),
		apply:   func(input *linearapi.UpdateIssueInput) { input.CycleID = &cycleID },
	}
}

func issueFieldCycleClear(issue linearapi.Issue) issueFieldSave {
	return issueFieldSave{
		issueID: issue.ID,
		message: fieldClearMessage(issueFieldCycle),
		apply:   func(input *linearapi.UpdateIssueInput) { input.CycleID = clearedID() },
	}
}

func issueFieldDueDateSave(issue linearapi.Issue, text string) (issueFieldSave, error) {
	date := strings.TrimSpace(text)
	if err := validateLinearDate(date); err != nil {
		return issueFieldSave{}, err
	}
	return issueFieldSave{
		issueID: issue.ID,
		message: fieldSetMessage(issueFieldDueDate, date),
		apply:   func(input *linearapi.UpdateIssueInput) { input.DueDate = &date },
	}, nil
}

func issueFieldDueDateClear(issue linearapi.Issue) issueFieldSave {
	return issueFieldSave{
		issueID: issue.ID,
		message: fieldClearMessage(issueFieldDueDate),
		apply:   func(input *linearapi.UpdateIssueInput) { input.DueDate = clearedID() },
	}
}

func issueFieldEstimateSave(issue linearapi.Issue, text string) (issueFieldSave, error) {
	estimate, err := parseEstimateInput(text)
	if err != nil {
		return issueFieldSave{}, err
	}
	return issueFieldSave{
		issueID: issue.ID,
		message: fieldSetMessage(issueFieldEstimate, formatEstimate(&estimate)),
		apply:   func(input *linearapi.UpdateIssueInput) { input.Estimate = &estimate },
	}, nil
}

// ClearEstimate rather than a zero pointer, which Linear reads as an estimate of nought.
func issueFieldEstimateClear(issue linearapi.Issue) issueFieldSave {
	return issueFieldSave{
		issueID: issue.ID,
		message: fieldClearMessage(issueFieldEstimate),
		apply:   func(input *linearapi.UpdateIssueInput) { input.ClearEstimate = true },
	}
}

func issueFieldTeamSave(issue linearapi.Issue, teamID, teamName string) issueFieldSave {
	message := fmt.Sprintf("Moved %s", issue.Identifier)
	if teamName != "" {
		message = fmt.Sprintf("Moved %s to %s", issue.Identifier, teamName)
	}
	return issueFieldSave{
		issueID: issue.ID,
		message: message,
		apply:   func(input *linearapi.UpdateIssueInput) { input.TeamID = &teamID },
	}
}

func issueFieldParentSave(issue linearapi.Issue, parentID, parentName string) issueFieldSave {
	return issueFieldSave{
		issueID: issue.ID,
		message: fieldSetMessage(issueFieldParent, parentName),
		apply:   func(input *linearapi.UpdateIssueInput) { input.ParentID = &parentID },
	}
}

func issueFieldParentClear(issue linearapi.Issue) issueFieldSave {
	return issueFieldSave{
		issueID: issue.ID,
		message: fieldClearMessage(issueFieldParent),
		apply:   func(input *linearapi.UpdateIssueInput) { input.ParentID = clearedID() },
	}
}

// Linear reads an empty string as an explicit null; each caller needs its own address.
func clearedID() *string {
	empty := ""
	return &empty
}
