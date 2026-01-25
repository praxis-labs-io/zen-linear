package tui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/roeyazroel/linear-tui/internal/linearapi"
	"github.com/roeyazroel/linear-tui/internal/logger"
)

// CreateIssueModal manages the create issue form overlay.
type CreateIssueModal struct {
	app           *App
	modal         *tview.Flex
	form          *tview.Form
	assigneeField *tview.DropDown
	priorityField *tview.DropDown
	teamID        string
	projectID     string
	assigneeID    string
	assigneeName  string
	priority      int
	priorityLabel string
	onCreate      func(title, description, teamID, projectID, assigneeID string, priority int)
	cachedUsers   []struct{ ID, Name string }
}

// NewCreateIssueModal creates a new create issue modal.
func NewCreateIssueModal(app *App) *CreateIssueModal {
	cm := &CreateIssueModal{
		app:      app,
		priority: 3, // Default: Normal
	}

	// Create form
	cm.form = tview.NewForm()
	cm.form.SetBackgroundColor(app.theme.HeaderBg)
	cm.form.SetFieldBackgroundColor(app.theme.InputBg)
	cm.form.SetFieldTextColor(app.theme.Foreground)
	cm.form.SetButtonBackgroundColor(app.theme.Accent)
	cm.form.SetButtonTextColor(app.theme.SelectionText)
	cm.form.SetLabelColor(app.theme.Foreground)

	// Add title field
	cm.form.AddInputField("Title", "", 60, nil, nil)

	// Add description field
	cm.form.AddTextArea("Description", "", 60, 4, 0, nil)

	// Add assignee dropdown - will be populated when shown
	cm.form.AddDropDown("Assignee", []string{"Unassigned"}, 0, func(_ string, index int) {
		if index == 0 {
			cm.assigneeID = ""
			cm.assigneeName = ""
		} else if index > 0 && index <= len(cm.cachedUsers) {
			user := cm.cachedUsers[index-1]
			cm.assigneeID = user.ID
			cm.assigneeName = user.Name
		}
	})
	// Get the dropdown and style it
	if item := cm.form.GetFormItemByLabel("Assignee"); item != nil {
		if dropdown, ok := item.(*tview.DropDown); ok {
			cm.assigneeField = dropdown
		}
	}
	cm.assigneeField.SetFieldWidth(50)
	cm.assigneeField.SetListStyles(
		tcell.StyleDefault.Background(app.theme.HeaderBg).Foreground(app.theme.Foreground),
		tcell.StyleDefault.Background(app.theme.Accent).Foreground(app.theme.SelectionText),
	)

	// Add priority dropdown with all options
	priorities := []string{"No priority", "Urgent", "High", "Normal", "Low"}
	cm.form.AddDropDown("Priority", priorities, 3, func(option string, index int) {
		cm.priority = index
		cm.priorityLabel = option
	})
	// Get the dropdown and style it
	if item := cm.form.GetFormItemByLabel("Priority"); item != nil {
		if dropdown, ok := item.(*tview.DropDown); ok {
			cm.priorityField = dropdown
		}
	}
	cm.priorityField.SetFieldWidth(50)
	cm.priorityField.SetListStyles(
		tcell.StyleDefault.Background(app.theme.HeaderBg).Foreground(app.theme.Foreground),
		tcell.StyleDefault.Background(app.theme.Accent).Foreground(app.theme.SelectionText),
	)

	// Add action buttons
	cm.form.AddButton("Create", func() {
		var title, desc string
		if titleItem := cm.form.GetFormItemByLabel("Title"); titleItem != nil {
			if inputField, ok := titleItem.(*tview.InputField); ok {
				title = inputField.GetText()
			}
		}
		if descItem := cm.form.GetFormItemByLabel("Description"); descItem != nil {
			if textArea, ok := descItem.(*tview.TextArea); ok {
				desc = textArea.GetText()
			}
		}
		cm.Hide()
		if cm.onCreate != nil && title != "" {
			cm.onCreate(title, desc, cm.teamID, cm.projectID, cm.assigneeID, cm.priority)
		}
	})
	cm.form.AddButton("Cancel", func() {
		cm.Hide()
	})

	// Create header with instructions
	headerView := tview.NewTextView()
	headerView.SetText("Create New Issue")
	headerView.SetTextColor(app.theme.Accent)
	headerView.SetBackgroundColor(app.theme.HeaderBg)

	// Create help text
	helpView := tview.NewTextView()
	helpView.SetText("Tab: next field • Enter: open dropdown • Esc: cancel")
	helpView.SetTextColor(app.theme.SecondaryText)
	helpView.SetBackgroundColor(app.theme.HeaderBg)
	helpView.SetTextAlign(tview.AlignCenter)

	// Build modal content
	modalContent := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(headerView, 1, 0, false).
		AddItem(cm.form, 0, 1, true).
		AddItem(helpView, 1, 0, false)
	modalContent.Box = tview.NewBox().SetBackgroundColor(app.theme.HeaderBg)
	modalContent.SetBackgroundColor(app.theme.HeaderBg).
		SetBorder(true).
		SetBorderColor(app.theme.Accent).
		SetTitle(" New Issue ").
		SetTitleColor(app.theme.Foreground)
	padding := app.density.ModalPadding
	modalContent.SetBorderPadding(padding.Top, padding.Bottom, padding.Left, padding.Right)

	// Center the modal on screen
	cm.modal = tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().
			SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(modalContent, 18, 0, true).
			AddItem(nil, 0, 1, false), 75, 0, true).
		AddItem(nil, 0, 1, false)
	cm.modal.SetBackgroundColor(app.theme.Background)

	return cm
}

