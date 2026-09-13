package update

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstallerArgsPerPlatform(t *testing.T) {
	tests := []struct {
		goos   string
		script string
		name   string
		args   []string
	}{
		{
			goos:   "darwin",
			script: "/tmp/install.sh",
			name:   "sh",
			args:   []string{"/tmp/install.sh"},
		},
		{
			goos:   "linux",
			script: "/tmp/install.sh",
			name:   "sh",
			args:   []string{"/tmp/install.sh"},
		},
		{
			goos:   "windows",
			script: `C:\Temp\install.ps1`,
			name:   "powershell",
			args:   []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", `C:\Temp\install.ps1`},
		},
	}

	for _, tc := range tests {
		t.Run(tc.goos, func(t *testing.T) {
			name, args := installerArgs(tc.goos, tc.script)
			if name != tc.name {
				t.Fatalf("command = %q, want %q", name, tc.name)
			}
			if strings.Join(args, " ") != strings.Join(tc.args, " ") {
				t.Fatalf("args = %v, want %v", args, tc.args)
			}
		})
	}
}

func TestInstallScriptURLPerPlatform(t *testing.T) {
	if got := installScriptURL("windows"); got != InstallScriptWindowsURL {
		t.Fatalf("windows url = %q", got)
	}
	if got := installScriptURL("darwin"); got != InstallScriptURL {
		t.Fatalf("darwin url = %q", got)
	}
}

func TestStagedScriptCarriesThePlatformExtension(t *testing.T) {
	for goos, want := range map[string]string{"darwin": ".sh", "windows": ".ps1"} {
		path, cleanup, err := stageScript(goos, []byte("echo hi\n"))
		if err != nil {
			t.Fatalf("%s: %v", goos, err)
		}
		defer cleanup()

		if filepath.Ext(path) != want {
			t.Fatalf("%s: staged %q, want a %s file", goos, path, want)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != "echo hi\n" {
			t.Fatalf("%s: staged body = %q", goos, body)
		}
	}
}

func TestStagedScriptIsRemovedOnCleanup(t *testing.T) {
	path, cleanup, err := stageScript("darwin", []byte("echo hi\n"))
	if err != nil {
		t.Fatal(err)
	}
	cleanup()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("staged script still present at %s", path)
	}
}

func TestInstallHandsTheRunnerTheStagedScriptAndDirectory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("#!/bin/sh\nexit 0\n"))
	}))
	defer server.Close()

	var gotScript, gotDir string
	err := Install(context.Background(), InstallOptions{
		Dir:       "/opt/bin",
		ScriptURL: server.URL,
		Runner: func(_ context.Context, script, dir string, _ io.Writer) error {
			gotScript, gotDir = script, dir
			body, readErr := os.ReadFile(script)
			if readErr != nil {
				t.Errorf("staged script unreadable: %v", readErr)
			}
			if string(body) != "#!/bin/sh\nexit 0\n" {
				t.Errorf("staged body = %q", body)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	if gotDir != "/opt/bin" {
		t.Fatalf("INSTALL_DIR = %q, want /opt/bin", gotDir)
	}
	if gotScript == "" {
		t.Fatal("the runner was handed no script")
	}
	if _, err := os.Stat(gotScript); !os.IsNotExist(err) {
		t.Fatalf("staged script outlived the install at %s", gotScript)
	}
}

func TestInstallRefusesWhatItCouldNotDownload(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    string
	}{
		{
			name:    "not found",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) },
			want:    "404",
		},
		{
			name:    "empty body",
			handler: func(w http.ResponseWriter, _ *http.Request) {},
			want:    "empty",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(tc.handler)
			defer server.Close()

			ran := false
			err := Install(context.Background(), InstallOptions{
				Dir:       "/opt/bin",
				ScriptURL: server.URL,
				Runner: func(context.Context, string, string, io.Writer) error {
					ran = true
					return nil
				},
			})
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want it to mention %q", err, tc.want)
			}
			if ran {
				t.Fatal("the installer ran on a download that failed")
			}
		})
	}
}

func TestInstallRefusesAnEmptyDirectory(t *testing.T) {
	err := Install(context.Background(), InstallOptions{})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "install directory") {
		t.Fatalf("error = %v", err)
	}
}

func TestInstallDirIsTheRunningBinarysDirectory(t *testing.T) {
	dir, err := InstallDir()
	if err != nil {
		t.Fatalf("InstallDir: %v", err)
	}
	if !filepath.IsAbs(dir) {
		t.Fatalf("InstallDir = %q, want an absolute path", dir)
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Dir(resolved) {
		t.Fatalf("InstallDir = %q, want %q", dir, filepath.Dir(resolved))
	}
}

func TestTheInstallerSeesTheDirectoryAndNoPinnedVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the staged script is sh")
	}

	out := filepath.Join(t.TempDir(), "env")
	t.Setenv("ZEN_TEST_OUT", out)
	t.Setenv("VERSION", "v9.9.9")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("#!/bin/sh\nprintf '%s|%s' \"$INSTALL_DIR\" \"${VERSION:-}\" > \"$ZEN_TEST_OUT\"\n"))
	}))
	defer server.Close()

	if err := Install(context.Background(), InstallOptions{
		Dir:       "/opt/bin",
		ScriptURL: server.URL,
	}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	seen, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(seen) != "/opt/bin|" {
		t.Fatalf("the installer saw %q, want INSTALL_DIR set and VERSION emptied", seen)
	}
}

func TestAFailedInstallerIsReported(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the staged script is sh")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("#!/bin/sh\necho nope >&2\nexit 1\n"))
	}))
	defer server.Close()

	err := Install(context.Background(), InstallOptions{Dir: "/opt/bin", ScriptURL: server.URL})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "run the installer") {
		t.Fatalf("error = %v", err)
	}
}

func TestInstallRefusesAnOversizedScript(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, maxScriptBytes+1))
	}))
	defer server.Close()

	ran := false
	err := Install(context.Background(), InstallOptions{
		Dir:       "/opt/bin",
		ScriptURL: server.URL,
		Runner: func(context.Context, string, string, io.Writer) error {
			ran = true
			return nil
		},
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("error = %v", err)
	}
	if ran {
		t.Fatal("a truncated installer was executed")
	}
}
