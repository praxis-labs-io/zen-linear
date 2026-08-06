package tui

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/agents"
	"github.com/zen-linear/zen-linear/internal/config"
	"github.com/zen-linear/zen-linear/internal/linearapi"
	"github.com/zen-linear/zen-linear/internal/logger"
)

// App is the main application controller that manages all UI components.
type App struct {
	app *tview.Application
	// linearDeps holds the API client, its cache, and every func derived from
	// them; newLinearDeps builds it for NewApp and applySettings alike.
	linearDeps
	config    config.Config
	theme     Theme
	themeTags ThemeTags
	density   DensityProfile

	// activeWorkspaceName is the configured workspace the current API key
	// belongs to; empty for explicit keys and OAuth sessions.
	activeWorkspaceName string

	// UI components
	pages                  *tview.Pages
	mainLayout             *tview.Flex
	contentFlex            *tview.Flex
	navigationHidden       bool
	detailsHidden          bool
	layoutMode             layoutMode
	palettePreviousPane    FocusTarget
	navigationTree         *tview.TreeView
	navNodeLabels          map[*tview.TreeNode]navNodeLabel
	favorites              []linearapi.Favorite
	favoritesGroup         *tview.TreeNode
	allIssuesTable         *tview.Table
	myIssuesTable          *tview.Table
	searchInput            *tview.InputField
	searchResultsTable     *tview.Table
	searchPanel            *tview.Flex     // Search tab shell: input row + body
	searchBody             *tview.Flex     // Swappable slot: results table or placeholder
	searchPlaceholder      *tview.TextView // Centered empty/loading/error message
	issuesColumn           *tview.Flex     // Vertical flex holding the active issues tab
	detailsView            *tview.Flex     // Flex container for details (description + comments)
	detailsDescriptionView *tview.TextView // Scrollable description/metadata view
	detailsCommentsView    *tview.TextView // Scrollable comments view
	statusBar              *tview.TextView
	paletteModal           *tview.Flex
	paletteInput           *tview.InputField
	paletteList            *tview.List
	paletteModalContent    *tview.Flex
	paletteCtrl            *PaletteController
	pickerModal            *PickerModal
	issueFormModal         *IssueFormModal
	createCommentModal     *CreateCommentModal
	editDescriptionModal   *EditDescriptionModal
	editLabelsModal        *EditLabelsModal
	textInputModal         *TextInputModal
	multiSelectModal       *MultiSelectModal
	settingsModal          *SettingsModal
	promptTemplatesModal   *AgentPromptTemplatesModal
	agentPromptModal       *AgentPromptModal
	agentOutputModal       *AgentOutputModal
	confirmationModal      *ConfirmationModal
	agentRunner            *agents.Runner
	agentPromptTemplates   []config.AgentPromptTemplate

	// App state (protected by issuesMu)
	issuesMu            sync.RWMutex
	selectedIssue       *linearapi.Issue
	selectedNavigation  *NavigationNode
	issues              []linearapi.Issue
	focusedPane         FocusTarget
	activeIssuesSection IssuesSection // Which issues tab is on screen: All, My, or Search

	// Per-section issue tree state (for sub-issue hierarchy)
	allIssueRows  []IssueRow                  // Flattened rows for the "All Issues" table
	allIDToIssue  map[string]*linearapi.Issue // Quick lookup by issue ID for "All Issues"
	myIssueRows   []IssueRow                  // Flattened rows for the "My Issues" table
	myIDToIssue   map[string]*linearapi.Issue // Quick lookup by issue ID for "My Issues"
	expandedState map[string]bool             // Expanded state for parent issues (shared across sections)
	// pendingSectionRenders holds sections whose cells are stale because they
	// were off screen when the model changed, keyed to the row they should
	// select once painted.
	pendingSectionRenders map[IssuesSection]string

	// Filter/sort state
	richFilters IssueFilters
	sortFields  []SortField
	// configuredSortFields is the chain sort_by asked for at startup, kept so
	// the picker can offer it back after a session override.
	configuredSortFields []SortField
	collapsedGroups      map[string]bool
	statusMessage        string

	// Display settings of the active custom view, overriding config until
	// the user picks another list. The overridden flags keep in-session
	// manual grouping/sort choices ahead of the view's. UI-thread only.
	viewPrefs          *viewDisplayPrefs
	groupingOverridden bool
	sortOverridden     bool

	// Search tab state, independent from the main issues list. All fields
	// are read and written on the UI thread only.
	searchQuery         string
	searchIssues        []linearapi.Issue
	searchIssueRows     []IssueRow
	searchIDToIssue     map[string]*linearapi.Issue
	searchInputFocused  bool // sub-focus within FocusIssues + Search tab
	searchLoading       bool
	searchErr           error
	searchReturnSection IssuesSection // tab to return to on Esc from an empty input

	searchDebounceTimer      *time.Timer
	searchDebounceMu         sync.Mutex
	searchDebounceGeneration atomic.Int64
	searchFetchGeneration    atomic.Int64
	searchFetchCancel        context.CancelFunc

	detailDebounceTimer      *time.Timer
	detailDebounceMu         sync.Mutex
	detailDebounceGeneration atomic.Int64
	detailFetchGeneration    atomic.Int64
	detailFetchCancel        context.CancelFunc

	// Cached metadata for currently selected team
	currentUser    *linearapi.User
	teamUsers      []linearapi.User
	teamProjects   []linearapi.Project
	workflowStates []linearapi.WorkflowState
	teamCycles     []linearapi.Cycle
	teamLabels     []linearapi.IssueLabel

	// Loading state
	isLoading                      bool
	pendingRefresh                 bool
	pendingRefreshIssueID          string
	pendingRefreshAllowFocusChange bool
	refreshGeneration              atomic.Int64
	// resetGeneration counts workspace/settings resets, not refreshes, so a
	// fetch in flight across one can tell it belongs to the workspace the user
	// left. refreshGeneration is unusable for that: every refresh bumps it.
	resetGeneration atomic.Int64

	// apiUseBearer and apiOnUnauthorized are the auth mode fixed at startup.
	// applySettings carries them into the rebuilt client so an in-app settings
	// save keeps an OAuth session's bearer scheme and 401 refresh hook.
	apiUseBearer      bool
	apiOnUnauthorized func(context.Context) (string, error)

	// UI/self callbacks and test seams that are not derived from the API client.
	issueMatchesScopeFunc   func(context.Context, linearapi.FetchIssuesParams, string) (bool, error)
	queueUpdateDraw         func(func())
	openURLFunc             func(string) error
	copyToClipboardFunc     func(string) error
	refreshCompleted        func()
	preloadTeamMetadataFunc func(string)
	detailDebounce          time.Duration
	favoritesChanged        func()

	// UI update mutex (for test safety when queueUpdateDraw executes immediately)
	uiUpdateMu sync.Mutex

	// Details pane sub-view focus
	focusedDetailsView     bool // false = description, true = comments
	detailsCommentsVisible bool // Tracks whether comments view is shown
}

