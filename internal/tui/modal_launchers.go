package tui

import (
	"context"
	"fmt"

	"github.com/zen-linear/zen-linear/internal/config"
	"github.com/zen-linear/zen-linear/internal/linearapi"
	"github.com/zen-linear/zen-linear/internal/logger"
)

// ShowCreateIssueModal shows the issue form for a new issue.
func (a *App) ShowCreateIssueModal() {
	a.showCreateIssueModalWithParent("", nil)
}

// ShowCreateSubIssueModal shows the issue form with a parent issue pre-set.
func (a *App) ShowCreateSubIssueModal(parentID string) {
	a.showCreateIssueModalWithParent(parentID, a.issueRefForID(parentID))
}

// showCreateIssueModalWithParent shows the issue form seeded from whatever the
// navigation tree has selected, optionally with a parent.
func (a *App) showCreateIssueModalWithParent(parentID string, parentRef *linearapi.IssueRef) {
	projectID := ""
	if a.selectedNavigation != nil && a.selectedNavigation.IsProject {
		projectID = a.selectedNavigation.ID
	}
	cycleID := ""
	if a.selectedNavigation != nil && a.selectedNavigation.IsCycle {
		cycleID = a.selectedNavigation.CycleID
	}

	a.issueFormModal.Show(IssueFormOptions{
		Mode:      IssueFormCreate,
		TeamID:    a.GetSelectedTeamID(),
		Parent:    parentRef,
		ParentID:  parentID,
		ProjectID: projectID,
		CycleID:   cycleID,
	})
}

// ShowEditIssueModal shows the issue form prefilled from the selected issue.
func (a *App) ShowEditIssueModal() {
	issue := a.GetSelectedIssue()
	if issue == nil {
		a.flashStatus("No issue selected")
		return
	}
	parentRef := issue.Parent
	parentID := ""
	if parentRef != nil {
		parentID = parentRef.ID
	}

	a.issueFormModal.Show(IssueFormOptions{
		Mode:     IssueFormEdit,
		TeamID:   issue.TeamID,
		Issue:    issue,
		Parent:   parentRef,
		ParentID: parentID,
	})
}

// createIssueFromForm runs the create the issue form assembled and splices the
// new issue into the list. onDone reports the outcome to the form, which stays
// up until the write lands so a refusal keeps what was typed.
func (a *App) createIssueFromForm(input linearapi.CreateIssueInput, onDone func(error)) {
	createIssue := a.createIssueFunc
	if createIssue == nil {
		createIssue = a.api.CreateIssue
	}
	go func() {
		issue, err := createIssue(context.Background(), input)
		a.QueueUpdateDraw(func() {
			if err != nil {
				logger.ErrorWithErr(err, "tui.app: failed to create issue title=%s", input.Title)
				a.updateStatusBarWithError(err)
				if onDone != nil {
					onDone(err)
				}
				return
			}
			noun := "issue"
			if input.ParentID != "" {
				noun = "sub-issue"
			}
			logger.Info("tui.app: created %s issue=%s title=%s", noun, issue.Identifier, input.Title)
			a.flashSuccess(fmt.Sprintf("Created %s %s", noun, issue.Identifier))
			a.applyIssueInsert(issue)
			if onDone != nil {
				onDone(nil)
			}
		})
	}()
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
				a.flashSuccess(fmt.Sprintf("Updated description for %s", issue.Identifier))

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
	// The modal names this issue, so the write targets it even if a refresh
	// moves the selection while the labels are still loading.
	target := *issue

	teamID := target.TeamID
	if teamID == "" {
		teamID = a.GetSelectedTeamID()
	}
	if teamID == "" {
		logger.Warning("tui.app: cannot edit labels, no team context issue=%s", target.Identifier)
		a.updateStatusBarWithError(fmt.Errorf("cannot edit labels: no team context"))
		return
	}

	currentLabelIDs := make([]string, len(target.Labels))
	for i, lbl := range target.Labels {
		currentLabelIDs[i] = lbl.ID
	}

	go func() {
		logger.Debug("tui.app: loading labels for edit modal issue=%s team_id=%s", target.Identifier, teamID)
		ctx := context.Background()
		availableLabels, err := a.cache.GetIssueLabels(ctx, teamID)
		if err != nil {
			logger.ErrorWithErr(err, "tui.app: failed to load labels issue=%s team_id=%s", target.Identifier, teamID)
			a.QueueUpdateDraw(func() {
				a.updateStatusBarWithError(err)
			})
			return
		}
		logger.Debug("tui.app: loaded labels issue=%s count=%d", target.Identifier, len(availableLabels))

		items := make([]MultiSelectItem, len(availableLabels))
		for i, label := range availableLabels {
			items[i] = MultiSelectItem{ID: label.ID, Label: label.Name}
		}

		a.QueueUpdateDraw(func() {
			a.multiSelectModal.ShowWithContext("Edit Labels", a.issueContextLine(target), items, currentLabelIDs, func(labelIDs []string) {
				a.saveIssueField(issueFieldLabelsSave(target, labelIDs))
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
