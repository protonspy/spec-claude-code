package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/paths"
)

// No test here may reach a real install: `uv tool install` would spend minutes of
// CI resolving a Python distribution, and the result would depend on whether the
// machine happened to have Headroom already. Every test therefore replaces PATH
// wholesale with a directory it controls — which also makes "Headroom is absent"
// a fact of the test rather than a fact about the developer's laptop.
func isolatedPath(t *testing.T, bins ...string) {
	t.Helper()
	dir := t.TempDir()
	for _, bin := range bins {
		script, name := "#!/bin/sh\necho '"+bin+" 0.0.0-stub'\n", bin
		if runtime.GOOS == "windows" {
			script, name = "@echo "+bin+" 0.0.0-stub\r\n", bin+".bat"
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatalf("stub %s: %v", bin, err)
		}
	}
	t.Setenv("PATH", dir)
}

// withLaunchExec replaces the process launcher with a recorder, so the whole
// command surface is drivable without ever starting an agent.
func withLaunchExec(t *testing.T, code int) *launchCommand {
	t.Helper()
	var got launchCommand
	orig := launchExec
	launchExec = func(cmd launchCommand) (int, error) {
		got = cmd
		return code, nil
	}
	t.Cleanup(func() { launchExec = orig })
	return &got
}

func launchJSON(t *testing.T, args ...string) launchCommand {
	t.Helper()
	stdout, stderr, code := run(t, args...)
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d (stderr: %s)", code, ExitOK, stderr)
	}
	var cmd launchCommand
	if err := json.Unmarshal([]byte(stdout), &cmd); err != nil {
		t.Fatalf("stdout is not valid JSON (%v): %q", err, stdout)
	}
	return cmd
}

// Headroom on PATH is the whole point of the command: the agent starts behind the
// compression proxy without anybody having to remember the wrapper's spelling.
func TestLaunchWrapsWithHeadroomWhenItIsThere(t *testing.T) {
	root := initWorkspace(t)
	isolatedPath(t, "headroom", "claude")

	cmd := launchJSON(t, "launch", "--root", root, "--json")
	if cmd.Bin != "headroom" {
		t.Errorf("bin = %q, want headroom", cmd.Bin)
	}
	if strings.Join(cmd.Args, " ") != "wrap claude" {
		t.Errorf("args = %v, want [wrap claude]", cmd.Args)
	}
	if cmd.Harness != paths.Claude.ID {
		t.Errorf("harness = %q, want claude", cmd.Harness)
	}
	if cmd.Headroom == nil || !cmd.Headroom.Wrapping {
		t.Fatalf("headroom = %+v, want it wrapping", cmd.Headroom)
	}
	if cmd.Headroom.Install != installPresent {
		t.Errorf("install = %q, want %q", cmd.Headroom.Install, installPresent)
	}
}

// Headroom is an enhancement, so every way of not getting it degrades to starting
// the agent bare. A launch that refused to run because a compression proxy was
// missing would put scc's preference above what the user asked for.
func TestLaunchStartsBareWhenHeadroomIsMissing(t *testing.T) {
	root := initWorkspace(t)
	isolatedPath(t, "claude")

	cmd := launchJSON(t, "launch", "--root", root, "--json")
	if cmd.Bin != paths.Claude.Bin {
		t.Errorf("bin = %q, want %q", cmd.Bin, paths.Claude.Bin)
	}
	if len(cmd.Args) != 0 {
		t.Errorf("args = %v, want empty", cmd.Args)
	}
	if cmd.Headroom == nil || cmd.Headroom.Wrapping {
		t.Fatalf("headroom = %+v, want it reported as not wrapping", cmd.Headroom)
	}
	// Why it is not wrapping has to survive into the report: "started without
	// compression" is exactly the outcome somebody would otherwise not notice.
	if cmd.Headroom.Reason == "" {
		t.Error("the report does not say why the launch is unwrapped")
	}
}

