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
	settingsModalWidth     = 82
	// The context row is one line and does not grow, so more overrides than this are counted rather than named.
	envNoticeMaxFields = 3
)

type agentModelOption struct {
	id    string
	label string
}

func cursorModelOptions() []agentModelOption {
	options, err := cursorModelOptionsFromCLI()
	if err == nil && len(options) > 0 {
		return options
	}
	return cursorModelFallbackOptions()
}

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

func resolveCursorAgentBinary() (string, error) {
	if path, err := exec.LookPath("cursor-agent"); err == nil {
		return path, nil
	}
	if path, err := exec.LookPath("agent"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("cursor-agent not found in PATH")
}

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

func claudeModelOptions() []agentModelOption {
	return []agentModelOption{
		{id: "sonnet", label: "Claude Sonnet"},
		{id: "opus", label: "Claude Opus"},
		{id: "haiku", label: "Claude Haiku"},
	}
}

func defaultAgentModelOptions() ([]string, []string) {
	return []string{defaultAgentModelLabel}, []string{""}
}

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

type SettingsModal struct {
	app                  *App
	fm                   *FormModal
	endpointField        *tview.InputField
	timeoutField         *tview.InputField
	pageSizeField        *tview.InputField
	cacheTTLField        *tview.InputField
	searchDebounceField  *tview.InputField
	logFileField         *tview.InputField
	logLevelField        *FormPicker
	logLevelOptions      []string
	themeField           *FormPicker
	themeOptions         []string
	themeValues          []string
	densityField         *FormPicker
	roundedBordersField  *FormPicker
	imagesField          *FormPicker
	sessionRestoreField  *FormPicker
	updateCheckField     *FormPicker
	booleanOptions       []string
	densityOptions       []string
	densityValues        []string
	imagesOptions        []string
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
		imagesOptions:        []string{config.ImagesAuto, config.ImagesOff},
		booleanOptions:       []string{"enabled", "disabled"},
		agentModelOptions:    modelLabels,
		agentModelValues:     modelValues,
	}

	sm.fm = NewFormModal(app, "Settings")
	sm.fm.SetMaxWidth(settingsModalWidth)

	sm.fm.BeginSection("Appearance")
	sm.themeField = sm.fm.AddPicker("Theme", sm.themeOptions, 0, nil)
	sm.fm.EndRow()
	sm.densityField = sm.fm.AddPicker("Density", sm.densityOptions, 0, nil)
	sm.fm.EndRow()
	sm.roundedBordersField = sm.fm.AddPicker("Rounded borders", sm.booleanOptions, booleanOptionIndex(false), nil)
	sm.fm.EndRow()
	sm.imagesField = sm.fm.AddPicker("Images", sm.imagesOptions, 0, nil)
	sm.fm.EndRow()

	sm.fm.BeginSection("Startup")
	sm.sessionRestoreField = sm.fm.AddPicker("Restore session", sm.booleanOptions, booleanOptionIndex(true), nil)
	sm.fm.EndRow()
	sm.updateCheckField = sm.fm.AddPicker("Update check", sm.booleanOptions, booleanOptionIndex(true), nil)
	sm.fm.EndRow()
	sm.defaultTeamField = sm.fm.AddInput("Default team", "")
	sm.fm.SetPlaceholder(sm.defaultTeamField, "blank opens All Issues")
	sm.defaultProjectField = sm.fm.AddInput("Default project", "")
	sm.fm.SetPlaceholder(sm.defaultProjectField, "requires a default team")

	sm.fm.BeginSection("Agents")
	sm.agentProviderField = sm.fm.AddPicker("Provider", sm.agentProviderOptions, 0, func(text string, index int) {
		_ = index
		sm.setAgentModelOptionsForProvider(text)
	})
	sm.fm.EndRow()
	sm.agentSandboxField = sm.fm.AddPicker("Sandbox", sm.agentSandboxOptions, 0, nil)
	sm.fm.EndRow()
	sm.agentModelField = sm.fm.AddPicker("Model", sm.agentModelOptions, 0, nil)
	sm.fm.EndRow()
	sm.agentWorkspaceField = sm.fm.AddInput("Workspace", "")
	sm.fm.SetPlaceholder(sm.agentWorkspaceField, "blank runs in the current directory")

	sm.fm.BeginSection("Network")
	sm.endpointField = sm.fm.AddInput("API endpoint", "")
	sm.timeoutField = sm.fm.AddInput("Timeout", "")
	sm.pageSizeField = sm.fm.AddInput("Page size", "")
	sm.cacheTTLField = sm.fm.AddInput("Cache TTL", "")
	sm.searchDebounceField = sm.fm.AddInput("Search debounce", "")

	sm.fm.BeginSection("Logging")
	sm.logFileField = sm.fm.AddInput("Log file", "")
	sm.logLevelField = sm.fm.AddPicker("Log level", sm.logLevelOptions, 0, nil)
	sm.fm.EndRow()

	sm.fm.AddButtons(
		FormButton{Label: "Save", OnPress: sm.saveSettings},
		FormButton{Label: "Cancel", OnPress: sm.Hide},
	)
	sm.fm.SetOnSubmit(sm.saveSettings)
	sm.fm.SetOnCancel(sm.Hide)
	sm.fm.SetHint("↑↓ sidebar · ⏎ fields · Tab next · Esc back · ⌃⏎ save")

	return sm
}

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
	sm.lockEnvOverriddenFields()
	sm.setLogLevelSelection(settings.LogLevel)
	sm.setThemeSelection(settings.Theme)
	sm.setDensitySelection(settings.Density)
	sm.roundedBordersField.SetCurrentOption(booleanOptionIndex(settings.RoundedBorders))
	sm.setImagesSelection(settings.Images)
	sm.sessionRestoreField.SetCurrentOption(booleanOptionIndex(settings.SessionRestore))
	sm.updateCheckField.SetCurrentOption(booleanOptionIndex(settings.UpdateCheck))
	sm.setAgentProviderSelection(selectedProvider)
	sm.setAgentSandboxSelection(settings.AgentSandbox)
	sm.setAgentModelOptionsForProvider(selectedProvider)
	sm.setAgentModelSelection(settings.AgentModel)
	sm.agentWorkspaceField.SetText(settings.AgentWorkspace)
	sm.defaultTeamField.SetText(settings.DefaultTeam)
	sm.defaultProjectField.SetText(settings.DefaultProject)

	sm.fm.Show("settings")
}

