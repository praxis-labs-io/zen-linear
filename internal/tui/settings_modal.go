package tui

import (
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/praxis-labs-io/zen-linear/internal/agents"
	"github.com/praxis-labs-io/zen-linear/internal/config"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
	"github.com/rivo/tview"
)

const (
	defaultAgentModelLabel = "default (use provider default)"
	settingsModalWidth     = 110
)

// agentModelOption pairs a model id with its display label.
type agentModelOption struct {
	id    string
	label string
}

// cursorModelOptions returns Cursor model options, preferring the CLI list.
func cursorModelOptions() []agentModelOption {
	options, err := cursorModelOptionsFromCLI()
	if err == nil && len(options) > 0 {
		return options
	}
	return cursorModelFallbackOptions()
}

// cursorModelFallbackOptions returns a static fallback list for Cursor models.
func cursorModelFallbackOptions() []agentModelOption {
	return []agentModelOption{
		{id: "auto", label: "auto - Auto"},
		{id: "composer-1", label: "composer-1 - Composer 1"},
		{id: "gpt-5.2-codex", label: "gpt-5.2-codex - GPT-5.2 Codex"},
		{id: "gpt-5.2-codex-high", label: "gpt-5.2-codex-high - GPT-5.2 Codex High"},
		{id: "gpt-5.2-codex-low", label: "gpt-5.2-codex-low - GPT-5.2 Codex Low"},
		{id: "gpt-5.2-codex-xhigh", label: "gpt-5.2-codex-xhigh - GPT-5.2 Codex Extra High"},
		{id: "gpt-5.2-codex-fast", label: "gpt-5.2-codex-fast - GPT-5.2 Codex Fast"},
		{id: "gpt-5.2-codex-high-fast", label: "gpt-5.2-codex-high-fast - GPT-5.2 Codex High Fast"},
		{id: "gpt-5.2-codex-low-fast", label: "gpt-5.2-codex-low-fast - GPT-5.2 Codex Low Fast"},
		{id: "gpt-5.2-codex-xhigh-fast", label: "gpt-5.2-codex-xhigh-fast - GPT-5.2 Codex Extra High Fast"},
		{id: "gpt-5.1-codex-max", label: "gpt-5.1-codex-max - GPT-5.1 Codex Max"},
		{id: "gpt-5.1-codex-max-high", label: "gpt-5.1-codex-max-high - GPT-5.1 Codex Max High"},
		{id: "gpt-5.2", label: "gpt-5.2 - GPT-5.2"},
		{id: "opus-4.5-thinking", label: "opus-4.5-thinking - Claude 4.5 Opus (Thinking)"},
		{id: "gpt-5.2-high", label: "gpt-5.2-high - GPT-5.2 High"},
		{id: "gemini-3-pro", label: "gemini-3-pro - Gemini 3 Pro"},
		{id: "opus-4.5", label: "opus-4.5 - Claude 4.5 Opus"},
		{id: "sonnet-4.5", label: "sonnet-4.5 - Claude 4.5 Sonnet"},
		{id: "sonnet-4.5-thinking", label: "sonnet-4.5-thinking - Claude 4.5 Sonnet (Thinking)"},
		{id: "gpt-5.1-high", label: "gpt-5.1-high - GPT-5.1 High"},
		{id: "gemini-3-flash", label: "gemini-3-flash - Gemini 3 Flash"},
		{id: "grok", label: "grok - Grok"},
	}
}

// cursorModelOptionsFromCLI loads model options from cursor-agent.
func cursorModelOptionsFromCLI() ([]agentModelOption, error) {
	binary, err := resolveCursorAgentBinary()
	if err != nil {
		return nil, err
	}
	output, err := exec.Command(binary, "--list-models").CombinedOutput()
	if err != nil {
		logger.Debug("tui.settings: failed to list cursor models binary=%s error=%v", binary, err)
		return nil, fmt.Errorf("list models: %w", err)
	}
	options := parseCursorModelOptions(string(output))
	if len(options) == 0 {
		return nil, fmt.Errorf("no cursor models parsed")
	}
	return options, nil
}

