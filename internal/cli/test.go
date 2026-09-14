package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/render"
	"github.com/protonspy/spec-claude-code/internal/testrun"
	"github.com/protonspy/spec-claude-code/internal/validate"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// runTest is `scc test`: the project's own suite, run, and its answer read back
// as a report the delivery gate can compare against a floor.
//
// scc knows nothing about testing and this command is how it stays that way. The
// project records one command, that command prints one object, and everything
// after that is arithmetic:
//
//	{"total": 412, "coverage": 88.4}
//
// Bare, it runs and reports. `set` records the command, `show` prints what is
// recorded, `clear` takes it back out — subcommands rather than flags on the run,
// because recording a command and running it are different acts and a flag that
// silently changed the workspace's configuration on the way past would be the
// wrong shape for either.
func runTest(args []string) int {
	if len(args) == 0 {
		return runTestRun(nil)
	}
	switch args[0] {
	case "set":
		return runTestSet(args[1:])
	case "show":
		return runTestShow(args[1:])
	case "clear", "unset":
		return runTestClear(args[1:])
	case "run":
		return runTestRun(args[1:])
	case "help", "-h", "--help":
		testUsage()
		return ExitOK
	default:
		// Not an unknown-command error: a bare `scc test --json` has to work, and
		// every flag would otherwise have to be spelled after a subcommand nobody
		// wants to type.
		if strings.HasPrefix(args[0], "-") {
			return runTestRun(args)
		}
		render.Err(fmt.Sprintf("unknown test command %q", args[0]))
		testUsage()
		return ExitError
	}
}

func testUsage() {
	fmt.Fprintf(os.Stderr, `%s test — the suite, as a number the delivery gate can check

Usage:
  %s test [--json]            Run the recorded command; exit 2 below the floor
  %s test set "<command>"     Record the command (--min <percent> sets the floor)
  %s test show                Print what this workspace records
  %s test clear               Remove it

The command has to print one JSON object, anywhere in its output:

  {"total": 412, "coverage": 88.4}

total is how many tests ran and coverage is the percentage — a string like
"88.4%%" is read the same as the number. Zero tests is a finding whatever the
coverage says, since a suite that does not exist covers nothing.

The floor is %s%% unless this workspace records another. It is checked by
%s validate --tests and by the pre-push hook, which is where a branch becomes a
pull request; a bare %s validate never runs your suite, because a gate that
costs a minute per commit is a gate somebody turns off.

The command is stored in <harness>/scc-manifest.json and run through your shell.
It is committed, so treat it the way you treat a Makefile target: something a
fresh clone of this repository is expected to run.
`, prog(), prog(), prog(), prog(), prog(),
		testrun.Percent(testrun.DefaultMinCoverage), prog(), prog())
}

// testReport is the frozen JSON shape of a run, and the same values the human
// lines are printed from, so the two cannot describe different outcomes.
type testReport struct {
	testrun.Result
	// Passed is redundant with the numbers and written down anyway: it is the
	// field a shell one-liner reads, and it survives a caller that never looks at
	// the rest.
	Passed bool `json:"passed"`
	// Findings is what `scc validate --tests` would report for this same run, so
	// the two commands cannot disagree about whether a run is acceptable.
	finding.Document
}

func runTestRun(args []string) int {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if !noPositionals(rest, "test") {
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}
	cfg, err := testrun.Load(target)
	if err != nil {
		render.Err(fmt.Sprintf("test: %v", err))
		return ExitError
	}
	if !cfg.Configured() {
		// A usage error rather than a finding: somebody typed the command that runs
		// the suite, and there is no suite to run. `scc validate --tests` is where
		// the same state is a finding, because there it is an answer about the
		// workspace rather than a command that could not do what it was asked.
		render.Err("no test command is recorded for this workspace")
		render.Detail(fmt.Sprintf("  record one with: %s test set \"<command>\"", prog()))
		return ExitError
	}

	// Said before it runs, always. The command comes out of a committed file and
	// goes into a shell, so the line that says what is about to execute is part of
	// the contract rather than a status nicety.
	if !*jsonOut {
		render.Info("running: " + cfg.Command)
	}
	// The suite's own output goes to stderr in both streams: stdout carries scc's
	// report and nothing else, which is what lets `scc test --json` be piped.
	res, err := testrun.Run(target, cfg, os.Stderr)
	if err != nil {
		render.Err(fmt.Sprintf("test: %v", err))
		return ExitError
	}

	// The findings come from the validator rather than from a second copy of the
	// rules here — and from this run rather than from a second one, so `scc test`
	// and `scc validate --tests` cannot disagree about the same numbers and a flaky
	// suite cannot be reported passing and failing in one command.
	set := validate.TestFindings(target, res)

	if *jsonOut {
		if code := emitJSON(testReport{Result: res, Passed: set.Empty(), Document: set.Document()}); code != ExitOK {
			return code
		}
		return set.ExitCode()
	}
	reportTestRun(res)
	set.Report("test")
	return set.ExitCode()
}

