package cache

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/praxis-labs-io/zen-linear/internal/linearapi"
)

func navFixture() NavData {
	return NavData{
		Teams: []linearapi.Team{
			{ID: "team-1", Key: "ENG", Name: "Engineering"},
			{ID: "team-2", Key: "NEX", Name: "Nexa"},
		},
		Favorites: []linearapi.Favorite{
			{ID: "fav-1", Type: "project", ProjectID: "proj-1", ProjectName: "Website", SortOrder: 1.5},
		},
	}
}

func TestRecordNavRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nav-cache.json")
	want := navFixture()

	if err := RecordNav(path, "Alpha", want); err != nil {
		t.Fatalf("RecordNav: %v", err)
	}

	file, err := LoadNav(path)
	if err != nil {
		t.Fatalf("LoadNav: %v", err)
	}
	got, ok := file.DataFor("Alpha")
	if !ok {
		t.Fatal("DataFor(\"Alpha\") = false, want the recorded tree")
	}
	if len(got.Teams) != 2 || got.Teams[1].Name != "Nexa" {
		t.Fatalf("teams = %+v, want the two recorded teams", got.Teams)
	}
	if len(got.Favorites) != 1 || got.Favorites[0].ProjectName != "Website" {
		t.Fatalf("favorites = %+v, want the recorded favorite", got.Favorites)
	}
}

// TestRecordNavKeepsOtherWorkspaces covers the switch back: recording one
// workspace must not drop the tree cached for another.
func TestRecordNavKeepsOtherWorkspaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nav-cache.json")

	if err := RecordNav(path, "Alpha", navFixture()); err != nil {
		t.Fatalf("RecordNav(Alpha): %v", err)
	}
	beta := NavData{Teams: []linearapi.Team{{ID: "team-9", Key: "OPS", Name: "Ops"}}}
	if err := RecordNav(path, "Beta", beta); err != nil {
		t.Fatalf("RecordNav(Beta): %v", err)
	}

	file, err := LoadNav(path)
	if err != nil {
		t.Fatalf("LoadNav: %v", err)
	}
	if _, ok := file.DataFor("Alpha"); !ok {
		t.Fatal("Alpha lost its cached tree")
	}
	got, ok := file.DataFor("Beta")
	if !ok || len(got.Teams) != 1 || got.Teams[0].Name != "Ops" {
		t.Fatalf("Beta = %+v (ok=%v), want the one Ops team", got, ok)
	}
}

// TestDataForNormalizesWorkspaceName covers a rename of case or padding in the
// config, which must still find the cached tree.
func TestDataForNormalizesWorkspaceName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nav-cache.json")
	if err := RecordNav(path, "Alpha", navFixture()); err != nil {
		t.Fatalf("RecordNav: %v", err)
	}

	file, err := LoadNav(path)
	if err != nil {
		t.Fatalf("LoadNav: %v", err)
	}
	if _, ok := file.DataFor("  alpha "); !ok {
		t.Fatal("DataFor(\"  alpha \") = false, want the recorded tree")
	}
}

func TestDataForRejectsEmptyEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nav-cache.json")
	if err := RecordNav(path, "Alpha", NavData{}); err != nil {
		t.Fatalf("RecordNav: %v", err)
	}

	file, err := LoadNav(path)
	if err != nil {
		t.Fatalf("LoadNav: %v", err)
	}
	if _, ok := file.DataFor("Alpha"); ok {
		t.Fatal("DataFor() = true for an empty entry, want false")
	}
}

func TestLoadNavMissingFileIsEmpty(t *testing.T) {
	file, err := LoadNav(filepath.Join(t.TempDir(), "nav-cache.json"))
	if err != nil {
		t.Fatalf("LoadNav: %v", err)
	}
	if _, ok := file.DataFor("Alpha"); ok {
		t.Fatal("DataFor() = true for a missing file, want false")
	}
}

// TestLoadNavDiscardsOtherVersions covers a file written by a build whose
// shape moved. Reading it as if it matched would paint a broken tree.
func TestLoadNavDiscardsOtherVersions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nav-cache.json")
	stale := `{"version":99,"workspaces":{"alpha":{"teams":[{"ID":"team-1","Key":"ENG","Name":"Engineering"}]}}}`
	if err := os.WriteFile(path, []byte(stale), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	file, err := LoadNav(path)
	if err != nil {
		t.Fatalf("LoadNav: %v", err)
	}
	if _, ok := file.DataFor("Alpha"); ok {
		t.Fatal("DataFor() = true for a stale version, want false")
	}
}

func TestRecordNavUsesPrivatePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("NTFS has no unix permission bits and os.Chmod only toggles the read-only flag there, so this is not a guarantee Windows makes")
	}

	path := filepath.Join(t.TempDir(), "nested", "nav-cache.json")

	if err := RecordNav(path, "Alpha", navFixture()); err != nil {
		t.Fatalf("RecordNav: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions = %o, want 0600", info.Mode().Perm())
	}
}
