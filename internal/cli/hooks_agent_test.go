package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/hooks"
	"github.com/protonspy/spec-claude-code/internal/paths"
)

// readSettings is the harness's own file, parsed loosely — the point of these
// tests is what scc did and did not touch in a document it does not own.
func readSettings(t *testing.T, root string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(paths.Claude.Settings(root))
	if err != nil {
		t.Fatalf("ReadFile settings: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("settings.json is not valid JSON (%v): %s", err, b)
	}
	return out
}

func hookEvents(t *testing.T, root string) map[string]any {
	t.Helper()
	all := readSettings(t, root)
	h, ok := all["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("settings.json carries no hooks object: %+v", all)
	}
	return h
}

// init registers the in-session hooks in the harness's own settings file. They
// are the layer a git hook cannot reach: the end of a turn, where a finding is
// cheapest to act on.
func TestInitRegistersTheHarnessHooks(t *testing.T) {
	root := initWorkspace(t)
	events := hookEvents(t, root)
	for _, e := range hooks.Events() {
		if _, ok := events[string(e)]; !ok {
			t.Errorf("no %s entry in settings.json: %+v", e, events)
		}
	}
}

// Codex and opencode are not missing a file scc could write — they have no
// mechanism that would read one. A harness with no hook surface contributes
// nothing rather than a finding.
func TestHarnessWithoutAHookSurfaceGetsNothing(t *testing.T) {
	// A git repository, so the git half is genuinely available and the only thing
	// under test is what the harness half does with a tool that reads no settings.
	root := gitInit(t)
	if _, stderr, code := run(t, "init", "--no-rtk", "--codex", "--root", root); code != ExitOK {
		t.Fatalf("init: exit = %d (stderr: %s)", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, paths.CodexDir, "settings.json")); err == nil {
		t.Error("scc wrote a settings file into a harness that reads none")
	}
	// Still a clean check: the git hooks are installed and there is no harness
	// registration that could be missing.
	if _, stderr, code := run(t, "hooks", "check", "--root", root); code != ExitOK {
		t.Errorf("hooks check: exit = %d, want %d — a harness with no surface is not a gap (stderr: %s)",
			code, ExitOK, stderr)
	}
}

// Both halves absent is the one case worth an error: hooks were asked for in a
// directory with nowhere to put any.
func TestHooksReportWhenThereIsNowhereToPutThem(t *testing.T) {
	root := t.TempDir()
	if _, stderr, code := run(t, "init", "--no-rtk", "--codex", "--root", root); code != ExitOK {
		t.Fatalf("init: exit = %d (stderr: %s)", code, stderr)
	}
	_, stderr, code := run(t, "hooks", "check", "--root", root)
	if code != ExitError {
		t.Errorf("exit = %d, want %d with neither git nor a hook surface", code, ExitError)
	}
	if !strings.Contains(stderr, "git") {
		t.Errorf("stderr does not say what is missing: %q", stderr)
	}
}

// The settings file belongs to the harness and to whoever else writes into it.
// scc splices: its own entry is replaced, everything else survives — including
// keys this build has never heard of, which is the difference between editing a
// file and owning it.
func TestHarnessHooksPreserveEverythingElse(t *testing.T) {
	root := initWorkspace(t)
	mine := map[string]any{
		"model":        "opus",
		"permissions":  map[string]any{"allow": []any{"Bash(ls:*)"}},
		"somethingScc": map[string]any{"hasNeverHeardOf": true},
		"hooks": map[string]any{
			"Stop": []any{map[string]any{
				"hooks": []any{map[string]any{"type": "command", "command": "make lint"}},
			}},
			"PreToolUse": []any{map[string]any{
				"matcher": "Bash",
				"hooks":   []any{map[string]any{"type": "command", "command": "./guard.sh"}},
			}},
		},
	}
	b, err := json.MarshalIndent(mine, "", "  ")
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(paths.Claude.Settings(root), b, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, stderr, code := run(t, "hooks", "install", "--root", root); code != ExitOK {
		t.Fatalf("hooks install: exit = %d (stderr: %s)", code, stderr)
	}

	all := readSettings(t, root)
	if all["model"] != "opus" {
		t.Errorf("scc dropped a key it does not own: %+v", all)
	}
	if _, ok := all["somethingScc"]; !ok {
		t.Errorf("scc dropped a key it has never heard of: %+v", all)
	}
	if _, ok := all["permissions"]; !ok {
		t.Errorf("scc dropped the permissions block: %+v", all)
	}
	events := hookEvents(t, root)
	if _, ok := events["PreToolUse"]; !ok {
		t.Errorf("scc dropped an event it does not write: %+v", events)
	}
	// The other Stop entry is still there, alongside scc's.
	stop, _ := events["Stop"].([]any)
	if len(stop) != 2 {
		t.Errorf("Stop = %+v, want somebody else's entry kept beside scc's", stop)
	}

	// And removing takes back only scc's.
	if _, stderr, code := run(t, "hooks", "remove", "--root", root); code != ExitOK {
		t.Fatalf("hooks remove: exit = %d (stderr: %s)", code, stderr)
	}
	events = hookEvents(t, root)
	stop, _ = events["Stop"].([]any)
	if len(stop) != 1 {
		t.Errorf("Stop = %+v, want the foreign entry alone", stop)
	}
	if _, ok := events["SessionStart"]; ok {
		t.Errorf("SessionStart survived a remove: %+v", events)
	}
	if readSettings(t, root)["model"] != "opus" {
		t.Error("remove dropped a key scc does not own")
	}
}

// A settings file scc cannot parse is one scc will not rewrite. Reformatting
// somebody's broken JSON into valid JSON of scc's choosing would destroy the
// thing they were in the middle of fixing.
func TestHarnessHooksRefuseToRewriteInvalidJSON(t *testing.T) {
	root := initWorkspace(t)
	broken := "{ \"model\": \"opus\",\n"
	if err := os.WriteFile(paths.Claude.Settings(root), []byte(broken), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, stderr, code := run(t, "hooks", "install", "--root", root); code != ExitOK {
		t.Fatalf("exit = %d, want the run to report rather than fail (stderr: %s)", code, stderr)
	}
	got, err := os.ReadFile(paths.Claude.Settings(root))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != broken {
		t.Errorf("scc rewrote a file it could not parse:\n%s", got)
	}
}

// Installing twice changes nothing the second time — the command is idempotent,
// and a workspace whose settings file churned on every run would diff against
// itself in every commit.
func TestHarnessHooksAreIdempotent(t *testing.T) {
	root := initWorkspace(t)
	once, err := os.ReadFile(paths.Claude.Settings(root))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if _, stderr, code := run(t, "hooks", "install", "--root", root); code != ExitOK {
		t.Fatalf("hooks install: exit = %d (stderr: %s)", code, stderr)
	}
	twice, err := os.ReadFile(paths.Claude.Settings(root))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(once) != string(twice) {
		t.Errorf("a second install rewrote the file:\n%s\n---\n%s", once, twice)
	}
}

// SessionStart says the one thing only the agent can fix, and says it once: this
// workspace has no test command, and here is the contract for writing one.
func TestSessionStartAsksForTheTestCommand(t *testing.T) {
	root := initWorkspace(t)
	stdout, _, code := run(t, "hooks", "run", "session-start", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d — a harness hook never blocks", code, ExitOK)
	}
	ctx := additionalContext(t, stdout)
	if !strings.Contains(ctx, "check set") || !strings.Contains(ctx, "coverage") {
		t.Errorf("additionalContext does not instruct: %q", ctx)
	}

	// Once it is recorded there is nothing to say, and silence is what keeps the
	// line that does appear worth reading.
	setTest(t, root, echoJSON(10, "99"))
	stdout, _, code = run(t, "hooks", "run", "session-start", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout = %q, want nothing once the command is recorded", stdout)
	}
}

// Stop reports findings to the agent, and reports them as context rather than as
// a refusal: a hook that can stop a turn can also loop one.
func TestStopReportsFindingsWithoutBlocking(t *testing.T) {
	root := initWorkspace(t)
	skipRest(t, root, "test")
	setTest(t, root, echoJSON(10, "99"))
	if _, stderr, code := run(t, "plan", "new", "broken", "--root", root); code != ExitOK {
		t.Fatalf("plan new: exit = %d (stderr: %s)", code, stderr)
	}
	// A section the plan format does not allow is a finding the plan validator
	// reports on every run.
	plan := filepath.Join(root, "plans", "broken.md")
	raw, err := os.ReadFile(plan)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	extra := "\n## Notes\n\nnot a section this format allows.\n"
	if err := os.WriteFile(plan, append(raw, []byte(extra)...), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	stdout, _, code := run(t, "hooks", "run", "stop", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d — Stop never blocks a turn", code, ExitOK)
	}
	ctx := additionalContext(t, stdout)
	if !strings.Contains(ctx, "scc validate") {
		t.Errorf("additionalContext does not name the findings: %q", ctx)
	}
	// It never runs the suite: that is the pre-push hook's cost to pay, not every
	// turn's.
	if strings.Contains(ctx, "coverage is") {
		t.Errorf("the Stop hook ran the test gate: %q", ctx)
	}
}

// A clean workspace with nothing undelivered says nothing at all. Silence on the
// common path is what makes the line that appears worth reading.
func TestStopIsSilentWhenThereIsNothingToSay(t *testing.T) {
	root := initWorkspace(t)
	stdout, _, code := run(t, "hooks", "run", "stop", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout = %q, want nothing from a clean workspace", stdout)
	}
}

// additionalContext pulls the field the agent actually reads out of the hook's
// document, and fails the test if the envelope is not the shape the harness
// expects.
func additionalContext(t *testing.T, stdout string) string {
	t.Helper()
	if strings.TrimSpace(stdout) == "" {
		t.Fatal("the hook printed nothing")
	}
	var doc struct {
		Specific struct {
			EventName         string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout is not the harness's hook shape (%v): %q", err, stdout)
	}
	if doc.Specific.EventName == "" {
		t.Errorf("the document names no event: %q", stdout)
	}
	return doc.Specific.AdditionalContext
}

// Both hook stages sync the symbol graph. That is the deterministic half of the
// staleness problem: a stale graph answers confidently about code that changed,
// and "remember to sync" is the kind of instruction this product exists to stop
// relying on. The two moments bookend a task — the top of a session, and the end
// of every turn that just wrote code.
func TestBothStagesSyncTheGraph(t *testing.T) {
	root := initWorkspace(t)
	for _, stage := range []string{"session-start", "stop"} {
		// No CodeGraph on PATH and no graph on disk: the sync is a no-op and the
		// stage still answers. Degrading silently is the contract — a line about
		// plumbing at the end of every turn is context the agent cannot act on.
		isolatedPath(t)
		if _, stderr, code := run(t, "hooks", "run", stage, "--root", root); code != ExitOK {
			t.Errorf("%s: exit = %d, want %d without CodeGraph (stderr: %s)", stage, code, ExitOK, stderr)
		}
	}
}

// The harness is told how long to wait, and what it is told has to match what the
// stage actually does.
//
// Two shapes, and the split is the design rather than a tolerance. SessionStart
// and Stop sync the symbol graph, so a timeout shorter than a CodeGraph run is a
// hook the harness kills halfway. UserPromptSubmit runs no subprocess at all and
// sits between a keystroke and the agent starting, so its ceiling is deliberately
// an order of magnitude tighter — and a change that gave it room for a
// subprocess would mean something had started running one there.
func TestHookTimeoutsMatchWhatTheStageDoes(t *testing.T) {
	root := initWorkspace(t)
	events := hookEvents(t, root)
	for _, e := range hooks.Events() {
		entries, _ := events[string(e)].([]any)
		if len(entries) != 1 {
			t.Fatalf("%s: entries = %+v", e, entries)
		}
		entry, _ := entries[0].(map[string]any)
		inner, _ := entry["hooks"].([]any)
		first, _ := inner[0].(map[string]any)
		timeout, _ := first["timeout"].(float64)
		if int(timeout) != e.Timeout() {
			t.Errorf("%s timeout = %v, want %d", e, timeout, e.Timeout())
		}
		if e == hooks.UserPromptSubmit {
			if timeout > 15 {
				t.Errorf("%s timeout = %v: it runs on every prompt, in front of the person "+
					"waiting, and reads two directories — a subprocess-sized budget here means "+
					"something started running one", e, timeout)
			}
			continue
		}
		if timeout < 60 {
			t.Errorf("%s timeout = %v: it syncs the graph, and a timeout shorter than the work "+
				"is a hook the harness kills halfway", e, timeout)
		}
	}
}
