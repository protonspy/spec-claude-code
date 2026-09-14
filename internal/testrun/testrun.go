// Package testrun is the project's own test command and the report it has to
// produce: how the command is recorded, how it is run, and how its answer is read
// back as a number a validator can compare against a floor.
//
// It exists because "the tests pass" and "enough of the code is covered" were the
// two claims in this methodology that nothing mechanical could check. A rule can
// ask for them, a reviewer can look for them, and under `autonomy: auto` nobody
// does either. The delivery gate needed a number, and a number needs a command
// that produces one.
//
// **scc does not know how to test anything, and this package is why it does not
// have to.** It runs one command the project recorded and reads one object out of
// what that command printed:
//
//	{"total": 412, "coverage": 88.4}
//
// That contract is the whole interface. A Go project pipes `go test -cover`
// through a script, a Node project reads its own coverage summary, a Makefile
// target hides either — scc never learns the difference, and the check stays
// deterministic because it is a comparison rather than a judgment.
//
// **The command is arbitrary code, and that is worth saying out loud.** It is
// recorded in `<harness>/scc-manifest.json`, which is a committed file that
// arrives with a clone, so a hostile manifest is a hostile command. Three things
// bound it: nothing here runs on a bare `scc validate`, the command is printed
// before it is run, and the hook that runs it is one the user installed in their
// own checkout. A project that would not run `make test` from a fresh clone
// should not record a command here either.
package testrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"

	"strconv"
	"strings"
	"time"

	"github.com/protonspy/spec-claude-code/internal/manifest"
	"github.com/protonspy/spec-claude-code/internal/paths"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// DefaultMinCoverage is the floor a workspace gets without saying anything: 86
// percent.
//
// It is a default rather than a constant so a project can move it, and a default
// rather than nothing because a floor nobody set is a floor nobody meets. Moving
// it down is allowed and visible — it is a number in a committed file, which is
// exactly where a weakened gate should have to be argued for.
const DefaultMinCoverage = 86.0

// Timeout bounds one run. A suite that hangs would otherwise hang the push that
// called it, and a gate that hangs is a gate somebody removes rather than debugs.
const Timeout = 10 * time.Minute

// Config is what a workspace recorded about its own tests.
type Config struct {
	// Command is the shell command that runs the suite and prints the report.
	Command string `json:"command,omitempty"`
	// MinCoverage is the floor in percent. Zero means DefaultMinCoverage — the
	// manifest key is absent in every workspace written before this existed, and
	// an absent floor has to mean the default rather than "no floor at all".
	MinCoverage float64 `json:"min_coverage,omitempty"`
}

// Configured reports whether this workspace has a test command at all.
func (c Config) Configured() bool { return strings.TrimSpace(c.Command) != "" }

// Floor is the coverage percentage this workspace has to reach.
func (c Config) Floor() float64 {
	if c.MinCoverage <= 0 {
		return DefaultMinCoverage
	}
	return c.MinCoverage
}

// Load reads the test command out of this workspace's manifests.
//
// A repo worked on from two harnesses has two manifests and one test suite, so
// the first harness that records a command wins and `Save` writes to all of them.
// Disagreeing copies are a state scc will not invent a rule for: the command is a
// property of the project, not of the tool it is being edited from.
//
// A workspace that records nothing is not an error. Absence is a normal answer
// here exactly as it is in internal/git — it means this project has not wired its
// tests up, which the caller reports or ignores depending on whether the gate was
// asked for.
func Load(root string) (Config, error) {
	for _, h := range workspace.Harnesses(root) {
		m, found, err := manifest.Load(root, h)
		if err != nil {
			return Config{}, err
		}
		if found && strings.TrimSpace(m.Test) != "" {
			return Config{Command: m.Test, MinCoverage: m.MinCoverage}, nil
		}
	}
	return Config{}, nil
}

// Save records the command in every harness manifest this workspace has, and
// reports which files it wrote.
//
// Every harness rather than one: the next command to read it takes the first that
// has it, so writing a single manifest would make the answer depend on which tool
// the reader was standing in.
func Save(root string, cfg Config) ([]string, error) {
	all := workspace.Harnesses(root)
	if len(all) == 0 {
		return nil, fmt.Errorf("%s is not an scc workspace", root)
	}
	var wrote []string
	for _, h := range all {
		m, found, err := manifest.Load(root, h)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		m.Test, m.MinCoverage = strings.TrimSpace(cfg.Command), cfg.MinCoverage
		if err := manifest.Save(root, h, m); err != nil {
			return nil, err
		}
		wrote = append(wrote, h.Manifest(root))
	}
	return wrote, nil
}

// ManifestPath is the file a finding about the test configuration points at: the
// first harness's manifest, which is where the command was read from.
func ManifestPath(root string) string {
	all := workspace.Harnesses(root)
	if len(all) == 0 {
		return paths.Claude.Manifest(root)
	}
	return all[0].Manifest(root)
}

