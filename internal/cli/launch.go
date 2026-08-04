package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/headroom"
	"github.com/protonspy/spec-claude-code/internal/paths"
	"github.com/protonspy/spec-claude-code/internal/render"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// runLaunch starts a harness in this workspace, through Headroom's compression
// proxy when it can.
//
// It exists because "which agent, from which directory, with what in front of it"
// is a question scc already knows the answer to. The workspace walk finds the root
// whatever subdirectory the shell is in, the manifest says which harnesses were
// scaffolded, and the harness profile names the binary — so `scc launch` is one
// word where the alternative is remembering to cd first and to spell the wrapper
// right.
//
// Headroom is the default rather than a flag, which is a deliberate reversal of
// how RTK is wired. The difference is who bears the cost of being wrong: RTK's
// block edits a file the user owns and tells the agent to prefix every command
// with a binary the machine may not have, so it stays opt-in. Headroom wraps one
// process for the length of one session, changes nothing on disk, and degrades to
// starting the agent bare — so defaulting to it costs nothing when it is absent
// and saves context when it is there.
//
// The agent's own exit code is passed straight through, which is the one place
// scc's 0/1/2 contract does not apply — and it has to be. A launcher that
// flattened the exit status of what it launched would be unusable in the scripts
// people actually write. scc's own failures, before anything is started, still
// report 1.
func runLaunch(args []string) int {
	// Split on `--` before the flag package sees it, so `scc launch claude --
	// --resume` can tell scc's flags from the agent's. Doing this by hand rather
	// than leaning on flag's own terminator is what keeps the harness name a
	// positional while everything after `--` stays untouched.
	own, passthrough := splitPassthrough(args)

	fs := flag.NewFlagSet("launch", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	noHeadroom := fs.Bool("no-headroom", false, "start the agent directly, without Headroom's compression proxy")
	noInstall := fs.Bool("no-install", false, "never install Headroom; use it only if it is already on PATH")
	yes := fs.Bool("yes", false, "answer the install prompt with yes, for an unattended run")
	dryRun := fs.Bool("dry-run", false, "print the command this would run, and run nothing")
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, own)
	if err != nil {
		return ExitError
	}

	target, ok := resolveRoot(*root)
	if !ok {
		return ExitError
	}
	if !requireWorkspace(target) {
		return ExitError
	}

	harness, err := launchHarness(target, rest, *jsonOut)
	if err != nil {
		render.Err(err.Error())
		return ExitError
	}

	// --json and --dry-run both report the plan and start nothing. For --json that
	// is not a shortcut but the only coherent answer: the agent inherits this
	// terminal and writes to the same stdout, so a launched session and a clean
	// JSON document on stdout cannot both exist.
	plan := *jsonOut || *dryRun
	opts := headroomOptions{
		disabled:  *noHeadroom,
		noInstall: *noInstall || plan,
		yes:       *yes,
		quiet:     *jsonOut,
	}

	cmd := launchCommand{Harness: harness.ID, Dir: target, Bin: harness.Bin, Args: passthrough}
	if hr := resolveHeadroom(harness, opts); hr != nil {
		cmd.Headroom = hr
		if hr.Wrapping {
			cmd.Bin = headroom.Bin
			cmd.Args = headroom.WrapArgs(hr.Agent, passthrough)
		}
	}
	if cmd.Args == nil {
		// A JSON consumer gets [] rather than null: the field is a command line,
		// and an empty one is still a list.
		cmd.Args = []string{}
	}

	if *jsonOut {
		return emitJSON(cmd)
	}
	if *dryRun {
		render.Info(cmd.String())
		return ExitOK
	}

	if _, err := exec.LookPath(cmd.Bin); err != nil {
		render.Err(fmt.Sprintf("%s is not on PATH", cmd.Bin))
		render.Detail(fmt.Sprintf("  install %s, or run `%s launch --dry-run` to see the command", harness.Label, prog()))
		return ExitError
	}
	render.Info(cmd.String())
	code, err := launchExec(cmd)
	if err != nil {
		render.Err(fmt.Sprintf("could not start %s: %v", cmd.Bin, err))
		return ExitError
	}
	return code
}

