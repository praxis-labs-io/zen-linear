package tui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// Renderers are cached per wrap width and rebuilt when the theme changes.
// A single slot is not enough: the agent output modal renders unwrapped from
// its own goroutine while the details pane renders at its measure on the event
// loop, so one slot both races and hands each caller the other's width. The
// mutex covers rendering too, since a TermRenderer is not safe to share.
// markdownWriter is one width's renderer and the lock serializing its use.
// The lock is per width, not global: the agent output modal renders unwrapped
// on its own goroutine so a long answer does not block the UI, and a global
// one would let that render hold the event loop inside Draw.
type markdownWriter struct {
	mu       sync.Mutex
	renderer *glamour.TermRenderer
}

var (
	markdownMu        sync.Mutex
	markdownRenderers = map[int]*markdownWriter{}
	markdownTheme     = LinearTheme
)

// initMarkdownRenderer records the theme markdown is rendered in and drops the
// renderers built for the previous one.
func initMarkdownRenderer(theme Theme) {
	markdownMu.Lock()
	defer markdownMu.Unlock()
	markdownTheme = theme
	clear(markdownRenderers)
}

// markdownRendererFor returns the writer for a wrap width, building one the
// first time that width is asked for. Width 0 leaves glamour's wrapping off,
// for callers whose view does its own. The map lock is held only long enough
// to find or build the entry, never across a render.
func markdownRendererFor(width int) *markdownWriter {
	markdownMu.Lock()
	defer markdownMu.Unlock()

	if writer, ok := markdownRenderers[width]; ok {
		return writer
	}
	// Linear treats a single newline as a hard break; CommonMark folds one into
	// the paragraph, which runs a line-per-thought comment together.
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(themeMarkdownStyle(markdownTheme)),
		glamour.WithWordWrap(width),
		glamour.WithPreservedNewLines(),
	)
	if err != nil {
		renderer, err = glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(width),
			glamour.WithPreservedNewLines(),
		)
		if err != nil {
			return nil
		}
	}
	writer := &markdownWriter{renderer: renderer}
	markdownRenderers[width] = writer
	return writer
}