// Report is what the command has to print: how many tests ran, and how much of
// the code they covered.
//
// Two numbers rather than one, because coverage alone is a claim that can be true
// of a suite that does not exist. Zero tests and 100 percent covered is the shape
// of a project that deleted its tests, and a floor that passed it would be worse
// than no floor.
type Report struct {
	Total    int     `json:"total"`
	Coverage float64 `json:"coverage"`
}

// Result is one run of the command, whatever became of it.
type Result struct {
	Command string `json:"command"`
	// Report is what the command printed, valid only when Reported is true.
	Report
	// Reported says a well-formed report was found in the output. False when the
	// command printed nothing scc could read, which is a different failure from a
	// suite that ran and came up short.
	Reported bool `json:"reported"`
	// ExitCode is the command's own. Non-zero is a failing suite: every runner
	// worth recording here exits non-zero when a test fails, and a wrapper that
	// swallows that is hiding the thing the gate is for.
	ExitCode int `json:"exit_code"`
	// Floor is the percentage this run had to reach, resolved from the config.
	Floor float64 `json:"floor"`
	// Tail is the last few lines of the output, kept for the message when a run
	// fails. Whole output would put a test log into a JSON document and into a
	// session's context, which is what the report exists to avoid.
	Tail string `json:"tail,omitempty"`
	// TimedOut says the command was killed at Timeout rather than finishing.
	TimedOut bool `json:"timed_out,omitempty"`
}

// Passed reports whether this run clears every bar: the suite ran, it reported,
// tests exist, and coverage reached the floor.
func (r Result) Passed() bool {
	return r.ExitCode == 0 && !r.TimedOut && r.Reported && r.Total > 0 && r.Coverage >= r.Floor
}

// Run executes the recorded command from root and reads its report back.
//
// The command's own output goes to out and never to scc's stdout, in every
// caller. stdout carries scc's document — the report, or a validator's JSON — and
// a test log interleaved with it would break the one stream machines read.
//
// An error is returned only when the command could not be started at all. A suite
// that ran and failed is a Result, not an error: it is an answer to the question,
// and the caller turns answers into findings.
func Run(root string, cfg Config, out io.Writer) (Result, error) {
	res := Result{Command: cfg.Command, Floor: cfg.Floor()}
	if !cfg.Configured() {
		return res, errors.New("no test command is recorded for this workspace")
	}

	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	cmd := command(ctx, cfg.Command)
	cmd.Dir = root
	// Captured and echoed at once: the buffer is what gets parsed, and the writer
	// is so a person watching a ten-minute suite can see it working. Both streams,
	// because a runner that prints its summary to stderr is common enough that
	// parsing only stdout would fail on it for no reason the user could see.
	var buf strings.Builder
	cmd.Stdout = io.MultiWriter(&buf, out)
	cmd.Stderr = io.MultiWriter(&buf, out)

	err := cmd.Run()
	res.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		res.ExitCode = exit.ExitCode()
	case res.TimedOut:
		res.ExitCode = -1
	default:
		return res, fmt.Errorf("could not run %q: %w", cfg.Command, err)
	}

	res.Tail = tail(buf.String(), 12)
	if rep, ok := Parse(buf.String()); ok {
		res.Report, res.Reported = rep, true
	}
	return res, nil
}

// Parse finds the report in a command's output.
//
// The whole output first, then the last line that is a JSON object — in that
// order, because a script whose only output is the report is the clean case and a
// test runner that prints a hundred lines before it is the common one. Last
// rather than first: a report is a summary, and a summary comes after what it
// summarizes.
func Parse(out string) (Report, bool) {
	if r, ok := object(strings.TrimSpace(out)); ok {
		return r, true
	}
	lines := strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "{") {
			continue
		}
		if r, ok := object(line); ok {
			return r, true
		}
	}
	return Report{}, false
}

// object reads one JSON object as a report. A candidate without a coverage
// number is not a report — some other object the command happened to print — so
// it is refused rather than read as zero percent, which would fail the gate for
// the wrong reason.
func object(s string) (Report, bool) {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return Report{}, false
	}
	cov, ok := number(raw["coverage"])
	if !ok {
		return Report{}, false
	}
	total, _ := number(raw["total"])
	return Report{Total: int(total), Coverage: cov}, true
}

// number reads a JSON value that is meant to be a number but may have been
// printed as one by a shell script: 88.4, "88.4", or "88.4%" all mean the same
// thing, and a gate that rejected the last two would be a gate people fight.
func number(raw json.RawMessage) (float64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return f, true
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return 0, false
	}
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "%"))
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// tail keeps the last n non-empty lines, which is where a runner puts the reason
// it failed.
func tail(out string, n int) string {
	var keep []string
	lines := strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n")
	for i := len(lines) - 1; i >= 0 && len(keep) < n; i-- {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}
		keep = append([]string{strings.TrimRight(lines[i], " \t")}, keep...)
	}
	return strings.Join(keep, "\n")
}

// Percent formats a coverage number the way every message here prints it: one
// decimal, no trailing zero noise beyond that, so 86 and 86.4 both read as
// numbers a person typed.
func Percent(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64) + "%"
}
