package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/gate"
	"github.com/protonspy/spec-claude-code/internal/manifest"
	"github.com/protonspy/spec-claude-code/internal/paths"
)

// echoJSON is a command that prints one line on either shell scc runs. The
// Windows side deliberately does not escape its quotes: cmd.exe prints them as
// typed, and a test that escaped them would be testing its own helper.
func echoJSON(total int, coverage string) string {
	return echoPlain(`{"total": ` + itoa(total) + `, "coverage": ` + coverage + `}`)
}

// echoPlain prints one line and exits 0, whatever the line is.
func echoPlain(s string) string {
	if runtime.GOOS == "windows" {
		return "echo " + s
	}
	return "printf '%s\\n' '" + s + "'"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var out []byte
	for ; n > 0; n /= 10 {
		out = append([]byte{byte('0' + n%10)}, out...)
	}
	return string(out)
}

// failing is a command that exits non-zero on either shell.
func failing() string {
	if runtime.GOOS == "windows" {
		return "exit /b 1"
	}
	return "exit 1"
}

// setTest records the test gate's command the way a user would, through the CLI,
// so these tests exercise the path that writes the manifest rather than a
// hand-built file.
func setTest(t *testing.T, root string, args ...string) {
	t.Helper()
	setGate(t, root, "test", args...)
}

// setGate records any gate, and skipGate declines one — the two states the
// validator has to tell apart.
func setGate(t *testing.T, root, kind string, args ...string) {
	t.Helper()
	if _, stderr, code := run(t, append([]string{"check", "set", kind, "--root", root}, args...)...); code != ExitOK {
		t.Fatalf("check set %s: exit = %d (stderr: %s)", kind, code, stderr)
	}
}

// The command lands in the manifest, which is the whole point of putting it
// there: one file per harness, already committed, already the workspace marker.
func TestTestSetRecordsTheCommandInTheManifest(t *testing.T) {
	root := initWorkspace(t)
	setTest(t, root, "make test-report")

	m, found, err := manifest.Load(root, paths.Claude)
	if err != nil || !found {
		t.Fatalf("Load: %v (found %v)", err, found)
	}
	if m.Test != "make test-report" {
		t.Errorf("test = %q, want the recorded command", m.Test)
	}
	// Nothing was said about the floor, so nothing is written — and the default is
	// what applies. A key that appeared with a value nobody chose would make the
	// built-in default unreadable from the file.
	if m.MinCoverage != 0 {
		t.Errorf("min_coverage = %v, want it absent", m.MinCoverage)
	}
	raw, err := os.ReadFile(paths.Claude.Manifest(root))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), `"test": "make test-report"`) {
		t.Errorf("the manifest does not carry the command:\n%s", raw)
	}
}

// A workspace with no command in it writes no key at all, so every manifest that
// existed before this feature is byte-identical after it.
func TestInitWritesTheCheckBase(t *testing.T) {
	root := initWorkspace(t)
	raw, err := os.ReadFile(paths.Claude.Manifest(root))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	// The four questions are in the file from the first command, so the first
	// person or agent to open it sees what this project has to answer. Empty, not
	// absent: `scc check set` is what fills them in.
	for _, key := range []string{`"build": ""`, `"format": ""`, `"lint": ""`, `"test": ""`} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("the scaffolded base has no %s:\n%s", key, raw)
		}
	}
	// The floor stays out until somebody sets one: it has a working default, and
	// writing it here would pin every workspace to today's number.
	if strings.Contains(string(raw), `"min_coverage"`) {
		t.Errorf("init pinned a coverage floor nobody chose:\n%s", raw)
	}
	// And the base reads as a base — one block at the top, not four keys with
	// every content hash in the workspace between them.
	if i, j := strings.Index(string(raw), `"check"`), strings.Index(string(raw), `"files"`); i < 0 || i > j {
		t.Errorf("check is not above files (%d vs %d)", i, j)
	}
	// It is also the state `scc check show` reports, so the file and the command
	// cannot disagree about what has been decided.
	// An undecided gate is a warning, so it lands on stderr — both streams together
	// are what a person sees.
	stdout, stderr, code := run(t, "check", "show", "--root", root)
	if code != ExitOK {
		t.Fatalf("check show: exit = %d", code)
	}
	if !strings.Contains(stdout+stderr, "not recorded") {
		t.Errorf("check show does not report the empty base: %q", stdout+stderr)
	}
}

