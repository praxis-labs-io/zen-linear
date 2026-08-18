package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/rivo/tview"
)

func activityAt(minutes int, kind linearapi.IssueActivityKind) linearapi.IssueActivity {
	return linearapi.IssueActivity{
		Kind:      kind,
		CreatedAt: time.Now().Add(time.Duration(minutes) * time.Minute),
		Actor:     linearapi.User{ID: "u1", DisplayName: "drew"},
	}
}

// pageLines draws the page and returns its rows. Drawn rather than read off the
// text, because glamour breaks a rendered body with color tags mid-sentence and
// only the screen has the words back in one piece. One page line is one row, so
// the indices are the page's own.
func pageLines(t *testing.T, app *App) []string {
	t.Helper()
	return drawComments(t, app, 90)
}

// lineIndex finds the one page line containing want, failing if it is missing
// or drawn more than once.
func lineIndex(t *testing.T, lines []string, want string) int {
	t.Helper()
	found := -1
	for i, line := range lines {
		if strings.Contains(line, want) {
			if found >= 0 {
				t.Fatalf("%q is on lines %d and %d", want, found, i)
			}
			found = i
		}
	}
	if found < 0 {
		t.Fatalf("%q is not on the page:\n%s", want, strings.Join(lines, "\n"))
	}
	return found
}

func newActivityTestApp(t *testing.T, events []linearapi.IssueActivity) *App {
	t.Helper()

	app := newDetailsTestApp(t)
	issue := detailsFixture()
	issue.Comments = threadedComments()
	issue.Activity = events
	app.selectedIssue = issue
	app.updateDetailsView()
	drawComments(t, app, 90)
	return app
}

// The threaded fixture's roots sit at -60, -40 and -20 minutes, with root-1's
// replies at -50 and -30.
func TestActivityInterleavesWithTheComments(t *testing.T) {
	events := []linearapi.IssueActivity{
		activityAt(-70, linearapi.IssueActivityCreated),
		activityAt(-45, linearapi.IssueActivityTitleChanged),
		activityAt(-10, linearapi.IssueActivityDescriptionUpdated),
	}
	app := newActivityTestApp(t, events)
	lines := pageLines(t, app)

	created := lineIndex(t, lines, "created the issue")
	root1 := lineIndex(t, lines, "The debounce is the problem.")
	title := lineIndex(t, lines, "changed the title")
	root2 := lineIndex(t, lines, "Unrelated thought.")
	orphan := lineIndex(t, lines, "Answering something older.")
	described := lineIndex(t, lines, "updated the description")

	if created > root1 {
		t.Errorf("the creation is below the first comment (%d > %d)", created, root1)
	}
	if root1 >= title || title >= root2 {
		t.Errorf("the title change is not between the two roots: %d, %d, %d", root1, title, root2)
	}
	if described < orphan {
		t.Errorf("the last event is above the last comment (%d < %d)", described, orphan)
	}
}

// A thread is placed as a whole. An event stamped inside one lands after its
// last reply, because a card split off from its rail would leave the rail
// trailing into a line that is not a card.
func TestAnEventInsideAThreadLandsAfterIt(t *testing.T) {
	app := newActivityTestApp(t, []linearapi.IssueActivity{
		activityAt(-55, linearapi.IssueActivityTitleChanged),
	})
	lines := pageLines(t, app)

	root := lineIndex(t, lines, "The debounce is the problem.")
	firstReply := lineIndex(t, lines, "Which one?")
	lastReply := lineIndex(t, lines, "The detail one.")
	event := lineIndex(t, lines, "changed the title")

	if event < lastReply {
		t.Errorf("the event split the thread: root %d, replies %d and %d, event %d",
			root, firstReply, lastReply, event)
	}
}

// Each event is one row, and a run of them carries no blank row between.
func TestAnActivityRunIsOneRowPerEventWithNoGaps(t *testing.T) {
	app := newActivityTestApp(t, []linearapi.IssueActivity{
		activityAt(-12, linearapi.IssueActivityTitleChanged),
		activityAt(-11, linearapi.IssueActivityDescriptionUpdated),
		activityAt(-10, linearapi.IssueActivityCreated),
	})
	lines := pageLines(t, app)

	title := lineIndex(t, lines, "changed the title")
	description := lineIndex(t, lines, "updated the description")
	created := lineIndex(t, lines, "created the issue")

	if description != title+1 || created != description+1 {
		t.Errorf("the run is not three adjacent rows: %d, %d, %d", title, description, created)
	}
}

