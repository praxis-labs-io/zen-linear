package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

const (
	InstallScriptURL = "https://raw.githubusercontent.com/praxis-labs-io/zen-linear/main/install.sh"

	InstallScriptWindowsURL = "https://raw.githubusercontent.com/praxis-labs-io/zen-linear/main/install.ps1"

	DevVersion = devVersion

	maxScriptBytes = 1 << 20

	scriptTimeout = 30 * time.Second
)

type InstallRunner func(ctx context.Context, script, dir string, out io.Writer) error

// InstallOptions is what an install needs. Only Dir is required.
type InstallOptions struct {
	Dir string
	// Out nil discards the installer's output.
	Out io.Writer
	// ScriptURL empty means this platform's installer.
	ScriptURL string
	Client    *http.Client
	Runner    InstallRunner
}

// Install fetches the platform's published installer and runs it with
// INSTALL_DIR set to Dir.
func Install(ctx context.Context, opts InstallOptions) error {
	if opts.Dir == "" {
		return errors.New("install directory is empty")
	}

	script, err := fetchInstallScript(ctx, opts)
	if err != nil {
		return err
	}

	path, cleanup, err := stageScript(runtime.GOOS, script)
	if err != nil {
		return err
	}
	defer cleanup()

	run := opts.Runner
	if run == nil {
		run = runInstallScript
	}

	return run(ctx, path, opts.Dir, opts.Out)
}

func installScriptURL(goos string) string {
	if goos == "windows" {
		return InstallScriptWindowsURL
	}
	return InstallScriptURL
}

func fetchInstallScript(ctx context.Context, opts InstallOptions) ([]byte, error) {
	endpoint := opts.ScriptURL
	if endpoint == "" {
		endpoint = installScriptURL(runtime.GOOS)
	}
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: scriptTimeout}
	}

	ctx, cancel := context.WithTimeout(ctx, scriptTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build the installer request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download the installer: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the installer download answered %s", resp.Status)
	}

	script, err := io.ReadAll(io.LimitReader(resp.Body, maxScriptBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read the installer: %w", err)
	}
	if len(script) == 0 {
		return nil, errors.New("the installer download was empty")
	}
	if len(script) > maxScriptBytes {
		return nil, fmt.Errorf("the installer is larger than %d bytes", maxScriptBytes)
	}

	return script, nil
}

// PowerShell refuses -File on a path that is not .ps1, so the extension matters.
func stageScript(goos string, script []byte) (string, func(), error) {
	pattern := "zen-linear-install-*.sh"
	if goos == "windows" {
		pattern = "zen-linear-install-*.ps1"
	}

	file, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", nil, fmt.Errorf("stage the installer: %w", err)
	}
	path := file.Name()
	cleanup := func() { _ = os.Remove(path) }

	if _, err := file.Write(script); err != nil {
		_ = file.Close()
		cleanup()
		return "", nil, fmt.Errorf("stage the installer: %w", err)
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("stage the installer: %w", err)
	}

	return path, cleanup, nil
}

func installerArgs(goos, script string) (string, []string) {
	if goos == "windows" {
		return "powershell", []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script}
	}
	return "sh", []string{script}
}

func runInstallScript(ctx context.Context, script, dir string, out io.Writer) error {
	name, args := installerArgs(runtime.GOOS, script)

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "INSTALL_DIR="+dir, "VERSION=")
	cmd.Stdout = out
	cmd.Stderr = out

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run the installer: %w", err)
	}

	return nil
}

// InstallDir returns the running binary's directory, symlinks resolved.
func InstallDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve the running binary: %w", err)
	}

	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("resolve the running binary: %w", err)
	}

	return filepath.Dir(resolved), nil
}
