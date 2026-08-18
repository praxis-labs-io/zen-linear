package tui

import (
	"fmt"
	"strings"

	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
)

// issueField names a field of an issue: what a save is keyed by, and what the
// details pane's field cursor points at.
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

// issueFieldNames is what the status bar calls each field.
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

// issueFieldSave is one field's write. Build one with a constructor below, so
// the id and the message come from the same place.
type issueFieldSave struct {
	// Captured where the user chose the issue. A picker outlives a refresh
	// that moves the selection; reading it at send time writes to that.
	issueID string
	message string
	apply   func(*linearapi.UpdateIssueInput)
}

// saveIssueField sends one field and flashes what it did.
func (a *App) saveIssueField(save issueFieldSave) {
	a.saveIssueFieldWithResult(save, nil)
}

// saveIssueFieldWithResult is saveIssueField plus the outcome, for a caller
// holding words it cannot put back once its box has gone.
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

// The three shapes a save reports. One rule here is how "Changed status for
// ZNL-1" and "Updated due date" stop being in the same app.
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

// fieldUpdateMessage is for a field whose new value is not one name to print.
func fieldUpdateMessage(field issueField) string {
	return fmt.Sprintf("Updated %s", issueFieldNames[field])
}

// Refuses the empty title Linear refuses, so a caller can keep its editor open.
// The message names no value: a title is too long for the toast corner.
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

// Sent as written, since leading whitespace is an indented code block to
// Linear. Empty is the clear: Linear reads the empty string as one.
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

// A milestone belongs to one project, so a project change nulls it rather than
// orphaning it against the new one.
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

// Refuses what Linear would refuse, so a caller can keep its editor open on the
// text the user actually typed.
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

// ClearEstimate rather than a zero pointer, which Linear reads as an estimate
// of nought.
func issueFieldEstimateClear(issue linearapi.Issue) issueFieldSave {
	return issueFieldSave{
		issueID: issue.ID,
		message: fieldClearMessage(issueFieldEstimate),
		apply:   func(input *linearapi.UpdateIssueInput) { input.ClearEstimate = true },
	}
}

// A team move renumbers the issue, so the message names the identifier the user
// picked it by rather than the one it now has.
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

// clearedID is the empty string Linear reads as an explicit null. Each caller
// needs its own address, hence a function and not a variable.
func clearedID() *string {
	empty := ""
	return &empty
}
