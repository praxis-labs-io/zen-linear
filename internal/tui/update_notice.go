package tui

import (
	"context"
	"fmt"

	"github.com/praxis-labs-io/zen-linear/internal/logger"
	"github.com/praxis-labs-io/zen-linear/internal/update"
)

type checkForUpdate func(ctx context.Context, current string) (update.Result, error)

func fetchLatestRelease(ctx context.Context, current string) (update.Result, error) {
	path, err := update.Path()
	if err != nil {
		return update.Result{}, fmt.Errorf("resolve the update cache path: %w", err)
	}
	return update.Check(ctx, update.Options{Current: current, CachePath: path})
}

func (a *App) updateChecker() checkForUpdate {
	if a.checkUpdateFunc != nil {
		return a.checkUpdateFunc
	}
	return fetchLatestRelease
}

func (a *App) startUpdateCheck() {
	if !a.config.UpdateCheck {
		return
	}
	current := a.version
	check := a.updateChecker()
	if current == "" {
		return
	}

	go func() {
		result, err := check(context.Background(), current)
		if err != nil {
			logger.Debug("tui.update: update check did not complete: %v", err)
			return
		}
		if !result.Available {
			return
		}
		logger.Info("tui.update: %s is available, running %s", result.Latest, current)
		a.QueueUpdateDraw(func() {
			a.pendingNotice = updateNoticeText(result.Latest)
			a.reportPendingNotice()
		})
	}()
}

func updateNoticeText(latest string) string {
	return fmt.Sprintf("%s is available. Run zen-linear update.", latest)
}
