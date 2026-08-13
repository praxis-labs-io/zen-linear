package session

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestLoad covers the states a session file can be in on disk.
func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		write   bool
		content string
		want    File
		wantErr bool
	}{
		{
			name:  "missing file returns empty record",
			write: false,
			want:  File{},
		},
		{
			name:    "malformed json is an error",
			write:   true,
			content: "{",
			wantErr: true,
		},
		{
			name:    "wrong version is discarded",
			write:   true,
			content: `{"version": 99, "last_workspace": "acme"}`,
			want:    File{},
		},
		{
			name:    "current version parses",
			write:   true,
			content: `{"version": 1, "last_workspace": "Acme", "workspaces": {"acme": {"nav": {"kind": "team", "team_id": "t1"}, "issue_id": "i1"}}}`,
			want: File{
				Version:       1,
				LastWorkspace: "Acme",
				Workspaces: map[string]State{
					"acme": {Nav: NavSelection{Kind: NavTeam, TeamID: "t1"}, IssueID: "i1"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "session.json")
			if tt.write {
				if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
					t.Fatalf("write fixture: %v", err)
				}
			}

			got, err := Load(path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Load() error = nil, want error")
				}
				if !reflect.DeepEqual(got, File{}) {
					t.Fatalf("Load() = %+v, want zero record on error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Load() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestLoadEmptyPath verifies an unresolved path is rejected rather than read.
func TestLoadEmptyPath(t *testing.T) {
	if _, err := Load(""); err == nil {
		t.Fatal("Load(\"\") error = nil, want error")
	}
}

// TestRecordKeepsOtherWorkspaces verifies switching between workspaces does
// not drop the place saved for the one left behind.
func TestRecordKeepsOtherWorkspaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "session.json")

	stateA := State{Nav: NavSelection{Kind: NavProject, TeamID: "t1", ProjectID: "p1"}, IssueID: "i1"}
	stateB := State{Nav: NavSelection{Kind: NavTeam, TeamID: "t2"}, Search: "login"}

	if err := Record(path, "Alpha", stateA); err != nil {
		t.Fatalf("Record(Alpha) error: %v", err)
	}
	if err := Record(path, "Beta", stateB); err != nil {
		t.Fatalf("Record(Beta) error: %v", err)
	}

	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if file.LastWorkspace != "Beta" {
		t.Fatalf("LastWorkspace = %q, want Beta", file.LastWorkspace)
	}
	if got, ok := file.StateFor("alpha"); !ok || !reflect.DeepEqual(got, stateA) {
		t.Fatalf("StateFor(alpha) = %+v, %v, want %+v, true", got, ok, stateA)
	}
	if got, ok := file.StateFor("Beta"); !ok || !reflect.DeepEqual(got, stateB) {
		t.Fatalf("StateFor(Beta) = %+v, %v, want %+v, true", got, ok, stateB)
	}
}

// TestRecordReplacesCorruptFile verifies a bad record on disk does not block
// the next save.
func TestRecordReplacesCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	if err := os.WriteFile(path, []byte("{ not json"), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	state := State{Nav: NavSelection{Kind: NavAll}}
	if err := Record(path, "Alpha", state); err != nil {
		t.Fatalf("Record() error: %v", err)
	}

	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got, ok := file.StateFor("Alpha"); !ok || !reflect.DeepEqual(got, state) {
		t.Fatalf("StateFor(Alpha) = %+v, %v, want %+v, true", got, ok, state)
	}
}

// TestMarkLastKeepsSavedStates verifies marking the open workspace does not
// disturb any workspace's saved place.
func TestMarkLastKeepsSavedStates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	state := State{Nav: NavSelection{Kind: NavCycle, TeamID: "t1", CycleID: "c1"}, IssueID: "i1"}
	if err := Record(path, "Alpha", state); err != nil {
		t.Fatalf("Record() error: %v", err)
	}

	if err := MarkLast(path, "Beta"); err != nil {
		t.Fatalf("MarkLast() error: %v", err)
	}

	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if file.LastWorkspace != "Beta" {
		t.Fatalf("LastWorkspace = %q, want Beta", file.LastWorkspace)
	}
	if got, ok := file.StateFor("Alpha"); !ok || !reflect.DeepEqual(got, state) {
		t.Fatalf("StateFor(Alpha) = %+v, %v, want %+v, true", got, ok, state)
	}
}

// TestRecordUnnamedWorkspace verifies an API-key or OAuth session, which has
// no workspace name, round-trips under the empty key.
func TestRecordUnnamedWorkspace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")

	state := State{Nav: NavSelection{Kind: NavCustomView, CustomViewID: "cv1", FavoriteID: "f1"}}
	if err := Record(path, "", state); err != nil {
		t.Fatalf("Record() error: %v", err)
	}

	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if file.LastWorkspace != "" {
		t.Fatalf("LastWorkspace = %q, want empty", file.LastWorkspace)
	}
	if got, ok := file.StateFor(""); !ok || !reflect.DeepEqual(got, state) {
		t.Fatalf("StateFor(\"\") = %+v, %v, want %+v, true", got, ok, state)
	}
	if _, ok := file.StateFor("Alpha"); ok {
		t.Fatal("StateFor(Alpha) found the unnamed entry")
	}
}

// TestSaveRoundTrip verifies filters and the full state survive a write.
func TestSaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	estimate := 3.0
	want := File{
		Version:       currentVersion,
		LastWorkspace: "Alpha",
		Workspaces: map[string]State{
			"alpha": {
				Nav:     NavSelection{Kind: NavStatus, TeamID: "t1", StateID: "s1"},
				IssueID: "i1",
				Search:  "login",
				Filters: Filters{
					AssigneeID:   "u1",
					AssigneeName: "Drew",
					LabelIDs:     []string{"l1", "l2"},
					LabelNames:   []string{"bug", "ui"},
					DueDate:      "2026-08-06",
					Estimate:     &estimate,
				},
			},
		},
	}

	if err := Save(path, want); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
}