// resolveCursorAgentBinary resolves the cursor-agent executable path.
func resolveCursorAgentBinary() (string, error) {
	if path, err := exec.LookPath("cursor-agent"); err == nil {
		return path, nil
	}
	if path, err := exec.LookPath("agent"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("cursor-agent not found in PATH")
}

// parseCursorModelOptions parses `cursor-agent --list-models` output into options.
func parseCursorModelOptions(output string) []agentModelOption {
	clean := stripANSICodes(output)
	lines := strings.Split(clean, "\n")
	var options []agentModelOption
	for _, line := range lines {
		item := strings.TrimSpace(line)
		if item == "" {
			continue
		}
		lower := strings.ToLower(item)
		if strings.HasPrefix(lower, "loading models") || strings.HasPrefix(lower, "available models") || strings.HasPrefix(lower, "tip:") {
			continue
		}
		item = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(item, "(current)", ""), "(default)", ""))
		if item == "" {
			continue
		}
		id, label := parseModelLine(item)
		if id == "" {
			continue
		}
		options = append(options, agentModelOption{id: id, label: label})
	}
	return options
}

// parseModelLine splits a "id - label" line into id and label.
func parseModelLine(item string) (string, string) {
	parts := strings.SplitN(item, " - ", 2)
	if len(parts) == 1 {
		id := strings.TrimSpace(parts[0])
		return id, id
	}
	id := strings.TrimSpace(parts[0])
	label := strings.TrimSpace(parts[1])
	if id == "" || label == "" {
		return "", ""
	}
	return id, fmt.Sprintf("%s - %s", id, label)
}

// stripANSICodes removes ANSI escape sequences from CLI output.
func stripANSICodes(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	skipping := false
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if skipping {
			if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
				skipping = false
			}
			continue
		}
		if ch == 0x1b {
			skipping = true
			continue
		}
		if ch == '\r' {
			continue
		}
		builder.WriteByte(ch)
	}
	return builder.String()
}

// claudeModelOptions returns Claude model options supported by `claude --model`.
func claudeModelOptions() []agentModelOption {
	return []agentModelOption{
		{id: "sonnet", label: "Claude Sonnet"},
		{id: "opus", label: "Claude Opus"},
		{id: "haiku", label: "Claude Haiku"},
	}
}

// defaultAgentModelOptions returns the default-only model dropdown values.
func defaultAgentModelOptions() ([]string, []string) {
	return []string{defaultAgentModelLabel}, []string{""}
}

// selectAvailableProvider chooses a valid provider key from available options.
func selectAvailableProvider(configProvider string, available []string) string {
	normalized := strings.ToLower(strings.TrimSpace(configProvider))
	for _, option := range available {
		if option == normalized {
			return option
		}
	}
	if len(available) > 0 {
		return available[0]
	}
	return ""
}

// agentModelOptionsForProvider builds model labels and values for a provider.
func agentModelOptionsForProvider(provider string) ([]string, []string) {
	labels, values := defaultAgentModelOptions()
	normalized := strings.ToLower(strings.TrimSpace(provider))
	if normalized == "" {
		return labels, values
	}
	var options []agentModelOption
	switch normalized {
	case "cursor":
		options = cursorModelOptions()
	case "claude":
		options = claudeModelOptions()
	default:
		return labels, values
	}
	for _, option := range options {
		labels = append(labels, option.label)
		values = append(values, option.id)
	}
	return labels, values
}

// SettingsModal manages the settings form overlay.
type SettingsModal struct {
	app                 *App
	fm                  *FormModal
	endpointField       *tview.InputField
	timeoutField        *tview.InputField
	pageSizeField       *tview.InputField
	cacheTTLField       *tview.InputField
	searchDebounceField *tview.InputField
	logFileField        *tview.InputField
	logLevelField       *FormPicker
	logLevelOptions     []string
	themeField          *FormPicker
	themeOptions        []string
	themeValues         []string
	densityField        *FormPicker
	roundedBordersField *FormPicker
	sessionRestoreField *FormPicker
	// booleanOptions backs every on/off picker, so they read the same way.
	booleanOptions       []string
	densityOptions       []string
	densityValues        []string
	agentProviderField   *FormPicker
	agentProviderOptions []string
	agentSandboxField    *FormPicker
	agentSandboxOptions  []string
	agentModelField      *FormPicker
	agentModelOptions    []string
	agentModelValues     []string
	agentWorkspaceField  *tview.InputField
	defaultTeamField     *tview.InputField
	defaultProjectField  *tview.InputField
}

