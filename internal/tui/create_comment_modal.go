package tui

import (
	"context"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/roeyazroel/linear-tui/internal/linearapi"
	"github.com/roeyazroel/linear-tui/internal/logger"
)

// CreateCommentModal manages the create comment form overlay.
type CreateCommentModal struct {
	app       *App
	fm        *FormModal
	bodyField *tview.TextArea
	issueID   string
	onCreate  func(issueID, body string)
}

// NewCreateCommentModal creates a new create comment modal.
func NewCreateCommentModal(app *App) *CreateCommentModal {
	ccm := &CreateCommentModal{app: app}
	ccm.fm = NewFormModal(app, "New Comment")
	ccm.bodyField = ccm.fm.AddTextArea("Comment", "", 5)
	submit := func() {
		body := ccm.bodyField.GetText()
		ccm.Hide()
		if ccm.onCreate != nil && body != "" {
			ccm.onCreate(ccm.issueID, body)
		}
	}
	ccm.fm.AddButtons(
		FormButton{Label: "Comment", OnPress: submit},
		FormButton{Label: "Cancel", OnPress: ccm.Hide},
	)
	ccm.fm.SetOnSubmit(submit)
	ccm.fm.SetOnCancel(ccm.Hide)
	ccm.fm.SetHint("Esc cancel · ⌃⏎ submit")
	return ccm
}

// Show displays the create comment modal. contextLine names the issue above
// the field.
func (ccm *CreateCommentModal) Show(issueID, contextLine string, onCreate func(issueID, body string)) {
	ccm.issueID = issueID
	ccm.onCreate = onCreate
	ccm.fm.SetContext(contextLine)
	ccm.bodyField.SetText("", true)
	ccm.fm.Show("create_comment")
}

// Hide hides the create comment modal.
func (ccm *CreateCommentModal) Hide() {
	ccm.fm.Hide("create_comment")
}

// HandleKey handles keyboard input for the create comment modal.
func (ccm *CreateCommentModal) HandleKey(event *tcell.EventKey) *tcell.EventKey {
	return ccm.fm.HandleKey(event)
}

// GetModal returns the modal flex for adding to pages.
func (ccm *CreateCommentModal) GetModal() *tview.Flex {
	return ccm.fm.Root()
}

// handleCreateComment handles comment creation.
func (a *App) handleCreateComment(issueID, body string) {
	go func() {
		ctx := context.Background()
		_, err := a.GetAPI().CreateComment(ctx, linearapi.CreateCommentInput{
			IssueID: issueID,
			Body:    body,
		})

		a.app.QueueUpdateDraw(func() {
			if err != nil {
				logger.ErrorWithErr(err, "tui.app: failed to create comment issue=%s", issueID)
				a.updateStatusBarWithError(err)
				return
			}

			logger.Info("tui.app: created comment issue=%s", issueID)
			a.flashStatus("Comment added")

			// Refresh the selected issue to show the new comment
			a.issuesMu.RLock()
			selectedIssue := a.selectedIssue
			a.issuesMu.RUnlock()
			if selectedIssue != nil && selectedIssue.ID == issueID {
				a.fetchingIssueID = issueID
				go func() {
					fullIssue, fetchErr := a.api.FetchIssueByID(ctx, issueID)
					a.app.QueueUpdateDraw(func() {
						if a.fetchingIssueID == issueID {
							if fetchErr != nil {
								logger.ErrorWithErr(fetchErr, "tui.app: failed to refresh issue after comment creation issue=%s", issueID)
								return
							}
							a.issuesMu.Lock()
							a.selectedIssue = &fullIssue
							a.issuesMu.Unlock()
							a.updateDetailsView()
						}
					})
				}()
			}
		})
	}()
}
