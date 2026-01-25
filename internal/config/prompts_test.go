package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestEnsurePromptTemplatesFileCreatesDefaults verifies missing prompts are created with defaults.
func TestEnsurePromptTemplatesFileCreatesDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	promptsPath := filepath.Join(tmpDir, "nested", "prompts.json")

	templates, err := EnsurePromptTemplatesFile(promptsPath)
	if err != nil {
		t.Fatalf("EnsurePromptTemplatesFile() error: %v", err)
	}

	if _, err := os.Stat(promptsPath); err != nil {
		t.Fatalf("prompts file not created: %v", err)
	}

	if _, err := os.Stat(filepath.Dir(promptsPath)); err != nil {
		t.Fatalf("prompts directory not created: %v", err)
	}

	assertPromptTemplatesEqual(t, templates, DefaultAgentPromptTemplates())
}

// TestLoadPromptTemplatesFiltersInvalid verifies invalid entries are dropped.
func TestLoadPromptTemplatesFiltersInvalid(t *testing.T) {
	tmpDir := t.TempDir()
	promptsPath := filepath.Join(tmpDir, "prompts.json")

	data := []byte(`[
  {"name": "  ", "prompt": "ignored"},
  {"name": "Valid", "prompt": "  Use this  "}
]`)
	if err := os.WriteFile(promptsPath, data, 0644); err != nil {
		t.Fatalf("write prompts file: %v", err)
	}

	templates, err := LoadPromptTemplates(promptsPath)
	if err != nil {
		t.Fatalf("LoadPromptTemplates() error: %v", err)
	}

	expected := []AgentPromptTemplate{{Name: "Valid", Prompt: "Use this"}}
	assertPromptTemplatesEqual(t, templates, expected)
}

// TestLoadPromptTemplatesEmptyFallback verifies empty templates fall back to defaults.
func TestLoadPromptTemplatesEmptyFallback(t *testing.T) {
	tmpDir := t.TempDir()
	promptsPath := filepath.Join(tmpDir, "prompts.json")

	if err := os.WriteFile(promptsPath, []byte(`[]`), 0644); err != nil {
		t.Fatalf("write prompts file: %v", err)
	}

	templates, err := LoadPromptTemplates(promptsPath)
	if err != nil {
		t.Fatalf("LoadPromptTemplates() error: %v", err)
	}

	assertPromptTemplatesEqual(t, templates, DefaultAgentPromptTemplates())
}

// assertPromptTemplatesEqual compares prompt template values in tests.
func assertPromptTemplatesEqual(t *testing.T, got []AgentPromptTemplate, want []AgentPromptTemplate) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Prompt templates mismatch: got %+v, want %+v", got, want)
	}
}
