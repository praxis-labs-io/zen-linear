package tui

import (
	"context"
	"time"

	"github.com/zen-linear/zen-linear/internal/linearapi"
	"github.com/zen-linear/zen-linear/internal/logger"
)

// defaultDetailDebounce bounds how far the details pane lags the cursor.
// Skimming with j/k must not render markdown or hit the API once per row.
const defaultDetailDebounce = 120 * time.Millisecond

// onIssueSelected records a selection made by moving the cursor. The pane render
// and the detail fetch are both deferred; the selection itself is not, because
// the commands read a.selectedIssue the moment a key lands.
func (a *App) onIssueSelected(issue linearapi.Issue) {
	logger.Debug("tui.issue_details_fetch: issue selected issue=%s", issue.Identifier)
	a.setSelectedIssue(issue)
	a.scheduleDetailDebounce(issue)
}

// selectIssueNow selects an issue and loads its details without waiting out the
// debounce, for the deliberate moves where the pane is expected to be current:
// opening it with Enter, or landing on a new issue after an edit.
func (a *App) selectIssueNow(issue linearapi.Issue) {
	logger.Debug("tui.issue_details_fetch: issue selected issue=%s", issue.Identifier)
	a.setSelectedIssue(issue)
	a.cancelDetailDebounce()
	a.loadIssueDetails(issue)
}

// setSelectedIssue takes the list model's copy of an issue, carrying over the
// connections only the detail fetch selects. Assigning the list copy straight
// over a hydrated selection empties comments, activity, relations, subscribers,
// and attachments out of the pane until the refetch lands, which reads as a
// flicker every time a tab switch reselects the issue already on screen.
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

// scheduleDetailDebounce defers the render and the fetch for the current
// selection, mirroring the search path.
func (a *App) scheduleDetailDebounce(issue linearapi.Issue) {
	generation := a.detailDebounceGeneration.Add(1)
	// The selection moved, so anything in flight is already stale. Drop it now
	// rather than a debounce window later, or a held j leaves one full detail
	// query per row running against the API.
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

// cancelDetailDebounce drops a pending render and fetch. UI thread only, like
// the rest of the details path.
func (a *App) cancelDetailDebounce() {
	a.detailDebounceGeneration.Add(1)

	a.detailDebounceMu.Lock()
	if a.detailDebounceTimer != nil {
		a.detailDebounceTimer.Stop()
		a.detailDebounceTimer = nil
	}
	a.detailDebounceMu.Unlock()
}

// loadIssueDetails renders what the list already carries, then fetches the
// nested connections it does not: comments, relations, subscribers, attachments.
func (a *App) loadIssueDetails(issue linearapi.Issue) {
	a.updateDetailsView()
	a.loadIssueDetailsByID(issue.ID)
}

// loadIssueDetailsByID fetches one issue's full detail and applies it only if no
// newer selection has taken over. Cancels the request in flight when superseded,
// so a discarded result is not also a connection held for 30 seconds.
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
			// The post-mutation callers capture an id before a round trip the
			// cursor can outrun, and every path that repoints the list writes
			// selectedIssue without touching the generation. The selection
			// itself is the only reliable answer to who owns the pane.
			if selected := a.GetSelectedIssue(); selected == nil || selected.ID != issueID {
				return
			}
			if err != nil {
				logger.ErrorWithErr(err, "tui.issue_details_fetch: failed to fetch full issue details issue_id=%s", issueID)
				// Keep the partial issue data we already have.
				return
			}
			a.issuesMu.Lock()
			// A comment posted while this fetch was out is not in what came
			// back, and the id guard above cannot see it: posting does not move
			// the selection. Assigning wholesale would take the card off screen
			// again a moment after it landed.
			if a.selectedIssue != nil {
				fullIssue.Comments = mergeComments(fullIssue.Comments, a.selectedIssue.Comments, requestedAt)
			}
			a.selectedIssue = &fullIssue
			a.issuesMu.Unlock()
			a.updateDetailsView()
		})
	}()
}

// mergeComments folds back the comments the fetch could not have seen, in
// timestamp order. since is when the request went out, so a held comment newer
// than that is one written after the server answered the question.
//
// Anything older is the fetch's to answer for. A held comment absent from a
// result that could have carried it is a comment somebody deleted, and folding
// that one back is what used to leave it on screen until a restart. Held
// comments the fetch did return are matched by id and the fetched copy wins, so
// an edit made elsewhere still lands.
//
// since is this machine's clock and CreatedAt is Linear's. A skew wider than
// the round trip holds a deleted comment for one refresh, or drops a
// just-posted one for one refresh; the next fetch settles either.
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

// cancelDetailFetch stops the in-flight detail request and invalidates it in one
// step. Canceling without the bump leaves the dead goroutine's generation intact,
// so its result lands on top of the row the cursor moved to.
func (a *App) cancelDetailFetch() {
	a.detailFetchGeneration.Add(1)
	if a.detailFetchCancel != nil {
		a.detailFetchCancel()
		a.detailFetchCancel = nil
	}
}

// abandonDetailFetch invalidates both the pending and the in-flight detail work,
// for the paths that drop the selection entirely.
func (a *App) abandonDetailFetch() {
	a.cancelDetailDebounce()
	a.cancelDetailFetch()
}
