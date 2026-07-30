package cli

import (
	"flag"
	"fmt"
	"os"

	"github.com/protonspy/spec-claude-code/internal/render"
	"github.com/protonspy/spec-claude-code/internal/scaffold"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// runInit scaffolds a workspace: the rules, the review agents, the directory
// layout, and the manifest that makes the directory findable as a workspace.
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
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if !noPositionals(rest, "init") {
		return ExitError
	}

	target, ok := resolveRoot(*root)
	if !ok {
		return ExitError
	}

	res, err := scaffold.Apply(target, scaffold.Options{SCCVersion: version, Force: *force})
	if err != nil {
		render.Err(fmt.Sprintf("init failed: %v", err))
		return ExitError
	}

	if *jsonOut {
		return emitJSON(res)
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
		render.OK(fmt.Sprintf("workspace ready in %s", workspace.Relative(mustCwd(), target)))
		render.Info("read CLAUDE.md, then fill in .claude/rules/project.md with this project's commands")
	}
	return ExitOK
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
