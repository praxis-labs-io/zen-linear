package tui

import (
	"cmp"
	"context"
	"fmt"
	"sort"
	"strings"
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
)

// SortField represents a field to sort issues by.
type SortField string

const (
	SortByUpdatedAt SortField = "updatedAt"
	SortByCreatedAt SortField = "createdAt"
	SortByPriority  SortField = "priority"
	SortByStatus    SortField = "status"
)

// IssueFilters contains structured filters applied in addition to navigation.
type IssueFilters struct {
	AssigneeID   string
	AssigneeName string
	LabelIDs     []string
	LabelNames   []string
	StateID      string
	StateName    string
	ProjectID    string
	ProjectName  string
	CycleID      string
	CycleName    string
	DueDate      linearapi.DateFilter
	Estimate     linearapi.NumberFilter
}

func (f IssueFilters) Empty() bool {
	return f.AssigneeID == "" &&
		len(f.LabelIDs) == 0 &&
		f.StateID == "" &&
		f.ProjectID == "" &&
		f.CycleID == "" &&
		f.DueDate.Empty() &&
		f.Estimate.Empty()
}

func (f IssueFilters) Summary() string {
	parts := make([]string, 0, 8)
	if f.AssigneeID != "" {
		label := f.AssigneeName
		if label == "" {
			label = f.AssigneeID
		}
		parts = append(parts, "assignee="+label)
	}
	if len(f.LabelIDs) > 0 {
		labels := f.LabelNames
		if len(labels) == 0 {
			labels = f.LabelIDs
		}
		parts = append(parts, "labels="+strings.Join(labels, ","))
	}
	if f.StateID != "" {
		label := f.StateName
		if label == "" {
			label = f.StateID
		}
		parts = append(parts, "status="+label)
	}
	if f.ProjectID != "" {
		label := f.ProjectName
		if label == "" {
			label = f.ProjectID
		}
		parts = append(parts, "project="+label)
	}
	if f.CycleID != "" {
		label := f.CycleName
		if label == "" {
			label = f.CycleID
		}
		parts = append(parts, "cycle="+label)
	}
	if !f.DueDate.Empty() {
		parts = append(parts, "due="+formatDateFilterSummary(f.DueDate))
	}
	if !f.Estimate.Empty() {
		parts = append(parts, "estimate="+formatNumberFilterSummary(f.Estimate))
	}
	return strings.Join(parts, ", ")
}

func formatDateFilterSummary(filter linearapi.DateFilter) string {
	switch {
	case filter.Eq != "":
		return filter.Eq
	case filter.GTE != "":
		return ">=" + filter.GTE
	case filter.GT != "":
		return ">" + filter.GT
	case filter.LTE != "":
		return "<=" + filter.LTE
	case filter.LT != "":
		return "<" + filter.LT
	case filter.Null != nil && *filter.Null:
		return "none"
	case filter.Null != nil:
		return "set"
	default:
		return ""
	}
}

func formatNumberFilterSummary(filter linearapi.NumberFilter) string {
	switch {
	case filter.Eq != nil:
		return formatEstimate(filter.Eq)
	case filter.GTE != nil:
		return ">=" + formatEstimate(filter.GTE)
	case filter.GT != nil:
		return ">" + formatEstimate(filter.GT)
	case filter.LTE != nil:
		return "<=" + formatEstimate(filter.LTE)
	case filter.LT != nil:
		return "<" + formatEstimate(filter.LT)
	case filter.Null != nil && *filter.Null:
		return "none"
	case filter.Null != nil:
		return "set"
	default:
		return ""
	}
}

// App is the main application controller that manages all UI components.
type App struct {
	app       *tview.Application
	api       *linearapi.Client
	cache     *cache.TeamCache
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
	navNodeOriginalText    map[*tview.TreeNode]string
	favorites              []linearapi.Favorite
	favoritesGroup         *tview.TreeNode
	issuesTable            *tview.Table // Legacy - kept for backward compatibility during migration
	myIssuesTable          *tview.Table
	otherIssuesTable       *tview.Table
	allIssuesTable         *tview.Table
	searchInput            *tview.InputField
	searchResultsTable     *tview.Table
	searchPanel            *tview.Flex     // Search tab shell: input row + body
	searchBody             *tview.Flex     // Swappable slot: results table or placeholder
	searchPlaceholder      *tview.TextView // Centered empty/loading/error message
	issuesColumn           *tview.Flex     // Vertical flex containing My/Other tables
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
	createIssueModal       *CreateIssueModal
	createCommentModal     *CreateCommentModal
	editTitleModal         *EditTitleModal
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
	activeIssuesSection IssuesSection // Tracks which issues section (My/Other) is currently active

	// Issue tree state (for sub-issue hierarchy)
	// Legacy fields - kept for backward compatibility during migration
	issueRows []IssueRow                  // Flattened rows for table rendering
	idToIssue map[string]*linearapi.Issue // Quick lookup by issue ID
	// Per-section issue tree state
	myIssueRows    []IssueRow                  // Flattened rows for "My Issues" table
	myIDToIssue    map[string]*linearapi.Issue // Quick lookup by issue ID for "My Issues"
	otherIssueRows []IssueRow                  // Flattened rows for "Other Issues" table
	otherIDToIssue map[string]*linearapi.Issue // Quick lookup by issue ID for "Other Issues"
	expandedState  map[string]bool             // Expanded state for parent issues (shared across sections)
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

	// Cached metadata for currently selected team
	currentUser    *linearapi.User
	teamUsers      []linearapi.User
	teamProjects   []linearapi.Project
	workflowStates []linearapi.WorkflowState
	teamCycles     []linearapi.Cycle

	// Loading state
	isLoading                      bool
	pendingRefresh                 bool
	pendingRefreshIssueID          string
	pendingRefreshAllowFocusChange bool
	pickerActive                   bool
	refreshGeneration              atomic.Int64

	// Lazy loading helpers (overridable in tests)
	fetchIssuesPage         func(context.Context, linearapi.FetchIssuesParams, *string) (linearapi.IssuePage, error)
	fetchIssueByID          func(context.Context, string) (linearapi.Issue, error)
	issueMatchesScopeFunc   func(context.Context, linearapi.FetchIssuesParams, string) (bool, error)
	fetchViewPrefsFunc      func(context.Context, string) (*linearapi.ViewPreferencesValues, error)
	queueUpdateDraw         func(func())
	updateIssueFunc         func(context.Context, linearapi.UpdateIssueInput) (linearapi.Issue, error)
	createIssueRelationFunc func(context.Context, linearapi.CreateIssueRelationInput) (linearapi.IssueRelation, error)
	deleteIssueRelationFunc func(context.Context, string) error
	subscribeIssueFunc      func(context.Context, string) (linearapi.Issue, error)
	unsubscribeIssueFunc    func(context.Context, string) (linearapi.Issue, error)
	openURLFunc             func(string) error
	copyToClipboardFunc     func(string) error
	refreshCompleted        func()
	fetchProjectsFunc       func(context.Context, string) ([]linearapi.Project, error)
	fetchWorkflowStatesFunc func(context.Context, string) ([]linearapi.WorkflowState, error)
	fetchCyclesFunc         func(context.Context, string) ([]linearapi.Cycle, error)
	preloadTeamMetadataFunc func(string)

	createFavoriteFunc     func(context.Context, linearapi.FavoriteTarget) (linearapi.Favorite, error)
	deleteFavoriteFunc     func(context.Context, string) error
	updateFavoriteSortFunc func(context.Context, string, float64) error
	moveFavoriteFunc       func(context.Context, string, string, float64) error
	favoritesChanged       func()

	// UI update mutex (for test safety when queueUpdateDraw executes immediately)
	uiUpdateMu sync.Mutex

	// Race-safety for issue detail fetching
	fetchingIssueID string // Tracks which issue ID we're currently fetching

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
func NewApp(api *linearapi.Client, cfg config.Config, templates []config.AgentPromptTemplate) *App {
	if len(templates) == 0 {
		templates = config.DefaultAgentPromptTemplates()
	}
	theme := ResolveTheme(cfg.Theme)
	density := ResolveDensity(cfg.Density)
	initMarkdownRenderer(theme)

	app := &App{
		app:                  tview.NewApplication(),
		api:                  api,
		cache:                cache.NewTeamCache(api, cfg.CacheTTL),
		config:               cfg,
		theme:                theme,
		themeTags:            NewThemeTags(theme),
		density:              density,
		pages:                tview.NewPages(),
		focusedPane:          FocusNavigation,
		sortFields:           parseSortFields(cfg.SortBy),
		configuredSortFields: parseSortFields(cfg.SortBy),
		expandedState:        make(map[string]bool),
		navNodeOriginalText:  make(map[*tview.TreeNode]string),
		idToIssue:            make(map[string]*linearapi.Issue),
		myIDToIssue:          make(map[string]*linearapi.Issue),
		otherIDToIssue:       make(map[string]*linearapi.Issue),
		searchIDToIssue:      make(map[string]*linearapi.Issue),
		activeIssuesSection:  IssuesSectionMy, // Default to My Issues (falls back to Other when empty)
		agentPromptTemplates: templates,
		activeWorkspaceName:  workspaceNameForKey(cfg.Workspaces, cfg.LinearAPIKey),
		// Details opens on demand (Enter or the palette toggle); the list
		// gets the room.
		detailsHidden: true,
	}

	app.paletteCtrl = NewPaletteController(DefaultCommands(app))
	app.fetchIssuesPage = api.FetchIssuesPage
	app.fetchIssueByID = api.FetchIssueByID
	app.fetchViewPrefsFunc = api.FetchCustomViewPreferences
	app.updateIssueFunc = api.UpdateIssue
	app.createIssueRelationFunc = api.CreateIssueRelation
	app.deleteIssueRelationFunc = api.DeleteIssueRelation
	app.subscribeIssueFunc = api.SubscribeToIssue
	app.unsubscribeIssueFunc = api.UnsubscribeFromIssue
	app.openURLFunc = openURL
	app.copyToClipboardFunc = copyToClipboard
	app.fetchProjectsFunc = app.cache.GetProjects
	app.fetchWorkflowStatesFunc = app.cache.GetWorkflowStates
	app.fetchCyclesFunc = app.cache.GetCycles
	app.preloadTeamMetadataFunc = app.preloadTeamMetadata
	app.createFavoriteFunc = app.api.CreateFavorite
	app.deleteFavoriteFunc = app.api.DeleteFavorite
	app.updateFavoriteSortFunc = app.api.UpdateFavoriteSortOrder
	app.moveFavoriteFunc = app.api.MoveFavorite
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
			user, err := a.cache.GetCurrentUser(ctx)
			if err != nil {
				logger.Warning("tui.app: failed to load current user error=%v", err)
				return
			}
			a.currentUser = &user
			logger.Debug("tui.app: current user loaded user=%s", user.DisplayName)
		}()

		teams, favorites, err := a.fetchNavigationData(ctx)
		wg.Wait()
		logger.Debug("tui.app: startup fetches completed elapsed=%s", time.Since(started))
		if err != nil {
			logger.ErrorWithErr(err, "tui.app: failed to load teams")
			a.app.QueueUpdateDraw(func() {
				a.updateStatusBarWithError(err)
			})
			// No tree, but All Issues does not need one.
			a.refreshIssuesWithFocusChange(false)
			return
		}

		a.app.QueueUpdateDraw(func() {
			a.rebuildNavigationTree(teams, favorites)
		})

		// Default navigation triggers its own refresh after applying the
		// configured selection.
		if !a.applyDefaultNavigation(ctx, teams) {
			// Startup refresh must not steal focus from the navigation pane.
			a.refreshIssuesWithFocusChange(false)
		}
	}()
}

