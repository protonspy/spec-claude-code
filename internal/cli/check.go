package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/gate"
	"github.com/protonspy/spec-claude-code/internal/render"
	"github.com/protonspy/spec-claude-code/internal/validate"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// runCheck is `scc check`: the project's own build, format, lint and test
// commands, run and judged.
//
// scc knows nothing about building, linting or testing, and this command is how
// it stays that way. The project records one command per gate, each is judged by
// its exit status, and the test gate prints one object more:
//
//	{"total": 412, "coverage": 88.4}
//
// Bare, it runs every recorded gate in order. `set` records one, `skip` declines
// one for a project that has no such step, `show` prints what is recorded, and
// `clear` un-decides. Subcommands rather than flags on the run, because recording
// a command and running it are different acts and a flag that silently changed
// the workspace's configuration on the way past would be the wrong shape for
// either.
func runCheck(args []string) int {
	if len(args) == 0 {
		return runCheckRun(nil)
	}
	switch args[0] {
	case "set":
		return runCheckSet(args[1:])
	case "skip":
		return runCheckSkip(args[1:])
	case "show":
		return runCheckShow(args[1:])
	case "clear", "unset":
		return runCheckClear(args[1:])
	case "run":
		return runCheckRun(args[1:])
	case "help", "-h", "--help":
		checkUsage()
		return ExitOK
	default:
		// A gate name runs that gate, and a flag runs them all: `scc check test`
		// and `scc check --json` both have to work without a subcommand nobody
		// wants to type.
		if strings.HasPrefix(args[0], "-") {
			return runCheckRun(args)
		}
		if _, err := gate.ParseKind(args[0]); err == nil {
			return runCheckRun(args)
		}
		render.Err(fmt.Sprintf("unknown check command %q", args[0]))
		checkUsage()
		return ExitError
	}
}

func checkUsage() {
	fmt.Fprintf(os.Stderr, `%s check — the project's own commands, as a gate

Usage:
  %s check [<gate>] [--json]      Run every recorded gate, or one; exit 2 on findings
  %s check set <gate> "<command>" Record a gate's command (--min <percent> for test)
  %s check skip <gate>            This project has no such step; the gate goes quiet
  %s check show                   Print what this workspace records
  %s check clear <gate>           Un-decide a gate

Gates, in the order they run:
  build    %s
  format   %s
  lint     %s
  test     %s

Build first because nothing else means anything if the code does not compile,
then cheapest to dearest. A run stops at the first gate that fails: a lint
report over a broken build is derived noise.

The test gate has to print one JSON object, anywhere in its output:

  {"total": 412, "coverage": 88.4}

total is how many tests ran and coverage is the percentage — a string like
"88.4%%" is read the same as the number. Zero tests is a finding whatever the
coverage says, since a suite that does not exist covers nothing. The floor is
%s%% unless this workspace records another.

A gate nobody has recorded is a finding; one you have skipped is silence. That
is the difference between a project that has no formatter and a project that
forgot to say what its formatter is.

These are checked by %s validate --checks and by the pre-push hook, which is
where a branch becomes a pull request; a bare %s validate never runs them,
because a gate that costs a minute per commit is a gate somebody turns off.

The commands are stored in <harness>/scc-manifest.json and run through your
shell. They are committed, so treat them the way you treat a Makefile target:
something a fresh clone of this repository is expected to run.
`, prog(), prog(), prog(), prog(), prog(), prog(),
		gate.Build.What(), gate.Format.What(), gate.Lint.What(), gate.Test.What(),
		gate.Percent(gate.DefaultMinCoverage), prog(), prog())
}

// checkReport is the frozen JSON shape of a run, and the same values the human
// lines are printed from, so the two cannot describe different outcomes.
type checkReport struct {
	// Gates is one entry per gate that ran, in order. A gate that was skipped or
	// never reached contributes nothing: the list is what happened, not what
	// could have.
	Gates []gate.Result `json:"gates"`
	// Skipped names the gates this workspace has declined, so a consumer can tell
	// "clean" from "nothing ran".
	Skipped []string `json:"skipped,omitempty"`
	// Passed is redundant with the findings and written down anyway: it is the
	// field a shell one-liner reads.
	Passed bool `json:"passed"`
	finding.Document
}

