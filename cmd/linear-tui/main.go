package main

import (
	"fmt"
	"os"

	"github.com/roeyazroel/linear-tui/internal/config"
	"github.com/roeyazroel/linear-tui/internal/linearapi"
	"github.com/roeyazroel/linear-tui/internal/logger"
	"github.com/roeyazroel/linear-tui/internal/tui"
)

func main() {
	// Handle --version flag
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println(VersionInfo())
		os.Exit(0)
	}

	// Load configuration from environment
	cfg, err := config.LoadFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		fmt.Fprintf(os.Stderr, "Please set the %s environment variable.\n", config.LinearAPIKeyEnv)
		os.Exit(1)
	}

	// Initialize logger
	logLevel := parseLogLevel(cfg.LogLevel)
	if err := logger.Init(cfg.LogFile, logLevel); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing logger: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := logger.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing logger: %v\n", err)
		}
	}()

	logger.Info("Application starting")
	logger.Debug("Configuration: APIEndpoint=%s, PageSize=%d, CacheTTL=%s",
		cfg.APIEndpoint, cfg.PageSize, cfg.CacheTTL)

	// Create Linear API client with full configuration
	apiClient := linearapi.NewClient(linearapi.ClientConfig{
		Token:    cfg.LinearAPIKey,
		Endpoint: cfg.APIEndpoint,
		Timeout:  cfg.Timeout,
	})

	// Create and run tview application
	app := tui.NewApp(apiClient, cfg)

	if err := app.Run(); err != nil {
		logger.ErrorWithErr(err, "Application error")
		fmt.Fprintf(os.Stderr, "Error running application: %v\n", err)
		// Note: logger.Close() will be called by defer, but os.Exit prevents defer execution
		// So we explicitly close here before exiting
		if closeErr := logger.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Error closing logger: %v\n", closeErr)
		}
		os.Exit(1) //nolint:gocritic // defer cleanup handled explicitly above
	}

	logger.Info("Application shutdown")
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
