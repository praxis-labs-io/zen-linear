package tui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/zen-linear/zen-linear/internal/agents"
	"github.com/zen-linear/zen-linear/internal/linearapi"
	"github.com/zen-linear/zen-linear/internal/logger"
)

// FormatShortcut returns a human-readable string for a shortcut. The rune is
// shown verbatim: shortcut dispatch is case-sensitive, so 'w' and 'W' are
// different binds and must render differently.
func FormatShortcut(r rune) string {
	if r == 0 {
		return ""
	}
	return string(r)
}

// Command represents a command that can be executed from the palette.
type Command struct {
	ID              string
	Title           string
	Keywords        []string
	ShortcutRune    rune   // The rune for the keyboard shortcut (e.g., 'r' for refresh)
	ShortcutDisplay string // Custom display text for shortcut (e.g., "/" or "Esc"), overrides ShortcutRune display
	Run             func(a *App)
}

// CommandContext provides context for command execution.
type CommandContext struct {
	SelectedIssue *linearapi.Issue
}

// handleAskAgent handles the ask agent command.
func handleAskAgent(a *App) {
	issue := a.GetSelectedIssue()
	if issue == nil {
		a.updateStatusBarWithError(fmt.Errorf("no issue selected"))
		return
	}

	if a.agentPromptModal == nil {
		a.agentPromptModal = NewAgentPromptModal(a)
	}
	if a.agentOutputModal == nil {
		a.agentOutputModal = NewAgentOutputModal(a)
	}
	if a.agentRunner == nil {
		a.agentRunner = agents.NewRunner()
	}

	issueID := issue.ID
	a.agentPromptModal.Show(a.issueContextLine(*issue), func(prompt string, workspace string) {
		prompt = strings.TrimSpace(prompt)
		if prompt == "" {
			return
		}
		workspace = strings.TrimSpace(workspace)

		go func() {
			fetchIssue := a.fetchIssueByID
			if fetchIssue == nil {
				fetchIssue = a.api.FetchIssueByID
			}

			fullIssue, err := fetchIssue(context.Background(), issueID)
			if err != nil {
				logger.ErrorWithErr(err, "tui.commands: failed to fetch issue for agent issue_id=%s", issueID)
				a.QueueUpdateDraw(func() {
					a.updateStatusBarWithError(err)
				})
				return
			}

			issueContext := agents.BuildIssueContext(fullIssue)
			runner := a.agentRunner

			selected, err := agents.ProviderForKey(a.config.AgentProvider, runner.LookPath)
			if err != nil {
				logger.Error("tui.commands: invalid agent provider provider=%s", a.config.AgentProvider)
				a.QueueUpdateDraw(func() {
					a.updateStatusBarWithError(err)
				})
				return
			}

			if _, ok := selected.ResolveBinary(); !ok {
				logger.Error("tui.commands: agent binary not found provider=%s", selected.Name())
				a.QueueUpdateDraw(func() {
					a.updateStatusBarWithError(fmt.Errorf("agent binary not found for %s", selected.Name()))
				})
				return
			}

			options := agents.AgentRunOptions{
				Workspace: workspace,
				Model:     strings.TrimSpace(a.config.AgentModel),
				Sandbox:   strings.TrimSpace(a.config.AgentSandbox),
			}

			ctx, cancel := context.WithCancel(context.Background())
			a.QueueUpdateDraw(func() {
				title := fmt.Sprintf(" %s Output ", selected.Name())
				a.agentOutputModal.Show(title, cancel)
				a.agentOutputModal.AppendLine(fmt.Sprintf("Starting %s agent run...", selected.Name()))
			})

			runErr := runner.Run(ctx, selected, prompt, issueContext, options, func(event agents.AgentEvent) {
				a.agentOutputModal.AppendEvent(event)
			}, func(line string) {
				a.agentOutputModal.AppendRawLine(line)
			}, func(runErr error) {
				a.agentOutputModal.AppendLine(fmt.Sprintf("error: %v", runErr))
			})

			if runErr != nil {
				a.QueueUpdateDraw(func() {
					a.agentOutputModal.AppendLine(fmt.Sprintf("error: %v", runErr))
					a.agentOutputModal.FailRun(runErr)
				})
				return
			}

			a.agentOutputModal.StopSpinner()
			a.agentOutputModal.AppendLine("Agent run completed.")
		}()
	})
}

