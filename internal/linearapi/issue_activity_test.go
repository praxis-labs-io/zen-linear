package linearapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fetchActivity serves one issue whose history is the given entries and returns
// the activity the detail fetch built from it. issueFields overrides the fields
// carried on the issue itself, which is where the creation event comes from.
func fetchActivity(t *testing.T, issueFields, historyNodes string) []IssueActivity {
	t.Helper()

	body := fmt.Sprintf(`{"data": {"issue": {
		"id": "issue-1",
		"identifier": "ABC-1",
		"title": "Activity",
		%s
		"labels": {"nodes": []},
		"children": {"nodes": []},
		"relations": {"nodes": []},
		"attachments": {"nodes": []},
		"subscribers": {"nodes": []},
		"comments": {"nodes": []},
		"history": {"nodes": [%s]}
	}}}`, issueFields, historyNodes)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	client := NewClient(ClientConfig{Endpoint: server.URL})
	issue, err := client.FetchIssueByID(context.Background(), "issue-1")
	if err != nil {
		t.Fatalf("FetchIssueByID() error: %v", err)
	}
	return issue.Activity
}

// entry wraps one history node's fields with the id and time every node has.
func entry(fields string) string {
	return `{"id": "h1", "createdAt": "2026-08-10T19:05:04Z", ` + fields + `}`
}

const actorDrew = `"actor": {"id": "u1", "name": "Drew White", "displayName": "drew"}`

