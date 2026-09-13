package cache

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
)

type TeamCache struct {
	client *linearapi.Client
	ttl    time.Duration

	mu sync.RWMutex

	teams          []linearapi.Team
	teamsExpiry    time.Time
	currentUser    *linearapi.User
	currentUserExp time.Time

	users       map[string][]linearapi.User
	usersExpiry map[string]time.Time

	projects       map[string][]linearapi.Project
	projectsExpiry map[string]time.Time

	states       map[string][]linearapi.WorkflowState
	statesExpiry map[string]time.Time

	cycles       map[string][]linearapi.Cycle
	cyclesExpiry map[string]time.Time

	labels       map[string][]linearapi.IssueLabel
	labelsExpiry map[string]time.Time

	projectMilestones       map[string][]linearapi.ProjectMilestone
	projectMilestonesExpiry map[string]time.Time

	inflight map[string]*cacheFlight
}

type cacheFlight struct {
	done chan struct{}
	err  error
}

func NewTeamCache(client *linearapi.Client, ttl time.Duration) *TeamCache {
	return &TeamCache{
		client:                  client,
		ttl:                     ttl,
		users:                   make(map[string][]linearapi.User),
		usersExpiry:             make(map[string]time.Time),
		projects:                make(map[string][]linearapi.Project),
		projectsExpiry:          make(map[string]time.Time),
		states:                  make(map[string][]linearapi.WorkflowState),
		statesExpiry:            make(map[string]time.Time),
		cycles:                  make(map[string][]linearapi.Cycle),
		cyclesExpiry:            make(map[string]time.Time),
		labels:                  make(map[string][]linearapi.IssueLabel),
		labelsExpiry:            make(map[string]time.Time),
		projectMilestones:       make(map[string][]linearapi.ProjectMilestone),
		projectMilestonesExpiry: make(map[string]time.Time),
		inflight:                make(map[string]*cacheFlight),
	}
}

