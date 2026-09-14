// Package gate is the project's own commands — build, test, lint, format — as
// something scc can run and judge rather than something a rule can only ask for.
//
// It exists because the delivery sequence's first step was the one step nothing
// could check. A rule can say "full suite and lint on the integrated branch"; a
// reviewer can look; under `autonomy: auto` nobody does either, and the step that
// certifies the work was the one certified by nobody.
//
// **scc does not know how to build, test, or lint anything, and this package is
// why it does not have to.** Each gate is one command the project recorded, run
// from the workspace root, judged by its exit status. The test gate asks for one
// thing more — a report it can compare against a floor:
//
//	{"total": 412, "coverage": 88.4}
//
// That is the whole interface. A Go project pipes `go test -cover` through a
// script, a Node project reads its own coverage summary, a Makefile target hides
// either — nothing here learns the difference, which is what keeps the check
// deterministic: it is a comparison, not a judgment.
//
// **A gate nobody recorded and a gate somebody declined are different states**,
// and keeping them apart is the point of Skipped. A language with no formatter
// and no linter is a real answer, and a project that records it says so once; a
// gate left unset is a decision nobody has made yet, and the validator asks for
// it. Collapsing the two would either nag every project that has no linter or go
// quiet about every project that forgot one.
//
// **The commands are arbitrary code, and that is worth saying out loud.** They
// are recorded in `<harness>/scc-manifest.json`, which is a committed file that
// arrives with a clone, so a hostile manifest is a hostile command. Three things
// bound it: nothing here runs on a bare `scc validate`, each command is printed
// before it runs, and the hook that runs them is one the user installed in their
// own checkout. A project you would not `make test` from a fresh clone is not one
// to record commands in.
package gate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/protonspy/spec-claude-code/internal/manifest"
	"github.com/protonspy/spec-claude-code/internal/paths"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// Kind is one gate.
type Kind string

// The four, in the order they run.
//
// Build first because nothing else means anything if the code does not compile —
// a lint report and a test failure over a broken build are derived noise. Then
// cheapest to dearest: a format check is seconds, a linter is tens of seconds, a
// suite is minutes. A run stops at the first failure for the same reason the
// order exists.
const (
	Build  Kind = "build"
	Test   Kind = "test"
	Lint   Kind = "lint"
	Format Kind = "format"
)

// Kinds returns the gates in run order.
func Kinds() []Kind { return []Kind{Build, Format, Lint, Test} }

// ParseKind resolves a gate name from the command line.
func ParseKind(s string) (Kind, error) {
	want := strings.ToLower(strings.TrimSpace(s))
	for _, k := range Kinds() {
		if string(k) == want {
			return k, nil
		}
	}
	return "", fmt.Errorf("unknown gate %q (want %s)", s, strings.Join(KindNames(), ", "))
}

// KindNames lists the gates for help text and errors.
func KindNames() []string {
	out := make([]string, 0, len(Kinds()))
	for _, k := range Kinds() {
		out = append(out, string(k))
	}
	return out
}

// What is the one line a report prints beside a gate.
func (k Kind) What() string {
	switch k {
	case Build:
		return "the project compiles"
	case Test:
		return "the suite passes, and covers enough of the code"
	case Lint:
		return "the best-practices layer that finds what tests do not"
	case Format:
		return "the formatter has nothing to say"
	}
	return ""
}

// Skipped is the value a gate carries when a project has decided it has none.
//
// A word rather than an absent key, because absent already means something else:
// nobody has decided. A language with no formatter and no linter is a real
// answer and it is recorded like one, so the validator can stay quiet about it
// without going quiet about the project that simply forgot.
const Skipped = "skipped"

// DefaultMinCoverage is the floor a workspace gets without saying anything: 86
// percent.
//
// A default rather than a constant so a project can move it, and a default rather
// than nothing because a floor nobody set is a floor nobody meets. Moving it down
// is allowed and visible — it is a number in a committed file, which is exactly
// where a weakened gate should have to be argued for.
const DefaultMinCoverage = 86.0

// Timeout bounds one run. A suite or a linter that hangs would otherwise hang the
// push that called it, and a gate that hangs is a gate somebody removes rather
// than debugs.
const Timeout = 10 * time.Minute