// launchCommand is both the frozen JSON shape and what the human line is printed
// from, so the two cannot describe different commands.
type launchCommand struct {
	Harness  string          `json:"harness"`
	Dir      string          `json:"dir"`
	Bin      string          `json:"bin"`
	Args     []string        `json:"args"`
	Headroom *headroomReport `json:"headroom,omitempty"`
}

// String is the command as a person would type it. Not shell-quoted, because it
// is a status line rather than something to paste: scc execs the argv directly
// and no shell is ever involved.
func (c launchCommand) String() string {
	return strings.TrimSpace(c.Bin + " " + strings.Join(c.Args, " "))
}

// headroomReport says what happened on the Headroom side of a launch — reported
// on every run rather than only the ones that wrapped, because "started without
// compression" is exactly the outcome somebody would otherwise not notice.
type headroomReport struct {
	// Wrapping is whether this launch actually goes through `headroom wrap`.
	Wrapping bool `json:"wrapping"`
	// Agent is the slug Headroom knows this harness by.
	Agent   string `json:"agent,omitempty"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
	// Install is what happened to the binary: present | installed | skipped |
	// failed. The same vocabulary `scc rtk` uses, for the same reason — it
	// describes the binary, which is a different question from what the command
	// ended up doing.
	Install string `json:"install"`
	// Reason names why a launch is not wrapping, for the run where that is a
	// surprise. Empty when it is.
	Reason string `json:"reason,omitempty"`
}

type headroomOptions struct {
	disabled  bool
	noInstall bool
	yes       bool
	quiet     bool
}

// resolveHeadroom decides whether this launch goes through Headroom, installing
// it first if the user says so. It returns nil only when the user asked for no
// Headroom at all — every other outcome is reportable.
//
// Nothing here returns an exit code, and that is the design: Headroom is an
// enhancement, so every way of not getting it degrades to starting the agent bare
// rather than to failing. A launch that refused to run because a compression
// proxy was missing would be scc putting its own preference above the thing the
// user actually asked for.
func resolveHeadroom(h paths.Harness, opts headroomOptions) *headroomReport {
	if opts.disabled {
		return nil
	}

	agent, wraps := headroom.Agent(h)
	if !wraps {
		return &headroomReport{Install: installSkipped, Reason: "Headroom does not wrap " + h.Label}
	}
	report := &headroomReport{Agent: agent}

	if p, ok := headroom.Path(); ok {
		report.Wrapping, report.Path, report.Version, report.Install = true, p, headroom.Version(p), installPresent
		return report
	}

	report.Install = installSkipped
	installer, available := headroom.Available()
	switch {
	case opts.noInstall:
		report.Reason = headroom.Bin + " is not on PATH"
	case !available:
		report.Reason = fmt.Sprintf("neither uv nor pip is on PATH, so %s cannot be installed", headroom.Bin)
	case opts.yes:
		// Asked for by flag; no question to put.
	case opts.quiet || !interactive():
		// Unattended. Installing a Python distribution without being asked, in a
		// CI job or under an agent, is not a decision scc gets to make silently.
		report.Reason = fmt.Sprintf("%s is not on PATH, and nobody is here to answer the install prompt", headroom.Bin)
	default:
		render.Warn(fmt.Sprintf("%s is not on PATH — %s compresses the agent's context before it reaches the model", headroom.Bin, headroom.Bin))
		render.Detail("  " + headroom.Repo)
		if !confirm(promptIn, fmt.Sprintf("Install it now with `%s`?", installer.Cmd)) {
			report.Reason = "install declined"
		}
	}
	if report.Reason != "" {
		warnUnwrapped(report, opts)
		return report
	}

	render.Info(fmt.Sprintf("installing %s: %s — this takes a few minutes", headroom.Bin, installer.Cmd))
	// The installer's own output goes to stderr in both streams when the caller is
	// emitting JSON, because stdout carries the document and nothing else.
	out := os.Stdout
	if opts.quiet {
		out = os.Stderr
	}
	if err := headroom.Install(installer, out, os.Stderr); err != nil {
		report.Install, report.Reason = installFailed, err.Error()
		warnUnwrapped(report, opts)
		return report
	}
	p, ok := headroom.Path()
	if !ok {
		report.Install = installFailed
		report.Reason = fmt.Sprintf("%s reported success but %s is still not on PATH", installer.Prog, headroom.Bin)
		warnUnwrapped(report, opts)
		return report
	}
	report.Wrapping, report.Path, report.Version, report.Install = true, p, headroom.Version(p), installInstalled
	render.OK(strings.TrimSpace(headroom.Bin + " installed: " + p + " " + report.Version))
	return report
}

// warnUnwrapped says, once, why the agent is starting without compression. It is
// a warning rather than a status line because the run is about to do less than
// the user asked for, and silence there is how somebody spends a month wondering
// why Headroom never seemed to help.
func warnUnwrapped(report *headroomReport, opts headroomOptions) {
	if opts.quiet {
		return
	}
	render.Warn(fmt.Sprintf("starting without %s: %s", headroom.Bin, report.Reason))
	if report.Install != installFailed {
		render.Detail("  install it with: " + headroom.InstallHint())
	}
}

// launchHarness picks which harness to start.
//
// An explicit name wins, and must be one this workspace was actually scaffolded
// for — starting Codex in a Claude-only repo would hand the user an agent with
// none of the methodology loaded, which is the exact failure scc exists to
// prevent, and it would do it while looking like it worked. Otherwise: the only
// harness here, or a picker when a person is at the terminal, or an error naming
// the choices when nobody is.
func launchHarness(root string, positionals []string, jsonOut bool) (paths.Harness, error) {
	here := workspace.Harnesses(root)

	if len(positionals) > 1 {
		return paths.Harness{}, fmt.Errorf("expected at most one harness name, got %d: %s",
			len(positionals), strings.Join(positionals, " "))
	}
	if len(positionals) == 1 {
		h, err := paths.ParseHarness(strings.ToLower(positionals[0]))
		if err != nil {
			return paths.Harness{}, err
		}
		for _, in := range here {
			if in.ID == h.ID {
				return h, nil
			}
		}
		return paths.Harness{}, fmt.Errorf("%s is not scaffolded here (found %s); run `%s init --%s` first",
			h.ID, harnessIDs(here), prog(), h.ID)
	}

	switch len(here) {
	case 1:
		return here[0], nil
	case 0:
		// requireWorkspace has already run, so this is unreachable through the CLI.
		return paths.Harness{}, fmt.Errorf("no harness is scaffolded here; run `%s init` first", prog())
	default:
		if jsonOut || !interactive() {
			return paths.Harness{}, fmt.Errorf("this workspace has %s; name the one to start, e.g. `%s launch %s`",
				harnessIDs(here), prog(), here[0].ID)
		}
		return promptHarness(promptIn, "Which harness do you want to start?", here), nil
	}
}

// harnessIDs lists a set for an error message, in the order paths declares them.
func harnessIDs(all []paths.Harness) string {
	ids := make([]string, 0, len(all))
	for _, h := range all {
		ids = append(ids, h.ID)
	}
	return strings.Join(ids, ", ")
}

// splitPassthrough divides scc's own arguments from the agent's at the first bare
// `--`. Everything after it is passed through untouched, so `scc launch claude --
// --dangerously-skip-permissions` reaches Claude Code rather than being rejected
// here as an unknown flag.
func splitPassthrough(args []string) (own, rest []string) {
	for i, a := range args {
		if a == "--" {
			return args[:i], args[i+1:]
		}
	}
	return args, nil
}

// launchExec starts the resolved command with this terminal attached and returns
// its exit code.
//
// A child process rather than an exec(2) replacement, because Windows is a
// first-class target and has no execve: one code path that behaves identically
// everywhere beats a faster one on two platforms out of three. It is a package
// var so the tests can drive the whole command without starting a real agent.
var launchExec = func(cmd launchCommand) (int, error) {
	c := exec.Command(cmd.Bin, cmd.Args...)
	c.Dir = cmd.Dir
	// The agent owns this terminal for the length of its session: it is
	// interactive, and anything scc interposed here would break its rendering.
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode(), nil
		}
		return ExitError, err
	}
	return ExitOK, nil
}
