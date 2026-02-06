package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/killme2008/devtap/internal/mcp"
)

// version is set by goreleaser via ldflags.
var version = "dev"

func main() {
	setVersionFromBuildInfo()
	mcp.ServerVersion = version
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func setVersionFromBuildInfo() {
	if version != "dev" {
		return
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	if info.Main.Version == "" || info.Main.Version == "(devel)" {
		return
	}
	version = info.Main.Version
}
