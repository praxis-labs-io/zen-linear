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

func run(args []string) int {
	if len(args) == 0 {
		return runTUI()
	}

	switch args[0] {
	case "--version", "-v":
		fmt.Println(VersionInfo())
		return 0
	case "help", "--help", "-h":
		printUsage(os.Stdout)
		return 0
	case "auth":
		return runAuth(args[1:])
	case "update":
		return runUpdate(os.Stdout, os.Stderr)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command %q\n\n", args[0])
		printUsage(os.Stderr)
		return 1
	}
}

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

	sessionPath, err := session.Path()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not resolve session path: %v\n", err)
	}
	sessionFile, err := session.Load(sessionPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: ignoring unreadable session file: %v\n", err)
	}

	navCachePath, err := cache.NavPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not resolve navigation cache path: %v\n", err)
	}
	var navCacheFile cache.NavFile
	if navCachePath != "" {
		navCacheFile, err = cache.LoadNav(navCachePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: ignoring unreadable navigation cache: %v\n", err)
		}
	}

	apiKey := os.Getenv(config.LinearAPIKeyEnv)
	if apiKey == "" {
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

	effective, envOverrides, err := config.ApplyEnvOverrides(settings)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading environment: %v\n", err)
		return 1
	}

	cfg, err := config.ConfigFromSettings(resolved.Token, effective)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		return 1
	}

	logLevel := parseLogLevel(cfg.LogLevel)
	opened, warning := logger.Start(cfg.LogFile, config.DefaultLogFile(), logLevel)
	if opened != "" {
		cfg.LogFile = opened
	}
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

	tui.DetectTerminalCapabilities()

	app := tui.NewApp(clientCfg, cfg, promptTemplates)
	app.WarnAtStartup(warning)
	app.UseSettingsFile(settingsPath)
	app.UseVersion(Version)
	app.UseFileSettings(settings, envOverrides)
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
