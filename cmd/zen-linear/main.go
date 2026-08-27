package main

import (
	"context"
	"fmt"
	"os"

	"github.com/praxis-labs-io/zen-linear/internal/auth"
	"github.com/praxis-labs-io/zen-linear/internal/auth/oauth"
	"github.com/praxis-labs-io/zen-linear/internal/cache"
	"github.com/praxis-labs-io/zen-linear/internal/config"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
	"github.com/praxis-labs-io/zen-linear/internal/session"
	"github.com/praxis-labs-io/zen-linear/internal/tui"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run executes the CLI entrypoint and returns a process exit code.
func run(args []string) int {
	if len(args) > 0 && (args[0] == "--version" || args[0] == "-v") {
		fmt.Println(VersionInfo())
		return 0
	}

	if len(args) > 0 && args[0] == "auth" {
		return runAuth(args[1:])
	}

	return runTUI()
}

// runAuth handles `zen-linear auth ...` subcommands.
func runAuth(args []string) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		auth.PrintAuthUsage(os.Stdout)
		return 0
	}

	storePath, err := auth.CredentialsPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving credentials path: %v\n", err)
		return 1
	}

	clientID := auth.ClientID(oauth.DefaultClientID)
	oauthClient := oauth.NewClient(oauth.ClientConfig{ClientID: clientID})
	ctx := context.Background()

	switch args[0] {
	case "login":
		if clientID == "" {
			fmt.Fprintf(os.Stderr, "Error: OAuth client id is not configured. Set %s.\n", config.LinearClientIDEnv)
			return 1
		}
		fmt.Println("Opening browser for Linear authorization...")
		if err := auth.Login(ctx, auth.LoginOptions{
			ClientID:    clientID,
			StorePath:   storePath,
			OAuthClient: oauthClient,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "Login failed: %v\n", err)
			return 1
		}
		fmt.Println("Login successful. Credentials stored in", storePath)
		return 0
	case "logout":
		if err := auth.Logout(ctx, auth.LogoutOptions{
			StorePath:   storePath,
			OAuthClient: oauthClient,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "Logout failed: %v\n", err)
			return 1
		}
		fmt.Println("Logged out. Stored OAuth credentials removed.")
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown auth command %q\n\n", args[0])
		auth.PrintAuthUsage(os.Stderr)
		return 1
	}
}

// runTUI boots the interactive application with resolved credentials.
func runTUI() int {
	settingsPath, err := config.ConfigFilePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error determining settings path: %v\n", err)
		return 1
	}

	settings, err := config.EnsureSettingsFile(settingsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading settings file: %v\n", err)
		return 1
	}

	storePath, err := auth.CredentialsPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving credentials path: %v\n", err)
		return 1
	}

	clientID := auth.ClientID(oauth.DefaultClientID)
	oauthClient := oauth.NewClient(oauth.ClientConfig{ClientID: clientID})
	ctx := context.Background()

	// The logger is not up yet, so these failures go to stderr like the other
	// pre-logger errors here. None is fatal: without a session file the app
	// opens on its configured defaults.
	sessionPath, err := session.Path()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not resolve session path: %v\n", err)
	}
	sessionFile, err := session.Load(sessionPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: ignoring unreadable session file: %v\n", err)
	}

	// The cached navigation tree is what the sidebar paints before the first
	// fetch answers. Without it the app just waits, as it always did.
	navCachePath, err := cache.NavPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not resolve navigation cache path: %v\n", err)
	}
	var navCacheFile cache.NavFile
	if navCachePath != "" {
		// Skipped on an unresolved path, which LoadNav would only reject with a
		// second warning about the same failure.
		navCacheFile, err = cache.LoadNav(navCachePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: ignoring unreadable navigation cache: %v\n", err)
		}
	}

	apiKey := os.Getenv(config.LinearAPIKeyEnv)
	if apiKey == "" {
		// With no explicit key, reopen the last session's workspace, else the
		// configured default (or the first whose key env var is set); OAuth
		// credentials remain the fallback.
		names := config.StartupWorkspaceNames(settings, sessionFile.LastWorkspace)
		if workspace, ok := config.StartupWorkspace(settings.Workspaces, names...); ok {
			apiKey = workspace.APIKey()
		}
	}
	resolved, err := auth.Resolve(ctx, apiKey, storePath, oauthClient)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading authentication: %v\n", err)
		return 1
	}

	cfg, err := config.ConfigFromSettings(resolved.Token, settings)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		return 1
	}

	logLevel := parseLogLevel(cfg.LogLevel)
	opened, warning := logger.Start(cfg.LogFile, config.DefaultLogFile(), logLevel)
	// Only a path that actually opened is adopted, so the settings modal names
	// where logs really go and saving from it does not write the refused path
	// back. Logging off is what the app fell back to rather than what the user
	// asked for: adopting it would write "log_file": "" on the next save and
	// turn one launch's permission problem into a permanent setting.
	if opened != "" {
		cfg.LogFile = opened
	}
	// Also on stderr, which is all there is when the TUI never starts.
	if warning != "" {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", warning)
	}
	defer func() {
		if err := logger.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing logger: %v\n", err)
		}
	}()

	logger.Info("app.main: application starting")
	logger.Debug("app.main: configuration endpoint=%s page_size=%d cache_ttl=%s auth_source=%s",
		cfg.APIEndpoint, cfg.PageSize, cfg.CacheTTL, resolved.Source)

	clientCfg := linearapi.ClientConfig{
		Token:     cfg.LinearAPIKey,
		UseBearer: resolved.Source == auth.TokenSourceOAuth,
		Endpoint:  cfg.APIEndpoint,
		Timeout:   cfg.Timeout,
	}
	if resolved.Source == auth.TokenSourceOAuth {
		clientCfg.OnUnauthorized = auth.NewRefreshFunc(storePath, oauthClient)
	}

	promptTemplates := config.DefaultAgentPromptTemplates()
	promptsPath, err := config.PromptTemplatesFilePath()
	if err != nil {
		logger.Warning("app.main: failed to resolve prompts file path: %v", err)
	} else {
		templates, err := config.EnsurePromptTemplatesFile(promptsPath)
		if err != nil {
			logger.Warning("app.main: failed to load prompts file path=%s error=%v", promptsPath, err)
		} else {
			promptTemplates = templates
		}
	}

	// Before NewApp resolves the theme, and before tcell owns the tty.
	tui.DetectTerminalColors()

	app := tui.NewApp(clientCfg, cfg, promptTemplates)
	// tcell clears the screen the moment it takes the tty, so the stderr line
	// above is gone before it can be read. The app says it again once it is up.
	app.WarnAtStartup(warning)
	app.UseSettingsFile(settingsPath)
	app.UseSession(sessionPath, sessionFile)
	app.UseNavCache(navCachePath, navCacheFile)

	if err := app.Run(); err != nil {
		logger.ErrorWithErr(err, "app.main: application error")
		fmt.Fprintf(os.Stderr, "Error running application: %v\n", err)
		if closeErr := logger.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Error closing logger: %v\n", closeErr)
		}
		return 1
	}

	logger.Info("app.main: application shutdown")
	return 0
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
