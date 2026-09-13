package tui

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/agents"
	"github.com/praxis-labs-io/zen-linear/internal/cache"
	"github.com/praxis-labs-io/zen-linear/internal/config"
	"github.com/praxis-labs-io/zen-linear/internal/images"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
	"github.com/praxis-labs-io/zen-linear/internal/session"
	"github.com/rivo/tview"
)

type App struct {
	app *tview.Application
	linearDeps
	config    config.Config
	theme     Theme
	themeTags ThemeTags
	density   DensityProfile

	activeWorkspaceName string

	settingsPath string

	sessionPath    string
	pendingSession *session.State

	navCachePath string
	navCache     cache.NavFile
	navTeams     []linearapi.Team

	pages                 *tview.Pages
	mainLayout            *tview.Flex
	contentFlex           *tview.Flex
	navigationHidden      bool
	detailsHidden         bool
	detailsZoomed         bool
	zoomPreviousPane      FocusTarget
	zoomPreviousHidden    bool
	layoutMode            layoutMode
	layoutFocusStale      bool
	swallowingClick       bool
	palettePreviousPane   FocusTarget
	navigationPanel       *tview.Flex
	navSearchFrame        *tview.Flex
	navSearchInput        *tview.InputField
	navigationTree        *tview.TreeView
	navLoadingNode        *tview.TreeNode
	navNodeLabels         map[*tview.TreeNode]navNodeLabel
	favorites             []linearapi.Favorite
	allIssuesNode         *tview.TreeNode
	favoritesGroup        *tview.TreeNode
	teamsGroup            *tview.TreeNode
	listIssuesTable       *tview.Table
	searchResultsTable    *tview.Table
	issuesPlaceholder     *tview.Flex
	issuesPlaceholderText *tview.TextView
	issuesColumn          *tview.Flex
	detailsView           *tview.Flex
	detailsPageView       *tview.TextView
	detailsHeaderRows     []detailsRow
	detailsBodyLines      []string
	detailsBodyImages     []pageImage
	// glamour sizes tables to the width it was handed, so a width change re-renders from the raw text.
	detailsDescriptionMarkdown string
	detailsFittedWidth         int
	detailsFittedHeight        int
	detailsCommentsSource      []linearapi.Comment
	detailsActivitySource      []linearapi.IssueActivity
	detailsFieldSpans          []fieldSpan
	// Apart from the page because a draw may not write to the tty; only the after-draw handler may.
	graphics      *graphicsState
	pendingImages []screenImage
	// An id outlives the page it was drawn on: the terminal still holds the bytes it names.
	imageStore         *images.Store
	imageCache         map[string]*loadedImage
	imageIDs           uint32
	detailsEdit        detailsEditState
	detailsIssueID     string
	detailsChooserSpan chooserSpan
	editGeneration     atomic.Uint64
	detailsFieldInput  *tview.InputField
	detailsEditorSpan  editorSpan
	detailsDescArea    *tview.TextArea
	savingDescriptions map[uint64]struct{}
	focusedCommentID   string
	commentSpans       []commentSpan
	commentPainted     commentPaint
	detailsPage        *detailsPage
	detailsComposeArea *tview.TextArea
	detailsComposePost *tview.Button
	detailsReplyArea   *tview.TextArea
	detailsReplyPost   *tview.Button
	detailsEditArea    *tview.TextArea
	detailsEditPost    *tview.Button
	// Keyed by issue: the box is one widget over a moving selection, so a draft left in it would post to the wrong issue.
	composeDrafts        map[string]string
	composeDraftIssueID  string
	composeReplyTo       map[string]string
	replyDrafts          map[string]string
	composeEditing       map[string]string
	deletingComments     map[string]struct{}
	savingComments       map[string]struct{}
	statusBar            *tview.TextView
	statusToast          *tview.TextView
	statusRow            *tview.Flex
	statusRowWidth       int
	loadingMessage       string
	paletteModal         *tview.Flex
	paletteInput         *tview.InputField
	paletteSearchFrame   *tview.Flex
	paletteList          *tview.List
	paletteCtrl          *PaletteController
	bindings             *resolvedKeybindings
	pickerModal          *PickerModal
	issueFormModal       *IssueFormModal
	textInputModal       *TextInputModal
	multiSelectModal     *MultiSelectModal
	settingsModal        *SettingsModal
	promptTemplatesModal *AgentPromptTemplatesModal
	agentPromptModal     *AgentPromptModal
	agentOutputModal     *AgentOutputModal
	confirmationModal    *ConfirmationModal
	keysModal            *KeysModal
	agentRunner          *agents.Runner
	agentPromptTemplates []config.AgentPromptTemplate

	issuesMu            sync.RWMutex
	selectedIssue       *linearapi.Issue
	selectedNavigation  *NavigationNode
	issues              []linearapi.Issue
	focusedPane         FocusTarget
	activeIssuesSection IssuesSection

	listIssueRows         []IssueRow
	listIDToIssue         map[string]*linearapi.Issue
	expandedState         map[string]bool
	pendingSectionRenders map[IssuesSection]string

	richFilters          IssueFilters
	sortFields           []SortField
	configuredSortFields []SortField
	collapsedGroups      map[string]bool
	statusMessage        string
	pendingWarning       string
	pendingNotice        string
	warningReported      bool
	version              string
	checkUpdateFunc      checkForUpdate
	fileSettings         config.Settings
	envOverrides         config.EnvOverrides
	statusLevel          statusLevel

	viewPrefs          *viewDisplayPrefs
	groupingOverridden bool
	sortOverridden     bool

	searchQuery          string
	searchIssues         []linearapi.Issue
	searchIssueRows      []IssueRow
	searchIDToIssue      map[string]*linearapi.Issue
	navSearchFocused     bool
	restoringSession     bool
	searchLoading        bool
	searchErr            error
	pendingSearchIssueID string

	statusFlashTimer      *time.Timer
	statusFlashMu         sync.Mutex
	statusFlashGeneration atomic.Int64

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

	currentUser    *linearapi.User
	teamUsers      []linearapi.User
	teamProjects   []linearapi.Project
	workflowStates []linearapi.WorkflowState
	teamCycles     []linearapi.Cycle
	teamLabels     []linearapi.IssueLabel
	metadataTeamID string

	isLoading                      bool
	navLoading                     bool
	issuesErr                      error
	issuesSettled                  bool
	loadingGeneration              int64
	loading                        *loadingIndicator
	pendingRefresh                 bool
	pendingRefreshIssueID          string
	pendingRefreshAllowFocusChange bool
	refreshGeneration              atomic.Int64
	// Separate from refreshGeneration, which every refresh bumps and so cannot tell a workspace reset apart.
	resetGeneration atomic.Int64

	apiUseBearer      bool
	apiOnUnauthorized func(context.Context) (string, error)

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

	uiUpdateMu sync.Mutex

	detailsFocus detailsFocus
}

