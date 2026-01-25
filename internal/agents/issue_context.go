package agents

import (
	"fmt"
	"strings"
	"time"

	"github.com/roeyazroel/linear-tui/internal/linearapi"
)

// BuildIssueContext renders title, description, and comments into plain text.
func BuildIssueContext(issue linearapi.Issue) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("Title: %s\n", issue.Title))

	if issue.Description == "" {
		builder.WriteString("Description: (none)\n")
	} else {
		builder.WriteString("Description:\n")
		builder.WriteString(issue.Description)
		builder.WriteString("\n")
	}

	if len(issue.Comments) == 0 {
		builder.WriteString("Comments: (none)\n")
		return strings.TrimSpace(builder.String())
	}

	builder.WriteString("Comments:\n")
	for i := 0; i < len(issue.Comments); i++ {
		comment := issue.Comments[i]
		author := formatAuthor(comment.Author)
		timestamp := formatTimestamp(comment.CreatedAt)
		body := comment.Body

		builder.WriteString(fmt.Sprintf("- %s at %s\n", author, timestamp))
		builder.WriteString(body)
		builder.WriteString("\n")

		if i < len(issue.Comments)-1 {
			builder.WriteString("\n")
		}
	}

	return strings.TrimSpace(builder.String())
}

// formatAuthor returns a consistent display name for a comment author.
func formatAuthor(author linearapi.User) string {
	if author.DisplayName != "" {
		return author.DisplayName
	}
	if author.Name != "" {
		return author.Name
	}
	return "Unknown"
}

// formatTimestamp returns an RFC3339 timestamp string or a placeholder.
func formatTimestamp(timestamp time.Time) string {
	if timestamp.IsZero() {
		return "unknown time"
	}
	return timestamp.Format(time.RFC3339)
}