// The command survives a re-scaffold. `init` and `update` both build the next
// manifest from scratch and fill in the file entries, so anything the project
// owns has to be carried across deliberately — and a gate that disappeared on the
// next `scc init` would be worse than no gate, because nobody would notice.
func TestTheTestCommandSurvivesInitAndUpdate(t *testing.T) {
	root := initWorkspace(t)
	setTest(t, root, "--min", "92", "make test-report")

	if _, stderr, code := run(t, "init", "--no-rtk", "--claude", "--root", root); code != ExitOK {
		t.Fatalf("second init: exit = %d (stderr: %s)", code, stderr)
	}
	cfg, err := gate.Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Command(gate.Test) != "make test-report" || cfg.Floor() != 92 {
		t.Errorf("config = %+v (floor %v), want it carried across init", cfg, cfg.Floor())
	}

	if _, stderr, code := run(t, "update", "--root", root, "--yes"); code != ExitOK {
		t.Fatalf("update: exit = %d (stderr: %s)", code, stderr)
	}
	cfg, err = gate.Load(root)
	if err != nil {
		t.Fatalf("Load after update: %v", err)
	}
	if cfg.Command(gate.Test) != "make test-report" || cfg.Floor() != 92 {
		t.Errorf("config = %+v (floor %v), want it carried across update", cfg, cfg.Floor())
	}
}

// One suite, two harnesses, one answer. Writing a single manifest would make the
// recorded command depend on which tool the reader happened to be standing in.
func TestTestSetWritesEveryHarnessManifest(t *testing.T) {
	root := t.TempDir()
	for _, h := range []string{"--codex", "--opencode"} {
		if _, stderr, code := run(t, "init", "--no-rtk", h, "--root", root); code != ExitOK {
			t.Fatalf("init %s: exit = %d (stderr: %s)", h, code, stderr)
		}
	}
	setTest(t, root, "make test-report")
	for _, h := range []paths.Harness{paths.Codex, paths.OpenCode} {
		m, found, err := manifest.Load(root, h)
		if err != nil || !found {
			t.Fatalf("%s: Load: %v (found %v)", h.ID, err, found)
		}
		if m.Test != "make test-report" {
			t.Errorf("%s records %q, want the command", h.ID, m.Test)
		}
	}
}

// A run that clears the floor is a pass, and the numbers are printed rather than
// only the verdict: "88.4% covered" is what tells somebody they have room, and a
// bare ✓ is what makes a gate feel arbitrary.
func TestTestRunReportsAndPasses(t *testing.T) {
	root := initWorkspace(t)
	skipRest(t, root, "test")
	setTest(t, root, echoJSON(412, "88.4"))

	stdout, stderr, code := run(t, "check", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d (stderr: %s)", code, ExitOK, stderr)
	}
	if !strings.Contains(stdout, "412 tests") || !strings.Contains(stdout, "88.4%") {
		t.Errorf("stdout does not report the numbers: %q", stdout)
	}
}

// Under the floor is exit 2 — a finding, not a crash. CI and an agent both branch
// on that code, and collapsing it into 1 would make "the coverage is low"
// indistinguishable from "scc could not run".
func TestTestRunReportsFindingsBelowTheFloor(t *testing.T) {
	root := initWorkspace(t)
	skipRest(t, root, "test")
	setTest(t, root, echoJSON(400, "72.5"))

	stdout, stderr, code := run(t, "check", "--root", root)
	if code != ExitFindings {
		t.Fatalf("exit = %d, want %d (stderr: %s)", code, ExitFindings, stderr)
	}
	if !strings.Contains(stdout+stderr, "below-floor") {
		t.Errorf("nothing named the rule that fired: %q", stdout+stderr)
	}
	// The floor it failed against is named, because "under the floor" without the
	// number is a finding nobody can act on.
	if !strings.Contains(stdout+stderr, "86%") {
		t.Errorf("the floor is not named: %q", stdout+stderr)
	}
}

