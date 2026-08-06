package linearapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestListIssueLabelsDropsAnotherTeamsLabels covers a create that Linear
// rejects outright: the workspace label connection is not scoped to a team, so
// it hands back every team's labels, and one foreign id fails the whole
// mutation with "labelIds for incorrect team".
func TestListIssueLabelsDropsAnotherTeamsLabels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(body, &request)

		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(request.Query, "team(id:") {
			_, _ = w.Write([]byte(`{"data":{"team":{"labels":{"nodes":[
				{"id":"own","name":"Own","color":"#fff"}
			]}}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"issueLabels":{"nodes":[
			{"id":"workspace","name":"Workspace","color":"#fff","team":{"id":""}},
			{"id":"own","name":"Own","color":"#fff","team":{"id":"team-1"}},
			{"id":"foreign","name":"Foreign","color":"#fff","team":{"id":"team-2"}}
		]}}}`))
	}))
	defer server.Close()

	client := NewClient(ClientConfig{Token: "t", Endpoint: server.URL})
	labels, err := client.ListIssueLabels(context.Background(), "team-1")
	if err != nil {
		t.Fatalf("ListIssueLabels returned %v", err)
	}

	got := make([]string, len(labels))
	for i, label := range labels {
		got[i] = label.ID
	}
	if len(got) != 2 || got[0] != "own" || got[1] != "workspace" {
		t.Fatalf("labels = %v, want the team's own and the workspace one, sorted by name", got)
	}
}
