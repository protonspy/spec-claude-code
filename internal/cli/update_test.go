package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/manifest"
	"github.com/protonspy/spec-claude-code/internal/paths"
	"github.com/protonspy/spec-claude-code/internal/workspace"
)

// A workspace this build just scaffolded is current, so update says so and exits
// 0 without asking anything.
func TestUpdateOnACurrentWorkspaceDoesNothing(t *testing.T) {
	root := initWorkspace(t)
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
	missing := ".claude/skills/adr/SKILL.md"
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
		if _, stderr, code := run(t, "init", "--"+h.ID, "--root", root); code != ExitOK {
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
