package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/codegraph"
	"github.com/protonspy/spec-claude-code/internal/hooks"
	"github.com/protonspy/spec-claude-code/internal/manifest"
	"github.com/protonspy/spec-claude-code/internal/paths"
	"github.com/protonspy/spec-claude-code/internal/rtk"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// A workspace this build just scaffolded is current, so update says so and exits
// 0 without asking anything — RTK's block in the entry file included, since init
// wrote it and it is not an edit of the user's.
func TestUpdateOnACurrentWorkspaceDoesNothing(t *testing.T) {
	isolatedPath(t)
	root := t.TempDir()
	if _, stderr, code := run(t, "init", "--claude", "--root", root); code != ExitOK {
		t.Fatalf("init: exit = %d (%s)", code, stderr)
	}
	stdout, stderr, code := run(t, "update", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit = %d (%s)", code, stderr)
	}
	if !strings.Contains(stdout, "nothing to do") {
		t.Errorf("stdout = %q, want it to report there is nothing to do", stdout)
	}
}

func TestUpdateNeedsAWorkspace(t *testing.T) {
	_, stderr, code := run(t, "update", "--root", t.TempDir())
	if code != ExitError {
		t.Fatalf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "not an scc workspace") {
		t.Errorf("stderr = %q", stderr)
	}
}

// The summary is the point of the command: what would be created, replaced, and
// deleted, before anything is written.
func TestUpdateReportsThePlanAndWritesNothingOnADryRun(t *testing.T) {
	root := initWorkspace(t)
	missing := ".claude/skills/scc-adr/SKILL.md"
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(missing))); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	edited := ".claude/rules/tasks.md"
	mine := "# mine\n"
	if err := workspace.AtomicWrite(filepath.Join(root, filepath.FromSlash(edited)), []byte(mine), 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}

	stdout, stderr, code := run(t, "update", "--root", root, "--dry-run")
	if code != ExitOK {
		t.Fatalf("exit = %d (%s)", code, stderr)
	}
	for _, want := range []string{missing, edited, "create", "you edited these", "dry run"} {
		if !strings.Contains(stdout+stderr, want) {
			t.Errorf("the report does not mention %q:\n%s%s", want, stdout, stderr)
		}
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(missing))); !os.IsNotExist(err) {
		t.Error("--dry-run wrote a file")
	}
	if got, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(edited))); string(got) != mine {
		t.Error("--dry-run touched an edited file")
	}
}

// Without a terminal there is nobody to confirm, so an unattended run refuses
// rather than applying silently — the confirmation is the feature.
func TestUpdateWithoutATerminalRequiresYes(t *testing.T) {
	withoutTerminal(t)
	root := initWorkspace(t)
	rel := ".claude/rules/specs.md"
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	_, stderr, code := run(t, "update", "--root", root)
	if code != ExitError {
		t.Fatalf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "--yes") {
		t.Errorf("stderr = %q, want it to name --yes", stderr)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); !os.IsNotExist(err) {
		t.Error("the refused update still wrote the file")
	}
}

func TestUpdateWithYesRestoresAndRecords(t *testing.T) {
	root := initWorkspace(t)
	rel := ".claude/rules/specs.md"
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	stdout, stderr, code := run(t, "update", "--root", root, "--yes")
	if code != ExitOK {
		t.Fatalf("exit = %d (%s)", code, stderr)
	}
	if !strings.Contains(stdout, rel) {
		t.Errorf("stdout does not name the restored file: %q", stdout)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
		t.Errorf("%s was not restored: %v", rel, err)
	}
	m, found, err := manifest.Load(root, paths.Claude)
	if err != nil || !found {
		t.Fatalf("Load = (%v, %v)", found, err)
	}
	if _, ok := m.Get(rel); !ok {
		t.Error("the restored file is not in the manifest")
	}
}

// An edited file survives an update and is named in the report, so the user finds
// out rather than discovering it in a diff later.
func TestUpdateKeepsEditedFilesAndSaysSo(t *testing.T) {
	root := initWorkspace(t)
	edited := ".claude/rules/delivery.md"
	mine := "# mine\n"
	if err := workspace.AtomicWrite(filepath.Join(root, filepath.FromSlash(edited)), []byte(mine), 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}
	// Something else has to be applicable, or the command correctly finds nothing
	// to do and never reaches the report.
	missing := ".claude/rules/specs.md"
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(missing))); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	stdout, stderr, code := run(t, "update", "--root", root, "--yes")
	if code != ExitOK {
		t.Fatalf("exit = %d (%s)", code, stderr)
	}
	if got, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(edited))); string(got) != mine {
		t.Error("update overwrote an edited file without --force")
	}
	if !strings.Contains(stderr, edited) || !strings.Contains(stderr, "--force") {
		t.Errorf("the run does not report the kept file and how to take the new one:\n%s%s", stdout, stderr)
	}
}