type FocusTarget int

const (
	FocusNavigation FocusTarget = iota
	FocusIssues
	FocusDetails
	FocusPalette
)

// UseSettingsFile sets the config file the settings modal saves back to.
func (a *App) UseSettingsFile(path string) { a.settingsPath = path }

// UseFileSettings records config.json's contents and the fields the environment overrides;
// the settings modal saves the file's value for an overridden field, never the environment's.
func (a *App) UseFileSettings(settings config.Settings, overrides config.EnvOverrides) {
	a.fileSettings = settings
	a.envOverrides = overrides
}

// WarnAtStartup holds a launch warning and shows it once the UI is up.
func (a *App) WarnAtStartup(warning string) { a.pendingWarning = warning }

func (a *App) reportPendingWarning() {
	if a.pendingWarning == "" {
		return
	}
	warning := a.pendingWarning
	a.pendingWarning = ""
	a.warningReported = true
	a.updateStatusBarWithError(errors.New(warning))
}

// UseVersion sets the build version the update check compares against; "dev" is never checked.
func (a *App) UseVersion(version string) { a.version = version }

func (a *App) reportPendingNotice() {
	if a.pendingNotice == "" || a.warningReported {
		return
	}
	notice := a.pendingNotice
	a.pendingNotice = ""
	a.updateStatusBarWithNotice(notice)
}

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
		listIDToIssue:        make(map[string]*linearapi.Issue),
		searchIDToIssue:      make(map[string]*linearapi.Issue),
		agentPromptTemplates: templates,
		graphics:             newGraphicsState(),
		activeWorkspaceName:  workspaceNameForKey(cfg.Workspaces, cfg.LinearAPIKey),
		detailsHidden:        true,
	}

	app.linearDeps = newLinearDeps(clientCfg, cfg.CacheTTL)
	app.rebuildImageStore(cfg.LinearAPIKey, clientCfg.UseBearer)
	app.apiUseBearer = clientCfg.UseBearer
	app.apiOnUnauthorized = clientCfg.OnUnauthorized
	app.rebuildCommands()
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

