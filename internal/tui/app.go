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
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
	"github.com/praxis-labs-io/zen-linear/internal/session"
	"github.com/rivo/tview"
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

	// settingsPath is the config file this App launched from. Resolved once, so
	// an in-app save writes back to the file it was loaded from.
	settingsPath string

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
	pages                 *tview.Pages
	mainLayout            *tview.Flex
	contentFlex           *tview.Flex
	navigationHidden      bool
	detailsHidden         bool
	detailsZoomed         bool
	zoomPreviousPane      FocusTarget
	zoomPreviousHidden    bool
	layoutMode            layoutMode
	layoutFocusStale      bool // a breakpoint moved the panes; the keyboard has yet to follow
	swallowingClick       bool // a press was swallowed; its release and click go the same way
	palettePreviousPane   FocusTarget
	navigationPanel       *tview.Flex // Navigation pane shell: query box + tree
	navSearchFrame        *tview.Flex // Bordered frame around the query box
	navSearchInput        *tview.InputField
	navigationTree        *tview.TreeView
	navLoadingNode        *tview.TreeNode // "Loading teams" node, until a tree replaces it
	navNodeLabels         map[*tview.TreeNode]navNodeLabel
	favorites             []linearapi.Favorite
	allIssuesNode         *tview.TreeNode
	favoritesGroup        *tview.TreeNode
	teamsGroup            *tview.TreeNode
	listIssuesTable       *tview.Table
	searchResultsTable    *tview.Table
	issuesPlaceholder     *tview.Flex     // Stands in for a table with no rows
	issuesPlaceholderText *tview.TextView // Centered loading/empty/error message
	issuesColumn          *tview.Flex     // Vertical flex holding the active issues tab
	detailsView           *tview.Flex     // The details pane: border, title, and the page
	detailsPageView       *tview.TextView // The page's text, scrolled as one
	// detailsHeaderRows is the metadata untruncated and detailsBodyLines the
	// description rendered, so a refit re-joins them without rebuilding either.
	detailsHeaderRows []detailsRow
	detailsBodyLines  []string
	// Kept raw as well as rendered: glamour sizes tables to the width it was
	// handed, so a width change has to re-run it.
	detailsDescriptionMarkdown string
	detailsFittedWidth         int
	detailsFittedHeight        int
	detailsCommentsSource      []linearapi.Comment
	detailsActivitySource      []linearapi.IssueActivity
	// detailsFieldSpans is where each editable field landed in the last render,
	// the way commentSpans is for the cards.
	detailsFieldSpans []fieldSpan
	// detailsEdit is the field cursor's mode. detailsIssueID is what the page
	// was last built for, which is what says the issue changed under it.
	detailsEdit    detailsEditState
	detailsIssueID string
	// detailsChooserSpan is where the open chooser landed in the last render.
	// editGeneration stamps each opening of a chooser or a box.
	detailsChooserSpan chooserSpan
	editGeneration     atomic.Uint64
	// detailsFieldInput is the box a typed field is edited in, one widget for
	// all three. detailsEditorSpan is where it landed in the last render.
	detailsFieldInput *tview.InputField
	detailsEditorSpan editorSpan
	// detailsDescArea is the box the description is rewritten in.
	// savingDescriptions holds the generations whose write is in flight.
	detailsDescArea    *tview.TextArea
	savingDescriptions map[uint64]struct{}
	// focusedCommentID is the card the ring is on and commentSpans is where
	// every card landed in the last render. The ring is held by id rather than
	// by index so a refetch that reorders the stack keeps it on the same card.
	focusedCommentID string
	commentSpans     []commentSpan
	// commentPainted is the ring the last render drew, so a focus change
	// repaints only when it changes something on the page.
	commentPainted     commentPaint
	detailsPage        *detailsPage    // The issue, the cards, and the boxes drawn in them
	detailsComposeArea *tview.TextArea // Where a comment gets written
	detailsComposePost *tview.Button   // Sends what is in the compose box
	detailsReplyArea   *tview.TextArea // Where a reply gets written, inside its thread
	detailsReplyPost   *tview.Button   // Sends what is in the reply box
	detailsEditArea    *tview.TextArea // Where a comment gets rewritten, in place of its card
	detailsEditPost    *tview.Button   // Saves what is in the edit box
	// composeDrafts holds what has been written and not posted, keyed by issue.
	// The box is one widget over a changing selection, so a draft has to follow
	// the issue it was written for; left in the box it would be posted to
	// whichever issue is on screen when the chord lands.
	composeDrafts       map[string]string
	composeDraftIssueID string
	// composeReplyTo holds the thread the reply box is open on, keyed by issue
	// the same way the draft is: an issue keeps the box it had open when the
	// selection moves away and comes back.
	composeReplyTo map[string]string
	// replyDrafts holds what has been written into a reply box and not sent,
	// keyed by the comment being answered. Closing the box is not losing the
	// words, and reopening on the same thread finds them.
	replyDrafts map[string]string
	// composeEditing holds the comment being rewritten, keyed by issue the way
	// the reply box is. There is no draft map beside it: an edit box always
	// opens on the comment as it stands, so nobody is shown a half-edit from
	// last week in place of what the comment actually says.
	composeEditing map[string]string
	// deletingComments and savingComments hold the comments whose mutation is
	// still out. The card and the box both stay on the page until Linear
	// answers, so without these a second confirm or a second Ctrl+Enter inside
	// the round trip sends the mutation twice, and the loser reports on a
	// comment the winner has already changed.
	deletingComments map[string]struct{}
	savingComments   map[string]struct{}
	// statusBar holds the pane hints; statusToast is the message corner it
	// shares the statusRow strip with, and statusRowWidth is what the last
	// draw measured that strip at.
	statusBar      *tview.TextView
	statusToast    *tview.TextView
	statusRow      *tview.Flex
	statusRowWidth int
	// loadingMessage is a fetch's progress, shown in the same corner when
	// nothing has been flashed over it.
	loadingMessage     string
	paletteModal       *tview.Flex
	paletteInput       *tview.InputField
	paletteSearchFrame *tview.Flex
	paletteList        *tview.List
	paletteCtrl        *PaletteController
	// bindings is config.Keybindings after validation, set when the command
	// registry is built. Nil until then, and every read is nil-safe.
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

	// App state (protected by issuesMu)
	issuesMu            sync.RWMutex
	selectedIssue       *linearapi.Issue
	selectedNavigation  *NavigationNode
	issues              []linearapi.Issue
	focusedPane         FocusTarget
	activeIssuesSection IssuesSection // What the issues pane is showing: the list or search

	// Per-section issue tree state (for sub-issue hierarchy)
	listIssueRows []IssueRow                  // Flattened rows for the navigation list
	listIDToIssue map[string]*linearapi.Issue // Quick lookup by issue ID for the list
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
	pendingWarning       string
	pendingNotice        string
	// warningReported is what keeps a nudge from painting over a real
	// problem, since the two share the hint line and arrive out of order.
	warningReported bool
	// version is what the release workflow stamped, "dev" in a working tree.
	version string
	// checkUpdateFunc is the update seam, replaced by tests.
	checkUpdateFunc checkForUpdate
	fileSettings    config.Settings
	envOverrides    config.EnvOverrides
	statusLevel     statusLevel

	// Display settings of the active custom view, overriding config until
	// the user picks another list. The overridden flags keep in-session
	// manual grouping/sort choices ahead of the view's. UI-thread only.
	viewPrefs          *viewDisplayPrefs
	groupingOverridden bool
	sortOverridden     bool

	// Search state, independent from the main issues list. All fields are read
	// and written on the UI thread only.
	searchQuery      string
	searchIssues     []linearapi.Issue
	searchIssueRows  []IssueRow
	searchIDToIssue  map[string]*linearapi.Issue
	navSearchFocused bool // sub-focus within FocusNavigation: the query box, not the tree
	// restoringSession is up while a session restore picks the saved node, so
	// that pick does not drop the query the restore just put back.
	restoringSession bool
	searchLoading    bool
	searchErr        error
	// pendingSearchIssueID is the restored session's issue, selected once when
	// the first search results land.
	pendingSearchIssueID string

	// statusFlashTimer takes a one-off message back off the bar. Each flash
	// owns a generation so a later one is never cleared on an older clock.
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

	// detailsFocus is what inside the details page holds the keyboard: a card,
	// one of the writing boxes, or its button.
	detailsFocus detailsFocus
}

