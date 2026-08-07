package tui

import (
	"fmt"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// defaultLoadingFrameInterval is how often the spinner advances. Fast enough to
// read as motion, slow enough that a whole slow launch costs a few dozen
// redraws.
const defaultLoadingFrameInterval = 100 * time.Millisecond

// loadingIndicator drives the spinner frames the waiting panes paint. tview has
// no frame loop of its own, so this owns a ticker that queues a redraw, and
// stops the goroutine rather than leaving it ticking over a pane that already
// has its answer.
type loadingIndicator struct {
	spinner *spinner

	mu     sync.Mutex
	ticker *time.Ticker
	done   chan struct{}
	frame  string
}

// newLoadingIndicator returns a stopped indicator already holding a frame, so a
// pane that mounts before the first tick still shows a glyph.
func newLoadingIndicator() *loadingIndicator {
	return &loadingIndicator{
		spinner: newSpinner(spinnerFramesDots),
		frame:   spinnerFramesDots[0],
	}
}

// start begins the frame loop, calling tick from its own goroutine. Starting an
// already running indicator does nothing, so the second of two overlapping
// fetches cannot double the frame rate.
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

// stop ends the frame loop and leaves the last frame in place.
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

// advance moves to the next frame.
func (l *loadingIndicator) advance() {
	frame := l.spinner.NextFrame()
	if frame == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.frame = frame
}

// Frame returns the glyph the panes should paint.
func (l *loadingIndicator) Frame() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.frame
}

// running reports whether the frame loop is live.
func (l *loadingIndicator) running() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.ticker != nil
}

// setNavLoading records whether the navigation fetch is out. UI thread only.
func (a *App) setNavLoading(loading bool) {
	a.navLoading = loading
	a.syncLoadingIndicator()
}

// setIssuesLoading records whether an issue fetch is out. UI thread only.
func (a *App) setIssuesLoading(loading bool) {
	a.isLoading = loading
	a.syncLoadingIndicator()
}

// finishIssuesLoad settles the pane when this refresh is the one that claimed
// it. A superseded refresh reaching a completion path must not clear the flag:
// the refresh that replaced it is still fetching, and the pane would drop to
// "No issues" with the spinner stopped while a page is on its way.
func (a *App) finishIssuesLoad(generation int64, err error) {
	if a.loadingGeneration != generation {
		return
	}
	a.issuesErr = err
	a.issuesSettled = true
	a.setIssuesLoading(false)
}

// syncLoadingIndicator runs the frame loop while something is in flight and
// stops it the moment nothing is, then repaints so the panes drop the spinner
// with it. UI thread only.
func (a *App) syncLoadingIndicator() {
	if a.loading == nil {
		a.loading = newLoadingIndicator()
	}
	if a.isLoading || a.navLoading {
		a.loading.start(a.loadingFrameInterval(), func() {
			a.QueueUpdateDraw(func() {
				a.loading.advance()
				a.paintLoadingSurfaces()
			})
		})
	} else {
		a.loading.stop()
	}
	// Paint on the way in as well as the way out: the panes carry the last
	// message until something rewrites them, and waiting for the first tick
	// leaves "No issues" up while a fetch is already out.
	a.paintLoadingSurfaces()
}

// loadingFrameInterval is how often the spinner advances.
func (a *App) loadingFrameInterval() time.Duration {
	if a.loadingFrameDelay > 0 {
		return a.loadingFrameDelay
	}
	return defaultLoadingFrameInterval
}

// paintLoadingSurfaces writes the current message into every pane that has
// nothing of its own to show. UI thread only.
func (a *App) paintLoadingSurfaces() {
	a.updateIssuesPlaceholder()
	if a.detailsDescriptionView != nil && a.GetSelectedIssue() == nil {
		a.detailsDescriptionView.SetText(a.emptyDetailsMessage())
	}
	if a.navLoadingNode != nil {
		a.navLoadingNode.SetText(a.navLoadingText())
	}
}

// navLoadingText is the tree's waiting node. It carries no color tags: the
// node's own color styles it, and tags would throw off the label padding
// padNavigationTree measures.
func (a *App) navLoadingText() string {
	return a.loadingFrame() + " Loading teams"
}

// spinnerLabel renders the glyph and what it is waiting on.
func (a *App) spinnerLabel(label string) string {
	return fmt.Sprintf("%s%s[-] %s%s[-]", a.themeTags.Accent, a.loadingFrame(), a.themeTags.SecondaryText, label)
}

// loadingFrame is the glyph to paint, whether or not the loop has started.
func (a *App) loadingFrame() string {
	if a.loading == nil {
		return spinnerFramesDots[0]
	}
	return a.loading.Frame()
}

// issuesPlaceholderMessage is what the issues pane says while it has no rows:
// what it is waiting on, why it failed, or that there is nothing to list. The
// line count rides along because the centering flex sizes the text row.
func (a *App) issuesPlaceholderMessage() (string, int) {
	switch {
	case a.isLoading || !a.issuesSettled:
		return a.spinnerLabel("Loading issues"), 1
	case a.issuesErr != nil:
		return fmt.Sprintf("%sCould not load issues[-]\n%s%v[-]", a.themeTags.Error, a.themeTags.SecondaryText, a.issuesErr), 2
	default:
		return fmt.Sprintf("%sNo issues[-]", a.themeTags.SecondaryText), 1
	}
}

// updateIssuesPlaceholder re-centers the message in the placeholder panel,
// mirroring updateSearchBody. UI thread only.
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

// emptyDetailsMessage is what the details pane says with no issue selected.
func (a *App) emptyDetailsMessage() string {
	if a.isLoading || !a.issuesSettled {
		return a.spinnerLabel("Loading issue")
	}
	return fmt.Sprintf("%sNo issue selected. Select an issue from the list to view details.[-]", a.themeTags.SecondaryText)
}

// buildIssuesPlaceholder builds the panel the All and My tabs mount when they
// have no rows. It carries its own border and title because it stands in for
// the table, which carries both. Rebuilt on a theme change, like the Search
// panel, since the colors are baked in here.
func (a *App) buildIssuesPlaceholder() {
	a.issuesPlaceholderText = tview.NewTextView()
	a.issuesPlaceholderText.
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetTextColor(a.theme.SecondaryText).
		SetBackgroundColor(a.theme.Background)

	a.issuesPlaceholder = tview.NewFlex().SetDirection(tview.FlexRow)
	// Flex sets dontClear and never paints its own background; restore the fill
	// so the layer beneath cannot bleed through.
	a.issuesPlaceholder.Box = tview.NewBox().SetBackgroundColor(a.theme.Background)
	a.issuesPlaceholder.
		SetBorder(true).
		SetTitleColor(a.theme.Foreground).
		SetBorderColor(a.theme.Border).
		SetBackgroundColor(a.theme.Background)

	a.updateIssuesPlaceholder()
}

// setIssuesPlaceholderBorder recolors the placeholder, which wears the pane's
// focus border whenever it is the thing mounted.
func (a *App) setIssuesPlaceholderBorder(color tcell.Color) {
	if a.issuesPlaceholder == nil {
		return
	}
	a.issuesPlaceholder.SetBorderColor(color)
}

// issuesPaneIsEmpty reports whether the tab on screen has nothing to render, so
// the placeholder takes the table's place. The Search tab keeps its own
// placeholder inside its panel.
func (a *App) issuesPaneIsEmpty() bool {
	if a.activeIssuesSection == IssuesSectionSearch {
		return false
	}
	return len(a.rowsForSection(a.activeIssuesSection)) == 0
}