// runIssueValueAction runs a synchronous copy/open action on a value drawn from
// the selected issue. When emptyMsg is set and value is empty it flashes that
// instead of acting; otherwise it runs action and flashes successMsg or the error.
func (a *App) runIssueValueAction(value, emptyMsg string, action func(string) error, successMsg string) {
	if emptyMsg != "" && value == "" {
		a.flashStatus(emptyMsg)
		return
	}
	if err := action(value); err != nil {
		a.updateStatusBarWithError(err)
		return
	}
	a.flashStatus(successMsg)
}

func handleOpenBrowserCommand(a *App) {
	issue := a.GetSelectedIssue()
	if issue == nil {
		a.flashStatus("No issue selected")
		return
	}
	openFn := a.openURLFunc
	if openFn == nil {
		openFn = openURL
	}
	a.runIssueValueAction(issue.URL, fmt.Sprintf("No URL for %s", issue.Identifier),
		openFn, fmt.Sprintf("Opened %s: %s", issue.Identifier, issue.URL))
}

func handleCopyIssueIDCommand(a *App) {
	issue := a.GetSelectedIssue()
	if issue == nil {
		a.flashStatus("No issue selected")
		return
	}
	copyFn := a.copyToClipboardFunc
	if copyFn == nil {
		copyFn = copyToClipboard
	}
	a.runIssueValueAction(issue.Identifier, "",
		copyFn, fmt.Sprintf("Copied issue ID: %s", issue.Identifier))
}

func handleCopyIssueURLCommand(a *App) {
	issue := a.GetSelectedIssue()
	if issue == nil {
		a.flashStatus("No issue selected")
		return
	}
	copyFn := a.copyToClipboardFunc
	if copyFn == nil {
		copyFn = copyToClipboard
	}
	a.runIssueValueAction(issue.URL, fmt.Sprintf("No URL for %s", issue.Identifier),
		copyFn, fmt.Sprintf("Copied issue URL: %s", issue.Identifier))
}

func handleOpenGitHubCommand(a *App) {
	issue := a.GetSelectedIssue()
	if issue == nil {
		a.flashStatus("No issue selected")
		return
	}
	for _, attachment := range issue.Attachments {
		if !strings.Contains(strings.ToLower(attachment.SourceType), "github") &&
			!strings.Contains(attachment.URL, "github.com") {
			continue
		}
		openFn := a.openURLFunc
		if openFn == nil {
			openFn = openURL
		}
		if err := openFn(attachment.URL); err != nil {
			a.updateStatusBarWithError(err)
			return
		}
		a.flashStatus(fmt.Sprintf("Opened GitHub: %s", attachment.URL))
		return
	}
	a.flashStatus(fmt.Sprintf("No GitHub link on %s", issue.Identifier))
}

func handleCopyBranchCommand(a *App) {
	issue := a.GetSelectedIssue()
	if issue == nil {
		a.flashStatus("No issue selected")
		return
	}
	copyFn := a.copyToClipboardFunc
	if copyFn == nil {
		copyFn = copyToClipboard
	}
	a.runIssueValueAction(issue.BranchName, fmt.Sprintf("No branch name for %s", issue.Identifier),
		copyFn, fmt.Sprintf("Copied branch name: %s", issue.BranchName))
}