// Show displays the create issue modal.
func (cm *CreateIssueModal) Show(teamID, projectID string, onCreate func(title, description, teamID, projectID, assigneeID string, priority int)) {
	logger.Debug("tui.create_issue: showing create issue modal team_id=%s project_id=%s", teamID, projectID)
	cm.teamID = teamID
	cm.projectID = projectID
	cm.onCreate = onCreate

	// Reset form fields
	if titleItem := cm.form.GetFormItemByLabel("Title"); titleItem != nil {
		if inputField, ok := titleItem.(*tview.InputField); ok {
			_ = inputField.SetText("")
		}
	}
	if descItem := cm.form.GetFormItemByLabel("Description"); descItem != nil {
		if textArea, ok := descItem.(*tview.TextArea); ok {
			_ = textArea.SetText("", true)
		}
	}

	// Reset selections
	cm.assigneeID = ""
	cm.assigneeName = ""
	cm.assigneeField.SetCurrentOption(0)
	cm.priority = 3 // Default to Normal
	cm.priorityLabel = "Normal"
	cm.priorityField.SetCurrentOption(3)

	// Show modal first with loading state for assignee
	cm.assigneeField.SetOptions([]string{"Loading..."}, nil)
	cm.app.pages.AddPage("create_issue", cm.modal, true, true)
	cm.app.pages.SendToFront("create_issue")
	cm.app.app.SetFocus(cm.form)

	// Load users asynchronously
	cm.loadUsers()
}

// loadUsers fetches team users and populates the assignee dropdown.
func (cm *CreateIssueModal) loadUsers() {
	users := cm.app.GetTeamUsers()
	if len(users) > 0 {
		cm.populateAssigneeDropdown(users)
		return
	}

	// Users not loaded yet, fetch them
	go func() {
		loadedUsers, err := cm.app.FetchTeamUsers(cm.teamID)
		if err != nil {
			cm.app.app.QueueUpdateDraw(func() {
				cm.assigneeField.SetOptions([]string{"Unassigned", "(Failed to load users)"}, nil)
			})
			return
		}
		cm.app.app.QueueUpdateDraw(func() {
			cm.populateAssigneeDropdown(loadedUsers)
		})
	}()
}

// populateAssigneeDropdown fills the assignee dropdown with users.
func (cm *CreateIssueModal) populateAssigneeDropdown(users []linearapi.User) {
	assigneeOptions := []string{"Unassigned"}
	cm.cachedUsers = make([]struct{ ID, Name string }, 0, len(users))
	for _, user := range users {
		displayName := user.Name
		if user.IsMe {
			displayName = fmt.Sprintf("%s (me)", user.Name)
		}
		assigneeOptions = append(assigneeOptions, displayName)
		cm.cachedUsers = append(cm.cachedUsers, struct{ ID, Name string }{user.ID, displayName})
	}
	cm.assigneeField.SetOptions(assigneeOptions, func(_ string, index int) {
		if index == 0 {
			cm.assigneeID = ""
			cm.assigneeName = ""
		} else if index > 0 && index <= len(cm.cachedUsers) {
			user := cm.cachedUsers[index-1]
			cm.assigneeID = user.ID
			cm.assigneeName = user.Name
		}
	})
	cm.assigneeField.SetCurrentOption(0)
}

// Hide hides the create issue modal.
func (cm *CreateIssueModal) Hide() {
	cm.app.pages.RemovePage("create_issue")
	cm.app.updateFocus()
}

// HandleKey handles keyboard input for the create issue modal.
func (cm *CreateIssueModal) HandleKey(event *tcell.EventKey) *tcell.EventKey {
	if event.Key() == tcell.KeyEscape {
		cm.Hide()
		return nil
	}
	return event
}

// GetModal returns the modal flex for adding to pages.
func (cm *CreateIssueModal) GetModal() *tview.Flex {
	return cm.modal
}
