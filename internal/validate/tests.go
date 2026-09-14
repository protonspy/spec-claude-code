package validate

import (
	"fmt"
	"os"

	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/testrun"
)

// Tests is the delivery gate's numeric half: the project's own suite, run, and
// its coverage compared against the floor it recorded.
//
// It is the eleventh validator and the second whose subject is not a file. The
// first, attribution, reads this branch's commits; this one runs a command and
// reads what it printed. Both exist for the same reason — a rule only a person
// can check is a rule that holds until the session nobody checked.
//
// **It is off unless asked for**, which is the one thing about it that is not
// like the other ten. Those read files that are already on disk and cost
// milliseconds; this one runs a test suite, and `scc validate` sits on the
// pre-commit path where a gate that costs a minute per commit is a gate somebody
// turns off — taking the other ten with it. So `scc validate --tests` and the
// pre-push hook run it, and those are the two moments it is worth its cost: the
// second is where a branch becomes a pull request, which is where this
// methodology says the claim is actually made.
func Tests(root string) (*finding.Set, error) {
	cfg, err := testrun.Load(root)
	if err != nil {
		return nil, err
	}
	// A workspace with no command recorded is a finding here, which is the one
	// place this validator departs from "silent when the subject is absent" — and
	// it can, because it only ever runs when somebody asked for the gate. Asking
	// for a coverage gate and being told nothing is how a project ships believing
	// it has one. What keeps this from firing on every push in a workspace that
	// never wired its tests up is the hook, which enables the check only when
	// there is a command to run.
	if !cfg.Configured() {
		set := &finding.Set{}
		set.Addf(rel(root, testrun.ManifestPath(root)), 0, "tests.not-configured", "%s", instruction(
			"no test command is recorded"))
		return set, nil
	}

	// The suite's own output goes to stderr and never to stdout: this runs inside
	// `scc validate`, whose stdout may be carrying a JSON document.
	res, err := testrun.Run(root, cfg, os.Stderr)
	if err != nil {
		return nil, err
	}
	return TestFindings(root, res), nil
}

// TestFindings judges a run that has already happened.
//
// It is separate from Tests so `scc test` can print the numbers and the findings
// from one execution of the suite. Running it twice — once to report and once to
// judge — would double the cost of the slowest thing scc does, and would let the
// two answers differ on a flaky suite, which is the worst possible way to learn
// that a suite is flaky.
//
// Four findings, deliberately distinct rather than one "tests failed". A suite
// that did not run, a suite whose report scc could not read, a suite with no
// tests in it, and a suite that ran and came up short are four different things
// to go and do, and a gate that called them all one thing would send the reader
// to the same dead end four times.
func TestFindings(root string, res testrun.Result) *finding.Set {
	set := &finding.Set{}
	// Relative, like every other validator's path: a finding names a file in this
	// repository, and an absolute path under somebody's temp directory is not one.
	at := rel(root, testrun.ManifestPath(root))

	if res.TimedOut {
		set.Addf(at, 0, "tests.timed-out",
			"`%s` was still running after %s and was stopped", res.Command, testrun.Timeout)
		return set
	}
	if res.ExitCode != 0 {
		set.Addf(at, 0, "tests.failed",
			"`%s` exited %d — the suite does not pass%s", res.Command, res.ExitCode, quoted(res.Tail))
	}
	if !res.Reported {
		set.Addf(at, 0, "tests.no-report", "%s%s", instruction(
			fmt.Sprintf("`%s` printed no report", res.Command)), quoted(res.Tail))
		return set
	}
	// Zero tests with a coverage number is the shape of a suite that was deleted,
	// and a floor that let it through would be worse than no floor: it would
	// certify the one repository with nothing to certify.
	if res.Total == 0 {
		set.Addf(at, 0, "tests.none",
			"`%s` reported 0 tests — coverage of %s means nothing without them",
			res.Command, testrun.Percent(res.Coverage))
	}
	if res.Coverage < res.Floor {
		set.Addf(at, 0, "tests.below-floor",
			"coverage is %s, under the %s this workspace requires",
			testrun.Percent(res.Coverage), testrun.Percent(res.Floor))
	}
	return set
}

// instruction turns "there is no working test command" into the thing to go and
// do about it.
//
// The agent is what fixes this, and it is the reader of every finding scc emits —
// so the two states that mean *no usable command* say what to write rather than
// only what is wrong. A finding the reader cannot act on is noise wearing a useful
// shape, and this is the one finding in the product whose fix is a command line
// rather than an edit to the file it points at.
//
// The command itself is deliberately not suggested: it depends on the project's
// runner and coverage tool, which scc does not know and the agent does. What is
// stated is the contract it has to satisfy.
func instruction(what string) string {
	return what + ` — record one with ` + "`scc test set \"<command>\"`" +
		`, built from this project's own test runner and coverage tool. ` +
		`It has to print {"total": N, "coverage": P} on stdout and keep the suite's exit status.`
}

// quoted appends a command's last lines to a message, or nothing when it printed
// nothing. The tail is there because "the suite failed" with no excerpt sends the
// reader off to re-run it by hand, which is the cost this whole mechanism exists
// to remove.
func quoted(tail string) string {
	if tail == "" {
		return ""
	}
	// No colon: the messages this is appended to end in sentences, and "exit
	// status.:" is what a punctuation mark added by the wrong layer looks like.
	return "\n" + tail
}