// NewSettingsModal creates a new settings modal.
func NewSettingsModal(app *App) *SettingsModal {
	availableProviders := agents.AvailableProviderKeys(exec.LookPath)
	selectedProvider := selectAvailableProvider(config.DefaultAgentProvider, availableProviders)
	modelLabels, modelValues := agentModelOptionsForProvider(selectedProvider)
	sm := &SettingsModal{
		app:                  app,
		logLevelOptions:      []string{"debug", "info", "warning", "error"},
		themeOptions:         []string{"Terminal (adaptive)", "Linear", "High contrast", "Color-blind friendly", "Rose Pine Moon (transparent)"},
		themeValues:          []string{config.ThemeTerminal, config.ThemeLinear, config.ThemeHighContrast, config.ThemeColorBlind, config.ThemeRosePineMoon},
		densityOptions:       []string{"Comfortable", "Compact"},
		densityValues:        []string{config.DensityComfortable, config.DensityCompact},
		agentProviderOptions: availableProviders,
		agentSandboxOptions:  []string{"enabled", "disabled"},
		booleanOptions:       []string{"enabled", "disabled"},
		agentModelOptions:    modelLabels,
		agentModelValues:     modelValues,
	}

	sm.fm = NewFormModal(app, "Settings")
	sm.fm.SetMaxWidth(settingsModalWidth)

	sm.endpointField = sm.fm.AddInput("API endpoint", "")
	sm.timeoutField = sm.fm.AddInput("Timeout", "")
	sm.pageSizeField = sm.fm.AddInput("Page size", "")
	sm.cacheTTLField = sm.fm.AddInput("Cache TTL", "")
	sm.searchDebounceField = sm.fm.AddInput("Search debounce", "")
	sm.logFileField = sm.fm.AddInput("Log file", "")

	sm.logLevelField = sm.fm.AddPicker("Log level", sm.logLevelOptions, 0, nil)
	sm.themeField = sm.fm.AddPicker("Theme", sm.themeOptions, 0, nil)
	sm.densityField = sm.fm.AddPicker("Density", sm.densityOptions, 0, nil)
	// Consecutive pickers share one row, so each group needs its own break or
	// all eight pack into a single row and clip their labels and values.
	sm.fm.EndRow()

	sm.roundedBordersField = sm.fm.AddPicker("Rounded borders", sm.booleanOptions, booleanOptionIndex(false), nil)
	sm.sessionRestoreField = sm.fm.AddPicker("Restore last session", sm.booleanOptions, booleanOptionIndex(true), nil)
	sm.fm.EndRow()

	sm.agentProviderField = sm.fm.AddPicker("Agent provider", sm.agentProviderOptions, 0, func(text string, index int) {
		_ = index
		sm.setAgentModelOptionsForProvider(text)
	})
	sm.agentSandboxField = sm.fm.AddPicker("Agent sandbox", sm.agentSandboxOptions, 0, nil)
	sm.agentModelField = sm.fm.AddPicker("Agent model", sm.agentModelOptions, 0, nil)

	sm.agentWorkspaceField = sm.fm.AddInput("Agent workspace (blank uses CWD)", "")
	sm.defaultTeamField = sm.fm.AddInput("Default team (blank opens All Issues)", "")
	sm.defaultProjectField = sm.fm.AddInput("Default project (requires default team)", "")

	sm.fm.AddButtons(
		FormButton{Label: "Save", OnPress: sm.saveSettings},
		FormButton{Label: "Cancel", OnPress: sm.Hide},
	)
	sm.fm.SetOnSubmit(sm.saveSettings)
	sm.fm.SetOnCancel(sm.Hide)
	sm.fm.SetHint("Esc cancel · Tab next · ⏎ open dropdown · ⌃⏎ save")

	return sm
}

