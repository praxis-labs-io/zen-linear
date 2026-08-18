package tui

import (
	"context"
	"fmt"
	"slices"

	"github.com/praxis-labs-io/zen-linear/internal/config"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
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
		TeamID:    a.GetSelectedTeamID(),
		Parent:    parentRef,
		ParentID:  parentID,
		ProjectID: projectID,
		CycleID:   cycleID,
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

// ShowEditLabelsModal shows the edit labels modal for the selected issue.
func (a *App) ShowEditLabelsModal() {
	issue := a.GetSelectedIssue()
	if issue == nil {
		return
	}
	// The modal names this issue, so the write targets it even if a refresh
	// moves the selection while the labels are still loading.
	target := *issue

	a.issueFieldOptions(issueFieldLabels, a.issueOptionScope(target), func(loaded []PickerItem) {
		items := make([]MultiSelectItem, 0, len(loaded))
		for _, item := range loaded {
			items = append(items, MultiSelectItem{ID: item.ID, Label: item.Label})
		}
		a.multiSelectModal.ShowWithContext("Edit Labels", a.issueContextLine(target), items, issueLabelIDs(target), func(labelIDs []string) {
			a.saveIssueField(issueFieldLabelsSave(target, labelIDs))
		})
	}, func(err error) {
		logger.ErrorWithErr(err, "tui.app: failed to load labels issue=%s", target.Identifier)
		a.updateStatusBarWithError(err)
	})
}

// issueLabelIDs is what an issue carries now, sorted so two sets compare.
func issueLabelIDs(issue linearapi.Issue) []string {
	ids := make([]string, len(issue.Labels))
	for i, label := range issue.Labels {
		ids[i] = label.ID
	}
	slices.Sort(ids)
	return ids
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
