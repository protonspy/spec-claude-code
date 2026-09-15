package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/codegraph"
	"github.com/protonspy/spec-claude-code/internal/render"
)

// runGraph is the workspace's symbol graph: build it, refresh it, ask it things.
//
// A wrap over CodeGraph rather than a reimplementation of it, and thin on
// purpose. What scc adds is the two things it already knows and CodeGraph does
// not: which directory is the workspace root — so `scc graph build` from specs/
// indexes the repo rather than a subtree — and whether the binary is there at
// all, answered once, the same way `scc rtk` answers it.
//
// The graph is not an scc artifact. It is not in the manifest, `scc update` never
// touches it, and `.codegraph/` is CodeGraph's directory on CodeGraph's schedule.
// scc's claim on it ends at composing the command line.
func runGraph(args []string) int {
	if len(args) == 0 {
		graphUsage()
		return ExitError
	}
	switch args[0] {
	case "help", "-h", "--help":
		graphUsage()
		return ExitOK
	case "build":
		return runGraphBuild(args[1:])
	case "sync":
		return runGraphSync(args[1:])
	case "status":
		return runGraphStatus(args[1:])
	case "query":
		return runGraphQuery(args[1:])
	case "scope":
		return runGraphScope(args[1:])
	case "explore":
		return runGraphExplore(args[1:])
	default:
		render.Err(fmt.Sprintf("unknown graph subcommand %q", args[0]))
		fmt.Fprintf(os.Stderr, "run `%s graph help` for the available subcommands\n", prog())
		return ExitError
	}
}

