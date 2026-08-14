package tui

import (
	"context"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// skimTestIssues is enough rows to hold j down over.
func skimTestIssues() []linearapi.Issue {
	return []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha", State: "Todo"},
		{ID: "issue-2", Identifier: "LIN-2", Title: "Beta", State: "Todo"},
		{ID: "issue-3", Identifier: "LIN-3", Title: "Gamma", State: "Todo"},
		{ID: "issue-4", Identifier: "LIN-4", Title: "Delta", State: "Todo"},
		{ID: "issue-5", Identifier: "LIN-5", Title: "Epsilon", State: "Todo"},
	}
}

// pressInIssuesTable drives the real table input capture, so the test exercises
// the path a keypress takes rather than calling the handler directly. It runs
// through QueueUpdateDraw because tview handles keys on the event loop, and the
// debounce callback lands there too; driving it from the test goroutine instead
// would race the timer over state neither side locks in production.
func pressInIssuesTable(app *App, key tcell.Key, r rune) {
	app.QueueUpdateDraw(func() {
		handler := app.tableForSection(IssuesSectionList).InputHandler()
		handler(tcell.NewEventKey(key, r, tcell.ModNone), func(tview.Primitive) {})
	})
}

// recordDetailFetches replaces the detail fetch with one that reports every id
// it is asked for, so a test can count what a skim actually sent.
func recordDetailFetches(app *App) <-chan string {
	fetched := make(chan string, 16)
	app.fetchIssueByID = func(_ context.Context, id string) (linearapi.Issue, error) {
		fetched <- id
		return linearapi.Issue{ID: id, Identifier: "FETCHED"}, nil
	}
	return fetched
}