// --no-headroom is the explicit "just start the agent", and it must not even ask
// the question — no PATH lookup, no report, no prompt.
func TestLaunchNoHeadroomSkipsItEntirely(t *testing.T) {
	root := initWorkspace(t)
	isolatedPath(t, "headroom", "claude")

	cmd := launchJSON(t, "launch", "--root", root, "--no-headroom", "--json")
	if cmd.Bin != paths.Claude.Bin {
		t.Errorf("bin = %q, want %q", cmd.Bin, paths.Claude.Bin)
	}
	if cmd.Headroom != nil {
		t.Errorf("headroom = %+v, want it absent from the report", cmd.Headroom)
	}
}

// Everything after `--` belongs to the agent. scc must not parse it, reject it,
// or reorder it — it goes behind the wrap slug exactly as typed.
func TestLaunchPassesArgumentsThroughToTheAgent(t *testing.T) {
	root := initWorkspace(t)
	isolatedPath(t, "headroom", "claude")

	cmd := launchJSON(t, "launch", "claude", "--json", "--root", root, "--", "--resume", "--model", "opus")
	if got, want := strings.Join(cmd.Args, " "), "wrap claude --resume --model opus"; got != want {
		t.Errorf("args = %q, want %q", got, want)
	}

	// And with no Headroom in front, the same arguments reach the binary directly.
	cmd = launchJSON(t, "launch", "claude", "--json", "--no-headroom", "--root", root, "--", "--resume")
	if got, want := strings.Join(cmd.Args, " "), "--resume"; got != want {
		t.Errorf("bare args = %q, want %q", got, want)
	}
}

// The agent's exit code is the command's exit code. A launcher that flattened the
// status of what it launched into scc's own 0/1/2 would be unusable in a script.
func TestLaunchPassesTheAgentsExitCodeThrough(t *testing.T) {
	root := initWorkspace(t)
	isolatedPath(t, "claude")
	got := withLaunchExec(t, 42)

	if _, stderr, code := run(t, "launch", "--root", root, "--no-headroom"); code != 42 {
		t.Errorf("exit = %d, want the agent's 42 (stderr: %s)", code, stderr)
	}
	if got.Bin != paths.Claude.Bin {
		t.Errorf("launched %q, want %q", got.Bin, paths.Claude.Bin)
	}
	// The workspace root, not the shell's directory: `scc launch` from a
	// subdirectory has to start the agent where the methodology lives.
	if !sameDir(t, got.Dir, root) {
		t.Errorf("started in %q, want the workspace root %q", got.Dir, root)
	}
}

// Run from a subdirectory with no --root, the agent still starts at the workspace
// root: the same upward walk every other command uses. Without it `scc launch`
// from specs/ would come up scoped to half the repo, which is the kind of thing
// nobody notices until the agent cannot find the rules.
func TestLaunchStartsAtTheWorkspaceRoot(t *testing.T) {
	root := initWorkspace(t)
	sub := filepath.Join(root, "specs", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	isolatedPath(t, "claude")
	got := withLaunchExec(t, 0)
	t.Chdir(sub)

	if _, stderr, code := run(t, "launch", "--no-headroom"); code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if !sameDir(t, got.Dir, root) {
		t.Errorf("started in %q, want the workspace root %q", got.Dir, root)
	}
}

// --dry-run and --json report the plan and start nothing. For --json that is the
// only coherent answer: the agent inherits this terminal and writes to the same
// stdout, so a live session and a clean JSON document cannot both exist.
func TestLaunchDryRunAndJSONStartNothing(t *testing.T) {
	root := initWorkspace(t)
	isolatedPath(t, "headroom", "claude")
	started := false
	orig := launchExec
	launchExec = func(launchCommand) (int, error) { started = true; return 0, nil }
	t.Cleanup(func() { launchExec = orig })

	stdout, stderr, code := run(t, "launch", "--root", root, "--dry-run")
	if code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "headroom wrap claude") {
		t.Errorf("--dry-run does not name the command: %q", stdout)
	}
	_ = launchJSON(t, "launch", "--root", root, "--json")
	if started {
		t.Error("--dry-run or --json started the agent")
	}
}

