package validate

import (
	"fmt"
	"os"

	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/gate"
)

// Checks is the delivery gate: the project's own build, format, lint and test
// commands, run, and judged.
//
// It is the eleventh validator and the second whose subject is not a file. The
// first, attribution, reads this branch's commits; this one runs commands and
// reads what they said. Both exist for the same reason — a rule only a person can
// check is a rule that holds until the session nobody checked.
//
// **It is off unless asked for**, which is the one thing about it that is not
// like the other ten. Those read files that are already on disk and cost
// milliseconds; these run a compiler, a linter and a test suite. `scc validate`
// sits on the pre-commit path where a gate that costs a minute per commit is a
// gate somebody turns off — taking the other ten with it. So `scc validate
// --checks` and the pre-push hook run them, and those are the two moments they
// are worth their cost: the second is where a branch becomes a pull request,
// which is where this methodology says the claim is actually made.
func Checks(root string) (*finding.Set, error) {
	cfg, err := gate.Load(root)
	if err != nil {
		return nil, err
	}
	set := &finding.Set{}
	at := rel(root, gate.ManifestPath(root))

	for _, k := range gate.Kinds() {
		// A gate somebody declined is silence. A language with no formatter and no
		// linter is a real answer, and a project that recorded it has already had
		// this conversation.
		if cfg.Declined(k) {
			continue
		}
		// A gate nobody has decided on is a finding, which is the one place this
		// validator departs from "silent when the subject is absent" — and it can,
		// because it only ever runs when somebody asked for the gate. Asking for a
		// delivery gate and being told nothing is how a project ships believing it
		// has one. What keeps this from firing on every push in a workspace that
		// never wired anything up is the hook, which enables the check only once
		// the workspace has decided something.
		if !cfg.Recorded(k) {
			set.Addf(at, 0, "check.not-configured", "%s", missing(k))
			continue
		}

		// The command's own output goes to stderr and never to stdout: this runs
		// inside `scc validate`, whose stdout may be carrying a JSON document.
		res, err := gate.Run(root, k, cfg, os.Stderr)
		if err != nil {
			return nil, err
		}
		one := CheckFindings(root, res)
		set.Extend(one)
		// Stop at the first gate that failed. The four are a pipeline and each
		// later one presupposes the earlier: a lint report and a test failure over
		// a broken build are derived noise, and four minutes spent producing them
		// is four minutes nobody gets back. One finding that names the real problem
		// beats four that agree with each other.
		if !one.Empty() {
			break
		}
	}
	return set, nil
}

// CheckFindings judges a run that has already happened.
//
// Separate from Checks so `scc check` can print the numbers and the findings from
// one execution. Running a suite twice — once to report and once to judge — would
// double the cost of the slowest thing scc does, and would let the two answers
// differ on a flaky suite, which is the worst possible way to learn that a suite
// is flaky.
//
// The findings are deliberately distinct rather than one "the gate failed". A
// command that did not run, a test command whose report could not be read, a
// suite with no tests in it, and one that came up short are four different things
// to go and do, and a gate that called them all one thing would send the reader
// to the same dead end four times.
func CheckFindings(root string, res gate.Result) *finding.Set {
	set := &finding.Set{}
	at := rel(root, gate.ManifestPath(root))

	if res.TimedOut {
		set.Addf(at, 0, "check.timed-out",
			"the %s gate — `%s` — was still running after %s and was stopped",
			res.Kind, res.Command, gate.Timeout)
		return set
	}
	if res.ExitCode != 0 {
		// The gate's name carries what failed; What() describes what a *passing*
		// run means and reads as nonsense appended to a failure.
		set.Addf(at, 0, "check.failed",
			"the %s gate — `%s` — exited %d%s", res.Kind, res.Command, res.ExitCode, quoted(res.Tail))
		return set
	}
	// Everything below is the test gate's alone: the other three are judged by
	// their exit status, and a compiler that exits 0 has said all it has to say.
	if res.Kind != gate.Test {
		return set
	}
	if !res.Reported {
		set.Addf(at, 0, "tests.no-report", "%s%s", noReport(res.Command), quoted(res.Tail))
		return set
	}
	// Zero tests with a coverage number is the shape of a suite that was deleted,
	// and a floor that let it through would be worse than no floor: it would
	// certify the one repository with nothing to certify.
	if res.Total == 0 {
		set.Addf(at, 0, "tests.none",
			"`%s` reported 0 tests — coverage of %s means nothing without them",
			res.Command, gate.Percent(res.Coverage))
	}
	if res.Coverage < res.Floor {
		set.Addf(at, 0, "tests.below-floor",
			"coverage is %s, under the %s this workspace requires",
			gate.Percent(res.Coverage), gate.Percent(res.Floor))
	}
	return set
}

// missing and noReport turn "there is no working command for this gate" into the
// thing to go and do about it.
//
// The agent is what fixes this, and it is the reader of every finding scc emits —
// so the states that mean *no usable command* say what to write rather than only
// what is wrong. These are the only findings in the product whose fix is a
// command line rather than an edit to the file they point at.
//
// The command itself is deliberately not suggested: it depends on the project's
// toolchain, which scc does not know and the agent does. What is stated is the
// contract, and the way out for a project that genuinely has no such tool —
// because a gate nobody can satisfy and nobody may decline is a gate people
// delete.
func missing(k gate.Kind) string {
	what := fmt.Sprintf("no %s command is recorded — record one with `scc check set %s \"<command>\"`, "+
		"built from this project's own toolchain (%s)", k, k, k.What())
	if k == gate.Test {
		what += `. It has to print {"total": N, "coverage": P} on stdout and keep the suite's exit status`
	}
	return what + fmt.Sprintf(". If this project has no %s step, say so once with `scc check skip %s`", k, k)
}

func noReport(command string) string {
	return fmt.Sprintf("`%s` printed no {\"total\": N, \"coverage\": P} object, so there is nothing to check — "+
		"make it print one on stdout, built from this project's own test runner and coverage tool", command)
}

// quoted appends a command's last lines to a message, or nothing when it printed
// nothing. The tail is there because "the gate failed" with no excerpt sends the
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
