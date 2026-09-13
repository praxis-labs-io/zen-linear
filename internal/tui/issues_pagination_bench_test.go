package tui

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/praxis-labs-io/zen-linear/internal/config"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
)

func benchIssues(count int) []linearapi.Issue {
	states := []string{"Todo", "In Progress", "In Review", "Backlog", "Done"}
	assignees := []string{benchUserID, "user-2", "user-3", "user-4"}
	issues := make([]linearapi.Issue, count)
	for i := range issues {
		issues[i] = linearapi.Issue{
			ID:         fmt.Sprintf("issue-%d", i),
			Identifier: fmt.Sprintf("ZNL-%d", i),
			Title:      fmt.Sprintf("Issue %d: a title of roughly the length these actually run to", i),
			State:      states[i%len(states)],
			Priority:   i % 5,
			AssigneeID: assignees[i%len(assignees)],
		}
	}
	return issues
}

const benchUserID = "user-1"

func newPaginationBenchApp(b *testing.B) *App {
	b.Helper()
	app := NewApp(linearapi.ClientConfig{}, config.Config{PageSize: 50, CacheTTL: time.Minute}, nil)
	app.queueUpdateDraw = func(f func()) { f() }
	app.currentUser = &linearapi.User{ID: benchUserID, Name: "Bench User"}
	app.fetchIssueByID = func(_ context.Context, id string) (linearapi.Issue, error) {
		return linearapi.Issue{ID: id}, nil
	}
	return app
}

func benchmarkPagination(b *testing.B, count, repaintEvery int) {
	const pageSize = 50
	issues := benchIssues(count)
	app := newPaginationBenchApp(b)

	b.ResetTimer()
	for b.Loop() {
		app.issuesMu.Lock()
		app.issues = nil
		app.selectedIssue = nil
		app.issuesMu.Unlock()

		merge := &pageMerge{seen: make(map[string]bool, count)}
		app.updateIssuesData(issues[:pageSize])
		app.issuesMu.RLock()
		merge.reset(app.issues)
		app.issuesMu.RUnlock()

		page := 0
		for start := pageSize; start < count; start += pageSize {
			end := min(start+pageSize, count)
			app.accumulateIssues(issues[start:end], merge)
			page++
			if repaintEvery > 0 && page%repaintEvery == 0 {
				app.renderAccumulatedIssues()
			}
		}
		app.renderAccumulatedIssues()
	}
}

func BenchmarkPagination2000PaintOnce(b *testing.B)     { benchmarkPagination(b, 2000, 0) }
func BenchmarkPagination2000PaintBudgeted(b *testing.B) { benchmarkPagination(b, 2000, 12) }
func BenchmarkPagination2000PaintPerPage(b *testing.B)  { benchmarkPagination(b, 2000, 1) }
func BenchmarkPagination5000PaintOnce(b *testing.B)     { benchmarkPagination(b, 5000, 0) }
func BenchmarkPagination5000PaintBudgeted(b *testing.B) { benchmarkPagination(b, 5000, 12) }
func BenchmarkPagination5000PaintPerPage(b *testing.B)  { benchmarkPagination(b, 5000, 1) }
