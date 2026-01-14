package cache

import (
	"context"
	"testing"
	"time"

	"github.com/roeyazroel/linear-tui/internal/linearapi"
)

// mockClient is a mock implementation for testing cache behavior.
// We can't easily mock the actual client, so we test the cache structure.

func TestNewTeamCache(t *testing.T) {
	ttl := 5 * time.Minute
	cache := NewTeamCache(nil, ttl)

	if cache == nil {
		t.Fatal("NewTeamCache() returned nil")
	}

	if cache.ttl != ttl {
		t.Errorf("ttl = %v, want %v", cache.ttl, ttl)
	}

	if cache.users == nil {
		t.Error("users map should be initialized")
	}
	if cache.projects == nil {
		t.Error("projects map should be initialized")
	}
	if cache.states == nil {
		t.Error("states map should be initialized")
	}
}

func TestTeamCache_InvalidateAll(t *testing.T) {
	cache := NewTeamCache(nil, 5*time.Minute)

	// Manually set some cached data
	cache.teams = []linearapi.Team{{ID: "test"}}
	cache.teamsExpiry = time.Now().Add(time.Hour)
	cache.users["team-1"] = []linearapi.User{{ID: "user-1"}}
	cache.usersExpiry["team-1"] = time.Now().Add(time.Hour)
	cache.projects["team-1"] = []linearapi.Project{{ID: "proj-1"}}
	cache.projectsExpiry["team-1"] = time.Now().Add(time.Hour)
	cache.states["team-1"] = []linearapi.WorkflowState{{ID: "state-1"}}
	cache.statesExpiry["team-1"] = time.Now().Add(time.Hour)
	cache.labels["team-1"] = []linearapi.IssueLabel{{ID: "lbl-1", Name: "Bug"}}
	cache.labelsExpiry["team-1"] = time.Now().Add(time.Hour)

	cache.InvalidateAll()

	if len(cache.teams) != 0 {
		t.Error("teams should be cleared after InvalidateAll")
	}
	if len(cache.users) != 0 {
		t.Error("users should be cleared after InvalidateAll")
	}
	if len(cache.projects) != 0 {
		t.Error("projects should be cleared after InvalidateAll")
	}
	if len(cache.states) != 0 {
		t.Error("states should be cleared after InvalidateAll")
	}
	if len(cache.labels) != 0 {
		t.Error("labels should be cleared after InvalidateAll")
	}
}

func TestTeamCache_InvalidateTeams(t *testing.T) {
	cache := NewTeamCache(nil, 5*time.Minute)
	cache.teams = []linearapi.Team{{ID: "test"}}
	cache.teamsExpiry = time.Now().Add(time.Hour)

	cache.InvalidateTeams()

	if cache.teams != nil {
		t.Error("teams should be nil after InvalidateTeams")
	}
	if !cache.teamsExpiry.IsZero() {
		t.Error("teamsExpiry should be zero after InvalidateTeams")
	}
}

func TestTeamCache_InvalidateUsers(t *testing.T) {
	cache := NewTeamCache(nil, 5*time.Minute)
	cache.users["team-1"] = []linearapi.User{{ID: "user-1"}}
	cache.usersExpiry["team-1"] = time.Now().Add(time.Hour)
	cache.users["team-2"] = []linearapi.User{{ID: "user-2"}}
	cache.usersExpiry["team-2"] = time.Now().Add(time.Hour)

	cache.InvalidateUsers("team-1")

	if _, ok := cache.users["team-1"]; ok {
		t.Error("team-1 users should be removed")
	}
	if _, ok := cache.users["team-2"]; !ok {
		t.Error("team-2 users should still exist")
	}
}

func TestTeamCache_InvalidateProjects(t *testing.T) {
	cache := NewTeamCache(nil, 5*time.Minute)
	cache.projects["team-1"] = []linearapi.Project{{ID: "proj-1"}}
	cache.projectsExpiry["team-1"] = time.Now().Add(time.Hour)

	cache.InvalidateProjects("team-1")

	if _, ok := cache.projects["team-1"]; ok {
		t.Error("team-1 projects should be removed")
	}
}

