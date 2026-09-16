package gate

import (
	"os"

	"github.com/protonspy/spec-claude-code/internal/assets"
	"github.com/protonspy/spec-claude-code/internal/manifest"
	"github.com/protonspy/spec-claude-code/internal/paths"
	"io"
	"runtime"
	"strings"
	"testing"
)

// The report is the whole interface between scc and a project's tests, so what
// counts as one is the thing most worth pinning down. Every case here is a shape a
// real runner or a three-line wrapper script actually prints.
func TestParseFindsTheReport(t *testing.T) {
	cases := []struct {
		name  string
		out   string
		want  Report
		found bool
	}{
		{"bare object", `{"total": 412, "coverage": 88.4}`, Report{412, 88.4}, true},
		{"trailing newline", "{\"total\":1,\"coverage\":100}\n", Report{1, 100}, true},
		{
			// The common case: a runner prints its log and the wrapper appends one line.
			name:  "after a log",
			out:   "ok  pkg/a  0.02s\nok  pkg/b  1.10s\n{\"total\": 7, \"coverage\": 91.2}\n",
			want:  Report{7, 91.2},
			found: true,
		},
		{
			// Last, not first: a summary comes after what it summarizes, and a runner
			// that prints a per-package object would otherwise win over the total.
			name:  "the last object wins",
			out:   "{\"total\": 1, \"coverage\": 10}\n{\"total\": 9, \"coverage\": 99}\n",
			want:  Report{9, 99},
			found: true,
		},
		// A shell script building JSON by hand quotes everything, and `go tool cover`
		// prints a percent sign. Rejecting either would be a gate people fight rather
		// than use.
		{"quoted numbers", `{"total": "412", "coverage": "88.4"}`, Report{412, 88.4}, true},
		{"percent sign", `{"total": 412, "coverage": "88.4%"}`, Report{412, 88.4}, true},
		{"zero tests is still a report", `{"total": 0, "coverage": 0}`, Report{0, 0}, true},

		{"nothing at all", "", Report{}, false},
		{"no json", "PASS\nok  pkg  0.2s\n", Report{}, false},
		{"not an object", `[1, 2, 3]`, Report{}, false},
		// An object with no coverage is some other thing the command printed. Reading
		// it as zero percent would fail the gate for a reason the user cannot see.
		{"object without coverage", `{"total": 5, "elapsed": 1.2}`, Report{}, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := Parse(c.out)
			if ok != c.found {
				t.Fatalf("found = %v, want %v (out: %q)", ok, c.found, c.out)
			}
			if ok && got != c.want {
				t.Errorf("report = %+v, want %+v", got, c.want)
			}
		})
	}
}

// The floor is a default rather than nothing, because a floor nobody set is a
// floor nobody meets — and absent has to keep meaning the default, since every
// manifest written before this key existed has no value in it.
func TestFloorDefaults(t *testing.T) {
	if got := (Config{}).Floor(); got != DefaultMinCoverage {
		t.Errorf("floor = %v, want the default %v", got, DefaultMinCoverage)
	}
	if got := (Config{MinCoverage: 95}).Floor(); got != 95 {
		t.Errorf("floor = %v, want the recorded 95", got)
	}
	// Zero is "unset", not "no floor at all": a workspace cannot turn the gate off
	// by recording a number that happens to be zero.
	if got := (Config{MinCoverage: 0}).Floor(); got != DefaultMinCoverage {
		t.Errorf("floor = %v, want the default for an unset value", got)
	}
}

