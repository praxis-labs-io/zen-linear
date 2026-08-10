package linearapi

import (
	"context"
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
