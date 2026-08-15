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
	if got := getDefaultLogFile(); got != want {
		t.Errorf("getDefaultLogFile() = %q, want %q", got, want)
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