// A gate nobody recorded and a gate somebody declined are different states, and
// keeping them apart is the whole reason Skipped is a word rather than an absent
// key: one is "decide this", the other is "already decided, there is none".
func TestRecordedDeclinedDecided(t *testing.T) {
	for _, c := range []struct {
		cmd                                string
		recorded, declined, decided, anyOf bool
	}{
		{"", false, false, false, false},
		{"   ", false, false, false, false},
		{"make lint", true, false, true, true},
		{Skipped, false, true, true, true},
	} {
		cfg := Config{Commands: map[Kind]string{Lint: c.cmd}}
		if got := cfg.Recorded(Lint); got != c.recorded {
			t.Errorf("Recorded(%q) = %v, want %v", c.cmd, got, c.recorded)
		}
		if got := cfg.Declined(Lint); got != c.declined {
			t.Errorf("Declined(%q) = %v, want %v", c.cmd, got, c.declined)
		}
		if got := cfg.Decided(Lint); got != c.decided {
			t.Errorf("Decided(%q) = %v, want %v", c.cmd, got, c.decided)
		}
		if got := cfg.Any(); got != c.anyOf {
			t.Errorf("Any() with lint %q = %v, want %v", c.cmd, got, c.anyOf)
		}
	}
}

// The gates run build-first, then cheapest to dearest. The order is the pipeline,
// so it is worth pinning: a lint report over a broken build is derived noise, and
// a suite that runs before a format check spends minutes to learn nothing.
func TestKindsRunInPipelineOrder(t *testing.T) {
	want := []Kind{Build, Format, Lint, Test}
	got := Kinds()
	if len(got) != len(want) {
		t.Fatalf("Kinds() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Kinds()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	for _, k := range got {
		if k.What() == "" {
			t.Errorf("%s has no line describing it", k)
		}
		if _, err := ParseKind(strings.ToUpper(string(k))); err != nil {
			t.Errorf("ParseKind(%q): %v", k, err)
		}
	}
	if _, err := ParseKind("vibes"); err == nil {
		t.Error("ParseKind accepted a gate that does not exist")
	}
}

// Only the test gate is judged on a report. A compiler that exits 0 has said
// everything it has to say, and holding it to a coverage floor it never prints
// would fail every build.
func TestOnlyTheTestGateIsJudgedOnItsReport(t *testing.T) {
	res, err := Run(t.TempDir(), Lint, Config{Commands: map[Kind]string{Lint: echoPlain("no issues")}}, io.Discard)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Reported {
		t.Errorf("result = %+v, want no report parsed for a non-test gate", res)
	}
	if !res.Passed() {
		t.Errorf("result = %+v, want a clean lint run to pass", res)
	}
}

// Run is the one place scc starts a process it did not choose, so what it does
// with the outcome matters more than the happy path: a passing suite, a failing
// one, and one that prints nothing readable are three different answers and none
// of them is an error.
func TestRunReadsTheCommandsAnswer(t *testing.T) {
	root := t.TempDir()

	res, err := Run(root, Test, Config{Commands: map[Kind]string{Test: echo(`{"total": 3, "coverage": 90}`)}}, io.Discard)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.Reported || res.Total != 3 || res.Coverage != 90 {
		t.Fatalf("result = %+v, want the reported numbers", res)
	}
	if res.ExitCode != 0 || !res.Passed() {
		t.Errorf("result = %+v, want a pass", res)
	}

	// A failing suite is a Result and not an error: it answered the question.
	res, err = Run(root, Test, Config{Commands: map[Kind]string{Test: fail()}}, io.Discard)
	if err != nil {
		t.Fatalf("Run on a failing command: %v", err)
	}
	if res.ExitCode == 0 {
		t.Errorf("exit = %d, want the command's own non-zero code", res.ExitCode)
	}
	if res.Passed() {
		t.Error("a failing suite passed the gate")
	}

	// So is one that ran fine and printed nothing scc can read.
	res, err = Run(root, Test, Config{Commands: map[Kind]string{Test: echo("PASS")}}, io.Discard)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Reported {
		t.Errorf("result = %+v, want no report found", res)
	}
	if res.Passed() {
		t.Error("a run with no report passed the gate")
	}
}

// The suite's output never reaches scc's stdout — it goes where the caller says,
// which is stderr everywhere in the product, because stdout carries the document.
func TestRunWritesTheOutputWhereItIsTold(t *testing.T) {
	var out strings.Builder
	if _, err := Run(t.TempDir(), Test, Config{Commands: map[Kind]string{Test: echo("hello from the suite")}}, &out); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "hello from the suite") {
		t.Errorf("out = %q, want the command's own output", out.String())
	}
}

