package tui

import (
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// threadedComments is a discussion with a reply, a reply to that reply, and a
// reply whose parent is older than the fetched page.
func threadedComments() []linearapi.Comment {
	now := time.Now()
	at := func(minutes int) time.Time { return now.Add(time.Duration(minutes) * time.Minute) }
	return []linearapi.Comment{
		{ID: "root-1", Body: "The debounce is the problem.", CreatedAt: at(-60), UpdatedAt: at(-60),
			Author: linearapi.User{ID: "u1", DisplayName: "drew", IsMe: true}, URL: "https://linear.app/c/root-1"},
		{ID: "reply-1", ParentID: "root-1", Body: "Which one?", CreatedAt: at(-50), UpdatedAt: at(-50),
			Author: linearapi.User{ID: "u2", DisplayName: "roey"}, URL: "https://linear.app/c/reply-1"},
		{ID: "root-2", Body: "Unrelated thought.", CreatedAt: at(-40), UpdatedAt: at(-40),
			Author: linearapi.User{ID: "u2", DisplayName: "roey"}, URL: "https://linear.app/c/root-2"},
		{ID: "reply-2", ParentID: "reply-1", Body: "The detail one.", CreatedAt: at(-30), UpdatedAt: at(-30),
			Author: linearapi.User{ID: "u1", DisplayName: "drew", IsMe: true}, URL: "https://linear.app/c/reply-2"},
		{ID: "orphan", ParentID: "off-the-page", Body: "Answering something older.", CreatedAt: at(-20), UpdatedAt: at(-20),
			Author: linearapi.User{ID: "u2", DisplayName: "roey"}, URL: "https://linear.app/c/orphan"},
	}
}

func rowIDs(rows []commentRow) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.Comment.ID)
	}
	return ids
}

func TestBuildCommentRows(t *testing.T) {
	tests := []struct {
		name      string
		comments  []linearapi.Comment
		wantIDs   []string
		wantDepth map[string]int
	}{
		{
			name:      "no comments",
			wantIDs:   []string{},
			wantDepth: map[string]int{},
		},
		{
			name: "roots keep their order",
			comments: []linearapi.Comment{
				{ID: "a"}, {ID: "b"},
			},
			wantIDs:   []string{"a", "b"},
			wantDepth: map[string]int{"a": 0, "b": 0},
		},
		{
			name:     "replies gather under their parent",
			comments: threadedComments(),
			// reply-2 answers reply-1 and lands in the same thread, not a
			// second level under it. orphan's parent is off the page, so it
			// reads as a root rather than disappearing.
			wantIDs: []string{"root-1", "reply-1", "reply-2", "root-2", "orphan"},
			wantDepth: map[string]int{
				"root-1": 0, "reply-1": 1, "reply-2": 1, "root-2": 0, "orphan": 0,
			},
		},
		{
			name: "a parent cycle does not hang the pane",
			comments: []linearapi.Comment{
				{ID: "a", ParentID: "b"}, {ID: "b", ParentID: "a"},
			},
			wantIDs:   []string{"a", "b"},
			wantDepth: map[string]int{"a": 0, "b": 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := buildCommentRows(tt.comments)
			if got := rowIDs(rows); !slices.Equal(got, tt.wantIDs) {
				t.Fatalf("rows = %v, want %v", got, tt.wantIDs)
			}
			for _, row := range rows {
				if got, want := row.Depth, tt.wantDepth[row.Comment.ID]; got != want {
					t.Errorf("%s is at depth %d, want %d", row.Comment.ID, got, want)
				}
			}
		})
	}
}