// FocusTarget indicates which pane has focus.
type FocusTarget int

const (
	FocusNavigation FocusTarget = iota
	FocusIssues
	FocusDetails
	FocusPalette
)

// NewApp creates a new application instance.
func NewApp(clientCfg linearapi.ClientConfig, cfg config.Config, templates []config.AgentPromptTemplate) *App {
	if len(templates) == 0 {
		templates = config.DefaultAgentPromptTemplates()
	}
	theme := ResolveTheme(cfg.Theme)
	density := ResolveDensity(cfg.Density)
	initMarkdownRenderer(theme)

	app := &App{
		app:                  tview.NewApplication(),
		config:               cfg,
		theme:                theme,
		themeTags:            NewThemeTags(theme),
		density:              density,
		pages:                tview.NewPages(),
		focusedPane:          FocusNavigation,
		sortFields:           parseSortFields(cfg.SortBy),
		configuredSortFields: parseSortFields(cfg.SortBy),
		expandedState:        make(map[string]bool),
		navNodeLabels:        make(map[*tview.TreeNode]navNodeLabel),
		allIDToIssue:         make(map[string]*linearapi.Issue),
		myIDToIssue:          make(map[string]*linearapi.Issue),
		searchIDToIssue:      make(map[string]*linearapi.Issue),
		agentPromptTemplates: templates,
		activeWorkspaceName:  workspaceNameForKey(cfg.Workspaces, cfg.LinearAPIKey),
		// Details opens on demand (Enter or the palette toggle); the list
		// gets the room.
		detailsHidden: true,
	}

	app.linearDeps = newLinearDeps(clientCfg, cfg.CacheTTL)
	app.apiUseBearer = clientCfg.UseBearer
	app.apiOnUnauthorized = clientCfg.OnUnauthorized
	app.paletteCtrl = NewPaletteController(DefaultCommands(app))
	app.openURLFunc = openURL
	app.copyToClipboardFunc = copyToClipboard
	app.preloadTeamMetadataFunc = app.preloadTeamMetadata
	app.queueUpdateDraw = func(f func()) {
		app.app.QueueUpdateDraw(f)
	}

	app.applyThemeStyles()

	app.buildLayout()
	app.bindGlobalKeys()

	return app
}

// Run starts the application and blocks until it exits.
func (a *App) Run() error {
	a.app.SetRoot(a.pages, true).EnableMouse(true)

	// Load initial data asynchronously
	a.loadInitialData()

	// Start the application event loop
	return a.app.Run()
}