// Run starts the app and blocks until it exits, writing the session on the way out.
func (a *App) Run() error {
	a.app.SetRoot(a.pages, true).EnableMouse(true)

	a.loadInitialData()

	err := a.app.Run()
	if a.loading != nil {
		a.loading.stop()
	}
	a.cancelStatusFlash()
	a.clearImages()
	a.persistSession()
	return err
}

// Snapshots the fetch seams and generation first: applySettings and resetCachedState replace them while its goroutines run.
func (a *App) loadInitialData() {
	fetchUser := a.fetchCurrentUserFunc
	generation := a.resetGeneration.Load()
	pendingSession := a.consumePendingSession()
	childFetchers := a.teamChildFetchers()
	navFetchers := a.navFetchers()
	workspace := a.activeWorkspaceName
	cached, hasCache := a.cachedNavData()
	a.setNavLoading(true)
	a.startUpdateCheck()
	go func() {
		defer a.notifyNavigationSettled()
		started := time.Now()
		ctx := context.Background()

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
			a.reportPendingWarning()
		})
		if fetched.err != nil {
			logger.ErrorWithErr(fetched.err, "tui.app: failed to load teams")
			a.QueueUpdateDraw(func() {
				a.reportNavigationFailure(fetched.err)
			})
			if hasCache {
				return
			}
			a.QueueUpdateDraw(func() {
				a.refreshIssuesWithFocusChange(false)
			})
			return
		}

		if hasCache && !fetched.favoritesOK {
			logger.Warning("tui.app: keeping the cached tree, favorites did not load")
			return
		}

		if hasCache && navDataUnchanged(cached, fetched.teams, fetched.favorites) {
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

func (a *App) reportNavigationFailure(err error) {
	a.updateStatusBarWithError(err)
	if a.navLoadingNode != nil {
		a.navLoadingNode.SetText("Could not load teams")
	}
}

func (a *App) openInitialList(ctx context.Context, state *session.State, teams []linearapi.Team, favorites []linearapi.Favorite, fetchers teamChildFetchers) bool {
	if a.applySessionNavigation(ctx, state, teams, favorites, fetchers) {
		return true
	}
	if !a.applyDefaultNavigation(ctx, teams) {
		a.QueueUpdateDraw(func() {
			a.refreshIssuesWithFocusChange(false)
		})
	}
	return false
}

func (a *App) rebuildAroundCurrentPlace(ctx context.Context, pending *session.State, teams []linearapi.Team, favorites []linearapi.Favorite, fetchers teamChildFetchers) {
	logger.Debug("tui.app: cached navigation tree is stale, rebuilding teams=%d favorites=%d", len(teams), len(favorites))
	a.QueueUpdateDraw(func() {
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
			a.QueueUpdateDraw(func() {
				a.flashError("That list is no longer in this workspace")
				a.refreshIssuesWithFocusChange(false)
			})
		}()
	})
}

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

func (a *App) applySettings(newCfg config.Config) {
	old := a.config
	a.config = newCfg
	a.rebuildCommands()

	if newCfg.Theme != old.Theme || newCfg.Density != old.Density || newCfg.RoundedBorders != old.RoundedBorders {
		a.applyThemeAndDensity()
	}

	if newCfg.LogFile != old.LogFile || newCfg.LogLevel != old.LogLevel {
		opened, warning := logger.Restart(newCfg.LogFile, config.DefaultLogFile(), parseLogLevel(newCfg.LogLevel))
		if opened != "" {
			a.config.LogFile = opened
		}
		if warning != "" {
			logger.Warning("tui.app: %s", warning)
			a.pendingWarning = warning
		}
		logger.Debug("tui.app: settings applied log_file=%s log_level=%s", a.config.LogFile, newCfg.LogLevel)
	}

	if newCfg.Images != old.Images {
		a.rebuildImageStore(newCfg.LinearAPIKey, a.apiUseBearer)
		a.updateDetailsView()
	}

	if newCfg.LinearAPIKey != old.LinearAPIKey || newCfg.APIEndpoint != old.APIEndpoint || newCfg.Timeout != old.Timeout {
		if a.selectedNavigation != nil {
			snapshot := a.sessionSnapshot()
			a.pendingSession = &snapshot
		}
		logger.Debug("tui.app: connection changed, reloading the workspace")
		a.reloadWorkspace()
		return
	}

	if newCfg.CacheTTL != old.CacheTTL {
		a.rebuildLinearDeps()
	}

	a.restoreModalFocus()
	if newCfg.UpdateCheck && !old.UpdateCheck {
		a.startUpdateCheck()
	}
	a.reportPendingWarning()
}