func (c *TeamCache) GetTeams(ctx context.Context) ([]linearapi.Team, error) {
	c.mu.RLock()
	if time.Now().Before(c.teamsExpiry) && len(c.teams) > 0 {
		teams := c.teams
		c.mu.RUnlock()
		return teams, nil
	}
	c.mu.RUnlock()

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

func (c *TeamCache) GetCurrentUser(ctx context.Context) (linearapi.User, error) {
	c.mu.RLock()
	if time.Now().Before(c.currentUserExp) && c.currentUser != nil {
		user := *c.currentUser
		c.mu.RUnlock()
		return user, nil
	}
	c.mu.RUnlock()

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

func getCachedOrFetch[T any](
	ctx context.Context,
	c *TeamCache,
	kind string,
	teamID string,
	cache map[string][]T,
	expiryMap map[string]time.Time,
	fetchFunc func(context.Context, string) ([]T, error),
) ([]T, error) {
	key := kind + ":" + teamID

	var flight *cacheFlight
	for {
		c.mu.Lock()
		if exp, ok := expiryMap[teamID]; ok && time.Now().Before(exp) {
			data := slices.Clone(cache[teamID])
			c.mu.Unlock()
			return data, nil
		}
		if running, ok := c.inflight[key]; ok {
			c.mu.Unlock()
			select {
			case <-running.done:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			c.mu.Lock()
			err := running.err
			c.mu.Unlock()
			if err != nil {
				return nil, err
			}
			continue
		}
		flight = &cacheFlight{done: make(chan struct{})}
		c.inflight[key] = flight
		c.mu.Unlock()
		break
	}

	logger.Debug("cache.team: cache miss team_id=%s, fetching from API", teamID)
	data, err := fetchFunc(ctx, teamID)

	c.mu.Lock()
	if err == nil {
		cache[teamID] = data
		expiryMap[teamID] = time.Now().Add(c.ttl)
		data = slices.Clone(data)
	}
	flight.err = err
	delete(c.inflight, key)
	close(flight.done)
	c.mu.Unlock()

	if err != nil {
		return nil, err
	}

	logger.Debug("cache.team: cached data team_id=%s count=%d ttl=%s", teamID, len(data), c.ttl)
	return data, nil
}

func (c *TeamCache) GetUsers(ctx context.Context, teamID string) ([]linearapi.User, error) {
	return getCachedOrFetch(ctx, c, "users", teamID, c.users, c.usersExpiry, c.client.ListUsers)
}

func (c *TeamCache) GetProjects(ctx context.Context, teamID string) ([]linearapi.Project, error) {
	return getCachedOrFetch(ctx, c, "projects", teamID, c.projects, c.projectsExpiry, c.client.ListProjects)
}

func (c *TeamCache) GetWorkflowStates(ctx context.Context, teamID string) ([]linearapi.WorkflowState, error) {
	return getCachedOrFetch(ctx, c, "states", teamID, c.states, c.statesExpiry, c.client.ListWorkflowStates)
}

func (c *TeamCache) GetCycles(ctx context.Context, teamID string) ([]linearapi.Cycle, error) {
	return getCachedOrFetch(ctx, c, "cycles", teamID, c.cycles, c.cyclesExpiry, c.client.ListCycles)
}

func (c *TeamCache) GetIssueLabels(ctx context.Context, teamID string) ([]linearapi.IssueLabel, error) {
	return getCachedOrFetch(ctx, c, "labels", teamID, c.labels, c.labelsExpiry, c.client.ListIssueLabels)
}

func (c *TeamCache) GetProjectMilestones(ctx context.Context, projectID string) ([]linearapi.ProjectMilestone, error) {
	return getCachedOrFetch(ctx, c, "milestones", projectID, c.projectMilestones, c.projectMilestonesExpiry, c.client.ListProjectMilestones)
}

func (c *TeamCache) InvalidateTeams() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.teams = nil
	c.teamsExpiry = time.Time{}
}

func (c *TeamCache) InvalidateUsers(teamID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.users, teamID)
	delete(c.usersExpiry, teamID)
}

func (c *TeamCache) InvalidateProjects(teamID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.projects, teamID)
	delete(c.projectsExpiry, teamID)
}

func (c *TeamCache) InvalidateWorkflowStates(teamID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.states, teamID)
	delete(c.statesExpiry, teamID)
}

func (c *TeamCache) InvalidateCycles(teamID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cycles, teamID)
	delete(c.cyclesExpiry, teamID)
}

func (c *TeamCache) InvalidateIssueLabels(teamID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.labels, teamID)
	delete(c.labelsExpiry, teamID)
}

func (c *TeamCache) InvalidateProjectMilestones(projectID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.projectMilestones, projectID)
	delete(c.projectMilestonesExpiry, projectID)
}

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
	c.cycles = make(map[string][]linearapi.Cycle)
	c.cyclesExpiry = make(map[string]time.Time)
	c.labels = make(map[string][]linearapi.IssueLabel)
	c.labelsExpiry = make(map[string]time.Time)
	c.projectMilestones = make(map[string][]linearapi.ProjectMilestone)
	c.projectMilestonesExpiry = make(map[string]time.Time)
}

// PreloadTeamMetadata warms a team's users, projects, states, cycles and labels
// in parallel and returns the first error.
func (c *TeamCache) PreloadTeamMetadata(ctx context.Context, teamID string) error {
	var wg sync.WaitGroup
	var usersErr, projectsErr, statesErr, cyclesErr, labelsErr error

	wg.Add(5)

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
		_, cyclesErr = c.GetCycles(ctx, teamID)
	}()

	go func() {
		defer wg.Done()
		_, labelsErr = c.GetIssueLabels(ctx, teamID)
	}()

	wg.Wait()

	if usersErr != nil {
		return usersErr
	}
	if projectsErr != nil {
		return projectsErr
	}
	if statesErr != nil {
		return statesErr
	}
	if cyclesErr != nil {
		return cyclesErr
	}
	if labelsErr != nil {
		return labelsErr
	}

	return nil
}
