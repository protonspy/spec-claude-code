// Package cli is the headless command surface — the whole product. Everything
// scc can do is reachable from here through flags, with machine-readable output,
// so an agent or a CI job drives it exactly as well as a human does.
//
// Run is the entire dispatcher: it switches on args[0] and hands off to a
// run<Resource> function living in a file named for that resource (spec.go,
// wiki.go, …). Each handler owns its own flag.FlagSet. Adding a subcommand means
// adding a case here plus one file — nothing is registered dynamically, so the
// set of commands is readable in one place.
package cli

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/render"
)

// version is stamped at build time via
// -ldflags "-X github.com/protonspy/spec-claude-code/internal/cli.version=vX.Y.Z".
// A source build reports "dev".
var version = "dev"

// Exit codes are the contract every caller branches on. Keep them distinct: a
// validation *finding* is a legitimate answer to a lint question, not a failure
// of the tool, and CI needs to tell the two apart.
const (
	ExitOK       = 0 // the command did what was asked
	ExitError    = 1 // usage error, or the command could not run
	ExitFindings = 2 // the command ran and reported validation findings
)

// prog is the name to print in usage text. The npm launcher sets SCC_PROG when
// invoked through `npx`, so help output echoes the spelling the user actually
// typed instead of a bare "scc" they may not have on PATH.
func prog() string {
	if p := strings.TrimSpace(os.Getenv("SCC_PROG")); p != "" {
		return p
	}
	return "scc"
}

// Run dispatches args (os.Args[1:]) and returns the process exit code. It never
// calls os.Exit, so tests can drive the whole surface in-process.
func Run(args []string) int {
	if len(args) == 0 {
		usage()
		return ExitError
	}
	switch args[0] {
	case "help", "-h", "--help":
		usage()
		return ExitOK
	case "version", "-v", "--version":
		return runVersion(args[1:])
	case "init":
		return runInit(args[1:])
	case "update":
		return runUpdate(args[1:])
	case "rtk":
		return runRTK(args[1:])
	case "launch":
		return runLaunch(args[1:])
	case "graph":
		return runGraph(args[1:])
	case "spec":
		return runSpec(args[1:])
	case "plan":
		return runPlan(args[1:])
	case "skill":
		return runSkill(args[1:])
	case "validate":
		return runValidateAll(args[1:])
	default:
		render.Err(fmt.Sprintf("unknown command %q", args[0]))
		fmt.Fprintf(os.Stderr, "run `%s help` for the available commands\n", prog())
		return ExitError
	}
}

// runVersion prints the stamped version, plus the build's Go toolchain and
// platform under --json so a bug report carries them without a second command.
func runVersion(args []string) int {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := addJSON(fs)
	if err := fs.Parse(args); err != nil {
		return ExitError
	}
	if *jsonOut {
		return emitJSON(struct {
			Version  string `json:"version"`
			Go       string `json:"go"`
			Platform string `json:"platform"`
		}{version, runtime.Version(), runtime.GOOS + "/" + runtime.GOARCH})
	}
	fmt.Println(version)
	return ExitOK
}

func usage() {
	fmt.Fprintf(os.Stderr, `%s — spec-driven development for Claude Code, Codex, and opencode

Usage:
  %s <command> [flags]

Commands:
  init      Scaffold a workspace: rules, agents, skills, commands, layout, manifest
  update    Bring the managed files onto this build's templates, after showing the plan
  rtk       Install RTK if missing and put its usage block in the entry file
  launch    Start a harness in this workspace, through Headroom's compression proxy
  graph     The workspace's symbol graph — build | sync | status | query | explore
  spec      Create and inspect specs — new | list | show | delete | validate
  plan      Create and inspect plans — new | list | delete | validate
  skill     Agent Skills conformance — validate
  validate  Run every applicable validator; exit 2 on findings
  version   Print the version
  help      Show this help

Run "%s <command> help" for a command's own flags.

Every command accepts --json for machine-readable output on stdout, and --root to
act on a workspace other than the enclosing one.

Exit codes:
  0  ok
  1  usage or runtime error
  2  validation findings

"%s launch" is the one exception: it returns whatever the agent it started returned.
`, render.Bold(prog()), prog(), prog(), prog())
}