func (a *App) rebuildLinearDeps() {
	a.linearDeps = newLinearDeps(linearapi.ClientConfig{
		Token:          a.config.LinearAPIKey,
		Endpoint:       a.config.APIEndpoint,
		Timeout:        a.config.Timeout,
		UseBearer:      a.apiUseBearer,
		OnUnauthorized: a.apiOnUnauthorized,
	}, a.config.CacheTTL)
}

func (a *App) reloadWorkspace() {
	a.rebuildLinearDeps()
	a.rebuildImageStore(a.config.LinearAPIKey, a.apiUseBearer)

	logger.Debug("tui.app: resetting cached state before reload")
	a.resetCachedState()
	a.loadInitialData()
	a.restoreModalFocus()
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

func (a *App) resetCachedState() {
	a.issuesMu.Lock()
	a.issues = nil
	a.listIssueRows = nil
	a.listIDToIssue = make(map[string]*linearapi.Issue)
	a.issuesMu.Unlock()

	a.selectedNavigation = nil
	a.resetNavigationTree()
	if a.detailsZoomed {
		a.releaseDetailsZoom()
		a.rebuildContentLayout()
	}
	a.currentUser = nil
	a.clearImages()
	a.imageCache = nil
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
	if a.navSearchInput != nil {
		a.navSearchInput.SetText("")
	}
	a.cancelSearchDebounce()
	a.clearSearchResults()
	a.searchQuery = ""
	a.navSearchFocused = false
	a.pendingSearchIssueID = ""
	a.activeIssuesSection = IssuesSectionList
	a.expandedState = make(map[string]bool)
	a.pendingSectionRenders = nil
	if a.listIssuesTable != nil {
		a.listIssuesTable.Clear()
	}
	a.updateIssuesColumnLayout()

	a.issuesErr = nil
	a.issuesSettled = false
	a.setIssuesLoading(false)
	a.setNavLoading(false)
	a.pendingRefresh = false
	a.pendingRefreshIssueID = ""
	a.pendingRefreshAllowFocusChange = true
	a.refreshGeneration.Add(1)
	a.resetGeneration.Add(1)
	a.clearSelectedIssue()
}

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

func (a *App) buildLayout() {
	a.navigationTree = a.buildNavigationTree()
	a.buildNavigationPanel()
	a.listIssuesTable = a.buildIssuesTable(IssuesSectionList)
	a.searchResultsTable = a.buildIssuesTable(IssuesSectionSearch)
	a.buildIssuesPlaceholder()
	a.issuesColumn = tview.NewFlex().SetDirection(tview.FlexRow)
	a.updateIssuesColumnLayout()
	a.detailsView = a.buildDetailsView()
	a.buildStatusBar()

	a.contentFlex = tview.NewFlex()

	a.mainLayout = tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(a.contentFlex, 0, 1, true).
		AddItem(a.statusRow, 1, 1, false)

	a.rebuildContentLayout()

	a.paletteModal = a.buildPaletteModal()

	a.pickerModal = NewPickerModal(a)
	a.issueFormModal = NewIssueFormModal(a)
	a.textInputModal = NewTextInputModal(a)
	a.multiSelectModal = NewMultiSelectModal(a)
	a.settingsModal = NewSettingsModal(a)
	a.promptTemplatesModal = NewAgentPromptTemplatesModal(a)
	a.agentPromptModal = NewAgentPromptModal(a)
	a.agentOutputModal = NewAgentOutputModal(a)
	a.confirmationModal = NewConfirmationModal(a)
	a.keysModal = NewKeysModal(a)
	a.agentRunner = agents.NewRunner()

	a.app.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		width, _ := screen.Size()
		a.watchLayoutWidth(width)
		a.beginImageFrame()
		return false
	})

	a.app.SetAfterDrawFunc(a.drawImages)

	a.pages.AddPage("main", a.mainLayout, true, true)
	a.pages.AddPage("palette", a.paletteModal, true, false)

	a.updateFocus()
}