// A command that cannot even be recorded is an error rather than a silent pass:
// exiting 0 for a gate that never ran is the one outcome this must not produce.
func TestRunRefusesAnEmptyCommand(t *testing.T) {
	if _, err := Run(t.TempDir(), Test, Config{}, io.Discard); err == nil {
		t.Error("Run with no command returned no error")
	}
}

// Passed is the whole gate in one predicate, and every clause of it earns its
// place — most of all the one about zero tests, which is the shape of a suite that
// was deleted rather than written.
func TestPassedNeedsEveryClause(t *testing.T) {
	ok := Result{Kind: Test, Reported: true, Report: Report{Total: 10, Coverage: 90}, Floor: 86}
	if !ok.Passed() {
		t.Fatal("a clean run did not pass")
	}
	for name, r := range map[string]Result{
		"failing suite": {Kind: Test, ExitCode: 1, Reported: true, Report: Report{Total: 10, Coverage: 90}, Floor: 86},
		"timed out":     {Kind: Test, TimedOut: true, Reported: true, Report: Report{Total: 10, Coverage: 90}, Floor: 86},
		"no report":     {Kind: Test, Report: Report{Total: 10, Coverage: 90}, Floor: 86},
		"no tests":      {Kind: Test, Reported: true, Report: Report{Total: 0, Coverage: 100}, Floor: 86},
		"under floor":   {Kind: Test, Reported: true, Report: Report{Total: 10, Coverage: 85.9}, Floor: 86},
	} {
		if r.Passed() {
			t.Errorf("%s passed the gate: %+v", name, r)
		}
	}
	// Exactly at the floor is a pass. 86 percent has to mean "at least 86", or the
	// number in the documentation is not the number in the code.
	at := Result{Kind: Test, Reported: true, Report: Report{Total: 10, Coverage: 86}, Floor: 86}
	if !at.Passed() {
		t.Error("coverage exactly at the floor did not pass")
	}
}

func TestPercent(t *testing.T) {
	for in, want := range map[float64]string{86: "86%", 88.4: "88.4%", 100: "100%"} {
		if got := Percent(in); got != want {
			t.Errorf("Percent(%v) = %q, want %q", in, got, want)
		}
	}
}

// echo is a command that prints one line on both shells scc runs. Quoting differs
// enough between sh and cmd that a shared literal would test the shell rather than
// the code — and the unescaped quotes on the Windows side are the point: cmd
// prints them as typed, and a run that came back with backslashes in it would be
// os/exec quoting the line for a runtime cmd.exe does not use.
func echo(s string) string {
	if runtime.GOOS == "windows" {
		return "echo " + s
	}
	return "printf '%s\\n' " + "'" + s + "'"
}

// fail is a command that exits non-zero on either shell.
func fail() string {
	if runtime.GOOS == "windows" {
		return "exit /b 3"
	}
	return "exit 3"
}

// A recorded command reaches the shell as the user typed it, quotes included.
//
// This is a Windows test that runs everywhere. os/exec quotes each argument for
// the C runtime's rules and cmd.exe does not use those rules, so `cmd /c` with an
// argument vector turns `-run "TestX"` into `-run \"TestX\"` — a command that
// runs the wrong thing and reports success. The fix is a raw command line on that
// platform, and this is what holds it in place.
func TestRunPassesQuotesThroughUnmangled(t *testing.T) {
	res, err := Run(t.TempDir(), Test, Config{Commands: map[Kind]string{Test: echo(`{"total": 2, "coverage": 99}`)}}, io.Discard)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Contains(res.Tail, `\"`) {
		t.Errorf("the shell saw escaped quotes: %q", res.Tail)
	}
	if !res.Reported || res.Total != 2 {
		t.Errorf("result = %+v, want the report the command printed", res)
	}
}

// echoPlain prints one line and exits 0 on either shell.
func echoPlain(s string) string { return echo(s) }