// reportTestRun prints the numbers, whatever they turned out to be. The findings
// are printed after it by the caller: this says what the run measured, and the
// findings say what is wrong with it.
func reportTestRun(res testrun.Result) {
	if !res.Reported {
		return
	}
	line := fmt.Sprintf("%d tests, %s covered (floor %s)",
		res.Total, testrun.Percent(res.Coverage), testrun.Percent(res.Floor))
	if res.Coverage < res.Floor || res.Total == 0 {
		render.Warn(line)
		return
	}
	render.OK(line)
}

func runTestSet(args []string) int {
	fs := flag.NewFlagSet("test set", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	min := fs.Float64("min", 0, "the coverage `floor` in percent (default: the built-in 86)")
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}

	cfg, err := testrun.Load(target)
	if err != nil {
		render.Err(fmt.Sprintf("test set: %v", err))
		return ExitError
	}
	// The command is every remaining positional joined, so both `scc test set "go
	// test ./..."` and the same line without quotes record the same thing. A shell
	// that split the argument is a shell the user cannot see, and a command stored
	// as its first word would fail on the run rather than here.
	if len(rest) > 0 {
		cfg.Command = strings.Join(rest, " ")
	}
	// A --min with no command is how a floor is raised on a workspace that already
	// has one; a command with no --min leaves the floor where it was.
	if fs.Lookup("min").Value.String() != "0" {
		cfg.MinCoverage = *min
	}
	if !cfg.Configured() {
		render.Err("test set: name the command to run, e.g. `" + prog() + " test set \"make test-report\"`")
		return ExitError
	}
	if cfg.MinCoverage < 0 || cfg.MinCoverage > 100 {
		render.Err(fmt.Sprintf("test set: --min is a percentage, so %s is not one", testrun.Percent(cfg.MinCoverage)))
		return ExitError
	}

	wrote, err := testrun.Save(target, cfg)
	if err != nil {
		render.Err(fmt.Sprintf("test set: %v", err))
		return ExitError
	}
	if *jsonOut {
		return emitJSON(struct {
			testrun.Config
			Floor float64  `json:"floor"`
			Files []string `json:"files"`
		}{cfg, cfg.Floor(), relAll(wrote)})
	}
	for _, f := range wrote {
		render.OK(fmt.Sprintf("%s — test: %s", finding.Rel(workspace.Relative(mustCwd(), f)), cfg.Command))
	}
	render.Info(fmt.Sprintf("coverage floor: %s", testrun.Percent(cfg.Floor())))
	return ExitOK
}

func runTestShow(args []string) int {
	fs := flag.NewFlagSet("test show", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if !noPositionals(rest, "test show") {
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}
	cfg, err := testrun.Load(target)
	if err != nil {
		render.Err(fmt.Sprintf("test show: %v", err))
		return ExitError
	}
	if *jsonOut {
		return emitJSON(struct {
			testrun.Config
			Floor      float64 `json:"floor"`
			Configured bool    `json:"configured"`
		}{cfg, cfg.Floor(), cfg.Configured()})
	}
	if !cfg.Configured() {
		render.Warn("no test command is recorded")
		render.Detail(fmt.Sprintf("  record one with: %s test set \"<command>\"", prog()))
		return ExitOK
	}
	render.Info("command: " + cfg.Command)
	render.Info("floor:   " + testrun.Percent(cfg.Floor()))
	return ExitOK
}

func runTestClear(args []string) int {
	fs := flag.NewFlagSet("test clear", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	root := addRoot(fs)
	jsonOut := addJSON(fs)
	rest, err := parseFlags(fs, args)
	if err != nil {
		return ExitError
	}
	if !noPositionals(rest, "test clear") {
		return ExitError
	}
	target, ok := resolveRoot(*root)
	if !ok || !requireWorkspace(target) {
		return ExitError
	}
	wrote, err := testrun.Save(target, testrun.Config{})
	if err != nil {
		render.Err(fmt.Sprintf("test clear: %v", err))
		return ExitError
	}
	if *jsonOut {
		return emitJSON(struct {
			Files []string `json:"files"`
		}{relAll(wrote)})
	}
	for _, f := range wrote {
		render.OK(fmt.Sprintf("%s — test command removed", finding.Rel(workspace.Relative(mustCwd(), f))))
	}
	return ExitOK
}

// relAll makes a list of absolute paths printable and stable across machines,
// which is what a JSON consumer needs from a field naming files in a repository.
func relAll(paths []string) []string {
	out := make([]string, 0, len(paths))
	cwd := mustCwd()
	for _, p := range paths {
		if cwd != "" {
			p = workspace.Relative(cwd, p)
		}
		out = append(out, finding.Rel(p))
	}
	return out
}
