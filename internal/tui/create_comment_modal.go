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
	app        *App
	modal      *tview.Flex
	form       *tview.Form
	bodyField  *tview.TextArea
	issueID    string
	onCreate   func(issueID, body string)
}

// NewCreateCommentModal creates a new create comment modal.
func NewCreateCommentModal(app *App) *CreateCommentModal {
	ccm := &CreateCommentModal{
		app: app,
	}

	// Create form
	ccm.form = tview.NewForm()
	ccm.form.SetBackgroundColor(LinearTheme.HeaderBg)
	ccm.form.SetFieldBackgroundColor(tcell.ColorDarkGray)
	ccm.form.SetFieldTextColor(tcell.ColorWhite)
	ccm.form.SetButtonBackgroundColor(LinearTheme.Accent)
	ccm.form.SetButtonTextColor(tcell.ColorWhite)
	ccm.form.SetLabelColor(LinearTheme.Foreground)

	// Add comment body field
	ccm.form.AddTextArea("Comment", "", 60, 8, 0, nil)
	ccm.bodyField = ccm.form.GetFormItemByLabel("Comment").(*tview.TextArea)

	// Add action buttons
	ccm.form.AddButton("Comment", func() {
		body := ccm.bodyField.GetText()
		ccm.Hide()
		if ccm.onCreate != nil && body != "" {
			ccm.onCreate(ccm.issueID, body)
		}
	})
	ccm.form.AddButton("Cancel", func() {
		ccm.Hide()
	})

	// Create header with instructions
	headerView := tview.NewTextView()
	headerView.SetText("Add Comment")
	headerView.SetTextColor(tcell.ColorYellow)
	headerView.SetBackgroundColor(LinearTheme.HeaderBg)

	// Create help text
	helpView := tview.NewTextView()
	helpView.SetText("Esc: cancel • Ctrl+Enter / Cmd+Enter: submit")
	helpView.SetTextColor(tcell.ColorGray)
	helpView.SetBackgroundColor(LinearTheme.HeaderBg)
	helpView.SetTextAlign(tview.AlignCenter)

	// Build modal content
	modalContent := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(headerView, 1, 0, false).
		AddItem(ccm.form, 0, 1, true).
		AddItem(helpView, 1, 0, false)
	modalContent.SetBackgroundColor(LinearTheme.HeaderBg).
		SetBorder(true).
		SetBorderColor(LinearTheme.Accent).
		SetTitle(" New Comment ").
		SetTitleColor(tcell.ColorWhite)

	// Center the modal on screen
	ccm.modal = tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(modalContent, 12, 0, true).
			AddItem(nil, 0, 1, false), 75, 0, true).
		AddItem(nil, 0, 1, false)
	ccm.modal.SetBackgroundColor(LinearTheme.Background)

	return ccm
}

// Show displays the create comment modal.
func (ccm *CreateCommentModal) Show(issueID string, onCreate func(issueID, body string)) {
	ccm.issueID = issueID
	ccm.onCreate = onCreate

	// Reset form field
	ccm.bodyField.SetText("", true)

	// Show modal
	ccm.app.pages.AddPage("create_comment", ccm.modal, true, true)
	ccm.app.pages.SendToFront("create_comment")
	ccm.app.app.SetFocus(ccm.form)
}

// Hide hides the create comment modal.
func (ccm *CreateCommentModal) Hide() {
	ccm.app.pages.RemovePage("create_comment")
	ccm.app.updateFocus()
}

// HandleKey handles keyboard input for the create comment modal.
func (ccm *CreateCommentModal) HandleKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEscape:
		ccm.Hide()
		return nil
	case tcell.KeyEnter:
		// Check for Ctrl+Enter or Cmd+Enter
		mod := event.Modifiers()
		platformMod := GetPlatformModifier()
		if mod&platformMod != 0 {
			// Submit comment
			body := ccm.bodyField.GetText()
			if body != "" {
				ccm.Hide()
				if ccm.onCreate != nil {
					ccm.onCreate(ccm.issueID, body)
				}
			}
			return nil
		}
	}
	return event
}

// GetModal returns the modal flex for adding to pages.
func (ccm *CreateCommentModal) GetModal() *tview.Flex {
	return ccm.modal
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
				logger.ErrorWithErr(err, "Failed to create comment on issue %s", issueID)
				a.updateStatusBarWithError(err)
				return
			}

			logger.Info("Created comment on issue %s", issueID)

			// Refresh the selected issue to show the new comment
			if a.selectedIssue != nil && a.selectedIssue.ID == issueID {
				a.fetchingIssueID = issueID
				go func() {
					fullIssue, fetchErr := a.api.FetchIssueByID(ctx, issueID)
					a.app.QueueUpdateDraw(func() {
						if a.fetchingIssueID == issueID {
							if fetchErr != nil {
								logger.ErrorWithErr(fetchErr, "Failed to refresh issue after comment creation")
								return
							}
							a.selectedIssue = &fullIssue
							a.updateDetailsView()
						}
					})
				}()
			}
		})
	}()
}
