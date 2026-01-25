package cache

import (
	"context"
	"sync"
	"time"

	"github.com/roeyazroel/linear-tui/internal/linearapi"
	"github.com/roeyazroel/linear-tui/internal/logger"
)

// TeamCache provides TTL-based caching for team-scoped metadata.
// It caches teams, users, projects, workflow states, and labels to reduce API calls.
type TeamCache struct {
	client *linearapi.Client
	ttl    time.Duration

	mu sync.RWMutex

	// Global caches
	teams          []linearapi.Team
	teamsExpiry    time.Time
	currentUser    *linearapi.User
	currentUserExp time.Time

	// Per-team caches
	users       map[string][]linearapi.User
	usersExpiry map[string]time.Time

	projects       map[string][]linearapi.Project
	projectsExpiry map[string]time.Time

	states       map[string][]linearapi.WorkflowState
	statesExpiry map[string]time.Time

	// Label caches (merged team + workspace labels per team)
	labels       map[string][]linearapi.IssueLabel
	labelsExpiry map[string]time.Time
}

// NewTeamCache creates a new team cache with the given client and TTL.
func NewTeamCache(client *linearapi.Client, ttl time.Duration) *TeamCache {
	return &TeamCache{
		client:         client,
		ttl:            ttl,
		users:          make(map[string][]linearapi.User),
		usersExpiry:    make(map[string]time.Time),
		projects:       make(map[string][]linearapi.Project),
		projectsExpiry: make(map[string]time.Time),
		states:         make(map[string][]linearapi.WorkflowState),
		statesExpiry:   make(map[string]time.Time),
		labels:         make(map[string][]linearapi.IssueLabel),
		labelsExpiry:   make(map[string]time.Time),
	}
}

// GetTeams returns cached teams or fetches them from the API.
func (c *TeamCache) GetTeams(ctx context.Context) ([]linearapi.Team, error) {
	c.mu.RLock()
	if time.Now().Before(c.teamsExpiry) && len(c.teams) > 0 {
		teams := c.teams
		c.mu.RUnlock()
		return teams, nil
	}
	c.mu.RUnlock()

	// Fetch from API
	logger.Debug("cache.team: cache miss for teams, fetching from API")
	teams, err := c.client.ListTeams(ctx)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.teams = teams
	c.teamsExpiry = time.Now().Add(c.ttl)
	c.mu.Unlock()

	logger.Debug("cache.team: cached teams count=%d ttl=%s", len(teams), c.ttl)
	return teams, nil
}

// GetCurrentUser returns the cached current user or fetches from the API.
func (c *TeamCache) GetCurrentUser(ctx context.Context) (linearapi.User, error) {
	c.mu.RLock()
	if time.Now().Before(c.currentUserExp) && c.currentUser != nil {
		user := *c.currentUser
		c.mu.RUnlock()
		return user, nil
	}
	c.mu.RUnlock()

	// Fetch from API
	logger.Debug("cache.team: cache miss for current user, fetching from API")
	user, err := c.client.GetCurrentUser(ctx)
	if err != nil {
		return linearapi.User{}, err
	}

	c.mu.Lock()
	c.currentUser = &user
	c.currentUserExp = time.Now().Add(c.ttl)
	c.mu.Unlock()

	logger.Debug("cache.team: cached current user user=%s", user.DisplayName)
	return user, nil
}

// getCachedOrFetch is a generic helper function to get cached data or fetch from API.
func getCachedOrFetch[T any](
	ctx context.Context,
	c *TeamCache,
	teamID string,
	cache map[string][]T,
	expiryMap map[string]time.Time,
	fetchFunc func(context.Context, string) ([]T, error),
) ([]T, error) {
	c.mu.RLock()
	if exp, ok := expiryMap[teamID]; ok && time.Now().Before(exp) {
		data := cache[teamID]
		c.mu.RUnlock()
		return data, nil
	}
	c.mu.RUnlock()

	// Fetch from API
	logger.Debug("cache.team: cache miss team_id=%s, fetching from API", teamID)
	data, err := fetchFunc(ctx, teamID)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	cache[teamID] = data
	expiryMap[teamID] = time.Now().Add(c.ttl)
	c.mu.Unlock()

	logger.Debug("cache.team: cached data team_id=%s count=%d ttl=%s", teamID, len(data), c.ttl)
	return data, nil
}

