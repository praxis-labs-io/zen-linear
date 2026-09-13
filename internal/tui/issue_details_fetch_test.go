package tui

import (
	"context"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/rivo/tview"
)

func skimTestIssues() []linearapi.Issue {
	return []linearapi.Issue{
		{ID: "issue-1", Identifier: "LIN-1", Title: "Alpha", State: "Todo"},
		{ID: "issue-2", Identifier: "LIN-2", Title: "Beta", State: "Todo"},
		{ID: "issue-3", Identifier: "LIN-3", Title: "Gamma", State: "Todo"},
		{ID: "issue-4", Identifier: "LIN-4", Title: "Delta", State: "Todo"},
		{ID: "issue-5", Identifier: "LIN-5", Title: "Epsilon", State: "Todo"},
	}
}

func pressInIssuesTable(app *App, key tcell.Key, r rune) {
	app.QueueUpdateDraw(func() {
		handler := app.tableForSection(IssuesSectionList).InputHandler()
		handler(tcell.NewEventKey(key, r, tcell.ModNone), func(tview.Primitive) {})
	})
}

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
	app.detailDebounce = time.Hour
	recordDetailFetches(app)

	pressInIssuesTable(app, tcell.KeyRune, 'j')

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

func TestCanceledFetchCannotClobberANewerSelection(t *testing.T) {
	app, _ := newIssueUpdateTestApp(t, skimTestIssues())
	app.detailDebounce = time.Hour
	release := make(chan struct{})
	started := make(chan string, 4)
	app.fetchIssueByID = func(_ context.Context, id string) (linearapi.Issue, error) {
		started <- id
		<-release
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

func TestReselectingAnIssueKeepsItsFetchedDetail(t *testing.T) {
	app, _ := newIssueUpdateTestApp(t, skimTestIssues())
	app.detailDebounce = time.Hour
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

	app.QueueUpdateDraw(func() { app.jumpToSection(IssuesSectionList, 1) })

	selected := app.GetSelectedIssue()
	if selected == nil || len(selected.Subscribers) != 1 || len(selected.Comments) != 1 {
		t.Fatalf("reselected issue = %#v, want its subscribers and comments carried over", selected)
	}
}

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

	pressInIssuesTable(app, tcell.KeyRune, 'j')

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
	app.detailDebounce = time.Hour
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
