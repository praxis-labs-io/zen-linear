package tui

import (
	"fmt"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// A var so the test suite can park the frame loop; nothing in the app assigns it.
var defaultLoadingFrameInterval = 100 * time.Millisecond

type loadingIndicator struct {
	spinner *spinner

	mu     sync.Mutex
	ticker *time.Ticker
	done   chan struct{}
	frame  string
}

func newLoadingIndicator() *loadingIndicator {
	return &loadingIndicator{
		spinner: newSpinner(spinnerFramesDots),
		frame:   spinnerFramesDots[0],
	}
}

func (l *loadingIndicator) start(interval time.Duration, tick func()) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.ticker != nil {
		return
	}
	l.spinner.Start()
	l.ticker = time.NewTicker(interval)
	l.done = make(chan struct{})
	ticker, done := l.ticker, l.done
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				tick()
			}
		}
	}()
}

func (l *loadingIndicator) stop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.ticker == nil {
		return
	}
	l.ticker.Stop()
	close(l.done)
	l.ticker = nil
	l.done = nil
	l.spinner.Stop()
}

func (l *loadingIndicator) advance() {
	frame := l.spinner.NextFrame()
	if frame == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.frame = frame
}

func (l *loadingIndicator) Frame() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.frame
}

func (l *loadingIndicator) running() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.ticker != nil
}

func (a *App) setNavLoading(loading bool) {
	a.navLoading = loading
	a.syncLoadingIndicator()
}

func (a *App) setIssuesLoading(loading bool) {
	a.isLoading = loading
	if !loading {
		a.setLoadingMessage("")
	}
	a.syncLoadingIndicator()
}

func (a *App) setSearchLoading(loading bool) {
	a.searchLoading = loading
	a.syncLoadingIndicator()
}

// A superseded refresh must not clear the flag while the refresh that replaced it is still fetching.
func (a *App) finishIssuesLoad(generation int64, err error) {
	if a.loadingGeneration != generation {
		return
	}
	a.issuesErr = err
	a.issuesSettled = true
	a.setIssuesLoading(false)
}

func (a *App) syncLoadingIndicator() {
	if a.loading == nil {
		a.loading = newLoadingIndicator()
	}
	if a.isLoading || a.navLoading || a.searchLoading {
		a.loading.start(a.loadingFrameInterval(), func() {
			a.QueueUpdateDraw(func() {
				a.loading.advance()
				a.paintLoadingSurfaces()
			})
		})
	} else {
		a.loading.stop()
	}
	a.paintLoadingSurfaces()
}

func (a *App) loadingFrameInterval() time.Duration {
	if a.loadingFrameDelay > 0 {
		return a.loadingFrameDelay
	}
	return defaultLoadingFrameInterval
}

func (a *App) paintLoadingSurfaces() {
	a.updateIssuesPlaceholder()
	if a.detailsPageView != nil && a.GetSelectedIssue() == nil {
		a.detailsPageView.SetText(a.emptyDetailsMessage())
	}
	if a.navLoadingNode != nil {
		a.navLoadingNode.SetText(a.navLoadingText())
	}
}

// No color tags: they would throw off the label padding padNavigationTree measures.
func (a *App) navLoadingText() string {
	return a.loadingFrame() + " Loading teams"
}

func (a *App) spinnerLabel(label string) string {
	return fmt.Sprintf("%s%s[-] %s%s[-]", a.themeTags.Accent, a.loadingFrame(), a.themeTags.SecondaryText, label)
}

func (a *App) loadingFrame() string {
	if a.loading == nil {
		return spinnerFramesDots[0]
	}
	return a.loading.Frame()
}

func (a *App) issuesPlaceholderMessage() (string, int) {
	if a.activeIssuesSection == IssuesSectionSearch {
		switch {
		case a.searchErr != nil:
			return fmt.Sprintf("%sSearch failed[-]\n%s%v[-]", a.themeTags.Error, a.themeTags.SecondaryText, a.searchErr), 2
		case a.searchLoading:
			return a.spinnerLabel("Searching"), 1
		default:
			return fmt.Sprintf("%sNo results[-]", a.themeTags.SecondaryText), 1
		}
	}
	switch {
	case a.isLoading || !a.issuesSettled:
		return a.spinnerLabel("Loading issues"), 1
	case a.issuesErr != nil:
		return fmt.Sprintf("%sCould not load issues[-]\n%s%v[-]", a.themeTags.Error, a.themeTags.SecondaryText, a.issuesErr), 2
	default:
		return fmt.Sprintf("%sNo issues[-]", a.themeTags.SecondaryText), 1
	}
}

func (a *App) updateIssuesPlaceholder() {
	if a.issuesPlaceholder == nil || a.issuesPlaceholderText == nil {
		return
	}
	message, lines := a.issuesPlaceholderMessage()
	a.issuesPlaceholderText.SetText(message)
	a.issuesPlaceholder.Clear()
	a.issuesPlaceholder.
		AddItem(nil, 0, 1, false).
		AddItem(a.issuesPlaceholderText, lines, 0, false).
		AddItem(nil, 0, 1, false)
}

func (a *App) emptyDetailsMessage() string {
	if a.isLoading || !a.issuesSettled {
		return a.spinnerLabel("Loading issue")
	}
	return fmt.Sprintf("%sNo issue selected. Select an issue from the list to view details.[-]", a.themeTags.SecondaryText)
}

func (a *App) buildIssuesPlaceholder() {
	a.issuesPlaceholderText = tview.NewTextView()
	a.issuesPlaceholderText.
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetTextColor(a.theme.SecondaryText).
		SetBackgroundColor(a.theme.Background)

	a.issuesPlaceholder = tview.NewFlex().SetDirection(tview.FlexRow)
	a.issuesPlaceholder.Box = tview.NewBox().SetBackgroundColor(a.theme.Background)
	a.issuesPlaceholder.
		SetBorder(true).
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(a.theme.Foreground).
		SetBorderColor(a.theme.Border).
		SetBackgroundColor(a.theme.Background)
	a.attachIssuesContext(a.issuesPlaceholder.Box)

	a.updateIssuesPlaceholder()
}

func (a *App) setIssuesPlaceholderBorder(color tcell.Color) {
	if a.issuesPlaceholder == nil {
		return
	}
	a.issuesPlaceholder.SetBorderColor(color)
}

func (a *App) issuesPaneIsEmpty() bool {
	return len(a.rowsForSection(a.activeIssuesSection)) == 0
}
