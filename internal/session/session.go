// Package session persists where the user left off: the workspace, the
// navigation selection, the focused issue, the active tab, the filters, and
// the search query. It lives outside internal/config because config models
// settings a user writes by hand, and this file is written only by the app.
package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zen-linear/zen-linear/internal/config"
)

// currentVersion is the schema version this build reads and writes. A file at
// any other version is discarded rather than migrated.
const currentVersion = 1

// fileName is the session file under the application directory.
const fileName = "session.json"

// NavKind names the sort of issue list a saved selection reopens.
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

// NavSelection locates a navigation selection. The fields mirror the
// discriminators the issue fetch reads, so a restored selection scopes the
// list the same way the live one did.
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

// Filters mirrors the issue list's rich filters. It is declared here rather
// than reusing the linearapi filter types, which carry no JSON tags and are
// shaped for GraphQL. Display names ride along with the ids so a restored
// session shows names in the status bar instead of raw ids.
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

// State is one workspace's restore point.
type State struct {
	Nav     NavSelection `json:"nav"`
	IssueID string       `json:"issue_id,omitempty"`
	Section string       `json:"section,omitempty"`
	Filters Filters      `json:"filters,omitempty"`
	Search  string       `json:"search,omitempty"`
}

// File is the on-disk session record. States are keyed by workspace because
// every id inside one is workspace-scoped, so a single record would drop the
// user's place on every switch away and back.
type File struct {
	Version       int              `json:"version"`
	LastWorkspace string           `json:"last_workspace,omitempty"`
	Workspaces    map[string]State `json:"workspaces,omitempty"`
}

// Path returns the session file path under the application directory.
func Path() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fileName), nil
}

// StateFor returns the saved state for a workspace name. A session opened
// with a bare API key or OAuth has no workspace name and keys on the empty
// string, which no configured workspace can collide with: settings validation
// requires every workspace to be named.
func (f File) StateFor(workspaceName string) (State, bool) {
	state, ok := f.Workspaces[workspaceKey(workspaceName)]
	return state, ok
}

// Load reads the session file. A missing file is not an error: it means the
// app has never been quit, so there is nothing to restore. A file written by
// another schema version is discarded.
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

// Save writes the session file, creating directories as needed. The write goes
// to a temp file and renames over the target: an interrupted in-place write
// would leave JSON that Load rejects, and the next Record would then rebuild
// from empty, costing every workspace its saved place rather than just the one
// being written. Mode 0600 matches the credentials store, since the file holds
// the user's search text and the issues they were reading.
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

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create session directory: %w", err)
	}

	temp, err := os.CreateTemp(dir, ".session-*.json")
	if err != nil {
		return fmt.Errorf("create session temp file: %w", err)
	}
	tempName := temp.Name()
	defer func() {
		// Harmless once the rename succeeded; the temp file is gone by then.
		_ = os.Remove(tempName)
	}()

	if _, err := temp.Write(data); err != nil {
		// The write error is the one worth reporting; the temp file is removed
		// either way by the deferred cleanup.
		_ = temp.Close()
		return fmt.Errorf("write session file: %w", err)
	}
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return fmt.Errorf("set session file permissions: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close session file: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("replace session file: %w", err)
	}

	return nil
}

// Record stores one workspace's state and marks it as the last one open,
// keeping the other workspaces' entries. An unreadable file is replaced
// rather than propagated: a corrupt record must not block the next save.
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

// MarkLast records which workspace is open without touching any saved state.
// A workspace switch marks the incoming workspace before its data has loaded,
// so there is nothing worth writing as its state yet, but a crash before the
// next quit should still reopen it rather than the one left behind.
func MarkLast(path, workspaceName string) error {
	file, err := Load(path)
	if err != nil {
		file = File{}
	}

	file.LastWorkspace = strings.TrimSpace(workspaceName)

	return Save(path, file)
}

// workspaceKey normalizes a workspace name into a map key so a rename of case
// or padding in the config still finds the saved state.
func workspaceKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
