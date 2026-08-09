package tui

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/agents"
	"github.com/zen-linear/zen-linear/internal/cache"
	"github.com/zen-linear/zen-linear/internal/config"
	"github.com/zen-linear/zen-linear/internal/linearapi"
	"github.com/zen-linear/zen-linear/internal/logger"
	"github.com/zen-linear/zen-linear/internal/session"
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

	// sessionPath is the file the quit flush writes; empty disables the write.
	sessionPath string
	// pendingSession is the place to reopen, applied once by loadInitialData.
	pendingSession *session.State

	// navCachePath is the disk copy of the navigation tree; empty disables it.
	// navCache is that copy as it was at launch, and navTeams is the team list
	// the tree on screen was built from.
	navCachePath string
	navCache     cache.NavFile
	navTeams     []linearapi.Team

	// UI components
	pages                  *tview.Pages
	mainLayout             *tview.Flex
	contentFlex            *tview.Flex
	navigationHidden       bool
	detailsHidden          bool
	detailsZoomed          bool
	layoutMode             layoutMode
	palettePreviousPane    FocusTarget
	navigationTree         *tview.TreeView
	navLoadingNode         *tview.TreeNode // "Loading teams" node, until a tree replaces it
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
	issuesPlaceholder      *tview.Flex     // Stands in for the All/My table when it has no rows
	issuesPlaceholderText  *tview.TextView // Centered loading/empty/error message
	issuesColumn           *tview.Flex     // Vertical flex holding the active issues tab
	detailsView            *tview.Flex     // Flex container for details (description + comments)
	detailsDescriptionView *tview.TextView // Scrollable description/metadata view
	// detailsHeaderLines is the metadata block untruncated, kept so a resize
	// can refit it, and detailsBody is the description already rendered so the
	// refit does not re-run the markdown renderer. detailsFittedWidth is the
	// pane width the two were last joined at.
	detailsHeaderLines   []string
	detailsBody          string
	detailsFittedWidth   int
	detailsCommentsView  *tview.TextView // Scrollable comments view
	statusBar            *tview.TextView
	paletteModal         *tview.Flex
	paletteInput         *tview.InputField
	paletteList          *tview.List
	paletteModalContent  *tview.Flex
	paletteCtrl          *PaletteController
	pickerModal          *PickerModal
	issueFormModal       *IssueFormModal
	createCommentModal   *CreateCommentModal
	editDescriptionModal *EditDescriptionModal
	editLabelsModal      *EditLabelsModal
	textInputModal       *TextInputModal
	multiSelectModal     *MultiSelectModal
	settingsModal        *SettingsModal
	promptTemplatesModal *AgentPromptTemplatesModal
	agentPromptModal     *AgentPromptModal
	agentOutputModal     *AgentOutputModal
	confirmationModal    *ConfirmationModal
	agentRunner          *agents.Runner
	agentPromptTemplates []config.AgentPromptTemplate

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
	// pendingSearchIssueID is the restored session's issue, selected once when
	// the first search results land.
	pendingSearchIssueID string

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
	// metadataTeamID names the team the five caches above were filled for.
	// They follow the navigation tree, which is not always the team of the
	// issue a modal is working on.
	metadataTeamID string

	// Loading state
	isLoading bool
	// navLoading is the navigation fetch, which outlives the first paint now
	// that the tree can come from disk.
	navLoading bool
	// issuesErr is the last issue fetch failure, shown in the pane while the
	// list has nothing to show instead. issuesSettled stays false until a fetch
	// has finished, so a launch that has not started one yet reads as loading
	// rather than as an empty workspace.
	issuesErr     error
	issuesSettled bool
	// loadingGeneration is the refresh that owns the loading flag, so a
	// superseded one cannot clear it out from under its replacement.
	loadingGeneration              int64
	loading                        *loadingIndicator
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
	navigationSettled       func()
	preloadTeamMetadataFunc func(string)
	detailDebounce          time.Duration
	loadingFrameDelay       time.Duration
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
	err := a.app.Run()
	// The frame loop would otherwise outlive the event loop, queueing draws
	// nothing is left to run.
	if a.loading != nil {
		a.loading.stop()
	}
	// Every quit path ends here with the event loop stopped, so the snapshot
	// is settled and no queued update can move it. Recorded on a loop error
	// too: the user's place is worth keeping whichever way the app came down.
	a.persistSession()
	return err
}