// applySettings updates runtime dependencies to match a new configuration.
func (a *App) applySettings(newCfg config.Config) {
	a.config = newCfg
	a.applyThemeAndDensity()

	logLevel := parseLogLevel(newCfg.LogLevel)
	if err := logger.Reinit(newCfg.LogFile, logLevel); err != nil {
		logger.ErrorWithErr(err, "tui.app: failed to reinitialize logger")
		a.QueueUpdateDraw(func() {
			a.updateStatusBarWithError(err)
		})
		return
	}
	logger.Debug("tui.app: settings applied log_file=%s log_level=%s", newCfg.LogFile, newCfg.LogLevel)

	a.api = linearapi.NewClient(linearapi.ClientConfig{
		Token:    newCfg.LinearAPIKey,
		Endpoint: newCfg.APIEndpoint,
		Timeout:  newCfg.Timeout,
	})
	a.cache = cache.NewTeamCache(a.api, newCfg.CacheTTL)
	a.fetchIssuesPage = a.api.FetchIssuesPage
	a.fetchIssueByID = a.api.FetchIssueByID
	a.fetchViewPrefsFunc = a.api.FetchCustomViewPreferences
	a.updateIssueFunc = a.api.UpdateIssue
	a.createIssueRelationFunc = a.api.CreateIssueRelation
	a.deleteIssueRelationFunc = a.api.DeleteIssueRelation
	a.subscribeIssueFunc = a.api.SubscribeToIssue
	a.unsubscribeIssueFunc = a.api.UnsubscribeFromIssue
	a.fetchProjectsFunc = a.cache.GetProjects
	a.fetchWorkflowStatesFunc = a.cache.GetWorkflowStates
	a.fetchCyclesFunc = a.cache.GetCycles
	a.createFavoriteFunc = a.api.CreateFavorite
	a.deleteFavoriteFunc = a.api.DeleteFavorite
	a.updateFavoriteSortFunc = a.api.UpdateFavoriteSortOrder
	a.moveFavoriteFunc = a.api.MoveFavorite

	logger.Debug("tui.app: resetting cached state after settings change")
	a.resetCachedState()
	a.loadInitialData()
}

func (a *App) applyThemeAndDensity() {
	a.theme = ResolveTheme(a.config.Theme)
	a.themeTags = NewThemeTags(a.theme)
	a.density = ResolveDensity(a.config.Density)
	initMarkdownRenderer(a.theme)

	a.applyThemeStyles()
	a.applyThemeToComponents()
	a.applyDensityToComponents()
	a.rebuildModals()
	a.updateStatusBar()
	a.updateDetailsView()
	a.updatePaletteList()
}

func (a *App) applyThemeStyles() {
	tview.Styles.PrimitiveBackgroundColor = a.theme.Background
	tview.Styles.ContrastBackgroundColor = a.theme.Background
	tview.Styles.MoreContrastBackgroundColor = a.theme.HeaderBg
	tview.Styles.BorderColor = a.theme.Border
	tview.Styles.TitleColor = a.theme.Foreground
	tview.Styles.GraphicsColor = a.theme.Border
	tview.Styles.PrimaryTextColor = a.theme.Foreground
	tview.Styles.SecondaryTextColor = a.theme.SecondaryText
	tview.Styles.TertiaryTextColor = a.theme.SecondaryText
	tview.Styles.InverseTextColor = a.theme.InverseTextColor()
	tview.Styles.ContrastSecondaryTextColor = a.theme.SecondaryText

	// Square by default; the setting swaps in rounded corner runes. Both
	// branches assign so toggling the setting at runtime restores either look.
	if a.config.RoundedBorders {
		tview.Borders.TopLeft = '\u256d'     // ╭
		tview.Borders.TopRight = '\u256e'    // ╮
		tview.Borders.BottomLeft = '\u2570'  // ╰
		tview.Borders.BottomRight = '\u256f' // ╯
	} else {
		tview.Borders.TopLeft = tview.BoxDrawingsLightDownAndRight
		tview.Borders.TopRight = tview.BoxDrawingsLightDownAndLeft
		tview.Borders.BottomLeft = tview.BoxDrawingsLightUpAndRight
		tview.Borders.BottomRight = tview.BoxDrawingsLightUpAndLeft
	}

	// Focused panes are already highlighted via BorderFocus; keep single-line
	// borders instead of tview's default double-line focus runes.
	tview.Borders.HorizontalFocus = tview.Borders.Horizontal
	tview.Borders.VerticalFocus = tview.Borders.Vertical
	tview.Borders.TopLeftFocus = tview.Borders.TopLeft
	tview.Borders.TopRightFocus = tview.Borders.TopRight
	tview.Borders.BottomLeftFocus = tview.Borders.BottomLeft
	tview.Borders.BottomRightFocus = tview.Borders.BottomRight
}

func (a *App) applyThemeToComponents() {
	if a.navigationTree != nil {
		a.navigationTree.SetBackgroundColor(a.theme.Background).
			SetBorderColor(a.theme.Border).
			SetTitleColor(a.theme.Foreground)
		a.recolorNavigationTree()
	}

	if a.myIssuesTable != nil {
		a.applyIssuesTableTheme(a.myIssuesTable)
		renderIssuesTableModel(a.myIssuesTable, a.myIssueRows, a.myIDToIssue, a.selectedIssueID(IssuesSectionMy), a.theme, a.issueColumns())
	}
	if a.otherIssuesTable != nil {
		a.applyIssuesTableTheme(a.otherIssuesTable)
		renderIssuesTableModel(a.otherIssuesTable, a.otherIssueRows, a.otherIDToIssue, a.selectedIssueID(IssuesSectionOther), a.theme, a.issueColumns())
	}
	if a.allIssuesTable != nil {
		a.applyIssuesTableTheme(a.allIssuesTable)
		renderIssuesTableModel(a.allIssuesTable, a.issueRows, a.idToIssue, a.selectedIssueID(IssuesSectionAll), a.theme, a.issueColumns())
	}
	if a.searchPanel != nil {
		// Rebuild the panel so the input picks up the new InputBg (tview
		// bakes it at construction), then restyle and re-render the results.
		a.applyIssuesTableTheme(a.searchResultsTable)
		a.buildSearchPanel()
		renderIssuesTableModel(a.searchResultsTable, a.searchIssueRows, a.searchIDToIssue, a.selectedIssueID(IssuesSectionSearch), a.theme, a.issueColumns())
		a.updateIssuesColumnLayout()
	}

	if a.detailsDescriptionView != nil {
		a.detailsDescriptionView.SetTitleColor(a.theme.Foreground).
			SetBorderColor(a.theme.Border).
			SetBackgroundColor(a.theme.Background)
	}
	if a.detailsCommentsView != nil {
		a.detailsCommentsView.SetTitleColor(a.theme.Foreground).
			SetBorderColor(a.theme.Border).
			SetBackgroundColor(a.theme.Background)
	}

	if a.statusBar != nil {
		a.statusBar.SetBackgroundColor(a.theme.HeaderBg)
	}
}

func (a *App) applyDensityToComponents() {
	if a.detailsDescriptionView != nil {
		padding := a.density.DetailsPadding
		a.detailsDescriptionView.SetBorderPadding(padding.Top, padding.Bottom, padding.Left, padding.Right)
	}
	if a.detailsCommentsView != nil {
		padding := a.density.DetailsPadding
		a.detailsCommentsView.SetBorderPadding(padding.Top, padding.Bottom, padding.Left, padding.Right)
	}
	if a.statusBar != nil {
		padding := a.density.StatusBarPadding
		a.statusBar.SetBorderPadding(padding.Top, padding.Bottom, padding.Left, padding.Right)
	}
	if a.agentOutputModal != nil {
		a.agentOutputModal.ApplyDensity(a.density)
	}
}

func (a *App) rebuildModals() {
	if a.pages != nil {
		a.pages.RemovePage("palette")
	}
	a.paletteModal = a.buildPaletteModal()
	if a.pages != nil {
		a.pages.AddPage("palette", a.paletteModal, true, false)
	}

	a.pickerModal = NewPickerModal(a)
	a.createIssueModal = NewCreateIssueModal(a)
	a.createCommentModal = NewCreateCommentModal(a)
	a.editTitleModal = NewEditTitleModal(a)
	a.editDescriptionModal = NewEditDescriptionModal(a)
	a.editLabelsModal = NewEditLabelsModal(a)
	a.textInputModal = NewTextInputModal(a)
	a.multiSelectModal = NewMultiSelectModal(a)
	a.settingsModal = NewSettingsModal(a)
	a.promptTemplatesModal = NewAgentPromptTemplatesModal(a)
	a.agentPromptModal = NewAgentPromptModal(a)
	if a.pages == nil || !a.pages.HasPage("agent_output") {
		a.agentOutputModal = NewAgentOutputModal(a)
	} else {
		a.agentOutputModal.ApplyTheme(a.theme)
		a.agentOutputModal.ApplyDensity(a.density)
	}
	a.confirmationModal = NewConfirmationModal(a)
}

func (a *App) applyIssuesTableTheme(table *tview.Table) {
	if table == nil {
		return
	}
	table.SetTitleColor(a.theme.Foreground).
		SetBorderColor(a.theme.Border).
		SetBackgroundColor(a.theme.Background)
	table.SetSelectedStyle(tcell.StyleDefault.
		Foreground(a.theme.SelectionText).
		Background(a.theme.SelectionBg).
		Bold(true))
}

func (a *App) recolorNavigationTree() {
	if a.navigationTree == nil {
		return
	}
	root := a.navigationTree.GetRoot()
	if root == nil {
		return
	}
	a.applyNavigationNodeColors(root)
}

func (a *App) applyNavigationNodeColors(node *tview.TreeNode) {
	if node == nil {
		return
	}
	ref := node.GetReference()
	if ref == nil {
		node.SetColor(a.theme.Accent)
	} else if navNode, ok := ref.(*NavigationNode); ok {
		if navNode.IsProject || navNode.IsStatus {
			node.SetColor(a.theme.SecondaryText)
		} else {
			node.SetColor(a.theme.Foreground)
		}
	}
	node.SetSelectedTextStyle(a.selectionStyle())
	for _, child := range node.GetChildren() {
		a.applyNavigationNodeColors(child)
	}
}

// selectionStyle is the selected-row style shared by the tree and tables.
// tview's default inverse-video selection paints text in the primitive
// background color, which is unreadable for themes with a transparent
// background.
func (a *App) selectionStyle() tcell.Style {
	return tcell.StyleDefault.
		Foreground(a.theme.SelectionText).
		Background(a.theme.SelectionBg).
		Bold(true)
}

// listSelectionStyle is the stronger accent selection used by modal lists
// (command palette, pickers), where the selected row is the primary object on
// screen and must stand apart from input fields and panel fills.
func (a *App) listSelectionStyle() tcell.Style {
	return tcell.StyleDefault.
		Foreground(a.theme.InverseTextColor()).
		Background(a.theme.Accent).
		Bold(true)
}

// applySelectionStyleToTree sets the shared selection style on a node subtree
// without touching node colors.
func (a *App) applySelectionStyleToTree(node *tview.TreeNode) {
	if node == nil {
		return
	}
	node.SetSelectedTextStyle(a.selectionStyle())
	for _, child := range node.GetChildren() {
		a.applySelectionStyleToTree(child)
	}
}

func (a *App) selectedIssueID(section IssuesSection) string {
	var table *tview.Table
	switch section {
	case IssuesSectionMy:
		table = a.myIssuesTable
	case IssuesSectionOther:
		table = a.otherIssuesTable
	case IssuesSectionAll:
		table = a.allIssuesTable
	case IssuesSectionSearch:
		table = a.searchResultsTable
	}
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
	a.selectedIssue = nil
	a.issues = nil
	a.issueRows = nil
	a.idToIssue = make(map[string]*linearapi.Issue)
	a.myIssueRows = nil
	a.myIDToIssue = make(map[string]*linearapi.Issue)
	a.otherIssueRows = nil
	a.otherIDToIssue = make(map[string]*linearapi.Issue)
	a.issuesMu.Unlock()

	a.selectedNavigation = nil
	a.currentUser = nil
	a.teamUsers = nil
	a.teamProjects = nil
	a.workflowStates = nil
	a.teamCycles = nil
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
	a.searchReturnSection = IssuesSectionMy
	a.activeIssuesSection = IssuesSectionOther
	a.expandedState = make(map[string]bool)

	a.isLoading = false
	a.pendingRefresh = false
	a.pendingRefreshIssueID = ""
	a.pendingRefreshAllowFocusChange = true
	// Bump generation to prevent in-flight refreshes from updating UI.
	a.refreshGeneration.Add(1)
	a.fetchingIssueID = ""
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

// fetchNavigationData fetches the teams and favorites the navigation tree is
// built from. A favorites failure is not fatal: the tree renders without them.
func (a *App) fetchNavigationData(ctx context.Context) ([]linearapi.Team, []linearapi.Favorite, error) {
	var teams []linearapi.Team
	var favorites []linearapi.Favorite
	var teamsErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		teams, teamsErr = a.cache.GetTeams(ctx)
	}()
	go func() {
		defer wg.Done()
		fetched, err := a.api.ListFavorites(ctx)
		if err != nil {
			logger.ErrorWithErr(err, "tui.app: failed to load favorites")
			return
		}
		favorites = fetched
	}()
	wg.Wait()

	if teamsErr != nil {
		return nil, nil, teamsErr
	}

	logger.Debug("tui.app: loaded teams count=%d favorites_count=%d", len(teams), len(favorites))
	return teams, favorites, nil
}