// Coverage with no tests behind it is the shape of a suite that was deleted, and
// it is the one case a naive floor would certify.
func TestTestRunRejectsCoverageWithNoTests(t *testing.T) {
	root := initWorkspace(t)
	skipRest(t, root, "test")
	setTest(t, root, echoJSON(0, "100"))

	stdout, stderr, code := run(t, "check", "--root", root)
	if code != ExitFindings {
		t.Fatalf("exit = %d, want %d (stderr: %s)", code, ExitFindings, stderr)
	}
	if !strings.Contains(stdout+stderr, "tests.none") {
		t.Errorf("100%% coverage over zero tests was accepted: %q", stdout+stderr)
	}
}

// A failing suite and a suite that printed nothing readable are different
// findings, because they are different things to go and do.
func TestTestRunSeparatesFailureFromSilence(t *testing.T) {
	root := initWorkspace(t)
	skipRest(t, root, "test")

	setTest(t, root, failing())
	stdout, stderr, code := run(t, "check", "--root", root)
	if code != ExitFindings {
		t.Fatalf("failing suite: exit = %d, want %d", code, ExitFindings)
	}
	if !strings.Contains(stdout+stderr, "check.failed") {
		t.Errorf("a failing gate was not reported as one: %q", stdout+stderr)
	}

	// A suite that passes and says so in prose: green, and still no number to check.
	setTest(t, root, echoPlain("PASS"))
	stdout, stderr, code = run(t, "check", "--root", root)
	if code != ExitFindings {
		t.Fatalf("silent suite: exit = %d, want %d", code, ExitFindings)
	}
	if !strings.Contains(stdout+stderr, "tests.no-report") {
		t.Errorf("a suite that printed no report was not reported as one: %q", stdout+stderr)
	}
}

// stdout carries the document and nothing else — not the suite's own output,
// which is the loudest thing in the room on a real project.
func TestTestJSONKeepsTheSuiteOffStdout(t *testing.T) {
	root := initWorkspace(t)
	skipRest(t, root, "test")
	setTest(t, root, echoJSON(10, "99"))

	stdout, _, code := run(t, "check", "--root", root, "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d", code, ExitOK)
	}
	var report struct {
		Gates   []gate.Result `json:"gates"`
		Skipped []string      `json:"skipped"`
		Passed  bool          `json:"passed"`
		Count   int           `json:"count"`
	}
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("stdout is not valid JSON (%v): %q", err, stdout)
	}
	// One gate ran and three were declined, and the document says which — "clean"
	// and "nothing ran" are different answers and a consumer has to be able to
	// tell them apart.
	if len(report.Gates) != 1 || len(report.Skipped) != 3 {
		t.Fatalf("report = %+v, want one gate run and three declined", report)
	}
	got := report.Gates[0]
	if got.Kind != gate.Test || got.Total != 10 || got.Coverage != 99 || got.Floor != 86 {
		t.Errorf("gate = %+v, want the numbers and the floor", got)
	}
	if !report.Passed || report.Count != 0 {
		t.Errorf("report = %+v, want a clean pass", report)
	}
}

// The floor moves, and moving it is one command rather than an edit to a JSON
// file — the same reason `scc notes add` exists.
func TestTestSetRecordsTheFloor(t *testing.T) {
	root := initWorkspace(t)
	skipRest(t, root, "test")
	setTest(t, root, "--min", "95", echoJSON(100, "90"))

	_, _, code := run(t, "check", "--root", root)
	if code != ExitFindings {
		t.Fatalf("exit = %d, want %d — 90%% is under a 95%% floor", code, ExitFindings)
	}
	// And a --min with no command raises the floor on the command already there.
	setTest(t, root, "--min", "80")
	if _, stderr, code := run(t, "check", "--root", root); code != ExitOK {
		t.Fatalf("exit = %d, want %d under an 80%% floor (stderr: %s)", code, ExitOK, stderr)
	}
}