// Installing a Python distribution is a decision, and a plan-only run has not
// been given it. Even with an installer on PATH and somebody at the terminal
// answering yes, --json and --dry-run must report and stop.
func TestLaunchNeverInstallsOnAPlanOnlyRun(t *testing.T) {
	root := initWorkspace(t)
	isolatedPath(t, "uv", "claude")
	withPrompt(t, "y\ny\n")

	for _, flag := range []string{"--json", "--dry-run"} {
		stdout, _, code := run(t, "launch", "--root", root, flag)
		if code != ExitOK {
			t.Fatalf("%s: exit = %d", flag, code)
		}
		if strings.Contains(stdout, "Install it now") {
			t.Errorf("%s asked to install: %q", flag, stdout)
		}
	}
	cmd := launchJSON(t, "launch", "--root", root, "--json")
	if cmd.Headroom == nil || cmd.Headroom.Install != installSkipped {
		t.Errorf("headroom = %+v, want the install reported as skipped", cmd.Headroom)
	}
}

// With nobody at the terminal — a CI job, or an agent driving scc — the install
// prompt cannot be asked, so it is not asked. The launch says why and goes ahead
// unwrapped rather than blocking on a read no one will answer.
func TestLaunchDoesNotAskUnattended(t *testing.T) {
	root := initWorkspace(t)
	isolatedPath(t, "uv", "claude")
	withoutTerminal(t)
	got := withLaunchExec(t, 0)

	stdout, stderr, code := run(t, "launch", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if strings.Contains(stdout, "Install it now") {
		t.Errorf("scc prompted with nobody there: %q", stdout)
	}
	if !strings.Contains(stderr, "without headroom") {
		t.Errorf("stderr does not say the launch is unwrapped: %q", stderr)
	}
	if got.Bin != paths.Claude.Bin {
		t.Errorf("launched %q, want the bare agent", got.Bin)
	}
}

// Asked and declined: the answer is respected for this run and the agent still
// starts. "No" means no compression, not no agent.
func TestLaunchFallsBackWhenTheInstallIsDeclined(t *testing.T) {
	root := initWorkspace(t)
	isolatedPath(t, "uv", "claude")
	withPrompt(t, "n\n")
	got := withLaunchExec(t, 0)

	stdout, stderr, code := run(t, "launch", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "Install it now") {
		t.Errorf("scc did not ask before installing: %q", stdout)
	}
	if got.Bin != paths.Claude.Bin {
		t.Errorf("launched %q, want the bare agent after declining", got.Bin)
	}
	if !strings.Contains(stderr, "declined") {
		t.Errorf("stderr does not record the declined install: %q", stderr)
	}
}

// --no-install is the standing "never build anything": no prompt, no install, use
// Headroom only if it is already there.
func TestLaunchNoInstallNeverAsks(t *testing.T) {
	root := initWorkspace(t)
	isolatedPath(t, "uv", "claude")
	withPrompt(t, "y\n")
	got := withLaunchExec(t, 0)

	stdout, _, code := run(t, "launch", "--root", root, "--no-install")
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if strings.Contains(stdout, "Install it now") {
		t.Errorf("--no-install still asked: %q", stdout)
	}
	if got.Bin != paths.Claude.Bin {
		t.Errorf("launched %q, want the bare agent", got.Bin)
	}
}

// Starting Codex in a Claude-only repo would hand the user an agent with none of
// the methodology loaded, while looking like it worked.
func TestLaunchRefusesAHarnessThatIsNotScaffolded(t *testing.T) {
	root := initWorkspace(t)
	isolatedPath(t, "codex")

	_, stderr, code := run(t, "launch", "codex", "--root", root)
	if code != ExitError {
		t.Errorf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "codex") || !strings.Contains(stderr, "init") {
		t.Errorf("stderr does not say how to fix it: %q", stderr)
	}
}

func TestLaunchRejectsAnUnknownHarness(t *testing.T) {
	root := initWorkspace(t)
	_, stderr, code := run(t, "launch", "nope", "--root", root)
	if code != ExitError {
		t.Errorf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "nope") {
		t.Errorf("stderr does not name the unknown harness: %q", stderr)
	}
}

// A repo worked on from two tools has no single answer, so scc asks — and when
// nobody is there to ask, it names the choices instead of guessing.
func TestLaunchResolvesAmbiguityByAskingOrNamingTheChoices(t *testing.T) {
	root := t.TempDir()
	for _, h := range []string{"--codex", "--opencode"} {
		if _, stderr, code := run(t, "init", h, "--root", root); code != ExitOK {
			t.Fatalf("init %s: exit = %d (%s)", h, code, stderr)
		}
	}
	isolatedPath(t, "codex", "opencode")

	withoutTerminal(t)
	_, stderr, code := run(t, "launch", "--root", root, "--no-headroom")
	if code != ExitError {
		t.Errorf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "codex") || !strings.Contains(stderr, "opencode") {
		t.Errorf("stderr does not name both harnesses: %q", stderr)
	}

	withPrompt(t, "2\n")
	got := withLaunchExec(t, 0)
	stdout, stderr, code := run(t, "launch", "--root", root, "--no-headroom")
	if code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "Which harness") {
		t.Errorf("launch did not ask:\n%s", stdout)
	}
	if got.Bin != paths.OpenCode.Bin {
		t.Errorf("launched %q, want %q", got.Bin, paths.OpenCode.Bin)
	}
}