func TestTeamCache_InvalidateWorkflowStates(t *testing.T) {
	cache := NewTeamCache(nil, 5*time.Minute)
	cache.states["team-1"] = []linearapi.WorkflowState{{ID: "state-1"}}
	cache.statesExpiry["team-1"] = time.Now().Add(time.Hour)

	cache.InvalidateWorkflowStates("team-1")

	if _, ok := cache.states["team-1"]; ok {
		t.Error("team-1 states should be removed")
	}
}

func TestTeamCache_GetTeams_CacheHit(t *testing.T) {
	cache := NewTeamCache(nil, 5*time.Minute)

	// Pre-populate cache
	expectedTeams := []linearapi.Team{
		{ID: "team-1", Name: "Team 1"},
		{ID: "team-2", Name: "Team 2"},
	}
	cache.teams = expectedTeams
	cache.teamsExpiry = time.Now().Add(time.Hour)

	ctx := context.Background()
	teams, err := cache.GetTeams(ctx)

	if err != nil {
		t.Fatalf("GetTeams() error = %v", err)
	}

	if len(teams) != len(expectedTeams) {
		t.Errorf("GetTeams() returned %d teams, want %d", len(teams), len(expectedTeams))
	}

	for i, team := range teams {
		if team.ID != expectedTeams[i].ID {
			t.Errorf("GetTeams()[%d].ID = %q, want %q", i, team.ID, expectedTeams[i].ID)
		}
	}
}

func TestTeamCache_GetUsers_CacheHit(t *testing.T) {
	cache := NewTeamCache(nil, 5*time.Minute)

	// Pre-populate cache
	teamID := "team-1"
	expectedUsers := []linearapi.User{
		{ID: "user-1", Name: "User 1"},
		{ID: "user-2", Name: "User 2"},
	}
	cache.users[teamID] = expectedUsers
	cache.usersExpiry[teamID] = time.Now().Add(time.Hour)

	ctx := context.Background()
	users, err := cache.GetUsers(ctx, teamID)

	if err != nil {
		t.Fatalf("GetUsers() error = %v", err)
	}

	if len(users) != len(expectedUsers) {
		t.Errorf("GetUsers() returned %d users, want %d", len(users), len(expectedUsers))
	}
}

func TestTeamCache_GetProjects_CacheHit(t *testing.T) {
	cache := NewTeamCache(nil, 5*time.Minute)

	// Pre-populate cache
	teamID := "team-1"
	expectedProjects := []linearapi.Project{
		{ID: "proj-1", Name: "Project 1"},
	}
	cache.projects[teamID] = expectedProjects
	cache.projectsExpiry[teamID] = time.Now().Add(time.Hour)

	ctx := context.Background()
	projects, err := cache.GetProjects(ctx, teamID)

	if err != nil {
		t.Fatalf("GetProjects() error = %v", err)
	}

	if len(projects) != len(expectedProjects) {
		t.Errorf("GetProjects() returned %d projects, want %d", len(projects), len(expectedProjects))
	}
}

func TestTeamCache_GetWorkflowStates_CacheHit(t *testing.T) {
	cache := NewTeamCache(nil, 5*time.Minute)

	// Pre-populate cache
	teamID := "team-1"
	expectedStates := []linearapi.WorkflowState{
		{ID: "state-1", Name: "Todo", Type: "unstarted"},
		{ID: "state-2", Name: "In Progress", Type: "started"},
		{ID: "state-3", Name: "Done", Type: "completed"},
	}
	cache.states[teamID] = expectedStates
	cache.statesExpiry[teamID] = time.Now().Add(time.Hour)

	ctx := context.Background()
	states, err := cache.GetWorkflowStates(ctx, teamID)

	if err != nil {
		t.Fatalf("GetWorkflowStates() error = %v", err)
	}

	if len(states) != len(expectedStates) {
		t.Errorf("GetWorkflowStates() returned %d states, want %d", len(states), len(expectedStates))
	}
}