func runCheckRun(args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return exitFor(err)
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}
	cfg, err := gate.Load(target)
	if err != nil {
		render.Err(fmt.Sprintf("check: %v", err))
		return ExitError
	}

	// A named gate runs alone; no name runs the pipeline.
	want := gate.Kinds()
	if len(rest) > 0 {
		one, err := gate.ParseKind(rest[0])
		if err != nil {
			render.Err(err.Error())
			return ExitError
		}
		if len(rest) > 1 {
			render.Err(fmt.Sprintf("check: expected at most one gate, got %s", strings.Join(rest, " ")))
			return ExitError
		}
		want = []gate.Kind{one}
	}

	report := checkReport{Gates: []gate.Result{}}
	set := &finding.Set{}
	for _, k := range want {
		if cfg.Declined(k) {
			report.Skipped = append(report.Skipped, string(k))
			if !*jsonOut {
				render.Info(fmt.Sprintf("%-7s skipped", k))
			}
			continue
		}
		if !cfg.Recorded(k) {
			// Naming a gate that is not recorded is a usage error: somebody typed
			// the command that runs it and there is nothing to run. Reached through
			// the pipeline it is a finding instead — there the question is about the
			// workspace rather than about a command that could not do as asked.
			if len(want) == 1 {
				render.Err(fmt.Sprintf("no %s command is recorded for this workspace", k))
				render.Detail(fmt.Sprintf("  record one with: %s check set %s \"<command>\"", prog(), k))
				render.Detail(fmt.Sprintf("  or, if this project has no %s step: %s check skip %s", k, prog(), k))
				return ExitError
			}
			set.Addf(finding.Rel(gate.ManifestPath(target)), 0, "check.not-configured",
				"no %s command is recorded; `%s check set %s \"<command>\"`, or `%s check skip %s`",
				k, prog(), k, prog(), k)
			continue
		}

		// Said before it runs, always. The command comes out of a committed file
		// and goes into a shell, so the line that says what is about to execute is
		// part of the contract rather than a status nicety.
		if !*jsonOut {
			render.Info(fmt.Sprintf("%-7s %s", k, cfg.Command(k)))
		}
		// The command's own output goes to stderr in both streams: stdout carries
		// scc's report and nothing else, which is what lets this be piped.
		res, err := gate.Run(target, k, cfg, os.Stderr)
		if err != nil {
			render.Err(fmt.Sprintf("check: %v", err))
			return ExitError
		}
		report.Gates = append(report.Gates, res)
		// The findings come from the validator rather than from a second copy of
		// the rules here — and from this run rather than from a second one, so
		// `scc check` and `scc validate --checks` cannot disagree about the same
		// numbers and a flaky suite cannot be reported passing and failing in one
		// command.
		one := validate.CheckFindings(target, res)
		set.Extend(one)
		if !*jsonOut {
			reportGate(res)
		}
		if !one.Empty() {
			// Same pipeline rule the validator follows: a later gate presupposes the
			// earlier one, so there is nothing to learn from running it.
			break
		}
	}

	report.Passed, report.Document = set.Empty(), set.Document()
	if *jsonOut {
		if code := emitJSON(report); code != ExitOK {
			return code
		}
		return set.ExitCode()
	}
	set.Report("check")
	return set.ExitCode()
}

// reportGate prints what one gate measured. The findings are printed after every
// gate by the caller: this says what happened, and the findings say what is wrong
// with it.
func reportGate(res gate.Result) {
	if res.Kind != gate.Test {
		if res.Passed() {
			render.OK(fmt.Sprintf("%-7s ok", res.Kind))
		}
		return
	}
	if !res.Reported {
		return
	}
	line := fmt.Sprintf("%-7s %d tests, %s covered (floor %s)",
		res.Kind, res.Total, gate.Percent(res.Coverage), gate.Percent(res.Floor))
	if res.Coverage < res.Floor || res.Total == 0 {
		render.Warn(line)
		return
	}
	render.OK(line)
}

// runCheckSet records one gate's command.
func runCheckSet(args []string) int {
	fs := flag.NewFlagSet("check set", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	min := fs.Float64("min", 0, "the test gate's coverage `floor` in percent (default: the built-in 86)")
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return exitFor(err)
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}
	if len(rest) == 0 {
		render.Err(fmt.Sprintf("check set: name a gate (%s)", strings.Join(gate.KindNames(), ", ")))
		return ExitError
	}
	k, err := gate.ParseKind(rest[0])
	if err != nil {
		render.Err(err.Error())
		return ExitError
	}

	cfg, err := gate.Load(target)
	if err != nil {
		render.Err(fmt.Sprintf("check set: %v", err))
		return ExitError
	}
	if cfg.Commands == nil {
		cfg.Commands = map[gate.Kind]string{}
	}
	// The command is every remaining positional joined, so both `scc check set
	// test "go test ./..."` and the same line without quotes record the same
	// thing. A shell that split the argument is a shell the user cannot see, and a
	// command stored as its first word would fail on the run rather than here.
	if len(rest) > 1 {
		cfg.Commands[k] = strings.Join(rest[1:], " ")
	}
	// A --min with no command is how a floor is raised on a gate that already has
	// one; a command with no --min leaves the floor where it was.
	// Whether the caller typed it, not whether the value came out non-zero:
	// reading the parsed value makes `--min 0` indistinguishable from an absent
	// flag, and 0 is the one floor somebody sets deliberately — a project turning
	// the floor off while keeping the gate.
	if isSet(fs, "min") {
		cfg.MinCoverage = *min
	}
	if cfg.Command(k) == gate.Skipped {
		render.Err(fmt.Sprintf("check set: %q is how a declined gate is recorded; use `%s check skip %s`",
			gate.Skipped, prog(), k))
		return ExitError
	}
	if !cfg.Recorded(k) {
		render.Err(fmt.Sprintf("check set: name the command to run, e.g. `%s check set %s \"make %s\"`", prog(), k, k))
		return ExitError
	}
	if cfg.MinCoverage < 0 || cfg.MinCoverage > 100 {
		render.Err(fmt.Sprintf("check set: --min is a percentage, so %s is not one", gate.Percent(cfg.MinCoverage)))
		return ExitError
	}
	return saveConfig(target, cfg, *jsonOut, fmt.Sprintf("%s: %s", k, cfg.Command(k)))
}