func TestHistoryEntriesMapToActivityKinds(t *testing.T) {
	tests := []struct {
		name  string
		node  string
		want  IssueActivityKind
		check func(*testing.T, IssueActivity)
	}{
		{
			name: "state move",
			node: entry(actorDrew + `, "fromState": {"id": "s1", "name": "Backlog", "type": "backlog"}, "toState": {"id": "s2", "name": "To Do", "type": "unstarted"}`),
			want: IssueActivityStateChanged,
			check: func(t *testing.T, event IssueActivity) {
				if event.FromState == nil || event.FromState.Name != "Backlog" {
					t.Errorf("FromState = %+v, want Backlog", event.FromState)
				}
				if event.ToState == nil || event.ToState.Type != "unstarted" {
					t.Errorf("ToState = %+v, want type unstarted", event.ToState)
				}
			},
		},
		{
			// The first move out of the initial state records no from side.
			name: "state set with no previous state",
			node: entry(actorDrew + `, "toState": {"id": "s2", "name": "To Do", "type": "unstarted"}`),
			want: IssueActivityStateChanged,
			check: func(t *testing.T, event IssueActivity) {
				if event.FromState != nil {
					t.Errorf("FromState = %+v, want nil", event.FromState)
				}
			},
		},
		{
			name: "assigned to someone else",
			node: entry(actorDrew + `, "toAssignee": {"id": "u2", "name": "Mike", "displayName": "mike"}`),
			want: IssueActivityAssigned,
			check: func(t *testing.T, event IssueActivity) {
				if event.Assignee == nil || event.Assignee.DisplayName != "mike" {
					t.Errorf("Assignee = %+v, want mike", event.Assignee)
				}
			},
		},
		{
			name: "self assigned",
			node: entry(actorDrew + `, "toAssignee": {"id": "u1", "name": "Drew White", "displayName": "drew"}`),
			want: IssueActivitySelfAssigned,
		},
		{
			name: "unassigned carries who it came off",
			node: entry(actorDrew + `, "fromAssignee": {"id": "u2", "name": "Mike", "displayName": "mike"}, "toAssignee": null`),
			want: IssueActivityUnassigned,
			check: func(t *testing.T, event IssueActivity) {
				if event.Assignee == nil || event.Assignee.DisplayName != "mike" {
					t.Errorf("Assignee = %+v, want mike", event.Assignee)
				}
			},
		},
		{
			name: "cycle added",
			node: entry(actorDrew + `, "toCycle": {"id": "c1", "name": null, "number": 154}`),
			want: IssueActivityCycleAdded,
			check: func(t *testing.T, event IssueActivity) {
				if got := event.Cycle.DisplayName(); got != "Cycle 154" {
					t.Errorf("Cycle.DisplayName() = %q, want %q", got, "Cycle 154")
				}
			},
		},
		{
			name: "cycle removed",
			node: entry(actorDrew + `, "fromCycle": {"id": "c1", "name": null, "number": 154}, "toCycle": null`),
			want: IssueActivityCycleRemoved,
		},
		{
			name: "project set",
			node: entry(actorDrew + `, "toProject": {"id": "p1", "name": "Feature Backlog"}`),
			want: IssueActivityProjectSet,
			check: func(t *testing.T, event IssueActivity) {
				if event.Project == nil || event.Project.Name != "Feature Backlog" {
					t.Errorf("Project = %+v, want Feature Backlog", event.Project)
				}
			},
		},
		{
			name: "project removed",
			node: entry(actorDrew + `, "fromProject": {"id": "p1", "name": "Feature Backlog"}, "toProject": null`),
			want: IssueActivityProjectRemoved,
		},
		{
			name: "milestone set",
			node: entry(actorDrew + `, "toProjectMilestone": {"id": "m1", "name": "Beta"}`),
			want: IssueActivityMilestoneSet,
			check: func(t *testing.T, event IssueActivity) {
				if event.Milestone == nil || event.Milestone.Name != "Beta" {
					t.Errorf("Milestone = %+v, want Beta", event.Milestone)
				}
			},
		},
		{
			name: "milestone removed",
			node: entry(actorDrew + `, "fromProjectMilestone": {"id": "m1", "name": "Beta"}, "toProjectMilestone": null`),
			want: IssueActivityMilestoneRemoved,
		},
		{
			name: "parent set",
			node: entry(actorDrew + `, "toParent": {"id": "i9", "identifier": "ABC-9", "title": "Epic"}`),
			want: IssueActivityParentSet,
			check: func(t *testing.T, event IssueActivity) {
				if event.Parent == nil || event.Parent.Identifier != "ABC-9" {
					t.Errorf("Parent = %+v, want ABC-9", event.Parent)
				}
			},
		},
		{
			name: "parent removed",
			node: entry(actorDrew + `, "fromParent": {"id": "i9", "identifier": "ABC-9", "title": "Epic"}, "toParent": null`),
			want: IssueActivityParentRemoved,
		},
		{
			name: "labels added",
			node: entry(actorDrew + `, "addedLabels": [{"id": "l1", "name": "Bug", "color": "#f00"}]`),
			want: IssueActivityLabelsAdded,
			check: func(t *testing.T, event IssueActivity) {
				if len(event.Labels) != 1 || event.Labels[0].Name != "Bug" {
					t.Errorf("Labels = %+v, want one Bug", event.Labels)
				}
			},
		},
		{
			name: "labels removed",
			node: entry(actorDrew + `, "removedLabels": [{"id": "l1", "name": "Bug", "color": "#f00"}]`),
			want: IssueActivityLabelsRemoved,
		},
		{
			name: "priority changed",
			node: entry(actorDrew + `, "fromPriority": 2, "toPriority": 4`),
			want: IssueActivityPriorityChanged,
			check: func(t *testing.T, event IssueActivity) {
				if event.FromPriority != 2 || event.ToPriority != 4 {
					t.Errorf("priority %d -> %d, want 2 -> 4", event.FromPriority, event.ToPriority)
				}
			},
		},
		{
			// 0 is Linear's "No priority", a real target. A value type would read
			// this as no change and drop the event.
			name: "priority cleared to none",
			node: entry(actorDrew + `, "fromPriority": 2, "toPriority": 0`),
			want: IssueActivityPriorityChanged,
			check: func(t *testing.T, event IssueActivity) {
				if event.ToPriority != 0 {
					t.Errorf("ToPriority = %d, want 0", event.ToPriority)
				}
			},
		},
		{
			name: "title changed",
			node: entry(actorDrew + `, "toTitle": "A new title"`),
			want: IssueActivityTitleChanged,
		},
		{
			name: "description updated",
			node: entry(actorDrew + `, "updatedDescription": true`),
			want: IssueActivityDescriptionUpdated,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			activity := fetchActivity(t, "", test.node)
			if len(activity) != 1 {
				t.Fatalf("got %d events, want 1: %+v", len(activity), activity)
			}
			if activity[0].Kind != test.want {
				t.Fatalf("Kind = %q, want %q", activity[0].Kind, test.want)
			}
			if activity[0].Actor.DisplayName != "drew" {
				t.Errorf("Actor.DisplayName = %q, want drew", activity[0].Actor.DisplayName)
			}
			if test.check != nil {
				test.check(t, activity[0])
			}
		})
	}
}

