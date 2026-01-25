package agents

import (
	"strings"
	"testing"
	"time"

	"github.com/roeyazroel/linear-tui/internal/linearapi"
)

// TestBuildIssueContext_RendersFields verifies basic rendering.
func TestBuildIssueContext_RendersFields(t *testing.T) {
	issue := linearapi.Issue{
		Title:       "Example Issue",
		Description: "Example description",
		Comments: []linearapi.Comment{
			{
				Body:      "First comment body",
				CreatedAt: time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC),
				Author: linearapi.User{
					DisplayName: "Jane Doe",
				},
			},
		},
	}

	output := BuildIssueContext(issue)

	if !strings.Contains(output, "Title: Example Issue") {
		t.Fatalf("missing title in output: %s", output)
	}
	if !strings.Contains(output, "Description:\nExample description") {
		t.Fatalf("missing description in output: %s", output)
	}
	if !strings.Contains(output, "Comments:") {
		t.Fatalf("missing comments section in output: %s", output)
	}
	if !strings.Contains(output, "Jane Doe") || !strings.Contains(output, "First comment body") {
		t.Fatalf("missing comment content in output: %s", output)
	}
}

// TestBuildIssueContext_EmptyFields verifies empty description/comments behavior.
func TestBuildIssueContext_EmptyFields(t *testing.T) {
	issue := linearapi.Issue{
		Title: "Empty Fields",
	}

	output := BuildIssueContext(issue)

	if !strings.Contains(output, "Description: (none)") {
		t.Fatalf("missing empty description marker: %s", output)
	}
	if !strings.Contains(output, "Comments: (none)") {
		t.Fatalf("missing empty comments marker: %s", output)
	}
}

// TestBuildIssueContext_NoTruncation verifies full context is preserved.
func TestBuildIssueContext_NoTruncation(t *testing.T) {
	longDesc := strings.Repeat("a", 1200)
	longComment := strings.Repeat("b", 900)

	comments := []linearapi.Comment{
		{Body: longComment},
		{Body: "second comment"},
	}

	issue := linearapi.Issue{
		Title:       "Full context",
		Description: longDesc,
		Comments:    comments,
	}

	output := BuildIssueContext(issue)

	if !strings.Contains(output, longDesc) {
		t.Fatalf("expected full description in output")
	}
	if !strings.Contains(output, longComment) {
		t.Fatalf("expected full comment in output")
	}
	if strings.Contains(output, "truncated") || strings.Contains(output, "omitted") {
		t.Fatalf("unexpected truncation markers in output: %s", output)
	}
}