// TestSaveSetsCurrentVersion verifies the writer stamps the version rather
// than trusting the caller.
func TestSaveSetsCurrentVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	if err := Save(path, File{Version: 0, LastWorkspace: "Alpha"}); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got.Version != currentVersion {
		t.Fatalf("Version = %d, want %d", got.Version, currentVersion)
	}
}

// TestPathUnderConfigDir verifies the file sits beside the other app state.
func TestPathUnderConfigDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := Path()
	if err != nil {
		t.Fatalf("Path() error: %v", err)
	}
	want := filepath.Join(home, ".zen-linear", "session.json")
	if got != want {
		t.Fatalf("Path() = %q, want %q", got, want)
	}
}

// TestSaveUsesPrivatePermissions verifies the session file is not readable by
// other local users. It holds the search text and the issues the user was
// reading, so it follows the credentials store rather than the config file.
func TestSaveUsesPrivatePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "session.json")

	if err := Save(path, File{LastWorkspace: "Alpha"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions = %o, want 0600", info.Mode().Perm())
	}
}

// TestSaveTightensExistingPermissions verifies a session file left at 0644 by
// an earlier build is narrowed on the next write. os.WriteFile would have left
// it wide open forever.
func TestSaveTightensExistingPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	if err := os.WriteFile(path, []byte(`{"version":1}`), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := Save(path, File{LastWorkspace: "Alpha"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions = %o, want 0600", info.Mode().Perm())
	}
}

// TestSaveLeavesNoTempFiles verifies the atomic write cleans up after itself,
// so the config directory does not fill with .session-*.json on every quit.
func TestSaveLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.json")

	for range 3 {
		if err := Save(path, File{LastWorkspace: "Alpha"}); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "session.json" {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("directory holds %v, want only session.json", names)
	}
}