// rebuildNavigationTree rebuilds the navigation tree with real data.
func (a *App) rebuildNavigationTree(teams []linearapi.Team, favorites []linearapi.Favorite) {
	a.navNodeOriginalText = make(map[*tview.TreeNode]string)
	a.favorites = favorites
	rootLabel := "Linear"
	if a.activeWorkspaceName != "" {
		rootLabel = "Linear · " + a.activeWorkspaceName
	}
	root := tview.NewTreeNode(rootLabel).
		SetColor(a.theme.Accent).
		SetSelectable(false)

	// Add "All Issues" at the top
	allIssues := tview.NewTreeNode("All Issues").
		SetColor(a.theme.Foreground).
		SetReference(&NavigationNode{ID: "all", Text: "All Issues"}).
		SetExpanded(true)
	root.AddChild(allIssues)

	a.appendFavoritesSection(root, favorites)

	// Add teams
	for _, team := range teams {
		teamNode := tview.NewTreeNode(team.Name).
			SetColor(a.theme.Foreground).
			SetReference(&NavigationNode{
				ID:     team.ID,
				Text:   team.Name,
				IsTeam: true,
				TeamID: team.ID,
			}).
			SetExpanded(false)

		// Note: Team selection is handled by the tree's SetSelectedFunc in buildNavigationTree()
		// Do NOT set SetSelectedFunc here as it causes duplicate callbacks

		root.AddChild(teamNode)
	}

	a.applySelectionStyleToTree(root)
	a.navigationTree.SetRoot(root)
	a.navigationTree.SetCurrentNode(allIssues)
	a.selectedNavigation = &NavigationNode{ID: "all", Text: "All Issues"}
}

// onTeamExpanded loads projects for a team when it's expanded.
func (a *App) onTeamExpanded(teamID string, teamNode *tview.TreeNode) {
	// If already has children (projects loaded), just toggle expand
	if len(teamNode.GetChildren()) > 0 {
		teamNode.SetExpanded(!teamNode.IsExpanded())
		return
	}

	// Load projects, workflow states, and cycles asynchronously.
	go func() {
		logger.Debug("tui.app: loading navigation children team_id=%s", teamID)
		ctx := context.Background()

		// Warm all five team caches rather than the three this needs.
		// Selecting a team preloads the same set, and duplicating a subset here
		// meant every first click issued each request twice.
		if err := a.cache.PreloadTeamMetadata(ctx, teamID); err != nil {
			logger.ErrorWithErr(err, "tui.app: failed to load team metadata team_id=%s", teamID)
			a.app.QueueUpdateDraw(func() {
				a.updateStatusBarWithError(err)
			})
			return
		}
		projects, projectsErr := a.cache.GetProjects(ctx, teamID)
		states, statesErr := a.cache.GetWorkflowStates(ctx, teamID)
		cycles, cyclesErr := a.cache.GetCycles(ctx, teamID)
		if err := cmp.Or(projectsErr, statesErr, cyclesErr); err != nil {
			logger.ErrorWithErr(err, "tui.app: failed to read team metadata team_id=%s", teamID)
			a.app.QueueUpdateDraw(func() {
				a.updateStatusBarWithError(err)
			})
			return
		}
		logger.Debug("tui.app: loaded navigation children team_id=%s projects=%d states=%d cycles=%d", teamID, len(projects), len(states), len(cycles))

		a.app.QueueUpdateDraw(func() {
			// Double-check children haven't been added by another goroutine
			if len(teamNode.GetChildren()) > 0 {
				teamNode.SetExpanded(true)
				return
			}
			a.populateTeamNodeChildren(teamNode, teamID, projects, states, cycles)
			teamNode.SetExpanded(true)
		})
	}()
}

// populateTeamNodeChildren renders cycle, status, and project child nodes under a team node.
func (a *App) populateTeamNodeChildren(teamNode *tview.TreeNode, teamID string, projects []linearapi.Project, states []linearapi.WorkflowState, cycles []linearapi.Cycle) {
	if len(cycles) > 0 {
		sortCyclesForNavigation(cycles)
		cyclesGroup := tview.NewTreeNode("Cycles").
			SetColor(a.theme.SecondaryText).
			SetSelectable(false).
			SetReference(&NavigationNode{
				ID:      fmt.Sprintf("%s-cycles", teamID),
				Text:    "Cycles",
				TeamID:  teamID,
				IsCycle: true,
			})
		for _, cycle := range cycles {
			label := cycle.DisplayName()
			switch {
			case cycle.IsActive:
				label += " (active)"
			case cycle.IsNext:
				label += " (next)"
			case cycle.IsPrevious:
				label += " (previous)"
			}
			cycleNode := tview.NewTreeNode(label).
				SetColor(a.theme.SecondaryText).
				SetReference(&NavigationNode{
					ID:        cycle.ID,
					Text:      label,
					TeamID:    teamID,
					IsCycle:   true,
					CycleID:   cycle.ID,
					CycleName: cycle.DisplayName(),
				})
			cyclesGroup.AddChild(cycleNode)
		}
		teamNode.AddChild(cyclesGroup)
	}
	if len(states) > 0 {
		sort.Slice(states, func(i, j int) bool {
			return states[i].Position < states[j].Position
		})
		statusGroup := tview.NewTreeNode("Status").
			SetColor(a.theme.SecondaryText).
			SetSelectable(false).
			SetReference(&NavigationNode{
				ID:       fmt.Sprintf("%s-status", teamID),
				Text:     "Status",
				TeamID:   teamID,
				IsStatus: true,
			})
		for _, state := range states {
			stateNode := tview.NewTreeNode(state.Name).
				SetColor(a.theme.SecondaryText).
				SetReference(&NavigationNode{
					ID:        state.ID,
					Text:      state.Name,
					TeamID:    teamID,
					IsStatus:  true,
					StateID:   state.ID,
					StateName: state.Name,
				})
			statusGroup.AddChild(stateNode)
		}
		teamNode.AddChild(statusGroup)
	}
	for _, proj := range projects {
		projNode := tview.NewTreeNode(proj.Name).
			SetColor(a.theme.SecondaryText).
			SetReference(&NavigationNode{
				ID:        proj.ID,
				Text:      proj.Name,
				IsProject: true,
				TeamID:    teamID,
			})
		teamNode.AddChild(projNode)
	}
	a.applySelectionStyleToTree(teamNode)
}

func sortCyclesForNavigation(cycles []linearapi.Cycle) {
	sort.SliceStable(cycles, func(i, j int) bool {
		leftRank := cycleNavigationRank(cycles[i])
		rightRank := cycleNavigationRank(cycles[j])
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if cycles[i].IsFuture || cycles[i].IsNext {
			return cycles[i].StartsAt.Before(cycles[j].StartsAt)
		}
		return cycles[i].StartsAt.After(cycles[j].StartsAt)
	})
}

func cycleNavigationRank(cycle linearapi.Cycle) int {
	switch {
	case cycle.IsActive:
		return 0
	case cycle.IsNext:
		return 1
	case cycle.IsFuture:
		return 2
	case cycle.IsPrevious:
		return 3
	case cycle.IsPast:
		return 4
	default:
		return 5
	}
}

// buildLayout constructs the main UI layout.
func (a *App) buildLayout() {
	// Build all panes
	a.navigationTree = a.buildNavigationTree()
	// Build My Issues and Other Issues tables
	a.myIssuesTable = a.buildIssuesTable(" My Issues ", IssuesSectionMy)
	a.otherIssuesTable = a.buildIssuesTable(" Other Issues ", IssuesSectionOther)
	a.allIssuesTable = a.buildIssuesTable(" All Issues ", IssuesSectionAll)
	a.buildSearchPanel()
	// Create vertical flex for issues column
	a.issuesColumn = tview.NewFlex().SetDirection(tview.FlexRow)
	// Initially show only Other Issues table (My Issues will be added when issues are loaded)
	a.issuesColumn.AddItem(a.otherIssuesTable, 0, 1, false)
	// Legacy table for backward compatibility (will be removed after migration)
	a.issuesTable = a.otherIssuesTable
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
	a.createIssueModal = NewCreateIssueModal(a)
	a.createCommentModal = NewCreateCommentModal(a)
	a.editTitleModal = NewEditTitleModal(a)
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

// bindGlobalKeys sets up global keyboard shortcuts.
func (a *App) bindGlobalKeys() {
	a.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if a.pages.HasPage("confirmation") && a.confirmationModal != nil {
			return a.confirmationModal.HandleKey(event)
		}

		// Handle picker modal if active
		if a.pickerActive {
			return a.pickerModal.HandleKey(event)
		}

		// Check if create issue modal is visible and handle its keys
		if a.pages.HasPage("create_issue") && a.createIssueModal != nil {
			return a.createIssueModal.HandleKey(event)
		}

		// Check if create comment modal is visible and handle its keys
		if a.pages.HasPage("create_comment") && a.createCommentModal != nil {
			return a.createCommentModal.HandleKey(event)
		}

		// Check if edit title modal is visible and handle its keys
		if a.pages.HasPage("edit_title") && a.editTitleModal != nil {
			return a.editTitleModal.HandleKey(event)
		}

		// Check if edit description modal is visible and handle its keys
		if a.pages.HasPage("edit_description") && a.editDescriptionModal != nil {
			return a.editDescriptionModal.HandleKey(event)
		}

		// Check if edit labels modal is visible and handle its keys
		if a.pages.HasPage("edit_labels") && a.editLabelsModal != nil {
			return a.editLabelsModal.HandleKey(event)
		}

		if a.pages.HasPage("text_input") && a.textInputModal != nil {
			return a.textInputModal.HandleKey(event)
		}

		if a.pages.HasPage("multi_select") && a.multiSelectModal != nil {
			return a.multiSelectModal.HandleKey(event)
		}

		// Check if settings modal is visible and handle its keys
		if a.pages.HasPage("settings") && a.settingsModal != nil {
			return a.settingsModal.HandleKey(event)
		}

		// Check if prompt templates modal is visible and handle its keys
		if a.pages.HasPage("prompt_templates") && a.promptTemplatesModal != nil {
			return a.promptTemplatesModal.HandleKey(event)
		}

		// Check if agent prompt modal is visible and handle its keys
		if a.pages.HasPage("agent_prompt") && a.agentPromptModal != nil {
			return a.agentPromptModal.HandleKey(event)
		}

		// Check if agent output modal is visible and handle its keys
		if a.pages.HasPage("agent_output") && a.agentOutputModal != nil {
			return a.agentOutputModal.HandleKey(event)
		}

		// Handle palette first if it's open
		if a.focusedPane == FocusPalette {
			return a.handlePaletteKey(event)
		}

		// The search input owns keys next, so typed letters reach the field
		// instead of firing global or pane shortcuts.
		if a.searchInputActive() {
			return a.handleSearchInputKey(event)
		}

		// Global shortcuts (only when not in palette)
		switch event.Key() {
		case tcell.KeyCtrlC:
			a.app.Stop()
			return nil
		case tcell.KeyTab, tcell.KeyBacktab:
			// Tab cycles forward through panes (Navigation -> Issues -> Details)
			// When in Details pane, first cycle between description and comments
			// Only cycle when not in palette or modals
			isBackward := event.Key() == tcell.KeyBacktab || event.Modifiers()&tcell.ModShift != 0
			if a.focusedPane != FocusPalette {
				if a.focusedPane == FocusDetails {
					if !a.detailsCommentsVisible {
						if isBackward {
							a.cyclePanesBackward()
						} else {
							a.cyclePanesForward()
						}
						return nil
					}
					// Cycle between description and comments within details pane
					if !isBackward {
						// Tab: description -> comments -> next pane
						if a.focusedDetailsView {
							// Currently on comments, move to next pane
							a.focusedDetailsView = false // Reset for next time
							a.cyclePanesForward()
						} else {
							// Currently on description, move to comments
							a.focusedDetailsView = true
							a.updateFocus()
						}
					} else {
						// Shift+Tab: comments -> description -> previous pane
						if a.focusedDetailsView {
							// Currently on comments, move to description
							a.focusedDetailsView = false
							a.updateFocus()
						} else {
							// Currently on description, move to previous pane
							a.cyclePanesBackward()
						}
					}
				} else {
					if isBackward {
						// Shift+Tab cycles backward
						a.cyclePanesBackward()
					} else {
						a.cyclePanesForward()
					}
				}
			}
			return nil
		case tcell.KeyRune:
			switch event.Rune() {
			case a.actionKey("quit", 'q'):
				a.app.Stop()
				return nil
			case a.actionKey("open_palette", ':'):
				a.openPalette()
				return nil
			case a.actionKey("search", '/'):
				a.openSearchTab()
				return nil
			}
		}

		// Pane-specific shortcuts
		switch a.focusedPane {
		case FocusNavigation:
			return a.handleNavigationKey(event)
		case FocusIssues:
			return a.handleIssuesKey(event)
		case FocusDetails:
			return a.handleDetailsKey(event)
		}

		return event
	})
}