// DefaultCommands returns the default set of commands for the palette.
func DefaultCommands(app *App) []Command {
	lookPath := exec.LookPath
	if app != nil && app.agentRunner != nil && app.agentRunner.LookPath != nil {
		lookPath = app.agentRunner.LookPath
	}
	availableProviders := agents.AvailableProviderKeys(lookPath)

	commands := []Command{
		{
			ID:           "refresh",
			Title:        "Refresh issues",
			Keywords:     []string{"refresh", "reload", "r"},
			ShortcutRune: 'r',
			Run: func(a *App) {
				a.flashStatus("Refreshing issues...")
				a.refreshIssues()
			},
		},
		{
			ID:              "search",
			Title:           "Search issues",
			Keywords:        []string{"search", "find", "s", "/"},
			ShortcutDisplay: "/", // Handled globally, not via ShortcutRune
			Run: func(a *App) {
				a.openSearchTab()
			},
		},
		{
			ID:           "toggle_favorite",
			Title:        "Favorite / unfavorite navigation item",
			Keywords:     []string{"favorite", "unfavorite", "star", "pin", "bookmark"},
			ShortcutRune: 'f',
			Run:          handleToggleFavorite,
		},
		{
			ID:       "settings",
			Title:    "Settings",
			Keywords: []string{"settings", "config", "preferences"},
			Run: func(a *App) {
				a.ShowSettingsModal()
			},
		},
		{
			ID:           "switch_workspace",
			Title:        "Switch workspace",
			Keywords:     []string{"workspace", "switch", "account", "organization"},
			ShortcutRune: 'W',
			Run: func(a *App) {
				a.showWorkspacePicker()
			},
		},
		{
			ID:       "toggle_navigation_pane",
			Title:    "Toggle navigation pane",
			Keywords: []string{"navigation", "sidebar", "pane", "hide", "show", "toggle"},
			Run: func(a *App) {
				a.toggleNavigationPane()
			},
		},
		{
			ID:       "toggle_details_pane",
			Title:    "Toggle details pane",
			Keywords: []string{"details", "pane", "hide", "show", "toggle"},
			Run: func(a *App) {
				a.toggleDetailsPane()
			},
		},
		{
			ID:       "edit_prompt_templates",
			Title:    "Edit agent prompt templates",
			Keywords: []string{"agent", "prompt", "prompts", "template", "templates"},
			Run: func(a *App) {
				a.ShowPromptTemplatesModal()
			},
		},
		{
			ID:       "toggle_expand_all",
			Title:    "Toggle expand/collapse all sub-issues",
			Keywords: []string{"toggle", "expand", "collapse", "all", "sub-issues"},
			Run: func(a *App) {
				a.issuesMu.RLock()
				issues := a.issues
				a.issuesMu.RUnlock()
				if len(a.expandedState) > 0 {
					CollapseAll(a.expandedState)
					a.regroupIssues("Collapsed all sub-issues")
					return
				}
				ExpandAll(a.expandedState, issues)
				a.regroupIssues("Expanded all sub-issues")
			},
		},
		{
			ID:       "sort_by",
			Title:    "Sort issues by…",
			Keywords: []string{"sort", "order", "status", "priority", "updated", "created"},
			Run: func(a *App) {
				a.showSortByPicker()
			},
		},
		{
			ID:       "group_by",
			Title:    "Group issues by…",
			Keywords: []string{"group", "grouping", "status", "priority", "assignee", "cycle"},
			Run: func(a *App) {
				a.showGroupByPicker()
			},
		},
		{
			ID:       "subgroup_by",
			Title:    "Subgroup issues by…",
			Keywords: []string{"subgroup", "group", "grouping", "nested"},
			Run: func(a *App) {
				a.showSubgroupByPicker()
			},
		},
		{
			ID:           "open_browser",
			Title:        "Open in browser",
			Keywords:     []string{"open", "browser", "o", "web"},
			ShortcutRune: 'o',
			Run:          handleOpenBrowserCommand,
		},
		{
			ID:           "copy_id",
			Title:        "Copy issue ID",
			Keywords:     []string{"copy", "id", "c", "identifier"},
			ShortcutRune: 'y',
			Run:          handleCopyIssueIDCommand,
		},
		{
			ID:       "open_github",
			Title:    "Open GitHub link",
			Keywords: []string{"open", "github", "pull", "pr"},
			Run: func(a *App) {
				handleOpenGitHubCommand(a)
			},
		},
		{
			ID:       "copy_branch",
			Title:    "Copy branch name",
			Keywords: []string{"copy", "branch", "git", "checkout"},
			Run: func(a *App) {
				handleCopyBranchCommand(a)
			},
		},
		{
			ID:           "copy_url",
			Title:        "Copy issue URL",
			Keywords:     []string{"copy", "url", "link"},
			ShortcutRune: 'w', // 'w' for web URL
			Run:          handleCopyIssueURLCommand,
		},
		{
			ID:       "ask_agent",
			Title:    "Ask agent about selected issue",
			Keywords: []string{"agent", "ai", "claude", "cursor", "assistant"},
			Run:      handleAskAgent,
		},
		{
			ID:       "set_due_date",
			Title:    "Set due date",
			Keywords: []string{"due", "date", "deadline", "set"},
			Run: func(a *App) {
				a.showSetDueDateModal()
			},
		},
		{
			ID:       "clear_due_date",
			Title:    "Clear due date",
			Keywords: []string{"due", "date", "deadline", "clear", "remove"},
			Run: func(a *App) {
				a.clearDueDateForSelectedIssue()
			},
		},
		{
			ID:       "set_priority",
			Title:    "Set priority",
			Keywords: []string{"priority", "urgent", "high", "normal", "low", "set"},
			Run: func(a *App) {
				a.showSetPriorityPicker()
			},
		},
		{
			ID:       "edit_estimate",
			Title:    "Edit estimate",
			Keywords: []string{"estimate", "points", "edit"},
			Run: func(a *App) {
				a.showEditEstimateModal()
			},
		},
		{
			ID:       "clear_estimate",
			Title:    "Clear estimate",
			Keywords: []string{"estimate", "points", "clear", "remove"},
			Run: func(a *App) {
				a.clearEstimateForSelectedIssue()
			},
		},
		{
			ID:       "set_project",
			Title:    "Set project",
			Keywords: []string{"project", "set", "move"},
			Run: func(a *App) {
				a.showSetProjectPicker()
			},
		},
		{
			ID:       "clear_project",
			Title:    "Clear project",
			Keywords: []string{"project", "clear", "remove"},
			Run: func(a *App) {
				a.clearProjectForSelectedIssue()
			},
		},
		{
			ID:       "list_project_milestones",
			Title:    "List project milestones",
			Keywords: []string{"project", "milestone", "list"},
			Run: func(a *App) {
				a.listProjectMilestonesForSelectedIssue()
			},
		},
		{
			ID:       "set_milestone",
			Title:    "Set milestone",
			Keywords: []string{"project", "milestone", "set"},
			Run: func(a *App) {
				a.showSetMilestonePicker()
			},
		},
		{
			ID:       "clear_milestone",
			Title:    "Clear milestone",
			Keywords: []string{"project", "milestone", "clear", "remove"},
			Run: func(a *App) {
				a.clearMilestoneForSelectedIssue()
			},
		},
		{
			ID:       "filter_issues",
			Title:    "Filter issues",
			Keywords: []string{"filter", "issues", "query"},
			Run: func(a *App) {
				a.showFilterIssuesPicker()
			},
		},
		{
			ID:       "clear_filters",
			Title:    "Clear filters",
			Keywords: []string{"filter", "clear", "reset"},
			Run: func(a *App) {
				a.clearFilters()
			},
		},
		{
			ID:       "filter_assignee",
			Title:    "Filter by assignee",
			Keywords: []string{"filter", "assignee", "user"},
			Run: func(a *App) {
				a.showAssigneeFilter()
			},
		},
		{
			ID:       "filter_labels",
			Title:    "Filter by labels",
			Keywords: []string{"filter", "labels", "tags"},
			Run: func(a *App) {
				a.showLabelFilter()
			},
		},
		{
			ID:       "filter_status",
			Title:    "Filter by status",
			Keywords: []string{"filter", "status", "state"},
			Run: func(a *App) {
				a.showStatusFilter()
			},
		},
		{
			ID:       "filter_project",
			Title:    "Filter by project",
			Keywords: []string{"filter", "project"},
			Run: func(a *App) {
				a.showProjectFilter()
			},
		},
		{
			ID:       "filter_cycle",
			Title:    "Filter by cycle",
			Keywords: []string{"filter", "cycle", "sprint"},
			Run: func(a *App) {
				a.showCycleFilter()
			},
		},
		{
			ID:       "filter_due_date",
			Title:    "Filter by due date",
			Keywords: []string{"filter", "due", "date"},
			Run: func(a *App) {
				a.showDueDateFilter()
			},
		},
		{
			ID:       "filter_estimate",
			Title:    "Filter by estimate",
			Keywords: []string{"filter", "estimate", "points"},
			Run: func(a *App) {
				a.showEstimateFilter()
			},
		},
		{
			ID:       "add_issue_relation",
			Title:    "Add issue relation",
			Keywords: []string{"relation", "dependency", "blocking", "blocked", "related", "duplicate", "similar"},
			Run: func(a *App) {
				a.showAddIssueRelationPicker()
			},
		},
		{
			ID:       "remove_issue_relation",
			Title:    "Remove issue relation",
			Keywords: []string{"relation", "dependency", "remove", "unlink"},
			Run: func(a *App) {
				a.showRemoveIssueRelationPicker()
			},
		},
		{
			ID:       "subscribe_issue",
			Title:    "Subscribe",
			Keywords: []string{"subscribe", "watch", "subscriber"},
			Run: func(a *App) {
				a.subscribeSelectedIssue()
			},
		},
		{
			ID:       "unsubscribe_issue",
			Title:    "Unsubscribe",
			Keywords: []string{"unsubscribe", "watch", "subscriber"},
			Run: func(a *App) {
				a.unsubscribeSelectedIssue()
			},
		},
		{
			ID:       "open_attachment",
			Title:    "Open attachment",
			Keywords: []string{"attachment", "link", "open", "github", "jira", "slack", "url"},
			Run: func(a *App) {
				a.openSelectedAttachment()
			},
		},
		{
			ID:       "copy_attachment_url",
			Title:    "Copy attachment URL",
			Keywords: []string{"attachment", "link", "copy", "url"},
			Run: func(a *App) {
				a.copySelectedAttachmentURL()
			},
		},
		{
			ID:           "assign_me",
			Title:        "Assign to me",
			Keywords:     []string{"assign", "me", "self", "take"},
			ShortcutRune: 'm',
			Run: func(a *App) {
				issue := a.GetSelectedIssue()
				user := a.GetCurrentUser()
				if issue == nil || user == nil {
					a.flashStatus("No issue or current user selected")
					return
				}
				a.runIssueUpdate(
					linearapi.UpdateIssueInput{ID: issue.ID, AssigneeID: &user.ID},
					fmt.Sprintf("Assigned %s to %s", issue.Identifier, user.DisplayName),
				)
			},
		},
		{
			ID:           "unassign",
			Title:        "Unassign issue",
			Keywords:     []string{"unassign", "remove", "clear assignee"},
			ShortcutRune: 'u',
			Run: func(a *App) {
				issue := a.GetSelectedIssue()
				if issue == nil {
					a.flashStatus("No issue selected")
					return
				}
				emptyAssignee := ""
				a.runIssueUpdate(
					linearapi.UpdateIssueInput{ID: issue.ID, AssigneeID: &emptyAssignee},
					fmt.Sprintf("Unassigned %s", issue.Identifier),
				)
			},
		},
		{
			ID:           "archive",
			Title:        "Archive issue",
			Keywords:     []string{"archive", "delete", "remove"},
			ShortcutRune: 'x',
			Run: func(a *App) {
				issue := a.GetSelectedIssue()
				if issue == nil {
					a.flashStatus("No issue selected")
					return
				}
				a.confirmationModal.Show(
					"Archive Issue",
					fmt.Sprintf("Archive %s - %s?", issue.Identifier, issue.Title),
					"Archive",
					func() {
						go func() {
							ctx := context.Background()
							err := a.GetAPI().ArchiveIssue(ctx, issue.ID)
							a.QueueUpdateDraw(func() {
								if err != nil {
									logger.ErrorWithErr(err, "tui.commands: failed to archive issue issue=%s", issue.Identifier)
									a.updateStatusBarWithError(err)
									return
								}
								logger.Info("tui.commands: archived issue issue=%s", issue.Identifier)
								a.flashStatus(fmt.Sprintf("Archived %s", issue.Identifier))
								if len(issue.Children) > 0 {
									// Linear may archive sub-issues with the
									// parent; only a fetch answers for them.
									a.refreshIssues()
								} else {
									a.applyIssueRemoval(issue.ID)
								}
							})
						}()
					},
				)
			},
		},
		{
			ID:           "change_status",
			Title:        "Change status",
			Keywords:     []string{"status", "state", "workflow", "todo", "progress", "done"},
			ShortcutRune: 's',
			Run: func(a *App) {
				issue := a.GetSelectedIssue()
				if issue == nil {
					a.flashStatus("No issue selected")
					return
				}
				a.ShowStatusPicker(a.issueContextLine(*issue), func(stateID string) {
					a.runIssueUpdate(
						linearapi.UpdateIssueInput{ID: issue.ID, StateID: &stateID},
						fmt.Sprintf("Changed status for %s", issue.Identifier),
					)
				})
			},
		},
		{
			ID:           "set_cycle",
			Title:        "Set cycle",
			Keywords:     []string{"cycle", "sprint", "iteration", "set"},
			ShortcutRune: 'c',
			Run: func(a *App) {
				issue := a.GetSelectedIssue()
				if issue == nil {
					a.flashStatus("No issue selected")
					return
				}
				a.ShowCyclePicker(a.issueContextLine(*issue), func(cycleID string) {
					a.runIssueUpdate(linearapi.UpdateIssueInput{ID: issue.ID, CycleID: &cycleID}, fmt.Sprintf("Set cycle for %s", issue.Identifier))
				})
			},
		},
		{
			ID:       "clear_cycle",
			Title:    "Clear cycle",
			Keywords: []string{"cycle", "clear", "remove", "unset"},
			Run: func(a *App) {
				issue := a.GetSelectedIssue()
				if issue == nil {
					a.flashStatus("No issue selected")
					return
				}
				if issue.Cycle == nil {
					a.flashStatus("No cycle assigned")
					return
				}
				emptyCycleID := ""
				a.runIssueUpdate(
					linearapi.UpdateIssueInput{ID: issue.ID, CycleID: &emptyCycleID},
					fmt.Sprintf("Cleared cycle for %s", issue.Identifier),
				)
			},
		},
		{
			ID:           "assign_user",
			Title:        "Assign to user",
			Keywords:     []string{"assign", "user", "team", "member"},
			ShortcutRune: 'a',
			Run: func(a *App) {
				issue := a.GetSelectedIssue()
				if issue == nil {
					a.flashStatus("No issue selected")
					return
				}
				a.ShowUserPicker(a.issueContextLine(*issue), func(userID string) {
					a.runIssueUpdate(linearapi.UpdateIssueInput{ID: issue.ID, AssigneeID: &userID}, fmt.Sprintf("Assigned %s", issue.Identifier))
				})
			},
		},
		{
			ID:           "create_issue",
			Title:        "Create new issue",
			Keywords:     []string{"create", "new", "add", "issue"},
			ShortcutRune: 'n',
			Run: func(a *App) {
				teamID := a.GetSelectedTeamID()
				if teamID == "" {
					a.updateStatusBarWithError(fmt.Errorf("please select a team first"))
					return
				}
				a.ShowCreateIssueModal()
			},
		},
		{
			ID:           "edit_title",
			Title:        "Edit issue title",
			Keywords:     []string{"edit", "title", "rename"},
			ShortcutRune: 'e',
			Run: func(a *App) {
				issue := a.GetSelectedIssue()
				if issue == nil {
					a.flashStatus("No issue selected")
					return
				}
				a.ShowEditTitleModal()
			},
		},
		{
			ID:           "edit_description",
			Title:        "Edit issue description",
			Keywords:     []string{"edit", "description", "body", "details"},
			ShortcutRune: 'D',
			Run: func(a *App) {
				issue := a.GetSelectedIssue()
				if issue == nil {
					a.flashStatus("No issue selected")
					return
				}
				a.ShowEditDescriptionModal()
			},
		},
		{
			ID:           "edit_labels",
			Title:        "Edit issue labels",
			Keywords:     []string{"labels", "label", "tag", "tags"},
			ShortcutRune: 'g', // 'g' for tags (since 'l' is used for vim navigation)
			Run: func(a *App) {
				issue := a.GetSelectedIssue()
				if issue == nil {
					a.flashStatus("No issue selected")
					return
				}
				a.ShowEditLabelsModal()
			},
		},
		{
			ID:       "toggle_sub_issues",
			Title:    "Toggle sub-issues",
			Keywords: []string{"toggle", "expand", "collapse", "sub", "children"},
			// No shortcut - ⌘+T conflicts with new tab. Use Space key in table instead.
			Run: func(a *App) {
				issue := a.GetSelectedIssue()
				if issue == nil {
					a.flashStatus("No issue selected")
					return
				}
				a.toggleIssueExpanded(issue.ID)
			},
		},
		{
			ID:           "view_parent",
			Title:        "View parent issue",
			Keywords:     []string{"parent", "up", "back"},
			ShortcutRune: 'p',
			Run: func(a *App) {
				issue := a.GetSelectedIssue()
				if issue == nil {
					a.flashStatus("No issue selected")
					return
				}
				if issue.Parent == nil {
					a.flashStatus("No parent issue")
					return
				}
				if a.jumpToParent(issue.Parent.ID) {
					return
				}
				// A loaded parent with no row is hidden behind a collapsed
				// group or ancestor, which is not the same as never fetched.
				if _, loaded := a.allIDToIssue[issue.Parent.ID]; loaded {
					a.flashStatus("Parent issue is hidden by a collapsed group")
					return
				}
				a.flashStatus("Parent issue not loaded")
			},
		},
		{
			ID:           "expand_all",
			Title:        "Expand all sub-issues",
			Keywords:     []string{"expand", "all", "open"},
			ShortcutRune: ']',
			Run: func(a *App) {
				a.issuesMu.RLock()
				issues := a.issues
				selectedID := ""
				if a.selectedIssue != nil {
					selectedID = a.selectedIssue.ID
				}
				a.issuesMu.RUnlock()
				ExpandAll(a.expandedState, issues)
				a.renderIssueChange(selectedID, false)
			},
		},
		{
			ID:           "collapse_all",
			Title:        "Collapse all sub-issues",
			Keywords:     []string{"collapse", "all", "close"},
			ShortcutRune: '[',
			Run: func(a *App) {
				CollapseAll(a.expandedState)
				a.issuesMu.RLock()
				selectedID := ""
				if a.selectedIssue != nil {
					selectedID = a.selectedIssue.ID
				}
				a.issuesMu.RUnlock()
				a.renderIssueChange(selectedID, false)
			},
		},
		{
			ID:           "create_sub_issue",
			Title:        "Create sub-issue",
			Keywords:     []string{"create", "sub", "child", "new"},
			ShortcutRune: 'b',
			Run: func(a *App) {
				issue := a.GetSelectedIssue()
				if issue == nil {
					a.flashStatus("No issue selected")
					return
				}
				// Create sub-issue with current issue as parent
				a.ShowCreateSubIssueModal(issue.ID)
			},
		},
		{
			ID:           "set_parent",
			Title:        "Set parent issue",
			Keywords:     []string{"set", "parent", "link"},
			ShortcutRune: 'i',
			Run: func(a *App) {
				issue := a.GetSelectedIssue()
				if issue == nil {
					a.flashStatus("No issue selected")
					return
				}
				// Cannot set parent if this issue has children
				if len(issue.Children) > 0 {
					logger.Warning("tui.commands: cannot set parent on issue with sub-issues issue=%s", issue.Identifier)
					a.flashStatus("Cannot set parent on issue with sub-issues")
					return
				}
				a.ShowParentIssuePicker(a.issueContextLine(*issue), func(parentID string) {
					a.runIssueUpdate(
						linearapi.UpdateIssueInput{ID: issue.ID, ParentID: &parentID},
						fmt.Sprintf("Set parent for %s", issue.Identifier),
					)
				})
			},
		},
		{
			ID:           "remove_parent",
			Title:        "Remove parent",
			Keywords:     []string{"remove", "parent", "unlink", "top"},
			ShortcutRune: 'd',
			Run: func(a *App) {
				issue := a.GetSelectedIssue()
				if issue == nil {
					a.flashStatus("No issue selected")
					return
				}
				if issue.Parent == nil {
					a.flashStatus("No parent issue")
					return
				}
				a.confirmationModal.Show(
					"Remove Parent",
					fmt.Sprintf("Remove parent from %s?", issue.Identifier),
					"Remove",
					func() {
						emptyParent := ""
						a.runIssueUpdate(
							linearapi.UpdateIssueInput{ID: issue.ID, ParentID: &emptyParent},
							fmt.Sprintf("Removed parent from %s", issue.Identifier),
						)
					},
				)
			},
		},
		{
			ID:           "add_comment",
			Title:        "Add comment",
			Keywords:     []string{"add", "comment", "reply", "t"},
			ShortcutRune: 't',
			Run: func(a *App) {
				issue := a.GetSelectedIssue()
				if issue == nil {
					a.flashStatus("No issue selected")
					return
				}
				a.createCommentModal.Show(issue.ID, a.issueContextLine(*issue), a.handleCreateComment)
			},
		},
	}
	if len(availableProviders) == 0 {
		filtered := make([]Command, 0, len(commands))
		for _, command := range commands {
			if command.ID == "ask_agent" {
				continue
			}
			filtered = append(filtered, command)
		}
		commands = filtered
	}

	if app != nil {
		applyCommandKeybindings(commands, app.config.Keybindings)
	}
	return commands
}

