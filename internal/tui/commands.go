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

// CommandScope is where a command applies. A command reaches the keyboard and
// the palette only from a pane its scope covers, so a key cannot act on
// something the pane it was pressed in has no bearing on.
type CommandScope int

const (
	// ScopeGlobal commands apply everywhere. It is the zero value, so a
	// command that names no scope keeps working from every pane.
	ScopeGlobal CommandScope = iota
	// ScopeIssue commands act on the selected issue: the issues and details
	// panes.
	ScopeIssue
	// ScopeNavigation commands act on the navigation tree.
	ScopeNavigation
	// ScopeComment actions act on the comment the ring is on. No palette
	// command holds it: the scope exists so a comment key and an issue key can
	// share a rune, which is how r replies on a focused card and refreshes
	// everywhere else.
	ScopeComment
)

// CommandGroup is the heading a command files under in the palette's default
// list. Typing a query drops the headings and lists the matches flat.
type CommandGroup string

const (
	GroupIssue     CommandGroup = "Issue"
	GroupFields    CommandGroup = "Fields"
	GroupRelations CommandGroup = "Relations"
	GroupShare     CommandGroup = "Share"
	GroupList      CommandGroup = "List"
	GroupView      CommandGroup = "View"
	GroupAgent     CommandGroup = "Agent"
	GroupApp       CommandGroup = "App"
)

// commandGroupOrder is the order the palette stacks the headings in. A command
// whose group is missing here is listed after them all under no heading, which
// TestEveryCommandFilesUnderAHeading is what stops happening by accident.
var commandGroupOrder = []CommandGroup{
	GroupIssue,
	GroupFields,
	GroupRelations,
	GroupShare,
	GroupList,
	GroupView,
	GroupAgent,
	GroupApp,
}

// Command represents a command that can be executed from the palette.
type Command struct {
	ID              string
	Title           string
	Keywords        []string
	ShortcutRune    rune   // The rune for the keyboard shortcut (e.g., 'r' for refresh)
	ShortcutDisplay string // Custom display text for shortcut (e.g., "/" or "Esc"), overrides ShortcutRune display
	Scope           CommandScope
	Group           CommandGroup
	Run             func(a *App)
}

