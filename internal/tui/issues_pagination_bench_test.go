package tui

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/zen-linear/zen-linear/internal/config"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// benchIssues builds a workspace-shaped list: several statuses so grouping has
// real work to do, and titles long enough to exercise rune-width truncation.
func benchIssues(count int) []linearapi.Issue {
	states := []string{"Todo", "In Progress", "In Review", "Backlog", "Done"}
	issues := make([]linearapi.Issue, count)
	for i := range issues {
		issues[i] = linearapi.Issue{
			ID:         fmt.Sprintf("issue-%d", i),
			Identifier: fmt.Sprintf("ZNL-%d", i),
			Title:      fmt.Sprintf("Issue %d: a title of roughly the length these actually run to", i),
			State:      states[i%len(states)],
			Priority:   i % 5,
		}
	}
	return issues
}

func newPaginationBenchApp(b *testing.B) *App {
	b.Helper()
	app := NewApp(&linearapi.Client{}, config.Config{PageSize: 50, CacheTTL: time.Minute}, nil)
	app.queueUpdateDraw = func(f func()) { f() }
	// Selecting a row kicks off a detail fetch; this benchmark measures the
	// table, not the network.
	app.fetchIssueByID = func(_ context.Context, id string) (linearapi.Issue, error) {
		return linearapi.Issue{ID: id}, nil
	}
	return app
}

// benchmarkPagination replays count issues in 50-issue pages. paintPerPage
// selects the pre-ZNL-13 behavior, where every page regrouped and repainted the
// whole table; otherwise pages accumulate and the table paints once.
func benchmarkPagination(b *testing.B, count int, paintPerPage bool) {
	const pageSize = 50
	issues := benchIssues(count)
	app := newPaginationBenchApp(b)

	b.ResetTimer()
	for b.Loop() {
		app.issuesMu.Lock()
		app.issues = nil
		app.selectedIssue = nil
		app.issuesMu.Unlock()

		seen := make(map[string]bool, count)
		app.updateIssuesData(issues[:pageSize])
		app.issuesMu.RLock()
		for i := range app.issues {
			seen[app.issues[i].ID] = true
		}
		app.issuesMu.RUnlock()

		for start := pageSize; start < count; start += pageSize {
			end := min(start+pageSize, count)
			app.accumulateIssues(issues[start:end], seen)
			if paintPerPage {
				app.renderAccumulatedIssues()
			}
		}
		if !paintPerPage {
			app.renderAccumulatedIssues()
		}
	}
}

func BenchmarkPagination2000PaintOnce(b *testing.B)    { benchmarkPagination(b, 2000, false) }
func BenchmarkPagination2000PaintPerPage(b *testing.B) { benchmarkPagination(b, 2000, true) }
func BenchmarkPagination5000PaintOnce(b *testing.B)    { benchmarkPagination(b, 5000, false) }
func BenchmarkPagination5000PaintPerPage(b *testing.B) { benchmarkPagination(b, 5000, true) }
