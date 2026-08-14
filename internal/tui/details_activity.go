package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// Glyphs for the activity events that have no icon of their own. A state move
// and a priority change take theirs from the helpers the list and the header
// already use, so one issue reads the same in all three.
const (
	activityIconCreated  = "◆"
	activityIconAssignee = "◇"
	activityIconPlanning = "▸"
	activityIconLink     = "◈"
	activityIconEdit     = "≡"
)

// activityBodyFloor is the narrowest the actor and phrase are worth keeping. A
// pane tighter than this drops the age instead, since a name clipped to three
// cells names nobody.
const activityBodyFloor = 8

// activityLine renders one event as a single fitted row: an icon, who did it,
// what they did, and how long ago.
//
// The phrase is what shrinks. The actor sits ahead of it and the age behind it,
// because a feed already carries its order in the arrangement of its rows, and
// what a reader scans for is who and when.
func (a *App) activityLine(event linearapi.IssueActivity, width int) string {
	if width <= 1 {
		// truncateTagged hands the line back untouched at a width of one: it
		// wraps at width-1, and a wrap to zero returns nothing to cut on. An
		// unfitted row would overrun the pane.
		return ""
	}

	glyph, color := a.activityIcon(event)
	head := colorTag(color) + glyph + "[-] "
	body := a.themeTags.SecondaryText + a.activityBody(event)

	tail := ""
	if age := formatRelativeTime(event.CreatedAt); age != "" {
		// The dim tag is re-opened here: a truncated body ends in the reset
		// truncateTagged appends, which would leave the age undimmed.
		tail = a.themeTags.SecondaryText + " · " + age + "[-]"
	}

	room := width - tview.TaggedStringWidth(head) - tview.TaggedStringWidth(tail)
	if room >= activityBodyFloor {
		return head + truncateTagged(body, room) + tail
	}
	return truncateTagged(head+body, width)
}

// activityIcon is the glyph and color for an event's kind.
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

// activityBody is the actor and the phrase, joined. An event Linear recorded no
// actor for reads as the change alone rather than opening on a gap.
//
// Both are escaped before the theme tags go on. Every name in them comes from
// the API, the view has dynamic colors on, and a label called "[Bug]" would
// otherwise be read as a tag: swallowed on screen and four cells short of what
// the fit measured.
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

// activityPhrase says what changed, in Linear's own wording.
//
// The title and the description carry no value: both are long, both are already
// on the page above, and both would be the line that always truncates.
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

// priorityPhrase names which way a priority went. Linear's scale is inverted —
// 1 is Urgent and 4 is Low — so a larger number is the lower priority.
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