// runCommandShortcut fires the palette command bound to the rune, if any.
func (a *App) runCommandShortcut(r rune) bool {
	for _, cmd := range a.paletteCtrl.commands {
		if cmd.ShortcutRune != 0 && cmd.ShortcutRune == r {
			cmd.Run(a)
			return true
		}
	}
	return false
}

// handleNavigationKey handles keyboard input when navigation pane is focused.
func (a *App) handleNavigationKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyRight:
		a.focusedPane = FocusIssues
		a.updateFocus()
		return nil
	case tcell.KeyRune:
		switch r := event.Rune(); r {
		case 'l':
			a.focusedPane = FocusIssues
			a.updateFocus()
			return nil
		case a.actionKey("favorite_move_up", 'K'):
			if a.moveFavorite(a.currentNavigationNode(), -1) {
				return nil
			}
		case a.actionKey("favorite_move_down", 'J'):
			if a.moveFavorite(a.currentNavigationNode(), 1) {
				return nil
			}
		case a.actionKey("favorite_nest", 'L'):
			if a.nestFavorite(a.currentNavigationNode(), false) {
				return nil
			}
		case a.actionKey("favorite_unnest", 'H'):
			if a.nestFavorite(a.currentNavigationNode(), true) {
				return nil
			}
		case 'j', 'k', 'g', 'G', 'h':
			// Tree movement keys stay with the tree.
		default:
			// Command shortcuts work from the navigation pane too.
			if a.runCommandShortcut(r) {
				return nil
			}
		}
	}
	return event
}

// handleIssuesKey handles keyboard input when issues pane is focused.
func (a *App) handleIssuesKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEscape:
		// Esc in the search results returns to the search input.
		if a.effectiveIssuesSection() == IssuesSectionSearch {
			a.focusSearchInput()
			return nil
		}
	case tcell.KeyLeft:
		a.focusedPane = FocusNavigation
		a.updateFocus()
		return nil
	case tcell.KeyRight:
		a.focusedPane = FocusDetails
		a.focusedDetailsView = false // Start with description
		a.updateFocus()
		return nil
	case tcell.KeyRune:
		r := event.Rune()
		// Handle vim-style navigation first
		switch r {
		case 'h':
			a.focusedPane = FocusNavigation
			a.updateFocus()
			return nil
		case 'l':
			a.focusedPane = FocusDetails
			a.focusedDetailsView = false // Start with description
			a.updateFocus()
			return nil
		}
		// { and } cycle the issues tabs, lazygit-style ([ and ] keep their
		// original expand/collapse-all bindings).
		switch r {
		case a.actionKey("tab_prev", '{'):
			a.cycleIssuesSection(-1)
			return nil
		case a.actionKey("tab_next", '}'):
			a.cycleIssuesSection(1)
			return nil
		}
		// Handle command shortcuts (plain letters) - skip navigation keys
		if r != 'j' && r != 'k' { // j/k are handled by table for up/down
			if a.runCommandShortcut(r) {
				return nil
			}
		}
	}
	return event
}

// handleDetailsKey handles keyboard input when details pane is focused.
func (a *App) handleDetailsKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEnter:
		// Enter closes the details pane and returns to the issues list.
		a.focusedPane = FocusIssues
		a.toggleDetailsPane()
		return nil
	case tcell.KeyLeft:
		a.focusedPane = FocusIssues
		a.updateFocus()
		return nil
	case tcell.KeyRune:
		switch event.Rune() {
		case 'h':
			a.focusedPane = FocusIssues
			a.updateFocus()
			return nil
		case a.actionKey("tab_prev", '{'), a.actionKey("tab_next", '}'):
			// Cycle the Details/Comments tabs, lazygit-style.
			if a.detailsCommentsVisible {
				a.focusedDetailsView = !a.focusedDetailsView
				a.updateFocus()
			}
			return nil
		}
	}
	return event
}

// handlePaletteKey handles keyboard input when palette is open.
func (a *App) handlePaletteKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEscape:
		a.closePalette()
		return nil
	case tcell.KeyEnter:
		if cmd, ok := a.paletteCtrl.Selected(); ok {
			a.closePalette()
			cmd.Run(a)
			return nil
		}
		return nil
	case tcell.KeyUp:
		a.paletteCtrl.MoveCursorUp()
		a.updatePaletteList()
		return nil
	case tcell.KeyDown:
		a.paletteCtrl.MoveCursorDown()
		a.updatePaletteList()
		return nil
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		query := a.paletteCtrl.Query()
		if len(query) > 0 {
			a.paletteCtrl.SetQuery(query[:len(query)-1])
			a.paletteInput.SetText(a.paletteCtrl.Query())
			a.updatePaletteList()
		}
		return nil
	case tcell.KeyRune:
		query := a.paletteCtrl.Query() + string(event.Rune())
		a.paletteCtrl.SetQuery(query)
		a.paletteInput.SetText(query)
		a.updatePaletteList()
		return nil
	}
	return event
}

// cyclePanesForward cycles focus forward through panes.
// When in Issues pane, cycles: My Issues -> Other Issues -> Details
// Otherwise cycles: Navigation -> Issues -> Details -> Navigation
func (a *App) cyclePanesForward() {
	switch a.focusedPane {
	case FocusNavigation:
		a.focusedPane = FocusIssues
		// Set to My Issues if available, otherwise Other Issues. The Search
		// tab keeps its place: tabbing away and back must not clear it.
		if a.activeIssuesSection != IssuesSectionSearch {
			if len(a.myIssueRows) > 0 {
				a.activeIssuesSection = IssuesSectionMy
			} else {
				a.activeIssuesSection = IssuesSectionOther
			}
		}
	case FocusIssues:
		switch {
		case a.activeIssuesSection == IssuesSectionSearch:
			// The Search tab is a single section; no My/Other shuffle.
			a.focusedPane = FocusDetails
			a.focusedDetailsView = false
		case len(a.myIssueRows) > 0 && len(a.otherIssueRows) > 0:
			if a.activeIssuesSection == IssuesSectionMy {
				// Switch from My Issues to Other Issues
				a.activeIssuesSection = IssuesSectionOther
			} else {
				// Switch from Other Issues to Details pane
				a.focusedPane = FocusDetails
				a.focusedDetailsView = false // Start with description
			}
		default:
			// Only one section exists, move to Details
			a.focusedPane = FocusDetails
			a.focusedDetailsView = false // Start with description
		}
	case FocusDetails:
		a.focusedPane = FocusNavigation
		// FocusPalette is excluded from cycling
	}
	a.updateFocus()
}

// cyclePanesBackward cycles focus backward through panes.
// When in Issues pane, cycles: Other Issues -> My Issues -> Navigation
// Otherwise cycles: Details -> Issues (My Issues preferred) -> Navigation -> Details
func (a *App) cyclePanesBackward() {
	switch a.focusedPane {
	case FocusNavigation:
		a.focusedPane = FocusDetails
		a.focusedDetailsView = false // Start with description
	case FocusIssues:
		switch {
		case a.activeIssuesSection == IssuesSectionSearch:
			// The Search tab is a single section; no My/Other shuffle.
			a.focusedPane = FocusNavigation
		case len(a.myIssueRows) > 0 && len(a.otherIssueRows) > 0:
			if a.activeIssuesSection == IssuesSectionOther {
				// Switch from Other Issues to My Issues
				a.activeIssuesSection = IssuesSectionMy
			} else {
				// Switch from My Issues to Navigation pane
				a.focusedPane = FocusNavigation
			}
		default:
			// Only one section exists, move to Navigation
			a.focusedPane = FocusNavigation
		}
	case FocusDetails:
		a.focusedPane = FocusIssues
		// Set to My Issues if available, otherwise Other Issues (consistent
		// with forward cycle). The Search tab keeps its place.
		if a.activeIssuesSection != IssuesSectionSearch {
			if len(a.myIssueRows) > 0 {
				a.activeIssuesSection = IssuesSectionMy
			} else {
				a.activeIssuesSection = IssuesSectionOther
			}
		}
		// FocusPalette is excluded from cycling
	}
	a.updateFocus()
}

// updateFocus updates the focus state of all panes.
func (a *App) updateFocus() {
	// Hidden panes cannot take focus; fall back to the issues column.
	if (a.focusedPane == FocusNavigation && a.navigationHidden) ||
		(a.focusedPane == FocusDetails && a.detailsHidden) {
		a.focusedPane = FocusIssues
	}
	// In responsive modes the focused pane decides what is visible.
	if a.layoutMode != layoutWide {
		a.rebuildContentLayout()
	}
	switch a.focusedPane {
	case FocusNavigation:
		a.app.SetFocus(a.navigationTree)
		a.navigationTree.SetBorderColor(a.theme.BorderFocus)
		a.myIssuesTable.SetBorderColor(a.theme.Border)
		a.otherIssuesTable.SetBorderColor(a.theme.Border)
		a.allIssuesTable.SetBorderColor(a.theme.Border)
		a.searchPanel.SetBorderColor(a.theme.Border)
		a.detailsDescriptionView.SetBorderColor(a.theme.Border)
		a.detailsCommentsView.SetBorderColor(a.theme.Border)
		// Update all pane titles
		a.updateAllPaneTitles()
	case FocusIssues:
		// Focus the visible issues section
		a.myIssuesTable.SetBorderColor(a.theme.Border)
		a.otherIssuesTable.SetBorderColor(a.theme.Border)
		a.allIssuesTable.SetBorderColor(a.theme.Border)
		a.searchPanel.SetBorderColor(a.theme.Border)
		if a.effectiveIssuesSection() == IssuesSectionSearch {
			a.searchPanel.SetBorderColor(a.theme.BorderFocus)
			if a.searchInputFocused {
				a.app.SetFocus(a.searchInput)
			} else {
				a.app.SetFocus(a.searchResultsTable)
			}
		} else if table := a.tableForSection(a.effectiveIssuesSection()); table != nil {
			a.app.SetFocus(table)
			table.SetBorderColor(a.theme.BorderFocus)
		}
		// Update all pane titles
		a.updateAllPaneTitles()
		a.navigationTree.SetBorderColor(a.theme.Border)
		a.detailsDescriptionView.SetBorderColor(a.theme.Border)
		a.detailsCommentsView.SetBorderColor(a.theme.Border)
	case FocusDetails:
		// Focus the appropriate sub-view based on state
		if !a.detailsCommentsVisible {
			a.focusedDetailsView = false
		}
		a.updateDetailsLayout()
		if a.focusedDetailsView && a.detailsCommentsVisible {
			a.app.SetFocus(a.detailsCommentsView)
			a.detailsDescriptionView.SetBorderColor(a.theme.Border)
			a.detailsCommentsView.SetBorderColor(a.theme.BorderFocus)
		} else {
			a.app.SetFocus(a.detailsDescriptionView)
			a.detailsDescriptionView.SetBorderColor(a.theme.BorderFocus)
			a.detailsCommentsView.SetBorderColor(a.theme.Border)
		}
		a.navigationTree.SetBorderColor(a.theme.Border)
		a.myIssuesTable.SetBorderColor(a.theme.Border)
		a.otherIssuesTable.SetBorderColor(a.theme.Border)
		a.allIssuesTable.SetBorderColor(a.theme.Border)
		a.searchPanel.SetBorderColor(a.theme.Border)
		// Update all pane titles
		a.updateAllPaneTitles()
	case FocusPalette:
		a.app.SetFocus(a.paletteInput)
		a.navigationTree.SetBorderColor(a.theme.Border)
		a.myIssuesTable.SetBorderColor(a.theme.Border)
		a.otherIssuesTable.SetBorderColor(a.theme.Border)
		a.allIssuesTable.SetBorderColor(a.theme.Border)
		a.searchPanel.SetBorderColor(a.theme.Border)
		a.detailsDescriptionView.SetBorderColor(a.theme.Border)
		a.detailsCommentsView.SetBorderColor(a.theme.Border)
		// Update all pane titles
		a.updateAllPaneTitles()
	}
	a.updateStatusBar()
}

