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
func isolatedPath(t *testing.T, bins ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, bin := range bins {
		script, name := "#!/bin/sh\n"+stubHelp(bin, "sh")+"echo '"+bin+" 0.0.0-stub'\n", bin
		if runtime.GOOS == "windows" {
			script, name = "@echo off\r\n"+stubHelp(bin, "bat")+"echo "+bin+" 0.0.0-stub\r\n", bin+".bat"
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatalf("stub %s: %v", bin, err)
		}
	}
	t.Setenv("PATH", dir)
	return dir
}

// stubHelp gives the headroom stub a --help that advertises the opt-out flags,
// because scc reads them off the binary rather than carrying them: a stub that
// answered nothing would make every MCP assertion below pass for the wrong
// reason.
//
// Shell builtins only — no `find`, no `grep`, no `printf`. isolatedPath replaces
// PATH wholesale, so a stub that shelled out to anything would find nothing there
// and silently report a Headroom with no flags at all. The text is abridged, and
// deliberately free of the `|` a real help puts in `--code-memory [serena|none]`,
// which in a .bat is a redirect rather than a character.
func stubHelp(bin, shell string) string {
	if bin != "headroom" {
		return ""
	}
	opts := []string{
		"Options:",
		"  --no-mcp  Skip headroom MCP server registration",
		"  --code-memory  Code-memory MCP to register: serena or none",
		"  --no-context-tool, --no-rtk  Skip CLI context-tool setup",
	}
	if shell == "bat" {
		s := ":sccargs\r\nif \"%~1\"==\"\" goto sccrun\r\nif \"%~1\"==\"--help\" goto scchelp\r\nshift\r\ngoto sccargs\r\n:scchelp\r\n"
		for _, l := range opts {
			s += "echo " + l + "\r\n"
		}
		return s + "exit /b 0\r\n:sccrun\r\n"
	}
	s := "for a in \"$@\"; do\n  if [ \"$a\" = \"--help\" ]; then\n"
	for _, l := range opts {
		s += "    echo '" + l + "'\n"
	}
	return s + "    exit 0\n  fi\ndone\n"
}

// recordingStub replaces a stub with one that appends its arguments to a log, so
// a test can assert on what scc actually asked a third-party binary to do without
// that binary being installed.
func recordingStub(t *testing.T, dir, bin string) string {
	t.Helper()
	log := filepath.Join(dir, bin+".log")
	script, name := "#!/bin/sh\necho \"$@\" >> '"+log+"'\nexit 0\n", bin
	if runtime.GOOS == "windows" {
		script, name = "@echo off\r\necho %* >> \""+log+"\"\r\nexit /b 0\r\n", bin+".bat"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatalf("stub %s: %v", bin, err)
	}
	return log
}

