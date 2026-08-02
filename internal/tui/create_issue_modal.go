package tui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/linearapi"
	"github.com/zen-linear/zen-linear/internal/logger"
)

// CreateIssueModal manages the create issue form overlay.
type CreateIssueModal struct {
	app           *App
	fm            *FormModal
	parentView    *tview.TextView
	titleField    *tview.InputField
	descField     *tview.TextArea
	assigneeField *tview.DropDown
	cycleField    *tview.DropDown
	priorityField *tview.DropDown
	teamID        string
	projectID     string
	assigneeID    string
	assigneeName  string
	cycleID       string
	cycleName     string
	selectedCycle string
	priority      int
	priorityLabel string
	onCreate      func(title, description, teamID, projectID, assigneeID, cycleID string, priority int)
	cachedUsers   []struct{ ID, Name string }
	cachedCycles  []struct{ ID, Name string }
}

type CreateIssueModalOptions struct {
	TeamID    string
	ProjectID string
	Parent    *linearapi.IssueRef
	CycleID   string
}

// NewCreateIssueModal creates a new create issue modal.
func NewCreateIssueModal(app *App) *CreateIssueModal {
	cm := &CreateIssueModal{
		app:      app,
		priority: 3, // Default: Normal
	}

	cm.fm = NewFormModal(app, "New Issue")
	cm.parentView = cm.fm.AddStatic("")
	cm.titleField = cm.fm.AddInput("Title", "")
	cm.descField = cm.fm.AddTextArea("Description", "", 4)

	cm.assigneeField = cm.fm.AddPicker("Assignee", []string{"Unassigned"}, 0, func(_ string, index int) {
		if index == 0 {
			cm.assigneeID = ""
			cm.assigneeName = ""
		} else if index > 0 && index <= len(cm.cachedUsers) {
			user := cm.cachedUsers[index-1]
			cm.assigneeID = user.ID
			cm.assigneeName = user.Name
		}
	})
	cm.cycleField = cm.fm.AddPicker("Cycle", []string{"No cycle"}, 0, func(_ string, index int) {
		if index == 0 {
			cm.cycleID = ""
			cm.cycleName = ""
		} else if index > 0 && index <= len(cm.cachedCycles) {
			cycle := cm.cachedCycles[index-1]
			cm.cycleID = cycle.ID
			cm.cycleName = cycle.Name
		}
	})
	cm.priorityField = cm.fm.AddPicker("Priority", priorityLabels, 3, func(option string, index int) {
		cm.priority = index
		cm.priorityLabel = option
	})

	create := func() {
		title := cm.titleField.GetText()
		desc := cm.descField.GetText()
		cm.Hide()
		if cm.onCreate != nil && title != "" {
			cm.onCreate(title, desc, cm.teamID, cm.projectID, cm.assigneeID, cm.cycleID, cm.priority)
		}
	}
	cm.fm.AddButtons(
		FormButton{Label: "Create", OnPress: create},
		FormButton{Label: "Cancel", OnPress: cm.Hide},
	)
	cm.fm.SetOnSubmit(create)
	cm.fm.SetOnCancel(cm.Hide)
	cm.fm.SetHint("Esc cancel · Tab next · ⏎ open dropdown · ⌃⏎ create")

	return cm
}

// Show displays the create issue modal.
func (cm *CreateIssueModal) Show(teamID, projectID string, onCreate func(title, description, teamID, projectID, assigneeID, cycleID string, priority int)) {
	cm.ShowWithOptions(CreateIssueModalOptions{TeamID: teamID, ProjectID: projectID}, onCreate)
}

