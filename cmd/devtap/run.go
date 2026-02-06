package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/killme2008/devtap/internal/adapter"
	aideradapter "github.com/killme2008/devtap/internal/adapter/aider"
	"github.com/killme2008/devtap/internal/adapter/claudecode"
	"github.com/killme2008/devtap/internal/adapter/codex"
	"github.com/killme2008/devtap/internal/adapter/gemini"
	"github.com/killme2008/devtap/internal/adapter/opencode"
	"github.com/killme2008/devtap/internal/capture"
	"github.com/killme2008/devtap/internal/filter"
	"github.com/killme2008/devtap/internal/session"
)

func runRun(cmd *cobra.Command, args []string) error {
	dashIdx := cmd.ArgsLenAtDash()
	if dashIdx < 0 || dashIdx >= len(args) {
		return fmt.Errorf("usage: devtap [flags] -- <command> [args...]\n\nNo command specified after '--'")
	}

	cmdArgs := args[dashIdx:]
	if len(cmdArgs) == 0 {
		return fmt.Errorf("no command specified after '--'")
	}

	filterRegex, _ := cmd.Flags().GetString("filter-regex")
	filterInvert, _ := cmd.Flags().GetBool("filter-invert")
	tag, _ := cmd.Flags().GetString("tag")
	adapterName, _ := cmd.Flags().GetString("adapter")
	sessionFlag, _ := cmd.Flags().GetString("session")
	debounce, _ := cmd.Flags().GetDuration("debounce")

	if tag == "" {
		if name := capture.CommandName(cmdArgs); name != "" {
			tag = filepath.Base(name)
		} else {
			tag = filepath.Base(cmdArgs[0])
		}
	}

	var lineFilter capture.LineFilter
	if filterRegex != "" {
		f, err := filter.Regex(filterRegex, filterInvert)
		if err != nil {
			return fmt.Errorf("invalid filter regex: %w", err)
		}
		lineFilter = f
	}

	sessionID, err := resolveSession(adapterName, sessionFlag)
	if err != nil {
		return fmt.Errorf("resolve session: %w", err)
	}

	s, err := openStore(cmd)
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}
	defer func() { _ = s.Close() }()

	var result *capture.RunResult

	if debounce > 0 {
		runner := capture.NewLongRunner(s, sessionID, tag, lineFilter, debounce)
		result, err = runner.Run(cmdArgs)
	} else {
		runner := capture.New(s, sessionID, tag, lineFilter)
		result, err = runner.Run(cmdArgs)
	}

	if err != nil {
		return fmt.Errorf("run command: %w", err)
	}

	os.Exit(result.ExitCode)
	return nil
}

// getAdapter returns the adapter instance for the given name.
func getAdapter(name string) adapter.Adapter {
	switch name {
	case "claude-code":
		return claudecode.New()
	case "codex":
		return codex.New()
	case "aider":
		return aideradapter.New()
	case "opencode":
		return opencode.New()
	case "gemini":
		return gemini.New()
	default:
		return claudecode.New()
	}
}

func resolveSession(adapterName, sessionFlag string) (string, error) {
	if sessionFlag != "auto" && sessionFlag != "pick" {
		return sessionFlag, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	projectDir := resolveProjectDir(cwd)

	a := getAdapter(adapterName)
	sessions, err := a.DiscoverSessions(projectDir)
	if err != nil {
		return "", err
	}
	if len(sessions) == 0 {
		return session.EncodeDir(projectDir), nil
	}
	if sessionFlag == "auto" {
		return sessions[0].ID, nil
	}
	return session.Pick(sessions)
}

func resolveProjectDir(start string) string {
	if root, ok := findGitRoot(start); ok {
		return root
	}
	if root, ok := findMarkerRoot(start); ok {
		return root
	}
	return start
}

func findGitRoot(start string) (string, bool) {
	for dir := start; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return "", false
}

func findMarkerRoot(start string) (string, bool) {
	markers := []string{
		"go.mod",
		"package.json",
		"pyproject.toml",
		"Cargo.toml",
		"pom.xml",
		"build.gradle",
		"build.gradle.kts",
		"composer.json",
		"Gemfile",
		"setup.py",
	}

	for dir := start; ; dir = filepath.Dir(dir) {
		for _, name := range markers {
			if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
				return dir, true
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return "", false
}