// The confirmation is the feature: shown the plan, a person who says no gets a
// workspace nothing happened to.
func TestUpdateAsksBeforeWritingAndHonorsNo(t *testing.T) {
	for answer, wantRestored := range map[string]bool{"n\n": false, "y\n": true, "\n": false} {
		withPrompt(t, answer)
		root := initWorkspace(t)
		rel := ".claude/rules/specs.md"
		if err := os.Remove(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("Remove: %v", err)
		}
		stdout, stderr, code := run(t, "update", "--root", root)
		if code != ExitOK {
			t.Fatalf("%q: exit = %d (%s)", answer, code, stderr)
		}
		if !strings.Contains(stdout, "Apply these changes?") {
			t.Errorf("%q: update did not ask:\n%s", answer, stdout)
		}
		_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		if restored := err == nil; restored != wantRestored {
			t.Errorf("%q: restored = %v, want %v", answer, restored, wantRestored)
		}
	}
}

// --json has no way to ask: the prompt goes to stdout, which is the document the
// caller is piping into jq. The combination is refused rather than answered with
// something that is not JSON.
func TestUpdateJSONWithoutYesIsRefused(t *testing.T) {
	withPrompt(t, "y\n")
	root := initWorkspace(t)
	rel := ".claude/rules/specs.md"
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	stdout, stderr, code := run(t, "update", "--root", root, "--json")
	if code != ExitError {
		t.Fatalf("exit = %d, want %d", code, ExitError)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout is not empty: %q", stdout)
	}
	if !strings.Contains(stderr, "--yes") || !strings.Contains(stderr, "--dry-run") {
		t.Errorf("stderr = %q, want it to name both ways out", stderr)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); !os.IsNotExist(err) {
		t.Error("the refused run still wrote the file")
	}
}

