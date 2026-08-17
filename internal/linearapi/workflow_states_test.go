package linearapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// statesResponse serves one team's states beside whatever defaultIssueState
// the case is about.
func statesResponse(t *testing.T, body string) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return NewClient(ClientConfig{Token: "t", Endpoint: server.URL})
}

// The create form names this state in place of its "Team default" row, so
// marking the wrong one files every new issue where nobody chose.
func TestWorkflowStatesMarkTheTeamsDefault(t *testing.T) {
	client := statesResponse(t, `{"data":{"team":{
		"defaultIssueState":{"id":"state-backlog"},
		"states":{"nodes":[
			{"id":"state-todo","name":"Todo","type":"unstarted","position":1},
			{"id":"state-backlog","name":"Backlog","type":"backlog","position":0}
		]}
	}}}`)

	states, err := client.ListWorkflowStates(context.Background(), "team-1")
	if err != nil {
		t.Fatalf("ListWorkflowStates returned %v", err)
	}

	marked := make([]string, 0, len(states))
	for _, state := range states {
		if state.IsDefault {
			marked = append(marked, state.ID)
		}
	}
	if len(marked) != 1 || marked[0] != "state-backlog" {
		t.Fatalf("default states = %v, want just state-backlog", marked)
	}
}

// The field is nullable. Falling back to the first state would name one the
// team never picked.
func TestATeamWithNoDefaultStateMarksNone(t *testing.T) {
	client := statesResponse(t, `{"data":{"team":{
		"defaultIssueState":null,
		"states":{"nodes":[
			{"id":"state-todo","name":"Todo","type":"unstarted","position":1},
			{"id":"state-backlog","name":"Backlog","type":"backlog","position":0}
		]}
	}}}`)

	states, err := client.ListWorkflowStates(context.Background(), "team-1")
	if err != nil {
		t.Fatalf("ListWorkflowStates returned %v", err)
	}

	for _, state := range states {
		if state.IsDefault {
			t.Fatalf("state %s marked default, want none where the team set none", state.ID)
		}
	}
}
