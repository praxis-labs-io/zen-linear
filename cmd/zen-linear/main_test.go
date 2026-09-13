package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestRunAuthHelp(t *testing.T) {
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	code := run([]string{"auth", "help"})

	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	out := buf.String()
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(out, "auth login") || !strings.Contains(out, "auth logout") {
		t.Fatalf("usage output = %q", out)
	}
}

func TestRunAuthUnknown(t *testing.T) {
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w

	code := run([]string{"auth", "nope"})

	_ = w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	out := buf.String()
	if code != 1 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(out, "Unknown auth command") {
		t.Fatalf("stderr = %q", out)
	}
}

func TestRunVersion(t *testing.T) {
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	code := run([]string{"--version"})

	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if strings.TrimSpace(buf.String()) == "" {
		t.Fatal("expected version output")
	}
}

func TestRunHelpListsEveryCommand(t *testing.T) {
	for _, arg := range []string{"help", "--help", "-h"} {
		oldStdout := os.Stdout
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		os.Stdout = w

		code := run([]string{arg})

		_ = w.Close()
		os.Stdout = oldStdout

		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		out := buf.String()

		if code != 0 {
			t.Fatalf("%s: exit code = %d", arg, code)
		}
		for _, command := range []string{"auth login", "auth logout", "update", "help", "--version"} {
			if !strings.Contains(out, command) {
				t.Fatalf("%s: usage does not name %q: %q", arg, command, out)
			}
		}
	}
}

func TestRunUnknownCommandPrintsUsage(t *testing.T) {
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w

	code := run([]string{"nope"})

	_ = w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	out := buf.String()

	if code != 1 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(out, "Unknown command") {
		t.Fatalf("stderr = %q", out)
	}
	if !strings.Contains(out, "zen-linear update") {
		t.Fatalf("stderr carried no usage: %q", out)
	}
}
