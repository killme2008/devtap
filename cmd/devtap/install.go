package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/killme2008/devtap/internal/adapter"
)

func installCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Configure AI tool integration with devtap",
		Long: `Set up devtap integration for the specified AI coding tool.

For MCP-capable tools (Claude Code, Codex), this writes .mcp.json in the
project directory. For aider, this creates a lint wrapper script.`,
		SilenceUsage: true,
		RunE:         runInstall,
	}

	cmd.Flags().Bool("auto-loop", false, "configure Stop hook for auto-loop mode (Claude Code only)")
	cmd.Flags().Int("max-retries", 5, "max retries for auto-loop")

	return cmd
}

func runInstall(cmd *cobra.Command, args []string) error {
	adapterName, _ := cmd.Flags().GetString("adapter")
	autoLoop, _ := cmd.Flags().GetBool("auto-loop")
	maxRetries, _ := cmd.Flags().GetInt("max-retries")
	sessionFlag, _ := cmd.Flags().GetString("session")
	storeFlag, _ := cmd.Flags().GetString("store")

	projectDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	var extraArgs []string
	if sessionFlag != "" && sessionFlag != "auto" {
		extraArgs = append(extraArgs, "--session", sessionFlag)
	}
	if storeFlag != "" {
		extraArgs = append(extraArgs, "--store", storeFlag)
	}

	config := adapter.InstallConfig{
		ProjectDir: projectDir,
		AutoLoop:   autoLoop,
		MaxRetries: maxRetries,
		ExtraArgs:  extraArgs,
	}

	a := getAdapter(adapterName)
	if err := a.Install(config); err != nil {
		return fmt.Errorf("install: %w", err)
	}

	switch adapterName {
	case "claude-code":
		fmt.Println("Installed devtap MCP server for Claude Code.")
		fmt.Println("Created .mcp.json in project directory.")
		if autoLoop {
			fmt.Println("Auto-loop Stop hook configured in ~/.claude/settings.json")
		}
		fmt.Println("\nClaude Code will automatically discover the MCP server on next session.")

	case "codex":
		fmt.Println("Installed devtap MCP server for Codex CLI.")
		fmt.Println("Created .codex/config.toml with devtap MCP server.")
		fmt.Println("\nCodex CLI will automatically discover the MCP server on next session.")

	case "opencode":
		fmt.Println("Installed devtap MCP server for OpenCode.")
		fmt.Println("Created opencode.json with devtap MCP server.")
		fmt.Println("\nOpenCode will automatically discover the MCP server on next session.")

	case "gemini":
		fmt.Println("Installed devtap MCP server for Gemini CLI.")
		fmt.Println("Created .gemini/settings.json with devtap MCP server.")
		fmt.Println("\nGemini CLI will automatically discover the MCP server on next session.")

	case "aider":
		fmt.Println("Installed devtap integration for aider.")
		fmt.Println("Created .devtap-aider-lint.sh")
		fmt.Println("\nUsage:")
		fmt.Println("  aider --lint-cmd \"sh .devtap-aider-lint.sh\" --auto-lint")
	}

	return nil
}