// Config is what a workspace recorded about its own commands.
type Config struct {
	// Commands is the recorded command per gate. An absent entry is a gate
	// nobody has decided on; Skipped is one somebody declined.
	Commands map[Kind]string `json:"commands,omitempty"`
	// MinCoverage is the test gate's floor in percent. Zero means
	// DefaultMinCoverage — the manifest key is absent in every workspace written
	// before this existed, and an absent floor has to mean the default.
	MinCoverage float64 `json:"min_coverage,omitempty"`
}

// Command is what this gate runs, or "" when nothing is recorded.
func (c Config) Command(k Kind) string { return strings.TrimSpace(c.Commands[k]) }

// Recorded reports whether this gate has a command to run — a skip is a decision,
// not a command.
func (c Config) Recorded(k Kind) bool {
	v := c.Command(k)
	return v != "" && v != Skipped
}

// Declined reports whether this gate was deliberately skipped.
func (c Config) Declined(k Kind) bool { return c.Command(k) == Skipped }

// Decided reports whether anything at all has been said about this gate.
func (c Config) Decided(k Kind) bool { return c.Command(k) != "" }

// Any reports whether this workspace has decided anything about any gate, which
// is what the pre-push hook asks before enabling the check at all.
func (c Config) Any() bool {
	for _, k := range Kinds() {
		if c.Decided(k) {
			return true
		}
	}
	return false
}

// Floor is the coverage percentage the test gate has to reach.
func (c Config) Floor() float64 {
	if c.MinCoverage <= 0 {
		return DefaultMinCoverage
	}
	return c.MinCoverage
}

// Load reads the recorded commands out of this workspace's manifests.
//
// A repo worked on from two harnesses has two manifests and one set of commands,
// so the first harness that records anything wins and Save writes to all of them.
// Disagreeing copies are a state scc will not invent a rule for: these are
// properties of the project, not of the tool it is being edited from.
//
// A workspace that records nothing is not an error. Absence is a normal answer
// here exactly as it is in internal/git — it means this project has not wired its
// commands up, which the caller reports or ignores depending on whether the gate
// was asked for.
func Load(root string) (Config, error) {
	for _, h := range workspace.Harnesses(root) {
		m, found, err := manifest.Load(root, h)
		if err != nil {
			return Config{}, err
		}
		if !found {
			continue
		}
		cfg := fromManifest(m)
		if cfg.Any() {
			return cfg, nil
		}
	}
	return Config{}, nil
}

// Save records the commands in every harness manifest this workspace has, and
// reports which files it wrote.
//
// Every harness rather than one: the next command to read them takes the first
// that has any, so writing a single manifest would make the answer depend on
// which tool the reader was standing in.
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
		toManifest(m, cfg)
		if err := manifest.Save(root, h, m); err != nil {
			return nil, err
		}
		wrote = append(wrote, h.Manifest(root))
	}
	return wrote, nil
}

// fromManifest and toManifest are the only places that know which manifest key
// holds which gate. Flat keys rather than a nested object, because the manifest
// is read by people in a diff and `"lint": "skipped"` is the whole state at a
// glance.
func fromManifest(m *manifest.Manifest) Config {
	cfg := Config{Commands: map[Kind]string{}, MinCoverage: m.MinCoverage}
	for k, v := range map[Kind]string{Build: m.Build, Test: m.Test, Lint: m.Lint, Format: m.Format} {
		if strings.TrimSpace(v) != "" {
			cfg.Commands[k] = strings.TrimSpace(v)
		}
	}
	return cfg
}

func toManifest(m *manifest.Manifest, cfg Config) {
	m.Build, m.Test = cfg.Command(Build), cfg.Command(Test)
	m.Lint, m.Format = cfg.Command(Lint), cfg.Command(Format)
	m.MinCoverage = cfg.MinCoverage
}

// ManifestPath is the file a finding about the configuration points at: the first
// harness's manifest, which is where the commands were read from.
func ManifestPath(root string) string {
	all := workspace.Harnesses(root)
	if len(all) == 0 {
		return paths.Claude.Manifest(root)
	}
	return all[0].Manifest(root)
}

// Report is what the test gate's command has to print: how many tests ran, and
// how much of the code they covered.
//
// Two numbers rather than one, because coverage alone is a claim that can be true
// of a suite that does not exist. Zero tests and 100 percent covered is the shape
// of a project that deleted its tests, and a floor that passed it would be worse
// than no floor.
type Report struct {
	Total    int     `json:"total"`
	Coverage float64 `json:"coverage"`
}

