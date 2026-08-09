package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/zen-linear/zen-linear/internal/linearapi"
)

// markdownRenderer is a shared glamour renderer for markdown content.
var markdownRenderer *glamour.TermRenderer

// initMarkdownRenderer initializes the glamour markdown renderer with colors
// derived from the theme. Word wrap stays disabled: glamour cannot know the
// pane width, and pre-wrapped output re-wraps badly inside the text views.
func initMarkdownRenderer(theme Theme) {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(themeMarkdownStyle(theme)),
		glamour.WithWordWrap(0),
	)
	if err != nil {
		// Fallback: create a basic renderer if the themed style fails
		markdownRenderer, _ = glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(0),
		)
		return
	}
	markdownRenderer = renderer
}

// renderMarkdown renders markdown content using glamour.
// Falls back to plain text if rendering fails.
func renderMarkdown(content string) string {
	if markdownRenderer == nil {
		initMarkdownRenderer(LinearTheme)
	}

	rendered, err := markdownRenderer.Render(content)
	if err != nil {
		// Fallback to plain text on error
		return content
	}

	// Trim extra whitespace that glamour may add
	return strings.TrimSpace(rendered)
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
		SetBorderColor(a.theme.Border).
		SetBackgroundColor(tcell.ColorDefault)
	padding := a.density.DetailsPadding
	a.detailsDescriptionView.SetBorderPadding(padding.Top, padding.Bottom, padding.Left, padding.Right)
	// The metadata header is fitted to the pane, so it has to re-fit when the
	// pane resizes. The draw func is the only place the live width is known.
	// A pane narrower than its own border and padding would otherwise hand
	// tview a negative content rect, which it draws from without checking.
	a.detailsDescriptionView.SetDrawFunc(func(_ tcell.Screen, x, y, width, height int) (int, int, int, int) {
		inner := a.density.DetailsPadding
		innerWidth := max(0, width-2-inner.Left-inner.Right)
		innerHeight := max(0, height-2-inner.Top-inner.Bottom)
		measure, gutter := readingMeasure(innerWidth)
		a.refitDetailsHeader(measure)
		return x + 1 + inner.Left + gutter, y + 1 + inner.Top, measure, innerHeight
	})

	// Create comments view (bottom section, scrollable, fixed height)
	a.detailsCommentsView = tview.NewTextView()
	a.detailsCommentsView.SetDynamicColors(true).
		SetWrap(true).
		SetWordWrap(true).
		SetBorder(true).
		SetTitle(" Comments ").
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(a.theme.Foreground).
		SetBorderColor(a.theme.Border).
		SetBackgroundColor(tcell.ColorDefault)
	a.detailsCommentsView.SetBorderPadding(padding.Top, padding.Bottom, padding.Left, padding.Right)
	a.detailsCommentsView.SetDrawFunc(func(_ tcell.Screen, x, y, width, height int) (int, int, int, int) {
		inner := a.density.DetailsPadding
		innerWidth := max(0, width-2-inner.Left-inner.Right)
		innerHeight := max(0, height-2-inner.Top-inner.Bottom)
		measure, gutter := readingMeasure(innerWidth)
		return x + 1 + inner.Left + gutter, y + 1 + inner.Top, measure, innerHeight
	})

	// Create flex layout; comments are added conditionally after issue selection.
	detailsFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	a.detailsView = detailsFlex
	a.setDetailsCommentsVisibility(false)

	return a.detailsView
}

