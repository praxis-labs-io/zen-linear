package tui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/rivo/tview"
)

// Locked per width, not globally: the agent output modal renders on its own goroutine and must not hold Draw.
type markdownWriter struct {
	mu       sync.Mutex
	renderer *glamour.TermRenderer
}

var (
	markdownMu        sync.Mutex
	markdownRenderers = map[int]*markdownWriter{}
	markdownTheme     = LinearTheme
)

func initMarkdownRenderer(theme Theme) {
	markdownMu.Lock()
	defer markdownMu.Unlock()
	markdownTheme = theme
	clear(markdownRenderers)
}

func markdownRendererFor(width int) *markdownWriter {
	markdownMu.Lock()
	defer markdownMu.Unlock()

	if writer, ok := markdownRenderers[width]; ok {
		return writer
	}
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
	lines := strings.Split(strings.TrimSpace(rendered), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	return strings.Join(lines, "\n")
}

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

const detailsMeasure = 90

func (a *App) trailingPad() string {
	return strings.Repeat("\n", a.density.DetailsPadding.Bottom)
}

func detailsDivider(width int) string {
	if width <= 0 {
		width = 40
	}
	return strings.Repeat("─", width)
}

func readingMeasure(innerWidth int) (measure int, gutter int) {
	measure = min(innerWidth, detailsMeasure)
	return measure, (innerWidth - measure) / 2
}

func (a *App) buildDetailsView() *tview.Flex {
	a.detailsPageView = tview.NewTextView()
	a.detailsPageView.SetDynamicColors(true).
		SetWrap(false)
	a.detailsPageView.SetBackgroundColor(a.theme.Background)
	a.buildDetailsPage()

	a.detailsView = tview.NewFlex().SetDirection(tview.FlexRow)
	a.detailsView.Box = tview.NewBox().SetBackgroundColor(a.theme.Background)
	a.detailsView.
		SetBorder(true).
		SetTitle(" Details ").
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(a.theme.Foreground).
		SetBorderColor(a.theme.Border).
		SetBackgroundColor(a.theme.Background)
	padding := a.density.DetailsPadding
	a.detailsView.SetBorderPadding(0, 0, padding.Left, padding.Right)
	a.detailsView.AddItem(a.detailsPage, 0, 1, true)

	return a.detailsView
}

func viewHeight(view *tview.TextView) int {
	if _, _, _, height := view.GetInnerRect(); height > 0 {
		return height
	}
	return 0
}

// tview's TextView keeps its page size private, so the height comes off the inner rect the last draw set.
func (a *App) scrollDetailsHalfPage(direction int) {
	if a.detailsPageView == nil {
		return
	}
	step := max(1, viewHeight(a.detailsPageView)/2)
	row, column := a.detailsPageView.GetScrollOffset()
	a.detailsPageView.ScrollTo(max(0, row+direction*step), column)
}

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

func (a *App) detailsMeasureWidth() int {
	if a.detailsFittedWidth > 0 {
		return a.detailsFittedWidth
	}
	return detailsFallbackWidth
}

const detailsLabelGutter = 12

type detailsRow struct {
	text        string
	field       issueField
	label       string
	valueColumn int
}

type fieldSpan struct {
	field       issueField
	row         int
	valueColumn int
}

type detailsHeader struct {
	lines   []string
	fields  []fieldSpan
	chooser chooserSpan
	editor  editorSpan
	slots   []pageSlot
	images  []pageImage
}

func (a *App) detailsHeaderBlock(width int) detailsHeader {
	pad := a.density.DetailsPadding.Top
	lines := make([]string, pad, pad+len(a.detailsHeaderRows)+len(a.detailsBodyLines)+3)
	var spans []fieldSpan
	var pictures []pageImage
	indent := 0
	if a.detailsEdit.on {
		indent = detailsCursorGutter
	}
	chooser := noChooserSpan
	editor := noEditorSpan
	var slots []pageSlot
	for _, row := range a.detailsHeaderRows {
		if row.field != "" {
			spans = append(spans, fieldSpan{field: row.field, row: len(lines), valueColumn: row.valueColumn + indent})
		}
		text, editing := row.text, row.field != "" && row.field == a.detailsEdit.editing
		if editing {
			text = a.detailsLabelCell(row.label)
		}
		fieldRow := len(lines)
		lines = append(lines, truncateTagged(a.fieldCursorMarker(row)+text, a.detailsFittedWidth))
		if row.field == "" {
			continue
		}
		if editing {
			boxColumn, boxWidth := a.fieldEditorRect(row.valueColumn + indent)
			slots = append(slots, pageSlot{primitive: a.detailsFieldInput, row: fieldRow, height: 1, column: boxColumn, width: boxWidth})
			editor = editorSpan{start: fieldRow, end: fieldRow}
			if line, ok := a.fieldEditorError(boxColumn); ok {
				editor.end = len(lines)
				lines = append(lines, line)
			}
			continue
		}
		if row.field == a.detailsEdit.open {
			options, lit := a.fieldChooserLines(row.valueColumn + indent)
			if len(options) > 0 {
				chooser = chooserSpan{lit: len(lines) + lit, end: len(lines) + len(options) - 1}
			}
			lines = append(lines, options...)
		}
	}
	if len(lines) > 0 {
		lines = append(lines, a.detailsSeam(width)...)
	}
	spans = append(spans, fieldSpan{field: issueFieldDescription, row: len(lines), valueColumn: indent})
	labelRow := len(lines)
	lines = append(lines, a.descriptionLabelRow())
	if a.detailsEdit.editing == issueFieldDescription {
		column, inner := descriptionBoxRect(width)
		rows := writingBoxRows(a.detailsDescArea, inner)
		lines = append(lines, a.descriptionRail())
		slots = append(slots, pageSlot{
			primitive: a.detailsDescArea,
			row:       len(lines),
			height:    rows,
			column:    column,
			width:     inner,
		})
		editor = editorSpan{start: labelRow, end: len(lines) + rows - 1}
		for range rows {
			lines = append(lines, a.descriptionRail())
		}
	} else {
		pad := strings.Repeat(" ", indent)
		start := len(lines)
		for _, line := range a.detailsBodyLines {
			lines = append(lines, pad+line)
		}
		for _, image := range a.detailsBodyImages {
			image.row += start
			image.column += indent
			pictures = append(pictures, image)
		}
	}
	return detailsHeader{
		lines:   lines,
		fields:  spans,
		chooser: chooser,
		editor:  editor,
		slots:   slots,
		images:  pictures,
	}
}

func (a *App) detailsGridRow(field issueField, label, value string) detailsRow {
	return detailsRow{
		text:        a.detailsLabelCell(label) + value,
		field:       field,
		label:       label,
		valueColumn: detailsLabelGutter,
	}
}

func (a *App) detailsLabelCell(label string) string {
	if label == "" {
		return ""
	}
	pad := strings.Repeat(" ", max(1, detailsLabelGutter-len(label)-1))
	return a.themeTags.SecondaryText + label + ":[-]" + pad
}

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

// Reads the stored raw markdown rather than the issue so it can run from a draw without taking the issues lock.
func (a *App) renderDetailsBody(width int) {
	width = max(0, width-detailsCursorGutter)
	if a.detailsDescriptionMarkdown == "" {
		a.detailsBodyLines = []string{"", fmt.Sprintf("%sNo description available[-]", a.themeTags.SecondaryText)}
		a.detailsBodyImages = nil
		return
	}
	markdown, pictures := a.describedImages(a.detailsDescriptionMarkdown)

	lines := []string{""}
	for _, line := range commentBodyLines(markdown, width) {
		lines = append(lines, wrapTagged(line, width)...)
	}
	a.detailsBodyLines, a.detailsBodyImages = a.reserveImageRows(lines, pictures, width)
}

func (a *App) refitDetailsPage(width, height int) {
	if width == a.detailsFittedWidth && height == a.detailsFittedHeight {
		return
	}
	row, column := a.detailsPageView.GetScrollOffset()
	budget := 0
	if a.imagesEnabled() {
		budget = a.imageRowBudget()
	}
	a.detailsFittedHeight = height
	if width != a.detailsFittedWidth || (a.imagesEnabled() && a.imageRowBudget() != budget) {
		a.detailsFittedWidth = width
		a.renderDetailsBody(width)
	}
	a.renderDetailsPage()
	a.detailsPageView.ScrollTo(row, column)
}

func (a *App) updateDetailsView() {
	a.issuesMu.RLock()
	selectedIssue := a.selectedIssue
	a.issuesMu.RUnlock()
	issueID := ""
	if selectedIssue != nil {
		issueID = selectedIssue.ID
	}
	issueChanged := issueID != a.detailsIssueID
	a.detailsIssueID = issueID
	leftEdit := issueChanged && a.detailsEdit.on
	if leftEdit {
		a.releaseFieldEditor()
		a.detailsEdit = detailsEditState{}
	}
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

	keyColor := a.themeTags.SecondaryText
	valColor := a.themeTags.Foreground
	accentColor := a.themeTags.Accent
	sectionGap := a.density.DetailsSectionGap

	var headerRows []detailsRow

	headerRows = append(headerRows, detailsRow{text: fmt.Sprintf("%s%s[-]", accentColor, issue.Identifier)})
	headerRows = append(headerRows, detailsRow{
		text:  fmt.Sprintf("[b]%s%s[-]", valColor, issue.Title),
		field: issueFieldTitle,
	})
	for i := 0; i < sectionGap; i++ {
		headerRows = append(headerRows, detailsRow{})
	}

	stateIcon, stateColor := formatStateIcon(issue.State, a.theme)
	stateTag := colorTag(stateColor)
	headerRows = append(headerRows, a.detailsGridRow(issueFieldState, "Status",
		fmt.Sprintf("%s%s %s[-]", stateTag, stateIcon, issue.State)))

	assignee := "Unassigned"
	if issue.Assignee != "" {
		assignee = issue.Assignee
	}
	headerRows = append(headerRows, a.detailsGridRow(issueFieldAssignee, "Assignee",
		fmt.Sprintf("%s%s[-]", valColor, assignee)))

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

	team := a.teamName(issue.TeamID)
	if team == "" {
		team, _, _ = strings.Cut(issue.Identifier, "-")
	}
	headerRows = append(headerRows, a.detailsGridRow(issueFieldTeam, "Team",
		fmt.Sprintf("%s%s[-]", valColor, team)))

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
	headerRows = append(headerRows, a.detailsGridRow("", "Branch",
		fmt.Sprintf("%s%s[-]", valColor, branchName)))

	if issue.Parent != nil {
		parentText := fmt.Sprintf("%s - %s", issue.Parent.Identifier, issue.Parent.Title)
		headerRows = append(headerRows, a.detailsGridRow("", "Parent",
			fmt.Sprintf("%s%s[-]", accentColor, parentText)))
	}

	if len(issue.Children) > 0 {
		for i := 0; i < sectionGap; i++ {
			headerRows = append(headerRows, detailsRow{})
		}
		headerRows = append(headerRows, a.detailsGridRow("", "Sub-issues",
			fmt.Sprintf("%s%d items[-]", valColor, len(issue.Children))))
		for _, child := range issue.Children {
			childLine := fmt.Sprintf("  %s└─[-] %s%s[-] %s[%s][-] %s%s[-]",
				keyColor,
				accentColor, child.Identifier,
				keyColor, child.State,
				valColor, child.Title)
			headerRows = append(headerRows, detailsRow{text: childLine})
		}
	}

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

	a.detailsHeaderRows = headerRows
	a.detailsDescriptionMarkdown = issue.Description
	a.detailsCommentsSource = issue.Comments
	a.detailsActivitySource = issue.Activity
	a.dropEditForMissingComment()
	a.renderDetailsBody(a.detailsMeasureWidth())
	a.renderDetailsPage()
	a.resolveFieldCursor()
	if issueChanged {
		a.detailsPageView.ScrollToBeginning()
	}
	if leftEdit {
		a.updateStatusBar()
	}
	if a.commentSpanIndex(a.focusedCommentID) < 0 {
		a.focusedCommentID = ""
	}
}