func TestThreadRootID(t *testing.T) {
	comments := threadedComments()
	tests := []struct {
		name string
		id   string
		want string
	}{
		{name: "a root is its own thread", id: "root-1", want: "root-1"},
		{name: "a reply names its parent", id: "reply-1", want: "root-1"},
		{name: "a deeper reply names the thread", id: "reply-2", want: "root-1"},
		// Drawn as a root, because the page has nothing to nest it under, and
		// still answered to its own parent: Linear refuses a reply whose parent
		// is a reply, which is what posting against itself would be.
		{name: "an orphan answers the parent it lost", id: "orphan", want: "off-the-page"},
		{name: "an unknown comment is left alone", id: "gone", want: "gone"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := threadRootID(comments, tt.id); got != tt.want {
				t.Errorf("threadRootID(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

// TestRepliesIndentUnderTheirParent covers the thread reading as a step in.
// The rows are not trimmed here: the indent is the thing being measured.
func TestRepliesIndentUnderTheirParent(t *testing.T) {
	app := newThreadedTestApp(t)
	lines := drawComments(t, app, 80)

	parent := cardEdgeColumn(t, lines, "root-1 marker", 0)
	reply := cardEdgeColumn(t, lines, "reply marker", 1)
	if got := reply - parent; got != commentThreadIndent {
		t.Errorf("the reply card starts %d cells in from its parent, want %d", got, commentThreadIndent)
	}
}

// TestTheThreadRailJoinsRepliesToTheirParent covers the connector: a rail down
// the gutter, an elbow into each reply's byline, and a corner on the last one so
// the run stops rather than trailing into the next thread.
func TestTheThreadRailJoinsRepliesToTheirParent(t *testing.T) {
	app := newThreadedTestApp(t)
	lines := drawComments(t, app, 80)

	// The elbow and the corner carry a space the card's own rule and bottom
	// border do not, which is what tells ├─ the connector from ├─── the rule.
	rail := 0
	elbows, corners := 0, 0
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		switch {
		case strings.HasPrefix(trimmed, "├─ "):
			elbows++
		case strings.HasPrefix(trimmed, "╰─ "):
			corners++
		case trimmed == "│":
			rail++
		}
	}

	// One elbow for the first reply of the thread, one corner for the last.
	if elbows != 1 || corners != 1 {
		t.Errorf("drew %d elbows and %d corners, want one of each:\n%s", elbows, corners, strings.Join(lines, "\n"))
	}
	if rail == 0 {
		t.Errorf("the replies hang off no rail:\n%s", strings.Join(lines, "\n"))
	}

	// The elbow meets the byline, one row under the reply's top border.
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimLeft(line, " "), "├─ ") {
			if !strings.Contains(lines[i-1], "╭") {
				t.Errorf("the elbow is on row %d, want it against the card's byline:\n%s", i, strings.Join(lines[i-2:i+2], "\n"))
			}
			break
		}
	}
}

// cardEdgeColumn returns the column the nth card's top border starts at,
// counted in cells rather than bytes: the rail drawn in a reply's gutter is
// three bytes to the cell.
func cardEdgeColumn(t *testing.T, lines []string, what string, n int) int {
	t.Helper()
	seen := 0
	for _, line := range lines {
		at := strings.Index(line, "╭")
		if at < 0 {
			continue
		}
		if seen == n {
			return utf8.RuneCountInString(line[:at])
		}
		seen++
	}
	t.Fatalf("no card %d to measure (%s):\n%s", n, what, strings.Join(lines, "\n"))
	return 0
}

// TestEveryPageLineFitsThePane is the invariant the slots and the ring stand
// on: one line of the page is one row on the screen. A line wider than the
// pane would be wrapped by the view into two, and every box and every stop
// below it would be a row out of place.
func TestEveryPageLineFitsThePane(t *testing.T) {
	now := time.Now()
	unbreakable := linearapi.Comment{
		ID: "wide", CreatedAt: now, UpdatedAt: now,
		Author: linearapi.User{ID: "u1", DisplayName: "drew"},
		Body:   "https://example.com/a/very/long/path/that/cannot/be/broken/anywhere",
	}

	for _, fixture := range []struct {
		name     string
		comments []linearapi.Comment
		activity []linearapi.IssueActivity
	}{
		{name: "a thread", comments: threadedComments()},
		{name: "nothing written yet"},
		{name: "a line nothing can break", comments: []linearapi.Comment{unbreakable}},
		{
			name:     "a feed with activity",
			comments: threadedComments(),
			activity: []linearapi.IssueActivity{
				{
					Kind: linearapi.IssueActivityStateChanged, CreatedAt: now.Add(-time.Hour),
					Actor:     linearapi.User{ID: "u1", DisplayName: "a-display-name-nobody-would-choose"},
					FromState: &linearapi.WorkflowState{Name: "In Review, Pending Second Approval"},
					ToState:   &linearapi.WorkflowState{Name: "Blocked On Somebody Else Entirely"},
				},
				{
					Kind: linearapi.IssueActivityRelationAdded, CreatedAt: now.Add(-30 * time.Minute),
					Relation: "blocked by", RelatedIssue: "ZNL-108",
				},
			},
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			app := newDetailsTestApp(t)
			issue := detailsFixture()
			issue.Comments = fixture.comments
			issue.Activity = fixture.activity
			app.selectedIssue = issue
			app.updateDetailsView()

			for _, width := range []int{8, 10, 14, 20, 40, 90, 140} {
				drawComments(t, app, width)
				measure, _ := readingMeasure(width)
				for i, line := range strings.Split(app.detailsPageView.GetText(false), "\n") {
					if got := tview.TaggedStringWidth(line); got > measure {
						t.Errorf("width %d: line %d is %d cells in a %d pane: %q", width, i, got, measure, line)
					}
				}
			}
		})
	}
}
