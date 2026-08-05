package tui

import (
	"context"
	"fmt"

	"github.com/zen-linear/zen-linear/internal/config"
	"github.com/zen-linear/zen-linear/internal/linearapi"
	"github.com/zen-linear/zen-linear/internal/logger"
)

// ShowCreateIssueModal shows the create issue modal.
func (a *App) ShowCreateIssueModal() {
	a.showCreateIssueModalWithParent("", nil)
}

// ShowCreateSubIssueModal shows the create issue modal with a parent issue pre-set.
func (a *App) ShowCreateSubIssueModal(parentID string) {
	a.showCreateIssueModalWithParent(parentID, a.issueRefForID(parentID))
}

// showCreateIssueModalWithParent shows the create issue modal, optionally with a parent.
func (a *App) showCreateIssueModalWithParent(parentID string, parentRef *linearapi.IssueRef) {
	teamID := a.GetSelectedTeamID()
	projectID := ""
	if a.selectedNavigation != nil && a.selectedNavigation.IsProject {
		projectID = a.selectedNavigation.ID
	}
	cycleID := ""
	if a.selectedNavigation != nil && a.selectedNavigation.IsCycle {
		cycleID = a.selectedNavigation.CycleID
	}

	a.createIssueModal.ShowWithOptions(CreateIssueModalOptions{
		TeamID:    teamID,
		ProjectID: projectID,
		Parent:    parentRef,
		CycleID:   cycleID,
	}, func(title, description, tID, pID, assigneeID, cID string, priority int) {
		if title == "" {
			return
		}
		go func() {
			ctx := context.Background()
			input := linearapi.CreateIssueInput{
				TeamID:      tID,
				Title:       title,
				Description: description,
			}
			if pID != "" {
				input.ProjectID = pID
			}
			if assigneeID != "" {
				input.AssigneeID = assigneeID
			}
			if cID != "" {
				input.CycleID = cID
			}
			if priority > 0 {
				input.Priority = priority
			}
			if parentID != "" {
				input.ParentID = parentID
			}
			issue, err := a.api.CreateIssue(ctx, input)
			a.QueueUpdateDraw(func() {
				if err != nil {
					logger.ErrorWithErr(err, "tui.app: failed to create issue title=%s", title)
					a.updateStatusBarWithError(err)
					return
				}
				if parentID != "" {
					logger.Info("tui.app: created sub-issue issue=%s title=%s", issue.Identifier, title)
					a.flashStatus(fmt.Sprintf("Created sub-issue %s", issue.Identifier))
				} else {
					logger.Info("tui.app: created issue issue=%s title=%s", issue.Identifier, title)
					a.flashStatus(fmt.Sprintf("Created issue %s", issue.Identifier))
				}
				a.applyIssueInsert(issue)
			})
		}()
	})
}

func (a *App) issueRefForID(issueID string) *linearapi.IssueRef {
	if issueID == "" {
		return nil
	}
	a.issuesMu.RLock()
	defer a.issuesMu.RUnlock()
	if a.selectedIssue != nil && a.selectedIssue.ID == issueID {
		return &linearapi.IssueRef{ID: a.selectedIssue.ID, Identifier: a.selectedIssue.Identifier, Title: a.selectedIssue.Title}
	}
	for _, issue := range a.issues {
		if issue.ID == issueID {
			return &linearapi.IssueRef{ID: issue.ID, Identifier: issue.Identifier, Title: issue.Title}
		}
	}
	return nil
}

// ShowEditTitleModal shows the edit title modal.
func (a *App) ShowEditTitleModal() {
	issue := a.GetSelectedIssue()
	if issue == nil {
		return
	}

	a.editTitleModal.Show(issue.ID, issue.Title, a.issueContextLine(*issue), func(issueID, title string) {
		a.runIssueUpdate(
			linearapi.UpdateIssueInput{ID: issueID, Title: &title},
			fmt.Sprintf("Updated title for %s", issue.Identifier),
		)
	})
}