// setDetailsCommentsVisibility records whether the Comments tab exists and
// re-renders the tabbed details layout.
func (a *App) setDetailsCommentsVisibility(showComments bool) {
	if a.detailsView == nil || a.detailsDescriptionView == nil || a.detailsCommentsView == nil {
		return
	}
	a.detailsCommentsVisible = showComments
	if !showComments {
		a.focusedDetailsView = false
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

	fitted := make([]string, len(a.detailsHeaderLines))
	for i, line := range a.detailsHeaderLines {
		fitted[i] = truncateTagged(line, a.detailsFittedWidth)
	}

	// The compact density ends the header on the divider with no trailing gap
	// line, so the description needs its own break or it lands on the divider.
	text := strings.Join(fitted, "\n")
	if text != "" && a.detailsBody != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	a.detailsDescriptionView.SetText(text + a.detailsBody)
}

// refitDetailsHeader re-truncates the header for a pane width, keeping the
// scroll position. It runs from the view's draw func, the only place the live
// width is known, so it skips the work whenever the width has not moved.
func (a *App) refitDetailsHeader(width int) {
	if width == a.detailsFittedWidth {
		return
	}
	a.detailsFittedWidth = width
	if len(a.detailsHeaderLines) == 0 {
		return
	}
	row, column := a.detailsDescriptionView.GetScrollOffset()
	a.renderDetailsDescription()
	a.detailsDescriptionView.ScrollTo(row, column)
}

// updateDetailsView updates the details view with the selected issue.
func (a *App) updateDetailsView() {
	a.issuesMu.RLock()
	selectedIssue := a.selectedIssue
	a.issuesMu.RUnlock()
	// The Comments tab is always reachable for a selected issue; an issue
	// without comments shows the empty state.
	a.setDetailsCommentsVisibility(selectedIssue != nil)
	if selectedIssue == nil {
		a.detailsHeaderLines = nil
		a.detailsBody = ""
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
	dividerColor := a.themeTags.Border
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

	for i := 0; i < sectionGap; i++ {
		headerLines = append(headerLines, "")
	}
	headerLines = append(headerLines, fmt.Sprintf("%s────────────────────────────────────────[-]", dividerColor))
	for i := 0; i < sectionGap; i++ {
		headerLines = append(headerLines, "")
	}

	// The description is rendered once into a buffer rather than straight into
	// the view, so refitting the header to a new pane width does not re-run
	// glamour. ANSIWriter translates glamour's escape codes to tview tags.
	var body strings.Builder
	writer := tview.ANSIWriter(&body)
	if issue.Description != "" {
		_, _ = fmt.Fprintf(writer, "%sDescription:[-]\n\n", keyColor)
		_, _ = fmt.Fprint(writer, renderMarkdown(issue.Description))
	} else {
		_, _ = fmt.Fprintf(writer, "%sNo description available[-]", keyColor)
	}

	a.detailsHeaderLines = headerLines
	a.detailsBody = body.String()
	a.renderDetailsDescription()
	a.detailsDescriptionView.ScrollToBeginning()

	// ===== Update Comments View =====
	a.detailsCommentsView.Clear()
	commentsWriter := tview.ANSIWriter(a.detailsCommentsView)

	if len(issue.Comments) > 0 {
		_, _ = fmt.Fprintf(commentsWriter, "%sComments:[-] (%d)\n\n", keyColor, len(issue.Comments))

		for i, comment := range issue.Comments {
			// Comment header: author and timestamp
			authorDisplay := comment.Author.DisplayName
			if authorDisplay == "" {
				authorDisplay = comment.Author.Name
			}
			if comment.Author.IsMe {
				authorDisplay = fmt.Sprintf("%s (me)", authorDisplay)
			}

			// Format timestamp
			timeStr := comment.CreatedAt.Format("Jan 2, 2006 3:04 PM")
			if !comment.UpdatedAt.Equal(comment.CreatedAt) {
				timeStr += " (edited)"
			}

			_, _ = fmt.Fprintf(commentsWriter, "%s%s[-] %s%s[-]\n", accentColor, authorDisplay, keyColor, timeStr)
			_, _ = fmt.Fprint(commentsWriter, "\n")

			// Render comment body as markdown
			renderedComment := renderMarkdown(comment.Body)
			_, _ = fmt.Fprint(commentsWriter, renderedComment)

			// Add separator between comments (but not after the last one)
			if i < len(issue.Comments)-1 {
				_, _ = fmt.Fprint(commentsWriter, "\n\n")
				_, _ = fmt.Fprintf(commentsWriter, "%s────────────────────────────────────────[-]\n\n", dividerColor)
			}
		}
	} else {
		// Empty state for comments
		_, _ = fmt.Fprintf(commentsWriter, "%sNo comments yet.[-]", keyColor)
	}

	a.detailsCommentsView.ScrollToBeginning()
	// Comments arrive with the async full-issue fetch; refresh the tab strip
	// so its count tracks what just rendered.
	a.updateAllPaneTitles()
}