// Show displays the settings modal with current configuration values.
func (sm *SettingsModal) Show() {
	logger.Debug("tui.settings: showing settings modal")
	settings := config.SettingsFromConfig(sm.app.config)
	availableProviders := agents.AvailableProviderKeys(exec.LookPath)
	sm.setAgentProviderOptions(availableProviders)
	selectedProvider := selectAvailableProvider(settings.AgentProvider, availableProviders)

	sm.endpointField.SetText(settings.APIEndpoint)
	sm.timeoutField.SetText(settings.Timeout)
	sm.pageSizeField.SetText(strconv.Itoa(settings.PageSize))
	sm.cacheTTLField.SetText(settings.CacheTTL)
	sm.searchDebounceField.SetText(settings.SearchDebounce)
	sm.logFileField.SetText(settings.ResolvedLogFile())
	sm.fm.SetContext(envOverrideNotice(sm.app.envOverrides))
	sm.setLogLevelSelection(settings.LogLevel)
	sm.setThemeSelection(settings.Theme)
	sm.setDensitySelection(settings.Density)
	sm.roundedBordersField.SetCurrentOption(booleanOptionIndex(settings.RoundedBorders))
	sm.sessionRestoreField.SetCurrentOption(booleanOptionIndex(settings.SessionRestore))
	sm.setAgentProviderSelection(selectedProvider)
	sm.setAgentSandboxSelection(settings.AgentSandbox)
	sm.setAgentModelOptionsForProvider(selectedProvider)
	sm.setAgentModelSelection(settings.AgentModel)
	sm.agentWorkspaceField.SetText(settings.AgentWorkspace)
	sm.defaultTeamField.SetText(settings.DefaultTeam)
	sm.defaultProjectField.SetText(settings.DefaultProject)

	sm.fm.Show("settings")
}

// currentAgentModelValue returns the currently selected model value.
func (sm *SettingsModal) currentAgentModelValue() string {
	index, _ := sm.agentModelField.GetCurrentOption()
	if index >= 0 && index < len(sm.agentModelValues) {
		return sm.agentModelValues[index]
	}
	return ""
}

// setAgentModelOptionsForProvider updates model options for the given provider.
func (sm *SettingsModal) setAgentModelOptionsForProvider(provider string) {
	// The provider picker's initial selection fires during construction,
	// before the model field exists; the struct literal already holds the
	// right initial options then.
	if sm.agentModelField == nil {
		return
	}
	currentValue := sm.currentAgentModelValue()
	labels, values := agentModelOptionsForProvider(provider)
	sm.agentModelOptions = labels
	sm.agentModelValues = values
	sm.fm.SetPickerOptions(sm.agentModelField, sm.agentModelOptions, nil)
	if currentValue != "" {
		sm.setAgentModelSelection(currentValue)
		return
	}
	sm.setAgentModelSelection("")
}

// setAgentProviderOptions updates the provider dropdown options and callback.
func (sm *SettingsModal) setAgentProviderOptions(options []string) {
	sm.agentProviderOptions = options
	if sm.agentProviderField == nil {
		return
	}
	sm.fm.SetPickerOptions(sm.agentProviderField, sm.agentProviderOptions, func(text string, index int) {
		_ = index
		sm.setAgentModelOptionsForProvider(text)
	})
}

// Hide hides the settings modal.
func (sm *SettingsModal) Hide() {
	logger.Debug("tui.settings: hiding settings modal")
	sm.fm.Hide("settings")
}

// Focus returns keyboard focus to the form, for when an overlay closes.
func (sm *SettingsModal) Focus() { sm.fm.Focus() }

// HandleKey handles keyboard input for the settings modal.
func (sm *SettingsModal) HandleKey(event *tcell.EventKey) *tcell.EventKey {
	return sm.fm.HandleKey(event)
}

// saveSettings validates input, persists settings, and applies them to the app.
func (sm *SettingsModal) saveSettings() {
	settings, err := sm.settingsFromForm()
	if err != nil {
		logger.ErrorWithErr(err, "tui.settings: failed to build settings from form")
		sm.app.updateStatusBarWithError(err)
		return
	}

	// settings is what goes on disk; the session keeps running with the
	// environment on top, or a save would quietly drop an override that is
	// still exported.
	effective, overrides, err := config.ApplyEnvOverrides(settings)
	if err != nil {
		logger.ErrorWithErr(err, "tui.settings: failed to apply environment overrides")
		sm.app.updateStatusBarWithError(err)
		return
	}

	newCfg, err := config.ConfigFromSettings(sm.app.config.LinearAPIKey, effective)
	if err != nil {
		logger.ErrorWithErr(err, "tui.settings: failed to parse settings")
		sm.app.updateStatusBarWithError(err)
		return
	}

	settingsPath := sm.app.settingsPath
	if err := config.SaveSettings(settingsPath, settings); err != nil {
		logger.ErrorWithErr(err, "tui.settings: failed to save settings path=%s", settingsPath)
		sm.app.updateStatusBarWithError(err)
		return
	}

	// The file has moved on, so a second save must restore against what is
	// there now rather than what launch read.
	sm.app.UseFileSettings(settings, overrides)

	logger.Debug("tui.settings: settings saved successfully path=%s", settingsPath)
	sm.Hide()
	sm.app.applySettings(newCfg)
}