// ShowEditDescriptionModal shows the edit description modal for the selected
// issue. Submitting empty text clears the description.
func (a *App) ShowEditDescriptionModal() {
	issue := a.GetSelectedIssue()
	if issue == nil {
		return
	}

	a.editDescriptionModal.Show(issue.ID, issue.Description, a.issueContextLine(*issue), func(issueID, description string) {
		go func() {
			ctx := context.Background()
			_, err := a.api.UpdateIssue(ctx, linearapi.UpdateIssueInput{
				ID:          issueID,
				Description: &description,
			})
			a.QueueUpdateDraw(func() {
				if err != nil {
					logger.ErrorWithErr(err, "tui.app: failed to update issue description issue=%s", issue.Identifier)
					a.updateStatusBarWithError(err)
					return
				}
				logger.Info("tui.app: updated issue description issue=%s", issue.Identifier)
				a.flashStatus(fmt.Sprintf("Updated description for %s", issue.Identifier))

				// Refetch the full issue so the details pane shows the new
				// description without losing comments.
				a.loadIssueDetailsByID(issueID)
			})
		}()
	})
}

// ShowEditLabelsModal shows the edit labels modal for the selected issue.
func (a *App) ShowEditLabelsModal() {
	issue := a.GetSelectedIssue()
	if issue == nil {
		return
	}

	teamID := issue.TeamID
	if teamID == "" {
		teamID = a.GetSelectedTeamID()
	}
	if teamID == "" {
		logger.Warning("tui.app: cannot edit labels, no team context issue=%s", issue.Identifier)
		a.updateStatusBarWithError(fmt.Errorf("cannot edit labels: no team context"))
		return
	}

	// Get current label IDs from the issue
	currentLabelIDs := make([]string, len(issue.Labels))
	for i, lbl := range issue.Labels {
		currentLabelIDs[i] = lbl.ID
	}

	// Load available labels asynchronously
	go func() {
		logger.Debug("tui.app: loading labels for edit modal issue=%s team_id=%s", issue.Identifier, teamID)
		ctx := context.Background()
		availableLabels, err := a.cache.GetIssueLabels(ctx, teamID)
		if err != nil {
			logger.ErrorWithErr(err, "tui.app: failed to load labels issue=%s team_id=%s", issue.Identifier, teamID)
			a.QueueUpdateDraw(func() {
				a.updateStatusBarWithError(err)
			})
			return
		}
		logger.Debug("tui.app: loaded labels issue=%s count=%d", issue.Identifier, len(availableLabels))

		a.QueueUpdateDraw(func() {
			a.editLabelsModal.Show(issue.ID, currentLabelIDs, availableLabels, a.issueContextLine(*issue), func(issueID string, labelIDs []string) {
				a.runIssueUpdate(
					linearapi.UpdateIssueInput{ID: issueID, LabelIDs: &labelIDs},
					fmt.Sprintf("Updated labels for %s", issue.Identifier),
				)
			})
		})
	}()
}

// ShowSettingsModal shows the settings modal.
func (a *App) ShowSettingsModal() {
	if a.settingsModal == nil {
		return
	}

	a.settingsModal.Show()
}

// ShowPromptTemplatesModal shows the prompt templates modal.
func (a *App) ShowPromptTemplatesModal() {
	if a.promptTemplatesModal == nil {
		return
	}

	promptsPath, err := config.PromptTemplatesFilePath()
	if err != nil {
		a.updateStatusBarWithError(err)
		return
	}

	templates, err := config.EnsurePromptTemplatesFile(promptsPath)
	if err != nil {
		a.updateStatusBarWithError(err)
		templates = a.agentPromptTemplates
		if len(templates) == 0 {
			templates = config.DefaultAgentPromptTemplates()
		}
	} else {
		a.agentPromptTemplates = templates
	}

	a.promptTemplatesModal.Show(templates, func(updated []config.AgentPromptTemplate) error {
		if err := config.SavePromptTemplates(promptsPath, updated); err != nil {
			return err
		}
		a.agentPromptTemplates = updated
		a.agentPromptModal = NewAgentPromptModal(a)
		return nil
	})
}