// FocusTarget indicates which pane has focus.
type FocusTarget int

const (
	FocusNavigation FocusTarget = iota
	FocusIssues
	FocusDetails
	FocusPalette
)

// UseSettingsFile installs the config file the settings modal saves back to.
func (a *App) UseSettingsFile(path string) { a.settingsPath = path }

// UseFileSettings records what config.json holds and which fields the
// environment took over. The settings modal shows the effective value but
// saves the file's, so an ephemeral variable cannot become a stored setting.
func (a *App) UseFileSettings(settings config.Settings, overrides config.EnvOverrides) {
	a.fileSettings = settings
	a.envOverrides = overrides
}

// WarnAtStartup holds a launch problem worth telling the user about once the UI
// is up. Anything printed before Run is painted over the moment tcell takes the
// tty, so a warning that only reached stderr is a warning nobody read.
func (a *App) WarnAtStartup(warning string) { a.pendingWarning = warning }

// reportPendingWarning shows a held warning and forgets it, so a later reload
// does not repeat it. It runs where the launch or reload has settled, since
// anything set before that is painted over by the loading chatter, and it goes
// on the hint line rather than the toast corner, which truncates to half the
// row and would drop the half that says what happened.
func (a *App) reportPendingWarning() {
	if a.pendingWarning == "" {
		return
	}
	warning := a.pendingWarning
	a.pendingWarning = ""
	a.warningReported = true
	a.updateStatusBarWithError(errors.New(warning))
}

