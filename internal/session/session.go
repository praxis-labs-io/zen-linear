// Package session persists where the user left off in each workspace.
package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/praxis-labs-io/zen-linear/internal/config"
)

const currentVersion = 2

const fileName = "session.json"

type NavKind string

const (
	NavAll        NavKind = "all"
	NavTeam       NavKind = "team"
	NavProject    NavKind = "project"
	NavStatus     NavKind = "status"
	NavCycle      NavKind = "cycle"
	NavCustomView NavKind = "custom_view"
	NavStateType  NavKind = "state_type"
)

type NavSelection struct {
	Kind         NavKind `json:"kind"`
	TeamID       string  `json:"team_id,omitempty"`
	ProjectID    string  `json:"project_id,omitempty"`
	StateID      string  `json:"state_id,omitempty"`
	CycleID      string  `json:"cycle_id,omitempty"`
	CustomViewID string  `json:"custom_view_id,omitempty"`
	StateType    string  `json:"state_type,omitempty"`
	FavoriteID   string  `json:"favorite_id,omitempty"`
}

type Filters struct {
	AssigneeID   string   `json:"assignee_id,omitempty"`
	AssigneeName string   `json:"assignee_name,omitempty"`
	LabelIDs     []string `json:"label_ids,omitempty"`
	LabelNames   []string `json:"label_names,omitempty"`
	StateID      string   `json:"state_id,omitempty"`
	StateName    string   `json:"state_name,omitempty"`
	ProjectID    string   `json:"project_id,omitempty"`
	ProjectName  string   `json:"project_name,omitempty"`
	CycleID      string   `json:"cycle_id,omitempty"`
	CycleName    string   `json:"cycle_name,omitempty"`
	DueDate      string   `json:"due_date,omitempty"`
	Estimate     *float64 `json:"estimate,omitempty"`
}

type State struct {
	Nav     NavSelection `json:"nav"`
	IssueID string       `json:"issue_id,omitempty"`
	Filters Filters      `json:"filters,omitempty"`
	// Search non-empty also means the issues pane was showing results.
	Search string `json:"search,omitempty"`
}

type File struct {
	Version       int              `json:"version"`
	LastWorkspace string           `json:"last_workspace,omitempty"`
	Workspaces    map[string]State `json:"workspaces,omitempty"`
}

func Path() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fileName), nil
}

func (f File) StateFor(workspaceName string) (State, bool) {
	state, ok := f.Workspaces[workspaceKey(workspaceName)]
	return state, ok
}

// Load reads the session file. A missing file or one at another schema version
// returns an empty File and no error.
func Load(path string) (File, error) {
	if path == "" {
		return File{}, fmt.Errorf("session path is empty")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return File{}, nil
		}
		return File{}, fmt.Errorf("read session file: %w", err)
	}

	var file File
	if err := json.Unmarshal(data, &file); err != nil {
		return File{}, fmt.Errorf("parse session file: %w", err)
	}
	if file.Version != currentVersion {
		return File{}, nil
	}

	return file, nil
}

func Save(path string, file File) error {
	if path == "" {
		return fmt.Errorf("session path is empty")
	}

	file.Version = currentVersion
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	data = append(data, '\n')

	if err := config.WriteFileAtomic(path, data, 0o600); err != nil {
		return fmt.Errorf("write session file: %w", err)
	}

	return nil
}

// Record saves one workspace's state and marks it last, keeping the others and
// replacing an unreadable file.
func Record(path, workspaceName string, state State) error {
	file, err := Load(path)
	if err != nil {
		file = File{}
	}
	if file.Workspaces == nil {
		file.Workspaces = make(map[string]State)
	}

	file.LastWorkspace = strings.TrimSpace(workspaceName)
	file.Workspaces[workspaceKey(workspaceName)] = state

	return Save(path, file)
}

func MarkLast(path, workspaceName string) error {
	file, err := Load(path)
	if err != nil {
		file = File{}
	}

	file.LastWorkspace = strings.TrimSpace(workspaceName)

	return Save(path, file)
}

func workspaceKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
