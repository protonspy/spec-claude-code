package testrun

import (
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

func TestConfigured(t *testing.T) {
	for _, c := range []struct {
		cmd  string
		want bool
	}{{"", false}, {"   ", false}, {"make test", true}} {
		if got := (Config{Command: c.cmd}).Configured(); got != c.want {
			t.Errorf("Configured(%q) = %v, want %v", c.cmd, got, c.want)
		}
	}
}

// Run is the one place scc starts a process it did not choose, so what it does
// with the outcome matters more than the happy path: a passing suite, a failing
// one, and one that prints nothing readable are three different answers and none
// of them is an error.
func TestRunReadsTheCommandsAnswer(t *testing.T) {
	root := t.TempDir()

	res, err := Run(root, Config{Command: echo(`{"total": 3, "coverage": 90}`)}, io.Discard)
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
	res, err = Run(root, Config{Command: fail()}, io.Discard)
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
	res, err = Run(root, Config{Command: echo("PASS")}, io.Discard)
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
	if _, err := Run(t.TempDir(), Config{Command: echo("hello from the suite")}, &out); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "hello from the suite") {
		t.Errorf("out = %q, want the command's own output", out.String())
	}
}

// A command that cannot even be recorded is an error rather than a silent pass:
// exiting 0 for a gate that never ran is the one outcome this must not produce.
func TestRunRefusesAnEmptyCommand(t *testing.T) {
	if _, err := Run(t.TempDir(), Config{}, io.Discard); err == nil {
		t.Error("Run with no command returned no error")
	}
}

// Passed is the whole gate in one predicate, and every clause of it earns its
// place — most of all the one about zero tests, which is the shape of a suite that
// was deleted rather than written.
func TestPassedNeedsEveryClause(t *testing.T) {
	ok := Result{Reported: true, Report: Report{Total: 10, Coverage: 90}, Floor: 86}
	if !ok.Passed() {
		t.Fatal("a clean run did not pass")
	}
	for name, r := range map[string]Result{
		"failing suite": {ExitCode: 1, Reported: true, Report: Report{Total: 10, Coverage: 90}, Floor: 86},
		"timed out":     {TimedOut: true, Reported: true, Report: Report{Total: 10, Coverage: 90}, Floor: 86},
		"no report":     {Report: Report{Total: 10, Coverage: 90}, Floor: 86},
		"no tests":      {Reported: true, Report: Report{Total: 0, Coverage: 100}, Floor: 86},
		"under floor":   {Reported: true, Report: Report{Total: 10, Coverage: 85.9}, Floor: 86},
	} {
		if r.Passed() {
			t.Errorf("%s passed the gate: %+v", name, r)
		}
	}
	// Exactly at the floor is a pass. 86 percent has to mean "at least 86", or the
	// number in the documentation is not the number in the code.
	at := Result{Reported: true, Report: Report{Total: 10, Coverage: 86}, Floor: 86}
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
	res, err := Run(t.TempDir(), Config{Command: echo(`{"total": 2, "coverage": 99}`)}, io.Discard)
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
