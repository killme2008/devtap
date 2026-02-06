package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/killme2008/devtap/internal/mcp"
	"github.com/killme2008/devtap/internal/store"
	greptimestore "github.com/killme2008/devtap/internal/store/greptimedb"
)

func drainCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "drain",
		Short: "Consume pending messages and output plain text",
		Long: `Read pending build output and print it as plain text.

Used by lint wrapper scripts (aider) and the Stop hook (auto-loop).
For MCP-capable tools, use "devtap mcp-serve" instead.`,
		SilenceUsage: true,
		RunE:         runDrain,
	}

	cmd.Flags().String("event", "", "hook event name (only needed for Stop auto-loop)")
	cmd.Flags().Bool("auto-loop", false, "enable auto-loop mode (block Stop if errors pending)")
	cmd.Flags().Int("max-retries", 5, "max retries for auto-loop")
	cmd.Flags().Int("max-lines", 10000, "max lines to drain")
	cmd.Flags().String("filter-sql", "", "SQL WHERE clause for GreptimeDB filtering")

	return cmd
}

func runDrain(cmd *cobra.Command, args []string) error {
	adapterName, _ := cmd.Flags().GetString("adapter")
	sessionFlag, _ := cmd.Flags().GetString("session")
	event, _ := cmd.Flags().GetString("event")
	autoLoop, _ := cmd.Flags().GetBool("auto-loop")
	maxRetries, _ := cmd.Flags().GetInt("max-retries")
	maxLines, _ := cmd.Flags().GetInt("max-lines")
	filterSQL, _ := cmd.Flags().GetString("filter-sql")

	sessionID, err := resolveSession(adapterName, sessionFlag)
	if err != nil {
		return fmt.Errorf("resolve session: %w", err)
	}

	s, err := openStore(cmd)
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}
	defer func() { _ = s.Close() }()

	// Drain messages (with SQL filter if using GreptimeDB)
	var messages []store.LogMessage
	if filterSQL != "" {
		if gs, ok := s.(*greptimestore.Store); ok {
			messages, err = gs.DrainSQL(sessionID, filterSQL, maxLines)
		} else {
			return fmt.Errorf("--filter-sql requires --store greptimedb")
		}
	} else {
		messages, err = s.Drain(sessionID, maxLines)
	}
	if err != nil {
		return fmt.Errorf("drain: %w", err)
	}

	messages = mcp.TruncateMessages(messages, maxLines)

	// Handle auto-loop Stop hook (Claude Code specific)
	if event == "Stop" && autoLoop {
		storeDir, err := defaultStoreDir()
		if err != nil {
			return fmt.Errorf("resolve store dir: %w", err)
		}
		return handleAutoLoopStop(storeDir, sessionID, messages, maxRetries)
	}

	// Reset retry counter on any non-Stop drain (e.g., user submitted a new prompt).
	if event != "" && event != "Stop" {
		if storeDir, err := defaultStoreDir(); err == nil {
			tracker := store.NewRetryTracker(storeDir)
			_ = tracker.Reset(sessionID)
		}
	}

	if len(messages) == 0 {
		return nil
	}

	// Plain text output
	fmt.Println(mcp.FormatMessages(messages))
	return nil
}

func handleAutoLoopStop(storeDir, sessionID string, messages []store.LogMessage, maxRetries int) error {
	hasErrors := false
	for _, msg := range messages {
		if msg.ExitCode != nil && *msg.ExitCode != 0 {
			hasErrors = true
			break
		}
	}

	if !hasErrors || len(messages) == 0 {
		// No errors — allow Claude to stop
		fmt.Print("{}")
		return nil
	}

	tracker := store.NewRetryTracker(storeDir)
	count, err := tracker.Get(sessionID)
	if err != nil {
		return fmt.Errorf("get retry count: %w", err)
	}

	if count >= maxRetries {
		// Exceeded retry limit — allow stop
		fmt.Print("{}")
		return nil
	}

	newCount, err := tracker.Increment(sessionID)
	if err != nil {
		return fmt.Errorf("increment retry count: %w", err)
	}

	// Annotate tags with attempt info
	for i := range messages {
		tag := messages[i].Tag
		if tag == "" {
			tag = "build"
		}
		messages[i].Tag = fmt.Sprintf("%s (attempt %d/%d)", tag, newCount, maxRetries)
	}

	reason := mcp.FormatMessages(messages)
	output, err := json.Marshal(map[string]any{
		"decision": "block",
		"reason":   reason,
	})
	if err != nil {
		return fmt.Errorf("marshal stop response: %w", err)
	}

	fmt.Print(string(output))
	return nil
}