// loadInitialData fetches user, navigation, and issues in a background goroutine.
func (a *App) loadInitialData() {
	// Snapshot the seam and the generation here: applySettings reassigns the
	// one and resetCachedState bumps the other, both on the UI thread, while
	// the goroutines below run.
	fetchUser := a.fetchCurrentUserFunc
	generation := a.resetGeneration.Load()
	go func() {
		started := time.Now()
		ctx := context.Background()

		// The nav tree needs teams and favorites; only the My/Other split needs
		// the current user. Overlapping the user fetch with the tree fetch takes
		// a full round trip off first paint. The issue refresh still waits on
		// the user, or it would split the first page wrong.
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.loadCurrentUser(ctx, fetchUser, generation)
		}()

		teams, favorites, err := a.fetchNavigationData(ctx)
		wg.Wait()
		logger.Debug("tui.app: startup fetches completed elapsed=%s", time.Since(started))
		if err != nil {
			logger.ErrorWithErr(err, "tui.app: failed to load teams")
			// The error needs its own draw. The refresh sets the status bar to
			// "Loading..." synchronously, so sharing a closure means tview only
			// ever paints the second message.
			a.app.QueueUpdateDraw(func() {
				a.updateStatusBarWithError(err)
			})
			// No tree, but All Issues does not need one.
			a.app.QueueUpdateDraw(func() {
				a.refreshIssuesWithFocusChange(false)
			})
			return
		}

		a.app.QueueUpdateDraw(func() {
			a.rebuildNavigationTree(teams, favorites)
		})

		// Default navigation triggers its own refresh after applying the
		// configured selection.
		if !a.applyDefaultNavigation(ctx, teams) {
			// Startup refresh must not steal focus from the navigation pane.
			a.app.QueueUpdateDraw(func() {
				a.refreshIssuesWithFocusChange(false)
			})
		}
	}()
}

// loadCurrentUser fetches the authenticated user and installs it on the UI
// thread. Callers must run it off the event loop; the queued write is what
// orders it against readers like GetCurrentUser and rebuildIssueRowModels.
// generation is the value read before the fetch started: a workspace switch
// bumps it, and a user from the workspace the user left must not land in the
// one they are now in.
func (a *App) loadCurrentUser(ctx context.Context, fetchUser func(context.Context) (linearapi.User, error), generation int64) {
	user, err := fetchUser(ctx)
	if err != nil {
		logger.Warning("tui.app: failed to load current user error=%v", err)
		return
	}
	a.QueueUpdateDraw(func() {
		if generation != a.resetGeneration.Load() {
			logger.Debug("tui.app: discarding superseded current user user=%s", user.DisplayName)
			return
		}
		a.currentUser = &user
	})
	logger.Debug("tui.app: current user loaded user=%s", user.DisplayName)
}

// applySettings updates runtime dependencies to match a new configuration.
func (a *App) applySettings(newCfg config.Config) {
	// Rebuilding the modals re-adds their pages, and tview hands focus from an
	// added page down to whichever pane the layout was built focused on. The
	// restore has to be last on every exit: resetCachedState moves the active
	// tab out from under an earlier one.
	defer a.restoreModalFocus()

	a.config = newCfg
	a.applyThemeAndDensity()

	logLevel := parseLogLevel(newCfg.LogLevel)
	if err := logger.Reinit(newCfg.LogFile, logLevel); err != nil {
		logger.ErrorWithErr(err, "tui.app: failed to reinitialize logger")
		// Queueing here would hang the app: the settings save and the workspace
		// switcher both call this from the event loop, and QueueUpdateDraw waits
		// on that same loop to drain it.
		a.updateStatusBarWithError(err)
		return
	}
	logger.Debug("tui.app: settings applied log_file=%s log_level=%s", newCfg.LogFile, newCfg.LogLevel)

	a.linearDeps = newLinearDeps(linearapi.ClientConfig{
		Token:          newCfg.LinearAPIKey,
		Endpoint:       newCfg.APIEndpoint,
		Timeout:        newCfg.Timeout,
		UseBearer:      a.apiUseBearer,
		OnUnauthorized: a.apiOnUnauthorized,
	}, newCfg.CacheTTL)

	logger.Debug("tui.app: resetting cached state after settings change")
	a.resetCachedState()
	a.loadInitialData()
}

func (a *App) selectedIssueID(section IssuesSection) string {
	table := a.tableForSection(section)
	if table == nil {
		return ""
	}
	row, _ := table.GetSelection()
	if row <= 0 {
		return ""
	}
	issue := a.getIssueFromRowForSection(row, section)
	if issue == nil {
		return ""
	}
	return issue.ID
}

