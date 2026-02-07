package main

import (
	"encoding/json"
	"fmt"
	"strings"

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
	event, _ := cmd.Flags().GetString("event")
	autoLoop, _ := cmd.Flags().GetBool("auto-loop")
	maxRetries, _ := cmd.Flags().GetInt("max-retries")
	maxLines, _ := cmd.Flags().GetInt("max-lines")
	filterSQL, _ := cmd.Flags().GetString("filter-sql")

	if maxLines <= 0 {
		maxLines = 10000
	}

	// If --filter-sql is set, fall back to single-source drain against the configured store.
	if filterSQL != "" {
		return runDrainSingleSource(cmd, filterSQL, maxLines, event, autoLoop, maxRetries)
	}

	sources, cleanup, err := resolveDrainSources(cmd)
	if err != nil {
		return fmt.Errorf("resolve drain sources: %w", err)
	}
	defer cleanup()

	multiSource := len(sources) > 1
	var allMessages []store.LogMessage
	var drainErrors []string
	successCount := 0

	// Track remaining message budget. Drain treats its limit as a message
	// count, so the budget uses the same unit. Line-level truncation is
	// handled afterward by TruncateMessages.
	remaining := maxLines

	for _, src := range sources {
		if remaining <= 0 {
			break
		}

		messages, err := src.Store.Drain(src.SessionID, remaining)
		if err != nil {
			if multiSource {
				drainErrors = append(drainErrors, fmt.Sprintf("source %q: %v", src.Label, err))
				fmt.Fprintf(cmd.ErrOrStderr(), "devtap: source %q unreachable: %v\n", src.Label, err)
				continue
			}
			return fmt.Errorf("drain: %w", err)
		}
		successCount++

		if multiSource {
			for i := range messages {
				host := messages[i].Host
				if host == "" {
					host = "unknown"
				}
				messages[i].Tag = fmt.Sprintf("%s/%s | %s", host, src.Label, messages[i].Tag)
			}
		}

		allMessages = append(allMessages, messages...)
		remaining -= len(messages)
	}

	// All sources failed — return error rather than silently succeeding.
	// Exception: Stop+autoLoop should fall through so the user isn't blocked.
	if multiSource && successCount == 0 && len(drainErrors) > 0 {
		if event != "Stop" || !autoLoop {
			return fmt.Errorf("all sources failed: %s", strings.Join(drainErrors, "; "))
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "devtap: all sources failed, allowing stop\n")
	}

	if multiSource && len(allMessages) > 0 {
		allMessages = mcp.DedupMessages(allMessages)
	}

	allMessages = mcp.TruncateMessages(allMessages, maxLines)

	// Handle auto-loop Stop hook (Claude Code specific)
	if event == "Stop" && autoLoop {
		storeDir, err := defaultStoreDir()
		if err != nil {
			return fmt.Errorf("resolve store dir: %w", err)
		}
		// Use the first source's session for retry tracking
		sessionID := sources[0].SessionID
		return handleAutoLoopStop(storeDir, sessionID, allMessages, maxRetries)
	}

	// Reset retry counter on any non-Stop drain (e.g., user submitted a new prompt).
	if event != "" && event != "Stop" {
		if storeDir, err := defaultStoreDir(); err == nil {
			tracker := store.NewRetryTracker(storeDir)
			for _, src := range sources {
				_ = tracker.Reset(src.SessionID)
			}
		}
	}

	if len(allMessages) == 0 {
		return nil
	}

	// Plain text output
	if multiSource {
		var sourceLabels []string
		for _, src := range sources {
			sourceLabels = append(sourceLabels, src.Label)
		}
		fmt.Printf("[devtap] Draining from %d sources: %s\n\n", len(sourceLabels), strings.Join(sourceLabels, ", "))
	}
	fmt.Println(mcp.FormatMessages(allMessages))
	return nil
}

// runDrainSingleSource handles drain with --filter-sql which only works with a single GreptimeDB store.
func runDrainSingleSource(cmd *cobra.Command, filterSQL string, maxLines int, event string, autoLoop bool, maxRetries int) error {
	adapterName, _ := cmd.Flags().GetString("adapter")
	sessionFlag, _ := cmd.Flags().GetString("session")

	sessionID, err := resolveSession(adapterName, sessionFlag)
	if err != nil {
		return fmt.Errorf("resolve session: %w", err)
	}

	s, err := openStore(cmd)
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}
	defer func() { _ = s.Close() }()

	gs, ok := s.(*greptimestore.Store)
	if !ok {
		return fmt.Errorf("--filter-sql requires --store greptimedb")
	}

	messages, err := gs.DrainSQL(sessionID, filterSQL, maxLines)
	if err != nil {
		return fmt.Errorf("drain: %w", err)
	}

	messages = mcp.TruncateMessages(messages, maxLines)

	if event == "Stop" && autoLoop {
		storeDir, err := defaultStoreDir()
		if err != nil {
			return fmt.Errorf("resolve store dir: %w", err)
		}
		return handleAutoLoopStop(storeDir, sessionID, messages, maxRetries)
	}

	if event != "" && event != "Stop" {
		if storeDir, err := defaultStoreDir(); err == nil {
			tracker := store.NewRetryTracker(storeDir)
			_ = tracker.Reset(sessionID)
		}
	}

	if len(messages) == 0 {
		return nil
	}

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