func (sm *SettingsModal) settingsFromForm() (config.Settings, error) {
	pageSizeText := strings.TrimSpace(sm.pageSizeField.GetText())
	pageSize, err := strconv.Atoi(pageSizeText)
	if err != nil {
		return config.Settings{}, fmt.Errorf("page size must be a number: %w", err)
	}

	_, logLevel := sm.logLevelField.GetCurrentOption()
	if logLevel == "" {
		logLevel = config.DefaultLogLevel
	}

	theme := sm.currentThemeValue()
	if theme == "" {
		theme = config.DefaultTheme
	}

	density := sm.currentDensityValue()
	if density == "" {
		density = config.DefaultDensity
	}

	_, agentProvider := sm.agentProviderField.GetCurrentOption()
	if len(sm.agentProviderOptions) == 0 {
		agentProvider = strings.TrimSpace(sm.app.config.AgentProvider)
	}
	if agentProvider == "" {
		agentProvider = config.DefaultAgentProvider
	}

	_, agentSandbox := sm.agentSandboxField.GetCurrentOption()
	if agentSandbox == "" {
		agentSandbox = config.DefaultAgentSandbox
	}

	agentModel := ""
	modelIndex, _ := sm.agentModelField.GetCurrentOption()
	if modelIndex >= 0 && modelIndex < len(sm.agentModelValues) {
		agentModel = sm.agentModelValues[modelIndex]
	}

	settings := config.Settings{
		APIEndpoint:    strings.TrimSpace(sm.endpointField.GetText()),
		Timeout:        strings.TrimSpace(sm.timeoutField.GetText()),
		PageSize:       pageSize,
		CacheTTL:       strings.TrimSpace(sm.cacheTTLField.GetText()),
		SearchDebounce: strings.TrimSpace(sm.searchDebounceField.GetText()),
		LogFile:        config.LogFileSetting(strings.TrimSpace(sm.logFileField.GetText())),
		LogLevel:       logLevel,
		Theme:          theme,
		Density:        density,
		// No form fields; carry the current values so saving settings never
		// strips them from the config file.
		GroupBy:        sm.app.config.GroupBy,
		SubgroupBy:     sm.app.config.SubgroupBy,
		SortBy:         sm.app.config.SortBy,
		Columns:        sm.app.config.Columns,
		RoundedBorders: booleanOptionValue(sm.roundedBordersField),
		SessionRestore: booleanOptionValue(sm.sessionRestoreField),
		AgentProvider:  agentProvider,
		AgentSandbox:   agentSandbox,
		AgentModel:     agentModel,
		AgentWorkspace: strings.TrimSpace(sm.agentWorkspaceField.GetText()),
		// No form field; carry the current value so saving settings never
		// strips it from the config file.
		Keybindings:    sm.app.config.Keybindings,
		DefaultTeam:    strings.TrimSpace(sm.defaultTeamField.GetText()),
		DefaultProject: strings.TrimSpace(sm.defaultProjectField.GetText()),
		// The form has no fields for these; carry the current values through
		// so saving settings never strips them from the config file.
		Workspaces:       sm.app.config.Workspaces,
		DefaultWorkspace: sm.app.config.DefaultWorkspace,
	}

	// The environment is where this session's value came from, not where the
	// next one's should. Writing it back would turn a variable exported for one
	// launch into a stored setting, which is the ZNL-145 shape.
	restoreEnvOverrides(&settings, sm.app.fileSettings, sm.app.envOverrides)

	return settings, nil
}

// envOverrideNotice names the fields the environment owns, for the line pinned
// above the form. Empty when it owns none, which hides the line.
func envOverrideNotice(overrides config.EnvOverrides) string {
	if len(overrides) == 0 {
		return ""
	}
	named := make([]string, 0, len(overrides))
	for field, variable := range overrides {
		named = append(named, fmt.Sprintf("%s ($%s)", field, variable))
	}
	sort.Strings(named)
	return "From the environment, shown but not saved: " + strings.Join(named, ", ")
}