func TestActivityPhrase(t *testing.T) {
	state := func(name string) *linearapi.WorkflowState { return &linearapi.WorkflowState{ID: "s", Name: name} }

	tests := []struct {
		name  string
		event linearapi.IssueActivity
		want  string
	}{
		{
			name:  "created",
			event: linearapi.IssueActivity{Kind: linearapi.IssueActivityCreated},
			want:  "created the issue",
		},
		{
			name: "state move",
			event: linearapi.IssueActivity{
				Kind: linearapi.IssueActivityStateChanged, FromState: state("Backlog"), ToState: state("To Do"),
			},
			want: "moved from Backlog to To Do",
		},
		{
			name:  "state set with no previous state",
			event: linearapi.IssueActivity{Kind: linearapi.IssueActivityStateChanged, ToState: state("To Do")},
			want:  "moved to To Do",
		},
		{
			name: "assigned",
			event: linearapi.IssueActivity{
				Kind: linearapi.IssueActivityAssigned, Assignee: &linearapi.User{DisplayName: "mike"},
			},
			want: "assigned mike",
		},
		{
			name:  "self assigned",
			event: linearapi.IssueActivity{Kind: linearapi.IssueActivitySelfAssigned},
			want:  "self-assigned the issue",
		},
		{
			name: "unassigned",
			event: linearapi.IssueActivity{
				Kind: linearapi.IssueActivityUnassigned, Assignee: &linearapi.User{DisplayName: "mike"},
			},
			want: "unassigned mike",
		},
		{
			name: "cycle added falls back to the number",
			event: linearapi.IssueActivity{
				Kind: linearapi.IssueActivityCycleAdded, Cycle: &linearapi.CycleRef{ID: "c", Number: 154},
			},
			want: "added issue to Cycle 154",
		},
		{
			name: "cycle removed",
			event: linearapi.IssueActivity{
				Kind: linearapi.IssueActivityCycleRemoved, Cycle: &linearapi.CycleRef{ID: "c", Number: 152},
			},
			want: "removed issue from Cycle 152",
		},
		{
			name: "project set",
			event: linearapi.IssueActivity{
				Kind: linearapi.IssueActivityProjectSet, Project: &linearapi.Project{Name: "Feature Backlog"},
			},
			want: "added issue to project Feature Backlog",
		},
		{
			name: "milestone set",
			event: linearapi.IssueActivity{
				Kind: linearapi.IssueActivityMilestoneSet, Milestone: &linearapi.ProjectMilestoneRef{Name: "Beta"},
			},
			want: "set milestone to Beta",
		},
		{
			name:  "milestone removed names nothing",
			event: linearapi.IssueActivity{Kind: linearapi.IssueActivityMilestoneRemoved},
			want:  "removed the milestone",
		},
		{
			name: "parent set",
			event: linearapi.IssueActivity{
				Kind: linearapi.IssueActivityParentSet, Parent: &linearapi.IssueRef{Identifier: "ZNL-9"},
			},
			want: "set parent to ZNL-9",
		},
		{
			name: "one label",
			event: linearapi.IssueActivity{
				Kind: linearapi.IssueActivityLabelsAdded, Labels: []linearapi.IssueLabel{{Name: "Bug"}},
			},
			want: "added label Bug",
		},
		{
			name: "several labels",
			event: linearapi.IssueActivity{
				Kind:   linearapi.IssueActivityLabelsRemoved,
				Labels: []linearapi.IssueLabel{{Name: "Bug"}, {Name: "Urgent"}},
			},
			want: "removed labels Bug, Urgent",
		},
		{
			name: "relation added",
			event: linearapi.IssueActivity{
				Kind: linearapi.IssueActivityRelationAdded, Relation: "related", RelatedIssue: "ZNL-108",
			},
			want: "added related issue ZNL-108",
		},
		{
			name: "relation removed",
			event: linearapi.IssueActivity{
				Kind: linearapi.IssueActivityRelationRemoved, Relation: "blocked by", RelatedIssue: "ZNL-77",
			},
			want: "removed blocked by issue ZNL-77",
		},
		{
			// Linear's scale is inverted, so the larger number is the lower
			// priority.
			name:  "priority lowered",
			event: linearapi.IssueActivity{Kind: linearapi.IssueActivityPriorityChanged, FromPriority: 2, ToPriority: 4},
			want:  "lowered priority from High to Low",
		},
		{
			name:  "priority raised",
			event: linearapi.IssueActivity{Kind: linearapi.IssueActivityPriorityChanged, FromPriority: 4, ToPriority: 1},
			want:  "raised priority from Low to Urgent",
		},
		{
			name:  "priority set from none",
			event: linearapi.IssueActivity{Kind: linearapi.IssueActivityPriorityChanged, ToPriority: 2},
			want:  "set priority to High",
		},
		{
			name:  "priority cleared",
			event: linearapi.IssueActivity{Kind: linearapi.IssueActivityPriorityChanged, FromPriority: 2},
			want:  "removed the priority",
		},
		{
			name:  "title carries no value",
			event: linearapi.IssueActivity{Kind: linearapi.IssueActivityTitleChanged},
			want:  "changed the title",
		},
		{
			name:  "description carries no value",
			event: linearapi.IssueActivity{Kind: linearapi.IssueActivityDescriptionUpdated},
			want:  "updated the description",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := activityPhrase(test.event); got != test.want {
				t.Errorf("activityPhrase() = %q, want %q", got, test.want)
			}
		})
	}
}