// A command that writes to both streams has its report read anyway.
//
// os/exec copies stdout and stderr on separate goroutines, so the capture buffer
// is written concurrently. Measured before it was locked: a test command that
// printed its report to stdout and one line to stderr came back with the report
// gone, and the gate reported `tests.no-report` on a command that had printed
// one. This is the regression test, and it is also what the race detector needs
// in order to have two writers to complain about.
func TestRunCapturesBothStreams(t *testing.T) {
	both := twoStreams(`{"total": 120, "coverage": 91.2}`, "running tests...")
	res, err := Run(t.TempDir(), Test, Config{Commands: map[Kind]string{Test: both}}, io.Discard)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.Reported || res.Total != 120 || res.Coverage != 91.2 {
		t.Fatalf("result = %+v, want the report parsed out of the interleaved output", res)
	}
	if !strings.Contains(res.Tail, "running tests") {
		t.Errorf("tail = %q, want the stderr line kept too", res.Tail)
	}
}

// twoStreams prints one line to stdout and one to stderr, on either shell.
func twoStreams(out, err string) string {
	if runtime.GOOS == "windows" {
		return "echo " + err + " 1>&2 & echo " + out
	}
	return "printf '%s\n' '" + err + "' >&2; printf '%s\n' '" + out + "'"
}

// scaffolded makes root look like a workspace for these harnesses: the manifest is
// the marker, and it is also where the commands live.
func scaffolded(t *testing.T, all ...paths.Harness) string {
	t.Helper()
	root := t.TempDir()
	for _, h := range all {
		if err := os.MkdirAll(h.Config(root), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := manifest.Save(root, h, manifest.New(assets.Version, h)); err != nil {
			t.Fatalf("manifest.Save: %v", err)
		}
	}
	return root
}

// A workspace that records nothing is not an error: absence means this project has
// not wired its commands up, which the caller reports or ignores depending on
// whether the gate was asked for.
func TestLoadAndSaveRoundTripThroughEveryManifest(t *testing.T) {
	root := scaffolded(t, paths.Claude, paths.Codex)

	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Any() {
		t.Errorf("a freshly scaffolded workspace reported commands: %+v", cfg)
	}

	want := Config{
		Commands: map[Kind]string{
			Build:  "go build ./...",
			Test:   "go run -C tools ./testreport",
			Lint:   "golangci-lint run",
			Format: Skipped,
		},
		MinCoverage: 91,
	}
	wrote, err := Save(root, want)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	// Every harness rather than one: the next reader takes the first that has
	// any, so writing a single manifest would make the answer depend on which
	// tool the reader was standing in.
	if len(wrote) != 2 {
		t.Errorf("Save wrote %v, want both manifests", wrote)
	}

	got, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, k := range Kinds() {
		if got.Command(k) != want.Command(k) {
			t.Errorf("%s = %q, want %q", k, got.Command(k), want.Command(k))
		}
	}
	if got.MinCoverage != want.MinCoverage {
		t.Errorf("MinCoverage = %v, want %v", got.MinCoverage, want.MinCoverage)
	}
	// `skipped` is a word rather than an absent key: a language with no formatter
	// is a real answer, and a gate left unrecorded is a decision nobody has made.
	if !got.Declined(Format) {
		t.Error("a gate recorded as skipped did not read back as declined")
	}
	if got.Declined(Build) {
		t.Error("a gate with a real command read back as declined")
	}

	// A finding about the configuration points at the manifest it was read from.
	if p := ManifestPath(root); p != paths.Claude.Manifest(root) {
		t.Errorf("ManifestPath = %q, want the first harness's manifest", p)
	}
	// And outside a workspace it still names a file, so a finding has somewhere to
	// point rather than an empty path.
	if p := ManifestPath(t.TempDir()); p == "" {
		t.Error("ManifestPath outside a workspace named nothing")
	}
	if _, err := Save(t.TempDir(), want); err == nil {
		t.Error("Save outside a workspace returned no error")
	}
}
