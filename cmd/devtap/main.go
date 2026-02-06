package main

import (
	"fmt"
	"os"

	"github.com/killme2008/devtap/internal/mcp"
)

// version is set by goreleaser via ldflags.
var version = "dev"

func main() {
	mcp.ServerVersion = version
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