// The state and priority glyphs come from the helpers the list and the header
// already use, so one issue reads the same everywhere it appears.
func TestActivityIconsComeFromTheSharedHelpers(t *testing.T) {
	app := newDetailsTestApp(t)

	stateGlyph, stateColor := formatStateIcon("In Progress", app.theme)
	gotGlyph, gotColor := app.activityIcon(linearapi.IssueActivity{
		Kind:    linearapi.IssueActivityStateChanged,
		ToState: &linearapi.WorkflowState{Name: "In Progress"},
	})
	if gotGlyph != stateGlyph || gotColor != stateColor {
		t.Errorf("state icon = %q/%v, want %q/%v", gotGlyph, gotColor, stateGlyph, stateColor)
	}

	priorityGlyph, priorityColor := formatPriority(4, app.theme)
	gotGlyph, gotColor = app.activityIcon(linearapi.IssueActivity{
		Kind: linearapi.IssueActivityPriorityChanged, ToPriority: 4,
	})
	if gotGlyph != priorityGlyph || gotColor != priorityColor {
		t.Errorf("priority icon = %q/%v, want %q/%v", gotGlyph, gotColor, priorityGlyph, priorityColor)
	}
}

// Linear records no actor on an automated transition. The line opens on the
// change rather than on a gap where a name would go.
func TestAnEventWithNoActorOpensOnThePhrase(t *testing.T) {
	app := newDetailsTestApp(t)
	line := app.activityLine(linearapi.IssueActivity{
		Kind: linearapi.IssueActivityTitleChanged, CreatedAt: time.Now(),
	}, 60)

	body := tview.Escape(line)
	if strings.Contains(body, "  changed") {
		t.Errorf("the line left a gap where the actor would be: %q", line)
	}
	if !strings.Contains(line, "changed the title") {
		t.Errorf("the line dropped the phrase: %q", line)
	}
}

// The phrase is what shrinks: the actor sits ahead of it and the age behind it,
// because those are what a feed is scanned for.
func TestANarrowLineKeepsTheActorAndTheAge(t *testing.T) {
	app := newDetailsTestApp(t)
	event := linearapi.IssueActivity{
		Kind:         linearapi.IssueActivityRelationAdded,
		Relation:     "related",
		RelatedIssue: "ZNL-108",
		CreatedAt:    time.Now().Add(-3 * time.Hour),
		Actor:        linearapi.User{ID: "u1", DisplayName: "drew"},
	}

	line := app.activityLine(event, 30)
	if !strings.Contains(line, "drew") {
		t.Errorf("the actor was clipped at width 30: %q", line)
	}
	if !strings.Contains(line, "3h") {
		t.Errorf("the age was dropped at width 30: %q", line)
	}
	if got := tview.TaggedStringWidth(line); got > 30 {
		t.Errorf("the line is %d cells in a 30 pane: %q", got, line)
	}

	// Below the floor the age goes rather than the name.
	if got := tview.TaggedStringWidth(app.activityLine(event, 12)); got > 12 {
		t.Errorf("the line is %d cells in a 12 pane", got)
	}
	// truncateTagged wraps at width-1, so a measure of one leaves nothing to
	// cut on and hands the line back whole.
	for _, width := range []int{0, 1} {
		if got := app.activityLine(event, width); got != "" {
			t.Errorf("width %d drew %q", width, got)
		}
	}
}

// Every name on the line comes from the API and the view has dynamic colors on,
// so a bracket in one would be read as a tag: swallowed on screen, and short of
// what the fit measured.
func TestActivityNamesCannotBeReadAsColorTags(t *testing.T) {
	app := newDetailsTestApp(t)
	line := app.activityLine(linearapi.IssueActivity{
		Kind:      linearapi.IssueActivityLabelsAdded,
		CreatedAt: time.Now(),
		Actor:     linearapi.User{ID: "u1", DisplayName: "[red]drew"},
		Labels:    []linearapi.IssueLabel{{Name: "[Bug]"}},
	}, 80)

	for _, want := range []string{"[red]drew", "[Bug]"} {
		if !strings.Contains(line, tview.Escape(want)) {
			t.Errorf("%q is not escaped on the line: %q", want, line)
		}
	}
}

// The ring walks cards. An activity line records no span, so it is not a stop.
func TestActivityLinesAreNotRingStops(t *testing.T) {
	app := newDetailsTestApp(t)
	issue := detailsFixture()
	issue.Comments = threadedComments()
	issue.Activity = []linearapi.IssueActivity{
		activityAt(-70, linearapi.IssueActivityCreated),
		activityAt(-45, linearapi.IssueActivityTitleChanged),
		activityAt(-10, linearapi.IssueActivityDescriptionUpdated),
	}
	issue.URL = "https://linear.app/praxis-labs/issue/ZNO-1"
	app.selectedIssue = issue
	app.updateDetailsView()
	focusCommentCards(app)
	drawComments(t, app, 80)

	want := []string{"root-1", "reply-1", "reply-2", "root-2", "orphan", blockIDCompose}
	got := make([]string, 0, len(want))
	for _, span := range app.commentSpans {
		got = append(got, span.id)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("the ring stops on %v, want %v", got, want)
	}
}