// appliesIn reports whether the command is reachable from a pane of the given
// scope.
func (c Command) appliesIn(scope CommandScope) bool {
	return c.Scope == ScopeGlobal || c.Scope == scope
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
				title := fmt.Sprintf("%s Output", selected.Name())
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
	a.flashSuccess(successMsg)
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
		a.flashSuccess(fmt.Sprintf("Opened GitHub: %s", attachment.URL))
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
			Group:        GroupList,
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
			Group:           GroupList,
			Title:           "Search issues",
			Keywords:        []string{"search", "find", "s", "/"},
			ShortcutDisplay: "/", // Handled globally, not via ShortcutRune
			Run: func(a *App) {
				a.focusNavSearch()
			},
		},
		{
			ID:       "toggle_favorite",
			Group:    GroupView,
			Scope:    ScopeNavigation,
			Title:    "Favorite / unfavorite navigation item",
			Keywords: []string{"favorite", "unfavorite", "star", "pin", "bookmark"},
			// Shifted, because it is destructive from the tree: lowercase f sat
			// next to the movement keys and unfavorited a view on a mistype.
			ShortcutRune: 'F',
			Run:          handleToggleFavorite,
		},
		{
			ID:       "settings",
			Group:    GroupApp,
			Title:    "Settings",
			Keywords: []string{"settings", "config", "preferences"},
			Run: func(a *App) {
				a.ShowSettingsModal()
			},
		},
		{
			ID:           "switch_workspace",
			Group:        GroupApp,
			Title:        "Switch workspace",
			Keywords:     []string{"workspace", "switch", "account", "organization"},
			ShortcutRune: 'w',
			Run: func(a *App) {
				a.showWorkspacePicker()
			},
		},
		{
			ID:           "toggle_navigation_pane",
			Group:        GroupView,
			Title:        "Toggle navigation pane",
			Keywords:     []string{"navigation", "sidebar", "pane", "hide", "show", "toggle"},
			ShortcutRune: '<',
			Run: func(a *App) {
				a.toggleNavigationPane()
			},
		},
		{
			ID:           "toggle_details_pane",
			Group:        GroupView,
			Title:        "Toggle details pane",
			Keywords:     []string{"details", "pane", "hide", "show", "toggle"},
			ShortcutRune: '>',
			Run: func(a *App) {
				a.toggleDetailsPane()
			},
		},
		{
			ID:           "zoom_details",
			Group:        GroupView,
			Title:        "Zoom details",
			Keywords:     []string{"zoom", "details", "focus", "full", "expand", "read", "view"},
			ShortcutRune: 'v',
			Run: func(a *App) {
				a.toggleDetailsZoom()
			},
		},
		{
			ID:       "edit_prompt_templates",
			Group:    GroupAgent,
			Title:    "Edit agent prompt templates",
			Keywords: []string{"agent", "prompt", "prompts", "template", "templates"},
			Run: func(a *App) {
				a.ShowPromptTemplatesModal()
			},
		},
		{
			ID:       "sort_by",
			Group:    GroupList,
			Title:    "Sort issues by…",
			Keywords: []string{"sort", "order", "status", "priority", "updated", "created"},
			Run: func(a *App) {
				a.showSortByPicker()
			},
		},
		{
			ID:       "group_by",
			Group:    GroupList,
			Title:    "Group issues by…",
			Keywords: []string{"group", "grouping", "status", "priority", "assignee", "cycle"},
			Run: func(a *App) {
				a.showGroupByPicker()
			},
		},
		{
			ID:       "subgroup_by",
			Group:    GroupList,
			Title:    "Subgroup issues by…",
			Keywords: []string{"subgroup", "group", "grouping", "nested"},
			Run: func(a *App) {
				a.showSubgroupByPicker()
			},
		},
		{
			ID:           "open_browser",
			Group:        GroupShare,
			Scope:        ScopeIssue,
			Title:        "Open in browser",
			Keywords:     []string{"open", "browser", "o", "web"},
			ShortcutRune: 'o',
			Run:          handleOpenBrowserCommand,
		},
		{
			ID:           "copy_id",
			Group:        GroupShare,
			Scope:        ScopeIssue,
			Title:        "Copy issue ID",
			Keywords:     []string{"copy", "id", "identifier"},
			ShortcutRune: 'i',
			Run:          handleCopyIssueIDCommand,
		},
		{
			ID:           "open_github",
			Group:        GroupShare,
			Scope:        ScopeIssue,
			Title:        "Open GitHub link",
			Keywords:     []string{"open", "github", "pull", "pr"},
			ShortcutRune: 'O',
			Run: func(a *App) {
				handleOpenGitHubCommand(a)
			},
		},
		{
			ID:           "copy_branch",
			Group:        GroupShare,
			Scope:        ScopeIssue,
			Title:        "Copy branch name",
			Keywords:     []string{"copy", "branch", "git", "checkout"},
			ShortcutRune: 'Y',
			Run: func(a *App) {
				handleCopyBranchCommand(a)
			},
		},
		{
			ID:           "copy_url",
			Group:        GroupShare,
			Scope:        ScopeIssue,
			Title:        "Copy issue URL",
			Keywords:     []string{"copy", "url", "link"},
			ShortcutRune: 'y',
			Run:          handleCopyIssueURLCommand,
		},
		{
			ID:       "ask_agent",
			Group:    GroupAgent,
			Scope:    ScopeIssue,
			Title:    "Ask agent about selected issue",
			Keywords: []string{"agent", "ai", "claude", "cursor", "assistant"},
			Run:      handleAskAgent,
		},
		{
			ID:       "set_due_date",
			Group:    GroupFields,
			Scope:    ScopeIssue,
			Title:    "Set due date",
			Keywords: []string{"due", "date", "deadline", "set"},
			Run: func(a *App) {
				a.showSetDueDateModal()
			},
		},
		{
			ID:       "clear_due_date",
			Group:    GroupFields,
			Scope:    ScopeIssue,
			Title:    "Clear due date",
			Keywords: []string{"due", "date", "deadline", "clear", "remove"},
			Run: func(a *App) {
				a.clearDueDateForSelectedIssue()
			},
		},
		{
			ID:           "set_priority",
			Group:        GroupFields,
			Scope:        ScopeIssue,
			Title:        "Set priority",
			Keywords:     []string{"priority", "urgent", "high", "normal", "low", "set"},
			ShortcutRune: 'p',
			Run: func(a *App) {
				a.showSetPriorityPicker()
			},
		},
		{
			ID:       "edit_estimate",
			Group:    GroupFields,
			Scope:    ScopeIssue,
			Title:    "Edit estimate",
			Keywords: []string{"estimate", "points", "edit"},
			Run: func(a *App) {
				a.showEditEstimateModal()
			},
		},
		{
			ID:       "clear_estimate",
			Group:    GroupFields,
			Scope:    ScopeIssue,
			Title:    "Clear estimate",
			Keywords: []string{"estimate", "points", "clear", "remove"},
			Run: func(a *App) {
				a.clearEstimateForSelectedIssue()
			},
		},
		{
			ID:           "set_project",
			Group:        GroupFields,
			Scope:        ScopeIssue,
			Title:        "Set project",
			Keywords:     []string{"project", "set", "move"},
			ShortcutRune: 'P',
			Run: func(a *App) {
				a.showSetProjectPicker()
			},
		},
		{
			ID:       "clear_project",
			Group:    GroupFields,
			Scope:    ScopeIssue,
			Title:    "Clear project",
			Keywords: []string{"project", "clear", "remove"},
			Run: func(a *App) {
				a.clearProjectForSelectedIssue()
			},
		},
		{
			ID:       "list_project_milestones",
			Group:    GroupFields,
			Scope:    ScopeIssue,
			Title:    "List project milestones",
			Keywords: []string{"project", "milestone", "list"},
			Run: func(a *App) {
				a.listProjectMilestonesForSelectedIssue()
			},
		},
		{
			ID:       "set_milestone",
			Group:    GroupFields,
			Scope:    ScopeIssue,
			Title:    "Set milestone",
			Keywords: []string{"project", "milestone", "set"},
			Run: func(a *App) {
				a.showSetMilestonePicker()
			},
		},
		{
			ID:       "clear_milestone",
			Group:    GroupFields,
			Scope:    ScopeIssue,
			Title:    "Clear milestone",
			Keywords: []string{"project", "milestone", "clear", "remove"},
			Run: func(a *App) {
				a.clearMilestoneForSelectedIssue()
			},
		},
		{
			ID:       "filter_issues",
			Group:    GroupList,
			Title:    "Filter issues",
			Keywords: []string{"filter", "issues", "query"},
			Run: func(a *App) {
				a.showFilterIssuesPicker()
			},
		},
		{
			ID:       "clear_filters",
			Group:    GroupList,
			Title:    "Clear filters",
			Keywords: []string{"filter", "clear", "reset"},
			Run: func(a *App) {
				a.clearFilters()
			},
		},
		{
			ID:       "filter_assignee",
			Group:    GroupList,
			Title:    "Filter by assignee",
			Keywords: []string{"filter", "assignee", "user"},
			Run: func(a *App) {
				a.showAssigneeFilter()
			},
		},
		{
			ID:       "filter_labels",
			Group:    GroupList,
			Title:    "Filter by labels",
			Keywords: []string{"filter", "labels", "tags"},
			Run: func(a *App) {
				a.showLabelFilter()
			},
		},
		{
			ID:       "filter_status",
			Group:    GroupList,
			Title:    "Filter by status",
			Keywords: []string{"filter", "status", "state"},
			Run: func(a *App) {
				a.showStatusFilter()
			},
		},
		{
			ID:       "filter_project",
			Group:    GroupList,
			Title:    "Filter by project",
			Keywords: []string{"filter", "project"},
			Run: func(a *App) {
				a.showProjectFilter()
			},
		},
		{
			ID:       "filter_cycle",
			Group:    GroupList,
			Title:    "Filter by cycle",
			Keywords: []string{"filter", "cycle", "sprint"},
			Run: func(a *App) {
				a.showCycleFilter()
			},
		},
		{
			ID:       "filter_due_date",
			Group:    GroupList,
			Title:    "Filter by due date",
			Keywords: []string{"filter", "due", "date"},
			Run: func(a *App) {
				a.showDueDateFilter()
			},
		},
		{
			ID:       "filter_estimate",
			Group:    GroupList,
			Title:    "Filter by estimate",
			Keywords: []string{"filter", "estimate", "points"},
			Run: func(a *App) {
				a.showEstimateFilter()
			},
		},
		{
			ID:       "add_issue_relation",
			Group:    GroupRelations,
			Scope:    ScopeIssue,
			Title:    "Add issue relation",
			Keywords: []string{"relation", "dependency", "blocking", "blocked", "related", "duplicate", "similar"},
			Run: func(a *App) {
				a.showAddIssueRelationPicker()
			},
		},
		{
			ID:       "remove_issue_relation",
			Group:    GroupRelations,
			Scope:    ScopeIssue,
			Title:    "Remove issue relation",
			Keywords: []string{"relation", "dependency", "remove", "unlink"},
			Run: func(a *App) {
				a.showRemoveIssueRelationPicker()
			},
		},
		{
			ID:       "subscribe_issue",
			Group:    GroupIssue,
			Scope:    ScopeIssue,
			Title:    "Subscribe",
			Keywords: []string{"subscribe", "watch", "subscriber"},
			Run: func(a *App) {
				a.subscribeSelectedIssue()
			},
		},
		{
			ID:       "unsubscribe_issue",
			Group:    GroupIssue,
			Scope:    ScopeIssue,
			Title:    "Unsubscribe",
			Keywords: []string{"unsubscribe", "watch", "subscriber"},
			Run: func(a *App) {
				a.unsubscribeSelectedIssue()
			},
		},
		{
			ID:       "open_attachment",
			Group:    GroupShare,
			Scope:    ScopeIssue,
			Title:    "Open attachment",
			Keywords: []string{"attachment", "link", "open", "github", "jira", "slack", "url"},
			Run: func(a *App) {
				a.openSelectedAttachment()
			},
		},
		{
			ID:       "copy_attachment_url",
			Group:    GroupShare,
			Scope:    ScopeIssue,
			Title:    "Copy attachment URL",
			Keywords: []string{"attachment", "link", "copy", "url"},
			Run: func(a *App) {
				a.copySelectedAttachmentURL()
			},
		},
		{
			ID:           "assign_me",
			Group:        GroupFields,
			Scope:        ScopeIssue,
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
				a.saveIssueField(issueFieldAssigneeSave(*issue, user.ID, formatUserDisplayName(*user)))
			},
		},
		{
			ID:           "unassign",
			Group:        GroupFields,
			Scope:        ScopeIssue,
			Title:        "Unassign issue",
			Keywords:     []string{"unassign", "remove", "clear assignee"},
			ShortcutRune: 'u',
			Run: func(a *App) {
				issue := a.GetSelectedIssue()
				if issue == nil {
					a.flashStatus("No issue selected")
					return
				}
				a.saveIssueField(issueFieldAssigneeClear(*issue))
			},
		},
		{
			ID:           "archive",
			Group:        GroupIssue,
			Scope:        ScopeIssue,
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
								a.flashSuccess(fmt.Sprintf("Archived %s", issue.Identifier))
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
			Group:        GroupFields,
			Scope:        ScopeIssue,
			Title:        "Change status",
			Keywords:     []string{"status", "state", "workflow", "todo", "progress", "done"},
			ShortcutRune: 's',
			Run: func(a *App) {
				issue := a.GetSelectedIssue()
				if issue == nil {
					a.flashStatus("No issue selected")
					return
				}
				// The picker names this issue, so the write targets it even if
				// a refresh moves the selection while the picker is open.
				target := *issue
				a.ShowFieldPicker(issueFieldState, a.issueOptionScope(target), a.issueContextLine(target), func(item PickerItem) {
					a.saveIssueField(issueFieldStateSave(target, item.ID, item.name()))
				})
			},
		},
		{
			ID:           "set_cycle",
			Group:        GroupFields,
			Scope:        ScopeIssue,
			Title:        "Set cycle",
			Keywords:     []string{"cycle", "sprint", "iteration", "set"},
			ShortcutRune: 'C',
			Run: func(a *App) {
				issue := a.GetSelectedIssue()
				if issue == nil {
					a.flashStatus("No issue selected")
					return
				}
				target := *issue
				a.ShowFieldPicker(issueFieldCycle, a.issueOptionScope(target), a.issueContextLine(target), func(item PickerItem) {
					a.saveIssueField(issueFieldCycleSave(target, item.ID, item.name()))
				})
			},
		},
		{
			ID:       "clear_cycle",
			Group:    GroupFields,
			Scope:    ScopeIssue,
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
				a.saveIssueField(issueFieldCycleClear(*issue))
			},
		},
		{
			ID:           "assign_user",
			Group:        GroupFields,
			Scope:        ScopeIssue,
			Title:        "Assign to user",
			Keywords:     []string{"assign", "user", "team", "member"},
			ShortcutRune: 'a',
			Run: func(a *App) {
				issue := a.GetSelectedIssue()
				if issue == nil {
					a.flashStatus("No issue selected")
					return
				}
				target := *issue
				a.ShowFieldPicker(issueFieldAssignee, a.issueOptionScope(target), a.issueContextLine(target), func(item PickerItem) {
					a.saveIssueField(issueFieldAssigneeSave(target, item.ID, item.name()))
				})
			},
		},
		{
			ID:           "change_team",
			Group:        GroupFields,
			Scope:        ScopeIssue,
			Title:        "Change team",
			Keywords:     []string{"team", "move", "change", "transfer"},
			ShortcutRune: 'T',
			Run: func(a *App) {
				a.showChangeTeamPicker()
			},
		},
		{
			ID:           "create_issue",
			Group:        GroupIssue,
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
			ID:           "edit_issue",
			Group:        GroupIssue,
			Scope:        ScopeIssue,
			Title:        "Edit issue",
			Keywords:     []string{"edit", "issue", "fields", "properties", "update"},
			ShortcutRune: 'e',
			Run: func(a *App) {
				a.enterDetailsEdit()
			},
		},
		{
			ID:       "edit_description",
			Group:    GroupIssue,
			Scope:    ScopeIssue,
			Title:    "Edit issue description",
			Keywords: []string{"edit", "description", "body", "details"},
			Run: func(a *App) {
				a.editIssueDescription()
			},
		},
		{
			ID:           "edit_labels",
			Group:        GroupIssue,
			Scope:        ScopeIssue,
			Title:        "Edit issue labels",
			Keywords:     []string{"labels", "label", "tag", "tags"},
			ShortcutRune: 't', // 't' for tags: 'l' is vim navigation and 'L' scrolls columns
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
			Group:    GroupRelations,
			Scope:    ScopeIssue,
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
			ID:       "view_parent",
			Group:    GroupRelations,
			Scope:    ScopeIssue,
			Title:    "View parent issue",
			Keywords: []string{"parent", "up", "back"},
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
				if _, loaded := a.listIDToIssue[issue.Parent.ID]; loaded {
					a.flashStatus("Parent issue is hidden by a collapsed group")
					return
				}
				a.flashStatus("Parent issue not loaded")
			},
		},
		{
			ID:       "expand_all",
			Group:    GroupList,
			Title:    "Expand all sub-issues",
			Keywords: []string{"expand", "all", "open"},
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
			ID:       "collapse_all",
			Group:    GroupList,
			Title:    "Collapse all sub-issues",
			Keywords: []string{"collapse", "all", "close"},
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
			Group:        GroupIssue,
			Scope:        ScopeIssue,
			Title:        "Create sub-issue",
			Keywords:     []string{"create", "sub", "child", "new"},
			ShortcutRune: 'N',
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
			ID:       "set_parent",
			Group:    GroupRelations,
			Scope:    ScopeIssue,
			Title:    "Set parent issue",
			Keywords: []string{"set", "parent", "link"},
			Run: func(a *App) {
				issue := a.GetSelectedIssue()
				if issue == nil {
					a.flashStatus("No issue selected")
					return
				}
				// Cannot set parent if this issue has children
				if len(issue.Children) > 0 {
					logger.Warning("tui.commands: cannot set parent on issue with sub-issues issue=%s", issue.Identifier)
					a.flashError("Cannot set parent on issue with sub-issues")
					return
				}
				target := *issue
				a.ShowParentIssuePicker(a.issueContextLine(target), func(item PickerItem) {
					name := ""
					if ref := a.issueRefForID(item.ID); ref != nil {
						name = ref.Identifier
					}
					a.saveIssueField(issueFieldParentSave(target, item.ID, name))
				})
			},
		},
		{
			ID:       "remove_parent",
			Group:    GroupRelations,
			Scope:    ScopeIssue,
			Title:    "Remove parent",
			Keywords: []string{"remove", "parent", "unlink", "top"},
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
					func() { a.saveIssueField(issueFieldParentClear(*issue)) },
				)
			},
		},
		{
			ID:           "add_comment",
			Group:        GroupIssue,
			Scope:        ScopeIssue,
			Title:        "Add comment",
			Keywords:     []string{"add", "comment", "reply"},
			ShortcutRune: 'c',
			Run: func(a *App) {
				issue := a.GetSelectedIssue()
				if issue == nil {
					a.flashStatus("No issue selected")
					return
				}
				if !a.openComposeBox() {
					a.flashStatus("No comments pane to write in")
				}
			},
		},
	}
	// Read before the agent filter runs: a binding for a command this session
	// happens not to offer names a real id, not a typo.
	scopes := commandScopes(commands)

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
		app.bindings = resolveKeybindings(app.config.Keybindings, scopes)
		applyCommandKeybindings(commands, app.bindings)
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