// updateAllPaneTitles updates all pane titles with visual indicators for the active pane.
func (a *App) updateAllPaneTitles() {
	// Update Navigation pane title
	if a.focusedPane == FocusNavigation {
		a.navigationTree.SetTitle(" ▶ Navigation ")
		a.navigationTree.SetTitleColor(a.theme.Accent)
	} else {
		a.navigationTree.SetTitle(" Navigation ")
		a.navigationTree.SetTitleColor(a.theme.Foreground)
	}

	// Update Issues pane tab strip
	isIssuesFocused := a.focusedPane == FocusIssues
	issuesTitle := a.issuesTabsTitle(isIssuesFocused)
	a.myIssuesTable.SetTitle(issuesTitle)
	a.myIssuesTable.SetTitleColor(a.theme.Foreground)
	a.otherIssuesTable.SetTitle(issuesTitle)
	a.otherIssuesTable.SetTitleColor(a.theme.Foreground)
	a.allIssuesTable.SetTitle(issuesTitle)
	a.allIssuesTable.SetTitleColor(a.theme.Foreground)
	if a.searchPanel != nil {
		a.searchPanel.SetTitle(issuesTitle)
		a.searchPanel.SetTitleColor(a.theme.Foreground)
	}

	// Update Details pane tab strip
	isDetailsFocused := a.focusedPane == FocusDetails
	if a.detailsDescriptionView != nil {
		detailsTitle := a.detailsTabsTitle(isDetailsFocused)
		a.detailsDescriptionView.SetTitle(detailsTitle)
		a.detailsDescriptionView.SetTitleColor(a.theme.Foreground)
		if a.detailsCommentsView != nil {
			a.detailsCommentsView.SetTitle(detailsTitle)
			a.detailsCommentsView.SetTitleColor(a.theme.Foreground)
		}
	}
}

// openPalette opens the command palette overlay.
func (a *App) openPalette() {
	a.paletteCtrl.SetIssueContext(a.focusedPane == FocusIssues || a.focusedPane == FocusDetails)
	a.paletteCtrl.Reset()
	a.paletteInput.SetText("")
	a.paletteInput.SetLabel("> ")
	a.paletteInput.SetPlaceholder("Type to filter commands...")
	a.updatePaletteList()
	a.pages.ShowPage("palette")
	a.pages.SendToFront("palette")
	if a.focusedPane != FocusPalette {
		a.palettePreviousPane = a.focusedPane
	}
	a.focusedPane = FocusPalette
	a.updateFocus()
}

// closePalette closes the command palette overlay.
func (a *App) closePalette() {
	a.pages.HidePage("palette")
	// Restore focus to the pane the palette was opened from.
	a.focusedPane = a.palettePreviousPane
	a.updateFocus()
}

func (a *App) searchDebounceDelay() time.Duration {
	if a.config.SearchDebounce > 0 {
		return a.config.SearchDebounce
	}
	return config.DefaultSearchDebounce
}

func (a *App) scheduleSearchDebounce(query string) {
	delay := a.searchDebounceDelay()
	generation := a.searchDebounceGeneration.Add(1)

	a.searchDebounceMu.Lock()
	if a.searchDebounceTimer != nil {
		a.searchDebounceTimer.Stop()
	}
	a.searchDebounceTimer = time.AfterFunc(delay, func() {
		if generation != a.searchDebounceGeneration.Load() {
			return
		}
		a.QueueUpdateDraw(func() {
			if generation != a.searchDebounceGeneration.Load() {
				return
			}
			a.performIssueSearch(query)
		})
	})
	a.searchDebounceMu.Unlock()
}

func (a *App) cancelSearchDebounce() {
	a.searchDebounceGeneration.Add(1)

	a.searchDebounceMu.Lock()
	if a.searchDebounceTimer != nil {
		a.searchDebounceTimer.Stop()
		a.searchDebounceTimer = nil
	}
	a.searchDebounceMu.Unlock()
}

// queueIssuesRefresh records a refresh request while a fetch is in progress.
func (a *App) queueIssuesRefresh(allowFocusChange bool, issueID ...string) {
	logger.Debug("tui.app: queueing issues refresh issue_id=%v", issueID)
	a.pendingRefresh = true
	a.pendingRefreshAllowFocusChange = allowFocusChange
	a.refreshGeneration.Add(1)
	if len(issueID) > 0 {
		a.pendingRefreshIssueID = issueID[0]
		return
	}
	a.pendingRefreshIssueID = ""
}

// runQueuedIssuesRefresh triggers any queued refresh after a fetch completes.
func (a *App) runQueuedIssuesRefresh() {
	if !a.pendingRefresh {
		return
	}
	issueID := a.pendingRefreshIssueID
	allowFocusChange := a.pendingRefreshAllowFocusChange
	logger.Debug("tui.app: running queued refresh issue_id=%s", issueID)
	a.pendingRefresh = false
	a.pendingRefreshIssueID = ""
	a.pendingRefreshAllowFocusChange = true
	if issueID != "" {
		go a.refreshIssuesWithFocusChange(allowFocusChange, issueID)
		return
	}
	go a.refreshIssuesWithFocusChange(allowFocusChange)
}

func (a *App) notifyRefreshCompleted() {
	if a.refreshCompleted != nil {
		a.refreshCompleted()
	}
}

// currentFetchParams describes the issue list as it is scoped right now: the
// rich filters plus whatever the navigation selection narrows to. Callers must
// build it on the UI thread, since it reads state a refresh reassigns.
func (a *App) currentFetchParams(orderBy string) linearapi.FetchIssuesParams {
	params := linearapi.FetchIssuesParams{
		First:   a.config.PageSize,
		OrderBy: orderBy,
	}
	a.applyRichFiltersToParams(&params)

	// Apply team/project/state filter based on navigation selection
	if a.selectedNavigation != nil {
		switch {
		case a.selectedNavigation.CustomViewID != "":
			params.CustomViewID = a.selectedNavigation.CustomViewID
		case a.selectedNavigation.StateType != "":
			params.TeamID = a.selectedNavigation.TeamID
			params.StateType = a.selectedNavigation.StateType
		case a.selectedNavigation.IsStatus:
			params.TeamID = a.selectedNavigation.TeamID
			params.StateID = a.selectedNavigation.StateID
		case a.selectedNavigation.IsCycle:
			params.TeamID = a.selectedNavigation.TeamID
			params.CycleID = a.selectedNavigation.CycleID
		case a.selectedNavigation.IsTeam:
			params.TeamID = a.selectedNavigation.TeamID
		case a.selectedNavigation.IsProject:
			params.TeamID = a.selectedNavigation.TeamID
			params.ProjectID = a.selectedNavigation.ID
		case a.selectedNavigation.TeamID != "":
			// A team-scoped All Issues favorite carries a team and none of the
			// flags above, so it must stay last to avoid shadowing them.
			params.TeamID = a.selectedNavigation.TeamID
		}
		// Workspace-wide "All Issues" reaches here with nothing set, unfiltered
	}
	return params
}

// refreshIssues fetches issues from the API and updates the UI.
// If issueID is provided, that issue will be selected after refresh.
func (a *App) refreshIssues(issueID ...string) {
	a.refreshIssuesWithFocusChange(true, issueID...)
}

// refreshIssuesWithFocusChange fetches issues and optionally shifts focus to the issues pane.
func (a *App) refreshIssuesWithFocusChange(allowFocusChange bool, issueID ...string) {
	if a.isLoading {
		a.queueIssuesRefresh(allowFocusChange, issueID...)
		return
	}
	a.isLoading = true

	targetID := ""
	if len(issueID) > 0 {
		targetID = issueID[0]
	}
	logger.Debug("tui.app: starting issues refresh target_issue_id=%s", targetID)
	generation := a.refreshGeneration.Add(1)
	var targetIssueID string
	if len(issueID) > 0 {
		targetIssueID = issueID[0]
	}

	allowFocus := allowFocusChange
	// Snapshot the chain here: setSortFields reassigns it on the UI thread
	// while this goroutine runs.
	orderBy := string(a.sortFields[0])
	params := a.currentFetchParams(orderBy)
	go func() {
		refreshStarted := time.Now()
		ctx := context.Background()

		fetchPage := a.fetchIssuesPage
		if fetchPage == nil {
			fetchPage = a.api.FetchIssuesPage
		}

		// A custom view carries its own display settings; fetch them first
		// so the issue query can use the view's sort. Failures fall back to
		// the configured defaults.
		var prefs *viewDisplayPrefs
		if params.CustomViewID != "" {
			fetchPrefs := a.fetchViewPrefsFunc
			if fetchPrefs == nil {
				fetchPrefs = a.api.FetchCustomViewPreferences
			}
			values, prefsErr := fetchPrefs(ctx, params.CustomViewID)
			if prefsErr != nil {
				logger.ErrorWithErr(prefsErr, "tui.app: failed to fetch view preferences view_id=%s", params.CustomViewID)
			} else if values != nil {
				logger.Debug("tui.app: view preferences view_id=%s grouping=%q subgrouping=%q ordering=%q direction=%q", params.CustomViewID, values.IssueGrouping, values.IssueSubGrouping, values.ViewOrdering, values.ViewOrderingDirection)
				prefs = resolveViewPrefs(values)
			}
			if prefs != nil && prefs.hasSort && !a.sortOverridden {
				params.OrderBy = string(prefs.sortField)
			}
		}

		pageCount := 0
		fetchedCount := 0
		logger.Debug("tui.app: refreshing issues team_id=%s project_id=%s state_id=%s cycle_id=%s assignee_id=%s labels=%d", params.TeamID, params.ProjectID, params.StateID, params.CycleID, params.AssigneeID, len(params.LabelIDs))
		page, err := fetchPage(ctx, params, nil)
		if err != nil {
			a.QueueUpdateDraw(func() {
				a.isLoading = false
				logger.ErrorWithErr(err, "tui.app: failed to fetch issues")
				a.updateStatusBarWithError(err)
				a.notifyRefreshCompleted()
				a.runQueuedIssuesRefresh()
			})
			return
		}
		if generation != a.refreshGeneration.Load() {
			a.QueueUpdateDraw(func() {
				a.isLoading = false
				a.notifyRefreshCompleted()
				a.runQueuedIssuesRefresh()
			})
			return
		}

		pageCount++
		fetchedCount += len(page.Issues)
		// Dedup state for the pages that follow. Kept across iterations so the
		// merge stays linear instead of rebuilding a set over the whole
		// accumulated slice once per page.
		seen := make(map[string]bool, len(page.Issues))
		a.QueueUpdateDraw(func() {
			logger.Debug("tui.app: fetched issues page=%d count=%d", pageCount, len(page.Issues))
			// Install (or clear) the active view's display settings with
			// the list they belong to.
			a.viewPrefs = prefs
			a.updateIssuesData(page.Issues, targetIssueID)
			for _, issue := range a.issues {
				seen[issue.ID] = true
			}
			if allowFocus {
				// Ensure focus is on issues table after initial load
				a.focusedPane = FocusIssues
				a.updateFocus()
			}
			if page.HasNext {
				a.statusBar.SetText(fmt.Sprintf("%sLoading more (page %d, fetched %d)...[-]", a.themeTags.Warning, pageCount, fetchedCount))
			}
		})

		after := page.EndCursor
		accumulated := false
		for page.HasNext {
			if generation != a.refreshGeneration.Load() {
				break
			}
			nextPage, err := fetchPage(ctx, params, after)
			if err != nil {
				a.QueueUpdateDraw(func() {
					logger.ErrorWithErr(err, "tui.app: failed to fetch more issues page=%d", pageCount+1)
					a.updateStatusBarWithError(err)
				})
				break
			}
			if generation != a.refreshGeneration.Load() {
				break
			}

			page = nextPage
			after = page.EndCursor
			pageCount++
			fetchedCount += len(page.Issues)
			a.QueueUpdateDraw(func() {
				// Merge without painting. A regroup and full repaint per page
				// costs roughly fifty times a single rebuild for the same end
				// state; the status text carries progress until the last page.
				if a.accumulateIssues(page.Issues, seen) {
					accumulated = true
				}
				if page.HasNext {
					a.statusBar.SetText(fmt.Sprintf("%sLoading more (page %d, fetched %d)...[-]", a.themeTags.Warning, pageCount, fetchedCount))
				}
			})
		}

		a.QueueUpdateDraw(func() {
			// A superseded refresh must not paint its partial list. The queued
			// refresh that replaced it runs next and owns the table.
			if accumulated && generation == a.refreshGeneration.Load() {
				a.renderAccumulatedIssues()
			}
			a.isLoading = false
			logger.Debug("tui.app: refresh completed pages=%d total_fetched=%d elapsed=%s", pageCount, fetchedCount, time.Since(refreshStarted))
			a.updateStatusBar()
			a.notifyRefreshCompleted()
			a.runQueuedIssuesRefresh()
		})
	}()

	// Show loading indicator
	a.QueueUpdateDraw(func() {
		a.statusBar.SetText(fmt.Sprintf("%sLoading...[-]", a.themeTags.Warning))
	})
}