func (cm *CreateIssueModal) ShowWithOptions(options CreateIssueModalOptions, onCreate func(title, description, teamID, projectID, assigneeID, cycleID string, priority int)) {
	logger.Debug("tui.create_issue: showing create issue modal team_id=%s project_id=%s", options.TeamID, options.ProjectID)
	cm.teamID = options.TeamID
	cm.projectID = options.ProjectID
	cm.onCreate = onCreate
	cm.selectedCycle = options.CycleID
	if options.Parent != nil {
		cm.fm.SetTitle("New Sub-Issue")
		cm.parentView.SetText(fmt.Sprintf("Parent: %s - %s", options.Parent.Identifier, options.Parent.Title))
	} else {
		cm.fm.SetTitle("New Issue")
		cm.parentView.SetText("")
	}

	// Reset form fields
	cm.titleField.SetText("")
	cm.descField.SetText("", true)

	// Reset selections
	cm.assigneeID = ""
	cm.assigneeName = ""
	cm.assigneeField.SetCurrentOption(0)
	cm.cycleID = ""
	cm.cycleName = ""
	cm.cycleField.SetCurrentOption(0)
	cm.priority = 3 // Default to Normal
	cm.priorityLabel = "Normal"
	cm.priorityField.SetCurrentOption(3)

	// Show modal first with loading state for async fields.
	cm.fm.SetPickerOptions(cm.assigneeField, []string{"Loading..."}, nil)
	cm.fm.SetPickerOptions(cm.cycleField, []string{"Loading..."}, nil)
	cm.fm.Show("create_issue")

	// Load users asynchronously
	cm.loadUsers()
	cm.loadCycles()
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
				cm.fm.SetPickerOptions(cm.assigneeField, []string{"Unassigned", "(Failed to load users)"}, nil)
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
	cm.fm.SetPickerOptions(cm.assigneeField, assigneeOptions, func(_ string, index int) {
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

func (cm *CreateIssueModal) loadCycles() {
	cycles := cm.app.GetTeamCycles()
	if len(cycles) > 0 {
		cm.populateCycleDropdown(cycles)
		return
	}

	go func() {
		loadedCycles, err := cm.app.FetchTeamCycles(cm.teamID)
		if err != nil {
			cm.app.app.QueueUpdateDraw(func() {
				cm.fm.SetPickerOptions(cm.cycleField, []string{"No cycle", "(Failed to load cycles)"}, nil)
			})
			return
		}
		cm.app.app.QueueUpdateDraw(func() {
			cm.populateCycleDropdown(loadedCycles)
		})
	}()
}

func (cm *CreateIssueModal) populateCycleDropdown(cycles []linearapi.Cycle) {
	cycleOptions := []string{"No cycle"}
	cm.cachedCycles = make([]struct{ ID, Name string }, 0, len(cycles))
	selectedIndex := 0
	for _, cycle := range cycles {
		displayName := cycle.DisplayName()
		switch {
		case cycle.IsActive:
			displayName += " (active)"
		case cycle.IsNext:
			displayName += " (next)"
		case cycle.IsPrevious:
			displayName += " (previous)"
		}
		cycleOptions = append(cycleOptions, displayName)
		cm.cachedCycles = append(cm.cachedCycles, struct{ ID, Name string }{cycle.ID, displayName})
		if cycle.ID == cm.selectedCycle {
			selectedIndex = len(cycleOptions) - 1
		}
	}
	cm.fm.SetPickerOptions(cm.cycleField, cycleOptions, func(_ string, index int) {
		if index == 0 {
			cm.cycleID = ""
			cm.cycleName = ""
		} else if index > 0 && index <= len(cm.cachedCycles) {
			cycle := cm.cachedCycles[index-1]
			cm.cycleID = cycle.ID
			cm.cycleName = cycle.Name
		}
	})
	cm.cycleField.SetCurrentOption(selectedIndex)
	if selectedIndex == 0 {
		cm.cycleID = ""
		cm.cycleName = ""
	} else {
		cycle := cm.cachedCycles[selectedIndex-1]
		cm.cycleID = cycle.ID
		cm.cycleName = cycle.Name
	}
}

// Hide hides the create issue modal.
func (cm *CreateIssueModal) Hide() {
	cm.fm.Hide("create_issue")
}

// HandleKey handles keyboard input for the create issue modal.
func (cm *CreateIssueModal) HandleKey(event *tcell.EventKey) *tcell.EventKey {
	return cm.fm.HandleKey(event)
}

// GetModal returns the modal flex for adding to pages.
func (cm *CreateIssueModal) GetModal() *tview.Flex {
	return cm.fm.Root()
}