// renderMarkdownAt renders markdown wrapped to width. Tables are why the width
// matters: glamour sizes their columns to it, and given nothing it lays a table
// out at its natural width, which the text view then re-wraps into a heap.
// Falls back to plain text if rendering fails.
func renderMarkdownAt(content string, width int) string {
	writer := markdownRendererFor(max(0, width))
	if writer == nil {
		return content
	}
	writer.mu.Lock()
	rendered, err := writer.renderer.Render(content)
	writer.mu.Unlock()
	if err != nil {
		return content
	}
	// Glamour pads every line out to the wrap width, which would push the
	// text view's own wrapping over by the padding on a full-width line.
	lines := strings.Split(strings.TrimSpace(rendered), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	return strings.Join(lines, "\n")
}

// renderMarkdown renders markdown without wrapping, for views that wrap it
// themselves.
func renderMarkdown(content string) string {
	return renderMarkdownAt(content, 0)
}

func formatIssueReference(ref linearapi.IssueRef) string {
	if ref.Identifier == "" {
		return ref.ID
	}
	if ref.Title == "" {
		return ref.Identifier
	}
	return fmt.Sprintf("%s - %s", ref.Identifier, ref.Title)
}

func formatUserDisplayName(user linearapi.User) string {
	if user.DisplayName != "" {
		return user.DisplayName
	}
	if user.Name != "" {
		return user.Name
	}
	return user.ID
}

// detailsMeasure caps the details pane's text at a readable line length. Prose
// set to the full width of a zoomed pane on a wide terminal is hard to track
// from one line to the next.
const detailsMeasure = 90

// detailsDrawFunc returns the draw func both details tabs use: it hands the
// tab its capped, centered content rect and tells it the width to lay out at.
// The draw func is the only place the live width is known, and a pane narrower
// than its own border and padding would otherwise hand tview a negative
// content rect, which it draws from without checking.
func (a *App) detailsDrawFunc(refit func(int)) func(tcell.Screen, int, int, int, int) (int, int, int, int) {
	return func(_ tcell.Screen, x, y, width, height int) (int, int, int, int) {
		inner := a.density.DetailsPadding
		innerWidth := max(0, width-2-inner.Left-inner.Right)
		innerHeight := max(0, height-2-inner.Top-inner.Bottom)
		measure, gutter := readingMeasure(innerWidth)
		refit(measure)
		return x + 1 + inner.Left + gutter, y + 1 + inner.Top, measure, innerHeight
	}
}

// detailsDivider draws the rule between sections at the width the text is set
// at, falling back to a short one before the first draw fixes that width.
func detailsDivider(width int) string {
	if width <= 0 {
		width = 40
	}
	return strings.Repeat("─", width)
}

// readingMeasure caps a pane's content width and centers what is left over,
// returning the width to set text at and the gutter to start it from.
func readingMeasure(innerWidth int) (measure int, gutter int) {
	measure = min(innerWidth, detailsMeasure)
	return measure, (innerWidth - measure) / 2
}

// buildDetailsView creates and configures the details view with separate description and comments sections.
func (a *App) buildDetailsView() *tview.Flex {
	// Create description/metadata view (top section, scrollable)
	a.detailsDescriptionView = tview.NewTextView()
	a.detailsDescriptionView.SetDynamicColors(true).
		SetWrap(true).
		SetWordWrap(true).
		SetBorder(true).
		SetTitle(" Details ").
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(a.theme.Foreground).
		SetBorderColor(a.theme.Border)
	// Not chained: SetBorder returns the Box, whose SetBackgroundColor leaves
	// the text style behind. tview fills the inner rect whenever the two
	// disagree, which the centered measure shows as a block of the wrong color.
	// The theme owns the value, the same as every other pane; a transparent
	// theme is transparent because its Background says so.
	a.detailsDescriptionView.SetBackgroundColor(a.theme.Background)
	padding := a.density.DetailsPadding
	a.detailsDescriptionView.SetBorderPadding(padding.Top, padding.Bottom, padding.Left, padding.Right)
	a.detailsDescriptionView.SetDrawFunc(a.detailsDrawFunc(a.refitDetailsHeader))

	// Create the comments page's text. The page around it owns the measure and
	// the refit, and the panel around that owns the border, the tab title and
	// the padding, so this one goes bare and unfitted.
	a.detailsCommentsView = tview.NewTextView()
	a.detailsCommentsView.SetDynamicColors(true).
		SetWrap(true).
		SetWordWrap(true)
	a.detailsCommentsView.SetBackgroundColor(a.theme.Background)
	a.buildDetailsCommentsPanel()

	// Create flex layout; comments are added conditionally after issue selection.
	detailsFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	a.detailsView = detailsFlex
	a.setDetailsCommentsVisibility(false)

	return a.detailsView
}

// viewHeight returns the rows a text view has to draw into. A box smaller than
// its own border and padding reports a negative rect, which reads as none.
func viewHeight(view *tview.TextView) int {
	if _, _, _, height := view.GetInnerRect(); height > 0 {
		return height
	}
	return 0
}

// visibleDetailsView returns the details tab currently on screen.
func (a *App) visibleDetailsView() *tview.TextView {
	if a.detailsCommentsVisible && a.focusedDetailsView {
		return a.detailsCommentsView
	}
	return a.detailsDescriptionView
}

// scrollDetailsHalfPage moves the details tab half a screen down (+1) or up
// (-1). tview's TextView stops at whole pages and keeps its page size private,
// so the height comes off the inner rect the draw func set.
func (a *App) scrollDetailsHalfPage(direction int) {
	view := a.visibleDetailsView()
	if view == nil {
		return
	}
	step := max(1, viewHeight(view)/2)
	row, column := view.GetScrollOffset()
	view.ScrollTo(max(0, row+direction*step), column)
}

// setDetailsCommentsVisibility records whether the Comments tab exists and
// re-renders the tabbed details layout.
func (a *App) setDetailsCommentsVisibility(showComments bool) {
	if a.detailsView == nil || a.detailsDescriptionView == nil || a.detailsCommentsPanel == nil {
		return
	}
	a.detailsCommentsVisible = showComments
	if !showComments {
		a.focusedDetailsView = false
		a.commentsFocus = commentsFocusCards
	}
	a.updateDetailsLayout()
}

// truncateTagged shortens a color-tagged line to width cells and marks the cut
// with an ellipsis. tview's wrapper does the measuring, so the cut never lands
// inside a tag, and the reset keeps a clipped color off the following line.
func truncateTagged(line string, width int) string {
	if width <= 0 || tview.TaggedStringWidth(line) <= width {
		return line
	}
	wrapped := tview.WordWrap(line, width-1)
	if len(wrapped) == 0 {
		return line
	}
	return wrapped[0] + "…[-]"
}

// renderDetailsDescription writes the metadata header and the description at
// the width of the pane's last draw. The header lines are single rows of
// fielded text, so they truncate; the description below is prose and keeps
// wrapping. Before the first draw the width is unknown and nothing is cut, so
// a render that beats the layout does not shorten the header to a stale box.
func (a *App) renderDetailsDescription() {
	if a.detailsDescriptionView == nil {
		return
	}

	fitted := make([]string, 0, len(a.detailsHeaderLines)+3)
	for _, line := range a.detailsHeaderLines {
		fitted = append(fitted, truncateTagged(line, a.detailsFittedWidth))
	}
	if len(fitted) > 0 {
		gap := a.density.DetailsSectionGap
		for i := 0; i < gap; i++ {
			fitted = append(fitted, "")
		}
		fitted = append(fitted, fmt.Sprintf("%s%s[-]", a.themeTags.Border, detailsDivider(a.detailsFittedWidth)))
		for i := 0; i < gap; i++ {
			fitted = append(fitted, "")
		}
	}

	// The compact density ends the header on the divider with no trailing gap
	// line, so the description needs its own break or it lands on the divider.
	text := strings.Join(fitted, "\n")
	if text != "" && a.detailsBody != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	a.detailsDescriptionView.SetText(text + a.detailsBody)
}

// renderDetailsBody renders the description markdown at the fitted width. The
// raw markdown is kept rather than the issue so this can run from a draw func,
// where taking the issues lock would be a poor idea.
func (a *App) renderDetailsBody() {
	if a.detailsDescriptionMarkdown == "" {
		a.detailsBody = fmt.Sprintf("%sNo description available[-]", a.themeTags.SecondaryText)
		return
	}
	var body strings.Builder
	writer := tview.ANSIWriter(&body)
	_, _ = fmt.Fprintf(writer, "%sDescription:[-]\n\n", a.themeTags.SecondaryText)
	_, _ = fmt.Fprint(writer, renderMarkdownAt(a.detailsDescriptionMarkdown, a.detailsFittedWidth))
	a.detailsBody = body.String()
}

// refitDetailsHeader re-fits the description tab to a pane width, keeping the
// scroll position. It runs from the view's draw func, the only place the live
// width is known, so it skips the work whenever the width has not moved.
// The body is re-rendered too, not just re-truncated: glamour sizes tables to
// the width it was given, so a stale one draws them at the old measure.
func (a *App) refitDetailsHeader(width int) {
	if width == a.detailsFittedWidth {
		return
	}
	a.detailsFittedWidth = width
	if len(a.detailsHeaderLines) == 0 {
		return
	}
	row, column := a.detailsDescriptionView.GetScrollOffset()
	a.renderDetailsBody()
	a.renderDetailsDescription()
	a.detailsDescriptionView.ScrollTo(row, column)
}

// refitDetailsComments re-renders the comments tab at a pane width, for the
// same reason the description needs it.
func (a *App) refitDetailsComments(width int) {
	if width == a.detailsCommentsFittedWidth {
		return
	}
	a.detailsCommentsFittedWidth = width
	if len(a.detailsCommentsSource) == 0 {
		return
	}
	row, column := a.detailsCommentsView.GetScrollOffset()
	a.renderDetailsComments()
	a.detailsCommentsView.ScrollTo(row, column)
}

// updateDetailsView updates the details view with the selected issue.
func (a *App) updateDetailsView() {
	a.issuesMu.RLock()
	selectedIssue := a.selectedIssue
	a.issuesMu.RUnlock()
	// The Comments tab is always reachable for a selected issue; an issue
	// without comments shows the empty state.
	a.setDetailsCommentsVisibility(selectedIssue != nil)
	// A half-written comment belongs to the issue it was written for, not to
	// the box, which stays put while the selection moves.
	if selectedIssue == nil {
		a.syncComposeDraft("")
	} else {
		a.syncComposeDraft(selectedIssue.ID)
	}
	if selectedIssue == nil {
		a.detailsHeaderLines = nil
		a.detailsBody = ""
		a.detailsDescriptionMarkdown = ""
		a.detailsCommentsSource = nil
		a.detailsDescriptionView.SetText(a.emptyDetailsMessage())
		a.detailsCommentsView.SetText("")
		a.updateAllPaneTitles()
		if a.focusedPane == FocusDetails && !a.detailsCommentsVisible {
			a.updateFocus()
		}
		return
	}

	issue := selectedIssue

	// Helper to colorize keys
	keyColor := a.themeTags.SecondaryText
	valColor := a.themeTags.Foreground
	accentColor := a.themeTags.Accent
	sectionGap := a.density.DetailsSectionGap

	// ===== Update Description/Metadata View =====
	var headerLines []string

	// Issue header info with styling
	headerLines = append(headerLines, fmt.Sprintf("%s%s[-]", accentColor, issue.Identifier))
	headerLines = append(headerLines, fmt.Sprintf("[b]%s%s[-]", valColor, issue.Title))
	for i := 0; i < sectionGap; i++ {
		headerLines = append(headerLines, "")
	}

	// Metadata grid simulation
	headerLines = append(headerLines, fmt.Sprintf("%sState:[-]      %s%s[-]", keyColor, valColor, issue.State))

	assignee := "Unassigned"
	if issue.Assignee != "" {
		assignee = issue.Assignee
	}
	headerLines = append(headerLines, fmt.Sprintf("%sAssignee:[-]   %s%s[-]", keyColor, valColor, assignee))

	headerLines = append(headerLines, fmt.Sprintf("%sPriority:[-]   %s%d[-]", keyColor, valColor, issue.Priority))

	cycle := "No cycle"
	if issue.Cycle != nil {
		cycle = issue.Cycle.DisplayName()
	}
	headerLines = append(headerLines, fmt.Sprintf("%sCycle:[-]      %s%s[-]", keyColor, valColor, cycle))

	project := "No project"
	if issue.ProjectName != "" {
		project = issue.ProjectName
	}
	headerLines = append(headerLines, fmt.Sprintf("%sProject:[-]    %s%s[-]", keyColor, valColor, project))

	headerLines = append(headerLines, fmt.Sprintf("%sDue date:[-]   %s%s[-]", keyColor, valColor, formatDueDate(issue.DueDate)))
	headerLines = append(headerLines, fmt.Sprintf("%sEstimate:[-]   %s%s[-]", keyColor, valColor, formatEstimate(issue.Estimate)))
	headerLines = append(headerLines, fmt.Sprintf("%sMilestone:[-]  %s%s[-]", keyColor, valColor, formatMilestoneName(issue.ProjectMilestone)))

	// Labels
	labelsText := "No labels"
	if len(issue.Labels) > 0 {
		labelNames := make([]string, len(issue.Labels))
		for i, lbl := range issue.Labels {
			labelNames[i] = lbl.Name
		}
		labelsText = strings.Join(labelNames, ", ")
	}
	headerLines = append(headerLines, fmt.Sprintf("%sLabels:[-]     %s%s[-]", keyColor, valColor, labelsText))

	branchName := issue.BranchName
	if branchName == "" {
		branchName = "-"
	}
	headerLines = append(headerLines, fmt.Sprintf("%sBranch:[-]     %s%s[-]", keyColor, valColor, branchName))

	// Parent issue (if this is a sub-issue)
	if issue.Parent != nil {
		parentText := fmt.Sprintf("%s - %s", issue.Parent.Identifier, issue.Parent.Title)
		headerLines = append(headerLines, fmt.Sprintf("%sParent:[-]     %s%s[-]", keyColor, accentColor, parentText))
	}

	// Sub-issues (if this is a parent issue)
	if len(issue.Children) > 0 {
		for i := 0; i < sectionGap; i++ {
			headerLines = append(headerLines, "")
		}
		headerLines = append(headerLines, fmt.Sprintf("%sSub-issues:[-] %s%d items[-]", keyColor, valColor, len(issue.Children)))
		for _, child := range issue.Children {
			// Show child identifier, state, and title
			childLine := fmt.Sprintf("  %s└─[-] %s%s[-] %s[%s][-] %s%s[-]",
				keyColor,
				accentColor, child.Identifier,
				keyColor, child.State,
				valColor, child.Title)
			headerLines = append(headerLines, childLine)
		}
	}

	if len(issue.Subscribers) > 0 {
		for i := 0; i < sectionGap; i++ {
			headerLines = append(headerLines, "")
		}
		subscribers := make([]string, 0, len(issue.Subscribers))
		for _, subscriber := range issue.Subscribers {
			subscribers = append(subscribers, formatUserDisplayName(subscriber))
		}
		headerLines = append(headerLines, fmt.Sprintf("%sSubscribers:[-] %s%s[-]", keyColor, valColor, strings.Join(subscribers, ", ")))
	}

	if len(issue.Relations) > 0 {
		for i := 0; i < sectionGap; i++ {
			headerLines = append(headerLines, "")
		}
		headerLines = append(headerLines, fmt.Sprintf("%sRelations:[-] %s%d items[-]", keyColor, valColor, len(issue.Relations)))
		for _, relation := range issue.Relations {
			ref := relation.RelatedIssue
			if relation.Inverse {
				ref = relation.Issue
			}
			headerLines = append(headerLines, fmt.Sprintf("  %s%s[-] %s%s[-]", keyColor, relation.DisplayType(), accentColor, formatIssueReference(ref)))
		}
	}

	if len(issue.Attachments) > 0 {
		for i := 0; i < sectionGap; i++ {
			headerLines = append(headerLines, "")
		}
		headerLines = append(headerLines, fmt.Sprintf("%sAttachments:[-] %s%d items[-]", keyColor, valColor, len(issue.Attachments)))
		for _, attachment := range issue.Attachments {
			title := attachment.Title
			if title == "" {
				title = attachment.URL
			}
			source := attachment.SourceType
			if source != "" {
				source = " (" + source + ")"
			}
			headerLines = append(headerLines, fmt.Sprintf("  %s%s%s[-] %s%s[-]", accentColor, title, source, keyColor, attachment.URL))
		}
	}

	// The rule closing the header is drawn at render time, not baked in here:
	// it spans the measure, and a refit only re-truncates these lines.
	a.detailsHeaderLines = headerLines
	// The raw markdown is kept, not just its rendering, because the width it
	// is laid out at can change without the issue changing.
	a.detailsDescriptionMarkdown = issue.Description
	a.detailsCommentsSource = issue.Comments
	a.renderDetailsBody()
	a.renderDetailsDescription()
	a.detailsDescriptionView.ScrollToBeginning()

	a.renderDetailsComments()
	a.detailsCommentsView.ScrollToBeginning()
	// The ring keeps its card across the async fetch that fills the tab in, and
	// drops it on an issue whose comments it is not on: ids do not survive a
	// change of issue, and nothing is lit until Tab says so.
	if a.commentSpanIndex(a.focusedCommentID) < 0 {
		a.focusedCommentID = ""
	}
	// Comments arrive with the async full-issue fetch; refresh the tab strip
	// so its count tracks what just rendered.
	a.updateAllPaneTitles()
}