// The type codes are undocumented. These are the ones a real workspace's
// history produced, plus the removals that follow the same pattern.
func TestRelationChangeCodesReadAsAddedOrRemoved(t *testing.T) {
	tests := []struct {
		code         string
		wantKind     IssueActivityKind
		wantRelation string
	}{
		{"ar", IssueActivityRelationAdded, "related"},
		{"rr", IssueActivityRelationRemoved, "related"},
		{"ax", IssueActivityRelationAdded, "blocking"},
		{"xr", IssueActivityRelationRemoved, "blocking"},
		{"ab", IssueActivityRelationAdded, "blocked by"},
		{"br", IssueActivityRelationRemoved, "blocked by"},
		{"ad", IssueActivityRelationAdded, "duplicate"},
		{"dr", IssueActivityRelationRemoved, "duplicate"},
		{"am", IssueActivityRelationAdded, "duplicate of"},
		{"mr", IssueActivityRelationRemoved, "duplicate of"},
	}

	for _, test := range tests {
		t.Run(test.code, func(t *testing.T) {
			node := entry(actorDrew + `, "relationChanges": [{"identifier": "ABC-9", "type": "` + test.code + `"}]`)
			activity := fetchActivity(t, "", node)
			if len(activity) != 1 {
				t.Fatalf("got %d events, want 1", len(activity))
			}
			if activity[0].Kind != test.wantKind {
				t.Errorf("Kind = %q, want %q", activity[0].Kind, test.wantKind)
			}
			if activity[0].Relation != test.wantRelation {
				t.Errorf("Relation = %q, want %q", activity[0].Relation, test.wantRelation)
			}
			if activity[0].RelatedIssue != "ABC-9" {
				t.Errorf("RelatedIssue = %q, want ABC-9", activity[0].RelatedIssue)
			}
		})
	}
}

// A code this does not recognize costs a line rather than printing a wrong one.
func TestAnUnknownRelationCodeDropsTheEvent(t *testing.T) {
	node := entry(actorDrew + `, "relationChanges": [{"identifier": "ABC-9", "type": "zz"}]`)
	if activity := fetchActivity(t, "", node); len(activity) != 0 {
		t.Fatalf("got %d events, want none: %+v", len(activity), activity)
	}
}

// Linear saves everything changed together as one entry, and the feed draws one
// icon and one phrase per line.
func TestOneHistoryEntryYieldsAnEventPerChange(t *testing.T) {
	node := entry(actorDrew +
		`, "fromState": {"id": "s1", "name": "To Do", "type": "unstarted"}` +
		`, "toState": {"id": "s2", "name": "In Progress", "type": "started"}` +
		`, "toAssignee": {"id": "u1", "name": "Drew White", "displayName": "drew"}`)

	activity := fetchActivity(t, "", node)
	want := []IssueActivityKind{IssueActivityStateChanged, IssueActivitySelfAssigned}
	if len(activity) != len(want) {
		t.Fatalf("got %d events, want %d: %+v", len(activity), len(want), activity)
	}
	for i, kind := range want {
		if activity[i].Kind != kind {
			t.Errorf("event %d = %q, want %q", i, activity[i].Kind, kind)
		}
		if activity[i].ID != "h1" || !activity[i].CreatedAt.Equal(activity[0].CreatedAt) {
			t.Errorf("event %d lost the entry's id or time: %+v", i, activity[i])
		}
	}
}

