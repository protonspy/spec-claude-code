package cli

import (
	"flag"
	"fmt"
	"os"
	"path"

	"github.com/protonspy/spec-claude-code/internal/paths"
	"github.com/protonspy/spec-claude-code/internal/render"
	"github.com/protonspy/spec-claude-code/internal/scaffold"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// runInit scaffolds a workspace: the rules, the review agents, the knowledge-base
// skills and their commands, the directory layout, and the manifest that makes the
// directory findable as a workspace.
//
// One harness per run, selected by flag and defaulting to Claude Code. Running it
// twice with different harnesses is supported and is how a repo worked on from two
// tools gets both trees — the second run leaves the first's files alone, including
// the entry file when the two harnesses share one.
//
// It is safe to re-run. Without --force it never overwrites anything, so "run init
// again" is the honest upgrade story until `scc update` exists — additive by
// construction, and it simply does not deliver improved templates to files that
// already exist.
func runInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	force := fs.Bool("force", false, "overwrite existing files, naming every edited file it clobbers")
	withRTK := fs.Bool("rtk", false, "also wire in RTK: install it if missing, and put its usage block in the entry file")
	picks := map[string]*bool{}
	for _, h := range paths.Harnesses() {
		picks[h.ID] = fs.Bool(h.ID, false, "scaffold for "+h.ID+" ("+h.EntryFile+", "+h.Dir+"/)")
	}
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if !noPositionals(rest, "init") {
		return ExitError
	}

	harness, err := chooseHarness(picks, *jsonOut)
	if err != nil {
		render.Err(err.Error())
		return ExitError
	}

	target, ok := resolveRoot(*root)
	if !ok {
		return ExitError
	}

	res, err := scaffold.Apply(target, scaffold.Options{SCCVersion: version, Harness: harness, Force: *force})
	if err != nil {
		render.Err(fmt.Sprintf("init failed: %v", err))
		return ExitError
	}

	// RTK is opt-in, and stays opt-in: wiring it in can shell out to cargo for
	// minutes and it tells the agent to prefix every command with a binary this
	// machine may not have. A default that does either would be scc making a
	// decision about someone else's toolchain.
	rtkCode := ExitOK
	var rtkOut *rtkReport
	if *withRTK {
		rtkOut, rtkCode = applyRTK(target, rtkOptions{quiet: *jsonOut})
	}

	if *jsonOut {
		if c := emitJSON(initReport{Result: res, RTK: rtkOut}); c != ExitOK {
			return c
		}
		return rtkCode
	}

	// Clobbered files come first: it is the only outcome that destroyed something,
	// so it must not be buried under a list of successes.
	for _, path := range res.Clobbered {
		render.Warn(fmt.Sprintf("overwrote your edited %s", path))
	}
	for _, c := range res.Changes {
		if c.Action == scaffold.Created {
			render.OK(c.Path)
		}
	}
	switch {
	case res.Created == 0 && res.Replaced == 0:
		render.Info(fmt.Sprintf("already up to date: %d files, nothing to write", res.Skipped))
	case res.AlreadyPresent:
		render.Info(fmt.Sprintf("%d created, %d replaced, %d left alone", res.Created, res.Replaced, res.Skipped))
	default:
		render.OK(fmt.Sprintf("%s workspace ready in %s", harness.Label, workspace.Relative(mustCwd(), target)))
		render.Info(fmt.Sprintf("read %s, then fill in %s/project.md with this project's commands",
			harness.EntryFile, path.Join(harness.Dir, harness.RulesSeg)))
	}
	return rtkCode
}

// initReport is init's JSON document. The scaffold result is embedded, so its shape
// is unchanged for every caller already parsing it, and "rtk" appears only in the
// runs that asked for RTK.
type initReport struct {
	*scaffold.Result
	RTK *rtkReport `json:"rtk,omitempty"`
}

// mustCwd is only ever used to make a path friendlier to print, so a failure
// degrades to the absolute path rather than to an error the user cannot act on.
func mustCwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return cwd
}
