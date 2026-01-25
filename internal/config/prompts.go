package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AgentPromptTemplate represents a named agent prompt preset.
type AgentPromptTemplate struct {
	Name   string `json:"name"`
	Prompt string `json:"prompt"`
}

// DefaultAgentPromptTemplates returns the built-in agent prompt templates.
func DefaultAgentPromptTemplates() []AgentPromptTemplate {
	return []AgentPromptTemplate{
		{
			Name:   "Create a plan",
			Prompt: "Create a plan for the selected Linear issue. Outline approach, steps, risks, and tests.",
		},
		{
			Name:   "Explore and research",
			Prompt: "Explore the codebase for the selected Linear issue. Summarize relevant files, behaviors, and open questions.",
		},
		{
			Name:   "Implement",
			Prompt: "Implement the selected Linear issue. Make focused changes and outline any tests to run.",
		},
	}
}

// PromptTemplatesFilePath returns the default prompts file path.
func PromptTemplatesFilePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".linear-tui", "prompts.json"), nil
}

// EnsurePromptTemplatesFile ensures the prompts file exists and returns its templates.
func EnsurePromptTemplatesFile(path string) ([]AgentPromptTemplate, error) {
	if path == "" {
		return nil, fmt.Errorf("prompts path is empty")
	}

	if _, err := os.Stat(path); err == nil {
		return LoadPromptTemplates(path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("stat prompts file: %w", err)
	}

	templates := DefaultAgentPromptTemplates()
	if err := SavePromptTemplates(path, templates); err != nil {
		return nil, err
	}

	return templates, nil
}

// LoadPromptTemplates loads prompt templates from a JSON file and validates them.
func LoadPromptTemplates(path string) ([]AgentPromptTemplate, error) {
	if path == "" {
		return nil, fmt.Errorf("prompts path is empty")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read prompts file: %w", err)
	}

	var templates []AgentPromptTemplate
	if err := json.Unmarshal(data, &templates); err != nil {
		return nil, fmt.Errorf("parse prompts file: %w", err)
	}

	valid := normalizePromptTemplates(templates)
	if len(valid) == 0 {
		return DefaultAgentPromptTemplates(), nil
	}

	return valid, nil
}

// SavePromptTemplates writes prompt templates to a JSON file, creating directories as needed.
func SavePromptTemplates(path string, templates []AgentPromptTemplate) error {
	if path == "" {
		return fmt.Errorf("prompts path is empty")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create prompts directory: %w", err)
	}

	data, err := json.MarshalIndent(templates, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal prompts: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write prompts file: %w", err)
	}

	return nil
}

// normalizePromptTemplates trims and filters templates to ensure required fields are present.
func normalizePromptTemplates(templates []AgentPromptTemplate) []AgentPromptTemplate {
	valid := make([]AgentPromptTemplate, 0, len(templates))
	for _, template := range templates {
		name := strings.TrimSpace(template.Name)
		prompt := strings.TrimSpace(template.Prompt)
		if name == "" || prompt == "" {
			continue
		}
		valid = append(valid, AgentPromptTemplate{
			Name:   name,
			Prompt: prompt,
		})
	}
	return valid
}
