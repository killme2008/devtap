package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	greptimestore "github.com/killme2008/devtap/internal/store/greptimedb"
)

func historyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Query historical build error trends (GreptimeDB only)",
		Long: `Displays historical build error data from GreptimeDB.
Requires --store greptimedb or store.backend = "greptimedb" in config.`,
		SilenceUsage: true,
		RunE:         runHistory,
	}

	cmd.Flags().String("since", "24h", "time range to query (e.g., 1h, 24h, 7d)")
	cmd.Flags().String("tag", "", "filter by tag")
	cmd.Flags().Int("limit", 20, "max entries to show")

	return cmd
}

func runHistory(cmd *cobra.Command, args []string) error {
	since, _ := cmd.Flags().GetString("since")
	tag, _ := cmd.Flags().GetString("tag")
	limit, _ := cmd.Flags().GetInt("limit")
	sessionFlag, _ := cmd.Flags().GetString("session")
	adapterName, _ := cmd.Flags().GetString("adapter")

	s, err := openStore(cmd)
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}
	defer func() { _ = s.Close() }()

	gs, ok := s.(*greptimestore.Store)
	if !ok {
		return fmt.Errorf("history command requires --store greptimedb")
	}

	sessionID, err := resolveSession(adapterName, sessionFlag)
	if err != nil {
		return fmt.Errorf("resolve session: %w", err)
	}

	duration, err := parseDuration(since)
	if err != nil {
		return fmt.Errorf("invalid --since value %q: %w", since, err)
	}

	entries, err := gs.History(sessionID, tag, duration, limit)
	if err != nil {
		return fmt.Errorf("query history: %w", err)
	}

	if len(entries) == 0 {
		fmt.Println("No history entries found.")
		return nil
	}

	fmt.Printf("Build history (last %s, adapter: %s):\n\n", since, adapterName)
	fmt.Printf("  %-20s %-15s %-6s  %s\n", "TIME", "TAG", "EXIT", "SUMMARY")
	fmt.Printf("  %-20s %-15s %-6s  %s\n", strings.Repeat("-", 20), strings.Repeat("-", 15), strings.Repeat("-", 6), strings.Repeat("-", 40))

	for _, e := range entries {
		exitStr := fmt.Sprintf("%d", e.ExitCode)
		if e.ExitCode == 0 {
			exitStr = "ok"
		}

		summary := e.Summary
		if len(summary) > 60 {
			summary = summary[:57] + "..."
		}

		fmt.Printf("  %-20s %-15s %-6s  %s\n",
			e.Timestamp.Format("2006-01-02 15:04:05"),
			e.Tag, exitStr, summary)
	}

	return nil
}

// parseDuration parses duration strings like "1h", "24h", "7d".
func parseDuration(s string) (time.Duration, error) {
	// Handle day suffix which time.ParseDuration doesn't support
	if strings.HasSuffix(s, "d") {
		days := strings.TrimSuffix(s, "d")
		var d int
		if _, err := fmt.Sscanf(days, "%d", &d); err != nil {
			return 0, fmt.Errorf("invalid day format: %s", s)
		}
		return time.Duration(d) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
