package cli

import (
	"flag"
	"fmt"
	"os"
	"path"

	"github.com/protonspy/spec-claude-code/internal/hooks"
	"github.com/protonspy/spec-claude-code/internal/paths"
	"github.com/protonspy/spec-claude-code/internal/render"
	"github.com/protonspy/spec-claude-code/internal/rtk"
	"github.com/protonspy/spec-claude-code/internal/scaffold"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// runInit scaffolds a workspace: the rules, the review agents, the knowledge-base
// skills and their commands, the directory layout, and the manifest that makes the
// directory findable as a workspace.
//
// Then the two steps that wire it into the tools around it rather than writing
// scc's own files — RTK's usage block in the entry file, and the git hooks — both
// on by default and both degrading to a line saying what is not wired. See
// initRTK and initHooks for why each is a default rather than a flag.
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
	withRTK := fs.Bool("rtk", false, "wire RTK in without asking: build it when it is missing, and put its usage block in the entry file")
	noRTK := fs.Bool("no-rtk", false, "scaffold without RTK's usage block in the entry file, and never run cargo")
	noHooks := fs.Bool("no-hooks", false, "scaffold without the git hooks that gate a commit and a push on `scc validate`")
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
	if *withRTK && *noRTK {
		render.Err("--rtk and --no-rtk contradict each other; pass one")
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

	// The two steps that wire this workspace into the tools around it, rather than
	// writing scc's own files: RTK's usage block in the entry file, and the git
	// hooks. Both are on by default and both degrade to a line saying what is not
	// wired — the hooks because they install nothing and shell out to nothing, RTK
	// because the block is what makes RTK work at all and the one part that is a
	// real decision, the cargo build, is still asked about.
	//
	// A directory that is not a git repository simply has nowhere to put the hooks,
	// which is not a failure of `init`.
	if *jsonOut {
		rtkOut, code := initRTK(target, *withRTK, *noRTK, true)
		if c := emitJSON(initReport{Result: res, RTK: rtkOut,
			Hooks: initHooks(target, *noHooks, true), Harness: initHarnessHooks(target, *noHooks)}); c != ExitOK {
			return c
		}
		return code
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
	// After the file listing, not before it: RTK reports on the entry file, and a
	// line saying the block went into CLAUDE.md above the line saying CLAUDE.md was
	// created reads as an edit to a file that does not exist yet. It is also where
	// the cargo question belongs — the workspace is on disk by now, so answering no
	// leaves something finished rather than something abandoned.
	_, rtkCode := initRTK(target, *withRTK, *noRTK, false)
	initHooks(target, *noHooks, false)
	return rtkCode
}

// initReport is init's JSON document. The scaffold result is embedded, so its shape
// is unchanged for every caller already parsing it; "rtk" appears on the runs that
// wired RTK in and "hooks" on the runs that had a git repository to write to, so
// the absence of a key is itself the answer to "was this step taken".
type initReport struct {
	*scaffold.Result
	RTK     *rtkReport            `json:"rtk,omitempty"`
	Hooks   []hooks.Status        `json:"hooks,omitempty"`
	Harness []hooks.HarnessStatus `json:"harness_hooks,omitempty"`
}

// initHooks wires the git hooks up as part of scaffolding, and says so.
//
// It degrades rather than fails, the way every optional step in this product
// does: no git, a hooks directory git will not name, or a pre-commit somebody
// else wrote all end in a scaffolded workspace with a line saying what is not
// gated. Refusing to finish `init` over a hook would put scc's own preference
// above the thing the user asked for.
func initHooks(root string, disabled, quiet bool) []hooks.Status {
	if disabled {
		return nil
	}
	agent := hooks.InstallHarness(root)
	all, err := hooks.Install(root, false)
	if err != nil {
		// No git repository to write into, which is not a failure of `init` — and
		// the harness hooks are unaffected, since they live in the workspace rather
		// than in `.git`.
		if !quiet {
			render.Info("no git hooks: " + err.Error())
		}
		reportInitHarness(agent, quiet)
		return nil
	}
	if quiet {
		return all
	}
	for _, s := range all {
		switch {
		case s.Note != "":
			render.Warn(fmt.Sprintf("%s hook left alone — %s", s.Stage, s.Note))
		case s.Action == hooks.Added || s.Action == hooks.Replaced:
			render.OK(fmt.Sprintf("%s hook — %s", s.Stage, s.Stage.Why()))
		}
	}
	reportInitHarness(agent, quiet)
	return all
}

// reportInitHarness says what was registered in the harness's own settings file.
//
// Separate from the git lines because it is a separate claim: these fire inside a
// session rather than around a commit, they are written into a file the harness
// owns rather than into `.git`, and — unlike the git hooks — they are committed,
// so everyone who clones the repository gets them. That last part is worth a line
// on the run that creates it.
func reportInitHarness(all []hooks.HarnessStatus, quiet bool) {
	if quiet {
		return
	}
	for _, s := range all {
		switch {
		case s.Note != "":
			render.Warn(fmt.Sprintf("%s left alone — %s", s.Event, s.Note))
		case s.Action == hooks.Added || s.Action == hooks.Replaced:
			render.OK(fmt.Sprintf("%s hook — %s", s.Event, s.Event.Why()))
		}
	}
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

// initRTK puts RTK's usage block in the entry file the agent is about to load,
// and makes sure the binary that block names is actually there.
//
// Setup is where this belongs. The block is what makes RTK work at all — an agent
// that never read it never types the prefix — so a workspace scaffolded without it
// has an entry file that is complete about the methodology and silent about the
// one tool standing between the agent and every command's full output. `scc
// launch` has wired it since the workspace's first session for exactly that
// reason; doing it at `init` is the same step at the first moment it can happen,
// and it means a workspace is wired whether the first session starts through
// `scc launch` or by typing `claude`.
//
// What stays a decision is the install, and the two halves are settled in that
// order. cargo is a Rust toolchain and minutes of build, so a bare `init` asks
// before running it and takes silence for no; `--rtk` is that consent given in
// advance, and `--no-rtk` declines the whole step. Nothing is written when the
// binary is absent, because guidance naming a command the machine cannot run is
// worse than no guidance: an agent that tries the prefix, watches it fail, and
// then reads the same file's rules learns to discount all of them.
//
// It degrades rather than fails. A missing cargo, a declined prompt, or a failed
// build all end in a scaffolded workspace and one line saying what is not wired —
// except under --rtk, where the user asked for this by name and a failure is an
// answer they need in the exit code.
func initRTK(root string, yes, disabled, quiet bool) (*rtkReport, int) {
	if disabled {
		return nil, ExitOK
	}
	if _, ok := rtk.Path(); !ok {
		if reason := rtkInstallOK(rtkAsk{yes: yes, quiet: quiet}); reason != "" {
			// --rtk named this step, so not getting it is an error and is said as
			// one — on stderr, which is also the only stream left when the caller is
			// emitting JSON on stdout. Without the flag it is a status line: the
			// workspace is scaffolded and complete, and RTK is the part that is not
			// wired yet.
			if yes {
				render.Err("--rtk: " + reason)
				render.Detail("  " + rtk.InstallCmd())
				return nil, ExitError
			}
			if !quiet {
				render.Info("no RTK block: " + reason)
				render.Detail(fmt.Sprintf("  wire it in later with: %s rtk", prog()))
			}
			return nil, ExitOK
		}
	}
	// keep, because a re-run of `init` is not the moment to replace a block
	// somebody put there themselves — `scc rtk` is where that trade-off is made
	// deliberately, and it is the same call `scc launch` makes for the same reason.
	report, code := applyRTK(root, rtkOptions{keep: true, quiet: quiet})
	if !yes {
		return report, ExitOK
	}
	return report, code
}

// initHarnessHooks is the JSON path's view of what was registered in the
// harness's settings file.
//
// A second call rather than a value threaded out of initHooks, because
// InstallHarness is idempotent and the alternative is a signature that returns
// two lists to satisfy one caller. It runs after initHooks in both paths, so what
// it reports is the state that run left behind.
func initHarnessHooks(root string, disabled bool) []hooks.HarnessStatus {
	if disabled {
		return nil
	}
	return hooks.LookHarness(root)
}