func (sm *SettingsModal) currentAgentModelValue() string {
	index, _ := sm.agentModelField.GetCurrentOption()
	if index >= 0 && index < len(sm.agentModelValues) {
		return sm.agentModelValues[index]
	}
	return ""
}

// The provider picker fires during construction, before agentModelField exists.
func (sm *SettingsModal) setAgentModelOptionsForProvider(provider string) {
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

func (sm *SettingsModal) Hide() {
	logger.Debug("tui.settings: hiding settings modal")
	sm.fm.Hide("settings")
}

func (sm *SettingsModal) Focus() { sm.fm.Focus() }

func (sm *SettingsModal) HandleKey(event *tcell.EventKey) *tcell.EventKey {
	return sm.fm.HandleKey(event)
}

func (sm *SettingsModal) saveSettings() {
	settings, err := sm.settingsFromForm()
	if err != nil {
		logger.ErrorWithErr(err, "tui.settings: failed to build settings from form")
		sm.app.updateStatusBarWithError(err)
		return
	}

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

	_, images := sm.imagesField.GetCurrentOption()
	if images == "" {
		images = config.DefaultImages
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
		APIEndpoint:      strings.TrimSpace(sm.endpointField.GetText()),
		Timeout:          strings.TrimSpace(sm.timeoutField.GetText()),
		PageSize:         pageSize,
		CacheTTL:         strings.TrimSpace(sm.cacheTTLField.GetText()),
		SearchDebounce:   strings.TrimSpace(sm.searchDebounceField.GetText()),
		LogFile:          config.LogFileSetting(strings.TrimSpace(sm.logFileField.GetText())),
		LogLevel:         logLevel,
		Theme:            theme,
		Density:          density,
		GroupBy:          sm.app.config.GroupBy,
		SubgroupBy:       sm.app.config.SubgroupBy,
		SortBy:           sm.app.config.SortBy,
		Columns:          sm.app.config.Columns,
		RoundedBorders:   booleanOptionValue(sm.roundedBordersField),
		Images:           images,
		SessionRestore:   booleanOptionValue(sm.sessionRestoreField),
		UpdateCheck:      booleanOptionValue(sm.updateCheckField),
		AgentProvider:    agentProvider,
		AgentSandbox:     agentSandbox,
		AgentModel:       agentModel,
		AgentWorkspace:   strings.TrimSpace(sm.agentWorkspaceField.GetText()),
		Keybindings:      sm.app.config.Keybindings,
		DefaultTeam:      strings.TrimSpace(sm.defaultTeamField.GetText()),
		DefaultProject:   strings.TrimSpace(sm.defaultProjectField.GetText()),
		Workspaces:       sm.app.config.Workspaces,
		DefaultWorkspace: sm.app.config.DefaultWorkspace,
	}

	restoreEnvOverrides(&settings, sm.app.fileSettings, sm.app.envOverrides)

	return settings, nil
}

func (sm *SettingsModal) lockEnvOverriddenFields() {
	overrides := sm.app.envOverrides
	for field, primitive := range map[string]tview.Primitive{
		config.FieldAPIEndpoint: sm.endpointField,
		config.FieldTimeout:     sm.timeoutField,
		config.FieldPageSize:    sm.pageSizeField,
		config.FieldCacheTTL:    sm.cacheTTLField,
		config.FieldLogFile:     sm.logFileField,
		config.FieldLogLevel:    sm.logLevelField.View(),
	} {
		sm.fm.SetLocked(primitive, overrides.Has(field))
	}
}

func envOverrideNotice(overrides config.EnvOverrides) string {
	if len(overrides) == 0 {
		return ""
	}
	fields := make([]string, 0, len(overrides))
	for field := range overrides {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	listed := fields
	suffix := ""
	if len(fields) > envNoticeMaxFields {
		listed = fields[:envNoticeMaxFields]
		suffix = fmt.Sprintf(", and %d more", len(fields)-envNoticeMaxFields)
	}
	return "From the environment: " + strings.Join(listed, ", ") + suffix
}

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

func (sm *SettingsModal) currentThemeValue() string {
	index, _ := sm.themeField.GetCurrentOption()
	if index >= 0 && index < len(sm.themeValues) {
		return sm.themeValues[index]
	}
	return ""
}

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

func (sm *SettingsModal) currentDensityValue() string {
	index, _ := sm.densityField.GetCurrentOption()
	if index >= 0 && index < len(sm.densityValues) {
		return sm.densityValues[index]
	}
	return ""
}

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

func (sm *SettingsModal) setImagesSelection(images string) {
	selected := 0
	for i, option := range sm.imagesOptions {
		if option == config.DefaultImages {
			selected = i
		}
		if option == images {
			selected = i
			break
		}
	}
	sm.imagesField.SetCurrentOption(selected)
}

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

func booleanOptionIndex(enabled bool) int {
	if enabled {
		return 0
	}
	return 1
}

func booleanOptionValue(picker *FormPicker) bool {
	index, _ := picker.GetCurrentOption()
	return index == 0
}

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