// UseVersion records what the release workflow stamped into this build, which
// is what the update check compares against. Nothing else in the app reads it:
// a working tree reports "dev" and is not behind anything.
func (a *App) UseVersion(version string) { a.version = version }

// reportPendingNotice shows a held notice and forgets it. A launch warning
// takes the line ahead of it and is not displaced: a log the app could not open
// is a problem, where this is a nudge that keeps until the next launch.
func (a *App) reportPendingNotice() {
	if a.pendingNotice == "" || a.warningReported {
		return
	}
	notice := a.pendingNotice
	a.pendingNotice = ""
	a.updateStatusBarWithNotice(notice)
}

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
		listIDToIssue:        make(map[string]*linearapi.Issue),
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

// Run starts the application and blocks until it exits.
func (a *App) Run() error {
	a.app.SetRoot(a.pages, true).EnableMouse(true)

	// Load initial data asynchronously
	a.loadInitialData()

	// Start the application event loop
	err := a.app.Run()
	// The frame loop and a flash still counting down would otherwise outlive
	// the event loop, queueing draws nothing is left to run.
	if a.loading != nil {
		a.loading.stop()
	}
	a.cancelStatusFlash()
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
	// Off the launch's critical path: nothing below waits on it, and it
	// reports through the status bar whenever it answers.
	a.startUpdateCheck()
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
			a.reportPendingWarning()
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
				a.flashError("That list is no longer in this workspace")
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
	// Keybindings are resolved once, when the registry is built, so a saved
	// change to them only lands if the registry is rebuilt with it.
	a.rebuildCommands()
	a.applyThemeAndDensity()

	logLevel := parseLogLevel(newCfg.LogLevel)
	opened, warning := logger.Restart(newCfg.LogFile, config.DefaultLogFile(), logLevel)
	// a.config is already newCfg; adopt the path actually opened so the settings
	// modal names where logs really go rather than the path that was refused.
	// Logging off is not adopted: it is where this save landed, not a setting,
	// and writing it back would turn one bad path into logging off for good.
	if opened != "" {
		newCfg.LogFile = opened
		a.config.LogFile = opened
	}
	if warning != "" {
		logger.Warning("tui.app: %s", warning)
		// Held rather than shown: the reload this ends in paints the hint line
		// back over anything set here. It surfaces when that reload settles.
		a.pendingWarning = warning
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
	a.listIssueRows = nil
	a.listIDToIssue = make(map[string]*linearapi.Issue)
	a.issuesMu.Unlock()

	a.selectedNavigation = nil
	a.resetNavigationTree()
	// The selection this zoom was opened on is going away, so the reading
	// view would be left holding the empty state with the list still hidden.
	if a.detailsZoomed {
		a.releaseDetailsZoom()
		a.rebuildContentLayout()
	}
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
	// Emptying the box fires its change handler, which arms a debounce, so the
	// cancel has to come after it. Canceled first, a stray search for the
	// empty string lands a quarter second later and calls updateFocus, which
	// can put the keyboard on a pane while a modal is still on screen.
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
	// Clearing the models is not enough: an off-screen section keeps its
	// painted cells until something repaints it, and dropping the pending
	// markers removes the only thing that would have.
	a.pendingSectionRenders = nil
	if a.listIssuesTable != nil {
		a.listIssuesTable.Clear()
	}
	// The reset moved the pane back to the list, and the column is still
	// mounting whatever was on screen.
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
	a.buildNavigationPanel()
	a.listIssuesTable = a.buildIssuesTable(IssuesSectionList)
	a.searchResultsTable = a.buildIssuesTable(IssuesSectionSearch)
	a.buildIssuesPlaceholder()
	// Create vertical flex for issues column
	a.issuesColumn = tview.NewFlex().SetDirection(tview.FlexRow)
	// The list is what the app opens on, and it has no rows yet, so what
	// actually mounts is the placeholder.
	a.updateIssuesColumnLayout()
	a.detailsView = a.buildDetailsView()
	a.buildStatusBar()

	// Horizontal split. rebuildContentLayout below owns the weights and which
	// panes are mounted.
	a.contentFlex = tview.NewFlex()

	// Create vertical layout: content + status bar
	a.mainLayout = tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(a.contentFlex, 0, 1, true).
		AddItem(a.statusRow, 1, 1, false)

	// Apply initial pane visibility (details is hidden by default).
	a.rebuildContentLayout()

	// Build palette modal
	a.paletteModal = a.buildPaletteModal()

	// Build picker and create issue modals
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
