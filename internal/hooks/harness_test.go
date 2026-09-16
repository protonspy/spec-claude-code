package hooks

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/paths"
)

// scaffolded makes root look like a workspace for the given harnesses, which is
// the only thing the harness-hook path asks of it: the manifest is the marker.
func scaffolded(t *testing.T, all ...paths.Harness) string {
	t.Helper()
	root := t.TempDir()
	for _, h := range all {
		if err := os.MkdirAll(h.Config(root), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(h.Manifest(root), []byte(`{"harness":"`+h.ID+`"}`), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	return root
}

func readSettings(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("the settings file scc wrote is not valid JSON: %v\n%s", err, raw)
	}
	return out
}

// Every event scc registers has to answer the three questions the report asks of
// it. A stage with no name is an entry whose command runs `scc hooks run ` and
// does nothing; a timeout of zero is the harness's own default rather than the
// one this reasoning picked.
func TestEveryEventIsFullyDescribed(t *testing.T) {
	for _, e := range Events() {
		if e.Stage() == "" {
			t.Errorf("%s has no stage name", e)
		}
		if e.Why() == "" {
			t.Errorf("%s has no line for the report", e)
		}
		if e.Timeout() <= 0 {
			t.Errorf("%s has timeout %d", e, e.Timeout())
		}
	}
	// The prompt stage sits between the keystroke and the agent, so it is an order
	// of magnitude tighter than the two that run while nobody is waiting.
	if UserPromptSubmit.Timeout() >= SessionStart.Timeout() {
		t.Errorf("the prompt stage (%ds) is not tighter than session start (%ds)",
			UserPromptSubmit.Timeout(), SessionStart.Timeout())
	}
	if Stop.Timeout() < SessionStart.Timeout() {
		t.Errorf("stop (%ds) is tighter than session start (%ds), though both sync the graph",
			Stop.Timeout(), SessionStart.Timeout())
	}
	var unknown Event = "Whatever"
	if unknown.Stage() != "" || unknown.Why() != "" {
		t.Error("an event scc does not know answered as though it did")
	}
	if unknown.Timeout() != SessionStart.Timeout() {
		t.Errorf("an unknown event's timeout is %d, want the default", unknown.Timeout())
	}
}

// Install, then install again: the second run has nothing to do, and a settings
// file that changed on every run would diff against itself in the repository it
// is committed to.
func TestInstallHarnessIsIdempotent(t *testing.T) {
	root := scaffolded(t, paths.Claude)

	first := InstallHarness(root)
	if len(first) != len(Events()) {
		t.Fatalf("got %d statuses, want %d: %+v", len(first), len(Events()), first)
	}
	for _, st := range first {
		if st.State != Installed || st.Action != Added {
			t.Errorf("%s: state=%q action=%q (%s)", st.Event, st.State, st.Action, st.Note)
		}
	}
	before, err := os.ReadFile(paths.Claude.Settings(root))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	for _, st := range InstallHarness(root) {
		if st.State != Installed || st.Action != Present {
			t.Errorf("second run: %s state=%q action=%q", st.Event, st.State, st.Action)
		}
	}
	after, err := os.ReadFile(paths.Claude.Settings(root))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("the second install rewrote the file:\n%s\n---\n%s", before, after)
	}
	if !HarnessOK(LookHarness(root)) {
		t.Error("LookHarness does not agree the hooks are installed")
	}
}

// The file belongs to the harness and to whoever else writes into it. scc's own
// entries are replaced; every other entry in the same event, every other event,
// and every key this build has never heard of survive exactly as they were.
func TestInstallHarnessPreservesEverythingItDoesNotOwn(t *testing.T) {
	root := scaffolded(t, paths.Claude)
	path := paths.Claude.Settings(root)
	if err := os.MkdirAll(paths.Claude.Config(root), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	const existing = `{
  "model": "opus",
  "permissions": {"allow": ["Bash(ls:*)"]},
  "somethingSccHasNeverHeardOf": {"deep": [1, 2]},
  "hooks": {
    "Stop": [{"hooks": [{"type": "command", "command": "make lint"}]}],
    "PreToolUse": [{"hooks": [{"type": "command", "command": "somebody-elses-tool"}]}]
  }
}`
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	InstallHarness(root)
	got := readSettings(t, path)

	for _, key := range []string{"model", "permissions", "somethingSccHasNeverHeardOf"} {
		if _, ok := got[key]; !ok {
			t.Errorf("install dropped %q from somebody else's settings file", key)
		}
	}
	hooks, _ := got["hooks"].(map[string]any)
	if _, ok := hooks["PreToolUse"]; !ok {
		t.Error("install dropped an event scc does not register")
	}
	stop, _ := hooks["Stop"].([]any)
	if len(stop) != 2 {
		t.Fatalf("Stop has %d entries, want somebody else's plus scc's: %+v", len(stop), stop)
	}
	if !hasCommand(stop, "make lint") {
		t.Errorf("install replaced somebody else's Stop hook: %+v", stop)
	}
	if !hasCommand(stop, command(StageStop)) {
		t.Errorf("install did not add scc's own Stop hook: %+v", stop)
	}
}

// Remove is the same promise from the other side, and the detail that matters is
// what it leaves: somebody else's entries, and no empty key where scc's used to
// be. A key holding an empty array is scc's litter in a file it does not own.
func TestRemoveHarnessLeavesNoLitter(t *testing.T) {
	root := scaffolded(t, paths.Claude)
	path := paths.Claude.Settings(root)
	if err := os.MkdirAll(paths.Claude.Config(root), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	const existing = `{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "make lint"}]}]}}`
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	InstallHarness(root)
	for _, st := range RemoveHarness(root) {
		if st.State != Missing || st.Action != Removed {
			t.Errorf("%s: state=%q action=%q", st.Event, st.State, st.Action)
		}
	}

	got := readSettings(t, path)
	hooks, ok := got["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("remove took the whole hooks object with somebody else's entry in it: %+v", got)
	}
	if _, ok := hooks[string(SessionStart)]; ok {
		t.Error("remove left an empty SessionStart key behind")
	}
	stop, _ := hooks[string(Stop)].([]any)
	if len(stop) != 1 || !hasCommand(stop, "make lint") {
		t.Errorf("remove did not leave somebody else's Stop hook alone: %+v", stop)
	}

	// A second remove has nothing to take out and says so rather than rewriting.
	for _, st := range RemoveHarness(root) {
		if st.Action != Skipped {
			t.Errorf("second remove: %s action=%q, want %q", st.Event, st.Action, Skipped)
		}
	}
}

func hasCommand(entries []any, want string) bool {
	for _, e := range entries {
		obj, _ := e.(map[string]any)
		inner, _ := obj["hooks"].([]any)
		for _, h := range inner {
			hook, _ := h.(map[string]any)
			if cmd, _ := hook["command"].(string); cmd == want {
				return true
			}
		}
	}
	return false
}

// A file scc cannot parse is a file scc will not rewrite: reformatting somebody's
// broken JSON into valid JSON of scc's choosing destroys what they were in the
// middle of fixing.
func TestABrokenSettingsFileIsReportedAndNotRewritten(t *testing.T) {
	root := scaffolded(t, paths.Claude)
	path := paths.Claude.Settings(root)
	if err := os.MkdirAll(paths.Claude.Config(root), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	const broken = "{\n  \"model\": \"opus\",\n"
	if err := os.WriteFile(path, []byte(broken), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	all := InstallHarness(root)
	if len(all) != 1 || all[0].State != Foreign || all[0].Note == "" {
		t.Fatalf("install on a broken file = %+v, want one Foreign status with a note", all)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(raw) != broken {
		t.Errorf("scc rewrote a file it could not parse:\n%s", raw)
	}
}

// An empty file is one somebody created and never filled in, not a syntax error
// to refuse over.
func TestAnEmptySettingsFileIsFilledInRatherThanRefused(t *testing.T) {
	root := scaffolded(t, paths.Claude)
	path := paths.Claude.Settings(root)
	if err := os.MkdirAll(paths.Claude.Config(root), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte("   \n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if !HarnessOK(InstallHarness(root)) {
		t.Error("install refused an empty settings file")
	}
}

// Recognition is by the exact command, because a loose match is scc editing
// somebody else's entry. Both of these were real near-misses for a substring
// test: one is another tool, the other is scc's command with something appended.
func TestOnlySccsOwnEntriesAreRecognized(t *testing.T) {
	for _, cmd := range []string{
		command(StageStop),
		"  " + command(StageStop) + "  ",
	} {
		if !ours(cmd) {
			t.Errorf("ours(%q) = false, and scc wrote that command", cmd)
		}
	}
	for _, cmd := range []string{
		"my-" + command(StageStop),
		command(StageStop) + " | tee log",
		"scc validate",
		"",
	} {
		if ours(cmd) {
			t.Errorf("ours(%q) = true, and scc did not write that command", cmd)
		}
	}
}

// An entry that is current but spelled with different whitespace or key order is
// not stale: the comparison is by value, because the file is hand-editable and a
// byte comparison would report a rewrite on every formatting difference.
func TestAReformattedEntryIsNotStale(t *testing.T) {
	root := scaffolded(t, paths.Claude)
	InstallHarness(root)
	path := paths.Claude.Settings(root)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	compact, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(path, compact, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if !HarnessOK(LookHarness(root)) {
		t.Error("a reformatted settings file was reported as not installed")
	}

	// An entry from another build — same command, different timeout — is stale,
	// and installing replaces it rather than adding a second one.
	stale := `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"` +
		command(StageStop) + `","timeout":1}]}]}}`
	if err := os.WriteFile(path, []byte(stale), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	var replaced bool
	for _, st := range InstallHarness(root) {
		if st.Event == Stop {
			replaced = st.Action == Replaced
		}
	}
	if !replaced {
		t.Error("an entry from another build was not reported as replaced")
	}
	hooks, _ := readSettings(t, path)["hooks"].(map[string]any)
	if entries, _ := hooks[string(Stop)].([]any); len(entries) != 1 {
		t.Errorf("replacing left %d entries, want one: %+v", len(entries), entries)
	}
}

// Codex and opencode are not missing a file scc could write — they have no
// mechanism that would read one, so they contribute nothing rather than a
// finding.
func TestAHarnessWithNoHookSurfaceContributesNothing(t *testing.T) {
	root := scaffolded(t, paths.Codex, paths.OpenCode)
	if all := InstallHarness(root); len(all) != 0 {
		t.Errorf("install reported %+v for harnesses with no hook surface", all)
	}
	if all := LookHarness(root); len(all) != 0 {
		t.Errorf("look reported %+v for harnesses with no hook surface", all)
	}
	// And HarnessOK over nothing is true: there is nothing missing.
	if !HarnessOK(nil) {
		t.Error("HarnessOK(nil) is false, so a Codex-only workspace reports a failure")
	}
}