func TestUpdateJSONDryRunEmitsThePlan(t *testing.T) {
	root := initWorkspace(t)
	stdout, stderr, code := run(t, "update", "--root", root, "--dry-run", "--json")
	if code != ExitOK {
		t.Fatalf("exit = %d (%s)", code, stderr)
	}
	var doc struct {
		Plans []struct {
			Harness string `json:"harness"`
			Items   []struct {
				Path   string `json:"path"`
				Action string `json:"action"`
			} `json:"items"`
		} `json:"plans"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout is not valid JSON (%v): %q", err, stdout)
	}
	if len(doc.Plans) != 1 || doc.Plans[0].Harness != paths.Claude.ID {
		t.Fatalf("plans = %+v, want one for claude", doc.Plans)
	}
	if len(doc.Plans[0].Items) == 0 {
		t.Error("the plan has no items")
	}
}

// A workspace with two trees updates both by default; a flag narrows it, and
// naming a harness the workspace does not have is an error rather than a no-op
// that reads like success.
func TestUpdateTargetsEveryInitializedHarness(t *testing.T) {
	root := t.TempDir()
	for _, h := range []paths.Harness{paths.Claude, paths.OpenCode} {
		if _, stderr, code := run(t, "init", "--no-rtk", "--"+h.ID, "--root", root); code != ExitOK {
			t.Fatalf("init --%s: %d (%s)", h.ID, code, stderr)
		}
	}
	for _, rel := range []string{".claude/rules/specs.md", ".opencode/rules/specs.md"} {
		if err := os.Remove(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("Remove: %v", err)
		}
	}
	stdout, stderr, code := run(t, "update", "--root", root, "--yes")
	if code != ExitOK {
		t.Fatalf("exit = %d (%s)", code, stderr)
	}
	for _, rel := range []string{".claude/rules/specs.md", ".opencode/rules/specs.md"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("%s was not restored: %v (%s)", rel, err, stdout)
		}
	}

	_, stderr, code = run(t, "update", "--root", root, "--codex", "--yes")
	if code != ExitError {
		t.Fatalf("exit = %d for a harness the workspace does not have, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "init --codex") {
		t.Errorf("stderr = %q, want it to say how to create that tree", stderr)
	}
}

// `scc update` reports a stale hook block and does not rewrite it: a "yes" about
// template files is not consent to edit .git.
func TestUpdateReportsStaleHooksWithoutRewritingThem(t *testing.T) {
	root := gitWorkspace(t)

	hooksDir, err := hooks.Dir(root)
	if err != nil {
		t.Fatalf("hooks.Dir: %v", err)
	}
	path := filepath.Join(hooksDir, "pre-commit")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	stale := strings.Replace(string(before), "scc hooks run pre-commit", "scc hooks run pre-commit --old", 1)
	if stale == string(before) {
		t.Fatal("the installed hook does not call back into scc, so this test's premise is gone")
	}
	if err := os.WriteFile(path, []byte(stale), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	stdout, stderr, code := run(t, "update", "--yes", "--root", root)
	if code != ExitOK {
		t.Fatalf("update: exit %d (%s)", code, stderr)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(after) != stale {
		t.Errorf("update rewrote a hook in .git:\n%s", after)
	}
	if !strings.Contains(stdout+stderr, "hooks") {
		t.Errorf("update said nothing about the stale hook:\n%s%s", stdout, stderr)
	}
}

// A workspace scaffolded without RTK's block gets it from update, planned before
// it is written like any other change — and the next update has nothing to say,
// with or without CodeGraph's block beside it, since both are scc's own writes.
func TestUpdateAddsAMissingRTKBlock(t *testing.T) {
	root := initWorkspace(t)
	stdout, stderr, code := run(t, "update", "--root", root, "--dry-run")
	if code != ExitOK {
		t.Fatalf("dry run: exit = %d (%s)", code, stderr)
	}
	if !strings.Contains(stdout, "RTK usage block") || strings.Contains(readEntry(t, root, paths.Claude.EntryFile), "rtk-instructions") {
		t.Fatalf("--dry-run did not plan the block, or wrote it:\n%s", stdout)
	}
	stdout, stderr, code = run(t, "update", "--root", root, "--dry-run", "--json")
	if code != ExitOK {
		t.Fatalf("dry run --json: exit = %d (%s)", code, stderr)
	}
	var planned struct {
		RTK []string `json:"rtk"`
	}
	if err := json.Unmarshal([]byte(stdout), &planned); err != nil {
		t.Fatalf("stdout is not valid JSON (%v): %q", err, stdout)
	}
	if len(planned.RTK) != 1 || planned.RTK[0] != paths.Claude.EntryFile {
		t.Errorf("planned rtk = %v, want [%s]", planned.RTK, paths.Claude.EntryFile)
	}

	stdout, stderr, code = run(t, "update", "--root", root, "--yes", "--json")
	if code != ExitOK {
		t.Fatalf("update: exit = %d (%s)", code, stderr)
	}
	var doc struct {
		RTK []blockFile `json:"rtk"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout is not valid JSON (%v): %q", err, stdout)
	}
	if len(doc.RTK) != 1 || doc.RTK[0].Path != paths.Claude.EntryFile || doc.RTK[0].Action != "added" {
		t.Errorf("rtk = %+v, want CLAUDE.md added", doc.RTK)
	}
	if !strings.Contains(readEntry(t, root, paths.Claude.EntryFile), "<!-- rtk-instructions") {
		t.Fatal("update --yes left no block")
	}

	entry := filepath.Join(root, paths.Claude.EntryFile)
	graph := codegraph.Markers.Open + " v1 -->\nscc graph explore\n" + codegraph.Markers.Close + "\n"
	if err := os.WriteFile(entry, []byte(readEntry(t, root, paths.Claude.EntryFile)+"\n"+graph), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	stdout, stderr, code = run(t, "update", "--root", root)
	if code != ExitOK || !strings.Contains(stdout, "nothing to do") {
		t.Errorf("second update: exit = %d, stdout = %q (%s), want nothing to do", code, stdout, stderr)
	}
}

// A block already there is kept, whatever version it claims: it may be `rtk
// init`'s own, and replacing it is `scc rtk`'s call, not an update's side effect.
func TestUpdateKeepsAnExistingRTKBlock(t *testing.T) {
	root := initWorkspace(t)
	mine := rtk.Markers.Open + " v9 -->\nmine, not scc's\n" + rtk.Markers.Close + "\n"
	entry := filepath.Join(root, paths.Claude.EntryFile)
	if err := os.WriteFile(entry, []byte(readEntry(t, root, paths.Claude.EntryFile)+"\n"+mine), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	stdout, stderr, code := run(t, "update", "--root", root, "--yes")
	if code != ExitOK {
		t.Fatalf("exit = %d (%s)", code, stderr)
	}
	if strings.Contains(stdout, "RTK") {
		t.Errorf("update planned a block that is already there:\n%s", stdout)
	}
	if got := readEntry(t, root, paths.Claude.EntryFile); !strings.Contains(got, "mine, not scc's") {
		t.Errorf("update replaced a block it did not write:\n%s", got)
	}
}

// --no-rtk leaves the entry file exactly as it is.
func TestUpdateNoRTKLeavesTheEntryFileAlone(t *testing.T) {
	root := initWorkspace(t)
	before := readEntry(t, root, paths.Claude.EntryFile)
	stdout, stderr, code := run(t, "update", "--root", root, "--yes", "--no-rtk")
	if code != ExitOK {
		t.Fatalf("exit = %d (%s)", code, stderr)
	}
	if !strings.Contains(stdout, "nothing to do") {
		t.Errorf("stdout = %q, want nothing to do under --no-rtk", stdout)
	}
	if readEntry(t, root, paths.Claude.EntryFile) != before {
		t.Error("--no-rtk still edited the entry file")
	}
}
