package main

import (
	"time"

	"github.com/spf13/cobra"
)

func rootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "devtap [flags] -- <command> [args...]",
		Short: "Bridge build/dev output to AI coding sessions",
		Long: `devtap captures stdout/stderr from build and development commands,
then feeds them into AI coding tool sessions via MCP (Model Context Protocol).

Supported AI tools: Claude Code, Codex, OpenCode, Gemini CLI, aider.

Quick start:
  devtap install --adapter claude-code   # configure MCP integration
  devtap -- go build ./...               # capture build output
  devtap -- npm run dev                  # capture long-running dev server

Homepage: https://github.com/killme2008/devtap`,
		Version:      version,
		SilenceUsage: true,
		RunE:         runRun, // default action: run a command
	}

	// Global flags
	cmd.PersistentFlags().StringP("adapter", "a", "claude-code", "AI tool adapter (claude-code, codex, opencode, gemini, aider)")
	cmd.PersistentFlags().StringP("session", "s", "auto", `target session ("auto", "pick", or UUID)`)
	cmd.PersistentFlags().String("store", "", `storage backend override ("file" or "greptimedb", default from config)`)

	// Run-specific flags
	cmd.Flags().String("filter-regex", "", "regex filter for output lines")
	cmd.Flags().Bool("filter-invert", false, "invert regex filter (exclude matching lines)")
	cmd.Flags().Int("max-lines", 10000, "max lines per message batch")
	cmd.Flags().String("tag", "", "log tag prefix (default: command name)")
	cmd.Flags().Duration("debounce", 2*time.Second, "flush interval for captured output (0 to disable periodic flush)")

	// Subcommands
	cmd.AddCommand(drainCmd())
	cmd.AddCommand(installCmd())
	cmd.AddCommand(statusCmd())
	cmd.AddCommand(historyCmd())
	cmd.AddCommand(mcpServeCmd())
	cmd.AddCommand(gcCmd())

	return cmd
}