// runCheckSkip records that this project has no such step.
//
// A command of its own rather than `set <gate> skipped`, because declining a gate
// and recording a command are different decisions and only one of them should be
// reachable by typing a word that happens to match.
func runCheckSkip(args []string) int {
	return decide(args, "check skip", gate.Skipped, "skipped")
}

// runCheckClear takes a gate back to undecided, which is not the same as skipping
// it: the validator will ask about it again.
func runCheckClear(args []string) int {
	return decide(args, "check clear", "", "cleared")
}

func decide(args []string, cmd, value, said string) int {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return exitFor(err)
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}
	if len(rest) != 1 {
		render.Err(fmt.Sprintf("%s: name one gate (%s)", cmd, strings.Join(gate.KindNames(), ", ")))
		return ExitError
	}
	k, err := gate.ParseKind(rest[0])
	if err != nil {
		render.Err(err.Error())
		return ExitError
	}
	cfg, err := gate.Load(target)
	if err != nil {
		render.Err(fmt.Sprintf("%s: %v", cmd, err))
		return ExitError
	}
	if cfg.Commands == nil {
		cfg.Commands = map[gate.Kind]string{}
	}
	cfg.Commands[k] = value
	return saveConfig(target, cfg, *jsonOut, fmt.Sprintf("%s: %s", k, said))
}

// saveConfig writes the configuration to every harness manifest and says where it
// went.
func saveConfig(root string, cfg gate.Config, jsonOut bool, said string) int {
	wrote, err := gate.Save(root, cfg)
	if err != nil {
		render.Err(fmt.Sprintf("check: %v", err))
		return ExitError
	}
	if jsonOut {
		return emitJSON(struct {
			gate.Config
			Floor float64  `json:"floor"`
			Files []string `json:"files"`
		}{cfg, cfg.Floor(), relAll(root, wrote)})
	}
	for _, f := range wrote {
		render.OK(fmt.Sprintf("%s — %s", finding.Rel(workspace.Relative(mustCwd(), f)), said))
	}
	return ExitOK
}

func runCheckShow(args []string) int {
	fs := flag.NewFlagSet("check show", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return exitFor(err)
	}
	if !noPositionals(rest, "check show") {
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}
	cfg, err := gate.Load(target)
	if err != nil {
		render.Err(fmt.Sprintf("check show: %v", err))
		return ExitError
	}
	if *jsonOut {
		return emitJSON(struct {
			gate.Config
			Floor float64 `json:"floor"`
			Any   bool    `json:"configured"`
		}{cfg, cfg.Floor(), cfg.Any()})
	}
	for _, k := range gate.Kinds() {
		switch {
		case cfg.Declined(k):
			render.Info(fmt.Sprintf("%-7s skipped", k))
		case cfg.Recorded(k):
			render.OK(fmt.Sprintf("%-7s %s", k, cfg.Command(k)))
		default:
			render.Warn(fmt.Sprintf("%-7s not recorded", k))
		}
	}
	if cfg.Recorded(gate.Test) {
		render.Info(fmt.Sprintf("floor   %s", gate.Percent(cfg.Floor())))
	}
	if !cfg.Any() {
		render.Detail(fmt.Sprintf("  record one with: %s check set <gate> \"<command>\"", prog()))
	}
	return ExitOK
}

// relAll makes a list of absolute paths printable and stable across machines,
// which is what a JSON consumer needs from a field naming files in a repository.
//
// Relative to the workspace root, which is what every other command's relPath
// answers. Against the working directory the same manifest came back as
// `.claude/scc-manifest.json` from the root and `../.claude/scc-manifest.json`
// from `specs/` — one field naming one file two ways, which is exactly what a
// consumer matching it against the manifest cannot do.
func relAll(root string, paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		out = append(out, relPath(root, p))
	}
	return out
}
