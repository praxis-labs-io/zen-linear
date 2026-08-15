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

// detailsLabelGutter is the column a gridded metadata value starts at. The
// label and the padding after it fill everything to its left.
const detailsLabelGutter = 12

// detailsRow is one line of the metadata header: the text read mode prints and
// the field it edits, empty on a row that edits nothing.
type detailsRow struct {
	text  string
	field issueField
	// Recorded here rather than measured at render, which would mean stripping
	// the color tags first.
	valueColumn int
}

// fieldSpan is where an editable field landed on the page, in page rows. The
// field cursor moves and scrolls by these, as the comment ring does by spans.
type fieldSpan struct {
	field       issueField
	row         int
	valueColumn int
}

// detailsHeaderBlock is the metadata, the rule under it, and the description as
// page lines, plus the row each editable field landed on.
func (a *App) detailsHeaderBlock(width int) ([]string, []fieldSpan) {
	lines := make([]string, 0, len(a.detailsHeaderRows)+len(a.detailsBodyLines)+3)
	var spans []fieldSpan
	// Every row shifts by the cursor gutter together, so the grid reads the same
	// in both modes rather than jumping a column as the cursor passes.
	indent := 0
	if a.detailsEdit.on {
		indent = detailsCursorGutter
	}
	for _, row := range a.detailsHeaderRows {
		if row.field != "" {
			spans = append(spans, fieldSpan{field: row.field, row: len(lines), valueColumn: row.valueColumn + indent})
		}
		// Cut at what the last draw measured, 0 before the first: a render that
		// beats the layout must not shorten the header to a box never on screen.
		lines = append(lines, truncateTagged(a.fieldCursorMarker(row)+row.text, a.detailsFittedWidth))
	}
	if len(lines) > 0 {
		lines = append(lines, a.detailsSeam(width)...)
	}
	return append(lines, a.detailsBodyLines...), spans
}

