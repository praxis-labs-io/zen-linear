package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/rivo/tview"
)

const (
	activityIconCreated  = "◆"
	activityIconAssignee = "◇"
	activityIconPlanning = "▸"
	activityIconLink     = "◈"
	activityIconEdit     = "≡"
)

const activityBodyFloor = 8

func (a *App) activityLine(event linearapi.IssueActivity, width int) string {
	if width <= 1 {
		return ""
	}

	glyph, color := a.activityIcon(event)
	head := colorTag(color) + glyph + "[-] "
	body := a.themeTags.SecondaryText + a.activityBody(event)

	tail := ""
	if age := formatRelativeTime(event.CreatedAt); age != "" {
		tail = a.themeTags.SecondaryText + " · " + age + "[-]"
	}

	room := width - tview.TaggedStringWidth(head) - tview.TaggedStringWidth(tail)
	if room >= activityBodyFloor {
		return head + truncateTagged(body, room) + tail
	}
	return truncateTagged(head+body, width)
}

func (a *App) activityIcon(event linearapi.IssueActivity) (string, tcell.Color) {
	switch event.Kind {
	case linearapi.IssueActivityStateChanged:
		state := ""
		if event.ToState != nil {
			state = event.ToState.Name
		}
		return formatStateIcon(state, a.theme)
	case linearapi.IssueActivityPriorityChanged:
		return formatPriority(event.ToPriority, a.theme)
	case linearapi.IssueActivityCreated:
		return activityIconCreated, a.theme.SecondaryText
	case linearapi.IssueActivityAssigned, linearapi.IssueActivitySelfAssigned, linearapi.IssueActivityUnassigned:
		return activityIconAssignee, a.theme.SecondaryText
	case linearapi.IssueActivityRelationAdded, linearapi.IssueActivityRelationRemoved,
		linearapi.IssueActivityLabelsAdded, linearapi.IssueActivityLabelsRemoved:
		return activityIconLink, a.theme.SecondaryText
	case linearapi.IssueActivityTitleChanged, linearapi.IssueActivityDescriptionUpdated:
		return activityIconEdit, a.theme.SecondaryText
	default:
		return activityIconPlanning, a.theme.SecondaryText
	}
}

func (a *App) activityBody(event linearapi.IssueActivity) string {
	phrase := tview.Escape(activityPhrase(event))
	actor := formatUserDisplayName(event.Actor)
	if actor == "" {
		return phrase
	}
	if event.Actor.IsMe {
		actor += " (me)"
	}
	return a.themeTags.AssigneeText + tview.Escape(actor) + a.themeTags.SecondaryText + " " + phrase
}

func activityPhrase(event linearapi.IssueActivity) string {
	switch event.Kind {
	case linearapi.IssueActivityCreated:
		return "created the issue"

	case linearapi.IssueActivityStateChanged:
		to := stateName(event.ToState)
		if from := stateName(event.FromState); from != "" {
			return fmt.Sprintf("moved from %s to %s", from, to)
		}
		return "moved to " + to

	case linearapi.IssueActivityAssigned:
		return "assigned " + assigneeName(event)
	case linearapi.IssueActivitySelfAssigned:
		return "self-assigned the issue"
	case linearapi.IssueActivityUnassigned:
		if name := assigneeName(event); name != "" {
			return "unassigned " + name
		}
		return "removed the assignee"

	case linearapi.IssueActivityCycleAdded:
		return "added issue to " + cycleName(event)
	case linearapi.IssueActivityCycleRemoved:
		return "removed issue from " + cycleName(event)

	case linearapi.IssueActivityProjectSet:
		return "added issue to project " + projectName(event)
	case linearapi.IssueActivityProjectRemoved:
		return "removed issue from project " + projectName(event)

	case linearapi.IssueActivityMilestoneSet:
		return "set milestone to " + milestoneName(event)
	case linearapi.IssueActivityMilestoneRemoved:
		return "removed the milestone"

	case linearapi.IssueActivityParentSet:
		return "set parent to " + parentName(event)
	case linearapi.IssueActivityParentRemoved:
		return "removed parent " + parentName(event)

	case linearapi.IssueActivityLabelsAdded:
		return "added " + labelPhrase(event.Labels)
	case linearapi.IssueActivityLabelsRemoved:
		return "removed " + labelPhrase(event.Labels)

	case linearapi.IssueActivityRelationAdded:
		return fmt.Sprintf("added %s issue %s", event.Relation, event.RelatedIssue)
	case linearapi.IssueActivityRelationRemoved:
		return fmt.Sprintf("removed %s issue %s", event.Relation, event.RelatedIssue)

	case linearapi.IssueActivityPriorityChanged:
		return priorityPhrase(event.FromPriority, event.ToPriority)

	case linearapi.IssueActivityTitleChanged:
		return "changed the title"
	case linearapi.IssueActivityDescriptionUpdated:
		return "updated the description"
	}
	return ""
}

// Linear's scale is inverted: 1 is Urgent and 4 is Low.
func priorityPhrase(from, to int) string {
	switch {
	case to == 0:
		return "removed the priority"
	case from == 0:
		return "set priority to " + priorityLabel(to)
	case to > from:
		return fmt.Sprintf("lowered priority from %s to %s", priorityLabel(from), priorityLabel(to))
	default:
		return fmt.Sprintf("raised priority from %s to %s", priorityLabel(from), priorityLabel(to))
	}
}

func labelPhrase(labels []linearapi.IssueLabel) string {
	names := make([]string, 0, len(labels))
	for _, label := range labels {
		names = append(names, label.Name)
	}
	noun := "label"
	if len(names) > 1 {
		noun = "labels"
	}
	return noun + " " + strings.Join(names, ", ")
}

func stateName(state *linearapi.WorkflowState) string {
	if state == nil {
		return ""
	}
	return state.Name
}

func assigneeName(event linearapi.IssueActivity) string {
	if event.Assignee == nil {
		return ""
	}
	return formatUserDisplayName(*event.Assignee)
}

func cycleName(event linearapi.IssueActivity) string {
	if event.Cycle == nil {
		return "a cycle"
	}
	return event.Cycle.DisplayName()
}

func projectName(event linearapi.IssueActivity) string {
	if event.Project == nil {
		return ""
	}
	return event.Project.Name
}

func milestoneName(event linearapi.IssueActivity) string {
	if event.Milestone == nil {
		return ""
	}
	return event.Milestone.Name
}

func parentName(event linearapi.IssueActivity) string {
	if event.Parent == nil {
		return ""
	}
	return event.Parent.Identifier
}