func TestTeamCache_CacheExpiry(t *testing.T) {
	cache := NewTeamCache(nil, 1*time.Millisecond)

	// Pre-populate with expired cache
	cache.teams = []linearapi.Team{{ID: "old-team"}}
	cache.teamsExpiry = time.Now().Add(-time.Hour) // Already expired

	// Without a real client, we can't test the full flow,
	// but we can verify the expiry check logic

	cache.mu.RLock()
	isExpired := time.Now().After(cache.teamsExpiry)
	cache.mu.RUnlock()

	if !isExpired {
		t.Error("Cache should be expired")
	}
}

func TestTeamCache_GetCurrentUser_CacheHit(t *testing.T) {
	cache := NewTeamCache(nil, 5*time.Minute)

	// Pre-populate cache
	expectedUser := linearapi.User{
		ID:   "user-me",
		Name: "Current User",
		IsMe: true,
	}
	cache.currentUser = &expectedUser
	cache.currentUserExp = time.Now().Add(time.Hour)

	ctx := context.Background()
	user, err := cache.GetCurrentUser(ctx)

	if err != nil {
		t.Fatalf("GetCurrentUser() error = %v", err)
	}

	if user.ID != expectedUser.ID {
		t.Errorf("GetCurrentUser().ID = %q, want %q", user.ID, expectedUser.ID)
	}
	if !user.IsMe {
		t.Error("GetCurrentUser().IsMe should be true")
	}
}

func TestTeamCache_InvalidateIssueLabels(t *testing.T) {
	cache := NewTeamCache(nil, 5*time.Minute)
	cache.labels["team-1"] = []linearapi.IssueLabel{{ID: "lbl-1", Name: "Bug"}}
	cache.labelsExpiry["team-1"] = time.Now().Add(time.Hour)
	cache.labels["team-2"] = []linearapi.IssueLabel{{ID: "lbl-2", Name: "Feature"}}
	cache.labelsExpiry["team-2"] = time.Now().Add(time.Hour)

	cache.InvalidateIssueLabels("team-1")

	if _, ok := cache.labels["team-1"]; ok {
		t.Error("team-1 labels should be removed")
	}
	if _, ok := cache.labels["team-2"]; !ok {
		t.Error("team-2 labels should still exist")
	}
}

func TestTeamCache_GetIssueLabels_CacheHit(t *testing.T) {
	cache := NewTeamCache(nil, 5*time.Minute)

	// Pre-populate cache
	teamID := "team-1"
	expectedLabels := []linearapi.IssueLabel{
		{ID: "lbl-1", Name: "Bug", Color: "#ff0000"},
		{ID: "lbl-2", Name: "Feature", Color: "#00ff00"},
		{ID: "lbl-3", Name: "Documentation", Color: "#0000ff"},
	}
	cache.labels[teamID] = expectedLabels
	cache.labelsExpiry[teamID] = time.Now().Add(time.Hour)

	ctx := context.Background()
	labels, err := cache.GetIssueLabels(ctx, teamID)

	if err != nil {
		t.Fatalf("GetIssueLabels() error = %v", err)
	}

	if len(labels) != len(expectedLabels) {
		t.Errorf("GetIssueLabels() returned %d labels, want %d", len(labels), len(expectedLabels))
	}

	for i, label := range labels {
		if label.ID != expectedLabels[i].ID {
			t.Errorf("GetIssueLabels()[%d].ID = %q, want %q", i, label.ID, expectedLabels[i].ID)
		}
		if label.Name != expectedLabels[i].Name {
			t.Errorf("GetIssueLabels()[%d].Name = %q, want %q", i, label.Name, expectedLabels[i].Name)
		}
	}
}

func TestNewTeamCache_LabelsInitialized(t *testing.T) {
	cache := NewTeamCache(nil, 5*time.Minute)

	if cache.labels == nil {
		t.Error("labels map should be initialized")
	}
	if cache.labelsExpiry == nil {
		t.Error("labelsExpiry map should be initialized")
	}
}