// resetCachedState clears cached user and issue data after config changes.
func (a *App) resetCachedState() {
	a.issuesMu.Lock()
	a.issues = nil
	a.allIssueRows = nil
	a.allIDToIssue = make(map[string]*linearapi.Issue)
	a.myIssueRows = nil
	a.myIDToIssue = make(map[string]*linearapi.Issue)
	a.issuesMu.Unlock()

	a.selectedNavigation = nil
	a.currentUser = nil
	a.teamUsers = nil
	a.teamProjects = nil
	a.workflowStates = nil
	a.teamCycles = nil
	a.teamLabels = nil
	a.richFilters = IssueFilters{}
	a.collapsedGroups = make(map[string]bool)
	a.viewPrefs = nil
	a.groupingOverridden = false
	a.sortOverridden = false
	a.searchQuery = ""
	a.clearSearchResults()
	if a.searchInput != nil {
		a.searchInput.SetText("")
	}
	a.updateSearchBody()
	a.cancelSearchDebounce()
	a.searchInputFocused = false
	a.searchReturnSection = IssuesSectionAll
	a.activeIssuesSection = IssuesSectionAll
	a.expandedState = make(map[string]bool)
	// Clearing the models is not enough: an off-screen tab keeps its painted
	// cells until something repaints it, and dropping the pending markers
	// removes the only thing that would have.
	a.pendingSectionRenders = nil
	for _, section := range []IssuesSection{IssuesSectionAll, IssuesSectionMy} {
		if table := a.tableForSection(section); table != nil {
			table.Clear()
		}
	}
	// The reset moved the active tab back to All, and the column is still
	// mounting whichever tab the user was on.
	a.updateIssuesColumnLayout()

	a.isLoading = false
	a.pendingRefresh = false
	a.pendingRefreshIssueID = ""
	a.pendingRefreshAllowFocusChange = true
	// Bump generation to prevent in-flight refreshes from updating UI.
	a.refreshGeneration.Add(1)
	a.resetGeneration.Add(1)
	// The pane would otherwise keep painting an issue nothing can act on, since
	// GetSelectedIssue is already nil.
	a.clearSelectedIssue()
}

// parseLogLevel converts a string log level to a logger.LogLevel.
func parseLogLevel(level string) logger.LogLevel {
	switch level {
	case "debug":
		return logger.LevelDebug
	case "info":
		return logger.LevelInfo
	case "warning":
		return logger.LevelWarning
	case "error":
		return logger.LevelError
	default:
		return logger.LevelWarning
	}
}

// buildLayout constructs the main UI layout.
func (a *App) buildLayout() {
	// Build all panes
	a.navigationTree = a.buildNavigationTree()
	a.allIssuesTable = a.buildIssuesTable(" All Issues ", IssuesSectionAll)
	a.myIssuesTable = a.buildIssuesTable(" My Issues ", IssuesSectionMy)
	a.buildSearchPanel()
	// Create vertical flex for issues column
	a.issuesColumn = tview.NewFlex().SetDirection(tview.FlexRow)
	// All is the tab the app opens on; the others mount on a tab switch.
	a.issuesColumn.AddItem(a.allIssuesTable, 0, 1, false)
	a.detailsView = a.buildDetailsView()
	a.statusBar = a.buildStatusBar()

	// Create horizontal split: navigation (20%) | issues (50%) | details (30%)
	a.contentFlex = tview.NewFlex().
		AddItem(a.navigationTree, 0, 2, true).
		AddItem(a.issuesColumn, 0, 5, false).
		AddItem(a.detailsView, 0, 3, false)

	// Create vertical layout: content + status bar
	a.mainLayout = tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(a.contentFlex, 0, 1, true).
		AddItem(a.statusBar, 1, 1, false)

	// Apply initial pane visibility (details is hidden by default).
	a.rebuildContentLayout()

	// Build palette modal
	a.paletteModal = a.buildPaletteModal()

	// Build picker and create issue modals
	a.pickerModal = NewPickerModal(a)
	a.issueFormModal = NewIssueFormModal(a)
	a.createCommentModal = NewCreateCommentModal(a)
	a.editDescriptionModal = NewEditDescriptionModal(a)
	a.editLabelsModal = NewEditLabelsModal(a)
	a.textInputModal = NewTextInputModal(a)
	a.multiSelectModal = NewMultiSelectModal(a)
	a.settingsModal = NewSettingsModal(a)
	a.promptTemplatesModal = NewAgentPromptTemplatesModal(a)
	a.agentPromptModal = NewAgentPromptModal(a)
	a.agentOutputModal = NewAgentOutputModal(a)
	a.confirmationModal = NewConfirmationModal(a)
	a.agentRunner = agents.NewRunner()

	// Reflow panes when the terminal width crosses a breakpoint.
	a.app.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		width, _ := screen.Size()
		a.watchLayoutWidth(width)
		return false
	})

	// Add main layout to pages
	a.pages.AddPage("main", a.mainLayout, true, true)
	a.pages.AddPage("palette", a.paletteModal, true, false)

	// Set initial focus
	a.updateFocus()
}