func (a *App) applyRichFiltersToParams(params *linearapi.FetchIssuesParams) {
	if params == nil {
		return
	}
	filters := a.richFilters
	if filters.AssigneeID != "" {
		params.AssigneeID = filters.AssigneeID
	}
	if len(filters.LabelIDs) > 0 {
		params.LabelIDs = append([]string(nil), filters.LabelIDs...)
	}
	if filters.StateID != "" {
		params.StateID = filters.StateID
	}
	if filters.ProjectID != "" {
		params.ProjectID = filters.ProjectID
	}
	if filters.CycleID != "" {
		params.CycleID = filters.CycleID
	}
	if !filters.DueDate.Empty() {
		params.DueDate = filters.DueDate
	}
	if !filters.Estimate.Empty() {
		params.Estimate = filters.Estimate
	}
}

// updateIssuesColumnLayout shows the active issues tab at full height.
func (a *App) updateIssuesColumnLayout() {
	a.issuesColumn.Clear()

	// A tab about to come on screen may still be holding cells from before the
	// last model change.
	a.flushPendingSectionRender(a.effectiveIssuesSection())

	// Without any My Issues, that tab disappears (without forgetting the
	// active choice: My re-applies once it has rows). The Search tab mounts
	// its input-plus-results panel instead of a bare table.
	if a.effectiveIssuesSection() == IssuesSectionSearch {
		a.issuesColumn.AddItem(a.searchPanel, 0, 1, false)
	} else {
		a.issuesColumn.AddItem(a.tableForSection(a.effectiveIssuesSection()), 0, 1, false)
	}

	// Update all pane titles to reflect current state
	a.updateAllPaneTitles()
}

// updateIssuesData updates the UI with new issues data.
// If issueID is provided, that issue will be selected if found in the list.
func (a *App) updateIssuesData(issues []linearapi.Issue, issueID ...string) {
	a.issuesMu.Lock()
	a.issues = issues
	a.sortIssuesLocally()

	// Determine target issue ID
	var targetIssueID string
	if len(issueID) > 0 && issueID[0] != "" {
		targetIssueID = issueID[0]
	} else if a.selectedIssue != nil {
		targetIssueID = a.selectedIssue.ID
	}
	a.issuesMu.Unlock()

	selectedIssue := a.rebuildIssuesTables(targetIssueID)
	if a.activeIssuesSection == IssuesSectionSearch {
		// A background refresh must not overwrite the search result the
		// user is browsing.
		a.updateStatusBar()
		return
	}
	if selectedIssue != nil {
		a.onIssueSelected(*selectedIssue)
	} else {
		a.issuesMu.Lock()
		a.selectedIssue = nil
		a.issuesMu.Unlock()
		a.updateDetailsView()
	}
	a.updateStatusBar()
}

// rebuildIssuesTables rebuilds issue rows and renders tables, returning the selected issue.
func (a *App) rebuildIssuesTables(targetIssueID string) *linearapi.Issue {
	a.rebuildIssueRowModels()

	a.renderIssueSections(a.sectionSelectionsFor(targetIssueID))

	// Show the active tab (after target selection may have switched it).
	a.updateIssuesColumnLayout()

	// Select issue and update details.
	var selectedIssue *linearapi.Issue
	if targetIssueID != "" {
		if issue, ok := a.myIDToIssue[targetIssueID]; ok {
			selectedIssue = issue
		} else if issue, ok := a.otherIDToIssue[targetIssueID]; ok {
			selectedIssue = issue
		}
	}

	// If no target issue, default to the first issue row (skipping group
	// headers, which carry no issue). The Search tab keeps its own selection.
	if selectedIssue == nil && a.activeIssuesSection != IssuesSectionSearch {
		if first := nextIssueRow(a.myIssueRows, 0, 1); first > 0 {
			if issue, ok := a.myIDToIssue[a.myIssueRows[first-1].IssueID]; ok {
				selectedIssue = issue
				a.activeIssuesSection = IssuesSectionMy
			}
		} else if first := nextIssueRow(a.otherIssueRows, 0, 1); first > 0 {
			if issue, ok := a.otherIDToIssue[a.otherIssueRows[first-1].IssueID]; ok {
				selectedIssue = issue
				a.activeIssuesSection = IssuesSectionOther
			}
		}
	}

	return selectedIssue
}

// accumulateIssues merges a fetched page into the issue list without sorting or
// painting. seen carries dedup state across pages. Reports whether anything was
// added, so a run of empty or fully duplicated pages skips the final repaint.
func (a *App) accumulateIssues(newIssues []linearapi.Issue, seen map[string]bool) bool {
	a.issuesMu.Lock()
	defer a.issuesMu.Unlock()

	added := false
	for _, issue := range newIssues {
		if seen[issue.ID] {
			continue
		}
		a.issues = append(a.issues, issue)
		seen[issue.ID] = true
		added = true
	}
	return added
}

// renderAccumulatedIssues sorts and repaints once pagination has finished.
func (a *App) renderAccumulatedIssues() {
	a.issuesMu.Lock()
	// Read the selection before sorting: selectedIssue can point into the
	// a.issues backing array, which the in-place sort reorders under it.
	previousID := ""
	if a.selectedIssue != nil {
		previousID = a.selectedIssue.ID
	}
	a.sortIssuesLocally()
	a.issuesMu.Unlock()

	selectedIssue := a.rebuildIssuesTables(previousID)

	a.issuesMu.Lock()
	// Reassign even when the id is unchanged: the old pointer aims into the
	// backing array the sort just reordered.
	a.selectedIssue = selectedIssue
	a.issuesMu.Unlock()

	selectedID := ""
	if selectedIssue != nil {
		selectedID = selectedIssue.ID
	}
	if selectedID != previousID {
		a.updateDetailsView()
	}
	a.updateStatusBar()
}

// sortIssuesLocally applies the sort chain. The API can only order by one
// timestamp, so every field past the first, and priority and status at any
// position, are resolved here. Callers must hold issuesMu.
func (a *App) sortIssuesLocally() {
	sortIssuesByFields(a.issues, a.effectiveSortFields())
}

// issueContextLine renders "ID · Title" for issue-scoped modals, so every
// form names the issue it modifies the same way.
func (a *App) issueContextLine(issue linearapi.Issue) string {
	title := []rune(strings.TrimSpace(issue.Title))
	const maxTitleRunes = 48
	if len(title) > maxTitleRunes {
		title = append(title[:maxTitleRunes-1], '…')
	}
	return fmt.Sprintf("%s%s[-] %s%s[-]", a.themeTags.Accent, issue.Identifier, a.themeTags.SecondaryText, string(title))
}

// issueColumns returns the configured issue list columns, or the default
// Linear-style layout.
func (a *App) issueColumns() []string {
	if len(a.config.Columns) == 0 {
		return DefaultIssueColumns
	}
	return a.config.Columns
}

// buildIssueRowsFor builds table rows for an issue list, honoring the
// grouping dimensions in effect (the active view's, else config).
func (a *App) buildIssueRowsFor(issues []linearapi.Issue) ([]IssueRow, map[string]*linearapi.Issue) {
	groupBy := a.effectiveGroupBy()
	if groupBy == GroupByNone {
		return BuildIssueRows(issues, a.expandedState)
	}
	return BuildGroupedIssueRows(issues, a.expandedState, groupBy, a.effectiveSubgroupBy(), a.collapsedGroups)
}

// toggleGroupCollapse collapses or expands a group header and keeps the
// header selected after the rebuild.
func (a *App) toggleGroupCollapse(section IssuesSection, header IssueRow) {
	if header.HeaderKey == "" {
		return
	}
	if a.collapsedGroups == nil {
		a.collapsedGroups = make(map[string]bool)
	}
	a.collapsedGroups[header.HeaderKey] = !a.collapsedGroups[header.HeaderKey]

	a.issuesMu.RLock()
	targetIssueID := ""
	if a.selectedIssue != nil {
		targetIssueID = a.selectedIssue.ID
	}
	a.issuesMu.RUnlock()
	selectedIssue := a.rebuildIssuesTables(targetIssueID)
	a.issuesMu.Lock()
	a.selectedIssue = selectedIssue
	a.issuesMu.Unlock()
	a.updateDetailsView()

	// Re-select the toggled header so repeated presses toggle in place.
	var table *tview.Table
	switch section {
	case IssuesSectionMy:
		table = a.myIssuesTable
	case IssuesSectionOther:
		table = a.otherIssuesTable
	}
	if table == nil {
		return
	}
	for index, row := range a.rowsForSection(section) {
		if row.IsHeader && row.HeaderKey == header.HeaderKey {
			table.Select(index+1, 0)
			break
		}
	}
}

// regroupIssues rebuilds the tables after a grouping change, keeping the
// current selection.
func (a *App) regroupIssues(message string) {
	a.issuesMu.RLock()
	targetIssueID := ""
	if a.selectedIssue != nil {
		targetIssueID = a.selectedIssue.ID
	}
	a.issuesMu.RUnlock()

	selectedIssue := a.rebuildIssuesTables(targetIssueID)
	a.issuesMu.Lock()
	a.selectedIssue = selectedIssue
	a.issuesMu.Unlock()
	a.updateDetailsView()
	a.flashStatus(message)
}

// showSortByPicker selects the list ordering. Every row is a whole ordering,
// so one keystroke settles it.
func (a *App) showSortByPicker() {
	a.pickerActive = true
	a.pickerModal.Show("Sort Issues By", sortOrderingPickerItems(a.configuredSortFields), func(item PickerItem) {
		a.pickerActive = false
		a.setSortFields(parseSortFields(strings.Split(item.ID, ",")))
	})
}

// groupDimensionPickerItems lists the grouping dimensions for the pickers.
func groupDimensionPickerItems() []PickerItem {
	return []PickerItem{
		{ID: GroupByNone, Label: "None"},
		{ID: GroupByStatus, Label: "Status"},
		{ID: GroupByPriority, Label: "Priority"},
		{ID: GroupByAssignee, Label: "Assignee"},
		{ID: GroupByCycle, Label: "Cycle"},
		{ID: GroupByProject, Label: "Project"},
		{ID: GroupByMilestone, Label: "Milestone"},
	}
}

