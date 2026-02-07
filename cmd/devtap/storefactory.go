package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/killme2008/devtap/internal/config"
	"github.com/killme2008/devtap/internal/mcp"
	"github.com/killme2008/devtap/internal/store"
	filestore "github.com/killme2008/devtap/internal/store/file"
	greptimestore "github.com/killme2008/devtap/internal/store/greptimedb"
)

// openStore creates a Store based on CLI flags and config file.
func openStore(cmd *cobra.Command) (store.Store, error) {
	storeDir, err := defaultStoreDir()
	if err != nil {
		return nil, err
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	// CLI flag overrides config file
	backend := cfg.Store.Backend
	if override, _ := cmd.Flags().GetString("store"); override != "" {
		backend = override
	}

	adapter, _ := cmd.Flags().GetString("adapter")
	if adapter == "" {
		adapter = "claude-code"
	}

	return openStoreByBackend(cfg, backend, storeDir, adapter)
}

// openStoreByBackend opens a store for the given backend name.
func openStoreByBackend(cfg *config.Config, backend, storeDir, adapter string) (store.Store, error) {
	switch backend {
	case "", "file":
		return filestore.New(storeDir, adapter)
	case "greptimedb":
		gs, err := greptimestore.New(cfg.Store.GreptimeDB, storeDir, adapter)
		if err != nil {
			fmt.Fprintf(os.Stderr, "devtap: greptimedb unavailable (%v), falling back to file store\n", err)
			return filestore.New(storeDir, adapter)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := gs.Ping(ctx); err != nil {
			_ = gs.Close()
			fmt.Fprintf(os.Stderr, "devtap: greptimedb unavailable (%v), falling back to file store\n", err)
			return filestore.New(storeDir, adapter)
		}
		return gs, nil
	default:
		return nil, fmt.Errorf("unknown store backend: %q (use \"file\" or \"greptimedb\")", backend)
	}
}

// openStoreStrict opens a store without fallback. Unlike openStoreByBackend,
// a greptimedb failure returns an error instead of silently falling back to
// the file store. This is appropriate for reader paths (drain / MCP) where
// falling back would silently drain the wrong data.
func openStoreStrict(cfg *config.Config, backend, storeDir, adapter string) (store.Store, error) {
	switch backend {
	case "", "file":
		return filestore.New(storeDir, adapter)
	case "greptimedb":
		gs, err := greptimestore.New(cfg.Store.GreptimeDB, storeDir, adapter)
		if err != nil {
			return nil, fmt.Errorf("greptimedb: %w", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := gs.Ping(ctx); err != nil {
			_ = gs.Close()
			return nil, fmt.Errorf("greptimedb ping: %w", err)
		}
		return gs, nil
	default:
		return nil, fmt.Errorf("unknown store backend: %q", backend)
	}
}

// resolveDrainSources returns 1 or 2 drain sources for the MCP server and drain command.
//
// - "configured" source: from CLI --store/--session flags (or global config defaults)
// - "local" source: from global config + auto-detected session
//
// If they resolve to the same (backend, session), returns a single source.
// When both sources use the same backend, they share one store instance.
func resolveDrainSources(cmd *cobra.Command) ([]mcp.DrainSource, func(), error) {
	storeDir, err := defaultStoreDir()
	if err != nil {
		return nil, nil, err
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("load config: %w", err)
	}

	adapterName, _ := cmd.Flags().GetString("adapter")
	if adapterName == "" {
		adapterName = "claude-code"
	}

	storeFlag, _ := cmd.Flags().GetString("store")
	sessionFlag, _ := cmd.Flags().GetString("session")

	// Resolve "configured" source: CLI flags take precedence.
	configuredBackend := cfg.Store.Backend
	if storeFlag != "" {
		configuredBackend = storeFlag
	}

	configuredSession := sessionFlag
	if configuredSession == "" || configuredSession == "auto" || configuredSession == "pick" {
		resolved, err := resolveSession(adapterName, configuredSession)
		if err != nil || resolved == "" {
			configuredSession = "default"
		} else {
			configuredSession = resolved
		}
	}

	// Use strict open for the configured store so that a greptimedb failure
	// is surfaced rather than silently falling back to the local file store
	// (which would drain wrong data). On failure, degrade to local-only.
	configuredStore, configuredErr := openStoreStrict(cfg, configuredBackend, storeDir, adapterName)

	// Resolve "local" source: always use global config backend + auto-detected session.
	localBackend := cfg.Store.Backend
	localSession, err := resolveSession(adapterName, "auto")
	if err != nil || localSession == "" {
		localSession = "default"
	}

	// Normalize backends for comparison: "" and "file" are the same.
	normBackend := func(b string) string {
		if b == "" {
			return "file"
		}
		return b
	}

	// Configured store unavailable — fall back to local-only single source.
	if configuredErr != nil {
		fmt.Fprintf(os.Stderr, "devtap: configured store %q unavailable (%v), using local only\n",
			configuredBackend, configuredErr)
		localStore, err := openStoreByBackend(cfg, localBackend, storeDir, adapterName)
		if err != nil {
			return nil, nil, fmt.Errorf("open local store: %w", err)
		}
		cleanup := func() { _ = localStore.Close() }
		return []mcp.DrainSource{
			{Store: localStore, SessionID: localSession, Label: localSession},
		}, cleanup, nil
	}

	// If both sources resolve to the same (backend, session), single source.
	if normBackend(configuredBackend) == normBackend(localBackend) && configuredSession == localSession {
		cleanup := func() { _ = configuredStore.Close() }
		return []mcp.DrainSource{
			{Store: configuredStore, SessionID: configuredSession, Label: configuredSession},
		}, cleanup, nil
	}

	// Two distinct sources. Open local store (share instance if same backend).
	var localStore store.Store
	if normBackend(configuredBackend) == normBackend(localBackend) {
		localStore = configuredStore
	} else {
		localStore, err = openStoreByBackend(cfg, localBackend, storeDir, adapterName)
		if err != nil {
			_ = configuredStore.Close()
			return nil, nil, fmt.Errorf("open local store: %w", err)
		}
	}

	cleanup := func() {
		_ = configuredStore.Close()
		if localStore != configuredStore {
			_ = localStore.Close()
		}
	}

	return []mcp.DrainSource{
		{Store: localStore, SessionID: localSession, Label: "local"},
		{Store: configuredStore, SessionID: configuredSession, Label: configuredSession},
	}, cleanup, nil
}

func defaultStoreDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".devtap"), nil
}
