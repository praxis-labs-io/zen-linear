package linearapi

import "strings"

// relationChangePhrases maps Linear's relationChanges type code to the relation
// as the fetched issue sees it, matching IssueRelation.DisplayType's wording.
//
// The codes are undocumented. Observed across a workspace's history they read
// as a letter per relation kind — r related, x blocking, b blocked by,
// d duplicate, m duplicate of — prefixed with "a" when the relation was added
// and suffixed with "r" when it was removed. The five addition codes and "xr",
// "br" were seen directly; "rr", "dr" and "mr" follow the pattern.
//
// An unrecognized code drops the event, so a vocabulary Linear extends costs a
// line rather than printing a wrong one.
var relationChangePhrases = map[string]string{
	"ar": "related",
	"rr": "related",
	"ax": "blocking",
	"xr": "blocking",
	"ab": "blocked by",
	"br": "blocked by",
	"ad": "duplicate",
	"dr": "duplicate",
	"am": "duplicate of",
	"mr": "duplicate of",
}

// relationChange reads a type code. added is false on a removal, and ok is
// false when the code is one this does not recognize.
func relationChange(code string) (relation string, added bool, ok bool) {
	relation, ok = relationChangePhrases[code]
	// No relation letter is "a", so a leading one can only be the add marker.
	return relation, strings.HasPrefix(code, "a"), ok
}

// actorUser flattens the two ways Linear names who made a change. A person
// acting through an integration is still the person, so the user wins when
// both are recorded.
//
// A bot arrives as a user carrying its own name, which is how it reads in the
// feed: "Linear moved issue to Cycle 152" wants no marking a name does not
// already carry.
func actorUser(user *historyUserNode, bot *actorBotNode) User {
	if converted := user.toUser(); converted != nil {
		return *converted
	}
	if bot == nil {
		return User{}
	}
	return User{
		ID:          string(bot.ID),
		Name:        string(bot.Name),
		DisplayName: string(bot.UserDisplayName),
	}
}

func (n *historyUserNode) toUser() *User {
	if n == nil || n.ID == "" {
		return nil
	}
	return &User{
		ID:          string(n.ID),
		Name:        string(n.Name),
		DisplayName: string(n.DisplayName),
		Email:       string(n.Email),
		IsMe:        bool(n.IsMe),
	}
}

func (n *historyStateNode) toState() *WorkflowState {
	if n == nil || n.ID == "" {
		return nil
	}
	return &WorkflowState{
		ID:   string(n.ID),
		Name: string(n.Name),
		Type: string(n.Type),
	}
}

func (n *historyCycleNode) toRef() *CycleRef {
	if n == nil || n.ID == "" {
		return nil
	}
	name := ""
	if n.Name != nil {
		name = string(*n.Name)
	}
	return &CycleRef{ID: string(n.ID), Name: name, Number: int(n.Number)}
}

func (n *historyNamedNode) toProject() *Project {
	if n == nil || n.ID == "" {
		return nil
	}
	return &Project{ID: string(n.ID), Name: string(n.Name)}
}

func (n *historyNamedNode) toMilestone() *ProjectMilestoneRef {
	if n == nil || n.ID == "" {
		return nil
	}
	return &ProjectMilestoneRef{ID: string(n.ID), Name: string(n.Name)}
}

func (n *historyIssueNode) toRef() *IssueRef {
	if n == nil || n.ID == "" {
		return nil
	}
	return &IssueRef{
		ID:         string(n.ID),
		Identifier: string(n.Identifier),
		Title:      string(n.Title),
	}
}

func toActivityLabels(nodes []historyLabelNode) []IssueLabel {
	labels := make([]IssueLabel, 0, len(nodes))
	for _, node := range nodes {
		labels = append(labels, IssueLabel{
			ID:    string(node.ID),
			Name:  string(node.Name),
			Color: string(node.Color),
		})
	}
	return labels
}

// withKind copies the entry's shared fields under kind, so the several events
// one history entry can produce all carry the same actor, time and id.
func (a IssueActivity) withKind(kind IssueActivityKind) IssueActivity {
	a.Kind = kind
	return a
}