// openURL opens a URL in the default browser.
func openURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		logger.Warning("tui.commands: unsupported OS for opening URLs os=%s", runtime.GOOS)
		return nil
	}

	if err := cmd.Start(); err != nil {
		logger.ErrorWithErr(err, "tui.commands: failed to open URL url=%s", url)
		return err
	}

	logger.Debug("tui.commands: opened URL in browser url=%s", url)
	return nil
}

// copyToClipboard copies text to the system clipboard.
func copyToClipboard(text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		cmd = exec.Command("xclip", "-selection", "clipboard")
	case "windows":
		cmd = exec.Command("clip")
	default:
		logger.Warning("tui.commands: unsupported OS for clipboard operations os=%s", runtime.GOOS)
		return nil
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		logger.ErrorWithErr(err, "tui.commands: failed to get stdin pipe for clipboard command")
		return err
	}

	if err := cmd.Start(); err != nil {
		logger.ErrorWithErr(err, "tui.commands: failed to start clipboard command")
		return err
	}

	_, err = stdin.Write([]byte(text))
	if err != nil {
		logger.ErrorWithErr(err, "tui.commands: failed to write to clipboard")
		return err
	}
	_ = stdin.Close()

	if err := cmd.Wait(); err != nil {
		logger.ErrorWithErr(err, "tui.commands: clipboard command failed")
		return err
	}

	logger.Debug("tui.commands: copied to clipboard text_length=%d", len(text))
	return nil
}