// detailsGridRow is one row of the metadata grid: the label padded out to the
// gutter, then a value that carries its own color.
func (a *App) detailsGridRow(field issueField, label, value string) detailsRow {
	// A label too long for the gutter takes one space rather than a negative
	// repeat, which would panic inside a draw. The gutter test catches it.
	pad := strings.Repeat(" ", max(1, detailsLabelGutter-len(label)-1))
	return detailsRow{
		text:        a.themeTags.SecondaryText + label + ":[-]" + pad + value,
		field:       field,
		valueColumn: detailsLabelGutter,
	}
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
	issueID := ""
	if selectedIssue != nil {
		issueID = selectedIssue.ID
	}
	// Read before anything below follows the selection. A rebuild on the same
	// issue keeps the reader's place; only a different one may move it.
	issueChanged := issueID != a.detailsIssueID
	a.detailsIssueID = issueID
	if issueChanged {
		// The cursor belongs to the issue it was aimed at. Carried onto another
		// one it would point at a field of a different issue.
		a.leaveDetailsEdit()
	}
	// A half-written comment belongs to the issue it was written for, not to
	// the box, which stays put while the selection moves.
	a.syncComposeDraft(issueID)
	if selectedIssue == nil {
		a.detailsHeaderRows = nil
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
	var headerRows []detailsRow

	// Issue header info with styling
	headerRows = append(headerRows, detailsRow{text: fmt.Sprintf("%s%s[-]", accentColor, issue.Identifier)})
	headerRows = append(headerRows, detailsRow{
		text:  fmt.Sprintf("[b]%s%s[-]", valColor, issue.Title),
		field: issueFieldTitle,
	})
	for i := 0; i < sectionGap; i++ {
		headerRows = append(headerRows, detailsRow{})
	}

	// The metadata grid, ordered the way it is read: what the issue is and who
	// has it, then where it sits in the plan, then its dates.
	stateIcon, stateColor := formatStateIcon(issue.State, a.theme)
	stateTag := colorTag(stateColor)
	headerRows = append(headerRows, a.detailsGridRow(issueFieldState, "State",
		fmt.Sprintf("%s%s %s[-]", stateTag, stateIcon, issue.State)))

	assignee := "Unassigned"
	if issue.Assignee != "" {
		assignee = issue.Assignee
	}
	headerRows = append(headerRows, a.detailsGridRow(issueFieldAssignee, "Assignee",
		fmt.Sprintf("%s%s[-]", valColor, assignee)))

	// The glyph is the list's, so a priority reads the same in both places; the
	// word is what the pane has room for and the column does not.
	priorityIcon, priorityColor := formatPriority(issue.Priority, a.theme)
	priorityTag := colorTag(priorityColor)
	headerRows = append(headerRows, a.detailsGridRow(issueFieldPriority, "Priority",
		fmt.Sprintf("%s%s %s[-]", priorityTag, priorityIcon, priorityLabel(issue.Priority))))

	labelsText := "No labels"
	if len(issue.Labels) > 0 {
		labelNames := make([]string, len(issue.Labels))
		for i, lbl := range issue.Labels {
			labelNames[i] = lbl.Name
		}
		labelsText = strings.Join(labelNames, ", ")
	}
	headerRows = append(headerRows, a.detailsGridRow(issueFieldLabels, "Labels",
		fmt.Sprintf("%s%s[-]", valColor, labelsText)))

	project := "No project"
	if issue.ProjectName != "" {
		project = issue.ProjectName
	}
	headerRows = append(headerRows, a.detailsGridRow(issueFieldProject, "Project",
		fmt.Sprintf("%s%s[-]", valColor, project)))

	headerRows = append(headerRows, a.detailsGridRow(issueFieldMilestone, "Milestone",
		fmt.Sprintf("%s%s[-]", valColor, formatMilestoneName(issue.ProjectMilestone))))

	cycle := "No cycle"
	if issue.Cycle != nil {
		cycle = issue.Cycle.DisplayName()
	}
	headerRows = append(headerRows, a.detailsGridRow(issueFieldCycle, "Cycle",
		fmt.Sprintf("%s%s[-]", valColor, cycle)))

	headerRows = append(headerRows, a.detailsGridRow(issueFieldDueDate, "Due date",
		fmt.Sprintf("%s%s[-]", valColor, formatDueDate(issue.DueDate))))
	headerRows = append(headerRows, a.detailsGridRow(issueFieldEstimate, "Estimate",
		fmt.Sprintf("%s%s[-]", valColor, formatEstimate(issue.Estimate))))

	branchName := issue.BranchName
	if branchName == "" {
		branchName = "-"
	}
	// Branch and Parent are on the grid and carry no field: one is Linear's to
	// write, the other has a picker of its own and its cycle checks.
	headerRows = append(headerRows, a.detailsGridRow("", "Branch",
		fmt.Sprintf("%s%s[-]", valColor, branchName)))

	if issue.Parent != nil {
		parentText := fmt.Sprintf("%s - %s", issue.Parent.Identifier, issue.Parent.Title)
		headerRows = append(headerRows, a.detailsGridRow("", "Parent",
			fmt.Sprintf("%s%s[-]", accentColor, parentText)))
	}

	// Sub-issues (if this is a parent issue)
	if len(issue.Children) > 0 {
		for i := 0; i < sectionGap; i++ {
			headerRows = append(headerRows, detailsRow{})
		}
		headerRows = append(headerRows, a.detailsGridRow("", "Sub-issues",
			fmt.Sprintf("%s%d items[-]", valColor, len(issue.Children))))
		for _, child := range issue.Children {
			// Show child identifier, state, and title
			childLine := fmt.Sprintf("  %s└─[-] %s%s[-] %s[%s][-] %s%s[-]",
				keyColor,
				accentColor, child.Identifier,
				keyColor, child.State,
				valColor, child.Title)
			headerRows = append(headerRows, detailsRow{text: childLine})
		}
	}

	// The three section labels below outgrow the grid, so they take a single
	// space and sit outside it.
	if len(issue.Subscribers) > 0 {
		for i := 0; i < sectionGap; i++ {
			headerRows = append(headerRows, detailsRow{})
		}
		subscribers := make([]string, 0, len(issue.Subscribers))
		for _, subscriber := range issue.Subscribers {
			subscribers = append(subscribers, formatUserDisplayName(subscriber))
		}
		headerRows = append(headerRows, detailsRow{
			text: fmt.Sprintf("%sSubscribers:[-] %s%s[-]", keyColor, valColor, strings.Join(subscribers, ", ")),
		})
	}

	if len(issue.Relations) > 0 {
		for i := 0; i < sectionGap; i++ {
			headerRows = append(headerRows, detailsRow{})
		}
		headerRows = append(headerRows, detailsRow{
			text: fmt.Sprintf("%sRelations:[-] %s%d items[-]", keyColor, valColor, len(issue.Relations)),
		})
		for _, relation := range issue.Relations {
			ref := relation.RelatedIssue
			if relation.Inverse {
				ref = relation.Issue
			}
			headerRows = append(headerRows, detailsRow{
				text: fmt.Sprintf("  %s%s[-] %s%s[-]", keyColor, relation.DisplayType(), accentColor, formatIssueReference(ref)),
			})
		}
	}

	if len(issue.Attachments) > 0 {
		for i := 0; i < sectionGap; i++ {
			headerRows = append(headerRows, detailsRow{})
		}
		headerRows = append(headerRows, detailsRow{
			text: fmt.Sprintf("%sAttachments:[-] %s%d items[-]", keyColor, valColor, len(issue.Attachments)),
		})
		for _, attachment := range issue.Attachments {
			title := attachment.Title
			if title == "" {
				title = attachment.URL
			}
			source := attachment.SourceType
			if source != "" {
				source = " (" + source + ")"
			}
			headerRows = append(headerRows, detailsRow{
				text: fmt.Sprintf("  %s%s%s[-] %s%s[-]", accentColor, title, source, keyColor, attachment.URL),
			})
		}
	}

	// The rules between sections are drawn at render time, since the measure
	// moves. Held nil, these rows are what says no issue is selected.
	a.detailsHeaderRows = headerRows
	// The raw markdown is kept, not just its rendering, because the width it
	// is laid out at can change without the issue changing.
	a.detailsDescriptionMarkdown = issue.Description
	a.detailsCommentsSource = issue.Comments
	a.detailsActivitySource = issue.Activity
	a.dropEditForMissingComment()
	a.renderDetailsBody(a.detailsMeasureWidth())
	a.renderDetailsPage()
	a.resolveFieldCursor()
	// Only a new issue starts at the top. Every save and every background
	// refresh comes through here, and a reset on those throws away the scroll.
	if issueChanged {
		a.detailsPageView.ScrollToBeginning()
	}
	// The ring keeps its card across the async fetch that fills the comments
	// in, and drops it on an issue whose comments it is not on: ids do not
	// survive a change of issue, and nothing is lit until a brace says so.
	if a.commentSpanIndex(a.focusedCommentID) < 0 {
		a.focusedCommentID = ""
	}
}