// toActivity converts one history entry into an event per change it records.
//
// One entry yields several events because the feed draws one icon and one
// phrase per line: an entry carrying a state move and an assignee change has no
// single icon and no one-line phrase. Splitting also makes dropping a change
// this does not render a per-change decision, so an entry that mixes a
// supported change with an unsupported one still produces its supported line.
// Siblings share CreatedAt and the sort is stable, so they stay adjacent.
//
// An entry with no time, or recording nothing renderable, returns no events.
func (n issueHistoryNode) toActivity() []IssueActivity {
	at := parseTime(string(n.CreatedAt))
	// The feed sorts on this and reads an age off it. A zero time heads the
	// feed above the issue's own creation and draws no age at all, so an entry
	// whose time did not parse is dropped rather than shown out of order.
	if at.IsZero() {
		return nil
	}
	base := IssueActivity{ID: string(n.ID), CreatedAt: at, Actor: actorUser(n.Actor, n.BotActor)}

	events := make([]IssueActivity, 0, 2)

	// A move needs somewhere it went. Linear records no from side on the first
	// move out of the initial state, but an issue always lands in a state.
	if n.ToState != nil {
		event := base.withKind(IssueActivityStateChanged)
		event.FromState, event.ToState = n.FromState.toState(), n.ToState.toState()
		events = append(events, event)
	}

	if assignee := n.ToAssignee.toUser(); assignee != nil {
		kind := IssueActivityAssigned
		if base.Actor.ID != "" && base.Actor.ID == assignee.ID {
			kind = IssueActivitySelfAssigned
		}
		event := base.withKind(kind)
		event.Assignee = assignee
		events = append(events, event)
	} else if previous := n.FromAssignee.toUser(); previous != nil {
		event := base.withKind(IssueActivityUnassigned)
		event.Assignee = previous
		events = append(events, event)
	}

	if n.ToPriority != nil {
		event := base.withKind(IssueActivityPriorityChanged)
		if n.FromPriority != nil {
			event.FromPriority = int(*n.FromPriority)
		}
		event.ToPriority = int(*n.ToPriority)
		events = append(events, event)
	}

	if n.ToTitle != nil {
		events = append(events, base.withKind(IssueActivityTitleChanged))
	}

	if n.UpdatedDescription != nil && bool(*n.UpdatedDescription) {
		events = append(events, base.withKind(IssueActivityDescriptionUpdated))
	}

	if project := n.ToProject.toProject(); project != nil {
		event := base.withKind(IssueActivityProjectSet)
		event.Project = project
		events = append(events, event)
	} else if previous := n.FromProject.toProject(); previous != nil {
		event := base.withKind(IssueActivityProjectRemoved)
		event.Project = previous
		events = append(events, event)
	}

	if milestone := n.ToProjectMilestone.toMilestone(); milestone != nil {
		event := base.withKind(IssueActivityMilestoneSet)
		event.Milestone = milestone
		events = append(events, event)
	} else if previous := n.FromProjectMilestone.toMilestone(); previous != nil {
		event := base.withKind(IssueActivityMilestoneRemoved)
		event.Milestone = previous
		events = append(events, event)
	}

	if parent := n.ToParent.toRef(); parent != nil {
		event := base.withKind(IssueActivityParentSet)
		event.Parent = parent
		events = append(events, event)
	} else if previous := n.FromParent.toRef(); previous != nil {
		event := base.withKind(IssueActivityParentRemoved)
		event.Parent = previous
		events = append(events, event)
	}

	if cycle := n.ToCycle.toRef(); cycle != nil {
		event := base.withKind(IssueActivityCycleAdded)
		event.Cycle = cycle
		events = append(events, event)
	} else if previous := n.FromCycle.toRef(); previous != nil {
		event := base.withKind(IssueActivityCycleRemoved)
		event.Cycle = previous
		events = append(events, event)
	}

	if len(n.AddedLabels) > 0 {
		event := base.withKind(IssueActivityLabelsAdded)
		event.Labels = toActivityLabels(n.AddedLabels)
		events = append(events, event)
	}
	if len(n.RemovedLabels) > 0 {
		event := base.withKind(IssueActivityLabelsRemoved)
		event.Labels = toActivityLabels(n.RemovedLabels)
		events = append(events, event)
	}

	for _, change := range n.RelationChanges {
		relation, added, ok := relationChange(string(change.Type))
		if !ok {
			continue
		}
		kind := IssueActivityRelationAdded
		if !added {
			kind = IssueActivityRelationRemoved
		}
		event := base.withKind(kind)
		event.RelatedIssue = string(change.Identifier)
		event.Relation = relation
		events = append(events, event)
	}

	return events
}
