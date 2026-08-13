package linearapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Linear returns the comments newest first, which is what keeps the query's
// cap of 100 on the most recent hundred. A thread reads the other way.
func TestIssueCommentsAreOldestFirst(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": {"issue": {
			"id": "issue-1",
			"identifier": "ABC-1",
			"title": "Ordering",
			"labels": {"nodes": []},
			"children": {"nodes": []},
			"relations": {"nodes": []},
			"attachments": {"nodes": []},
			"subscribers": {"nodes": []},
			"comments": {"nodes": [
				{"id": "third", "body": "third", "createdAt": "2026-08-10T19:05:21Z", "updatedAt": "2026-08-10T19:05:21Z", "user": {"id": "u1"}},
				{"id": "second", "body": "second", "createdAt": "2026-08-10T19:05:12Z", "updatedAt": "2026-08-10T19:05:12Z", "user": {"id": "u1"}},
				{"id": "first", "body": "first", "createdAt": "2026-08-10T19:05:04Z", "updatedAt": "2026-08-10T19:05:04Z", "user": {"id": "u1"}}
			]}
		}}}`))
	}))
	defer server.Close()

	client := NewClient(ClientConfig{Endpoint: server.URL})
	issue, err := client.FetchIssueByID(context.Background(), "issue-1")
	if err != nil {
		t.Fatalf("FetchIssueByID() error: %v", err)
	}

	want := []string{"first", "second", "third"}
	if len(issue.Comments) != len(want) {
		t.Fatalf("got %d comments, want %d", len(issue.Comments), len(want))
	}
	for i, id := range want {
		if issue.Comments[i].ID != id {
			t.Errorf("comment %d = %q, want %q", i, issue.Comments[i].ID, id)
		}
	}
}

// TestIssueCommentsCarryTheirThread covers the two fields the details page
// threads and links by. Linear returns a null parentId on a top-level comment,
// which has to read as no parent rather than as one.
func TestIssueCommentsCarryTheirThread(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": {"issue": {
			"id": "issue-1",
			"identifier": "ABC-1",
			"title": "Threads",
			"labels": {"nodes": []},
			"children": {"nodes": []},
			"relations": {"nodes": []},
			"attachments": {"nodes": []},
			"subscribers": {"nodes": []},
			"comments": {"nodes": [
				{"id": "reply", "body": "reply", "createdAt": "2026-08-10T19:05:12Z", "updatedAt": "2026-08-10T19:05:12Z", "parentId": "root", "url": "https://linear.app/c/reply", "user": {"id": "u1"}},
				{"id": "root", "body": "root", "createdAt": "2026-08-10T19:05:04Z", "updatedAt": "2026-08-10T19:05:04Z", "parentId": null, "url": "https://linear.app/c/root", "user": {"id": "u1"}}
			]}
		}}}`))
	}))
	defer server.Close()

	client := NewClient(ClientConfig{Endpoint: server.URL})
	issue, err := client.FetchIssueByID(context.Background(), "issue-1")
	if err != nil {
		t.Fatalf("FetchIssueByID() error: %v", err)
	}

	if len(issue.Comments) != 2 {
		t.Fatalf("got %d comments, want 2", len(issue.Comments))
	}
	root, reply := issue.Comments[0], issue.Comments[1]
	if root.ParentID != "" {
		t.Errorf("top-level comment has parent %q, want none", root.ParentID)
	}
	if reply.ParentID != "root" {
		t.Errorf("reply has parent %q, want %q", reply.ParentID, "root")
	}
	if reply.URL != "https://linear.app/c/reply" {
		t.Errorf("reply URL = %q, want the comment permalink", reply.URL)
	}
}

// TestCreateCommentSendsItsParent covers the input the mutation actually puts
// on the wire. The query golden pins the selection, not the variables, and a
// parentId left out of them posts every reply at top level.
func TestCreateCommentSendsItsParent(t *testing.T) {
	tests := []struct {
		name  string
		input CreateCommentInput
		want  any
	}{
		{
			name:  "a reply names its parent",
			input: CreateCommentInput{IssueID: "issue-1", Body: "hi", ParentID: "root"},
			want:  "root",
		},
		{
			// Linear rejects a null parentId, so an empty one is left out of
			// the input rather than sent as one.
			name:  "a top-level comment sends no parent",
			input: CreateCommentInput{IssueID: "issue-1", Body: "hi"},
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sent map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Variables struct {
						Input map[string]any `json:"input"`
					} `json:"variables"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decoding request: %v", err)
				}
				sent = body.Variables.Input
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"data": {"commentCreate": {"success": true, "comment": {"id": "new", "user": {"id": "u1"}}}}}`))
			}))
			defer server.Close()

			client := NewClient(ClientConfig{Endpoint: server.URL})
			if _, err := client.CreateComment(context.Background(), tt.input); err != nil {
				t.Fatalf("CreateComment() error: %v", err)
			}
			if got := sent["parentId"]; got != tt.want {
				t.Errorf("input parentId = %v, want %v", got, tt.want)
			}
		})
	}
}