// The picker offers only what is scaffolded here. Offering a harness that is not
// set up would be offering a broken answer.
func TestLaunchPickerOffersOnlyTheScaffoldedHarnesses(t *testing.T) {
	root := t.TempDir()
	for _, h := range []string{"--codex", "--opencode"} {
		if _, stderr, code := run(t, "init", h, "--root", root); code != ExitOK {
			t.Fatalf("init %s: exit = %d (%s)", h, code, stderr)
		}
	}
	isolatedPath(t, "codex", "opencode")
	withPrompt(t, "\n")
	withLaunchExec(t, 0)

	stdout, _, _ := run(t, "launch", "--root", root, "--no-headroom")
	if strings.Contains(stdout, paths.Claude.Label) {
		t.Errorf("the picker offered a harness that is not scaffolded here:\n%s", stdout)
	}
}

// Outside a workspace there is nothing to start the agent in, and the walk would
// otherwise fall back to whatever directory the user happened to be in.
func TestLaunchRequiresAWorkspace(t *testing.T) {
	_, stderr, code := run(t, "launch", "--root", t.TempDir())
	if code != ExitError {
		t.Errorf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "not an scc workspace") {
		t.Errorf("stderr = %q, want it to say the directory is not a workspace", stderr)
	}
}

// A harness scc knows about but that is not installed gets a message about the
// binary, not a stack trace from exec.
func TestLaunchReportsAMissingAgentBinary(t *testing.T) {
	root := initWorkspace(t)
	isolatedPath(t)

	_, stderr, code := run(t, "launch", "--root", root, "--no-headroom")
	if code != ExitError {
		t.Errorf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, paths.Claude.Bin) || !strings.Contains(stderr, "not on PATH") {
		t.Errorf("stderr = %q, want it to name the missing binary", stderr)
	}
}

// sameDir compares resolved paths rather than strings: t.TempDir can sit under a
// symlink on macOS, and Windows reports 8.3 short names.
func sameDir(t *testing.T, a, b string) bool {
	t.Helper()
	fa, err := os.Stat(a)
	if err != nil {
		t.Fatalf("Stat %s: %v", a, err)
	}
	fb, err := os.Stat(b)
	if err != nil {
		t.Fatalf("Stat %s: %v", b, err)
	}
	return os.SameFile(fa, fb)
}