// recorded is everything the stub was asked to do, or "" when it was never run.
func recorded(t *testing.T, log string) string {
	t.Helper()
	raw, err := os.ReadFile(log)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatalf("read %s: %v", log, err)
	}
	return string(raw)
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
	if got, want := strings.Join(cmd.Args, " "), "wrap claude --code-memory none --no-context-tool"; got != want {
		t.Errorf("args = %q, want %q", got, want)
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

// Headroom's wrap registers MCP servers into the user's agent config, and those
// registrations outlive the session that made them. scc asks for its own retrieve
// tool and nothing else, so a launch does not quietly install a code-memory
// server the workspace has not asked for — this workspace's code intelligence is
// CodeGraph's job.
func TestLaunchTurnsHeadroomsCodeMemoryOffByDefault(t *testing.T) {
	root := initWorkspace(t)
	isolatedPath(t, "headroom", "claude")

	cmd := launchJSON(t, "launch", "--root", root, "--json")
	if cmd.Headroom == nil {
		t.Fatal("no headroom report")
	}
	if cmd.Headroom.MCP != "retrieve" {
		t.Errorf("mcp = %q, want retrieve", cmd.Headroom.MCP)
	}
	if got := strings.Join(cmd.Headroom.Options, " "); !strings.Contains(got, "--code-memory none") {
		t.Errorf("options = %q, want them to decline the code-memory server", got)
	}
}

// Headroom's context-tool setup appends RTK guidance to the same entry file
// `scc rtk` splices, behind its own marker pair — so a workspace wired by both
// carries the instructions twice. Headroom already gates that behind
// HEADROOM_RTK; scc passes the flag anyway, so an environment that exports it for
// other reasons does not quietly turn every launch into a second copy.
func TestLaunchDeclinesHeadroomsContextToolByDefault(t *testing.T) {
	root := initWorkspace(t)
	isolatedPath(t, "headroom", "claude")

	cmd := launchJSON(t, "launch", "--root", root, "--json")
	if cmd.Headroom.ContextTool {
		t.Error("context_tool = true, want Headroom's own setup declined")
	}
	if got := strings.Join(cmd.Args, " "); !strings.Contains(got, "--no-context-tool") {
		t.Errorf("args = %q, want them to decline the context tool", got)
	}

	// And the escape hatch hands it back, without scc having an opinion left.
	cmd = launchJSON(t, "launch", "--root", root, "--headroom-context-tool", "--headroom-mcp", "all", "--json")
	if !cmd.Headroom.ContextTool {
		t.Error("context_tool = false under --headroom-context-tool")
	}
	if got, want := strings.Join(cmd.Args, " "), "wrap claude"; got != want {
		t.Errorf("args = %q, want %q", got, want)
	}
}

// The two ends of the same flag: `all` is scc keeping its hands off Headroom's
// defaults, and `none` also drops the retrieve tool the proxy needs to make its
// compression markers actionable.
func TestLaunchHeadroomMCPModes(t *testing.T) {
	root := initWorkspace(t)
	isolatedPath(t, "headroom", "claude")

	cmd := launchJSON(t, "launch", "--root", root, "--headroom-mcp", "all", "--json")
	if got, want := strings.Join(cmd.Args, " "), "wrap claude --no-context-tool"; got != want {
		t.Errorf("all: args = %q, want %q", got, want)
	}

	cmd = launchJSON(t, "launch", "--root", root, "--headroom-mcp", "none", "--json")
	if got, want := strings.Join(cmd.Args, " "), "wrap claude --code-memory none --no-mcp --no-context-tool"; got != want {
		t.Errorf("none: args = %q, want %q", got, want)
	}

	if _, stderr, code := run(t, "launch", "--root", root, "--headroom-mcp", "serena", "--json"); code != ExitError {
		t.Errorf("an undefined mode exited %d, want %d (stderr: %s)", code, ExitError, stderr)
	}
}

// Everything after `--` belongs to the agent, and scc's own options go in front of
// it — so a user who names the same flag after `--` is the one who wins, since
// wrap takes the last occurrence.
func TestLaunchPassesArgumentsThroughToTheAgent(t *testing.T) {
	root := initWorkspace(t)
	isolatedPath(t, "headroom", "claude")

	cmd := launchJSON(t, "launch", "claude", "--json", "--root", root, "--", "--resume", "--model", "opus")
	if got, want := strings.Join(cmd.Args, " "), "wrap claude --code-memory none --no-context-tool --resume --model opus"; got != want {
		t.Errorf("args = %q, want %q", got, want)
	}

	// And with no Headroom in front, the same arguments reach the binary directly.
	cmd = launchJSON(t, "launch", "claude", "--json", "--no-headroom", "--root", root, "--", "--resume")
	if got, want := strings.Join(cmd.Args, " "), "--resume"; got != want {
		t.Errorf("bare args = %q, want %q", got, want)
	}
}

// Launch is the one moment where indexing is free: the session is about to begin,
// nobody is waiting on a prompt, and the alternative is an agent whose first graph
// query answers out of last week's index. A workspace with no graph gets the full
// build; one that already has a graph gets the incremental refresh.
func TestLaunchBuildsTheGraphAndThenRefreshesIt(t *testing.T) {
	root := initWorkspace(t)
	dir := isolatedPath(t, "claude")
	log := recordingStub(t, dir, "codegraph")
	withLaunchExec(t, 0)

	if _, stderr, code := run(t, "launch", "--root", root, "--no-headroom"); code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if got := recorded(t, log); !strings.Contains(got, "init") {
		t.Errorf("codegraph was asked to do %q, want an init", got)
	}

	// Now the workspace has one, so the next launch refreshes rather than rebuilds.
	if err := os.Mkdir(filepath.Join(root, ".codegraph"), 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.Remove(log); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, stderr, code := run(t, "launch", "--root", root, "--no-headroom"); code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	got := recorded(t, log)
	if !strings.Contains(got, "sync") {
		t.Errorf("codegraph was asked to do %q, want a sync", got)
	}
	if strings.Contains(got, "init") {
		t.Errorf("codegraph re-initialized a workspace that already had a graph: %q", got)
	}
}

// A graph is an enhancement — the agent can still read files — so every way of not
// getting one ends in the agent starting anyway, with a line saying what happened.
// A launch that refused to run because an index was stale would put scc's
// tidiness above the thing the user asked for.
func TestLaunchStartsWithoutAGraphWhenCodeGraphIsMissing(t *testing.T) {
	root := initWorkspace(t)
	isolatedPath(t, "claude")
	withoutTerminal(t)
	got := withLaunchExec(t, 0)

	_, stderr, code := run(t, "launch", "--root", root, "--no-headroom")
	if code != ExitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if got.Bin != paths.Claude.Bin {
		t.Errorf("launched %q, want the agent to have started anyway", got.Bin)
	}
	if !strings.Contains(stderr, "symbol graph") {
		t.Errorf("stderr does not say the graph was skipped: %q", stderr)
	}
}

// --no-graph is the explicit "just start the agent": no lookup, no index, no
// report.
func TestLaunchNoGraphSkipsItEntirely(t *testing.T) {
	root := initWorkspace(t)
	dir := isolatedPath(t, "claude")
	log := recordingStub(t, dir, "codegraph")

	cmd := launchJSON(t, "launch", "--root", root, "--no-headroom", "--no-graph", "--json")
	if cmd.Graph != nil {
		t.Errorf("graph = %+v, want it absent from the report", cmd.Graph)
	}
	if got := recorded(t, log); got != "" {
		t.Errorf("--no-graph still ran codegraph: %q", got)
	}
}

// Indexing a repository is work, and a plan-only run has not been asked to do any.
// --json additionally cannot afford it: the indexer writes to stdout, which
// carries the document and nothing else.
func TestLaunchNeverIndexesOnAPlanOnlyRun(t *testing.T) {
	root := initWorkspace(t)
	dir := isolatedPath(t, "claude")
	log := recordingStub(t, dir, "codegraph")

	for _, flag := range []string{"--json", "--dry-run"} {
		if _, stderr, code := run(t, "launch", "--root", root, "--no-headroom", flag); code != ExitOK {
			t.Fatalf("%s: exit = %d (stderr: %s)", flag, code, stderr)
		}
		if got := recorded(t, log); strings.Contains(got, "init") || strings.Contains(got, "sync") {
			t.Errorf("%s indexed the workspace: %q", flag, got)
		}
	}
	cmd := launchJSON(t, "launch", "--root", root, "--no-headroom", "--json")
	if cmd.Graph == nil || cmd.Graph.Action != graphSkipped {
		t.Errorf("graph = %+v, want the index reported as skipped", cmd.Graph)
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

// RTK joins the preflight the other two integrations already went through, so a
// launch reports all three. --no-rtk is the standing "not in this workspace", and it
// has to leave the field out entirely rather than report a skip: the two are
// different answers, and only one of them means somebody decided.
func TestLaunchReportsRTKAndNoRTKOmitsIt(t *testing.T) {
	root := initWorkspace(t)
	isolatedPath(t, "claude")

	var with launchCommand
	stdout, _, code := run(t, "launch", "--root", root, "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if err := json.Unmarshal([]byte(stdout), &with); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, stdout)
	}
	if with.RTK == nil {
		t.Fatal("a launch reported no rtk field at all")
	}
	if with.RTK.Install != installSkipped {
		t.Errorf("install = %q, want %q with cargo absent", with.RTK.Install, installSkipped)
	}
	if with.RTK.Reason == "" {
		t.Error("rtk was skipped without saying why")
	}

	var without launchCommand
	stdout, _, code = run(t, "launch", "--root", root, "--json", "--no-rtk")
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if err := json.Unmarshal([]byte(stdout), &without); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, stdout)
	}
	if without.RTK != nil {
		t.Errorf("--no-rtk still reported %+v", without.RTK)
	}
}

// --no-install covers RTK too. The flag says "never build anything", and a cargo
// build that takes minutes is the most expensive thing it governs.
func TestLaunchNoInstallCoversRTK(t *testing.T) {
	root := initWorkspace(t)
	isolatedPath(t, "cargo", "claude")
	withPrompt(t, "\n")
	withLaunchExec(t, 0)

	stdout, stderr, code := run(t, "launch", "--root", root, "--no-install")
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if strings.Contains(stdout+stderr, "add its usage block") {
		t.Errorf("--no-install still offered the RTK setup: %q", stdout+stderr)
	}
}

// An install prompt defaults to yes and a bare Enter takes it, which is the opposite
// of confirm(). The balance is opposite too: a wrong yes here is a tool on the
// machine, where a wrong yes in confirm() is somebody's file.
func TestConfirmInstallDefaultsToYes(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"\n", true},
		{"y\n", true},
		{"  \n", true},
		{"n\n", false},
		{"N\n", false},
		{"no\n", false},
		{"não\n", false},
		{"", false}, // end of input is not an answer
	} {
		if got := confirmInstall(strings.NewReader(tc.in), "Install?"); got != tc.want {
			t.Errorf("confirmInstall(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
