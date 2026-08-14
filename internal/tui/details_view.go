package tui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
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

// trailingPad is the bottom padding written as text: the blank lines that close
// a scrolling pane, seen only once the reader reaches the end of it.
func (a *App) trailingPad() string {
	return strings.Repeat("\n", a.density.DetailsPadding.Bottom)
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

// buildDetailsView creates the details pane: one bordered panel around one
// page, which holds the issue, its comments, and the boxes they are written in.
func (a *App) buildDetailsView() *tview.Flex {
	// The page around this text owns the measure and the refit, and the panel
	// around that owns the border, the title and the padding, so the view goes
	// bare and unfitted.
	a.detailsPageView = tview.NewTextView()
	// Wrapping off, because the page counts its own lines: a line the view
	// wrapped would be one page line drawn as two screen rows, and every slot
	// and span below it a row out. Everything written to it is fitted to the
	// measure first, so there is nothing left to wrap.
	a.detailsPageView.SetDynamicColors(true).
		SetWrap(false)
	a.detailsPageView.SetBackgroundColor(a.theme.Background)
	a.buildDetailsPage()

	a.detailsView = tview.NewFlex().SetDirection(tview.FlexRow)
	// Restores the fill a Flex does not paint for itself, or the layer beneath
	// bleeds through every cell the page leaves blank.
	a.detailsView.Box = tview.NewBox().SetBackgroundColor(a.theme.Background)
	a.detailsView.
		SetBorder(true).
		SetTitle(" Details ").
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(a.theme.Foreground).
		SetBorderColor(a.theme.Border).
		SetBackgroundColor(a.theme.Background)
	padding := a.density.DetailsPadding
	// No bottom padding: the page writes its own at the end of the text, so the
	// gap is the end of the issue rather than a row the pane never uses.
	a.detailsView.SetBorderPadding(padding.Top, 0, padding.Left, padding.Right)
	// The page is the panel's focus item. With none flagged, Flex.Focus falls
	// through to the panel's own Box, whose InputHandler is nil, and any focus
	// tview delegates on its own leaves the pane dead to the keyboard.
	a.detailsView.AddItem(a.detailsPage, 0, 1, true)

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

// scrollDetailsHalfPage moves the details page half a screen down (+1) or up
// (-1). tview's TextView stops at whole pages and keeps its page size private,
// so the height comes off the inner rect the last draw set.
func (a *App) scrollDetailsHalfPage(direction int) {
	if a.detailsPageView == nil {
		return
	}
	step := max(1, viewHeight(a.detailsPageView)/2)
	row, column := a.detailsPageView.GetScrollOffset()
	a.detailsPageView.ScrollTo(max(0, row+direction*step), column)
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

// detailsMeasureWidth is the width the page lays out at: what the last draw
// measured, or a stand-in before the first one, which that draw's refit
// re-renders at the real measure.
func (a *App) detailsMeasureWidth() int {
	if a.detailsFittedWidth > 0 {
		return a.detailsFittedWidth
	}
	return detailsFallbackWidth
}

// detailsHeaderBlock is the top of the page: the metadata, the rule under it,
// and the description. The header lines are single rows of fielded text, so
// they truncate; the description below is prose and wraps.
//
// It returns lines rather than text because the page counts rows, and every
// card below this block is placed by that count.
func (a *App) detailsHeaderBlock(width int) []string {
	lines := make([]string, 0, len(a.detailsHeaderLines)+len(a.detailsBodyLines)+3)
	for _, line := range a.detailsHeaderLines {
		// Cut at what the last draw measured, which is 0 and cuts nothing before
		// the first one: a render that beats the layout must not shorten the
		// header to a box that was never on screen.
		lines = append(lines, truncateTagged(line, a.detailsFittedWidth))
	}
	if len(lines) > 0 {
		lines = append(lines, a.detailsSeam(width)...)
	}
	return append(lines, a.detailsBodyLines...)
}

// detailsSeam is the rule the page changes section on, spaced by the density.
// It is drawn at render time rather than baked into the header, because it
// spans the measure and the measure moves.
func (a *App) detailsSeam(width int) []string {
	gap := a.density.DetailsSectionGap
	lines := make([]string, 0, gap*2+1)
	for i := 0; i < gap; i++ {
		lines = append(lines, "")
	}
	lines = append(lines, fmt.Sprintf("%s%s[-]", a.themeTags.Border, detailsDivider(width)))
	for i := 0; i < gap; i++ {
		lines = append(lines, "")
	}
	return lines
}

// renderDetailsBody renders the description markdown to rows of the measure.
// The raw markdown is kept rather than the issue so this can run from a draw,
// where taking the issues lock would be a poor idea.
//
// The rows are wrapped here rather than by the view, for the reason the page
// turns wrapping off: a line the view wrapped is one page line drawn as two
// screen rows, and every card and box below it lands a row out. Glamour wraps
// prose to the measure but cannot break a bare URL and does not wrap code
// blocks or tables, so its output goes through the same wrap a comment body
// does.
func (a *App) renderDetailsBody(width int) {
	if a.detailsDescriptionMarkdown == "" {
		a.detailsBodyLines = []string{fmt.Sprintf("%sNo description available[-]", a.themeTags.SecondaryText)}
		return
	}
	label := truncateTagged(fmt.Sprintf("%sDescription:[-]", a.themeTags.SecondaryText), width)
	lines := []string{label, ""}
	for _, line := range commentBodyLines(a.detailsDescriptionMarkdown, width) {
		lines = append(lines, wrapTagged(line, width)...)
	}
	a.detailsBodyLines = lines
}

// refitDetailsPage re-renders the page at a pane width, keeping the scroll
// position. It runs from the page's draw, the only place the live width is
// known, so it skips the work whenever the width has not moved.
//
// The description is re-rendered too, not just re-truncated: glamour sizes
// tables to the width it was given, so a stale one draws them at the old
// measure.
func (a *App) refitDetailsPage(width int) {
	if width == a.detailsFittedWidth {
		return
	}
	a.detailsFittedWidth = width
	row, column := a.detailsPageView.GetScrollOffset()
	a.renderDetailsBody(width)
	a.renderDetailsPage()
	a.detailsPageView.ScrollTo(row, column)
}

// updateDetailsView updates the details view with the selected issue.
func (a *App) updateDetailsView() {
	a.issuesMu.RLock()
	selectedIssue := a.selectedIssue
	a.issuesMu.RUnlock()
	// A half-written comment belongs to the issue it was written for, not to
	// the box, which stays put while the selection moves.
	if selectedIssue == nil {
		a.syncComposeDraft("")
	} else {
		a.syncComposeDraft(selectedIssue.ID)
	}
	if selectedIssue == nil {
		a.detailsHeaderLines = nil
		a.detailsBodyLines = nil
		a.detailsDescriptionMarkdown = ""
		a.detailsCommentsSource = nil
		a.detailsActivitySource = nil
		a.renderDetailsPage()
		if a.focusedPane == FocusDetails {
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

	// The metadata grid, ordered the way it is read: what the issue is and who
	// has it, then where it sits in the plan, then its dates.
	stateIcon, stateColor := formatStateIcon(issue.State, a.theme)
	stateTag := colorTag(stateColor)
	headerLines = append(headerLines, fmt.Sprintf("%sState:[-]      %s%s %s[-]", keyColor, stateTag, stateIcon, issue.State))

	assignee := "Unassigned"
	if issue.Assignee != "" {
		assignee = issue.Assignee
	}
	headerLines = append(headerLines, fmt.Sprintf("%sAssignee:[-]   %s%s[-]", keyColor, valColor, assignee))

	// The glyph is the list's, so a priority reads the same in both places; the
	// word is what the pane has room for and the column does not.
	priorityIcon, priorityColor := formatPriority(issue.Priority, a.theme)
	priorityTag := colorTag(priorityColor)
	headerLines = append(headerLines, fmt.Sprintf("%sPriority:[-]   %s%s %s[-]", keyColor, priorityTag, priorityIcon, priorityLabel(issue.Priority)))

	labelsText := "No labels"
	if len(issue.Labels) > 0 {
		labelNames := make([]string, len(issue.Labels))
		for i, lbl := range issue.Labels {
			labelNames[i] = lbl.Name
		}
		labelsText = strings.Join(labelNames, ", ")
	}
	headerLines = append(headerLines, fmt.Sprintf("%sLabels:[-]     %s%s[-]", keyColor, valColor, labelsText))

	project := "No project"
	if issue.ProjectName != "" {
		project = issue.ProjectName
	}
	headerLines = append(headerLines, fmt.Sprintf("%sProject:[-]    %s%s[-]", keyColor, valColor, project))

	headerLines = append(headerLines, fmt.Sprintf("%sMilestone:[-]  %s%s[-]", keyColor, valColor, formatMilestoneName(issue.ProjectMilestone)))

	cycle := "No cycle"
	if issue.Cycle != nil {
		cycle = issue.Cycle.DisplayName()
	}
	headerLines = append(headerLines, fmt.Sprintf("%sCycle:[-]      %s%s[-]", keyColor, valColor, cycle))

	headerLines = append(headerLines, fmt.Sprintf("%sDue date:[-]   %s%s[-]", keyColor, valColor, formatDueDate(issue.DueDate)))
	headerLines = append(headerLines, fmt.Sprintf("%sEstimate:[-]   %s%s[-]", keyColor, valColor, formatEstimate(issue.Estimate)))

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

	// The rules between sections are drawn at render time, not baked in here:
	// they span the measure, and the measure moves. Held nil, these lines are
	// also what says no issue is selected.
	a.detailsHeaderLines = headerLines
	// The raw markdown is kept, not just its rendering, because the width it
	// is laid out at can change without the issue changing.
	a.detailsDescriptionMarkdown = issue.Description
	a.detailsCommentsSource = issue.Comments
	a.detailsActivitySource = issue.Activity
	a.dropEditForMissingComment()
	a.renderDetailsBody(a.detailsMeasureWidth())
	a.renderDetailsPage()
	a.detailsPageView.ScrollToBeginning()
	// The ring keeps its card across the async fetch that fills the comments
	// in, and drops it on an issue whose comments it is not on: ids do not
	// survive a change of issue, and nothing is lit until a brace says so.
	if a.commentSpanIndex(a.focusedCommentID) < 0 {
		a.focusedCommentID = ""
	}
}