func TestTestSetRejectsAFloorThatIsNotAPercentage(t *testing.T) {
	root := initWorkspace(t)
	if _, _, code := run(t, "check", "set", "test", "--root", root, "--min", "180", "make test"); code != ExitError {
		t.Errorf("exit = %d, want %d for a floor over 100", code, ExitError)
	}
}

// show and clear are the other half of "you never hand-edit the manifest".
func TestTestShowAndClear(t *testing.T) {
	root := initWorkspace(t)
	setTest(t, root, "make test-report")

	stdout, _, code := run(t, "check", "show", "--root", root)
	if code != ExitOK || !strings.Contains(stdout, "make test-report") {
		t.Errorf("show: exit = %d, stdout = %q", code, stdout)
	}
	if _, stderr, code := run(t, "check", "clear", "test", "--root", root); code != ExitOK {
		t.Fatalf("clear: exit = %d (stderr: %s)", code, stderr)
	}
	cfg, err := gate.Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Any() {
		t.Errorf("config = %+v, want it cleared", cfg)
	}
}

// Running the suite when there is no suite recorded is a usage error, not a
// finding: somebody typed the command that runs the tests and there are none to
// run. The same state under `scc validate --checks` is a finding, because there it
// is an answer about the workspace.
func TestTestRunWithoutACommandIsAUsageError(t *testing.T) {
	root := initWorkspace(t)
	// Naming a gate that is not recorded is a usage error: somebody typed the
	// command that runs it and there is nothing to run.
	_, stderr, code := run(t, "check", "test", "--root", root)
	if code != ExitError {
		t.Fatalf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "check set") || !strings.Contains(stderr, "check skip") {
		t.Errorf("stderr does not offer both ways out: %q", stderr)
	}

	// Reached through the pipeline the same state is a finding, because there the
	// question is about the workspace rather than about a command that could not
	// do as it was asked.
	_, _, code = run(t, "check", "--root", root)
	if code != ExitFindings {
		t.Errorf("exit = %d, want %d for the whole pipeline", code, ExitFindings)
	}
}

// A gate somebody declined is silence — that is the whole difference between a
// project with no formatter and a project that forgot to say what its formatter
// is, and it is what keeps the gate usable in a language that has neither.
func TestSkippedGatesAreSilent(t *testing.T) {
	root := initWorkspace(t)
	for _, k := range gate.Kinds() {
		skipGate(t, root, string(k))
	}
	stdout, stderr, code := run(t, "validate", "--root", root, "--checks")
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d with every gate declined (stderr: %s)", code, ExitOK, stderr)
	}
	if strings.Contains(stdout+stderr, "not-configured") {
		t.Errorf("a declined gate was reported as undecided: %q", stdout+stderr)
	}

	// And clearing one puts it back to undecided, which is not the same thing:
	// the validator asks about it again.
	if _, stderr, code := run(t, "check", "clear", "lint", "--root", root); code != ExitOK {
		t.Fatalf("check clear: exit = %d (stderr: %s)", code, stderr)
	}
	stdout, stderr, code = run(t, "validate", "--root", root, "--checks")
	if code != ExitFindings {
		t.Fatalf("exit = %d, want %d once a gate is undecided again", code, ExitFindings)
	}
	if !strings.Contains(stdout+stderr, "no lint command is recorded") {
		t.Errorf("the cleared gate was not reported: %q", stdout+stderr)
	}
}

// The pipeline stops at the first gate that fails. A lint report over a broken
// build is derived noise, and the minutes spent producing it are minutes nobody
// gets back.
func TestThePipelineStopsAtTheFirstFailure(t *testing.T) {
	root := initWorkspace(t)
	marker := filepath.Join(root, "lint-ran.txt")
	setGate(t, root, "build", failing())
	setGate(t, root, "lint", "echo ran > "+filepath.ToSlash(marker))
	skipGate(t, root, "format")
	skipGate(t, root, "test")

	stdout, stderr, code := run(t, "check", "--root", root)
	if code != ExitFindings {
		t.Fatalf("exit = %d, want %d (stderr: %s)", code, ExitFindings, stderr)
	}
	if !strings.Contains(stdout+stderr, "the build gate") {
		t.Errorf("the failing gate was not named: %q", stdout+stderr)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("a later gate ran after the build failed")
	}
}

