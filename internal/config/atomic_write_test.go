package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomicCreatesParentDirectories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deeper", "file.json")

	if err := WriteFileAtomic(path, []byte("hello\n"), 0o600); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello\n" {
		t.Fatalf("contents = %q, want %q", data, "hello\n")
	}
}

// TestWriteFileAtomicTightensExistingPermissions covers the file an earlier
// build left at 0644: the rename replaces the inode, so the new mode wins.
func TestWriteFileAtomicTightensExistingPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := WriteFileAtomic(path, []byte("new"), 0o600); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions = %o, want 0600", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "new" {
		t.Fatalf("contents = %q, want %q", data, "new")
	}
}

// TestWriteFileAtomicLeavesNoTempFiles verifies the write cleans up after
// itself, so the application directory does not fill with dotfiles.
func TestWriteFileAtomicLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.json")

	for range 3 {
		if err := WriteFileAtomic(path, []byte("payload"), 0o600); err != nil {
			t.Fatalf("WriteFileAtomic: %v", err)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "file.json" {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("directory holds %v, want only file.json", names)
	}
}

func TestWriteFileAtomicRejectsEmptyPath(t *testing.T) {
	if err := WriteFileAtomic("", []byte("payload"), 0o600); err == nil {
		t.Fatal("WriteFileAtomic(\"\") = nil, want an error")
	}
}