// GetUsers returns cached users for a team or fetches them from the API.
func (c *TeamCache) GetUsers(ctx context.Context, teamID string) ([]linearapi.User, error) {
	return getCachedOrFetch(ctx, c, teamID, c.users, c.usersExpiry, c.client.ListUsers)
}

// GetProjects returns cached projects for a team or fetches them from the API.
func (c *TeamCache) GetProjects(ctx context.Context, teamID string) ([]linearapi.Project, error) {
	return getCachedOrFetch(ctx, c, teamID, c.projects, c.projectsExpiry, c.client.ListProjects)
}

// GetWorkflowStates returns cached workflow states for a team or fetches from the API.
func (c *TeamCache) GetWorkflowStates(ctx context.Context, teamID string) ([]linearapi.WorkflowState, error) {
	return getCachedOrFetch(ctx, c, teamID, c.states, c.statesExpiry, c.client.ListWorkflowStates)
}

// GetIssueLabels returns cached labels (merged team + workspace) for a team or fetches from the API.
func (c *TeamCache) GetIssueLabels(ctx context.Context, teamID string) ([]linearapi.IssueLabel, error) {
	return getCachedOrFetch(ctx, c, teamID, c.labels, c.labelsExpiry, c.client.ListIssueLabels)
}

// InvalidateTeams clears the teams cache.
func (c *TeamCache) InvalidateTeams() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.teams = nil
	c.teamsExpiry = time.Time{}
}

// InvalidateUsers clears the users cache for a specific team.
func (c *TeamCache) InvalidateUsers(teamID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.users, teamID)
	delete(c.usersExpiry, teamID)
}

// InvalidateProjects clears the projects cache for a specific team.
func (c *TeamCache) InvalidateProjects(teamID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.projects, teamID)
	delete(c.projectsExpiry, teamID)
}

// InvalidateWorkflowStates clears the workflow states cache for a specific team.
func (c *TeamCache) InvalidateWorkflowStates(teamID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.states, teamID)
	delete(c.statesExpiry, teamID)
}

// InvalidateIssueLabels clears the labels cache for a specific team.
func (c *TeamCache) InvalidateIssueLabels(teamID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.labels, teamID)
	delete(c.labelsExpiry, teamID)
}

// InvalidateAll clears all caches.
func (c *TeamCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.teams = nil
	c.teamsExpiry = time.Time{}
	c.currentUser = nil
	c.currentUserExp = time.Time{}

	c.users = make(map[string][]linearapi.User)
	c.usersExpiry = make(map[string]time.Time)
	c.projects = make(map[string][]linearapi.Project)
	c.projectsExpiry = make(map[string]time.Time)
	c.states = make(map[string][]linearapi.WorkflowState)
	c.statesExpiry = make(map[string]time.Time)
	c.labels = make(map[string][]linearapi.IssueLabel)
	c.labelsExpiry = make(map[string]time.Time)
}

// PreloadTeamMetadata preloads all metadata for a team (users, projects, states, labels).
// This can be called when a team is selected to reduce perceived latency.
func (c *TeamCache) PreloadTeamMetadata(ctx context.Context, teamID string) error {
	// Load in parallel
	var wg sync.WaitGroup
	var usersErr, projectsErr, statesErr, labelsErr error

	wg.Add(4)

	go func() {
		defer wg.Done()
		_, usersErr = c.GetUsers(ctx, teamID)
	}()

	go func() {
		defer wg.Done()
		_, projectsErr = c.GetProjects(ctx, teamID)
	}()

	go func() {
		defer wg.Done()
		_, statesErr = c.GetWorkflowStates(ctx, teamID)
	}()

	go func() {
		defer wg.Done()
		_, labelsErr = c.GetIssueLabels(ctx, teamID)
	}()

	wg.Wait()

	// Return first error encountered
	if usersErr != nil {
		return usersErr
	}
	if projectsErr != nil {
		return projectsErr
	}
	if statesErr != nil {
		return statesErr
	}
	if labelsErr != nil {
		return labelsErr
	}

	return nil
}