// An entry recording only changes this does not render is dropped whole.
func TestAnEntryWithNothingRenderableIsDropped(t *testing.T) {
	if activity := fetchActivity(t, "", entry(actorDrew+`, "updatedDescription": false`)); len(activity) != 0 {
		t.Fatalf("got %d events, want none: %+v", len(activity), activity)
	}
	if activity := fetchActivity(t, "", entry(actorDrew)); len(activity) != 0 {
		t.Fatalf("got %d events, want none: %+v", len(activity), activity)
	}
}

func TestTheCreationEventHeadsTheFeed(t *testing.T) {
	const created = `"createdAt": "2026-08-01T10:00:00Z",`

	t.Run("from the creator", func(t *testing.T) {
		fields := created + `"creator": {"id": "u1", "name": "Drew White", "displayName": "drew"},`
		activity := fetchActivity(t, fields, entry(actorDrew+`, "toTitle": "renamed"`))
		if len(activity) != 2 {
			t.Fatalf("got %d events, want 2: %+v", len(activity), activity)
		}
		if activity[0].Kind != IssueActivityCreated {
			t.Fatalf("first event = %q, want created", activity[0].Kind)
		}
		if activity[0].Actor.DisplayName != "drew" || activity[0].IsBot {
			t.Errorf("actor = %+v, want drew and not a bot", activity[0].Actor)
		}
	})

	t.Run("from a bot when there is no creator", func(t *testing.T) {
		fields := created + `"creator": null, "botActor": {"id": "b1", "name": "Linear", "userDisplayName": "Linear"},`
		activity := fetchActivity(t, fields, "")
		if len(activity) != 1 {
			t.Fatalf("got %d events, want 1: %+v", len(activity), activity)
		}
		if !activity[0].IsBot || activity[0].Actor.DisplayName != "Linear" {
			t.Errorf("actor = %+v, IsBot = %v, want the bot", activity[0].Actor, activity[0].IsBot)
		}
	})

	// An event with no time would sort above the creation and read as though it
	// just happened.
	t.Run("dropped when the issue carries no created time", func(t *testing.T) {
		activity := fetchActivity(t, `"creator": {"id": "u1", "displayName": "drew"},`, "")
		if len(activity) != 0 {
			t.Fatalf("got %d events, want none: %+v", len(activity), activity)
		}
	})
}

// Linear returns the history newest first, which is what keeps the query's cap
// of 50 on the most recent fifty. A feed reads the other way.
func TestActivityIsOldestFirst(t *testing.T) {
	nodes := `{"id": "h3", "createdAt": "2026-08-10T19:05:21Z", "toTitle": "third"},` +
		`{"id": "h2", "createdAt": "2026-08-10T19:05:12Z", "toTitle": "second"},` +
		`{"id": "h1", "createdAt": "2026-08-10T19:05:04Z", "toTitle": "first"}`

	activity := fetchActivity(t, `"createdAt": "2026-08-01T10:00:00Z",`, nodes)
	want := []string{"issue-1", "h1", "h2", "h3"}
	if len(activity) != len(want) {
		t.Fatalf("got %d events, want %d", len(activity), len(want))
	}
	for i, id := range want {
		if activity[i].ID != id {
			t.Errorf("event %d = %q, want %q", i, activity[i].ID, id)
		}
	}
}

// Linear records no actor on an automated transition, an API key with no user,
// or a deleted account.
func TestAnEventSurvivesAMissingActor(t *testing.T) {
	node := entry(`"actor": null, "botActor": null, "toTitle": "renamed"`)
	activity := fetchActivity(t, "", node)
	if len(activity) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(activity), activity)
	}
	if activity[0].Actor != (User{}) {
		t.Errorf("Actor = %+v, want zero", activity[0].Actor)
	}
}

// A person acting through an integration is still the person.
func TestAUserActorWinsOverABotActor(t *testing.T) {
	node := entry(actorDrew + `, "botActor": {"id": "b1", "name": "Linear", "userDisplayName": "Linear"}, "toTitle": "renamed"`)
	activity := fetchActivity(t, "", node)
	if len(activity) != 1 {
		t.Fatalf("got %d events, want 1", len(activity))
	}
	if activity[0].IsBot || activity[0].Actor.DisplayName != "drew" {
		t.Errorf("actor = %+v, IsBot = %v, want drew", activity[0].Actor, activity[0].IsBot)
	}
}
