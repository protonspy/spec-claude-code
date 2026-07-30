package cli

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

// capture swaps os.Stdout/os.Stderr for pipes around f and returns what was
// written to each. It is the only safe way to assert on CLI output, because
// package render writes to the real files rather than to an injected writer.
func capture(t *testing.T, f func() int) (stdout, stderr string, code int) {
	t.Helper()
	origOut, origErr := os.Stdout, os.Stderr
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout, os.Stderr = outW, errW
	defer func() { os.Stdout, os.Stderr = origOut, origErr }()

	// Drain concurrently: a command that writes more than the pipe buffer would
	// otherwise block forever on the write instead of failing the test.
	outC, errC := make(chan string, 1), make(chan string, 1)
	go func() { b, _ := io.ReadAll(outR); outC <- string(b) }()
	go func() { b, _ := io.ReadAll(errR); errC <- string(b) }()

	code = f()

	_ = outW.Close()
	_ = errW.Close()
	return <-outC, <-errC, code
}

// run wraps capture around Run for the common case.
func run(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	return capture(t, func() int { return Run(args) })
}

func TestVersionPrintsStampedVersion(t *testing.T) {
	stdout, _, code := run(t, "version")
	if code != ExitOK {
		t.Errorf("exit = %d, want %d", code, ExitOK)
	}
	if strings.TrimSpace(stdout) != version {
		t.Errorf("stdout = %q, want %q", strings.TrimSpace(stdout), version)
	}
}

// --json output must be a clean document on stdout with nothing else mixed in —
// callers pipe it straight into jq.
func TestVersionJSON(t *testing.T) {
	stdout, stderr, code := run(t, "version", "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d (stderr: %s)", code, ExitOK, stderr)
	}
	var got struct {
		Version  string `json:"version"`
		Go       string `json:"go"`
		Platform string `json:"platform"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not valid JSON (%v): %q", err, stdout)
	}
	if got.Version != version {
		t.Errorf("version = %q, want %q", got.Version, version)
	}
	if got.Go == "" || got.Platform == "" {
		t.Errorf("go/platform not reported: %+v", got)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

func TestVersionAliases(t *testing.T) {
	for _, arg := range []string{"-v", "--version"} {
		if _, _, code := run(t, arg); code != ExitOK {
			t.Errorf("%s: exit = %d, want %d", arg, code, ExitOK)
		}
	}
}

// Usage goes to stderr so it never pollutes a stdout a caller is parsing.
func TestHelpGoesToStderr(t *testing.T) {
	for _, arg := range []string{"help", "-h", "--help"} {
		stdout, stderr, code := run(t, arg)
		if code != ExitOK {
			t.Errorf("%s: exit = %d, want %d", arg, code, ExitOK)
		}
		if stdout != "" {
			t.Errorf("%s: stdout = %q, want empty", arg, stdout)
		}
		if !strings.Contains(stderr, "Usage:") {
			t.Errorf("%s: stderr missing usage: %q", arg, stderr)
		}
	}
}

func TestNoArgsIsAUsageError(t *testing.T) {
	_, stderr, code := run(t)
	if code != ExitError {
		t.Errorf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "Usage:") {
		t.Errorf("stderr missing usage: %q", stderr)
	}
}

func TestUnknownCommand(t *testing.T) {
	_, stderr, code := run(t, "nope")
	if code != ExitError {
		t.Errorf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, `unknown command "nope"`) {
		t.Errorf("stderr = %q, want it to name the unknown command", stderr)
	}
}

// An unparseable flag is a usage error, not a crash and not a silent success.
func TestUnknownFlagIsAUsageError(t *testing.T) {
	if _, _, code := run(t, "version", "--nope"); code != ExitError {
		t.Errorf("exit = %d, want %d", code, ExitError)
	}
}

// SCC_PROG lets the npm launcher make help text echo the spelling the user typed
// (`npx @protonspy/scc`) instead of a bare binary name they may not have.
func TestProgHonorsEnvOverride(t *testing.T) {
	t.Setenv("SCC_PROG", "npx @protonspy/scc")
	if got := prog(); got != "npx @protonspy/scc" {
		t.Errorf("prog() = %q, want the override", got)
	}
	_, stderr, _ := run(t, "help")
	if !strings.Contains(stderr, "npx @protonspy/scc") {
		t.Errorf("usage did not use SCC_PROG: %q", stderr)
	}
}

func TestProgFallsBackToBinaryName(t *testing.T) {
	t.Setenv("SCC_PROG", "   ")
	if got := prog(); got != "scc" {
		t.Errorf("prog() = %q, want %q", got, "scc")
	}
}

// The three exit codes are a published contract; collapsing 2 into 1 would break
// every CI job and agent that branches on "ran but found problems".
func TestExitCodesAreDistinct(t *testing.T) {
	if ExitOK != 0 || ExitError != 1 || ExitFindings != 2 {
		t.Errorf("exit codes drifted: ok=%d error=%d findings=%d", ExitOK, ExitError, ExitFindings)
	}
}
