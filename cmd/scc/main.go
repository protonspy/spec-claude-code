// scc — spec-driven development for Claude Code.
//
// A single binary with one surface: a headless CLI. Every capability is
// reachable through flags with --json output and a stable exit-code contract,
// so an AI agent or a CI job drives it exactly as well as a human does.
package main

import (
	"os"

	"github.com/protonspy/spec-claude-code/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
