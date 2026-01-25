package agents

// AgentRunOptions captures optional overrides for agent runs.
type AgentRunOptions struct {
	// Workspace overrides the working directory and CLI workspace flag.
	Workspace string

	// Model selects a provider-specific model, when supported.
	Model string

	// Sandbox configures sandboxing for providers that support it.
	Sandbox string
}

// Provider defines how to invoke and interpret a terminal agent CLI.
type Provider interface {
	// Name returns a short display name for the provider.
	Name() string

	// ResolveBinary returns the executable path and whether it is available.
	ResolveBinary() (string, bool)

	// BuildArgs returns the argv for a non-interactive run.
	BuildArgs(prompt string, issueContext string, options AgentRunOptions) []string

	// ParseStreamLine attempts to extract display text from a stream-json line.
	ParseStreamLine(line []byte) (display string, ok bool)
}
