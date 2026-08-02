package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateLegacyDir(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(t *testing.T, home string)
		wantMigrated   bool
		wantLegacyGone bool
	}{
		{
			name: "moves legacy dir when target is absent",
			setup: func(t *testing.T, home string) {
				t.Helper()
				legacy := filepath.Join(home, legacyDirName)
				if err := os.MkdirAll(legacy, 0o755); err != nil {
					t.Fatalf("creating legacy dir: %v", err)
				}
				if err := os.WriteFile(filepath.Join(legacy, "config.json"), []byte(`{"theme":"linear"}`), 0o600); err != nil {
					t.Fatalf("writing legacy config: %v", err)
				}
			},
			wantMigrated:   true,
			wantLegacyGone: true,
		},
		{
			name: "leaves both alone when target already exists",
			setup: func(t *testing.T, home string) {
				t.Helper()
				for _, dir := range []string{legacyDirName, dirName} {
					if err := os.MkdirAll(filepath.Join(home, dir), 0o755); err != nil {
						t.Fatalf("creating %s: %v", dir, err)
					}
				}
				if err := os.WriteFile(filepath.Join(home, dirName, "config.json"), []byte(`{"theme":"linear"}`), 0o600); err != nil {
					t.Fatalf("writing current config: %v", err)
				}
			},
			wantMigrated:   true,
			wantLegacyGone: false,
		},
		{
			name:           "no-op when nothing to migrate",
			setup:          func(t *testing.T, _ string) { t.Helper() },
			wantMigrated:   false,
			wantLegacyGone: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			tt.setup(t, home)

			if err := MigrateLegacyDir(); err != nil {
				t.Fatalf("MigrateLegacyDir() error = %v", err)
			}

			_, err := os.Stat(filepath.Join(home, dirName))
			if gotMigrated := err == nil; gotMigrated != tt.wantMigrated {
				t.Errorf("target dir exists = %v, want %v", gotMigrated, tt.wantMigrated)
			}

			_, err = os.Stat(filepath.Join(home, legacyDirName))
			if gotGone := os.IsNotExist(err); gotGone != tt.wantLegacyGone {
				t.Errorf("legacy dir gone = %v, want %v", gotGone, tt.wantLegacyGone)
			}
		})
	}
}

// A renamed directory must keep symlinked config files resolving to their
// original targets, which is how the dotfiles-managed config survives.
func TestMigrateLegacyDirPreservesSymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	target := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(target, []byte(`{"theme":"rose_pine_moon"}`), 0o600); err != nil {
		t.Fatalf("writing symlink target: %v", err)
	}

	legacy := filepath.Join(home, legacyDirName)
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatalf("creating legacy dir: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(legacy, "config.json")); err != nil {
		t.Fatalf("creating symlink: %v", err)
	}

	if err := MigrateLegacyDir(); err != nil {
		t.Fatalf("MigrateLegacyDir() error = %v", err)
	}

	migrated := filepath.Join(home, dirName, "config.json")
	resolved, err := os.Readlink(migrated)
	if err != nil {
		t.Fatalf("migrated config is not a symlink: %v", err)
	}
	if resolved != target {
		t.Errorf("symlink target = %q, want %q", resolved, target)
	}

	data, err := os.ReadFile(migrated)
	if err != nil {
		t.Fatalf("reading through migrated symlink: %v", err)
	}
	if string(data) != `{"theme":"rose_pine_moon"}` {
		t.Errorf("content through symlink = %q, want the original config", data)
	}
}