// restoreEnvOverrides puts the file's value back for every field the
// environment took over, so a save writes what config.json should hold rather
// than what this launch happened to run with.
func restoreEnvOverrides(settings *config.Settings, fromFile config.Settings, overrides config.EnvOverrides) {
	if overrides.Has(config.FieldAPIEndpoint) {
		settings.APIEndpoint = fromFile.APIEndpoint
	}
	if overrides.Has(config.FieldTimeout) {
		settings.Timeout = fromFile.Timeout
	}
	if overrides.Has(config.FieldPageSize) {
		settings.PageSize = fromFile.PageSize
	}
	if overrides.Has(config.FieldCacheTTL) {
		settings.CacheTTL = fromFile.CacheTTL
	}
	if overrides.Has(config.FieldLogFile) {
		settings.LogFile = fromFile.LogFile
	}
	if overrides.Has(config.FieldLogLevel) {
		settings.LogLevel = fromFile.LogLevel
	}
}

// setLogLevelSelection updates the dropdown selection to match the provided level.
func (sm *SettingsModal) setLogLevelSelection(level string) {
	selected := 0
	for i, option := range sm.logLevelOptions {
		if option == config.DefaultLogLevel {
			selected = i
		}
		if option == level {
			selected = i
			break
		}
	}
	sm.logLevelField.SetCurrentOption(selected)
}

// currentThemeValue returns the currently selected theme value.
func (sm *SettingsModal) currentThemeValue() string {
	index, _ := sm.themeField.GetCurrentOption()
	if index >= 0 && index < len(sm.themeValues) {
		return sm.themeValues[index]
	}
	return ""
}

// setThemeSelection updates the dropdown selection to match the provided theme.
func (sm *SettingsModal) setThemeSelection(theme string) {
	selected := 0
	for i, value := range sm.themeValues {
		if value == config.DefaultTheme {
			selected = i
		}
		if value == theme {
			selected = i
			break
		}
	}
	sm.themeField.SetCurrentOption(selected)
}

// currentDensityValue returns the currently selected density value.
func (sm *SettingsModal) currentDensityValue() string {
	index, _ := sm.densityField.GetCurrentOption()
	if index >= 0 && index < len(sm.densityValues) {
		return sm.densityValues[index]
	}
	return ""
}

// setDensitySelection updates the dropdown selection to match the provided density.
func (sm *SettingsModal) setDensitySelection(density string) {
	selected := 0
	for i, value := range sm.densityValues {
		if value == config.DefaultDensity {
			selected = i
		}
		if value == density {
			selected = i
			break
		}
	}
	sm.densityField.SetCurrentOption(selected)
}

// setAgentProviderSelection updates the dropdown selection to match the provided provider.
func (sm *SettingsModal) setAgentProviderSelection(provider string) {
	if len(sm.agentProviderOptions) == 0 {
		return
	}
	selected := 0
	for i, option := range sm.agentProviderOptions {
		if option == provider {
			selected = i
			break
		}
	}
	sm.agentProviderField.SetCurrentOption(selected)
}

// booleanOptionIndex maps a flag onto its index in booleanOptions.
func booleanOptionIndex(enabled bool) int {
	if enabled {
		return 0
	}
	return 1
}

// booleanOptionValue reads a flag back off an on/off picker.
func booleanOptionValue(picker *FormPicker) bool {
	index, _ := picker.GetCurrentOption()
	return index == 0
}

// setAgentSandboxSelection updates the dropdown selection to match the provided sandbox value.
func (sm *SettingsModal) setAgentSandboxSelection(sandbox string) {
	selected := 0
	for i, option := range sm.agentSandboxOptions {
		if option == config.DefaultAgentSandbox {
			selected = i
		}
		if option == sandbox {
			selected = i
			break
		}
	}
	sm.agentSandboxField.SetCurrentOption(selected)
}

// setAgentModelSelection updates the dropdown selection to match the provided model.
func (sm *SettingsModal) setAgentModelSelection(model string) {
	selected := 0
	for i, value := range sm.agentModelValues {
		if value == "" {
			selected = i
		}
		if value == model {
			selected = i
			break
		}
	}
	sm.agentModelField.SetCurrentOption(selected)
}
