package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/rivo/tview"
)

// markdownRenderer is a shared glamour renderer for markdown content.
var markdownRenderer *glamour.TermRenderer

// initMarkdownRenderer initializes the glamour markdown renderer.
func initMarkdownRenderer() {
	var err error
	markdownRenderer, err = glamour.NewTermRenderer(
		glamour.WithStylePath("dark"),
		glamour.WithWordWrap(80),
	)
	if err != nil {
		// Fallback: create a basic renderer if custom style fails
		markdownRenderer, _ = glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(80),
		)
	}
}

// renderMarkdown renders markdown content using glamour.
// Falls back to plain text if rendering fails.
func renderMarkdown(content string) string {
	if markdownRenderer == nil {
		initMarkdownRenderer()
	}

	rendered, err := markdownRenderer.Render(content)
	if err != nil {
		// Fallback to plain text on error
		return content
	}

	// Trim extra whitespace that glamour may add
	return strings.TrimSpace(rendered)
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
		SetTitleColor(LinearTheme.Foreground).
		SetBorderColor(LinearTheme.Border).
		SetBackgroundColor(LinearTheme.Background)
	a.detailsDescriptionView.SetBorderPadding(1, 1, 2, 2)

	// Create comments view (bottom section, scrollable, fixed height)
	a.detailsCommentsView = tview.NewTextView()
	a.detailsCommentsView.SetDynamicColors(true).
		SetWrap(true).
		SetWordWrap(true).
		SetBorder(true).
		SetTitle(" Comments ").
		SetTitleColor(LinearTheme.Foreground).
		SetBorderColor(LinearTheme.Border).
		SetBackgroundColor(LinearTheme.Background)
	a.detailsCommentsView.SetBorderPadding(1, 1, 2, 2)

	// Create flex layout: description (flexible, ~60%) + comments (flexible, ~40%)
	// Both sections are scrollable independently
	detailsFlex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(a.detailsDescriptionView, 0, 3, true).  // 60% of space, gets focus by default
		AddItem(a.detailsCommentsView, 0, 2, false)     // 40% of space, always visible

	return detailsFlex
}

// updateDetailsView updates the details view with the selected issue.
func (a *App) updateDetailsView() {
	if a.selectedIssue == nil {
		a.detailsDescriptionView.SetText("[gray]No issue selected. Select an issue from the list to view details.[-]")
		a.detailsCommentsView.SetText("")
		return
	}

	issue := a.selectedIssue

	// Helper to colorize keys
	keyColor := "[#787878]"    // SecondaryText
	valColor := "[#EBEBF5]"    // Foreground
	accentColor := "[#5E6AD2]" // Accent

	// ===== Update Description/Metadata View =====
	var headerLines []string

	// Issue header info with styling
	headerLines = append(headerLines, fmt.Sprintf("%s%s[-]", accentColor, issue.Identifier))
	headerLines = append(headerLines, fmt.Sprintf("[b]%s%s[-]", valColor, issue.Title))
	headerLines = append(headerLines, "")

	// Metadata grid simulation
	headerLines = append(headerLines, fmt.Sprintf("%sState:[-]      %s%s[-]", keyColor, valColor, issue.State))

	assignee := "Unassigned"
	if issue.Assignee != "" {
		assignee = issue.Assignee
	}
	headerLines = append(headerLines, fmt.Sprintf("%sAssignee:[-]   %s%s[-]", keyColor, valColor, assignee))

	headerLines = append(headerLines, fmt.Sprintf("%sPriority:[-]   %s%d[-]", keyColor, valColor, issue.Priority))

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

	// Parent issue (if this is a sub-issue)
	if issue.Parent != nil {
		parentText := fmt.Sprintf("%s - %s", issue.Parent.Identifier, issue.Parent.Title)
		headerLines = append(headerLines, fmt.Sprintf("%sParent:[-]     %s%s[-]", keyColor, accentColor, parentText))
	}

	// Sub-issues (if this is a parent issue)
	if len(issue.Children) > 0 {
		headerLines = append(headerLines, "")
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

	headerLines = append(headerLines, "")
	headerLines = append(headerLines, "[#3C3C3C]────────────────────────────────────────[-]")
	headerLines = append(headerLines, "")

	// Set header first, then append description via ANSIWriter
	a.detailsDescriptionView.Clear()
	a.detailsDescriptionView.SetText(strings.Join(headerLines, "\n"))
	writer := tview.ANSIWriter(a.detailsDescriptionView)

	// Description
	if issue.Description != "" {
		fmt.Fprintf(writer, "%sDescription:[-]\n\n", keyColor)

		// Render description as markdown and write through ANSIWriter
		// ANSIWriter translates ANSI escape codes to tview color tags
		renderedDesc := renderMarkdown(issue.Description)
		fmt.Fprint(writer, renderedDesc)
	} else {
		fmt.Fprintf(writer, "%sNo description available[-]", keyColor)
	}

	a.detailsDescriptionView.ScrollToBeginning()

	// ===== Update Comments View =====
	a.detailsCommentsView.Clear()
	commentsWriter := tview.ANSIWriter(a.detailsCommentsView)

	if len(issue.Comments) > 0 {
		fmt.Fprintf(commentsWriter, "%sComments:[-] (%d)\n\n", keyColor, len(issue.Comments))

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

			fmt.Fprintf(commentsWriter, "%s%s[-] %s%s[-]\n", accentColor, authorDisplay, keyColor, timeStr)
			fmt.Fprint(commentsWriter, "\n")

			// Render comment body as markdown
			renderedComment := renderMarkdown(comment.Body)
			fmt.Fprint(commentsWriter, renderedComment)

			// Add separator between comments (but not after the last one)
			if i < len(issue.Comments)-1 {
				fmt.Fprint(commentsWriter, "\n\n")
				fmt.Fprint(commentsWriter, "[#3C3C3C]────────────────────────────────────────[-]\n\n")
			}
		}
	} else {
		// Empty state for comments
		fmt.Fprintf(commentsWriter, "%sNo comments yet.[-]", keyColor)
	}

	a.detailsCommentsView.ScrollToBeginning()
}
