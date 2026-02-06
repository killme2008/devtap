package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

func gcCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gc",
		Short: "Remove expired session data",
		Long: `Scans ~/.devtap/ and removes session/adapter directories
where all files are older than the specified TTL.
Empty session directories are cleaned up as well.`,
		SilenceUsage: true,
		RunE:         runGC,
	}

	cmd.Flags().String("ttl", "7d", "time-to-live for session data (e.g., 1h, 24h, 7d)")

	return cmd
}

func runGC(cmd *cobra.Command, args []string) error {
	ttlStr, _ := cmd.Flags().GetString("ttl")
	ttl, err := parseDuration(ttlStr)
	if err != nil {
		return fmt.Errorf("invalid --ttl value %q: %w", ttlStr, err)
	}

	storeDir, err := defaultStoreDir()
	if err != nil {
		return err
	}

	cutoff := time.Now().Add(-ttl)
	return gcStoreDir(storeDir, cutoff)
}

// gcStoreDir removes adapter directories where all files are older than cutoff,
// then removes empty session directories.
func gcStoreDir(storeDir string, cutoff time.Time) error {
	sessions, err := os.ReadDir(storeDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("Nothing to clean up.")
			return nil
		}
		return fmt.Errorf("read store dir: %w", err)
	}

	var removedDirs int
	for _, sessionEntry := range sessions {
		if !sessionEntry.IsDir() {
			continue
		}
		sessionPath := filepath.Join(storeDir, sessionEntry.Name())

		adapters, err := os.ReadDir(sessionPath)
		if err != nil {
			continue
		}

		for _, adapterEntry := range adapters {
			if !adapterEntry.IsDir() {
				// Remove stale non-directory files (e.g. retry_count)
				// that live at the session level alongside adapter dirs.
				filePath := filepath.Join(sessionPath, adapterEntry.Name())
				if info, err := adapterEntry.Info(); err == nil && info.ModTime().Before(cutoff) {
					_ = os.Remove(filePath)
				}
				continue
			}
			adapterPath := filepath.Join(sessionPath, adapterEntry.Name())
			if allFilesOlderThan(adapterPath, cutoff) {
				if err := os.RemoveAll(adapterPath); err == nil {
					removedDirs++
					fmt.Printf("  removed %s/%s\n", sessionEntry.Name(), adapterEntry.Name())
				}
			}
		}

		// Clean up empty session directory.
		remaining, _ := os.ReadDir(sessionPath)
		if len(remaining) == 0 {
			_ = os.Remove(sessionPath)
		}
	}

	if removedDirs == 0 {
		fmt.Println("Nothing to clean up.")
	} else {
		fmt.Printf("Removed %d expired adapter dir(s).\n", removedDirs)
	}
	return nil
}

// allFilesOlderThan returns true if every file in dir (recursively) has a
// modification time before cutoff. Returns true for empty directories.
func allFilesOlderThan(dir string, cutoff time.Time) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	if len(entries) == 0 {
		return true
	}
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if e.IsDir() {
			if !allFilesOlderThan(path, cutoff) {
				return false
			}
			continue
		}
		info, err := e.Info()
		if err != nil {
			return false
		}
		if !info.ModTime().Before(cutoff) {
			return false
		}
	}
	return true
}