// loadInitialData fetches user, navigation, and issues in a background goroutine.
func (a *App) loadInitialData() {
	// Snapshot the seams and the generation here: applySettings reassigns the
	// funcs and resetCachedState bumps the generation, both on the UI thread,
	// while the goroutines below run. consumePendingSession is snapshotted for
	// the same reason and because a second loadInitialData must not re-apply a
	// session the first one already claimed.
	fetchUser := a.fetchCurrentUserFunc
	generation := a.resetGeneration.Load()
	pendingSession := a.consumePendingSession()
	childFetchers := a.teamChildFetchers()
	navFetchers := a.navFetchers()
	workspace := a.activeWorkspaceName
	cached, hasCache := a.cachedNavData()
	a.setNavLoading(true)
	go func() {
		// Signals that the navigation fetch has been dealt with, whichever way
		// it went, for tests that have to wait on a launch with no refresh of
		// its own to watch.
		defer a.notifyNavigationSettled()
		started := time.Now()
		ctx := context.Background()

		// The nav tree needs teams and favorites; only the My/Other split needs
		// the current user. The two run together, and the tree can come from
		// disk, so first paint waits on neither. The issue refresh still waits
		// on the user, or it would split the first page wrong: loadCurrentUser
		// queues the assignment, so anything queued after userDone closes lands
		// behind it.
		var (
			fetched  fetchedNav
			userDone = make(chan struct{})
			navDone  = make(chan struct{})
		)
		go func() {
			defer close(userDone)
			a.loadCurrentUser(ctx, fetchUser, generation)
		}()
		go func() {
			defer close(navDone)
			fetched = fetchNavigationData(ctx, navFetchers)
		}()

		// A cached tree paints now rather than a fetch later, so the issue list
		// starts loading on the saved place while the tree fetch is still out.
		if hasCache {
			a.QueueUpdateDraw(func() {
				a.rebuildNavigationTree(cached.Teams, cached.Favorites)
			})
			<-userDone
			if a.openInitialList(ctx, pendingSession, cached.Teams, cached.Favorites, childFetchers) {
				pendingSession = nil
			}
		}

		<-navDone
		<-userDone
		logger.Debug("tui.app: startup fetches completed elapsed=%s", time.Since(started))
		a.QueueUpdateDraw(func() {
			a.setNavLoading(false)
		})
		if fetched.err != nil {
			logger.ErrorWithErr(fetched.err, "tui.app: failed to load teams")
			// The error needs its own draw. The refresh sets the status bar to
			// "Loading..." synchronously, so sharing a closure means tview only
			// ever paints the second message.
			a.QueueUpdateDraw(func() {
				a.reportNavigationFailure(fetched.err)
			})
			if hasCache {
				// The cached tree is up and its list is already loading; a
				// second refresh would only repeat the failure.
				return
			}
			// No tree, but All Issues does not need one.
			a.QueueUpdateDraw(func() {
				a.refreshIssuesWithFocusChange(false)
			})
			return
		}

		if hasCache && !fetched.favoritesOK {
			// Favorites are fetched separately and their failure is not fatal,
			// but a tree missing them is not the tree: recording it would poison
			// the cache, and rebuilding from it would drop the Favorites section
			// out from under whoever is reading one.
			logger.Warning("tui.app: keeping the cached tree, favorites did not load")
			return
		}

		if hasCache && navDataUnchanged(cached, fetched.teams, fetched.favorites) {
			// What is on screen is what the fetch returned, so there is nothing
			// to rebuild and nothing new to write.
			logger.Debug("tui.app: cached navigation tree still current teams=%d favorites=%d", len(fetched.teams), len(fetched.favorites))
			return
		}

		if fetched.favoritesOK {
			a.recordNavCache(workspace, fetched.teams, fetched.favorites)
		}

		if hasCache {
			a.rebuildAroundCurrentPlace(ctx, pendingSession, fetched.teams, fetched.favorites, childFetchers)
			return
		}

		a.QueueUpdateDraw(func() {
			a.rebuildNavigationTree(fetched.teams, fetched.favorites)
		})
		a.openInitialList(ctx, pendingSession, fetched.teams, fetched.favorites, childFetchers)
	}()
}

