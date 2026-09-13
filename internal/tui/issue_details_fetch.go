package tui

import (
	"context"
	"time"

	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
)

const defaultDetailDebounce = 120 * time.Millisecond

func (a *App) onIssueSelected(issue linearapi.Issue) {
	logger.Debug("tui.issue_details_fetch: issue selected issue=%s", issue.Identifier)
	a.setSelectedIssue(issue)
	a.scheduleDetailDebounce(issue)
}

func (a *App) selectIssueNow(issue linearapi.Issue) {
	logger.Debug("tui.issue_details_fetch: issue selected issue=%s", issue.Identifier)
	a.setSelectedIssue(issue)
	a.cancelDetailDebounce()
	a.loadIssueDetails(issue)
}

func (a *App) setSelectedIssue(issue linearapi.Issue) {
	a.issuesMu.Lock()
	defer a.issuesMu.Unlock()
	if a.selectedIssue != nil && a.selectedIssue.ID == issue.ID {
		issue.Comments = a.selectedIssue.Comments
		issue.Activity = a.selectedIssue.Activity
		issue.Relations = a.selectedIssue.Relations
		issue.Subscribers = a.selectedIssue.Subscribers
		issue.Attachments = a.selectedIssue.Attachments
	}
	a.selectedIssue = &issue
}

func (a *App) detailDebounceDelay() time.Duration {
	if a.detailDebounce > 0 {
		return a.detailDebounce
	}
	return defaultDetailDebounce
}

func (a *App) scheduleDetailDebounce(issue linearapi.Issue) {
	generation := a.detailDebounceGeneration.Add(1)
	a.cancelDetailFetch()

	a.detailDebounceMu.Lock()
	if a.detailDebounceTimer != nil {
		a.detailDebounceTimer.Stop()
	}
	a.detailDebounceTimer = time.AfterFunc(a.detailDebounceDelay(), func() {
		if generation != a.detailDebounceGeneration.Load() {
			return
		}
		a.QueueUpdateDraw(func() {
			if generation != a.detailDebounceGeneration.Load() {
				return
			}
			a.loadIssueDetails(issue)
		})
	})
	a.detailDebounceMu.Unlock()
}

func (a *App) cancelDetailDebounce() {
	a.detailDebounceGeneration.Add(1)

	a.detailDebounceMu.Lock()
	if a.detailDebounceTimer != nil {
		a.detailDebounceTimer.Stop()
		a.detailDebounceTimer = nil
	}
	a.detailDebounceMu.Unlock()
}

func (a *App) loadIssueDetails(issue linearapi.Issue) {
	a.updateDetailsView()
	a.loadIssueDetailsByID(issue.ID)
}

func (a *App) loadIssueDetailsByID(issueID string) {
	a.cancelDetailFetch()
	generation := a.detailFetchGeneration.Load()

	fetchIssue := a.fetchIssueByID
	if fetchIssue == nil {
		fetchIssue = a.api.FetchIssueByID
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.detailFetchCancel = cancel
	requestedAt := time.Now()
	go func() {
		defer cancel()
		logger.Debug("tui.issue_details_fetch: fetching full issue details issue_id=%s", issueID)
		fullIssue, err := fetchIssue(ctx, issueID)
		a.QueueUpdateDraw(func() {
			if generation != a.detailFetchGeneration.Load() {
				return
			}
			if selected := a.GetSelectedIssue(); selected == nil || selected.ID != issueID {
				return
			}
			if err != nil {
				logger.ErrorWithErr(err, "tui.issue_details_fetch: failed to fetch full issue details issue_id=%s", issueID)
				return
			}
			a.issuesMu.Lock()
			if a.selectedIssue != nil {
				fullIssue.Comments = mergeComments(fullIssue.Comments, a.selectedIssue.Comments, requestedAt)
			}
			a.selectedIssue = &fullIssue
			a.issuesMu.Unlock()
			a.updateDetailsView()
		})
	}()
}

func mergeComments(fetched, held []linearapi.Comment, since time.Time) []linearapi.Comment {
	if len(held) == 0 {
		return fetched
	}
	known := make(map[string]struct{}, len(fetched))
	for _, comment := range fetched {
		known[comment.ID] = struct{}{}
	}
	for _, comment := range held {
		if _, ok := known[comment.ID]; ok || comment.CreatedAt.Before(since) {
			continue
		}
		fetched = insertCommentInOrder(fetched, comment)
	}
	return fetched
}

func (a *App) cancelDetailFetch() {
	a.detailFetchGeneration.Add(1)
	if a.detailFetchCancel != nil {
		a.detailFetchCancel()
		a.detailFetchCancel = nil
	}
}

func (a *App) abandonDetailFetch() {
	a.cancelDetailDebounce()
	a.cancelDetailFetch()
}