func TestSkimmingIssuesFiresOneDetailFetch(t *testing.T) {
	app, _ := newIssueUpdateTestApp(t, skimTestIssues())
	app.detailDebounce = 80 * time.Millisecond
	fetched := recordDetailFetches(app)

	for range 4 {
		pressInIssuesTable(app, tcell.KeyRune, 'j')
	}

	select {
	case id := <-fetched:
		t.Fatalf("skimming fetched %s inside the debounce window", id)
	case <-time.After(30 * time.Millisecond):
	}

	select {
	case id := <-fetched:
		if id != "issue-5" {
			t.Fatalf("fetched %s, want issue-5, the row the skim landed on", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the skim never fetched the row it landed on")
	}

	select {
	case id := <-fetched:
		t.Fatalf("a second fetch for %s: the skim should collapse to one", id)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestSelectionUpdatesWhileTheDetailsPaneLags(t *testing.T) {
	app, _ := newIssueUpdateTestApp(t, skimTestIssues())
	app.detailDebounce = time.Hour // keep the debounce from firing mid-test
	recordDetailFetches(app)

	pressInIssuesTable(app, tcell.KeyRune, 'j')

	// Commands read selectedIssue the moment a key lands, so the debounce must
	// not defer the selection itself.
	selected := app.GetSelectedIssue()
	if selected == nil || selected.ID != "issue-2" {
		t.Fatalf("selected issue after one j = %#v, want issue-2", selected)
	}
}

func TestSupersededDetailFetchIsCanceled(t *testing.T) {
	app, _ := newIssueUpdateTestApp(t, skimTestIssues())
	app.detailDebounce = time.Millisecond
	contexts := make(chan context.Context, 8)
	app.fetchIssueByID = func(ctx context.Context, id string) (linearapi.Issue, error) {
		contexts <- ctx
		<-ctx.Done()
		return linearapi.Issue{}, ctx.Err()
	}

	pressInIssuesTable(app, tcell.KeyRune, 'j')
	var first context.Context
	select {
	case first = <-contexts:
	case <-time.After(2 * time.Second):
		t.Fatal("the first selection never fetched")
	}

	pressInIssuesTable(app, tcell.KeyRune, 'j')

	select {
	case <-first.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("a superseded detail fetch was left running against the API")
	}
}

func TestLateDetailResultIsDiscarded(t *testing.T) {
	app, _ := newIssueUpdateTestApp(t, skimTestIssues())
	app.detailDebounce = time.Millisecond
	release := make(chan struct{})
	started := make(chan string, 8)
	app.fetchIssueByID = func(_ context.Context, id string) (linearapi.Issue, error) {
		started <- id
		if id == "issue-2" {
			<-release
		}
		return linearapi.Issue{ID: id, Identifier: "FETCHED-" + id}, nil
	}

	pressInIssuesTable(app, tcell.KeyRune, 'j')
	waitForFetch(t, started, "issue-2")
	pressInIssuesTable(app, tcell.KeyRune, 'j')
	waitForFetch(t, started, "issue-3")

	// The older fetch returns last. Its result describes a row the cursor left.
	close(release)
	waitForCondition(t, 2*time.Second, func() bool {
		selected := app.GetSelectedIssue()
		return selected != nil && selected.Identifier == "FETCHED-issue-3"
	})

	time.Sleep(50 * time.Millisecond)
	if selected := app.GetSelectedIssue(); selected == nil || selected.ID != "issue-3" {
		t.Fatalf("selected issue = %#v, want issue-3", selected)
	}
}

// TestCanceledFetchCannotClobberANewerSelection covers the gap between the two
// generations: moving the cursor cancels the in-flight fetch during the debounce
// window, before the next load runs. A cancel that did not also invalidate would
// let the dead request's result land on top of the row the cursor moved to.
func TestCanceledFetchCannotClobberANewerSelection(t *testing.T) {
	app, _ := newIssueUpdateTestApp(t, skimTestIssues())
	app.detailDebounce = time.Hour // the second load must not mask the first
	release := make(chan struct{})
	started := make(chan string, 4)
	app.fetchIssueByID = func(_ context.Context, id string) (linearapi.Issue, error) {
		started <- id
		<-release
		// Deliberately ignores cancellation: the response can already be on the
		// wire when the cursor moves.
		return linearapi.Issue{ID: id, Identifier: "STALE"}, nil
	}

	pressInIssuesTable(app, tcell.KeyEnter, 0)
	waitForFetch(t, started, "issue-1")
	pressInIssuesTable(app, tcell.KeyRune, 'j')
	close(release)

	time.Sleep(100 * time.Millisecond)
	if selected := app.GetSelectedIssue(); selected == nil || selected.ID != "issue-2" {
		t.Fatalf("selected issue = %#v, want issue-2: a canceled fetch overwrote the newer selection", selected)
	}
}

// TestReselectingAnIssueKeepsItsFetchedDetail covers the tab switch that lands
// back on the issue already showing. The list model carries no comments,
// relations, subscribers, or attachments, so taking its copy wholesale empties
// those rows out of the pane until the refetch returns.
func TestReselectingAnIssueKeepsItsFetchedDetail(t *testing.T) {
	app, _ := newIssueUpdateTestApp(t, skimTestIssues())
	app.detailDebounce = time.Hour // a refetch must not paper over the strip
	hydrated := make(chan struct{}, 1)
	app.fetchIssueByID = func(_ context.Context, id string) (linearapi.Issue, error) {
		defer func() { hydrated <- struct{}{} }()
		return linearapi.Issue{
			ID:          id,
			Identifier:  "LIN-1",
			Comments:    []linearapi.Comment{{ID: "comment-1", Body: "hi"}},
			Subscribers: []linearapi.User{{ID: "user-1", Name: "Drew"}},
		}, nil
	}

	pressInIssuesTable(app, tcell.KeyEnter, 0)
	<-hydrated
	waitForCondition(t, 2*time.Second, func() bool {
		selected := app.GetSelectedIssue()
		return selected != nil && len(selected.Subscribers) == 1
	})

	// A tab switch reselects the same row from the list model.
	app.QueueUpdateDraw(func() { app.jumpToSection(IssuesSectionList, 1) })

	selected := app.GetSelectedIssue()
	if selected == nil || len(selected.Subscribers) != 1 || len(selected.Comments) != 1 {
		t.Fatalf("reselected issue = %#v, want its subscribers and comments carried over", selected)
	}
}

// TestPostMutationRefetchCannotRetargetTheSelection covers the callers that
// capture an issue id, run a mutation, and only then reload the detail. The
// cursor can outrun that round trip, and the reload must not drag the pane and
// GetSelectedIssue back to the issue the user left.
func TestPostMutationRefetchCannotRetargetTheSelection(t *testing.T) {
	app, _ := newIssueUpdateTestApp(t, skimTestIssues())
	app.detailDebounce = time.Hour
	release := make(chan struct{})
	started := make(chan string, 4)
	app.fetchIssueByID = func(_ context.Context, id string) (linearapi.Issue, error) {
		started <- id
		<-release
		return linearapi.Issue{ID: id, Identifier: "REFETCHED"}, nil
	}

	// The cursor moves off issue-1 while a mutation on it is still in flight.
	pressInIssuesTable(app, tcell.KeyRune, 'j')

	// The mutation lands afterwards and asks for issue-1's detail back. It
	// takes a fresh generation, so nothing else can invalidate it.
	app.QueueUpdateDraw(func() { app.loadIssueDetailsByID("issue-1") })
	waitForFetch(t, started, "issue-1")
	close(release)

	time.Sleep(100 * time.Millisecond)
	if selected := app.GetSelectedIssue(); selected == nil || selected.ID != "issue-2" {
		t.Fatalf("selected issue = %#v, want issue-2: a post-mutation refetch retargeted the selection", selected)
	}
}

func TestLandingOnAnEmptySectionDropsThePendingLoad(t *testing.T) {
	app, _ := newIssueUpdateTestApp(t, skimTestIssues())
	app.detailDebounce = 40 * time.Millisecond
	fetched := recordDetailFetches(app)

	pressInIssuesTable(app, tcell.KeyRune, 'j')
	// Search has no results, so the section is empty and the selection drops.
	app.QueueUpdateDraw(func() { app.jumpToSection(IssuesSectionSearch, 0) })

	select {
	case id := <-fetched:
		t.Fatalf("fetched %s for a list the cursor left", id)
	case <-time.After(200 * time.Millisecond):
	}
	if selected := app.GetSelectedIssue(); selected != nil {
		t.Fatalf("selected issue on an empty section = %#v, want none", selected)
	}
}

func TestEnterLoadsDetailsWithoutWaiting(t *testing.T) {
	app, _ := newIssueUpdateTestApp(t, skimTestIssues())
	app.detailDebounce = time.Hour // only an immediate load can fire
	fetched := recordDetailFetches(app)

	pressInIssuesTable(app, tcell.KeyEnter, 0)

	select {
	case id := <-fetched:
		if id != "issue-1" {
			t.Fatalf("Enter fetched %s, want issue-1", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Enter left the details pane waiting on the debounce")
	}
}

// TestMergeCommentsHoldsOnlyWhatTheFetchCouldNotSee covers both halves of the
// merge at once: a comment written while the request was out has to survive,
// and a comment the request could have carried and did not is one somebody
// deleted. Folding that second one back is what used to leave a deleted comment
// on screen until a restart.
func TestMergeCommentsHoldsOnlyWhatTheFetchCouldNotSee(t *testing.T) {
	since := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	comment := func(id string, at time.Time) linearapi.Comment {
		return linearapi.Comment{ID: id, Body: id, CreatedAt: at}
	}
	older := comment("older", since.Add(-time.Hour))
	newer := comment("newer", since.Add(time.Second))

	tests := []struct {
		name    string
		fetched []linearapi.Comment
		held    []linearapi.Comment
		want    []string
	}{
		{
			name:    "a comment posted while the fetch was out survives",
			fetched: []linearapi.Comment{older},
			held:    []linearapi.Comment{older, newer},
			want:    []string{"older", "newer"},
		},
		{
			name:    "a comment the fetch dropped stays gone",
			fetched: nil,
			held:    []linearapi.Comment{older},
			want:    nil,
		},
		{
			name:    "the fetched copy wins over the held one",
			fetched: []linearapi.Comment{{ID: "older", Body: "edited elsewhere", CreatedAt: older.CreatedAt}},
			held:    []linearapi.Comment{older},
			want:    []string{"edited elsewhere"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeComments(tt.fetched, tt.held, since)
			if len(got) != len(tt.want) {
				t.Fatalf("merged %d comments, want %d: %+v", len(got), len(tt.want), got)
			}
			for i, body := range tt.want {
				if got[i].Body != body {
					t.Errorf("comment %d body = %q, want %q", i, got[i].Body, body)
				}
			}
		})
	}
}

func waitForFetch(t *testing.T, started <-chan string, want string) {
	t.Helper()
	select {
	case got := <-started:
		if got != want {
			t.Fatalf("fetched %s, want %s", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("no fetch for %s", want)
	}
}
