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
			maxLines, _ := cmd.Flags().GetInt("max-lines")
			if maxLines <= 0 {
				maxLines = 10000
			}

			adapterName, _ := cmd.Flags().GetString("adapter")
			if adapterName == "" {
				adapterName = "claude-code"
			}

			sources, cleanup, err := resolveDrainSources(cmd)
			if err != nil {
				return fmt.Errorf("resolve drain sources: %w", err)
			}
			defer cleanup()

			// Pre-register adapter dir for each source so writers can discover us.
			if storeDir, err := defaultStoreDir(); err == nil {
				for _, src := range sources {
					_ = store.EnsureAdapterDir(storeDir, src.SessionID, adapterName)
				}
			}

			srv := mcp.NewMultiSourceServer(sources, maxLines)
			return srv.Run()
		},
	}

	cmd.Flags().Int("max-lines", 10000, "max lines to return per drain")

	return cmd
}