// showGroupByPicker selects the primary grouping dimension.
func (a *App) showGroupByPicker() {
	a.pickerActive = true
	a.pickerModal.Show("Group Issues By", groupDimensionPickerItems(), func(item PickerItem) {
		a.pickerActive = false
		// A manual grouping choice outranks the active view's for the
		// session.
		a.groupingOverridden = true
		a.config.GroupBy = item.ID
		if a.config.SubgroupBy == item.ID {
			a.config.SubgroupBy = GroupByNone
		}
		if item.ID == GroupByNone {
			a.regroupIssues("Grouping off")
		} else {
			a.regroupIssues("Grouped by " + item.Label)
		}
	})
}

// showSubgroupByPicker selects the secondary grouping dimension.
func (a *App) showSubgroupByPicker() {
	if a.config.GroupBy == GroupByNone {
		a.flashStatus("Set a grouping first (Group issues by…)")
		return
	}
	items := make([]PickerItem, 0, 4)
	for _, item := range groupDimensionPickerItems() {
		if item.ID != a.config.GroupBy {
			items = append(items, item)
		}
	}
	a.pickerActive = true
	a.pickerModal.Show("Subgroup Issues By", items, func(item PickerItem) {
		a.pickerActive = false
		a.groupingOverridden = true
		a.config.SubgroupBy = item.ID
		if item.ID == GroupByNone {
			a.regroupIssues("Subgrouping off")
		} else {
			a.regroupIssues("Subgrouped by " + item.Label)
		}
	})
}

// onIssueSelected handles when an issue is selected.
func (a *App) onIssueSelected(issue linearapi.Issue) {
	logger.Debug("tui.app: issue selected issue=%s", issue.Identifier)
	// Set selected issue immediately for quick UI feedback
	a.issuesMu.Lock()
	a.selectedIssue = &issue
	a.issuesMu.Unlock()
	a.updateDetailsView()

	// Fetch full issue details (including comments) in background
	issueID := issue.ID
	a.fetchingIssueID = issueID

	go func() {
		logger.Debug("tui.app: fetching full issue details issue=%s", issue.Identifier)
		ctx := context.Background()
		fetchIssue := a.fetchIssueByID
		if fetchIssue == nil {
			fetchIssue = a.api.FetchIssueByID
		}
		fullIssue, err := fetchIssue(ctx, issueID)

		a.QueueUpdateDraw(func() {
			// Race-safety: only apply if this is still the issue we're fetching
			if a.fetchingIssueID == issueID {
				if err != nil {
					logger.ErrorWithErr(err, "tui.app: failed to fetch full issue details issue=%s", issue.Identifier)
					// Keep the partial issue data we already have
					return
				}
				a.issuesMu.Lock()
				a.selectedIssue = &fullIssue
				a.issuesMu.Unlock()
				a.updateDetailsView()
			}
		})
	}()
}

// toggleIssueExpanded toggles the expand/collapse state of a parent issue.
func (a *App) toggleIssueExpanded(issueID string) {
	// Check both sections for the issue
	var issue *linearapi.Issue
	var ok bool
	if issue, ok = a.myIDToIssue[issueID]; !ok {
		if issue, ok = a.otherIDToIssue[issueID]; !ok {
			logger.Debug("tui.app: issue not found for toggle issue_id=%s", issueID)
			return
		}
	}

	if issue == nil {
		return
	}

	// Only toggle if this issue has children
	if len(issue.Children) == 0 {
		return
	}

	wasExpanded := a.expandedState[issueID]
	logger.Debug("tui.app: toggling issue expanded issue=%s was_expanded=%v", issue.Identifier, wasExpanded)

	ToggleExpanded(a.expandedState, issueID)

	// Rebuild rows for both sections
	currentUserID := ""
	if a.currentUser != nil {
		currentUserID = a.currentUser.ID
	}
	a.issuesMu.RLock()
	issues := a.issues
	a.issuesMu.RUnlock()
	myIssues, otherIssues := splitIssuesByAssignee(issues, currentUserID)
	a.myIssueRows, a.myIDToIssue = a.buildIssueRowsFor(myIssues)
	a.otherIssueRows, a.otherIDToIssue = a.buildIssueRowsFor(otherIssues)

	// The All Issues tab renders the full list.
	a.issueRows, a.idToIssue = a.buildIssueRowsFor(issues)

	// Render the tables, selecting the toggled issue
	var selectedMyIssueID, selectedOtherIssueID string
	selectedAllIssueID := issueID
	sectionPinned := a.activeIssuesSection == IssuesSectionAll || a.activeIssuesSection == IssuesSectionSearch
	if _, ok := a.myIDToIssue[issueID]; ok {
		selectedMyIssueID = issueID
		if !sectionPinned {
			a.activeIssuesSection = IssuesSectionMy
		}
	} else if _, ok := a.otherIDToIssue[issueID]; ok {
		selectedOtherIssueID = issueID
		if !sectionPinned {
			a.activeIssuesSection = IssuesSectionOther
		}
	}

	a.renderIssueSections(map[IssuesSection]string{
		IssuesSectionMy:    selectedMyIssueID,
		IssuesSectionOther: selectedOtherIssueID,
		IssuesSectionAll:   selectedAllIssueID,
	})
	a.updateIssuesColumnLayout()
}

// onNavigationSelected handles when a navigation item is selected.
func (a *App) onNavigationSelected(node *NavigationNode) {
	logger.Debug("tui.app: navigation selected node_id=%s node_text=%s is_team=%v is_project=%v is_cycle=%v is_issue=%v", node.ID, node.Text, node.IsTeam, node.IsProject, node.IsCycle, node.IsIssue)

	// A new list starts fresh: its own view settings apply again until the
	// user overrides them.
	a.groupingOverridden = false
	a.sortOverridden = false

	// A favorited issue is not a filter of its own: scope to its team and ask
	// the refresh to land on the issue via the target-issue plumbing.
	if node.IsIssue {
		a.selectedNavigation = &NavigationNode{
			ID:     node.TeamID,
			Text:   node.Text,
			TeamID: node.TeamID,
			IsTeam: true,
		}
		if node.TeamID != "" {
			go a.preloadTeamMetadataFunc(node.TeamID)
		}
		go a.refreshIssuesWithFocusChange(false, node.IssueID)
		return
	}

	a.selectedNavigation = node

	// Update selected team metadata for commands and create-issue defaults.
	if node.TeamID != "" {
		go a.preloadTeamMetadataFunc(node.TeamID)
	}

	// Refresh issues for the new selection - run in goroutine to avoid blocking
	// the tview callback (QueueUpdateDraw deadlocks if called from within a callback)
	go a.refreshIssuesWithFocusChange(false)
}

// preloadTeamMetadata warms team-scoped metadata caches for commands and create-issue defaults.
func (a *App) preloadTeamMetadata(teamID string) {
	logger.Debug("tui.app: preloading team metadata team_id=%s", teamID)
	ctx := context.Background()
	_ = a.cache.PreloadTeamMetadata(ctx, teamID)

	users, _ := a.cache.GetUsers(ctx, teamID)
	projects, _ := a.cache.GetProjects(ctx, teamID)
	states, _ := a.cache.GetWorkflowStates(ctx, teamID)
	cycles, _ := a.cache.GetCycles(ctx, teamID)

	logger.Debug("tui.app: loaded team metadata team_id=%s users_count=%d projects_count=%d states_count=%d cycles_count=%d", teamID, len(users), len(projects), len(states), len(cycles))
	a.app.QueueUpdateDraw(func() {
		a.teamUsers = users
		a.teamProjects = projects
		a.workflowStates = states
		a.teamCycles = cycles
	})
}

// setSortFields sets the sort chain and refreshes issues. A manual sort
// choice outranks the active view's ordering for the session, and follows the
// grouping pickers in updating the in-memory config so a later settings save
// records the choice instead of reverting it.
func (a *App) setSortFields(fields []SortField) {
	if len(fields) == 0 {
		return
	}
	logger.Debug("tui.app: setting sort chain fields=%s", sortChainLabel(fields))
	a.sortFields = fields
	a.sortOverridden = true
	a.config.SortBy = sortConfigNames(fields)

	// Reorder what is already loaded before the refresh, so the list matches
	// the status bar even when the fetch fails.
	a.issuesMu.Lock()
	a.sortIssuesLocally()
	a.issuesMu.Unlock()
	a.regroupIssues("")

	// Run in goroutine to avoid deadlock when called from tview callbacks
	go a.refreshIssues()
}

// updateStatusBar updates the status bar with current information.
func (a *App) updateStatusBar() {
	var helpText string
	keyColor := a.themeTags.SecondaryText

	switch a.focusedPane {
	case FocusNavigation:
		helpText = fmt.Sprintf("%s↑↓: navigate | Enter: select | Tab/→/l: next pane | Shift+Tab/←/h: prev pane | :: palette | /: search | q: quit[-]", keyColor)
	case FocusIssues:
		helpText = fmt.Sprintf("%sj/k: navigate | Enter: select | Tab/→/l: next pane | Shift+Tab/←/h: prev pane | :: palette | /: search | q: quit[-]", keyColor)
	case FocusDetails:
		helpText = fmt.Sprintf("%sj/k: scroll | Tab: switch description/comments | →/l: next pane | Shift+Tab/←/h: prev pane | :: palette | /: search | q: quit[-]", keyColor)
	case FocusPalette:
		helpText = fmt.Sprintf("%s↑↓: navigate | Enter: execute | Esc: close[-]", keyColor)
	default:
		helpText = fmt.Sprintf("%sj/k: navigate | Tab: next pane | Shift+Tab: prev pane | :: palette | /: search | q: quit[-]", keyColor)
	}

	navText := ""
	if a.selectedNavigation != nil {
		label := a.selectedNavigation.Text
		if a.selectedNavigation.IsStatus {
			if a.selectedNavigation.StateName != "" {
				label = fmt.Sprintf("Status: %s", a.selectedNavigation.StateName)
			} else {
				label = "Status"
			}
		} else if a.selectedNavigation.IsCycle {
			if a.selectedNavigation.CycleName != "" {
				label = fmt.Sprintf("Cycle: %s", a.selectedNavigation.CycleName)
			} else {
				label = "Cycle"
			}
		}
		navText = fmt.Sprintf("%s%s[-]", a.themeTags.Accent, label)
	}

	filterText := ""
	if !a.richFilters.Empty() {
		filterText = fmt.Sprintf("%sFilters: %s[-]", a.themeTags.Warning, a.richFilters.Summary())
	}

	a.issuesMu.RLock()
	issuesLen := len(a.issues)
	a.issuesMu.RUnlock()
	statusText := fmt.Sprintf("%s%d issues[-]", a.themeTags.Accent, issuesLen)
	if issuesLen == 0 {
		statusText = fmt.Sprintf("%sNo issues[-]", a.themeTags.SecondaryText)
	}

	sep := fmt.Sprintf("%s | [-]", a.themeTags.Border)

	sortText := fmt.Sprintf("%sSort: %s[-]", a.themeTags.SecondaryText, sortChainLabel(a.effectiveSortFields()))

	parts := []string{helpText}
	if navText != "" {
		parts = append(parts, navText)
	}
	if filterText != "" {
		parts = append(parts, filterText)
	}
	parts = append(parts, sortText)
	if a.statusMessage != "" {
		parts = append(parts, fmt.Sprintf("%s%s[-]", a.themeTags.Accent, a.statusMessage))
	}
	parts = append(parts, statusText)

	text := parts[0]
	for i := 1; i < len(parts); i++ {
		text += sep + parts[i]
	}

	a.statusBar.SetText(text)
}

// updateStatusBarWithError updates the status bar with an error message.
func (a *App) updateStatusBarWithError(err error) {
	a.statusBar.SetText(fmt.Sprintf("%sError: %v[-]", a.themeTags.Error, err))
}

func (a *App) flashStatus(message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	a.statusMessage = message
	a.statusBar.SetText(fmt.Sprintf("%s%s[-]", a.themeTags.Accent, message))
}

// GetAPI returns the Linear API client (used by commands).
func (a *App) GetAPI() *linearapi.Client {
	return a.api
}

// GetCache returns the team cache (used by commands).
func (a *App) GetCache() *cache.TeamCache {
	return a.cache
}