// A bare `scc validate` never runs the suite. That is the whole reason the check
// is behind an option: it sits on the pre-commit path, and a gate that costs a
// minute per commit is a gate somebody turns off — taking the other ten with it.
func TestValidateRunsTheSuiteOnlyWhenAsked(t *testing.T) {
	root := initWorkspace(t)
	skipRest(t, root, "test")
	// A command that leaves a trace, so "did it run" is a question about the
	// filesystem rather than about stdout.
	marker := filepath.Join(root, "ran.txt")
	setTest(t, root, "echo ran > "+filepath.ToSlash(marker))

	if _, stderr, code := run(t, "validate", "--root", root); code != ExitOK {
		t.Fatalf("validate: exit = %d (stderr: %s)", code, stderr)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("a bare `scc validate` ran the test suite")
	}

	// --checks asks for it, and then the command runs — and reports, since this one
	// prints no report of its own.
	stdout, stderr, code := run(t, "validate", "--root", root, "--checks")
	if code != ExitFindings {
		t.Fatalf("validate --checks: exit = %d, want %d (stderr: %s)", code, ExitFindings, stderr)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("--checks did not run the command: %v", err)
	}
	if !strings.Contains(stdout+stderr, "tests") {
		t.Errorf("the tests validator is missing from the report: %q", stdout+stderr)
	}
}

// Asking for the gate in a workspace that has no command is a finding rather than
// silence. Every other validator is quiet when its subject is absent; this one
// cannot be, because being told nothing is how a project ships believing it has a
// coverage gate.
func TestValidateTestsReportsAMissingCommand(t *testing.T) {
	root := initWorkspace(t)
	stdout, stderr, code := run(t, "validate", "--root", root, "--checks")
	if code != ExitFindings {
		t.Fatalf("exit = %d, want %d", code, ExitFindings)
	}
	if !strings.Contains(stdout+stderr, "check.not-configured") {
		t.Errorf("a gate nobody has decided on was not reported: %q", stdout+stderr)
	}
}

// skipGate declines a gate, which is the answer for a language with no formatter
// or no linter.
func skipGate(t *testing.T, root, kind string) {
	t.Helper()
	if _, stderr, code := run(t, "check", "skip", kind, "--root", root); code != ExitOK {
		t.Fatalf("check skip %s: exit = %d (stderr: %s)", kind, code, stderr)
	}
}

// skipRest declines every gate but one, so a test about that gate is not drowned
// in findings about the other three.
func skipRest(t *testing.T, root, keep string) {
	t.Helper()
	for _, k := range gate.Kinds() {
		if string(k) != keep {
			skipGate(t, root, string(k))
		}
	}
}

// A finding about the configuration points at the manifest, and it names it
// relative to the workspace root — which is what every other command's relPath
// answers. Against the working directory the same file came back as
// `.claude/scc-manifest.json` from the root and `../.claude/scc-manifest.json`
// from `specs/`, one field naming one file two ways.
func TestTheWrittenManifestsAreNamedFromTheRoot(t *testing.T) {
	root := initWorkspace(t)

	stdout, stderr, code := run(t, "check", "set", "build", "go build ./...", "--root", root, "--json")
	if code != ExitOK {
		t.Fatalf("check set: exit %d (%s)", code, stderr)
	}
	var doc struct {
		Wrote []string `json:"files"`
	}
	decode(t, stdout, &doc)
	if len(doc.Wrote) == 0 {
		t.Fatalf("check set reported no files written: %s", stdout)
	}
	for _, p := range doc.Wrote {
		if filepath.IsAbs(p) {
			t.Errorf("a written path is absolute: %q", p)
		}
		if strings.HasPrefix(p, "..") {
			t.Errorf("a written path escapes the workspace: %q", p)
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(p))); err != nil {
			t.Errorf("the reported path does not resolve under the root: %q", p)
		}
	}
}
