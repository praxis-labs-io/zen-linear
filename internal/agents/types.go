package agents

type AgentRunOptions struct {
	Workspace string

	Model string

	Sandbox string
}

type Provider interface {
	Name() string

	ResolveBinary() (string, bool)

	BuildArgs(prompt string, issueContext string, options AgentRunOptions) []string

	ParseStreamLine(line []byte) (display string, ok bool)
}
