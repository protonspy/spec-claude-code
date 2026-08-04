// Package codegraph wires CodeGraph — the local symbol graph a coding agent
// queries instead of reading files one at a time — into scc.
//
// The same three concerns kept apart here as in the Headroom integration, for the
// same reason: naming the argument vectors, which is pure data; finding the
// binary, which is a PATH lookup; and installing it, which needs a network and
// can take a while.
//
// scc composes command lines and nothing else. The graph's schema, the file
// watcher, and the MCP server CodeGraph registers into the agent's own config are
// CodeGraph's, and scc deliberately knows none of them — `codegraph init` is a
// process to run, not a format to parse.
package codegraph

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Repo is where CodeGraph is developed, for the error that has to send somebody
// somewhere.
const Repo = "https://github.com/colbymchenry/codegraph"

// Bin is the executable's name, as it appears on PATH.
const Bin = "codegraph"

// Pkg is the npm package that carries the CLI.
const Pkg = "@colbymchenry/codegraph"

// Dir is the per-project directory CodeGraph builds the graph into. Its presence
// is the whole test for "this workspace has been initialized": scc never reads
// what is inside, because the contents are a SQLite database on CodeGraph's
// schedule, not a format scc has any business knowing.
const Dir = ".codegraph"

// Indexed reports whether root has a graph.
func Indexed(root string) bool {
	info, err := os.Stat(filepath.Join(root, Dir))
	return err == nil && info.IsDir()
}

// Path reports where the codegraph binary is, and whether it is on PATH at all.
func Path() (string, bool) {
	p, err := exec.LookPath(Bin)
	if err != nil {
		return "", false
	}
	return p, true
}

// Version reports what `codegraph version` says, or "" when the binary cannot
// answer. Advisory only: it is printed, never branched on.
func Version(bin string) string {
	out, err := exec.Command(bin, "version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// InitArgs builds the graph for the first time. One step: `init` creates the
// directory and does the full index, and afterwards CodeGraph's own watcher keeps
// it current.
func InitArgs() []string { return []string{"init"} }

// SyncArgs is the incremental update, for a workspace that already has a graph
// and has been edited since the watcher last ran.
func SyncArgs() []string { return []string{"sync"} }

// IndexArgs is the full rebuild from scratch, for a graph that has gone wrong
// rather than merely stale.
func IndexArgs() []string { return []string{"index", "--force"} }

// StatusArgs reports the graph's statistics.
func StatusArgs(jsonOut bool) []string { return withJSON([]string{"status"}, jsonOut) }

// QueryArgs searches symbols by name. kind and limit are omitted when zero, so
// the defaults stay CodeGraph's rather than becoming scc's.
func QueryArgs(query, kind string, limit int, jsonOut bool) []string {
	args := []string{"query", query}
	if kind != "" {
		args = append(args, "--kind", kind)
	}
	if limit > 0 {
		args = append(args, "--limit", strconv.Itoa(limit))
	}
	return withJSON(args, jsonOut)
}

// ExploreArgs is the one-shot question: the relevant symbols' source plus the
// call paths between them.
//
// No --json here, and that is CodeGraph's design rather than an omission: explore
// is the CLI face of the codegraph_explore MCP tool and emits the same
// agent-shaped text. Passing a flag it does not define would fail the command.
func ExploreArgs(query string) []string { return []string{"explore", query} }

func withJSON(args []string, jsonOut bool) []string {
	if jsonOut {
		return append(args, "--json")
	}
	return args
}

// Installer is one way to get the CLI onto PATH.
type Installer struct {
	// Prog is the program that does the installing, looked up on PATH.
	Prog string
	// Args is the argument vector passed to Prog.
	Args []string
	// Cmd is the same command written the way a shell takes it, for printing.
	Cmd string
}

// Installers are the ways scc will install the CLI on the user's behalf.
//
// npm only, and the list is short on purpose. CodeGraph's headline install is a
// remote script piped into a shell — `curl … | sh`, `irm … | iex` — which is a
// fine thing for a person to type and not a thing scc gets to run for them: it
// executes whatever the URL serves at that moment, with no version pinned and
// nothing to inspect. InstallHint still names it, because somebody without npm
// needs a way through and that decision is theirs to make.
func Installers() []Installer {
	return []Installer{
		{
			Prog: "npm",
			Args: []string{"install", "-g", Pkg},
			Cmd:  "npm install -g " + Pkg,
		},
	}
}

// Available returns the first installer whose program is actually on this machine.
func Available() (Installer, bool) {
	for _, i := range Installers() {
		if _, err := exec.LookPath(i.Prog); err == nil {
			return i, true
		}
	}
	return Installer{}, false
}

// InstallHint is what to tell somebody who has to do it themselves: what scc
// would have run, then the standalone bundle for a machine with no npm.
func InstallHint() string {
	lines := make([]string, 0, len(Installers())+1)
	for _, i := range Installers() {
		lines = append(lines, i.Cmd)
	}
	lines = append(lines, "see "+Repo+" for the standalone bundle")
	return strings.Join(lines, "\n  or: ")
}

// Install runs i, streaming its output — a global npm install is slow enough that
// a silent command reads as a hang.
//
// A missing program is reported as itself rather than as a failed install: the
// user has to get npm first, which is a different problem from an install that
// broke.
func Install(i Installer, stdout, stderr io.Writer) error {
	prog, err := exec.LookPath(i.Prog)
	if err != nil {
		return fmt.Errorf("%s is not on PATH; install it, then run: %s", i.Prog, i.Cmd)
	}
	cmd := exec.Command(prog, i.Args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", i.Cmd, err)
	}
	return nil
}

// Run executes the CLI in root with the given arguments, streaming its output.
// The exit code is the child's, so a caller can tell "the graph has no answer"
// from "the command could not run".
func Run(bin, root string, args []string, stdout, stderr io.Writer) (int, error) {
	cmd := exec.Command(bin, args...)
	cmd.Dir = root
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode(), nil
		}
		return 1, err
	}
	return 0, nil
}
