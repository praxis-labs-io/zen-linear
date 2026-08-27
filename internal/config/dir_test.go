package config

import (
	"os"
	"path/filepath"
	"testing"
)

// A dotfiles setup links its configs into ~/.config, so the settings file is
// read from either home. Everything the app writes stays under Dir().
func TestConfigFilePath(t *testing.T) {
	tests := []struct {
		name     string
		xdgHome  bool
		writeXDG bool
		writeDir bool
		want     func(home, xdgBase string) string
	}{
		{
			name:     "prefers an existing file under the default XDG dir",
			writeXDG: true,
			want: func(home, _ string) string {
				return filepath.Join(home, ".config", xdgDirName, "config.json")
			},
		},
		{
			name:     "prefers the XDG file when both exist",
			writeXDG: true,
			writeDir: true,
			want: func(home, _ string) string {
				return filepath.Join(home, ".config", xdgDirName, "config.json")
			},
		},
		{
			name:     "honors XDG_CONFIG_HOME over ~/.config",
			xdgHome:  true,
			writeXDG: true,
			want: func(_, xdgBase string) string {
				return filepath.Join(xdgBase, xdgDirName, "config.json")
			},
		},
		{
			name:     "falls back to the app dir when no XDG file exists",
			writeDir: true,
			want: func(home, _ string) string {
				return filepath.Join(home, dirName, "config.json")
			},
		},
		{
			name: "falls back to the app dir when neither exists, which is where a first run creates one",
			want: func(home, _ string) string {
				return filepath.Join(home, dirName, "config.json")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)

			xdgBase := filepath.Join(home, ".config")
			if tt.xdgHome {
				xdgBase = t.TempDir()
				t.Setenv(xdgConfigHomeEnv, xdgBase)
			} else {
				t.Setenv(xdgConfigHomeEnv, "")
			}

			if tt.writeXDG {
				writeConfigJSON(t, filepath.Join(xdgBase, xdgDirName))
			}
			if tt.writeDir {
				writeConfigJSON(t, filepath.Join(home, dirName))
			}

			got, err := ConfigFilePath()
			if err != nil {
				t.Fatalf("ConfigFilePath() error = %v", err)
			}
			if want := tt.want(home, xdgBase); got != want {
				t.Errorf("ConfigFilePath() = %q, want %q", got, want)
			}
		})
	}
}

// The settings a real launch reads have to come from the XDG copy, not just
// the path.
func TestLoadSettingsReadsTheXDGConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(xdgConfigHomeEnv, "")

	dir := filepath.Join(home, ".config", xdgDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating XDG config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"theme":"rose_pine_moon"}`), 0o600); err != nil {
		t.Fatalf("writing XDG config: %v", err)
	}

	path, err := ConfigFilePath()
	if err != nil {
		t.Fatalf("ConfigFilePath() error = %v", err)
	}
	settings, err := LoadSettings(path)
	if err != nil {
		t.Fatalf("LoadSettings() error = %v", err)
	}
	if settings.Theme != "rose_pine_moon" {
		t.Errorf("Theme = %q, want the value from the XDG config", settings.Theme)
	}
}

// The log stays under Dir() even when the settings come from XDG, because a
// per-launch write into a dotfiles repo dirties it on every run.
func TestDefaultLogFileStaysUnderTheAppDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(xdgConfigHomeEnv, "")
	writeConfigJSON(t, filepath.Join(home, ".config", xdgDirName))

	want := filepath.Join(home, dirName, "app.log")
	if got := DefaultLogFile(); got != want {
		t.Errorf("DefaultLogFile() = %q, want %q", got, want)
	}
}

func writeConfigJSON(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatalf("writing config in %s: %v", dir, err)
	}
}

// Four writers used to race to create ~/.zen-linear, three of them at 0755.
// MkdirAll no-ops on one that exists, so whichever ran first decided the mode
// of the directory holding credentials.json.
func TestEnsureDirForOwnsTheAppDirsMode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	appDir := filepath.Join(home, ".zen-linear")

	t.Run("creates it private", func(t *testing.T) {
		dir, err := EnsureDirFor(filepath.Join(appDir, "app.log"))
		if err != nil {
			t.Fatalf("EnsureDirFor() error: %v", err)
		}
		if dir != appDir {
			t.Errorf("dir = %q, want %q", dir, appDir)
		}
		assertDirMode(t, appDir, DirMode)
	})

	t.Run("tightens one an older build left open", func(t *testing.T) {
		if err := os.Chmod(appDir, 0o755); err != nil {
			t.Fatalf("loosen the dir: %v", err)
		}
		if _, err := EnsureDirFor(filepath.Join(appDir, "credentials.json")); err != nil {
			t.Fatalf("EnsureDirFor() error: %v", err)
		}
		assertDirMode(t, appDir, DirMode)
	})
}

// The settings file is often written to $XDG_CONFIG_HOME/zen-linear, which is
// usually a dotfiles checkout. Narrowing a directory that is not ours would be
// a surprise in someone else's repo.
func TestEnsureDirForLeavesTheXDGDirAlone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	xdg := filepath.Join(home, ".config", "zen-linear")
	if err := os.MkdirAll(xdg, 0o755); err != nil {
		t.Fatalf("create the XDG dir: %v", err)
	}

	if _, err := EnsureDirFor(filepath.Join(xdg, "config.json")); err != nil {
		t.Fatalf("EnsureDirFor() error: %v", err)
	}
	assertDirMode(t, xdg, 0o755)
}

func assertDirMode(t *testing.T, dir string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat %s: %v", dir, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s mode = %o, want %o", dir, got, want)
	}
}