// reportNavigationFailure surfaces a tree fetch that failed. The waiting node
// has to be answered too: a spinner frozen mid-frame over "Loading teams" reads
// as still working. UI thread only.
func (a *App) reportNavigationFailure(err error) {
	a.updateStatusBarWithError(err)
	if a.navLoadingNode != nil {
		a.navLoadingNode.SetText("Could not load teams")
	}
}

// openInitialList opens the list the app starts on: the saved place, else the
// configured default, else the unscoped list. Reports whether the saved place
// resolved, so a caller can keep it for a second attempt against fresher data.
// Call it off the UI thread; the restore fetches a team's children.
func (a *App) openInitialList(ctx context.Context, state *session.State, teams []linearapi.Team, favorites []linearapi.Favorite, fetchers teamChildFetchers) bool {
	// The session restore and the configured default each trigger their own
	// refresh after applying a selection, so only one of the three runs.
	if a.applySessionNavigation(ctx, state, teams, favorites, fetchers) {
		return true
	}
	if !a.applyDefaultNavigation(ctx, teams) {
		// Startup refresh must not steal focus from the navigation pane.
		a.QueueUpdateDraw(func() {
			a.refreshIssuesWithFocusChange(false)
		})
	}
	return false
}

// rebuildAroundCurrentPlace repaints the tree from freshly fetched data and
// puts the user back on the list they were reading. It only runs when the fetch
// disagrees with the cached copy already on screen, so the usual launch never
// reaches it and nothing moves.
func (a *App) rebuildAroundCurrentPlace(ctx context.Context, pending *session.State, teams []linearapi.Team, favorites []linearapi.Favorite, fetchers teamChildFetchers) {
	logger.Debug("tui.app: cached navigation tree is stale, rebuilding teams=%d favorites=%d", len(teams), len(favorites))
	a.QueueUpdateDraw(func() {
		// A saved place the cached tree could not resolve is worth another try
		// against the fresh one; otherwise the place to keep is the live one.
		state := pending
		if state == nil {
			snapshot := a.sessionSnapshot()
			state = &snapshot
		}
		a.rebuildNavigationTree(teams, favorites)
		go func() {
			if a.applySessionNavigation(ctx, state, teams, favorites, fetchers) {
				return
			}
			// The list the user was reading is not in the fetched tree. The
			// rebuild already left the cursor on All Issues; falling through to
			// the configured default the way a launch does would move them a
			// second time, for something they did not do.
			a.QueueUpdateDraw(func() {
				a.flashStatus("That list is no longer in this workspace")
				a.refreshIssuesWithFocusChange(false)
			})
		}()
	})
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
	a.resetNavigationTree()
	// The selection this zoom was opened on is going away, so the reading
	// view would be left holding the empty state with the list still hidden.
	a.detailsZoomed = false
	a.currentUser = nil
	a.teamUsers = nil
	a.teamProjects = nil
	a.workflowStates = nil
	a.teamCycles = nil
	a.teamLabels = nil
	a.metadataTeamID = ""
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
	a.pendingSearchIssueID = ""
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

	a.issuesErr = nil
	a.issuesSettled = false
	a.setIssuesLoading(false)
	a.setNavLoading(false)
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
	a.buildIssuesPlaceholder()
	// Create vertical flex for issues column
	a.issuesColumn = tview.NewFlex().SetDirection(tview.FlexRow)
	// All is the tab the app opens on; the others mount on a tab switch. It has
	// no rows yet, so what actually mounts is the placeholder.
	a.updateIssuesColumnLayout()
	a.detailsView = a.buildDetailsView()
	a.statusBar = a.buildStatusBar()

	// Horizontal split. rebuildContentLayout below owns the weights and which
	// panes are mounted.
	a.contentFlex = tview.NewFlex()

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
