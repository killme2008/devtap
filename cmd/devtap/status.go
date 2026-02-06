package main

import (
	"fmt"

	"github.com/spf13/cobra"

	greptimestore "github.com/killme2008/devtap/internal/store/greptimedb"
)

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "status",
		Short:        "Show pending message counts per session",
		SilenceUsage: true,
		RunE:         runStatus,
	}
}

func runStatus(cmd *cobra.Command, args []string) error {
	s, err := openStore(cmd)
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}
	defer func() { _ = s.Close() }()

	adapter, _ := cmd.Flags().GetString("adapter")
	if adapter == "" {
		adapter = "claude-code"
	}

	// Try extended status for GreptimeDB
	if gs, ok := s.(*greptimestore.Store); ok {
		return showExtendedStatus(gs, adapter)
	}

	// File store: basic counts
	counts, err := s.Status()
	if err != nil {
		return fmt.Errorf("get status: %w", err)
	}

	if len(counts) == 0 {
		fmt.Println("No pending messages.")
		return nil
	}

	fmt.Printf("Pending messages (adapter: %s):\n", adapter)
	for session, count := range counts {
		fmt.Printf("  %s: %d message(s)\n", session, count)
	}
	return nil
}

func showExtendedStatus(gs *greptimestore.Store, adapter string) error {
	// Show pending counts first
	counts, err := gs.Status()
	if err != nil {
		return fmt.Errorf("get status: %w", err)
	}

	if len(counts) == 0 {
		fmt.Println("No pending messages.")
	} else {
		fmt.Printf("Pending messages (adapter: %s):\n", adapter)
		for session, count := range counts {
			fmt.Printf("  %s: %d message(s)\n", session, count)
		}
	}

	// Show extended stats
	stats, err := gs.StatusExtended()
	if err != nil {
		return nil // non-fatal, just skip extended info
	}

	if len(stats) > 0 {
		fmt.Println("\nRecent activity (GreptimeDB):")
		for _, st := range stats {
			fmt.Printf("  session=%s  tag=%s  adapter=%s  total=%d  errors=%d  last_seen=%s\n",
				st.SessionID, st.Tag, st.Adapter, st.Total, st.Errors,
				st.LastSeen.Format("15:04:05"))
		}
	}

	return nil
}