// runGraphBuild indexes the workspace.
//
// `init` when there is no graph and `index --force` when there is, because those
// are two different requests wearing one word: the first time is setup, and every
// later time is somebody saying the graph has gone wrong. Ordinary staleness is
// neither — that is `sync`, and mostly the watcher has already handled it.
func runGraphBuild(args []string) int {
	fs := flag.NewFlagSet("graph build", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	noInstall := fs.Bool("no-install", false, "never install CodeGraph; use it only if it is already on PATH")
	yes := fs.Bool("yes", false, "answer the install prompt with yes, for an unattended run")
	force := fs.Bool("force", false, "rebuild from scratch even when a graph is already there")
	rest, err := parseFlags(fs, args)
	if err != nil {
		return exitFor(err)
	}
	if !noPositionals(rest, "graph build") {
		return ExitError
	}
	target, bin, ok := graphTarget(*root, graphInstall{noInstall: *noInstall, yes: *yes})
	if !ok {
		return ExitError
	}

	roots, ok := loadScope(target, false)
	if !ok {
		return ExitError
	}
	// Decided per root rather than once: in a scoped workspace one tree can have a
	// graph and another not, and a single answer for both would either re-index what
	// was current or incrementally update what does not exist yet.
	code := ExitOK
	for _, r := range roots {
		if len(roots) > 1 {
			render.Info(fmt.Sprintf("── %s ──", r.Rel))
		}
		cmd := codegraph.InitArgs()
		switch {
		case *force:
			cmd = codegraph.IndexArgs()
		case r.Indexed():
			render.Info("a graph is already there; rebuilding it incrementally — pass --force for a full re-index")
			cmd = codegraph.SyncArgs()
		}
		if c := graphExec(bin, r.Dir, cmd); c != ExitOK {
			code = c
		}
	}
	return code
}

func runGraphSync(args []string) int {
	fs := flag.NewFlagSet("graph sync", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return exitFor(err)
	}
	if !noPositionals(rest, "graph sync") {
		return ExitError
	}
	target, bin, ok := graphTarget(*root, graphInstall{noInstall: true})
	if !ok {
		return ExitError
	}
	roots, ok := loadScope(target, false)
	if !ok || !requireGraphs(roots) {
		return ExitError
	}
	return graphFan(bin, roots, codegraph.SyncArgs())
}

// runGraphStatus reports what the graph holds. --check turns the same question
// into the findings code, so CI branches on "this checkout has no graph" the way
// it branches on `scc validate`.
func runGraphStatus(args []string) int {
	fs := flag.NewFlagSet("graph status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	check := fs.Bool("check", false, "report only; exit 2 when the workspace has no graph")
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return exitFor(err)
	}
	if !noPositionals(rest, "graph status") {
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}

	if *check {
		// Deliberately no binary lookup and no subprocess: --check answers whether
		// this workspace has a graph, and a CI runner without CodeGraph installed
		// still has a correct answer to that.
		roots, ok := loadScope(target, *jsonOut)
		if !ok {
			return ExitError
		}
		indexed := true
		for _, r := range roots {
			if !r.Indexed() {
				indexed = false
			}
		}
		if *jsonOut {
			if c := emitJSON(struct {
				Indexed bool             `json:"indexed"`
				Dir     string           `json:"dir"`
				Roots   []codegraph.Root `json:"roots"`
			}{indexed, codegraph.Dir, roots}); c != ExitOK {
				return c
			}
		} else {
			for _, r := range roots {
				if r.Indexed() {
					render.OK(fmt.Sprintf("%-28s has a graph in %s", r.Rel, codegraph.Dir))
					continue
				}
				render.Warn(fmt.Sprintf("%-28s no graph; run `%s graph build`", r.Rel, prog()))
			}
		}
		if !indexed {
			return ExitFindings
		}
		return ExitOK
	}

	_, bin, ok := graphTarget(target, graphInstall{noInstall: true})
	if !ok {
		return ExitError
	}
	roots, ok := loadScope(target, *jsonOut)
	if !ok || !requireGraphs(roots) {
		return ExitError
	}
	if *jsonOut && len(roots) > 1 {
		return graphFanJSON(bin, roots, codegraph.StatusArgs(true))
	}
	return graphFan(bin, roots, codegraph.StatusArgs(*jsonOut))
}

func runGraphQuery(args []string) int {
	fs := flag.NewFlagSet("graph query", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	kind := fs.String("kind", "", "restrict the search to one symbol kind, e.g. class or function")
	limit := fs.Int("limit", 0, "stop after this many results (default: CodeGraph's own)")
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return exitFor(err)
	}
	query, ok := graphQueryArg(rest, "query")
	if !ok {
		return ExitError
	}
	target, bin, ok := graphTarget(*root, graphInstall{noInstall: true})
	if !ok {
		return ExitError
	}
	roots, ok := loadScope(target, *jsonOut)
	if !ok || !requireGraphs(roots) {
		return ExitError
	}
	if *jsonOut && len(roots) > 1 {
		return graphFanJSON(bin, roots, codegraph.QueryArgs(query, *kind, *limit, true))
	}
	return graphFan(bin, roots, codegraph.QueryArgs(query, *kind, *limit, *jsonOut))
}

// runGraphExplore is the question worth asking: the relevant symbols' source and
// the call paths between them, in one answer, without the agent reading files to
// find them. No --json, because explore emits the same agent-shaped text as the
// MCP tool it is the CLI face of.
func runGraphExplore(args []string) int {
	fs := flag.NewFlagSet("graph explore", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return exitFor(err)
	}
	query, ok := graphQueryArg(rest, "explore")
	if !ok {
		return ExitError
	}
	target, bin, ok := graphTarget(*root, graphInstall{noInstall: true})
	if !ok {
		return ExitError
	}
	roots, ok := loadScope(target, false)
	if !ok || !requireGraphs(roots) {
		return ExitError
	}
	return graphFan(bin, roots, codegraph.ExploreArgs(query))
}

// graphQueryArg takes the search terms. Joined rather than required to be one
// argument, so an unquoted question reaches CodeGraph as the sentence it was
// typed as instead of being rejected as too many positionals.
func graphQueryArg(rest []string, sub string) (string, bool) {
	query := strings.TrimSpace(strings.Join(rest, " "))
	if query == "" {
		render.Err(fmt.Sprintf("`%s graph %s` needs something to look for", prog(), sub))
		return "", false
	}
	return query, true
}

type graphInstall struct {
	noInstall bool
	yes       bool
}

// graphTarget resolves the workspace root and finds the binary, installing it
// first when asked to. Unlike the Headroom path in `scc launch`, a missing binary
// here is a hard error rather than a degraded run: the whole command is the
// binary, so there is nothing left to fall back to.
func graphTarget(root string, opts graphInstall) (string, string, bool) {
	target, ok := resolveRoot(root)
	if !ok || !requireWorkspace(target) {
		return "", "", false
	}
	bin, ok := ensureCodeGraph(opts)
	if !ok {
		return "", "", false
	}
	return target, bin, true
}

// ensureCodeGraph finds the CLI, offering to install it when somebody is there to
// be asked.
func ensureCodeGraph(opts graphInstall) (string, bool) {
	if p, ok := codegraph.Path(); ok {
		return p, true
	}
	installer, available := codegraph.Available()
	ask := !opts.noInstall && available && (opts.yes || interactive())
	if ask && !opts.yes {
		render.Warn(fmt.Sprintf("%s is not on PATH — it is what builds and answers the graph", codegraph.Bin))
		render.Detail("  " + codegraph.Repo)
		ask = confirmInstall(promptIn, fmt.Sprintf("Install it now with `%s`?", installer.Cmd))
	}
	if !ask {
		render.Err(codegraph.Bin + " is not on PATH")
		render.Detail("  install it with: " + codegraph.InstallHint())
		return "", false
	}

	render.Info(fmt.Sprintf("installing %s: %s", codegraph.Bin, installer.Cmd))
	if err := codegraph.Install(installer, os.Stderr, os.Stderr); err != nil {
		render.Err(err.Error())
		return "", false
	}
	p, ok := codegraph.Path()
	if !ok {
		render.Err(fmt.Sprintf("%s reported success but %s is still not on PATH", installer.Prog, codegraph.Bin))
		return "", false
	}
	render.OK(strings.TrimSpace(codegraph.Bin + " installed: " + p + " " + codegraph.Version(p)))
	return p, true
}

// graphExec runs CodeGraph with this terminal attached.
//
// A child's non-zero exit becomes scc's 1: `scc graph` is a wrap, not a launcher,
// so the 0/1/2 contract holds here — the exemption `scc launch` has exists for a
// long-lived interactive session whose status a script has to see, which none of
// these are. It is a package var so tests can drive the surface without CodeGraph
// installed.
var graphExec = func(bin, root string, args []string) int {
	code, err := codegraph.Run(bin, root, args, os.Stdout, os.Stderr)
	if err != nil {
		render.Err(fmt.Sprintf("could not run %s: %v", bin, err))
		return ExitError
	}
	if code != 0 {
		return ExitError
	}
	return ExitOK
}

func graphUsage() {
	fmt.Fprintf(os.Stderr, `%s graph — the workspace's symbol graph, via CodeGraph

Usage:
  %s graph <subcommand> [flags]

Subcommands:
  build     Index the workspace (--force for a full rebuild)
  sync      Bring an existing graph up to date incrementally
  status    Show what the graph holds (--check exits 2 when there is none)
  query     Search symbols by name (--kind, --limit)
  explore   Relevant symbols' source plus the call paths between them
  scope     Which trees get indexed — show | set <dir>… | clear

By default the whole workspace is one graph. A scope narrows that:

  %s graph scope set backend/src frontend/src

**A scope is one graph per directory, not one graph filtered.** CodeGraph
indexes a single root at a time — no command takes two paths and none has an
include flag — so each scoped directory gets its own %s/ inside it, and
build, sync, status, query and explore all answer once per root. A trailing
/* is accepted and means the same as the directory; an interior glob such as
packages/*/src is expanded to the directories it matches.

The graph belongs to CodeGraph: scc never tracks it in the manifest and
"%s update" never touches it. Only the scope is scc's, recorded beside the
delivery gate's commands.

  %s
`, render.Bold(prog()), prog(), prog(), codegraph.Dir, prog(), codegraph.Repo)
}
