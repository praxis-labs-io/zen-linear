package tui

import (
	"context"
	"fmt"
	"slices"

	"github.com/praxis-labs-io/zen-linear/internal/config"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
)

func (a *App) ShowCreateIssueModal() {
	a.showCreateIssueModalWithParent("", nil)
}

func (a *App) ShowCreateSubIssueModal(parentID string) {
	a.showCreateIssueModalWithParent(parentID, a.issueRefForID(parentID))
}

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

func (a *App) ShowEditLabelsModal() {
	issue := a.GetSelectedIssue()
	if issue == nil {
		return
	}
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

func issueLabelIDs(issue linearapi.Issue) []string {
	ids := make([]string, len(issue.Labels))
	for i, label := range issue.Labels {
		ids[i] = label.ID
	}
	slices.Sort(ids)
	return ids
}

func (a *App) ShowKeysModal() {
	if a.keysModal == nil {
		return
	}
	a.keysModal.Show()
}

func (a *App) ShowSettingsModal() {
	if a.settingsModal == nil {
		return
	}

	a.settingsModal.Show()
}

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