// GetSelectedIssue returns the currently selected issue.
func (a *App) GetSelectedIssue() *linearapi.Issue {
	a.issuesMu.RLock()
	defer a.issuesMu.RUnlock()
	return a.selectedIssue
}

// GetSelectedTeamID returns the currently selected team ID, if any.
func (a *App) GetSelectedTeamID() string {
	if a.selectedNavigation != nil && a.selectedNavigation.TeamID != "" {
		return a.selectedNavigation.TeamID
	}
	// If we have a selected issue, use its team
	a.issuesMu.RLock()
	selectedIssue := a.selectedIssue
	a.issuesMu.RUnlock()
	if selectedIssue != nil {
		return selectedIssue.TeamID
	}
	return ""
}

// GetCurrentUser returns the current authenticated user.
func (a *App) GetCurrentUser() *linearapi.User {
	return a.currentUser
}

// GetTeamUsers returns the users for the currently selected team.
func (a *App) GetTeamUsers() []linearapi.User {
	return a.teamUsers
}

// FetchTeamUsers fetches users for a specific team from the API.
func (a *App) FetchTeamUsers(teamID string) ([]linearapi.User, error) {
	ctx := context.Background()
	users, err := a.cache.GetUsers(ctx, teamID)
	if err != nil {
		return nil, err
	}
	a.teamUsers = users
	return users, nil
}

// GetTeamProjects returns the projects for the currently selected team.
func (a *App) GetTeamProjects() []linearapi.Project {
	return a.teamProjects
}

// FetchTeamProjects fetches projects for a specific team from the API.
func (a *App) FetchTeamProjects(teamID string) ([]linearapi.Project, error) {
	ctx := context.Background()
	projects, err := a.cache.GetProjects(ctx, teamID)
	if err != nil {
		return nil, err
	}
	a.teamProjects = projects
	return projects, nil
}

// GetTeamCycles returns the cycles for the currently selected team.
func (a *App) GetTeamCycles() []linearapi.Cycle {
	return a.teamCycles
}

// FetchTeamCycles fetches cycles for a specific team from the API.
func (a *App) FetchTeamCycles(teamID string) ([]linearapi.Cycle, error) {
	ctx := context.Background()
	cycles, err := a.cache.GetCycles(ctx, teamID)
	if err != nil {
		return nil, err
	}
	sortCyclesForNavigation(cycles)
	a.teamCycles = cycles
	return cycles, nil
}

// GetWorkflowStates returns the workflow states for the currently selected team.
func (a *App) GetWorkflowStates() []linearapi.WorkflowState {
	return a.workflowStates
}

// QueueUpdateDraw queues a UI update function to be run in the main thread.
func (a *App) QueueUpdateDraw(f func()) {
	if a.queueUpdateDraw != nil {
		// Serialize UI updates when test overrides queueUpdateDraw to execute immediately
		a.uiUpdateMu.Lock()
		defer a.uiUpdateMu.Unlock()
		a.queueUpdateDraw(f)
		return
	}
	a.app.QueueUpdateDraw(f)
}

// loadPickerData loads picker data asynchronously if not already cached.
func (a *App) loadPickerData(
	resourceName string,
	hasData func() bool,
	loadData func(ctx context.Context, teamID string) error,
	onLoaded func(),
) {
	teamID := a.GetSelectedTeamID()
	if teamID == "" {
		logger.Warning("tui.app: cannot show %s picker, no team selected", resourceName)
		return
	}
	go func() {
		logger.Debug("tui.app: loading %s team_id=%s", resourceName, teamID)
		ctx := context.Background()
		if err := loadData(ctx, teamID); err != nil {
			logger.ErrorWithErr(err, "tui.app: failed to load %s team_id=%s", resourceName, teamID)
			a.QueueUpdateDraw(func() {
				a.updateStatusBarWithError(err)
			})
			return
		}
		logger.Debug("tui.app: loaded %s team_id=%s", resourceName, teamID)
		a.QueueUpdateDraw(onLoaded)
	}()
}

// ShowStatusPicker shows a picker for workflow states. contextLine names the
// issue being modified; empty for non-issue uses like filters.
func (a *App) ShowStatusPicker(contextLine string, onSelect func(stateID string)) {
	logger.Debug("tui.app: showing status picker")
	states := a.workflowStates
	if len(states) == 0 {
		a.loadPickerData(
			"workflow states",
			func() bool { return len(a.workflowStates) > 0 },
			func(ctx context.Context, teamID string) error {
				loadedStates, err := a.cache.GetWorkflowStates(ctx, teamID)
				if err != nil {
					return err
				}
				a.workflowStates = loadedStates
				return nil
			},
			func() {
				a.showStatusPickerWithStates(a.workflowStates, contextLine, onSelect)
			},
		)
		return
	}
	a.showStatusPickerWithStates(states, contextLine, onSelect)
}

func (a *App) showStatusPickerWithStates(states []linearapi.WorkflowState, contextLine string, onSelect func(stateID string)) {
	items := make([]PickerItem, 0, len(states))
	for _, state := range states {
		items = append(items, PickerItem{
			ID:    state.ID,
			Label: state.Name,
		})
	}

	a.pickerActive = true
	a.pickerModal.ShowWithContext("Select Status", contextLine, items, func(item PickerItem) {
		a.pickerActive = false
		onSelect(item.ID)
	})
}

// ShowUserPicker shows a picker for team users. contextLine names the issue
// being modified; empty for non-issue uses like filters.
func (a *App) ShowUserPicker(contextLine string, onSelect func(userID string)) {
	logger.Debug("tui.app: showing user picker")
	users := a.teamUsers
	if len(users) == 0 {
		a.loadPickerData(
			"users for picker",
			func() bool { return len(a.teamUsers) > 0 },
			func(ctx context.Context, teamID string) error {
				loadedUsers, err := a.cache.GetUsers(ctx, teamID)
				if err != nil {
					return err
				}
				a.teamUsers = loadedUsers
				return nil
			},
			func() {
				a.showUserPickerWithUsers(a.teamUsers, contextLine, onSelect)
			},
		)
		return
	}
	a.showUserPickerWithUsers(users, contextLine, onSelect)
}

func (a *App) showUserPickerWithUsers(users []linearapi.User, contextLine string, onSelect func(userID string)) {
	items := make([]PickerItem, 0, len(users))
	for _, user := range users {
		label := user.Name
		if user.IsMe {
			label += " (me)"
		}
		items = append(items, PickerItem{
			ID:    user.ID,
			Label: label,
		})
	}

	a.pickerActive = true
	a.pickerModal.ShowWithContext("Select Assignee", contextLine, items, func(item PickerItem) {
		a.pickerActive = false
		onSelect(item.ID)
	})
}

// ShowCyclePicker shows a picker for team cycles. contextLine names the
// issue being modified; empty for non-issue uses like filters.
func (a *App) ShowCyclePicker(contextLine string, onSelect func(cycleID string)) {
	logger.Debug("tui.app: showing cycle picker")
	cycles := a.teamCycles
	if len(cycles) == 0 {
		a.loadPickerData(
			"cycles for picker",
			func() bool { return len(a.teamCycles) > 0 },
			func(ctx context.Context, teamID string) error {
				loadedCycles, err := a.cache.GetCycles(ctx, teamID)
				if err != nil {
					return err
				}
				sortCyclesForNavigation(loadedCycles)
				a.teamCycles = loadedCycles
				return nil
			},
			func() {
				a.showCyclePickerWithCycles(a.teamCycles, contextLine, onSelect)
			},
		)
		return
	}
	a.showCyclePickerWithCycles(cycles, contextLine, onSelect)
}

func (a *App) showCyclePickerWithCycles(cycles []linearapi.Cycle, contextLine string, onSelect func(cycleID string)) {
	items := make([]PickerItem, 0, len(cycles))
	for _, cycle := range cycles {
		label := cycle.DisplayName()
		switch {
		case cycle.IsActive:
			label += " (active)"
		case cycle.IsNext:
			label += " (next)"
		case cycle.IsPrevious:
			label += " (previous)"
		}
		items = append(items, PickerItem{
			ID:    cycle.ID,
			Label: label,
		})
	}

	a.pickerActive = true
	a.pickerModal.ShowWithContext("Select Cycle", contextLine, items, func(item PickerItem) {
		a.pickerActive = false
		onSelect(item.ID)
	})
}

// ShowProjectPicker shows a picker for team projects. contextLine names the
// issue being modified; empty for non-issue uses like filters.
func (a *App) ShowProjectPicker(contextLine string, onSelect func(projectID string)) {
	logger.Debug("tui.app: showing project picker")
	projects := a.teamProjects
	if len(projects) == 0 {
		a.loadPickerData(
			"projects for picker",
			func() bool { return len(a.teamProjects) > 0 },
			func(ctx context.Context, teamID string) error {
				loadedProjects, err := a.cache.GetProjects(ctx, teamID)
				if err != nil {
					return err
				}
				a.teamProjects = loadedProjects
				return nil
			},
			func() {
				a.showProjectPickerWithProjects(a.teamProjects, contextLine, onSelect)
			},
		)
		return
	}
	a.showProjectPickerWithProjects(projects, contextLine, onSelect)
}

func (a *App) showProjectPickerWithProjects(projects []linearapi.Project, contextLine string, onSelect func(projectID string)) {
	items := make([]PickerItem, 0, len(projects))
	for _, project := range projects {
		items = append(items, PickerItem{
			ID:    project.ID,
			Label: project.Name,
		})
	}

	a.pickerActive = true
	a.pickerModal.ShowWithContext("Select Project", contextLine, items, func(item PickerItem) {
		a.pickerActive = false
		onSelect(item.ID)
	})
}

// ShowParentIssuePicker shows a picker for selecting a parent issue.
// It lists all top-level issues (issues without a parent) from the current
// list. contextLine names the issue being reparented.
func (a *App) ShowParentIssuePicker(contextLine string, onSelect func(parentID string)) {
	// Filter to only show issues that could be parents (no parent themselves)
	a.issuesMu.RLock()
	issues := a.issues
	selectedIssue := a.selectedIssue
	a.issuesMu.RUnlock()
	excludedIDs := excludedParentCandidateIDs(selectedIssue, issues)
	items := make([]PickerItem, 0)
	for _, issue := range issues {
		if issue.Parent == nil && !excludedIDs[issue.ID] {
			items = append(items, PickerItem{
				ID:    issue.ID,
				Label: issue.Identifier + " - " + issue.Title,
			})
		}
	}

	if len(items) == 0 {
		logger.Warning("tui.app: no parent issues available for picker")
		a.updateStatusBarWithError(fmt.Errorf("no parent issues available"))
		return
	}
	logger.Debug("tui.app: parent issue picker items count=%d", len(items))

	a.pickerActive = true
	a.pickerModal.ShowWithContext("Select Parent Issue", contextLine, items, func(item PickerItem) {
		a.pickerActive = false
		onSelect(item.ID)
	})
}

func excludedParentCandidateIDs(selected *linearapi.Issue, issues []linearapi.Issue) map[string]bool {
	excluded := make(map[string]bool)
	if selected == nil {
		return excluded
	}
	excluded[selected.ID] = true
	byID := make(map[string]linearapi.Issue, len(issues))
	for _, issue := range issues {
		byID[issue.ID] = issue
	}
	var visit func(issue linearapi.Issue)
	visit = func(issue linearapi.Issue) {
		for _, child := range issue.Children {
			if excluded[child.ID] {
				continue
			}
			excluded[child.ID] = true
			if fullChild, ok := byID[child.ID]; ok {
				visit(fullChild)
			}
		}
	}
	visit(*selected)
	return excluded
}

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
				a.fetchingIssueID = issueID
				go func() {
					fullIssue, fetchErr := a.api.FetchIssueByID(ctx, issueID)
					a.QueueUpdateDraw(func() {
						if a.fetchingIssueID == issueID {
							if fetchErr != nil {
								logger.ErrorWithErr(fetchErr, "tui.app: failed to refresh issue after description update issue=%s", issueID)
								return
							}
							a.issuesMu.Lock()
							a.selectedIssue = &fullIssue
							a.issuesMu.Unlock()
							a.updateDetailsView()
						}
					})
				}()
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
