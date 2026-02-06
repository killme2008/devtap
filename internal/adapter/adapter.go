package adapter

// Session represents an active AI coding tool session.
type Session struct {
	ID         string
	ProjectDir string
	Label      string // human-readable label for display
}

// InstallConfig holds configuration for installing devtap integration.
type InstallConfig struct {
	ProjectDir string
	AutoLoop   bool
	MaxRetries int
}

// Adapter abstracts the integration with different AI coding tools.
type Adapter interface {
	// Name returns the adapter identifier (e.g., "claude-code", "codex", "aider").
	Name() string

	// DiscoverSessions finds active sessions for this tool in the given project dir.
	DiscoverSessions(projectDir string) ([]Session, error)

	// Install configures the tool to integrate with devtap.
	// For MCP-capable tools, this writes .mcp.json in the project dir.
	// For tools without MCP, this creates tool-specific integration files.
	Install(config InstallConfig) error
}
