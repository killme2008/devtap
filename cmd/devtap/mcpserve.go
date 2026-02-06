package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/killme2008/devtap/internal/mcp"
	"github.com/killme2008/devtap/internal/store"
)

func mcpServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp-serve",
		Short: "Start MCP stdio server for AI tool integration",
		Long: `Start an MCP (Model Context Protocol) server over stdio.

This allows any MCP-compatible AI tool (Codex CLI, Claude Code, etc.) to
query devtap for build errors and status via standard MCP tool calls.

Exposed tools:
  get_build_errors   Drain pending build errors
  get_build_status   Get pending message counts per session`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore(cmd)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer func() { _ = s.Close() }()

			adapterName, _ := cmd.Flags().GetString("adapter")
			if adapterName == "" {
				adapterName = "claude-code"
			}
			sessionID, _ := cmd.Flags().GetString("session")
			maxLines, _ := cmd.Flags().GetInt("max-lines")
			if maxLines <= 0 {
				maxLines = 10000
			}

			// If session is "auto", use a default based on cwd
			if sessionID == "" || sessionID == "auto" {
				resolved, err := resolveSession(adapterName, "auto")
				if err != nil || resolved == "" {
					sessionID = "default"
				} else {
					sessionID = resolved
				}
			}

			// Pre-register adapter dir so writers can discover and fan-out to us.
			if storeDir, err := defaultStoreDir(); err == nil {
				_ = store.EnsureAdapterDir(storeDir, sessionID, adapterName)
			}

			srv := mcp.NewServer(s, sessionID, maxLines)
			return srv.Run()
		},
	}

	cmd.Flags().Int("max-lines", 10000, "max lines to return per drain")

	return cmd
}