// Result is one run of one gate, whatever became of it.
type Result struct {
	Kind    Kind   `json:"gate"`
	Command string `json:"command"`
	// Report is what a test command printed, valid only when Reported is true.
	// The other three gates are judged by their exit status alone.
	Report
	// Reported says a well-formed report was found in the output.
	Reported bool `json:"reported,omitempty"`
	// ExitCode is the command's own. Non-zero is a failing gate: every tool worth
	// recording here exits non-zero when it has something to say, and a wrapper
	// that swallows that is hiding the thing the gate is for.
	ExitCode int `json:"exit_code"`
	// Floor is the percentage a test run had to reach.
	Floor float64 `json:"floor,omitempty"`
	// Tail is the last few lines of the output, kept for the message when a run
	// fails. The whole output would put a build log into a JSON document and into
	// a session's context, which is what the report exists to avoid.
	Tail string `json:"tail,omitempty"`
	// TimedOut says the command was killed at Timeout rather than finishing.
	TimedOut bool `json:"timed_out,omitempty"`
}

// Passed reports whether this run clears every bar that applies to its gate: the
// command ran, and — for the test gate — it reported, tests exist, and coverage
// reached the floor.
func (r Result) Passed() bool {
	if r.ExitCode != 0 || r.TimedOut {
		return false
	}
	if r.Kind != Test {
		return true
	}
	return r.Reported && r.Total > 0 && r.Coverage >= r.Floor
}

// Run executes one gate's recorded command from root and reads back whatever it
// has to say.
//
// The command's own output goes to out and never to scc's stdout, in every
// caller. stdout carries scc's document — the report, or a validator's JSON — and
// a build log interleaved with it would break the one stream machines read.
//
// An error is returned only when the command could not be started at all. A gate
// that ran and failed is a Result, not an error: it is an answer to the question,
// and the caller turns answers into findings.
func Run(root string, k Kind, cfg Config, out io.Writer) (Result, error) {
	res := Result{Kind: k, Command: cfg.Command(k)}
	if k == Test {
		res.Floor = cfg.Floor()
	}
	if !cfg.Recorded(k) {
		return res, fmt.Errorf("no %s command is recorded for this workspace", k)
	}

	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	cmd := command(ctx, res.Command)
	cmd.Dir = root
	// Captured and echoed at once: the buffer is what gets parsed, and the writer
	// is so a person watching a ten-minute suite can see it working. Both streams,
	// because a runner that prints its summary to stderr is common enough that
	// parsing only stdout would fail on it for no reason the user could see.
	//
	// Locked, because os/exec copies each stream on its own goroutine whenever the
	// writer is not an *os.File — so a plain buffer here is two goroutines
	// appending to one builder. Measured before the lock: a command that wrote the
	// report to stdout and one line to stderr came back with the report gone, and
	// `tests.no-report` fired on a command that had printed one.
	buf := &syncBuffer{}
	cmd.Stdout = io.MultiWriter(buf, out)
	cmd.Stderr = io.MultiWriter(buf, out)

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
		return res, fmt.Errorf("could not run %q: %w", res.Command, err)
	}

	res.Tail = tail(buf.String(), 12)
	if k == Test {
		if rep, ok := Parse(buf.String()); ok {
			res.Report, res.Reported = rep, true
		}
	}
	return res, nil
}

// Parse finds the test gate's report in a command's output.
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

// Percent formats a coverage number the way every message here prints it, so 86
// and 86.4 both read as numbers a person typed.
func Percent(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64) + "%"
}

// syncBuffer is the captured output, safe for the two goroutines os/exec uses to
// copy a command's streams.
//
// The whole type exists because of that detail: os/exec copies stdout and stderr
// on separate goroutines whenever the writer is not an *os.File, so the obvious
// `var buf strings.Builder` behind two MultiWriters is a data race, and a losing
// one — the measured symptom was a test command whose report vanished and a
// `tests.no-report` finding on a command that had printed one.
//
// Interleaving between the two streams is still whatever the command produced,
// which is the same thing a terminal shows and all the report parser needs: it
// looks for one whole line.
type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
