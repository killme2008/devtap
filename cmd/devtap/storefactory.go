package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/killme2008/devtap/internal/config"
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

func defaultStoreDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".devtap"), nil
}
