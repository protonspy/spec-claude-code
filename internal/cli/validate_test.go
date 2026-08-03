package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/assets"
	"github.com/protonspy/spec-claude-code/internal/paths"
	"github.com/protonspy/spec-claude-code/internal/validate"
)

// The property that decides whether this tool is usable: what scc generates passes
// what scc checks. A validator that fires on the file the tool just wrote teaches the
// user to ignore all of them on their first run.
func TestFreshArtifactsPassTheirOwnValidators(t *testing.T) {
	root := initWorkspace(t)
	if _, stderr, code := run(t, "spec", "new", "user-auth", "--root", root); code != ExitOK {
		t.Fatalf("spec new: %d (%s)", code, stderr)
	}
	if _, stderr, code := run(t, "plan", "new", "checkout-revamp", "--root", root); code != ExitOK {
		t.Fatalf("plan new: %d (%s)", code, stderr)
	}
	stdout, stderr, code := run(t, "validate", "--root", root, "--json")
	if code != ExitOK {
		t.Errorf("validate on a freshly scaffolded workspace exited %d, want %d\nstdout: %s\nstderr: %s",
			code, ExitOK, stdout, stderr)
	}
	var doc struct {
		Findings   []map[string]any `json:"findings"`
		Count      int              `json:"count"`
		Validators []struct {
			Validator string `json:"validator"`
			Findings  int    `json:"findings"`
		} `json:"validators"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout is not valid JSON (%v): %q", err, stdout)
	}
	if doc.Count != 0 {
		t.Errorf("findings on fresh artifacts: %v", doc.Findings)
	}
	// Every validator ran and reported, even the ones with nothing to look at — which
	// is what lets `scc validate` be the single gate instead of a list the user has to
	// keep in sync.
	if len(doc.Validators) != len(validate.All()) {
		t.Errorf("validators = %+v, want one entry per validator (%d)", doc.Validators, len(validate.All()))
	}
}

// The shipped skills pass scc's own Agent Skills validator — and there are shipped
// skills for it to pass. The count check is the point: without it this test stays
// green on a workspace containing no skills at all, which is what it asserted for as
// long as none shipped.
func TestScaffoldedSkillsPassTheSkillValidator(t *testing.T) {
	root := initWorkspace(t)

	entries, err := os.ReadDir(paths.Claude.Skills(root))
	if err != nil {
		t.Fatalf("init scaffolded no skills directory: %v", err)
	}
	var got []string
	for _, e := range entries {
		if e.IsDir() {
			got = append(got, e.Name())
		}
	}
	if len(got) != len(assets.Skills()) {
		t.Errorf("scaffolded skills = %v, want %v", got, assets.Skills())
	}

	stdout, stderr, code := run(t, "skill", "validate", "--root", root, "--json")
	if code != ExitOK {
		t.Errorf("skill validate on the skills scc itself ships exited %d\nstdout: %s\nstderr: %s",
			code, stdout, stderr)
	}
}

// Every harness's tree is validated on its own terms: a workspace scaffolded for
// Codex or opencode has to pass the same gate, and the skills validator has to
// find the skills where that harness keeps them rather than only under .claude/.
func TestAFreshWorkspacePassesValidateInEveryHarness(t *testing.T) {
	for _, h := range paths.Harnesses() {
		root := t.TempDir()
		if _, stderr, code := run(t, "init", "--"+h.ID, "--root", root); code != ExitOK {
			t.Fatalf("%s: init: %d (%s)", h.ID, code, stderr)
		}
		if _, stderr, code := run(t, "spec", "new", "user-auth", "--root", root); code != ExitOK {
			t.Fatalf("%s: spec new: %d (%s)", h.ID, code, stderr)
		}
		stdout, stderr, code := run(t, "validate", "--root", root, "--json")
		if code != ExitOK {
			t.Errorf("%s: validate on a fresh workspace exited %d\nstdout: %s\nstderr: %s",
				h.ID, code, stdout, stderr)
		}
		// And the skills really were looked at: a broken one in this harness's
		// directory has to be found, or the pass above means nothing.
		bad := filepath.Join(h.Skills(root), "Bad_Name")
		if err := os.MkdirAll(bad, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(filepath.Join(bad, "SKILL.md"), []byte("---\nname: Bad_Name\n---\nbody\n"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if _, _, code := run(t, "skill", "validate", "--root", root); code != ExitFindings {
			t.Errorf("%s: a broken skill under %s went unreported (exit %d)", h.ID, h.Skills(root), code)
		}
	}
}

// Exit 2 is the contract: a finding is a legitimate answer to a lint question, not a
// failure of the tool, and CI branches on the difference.
func TestValidateExitsTwoOnFindings(t *testing.T) {
	root := initWorkspace(t)
	if _, _, code := run(t, "spec", "new", "billing", "--root", root); code != ExitOK {
		t.Fatal("setup failed")
	}
	// One task, no methodology: the finding this whole practice exists to produce.
	tasks := "# Billing — tasks\n\n- [ ] 1.1 Compute the total — R1.1\n"
	if err := os.WriteFile(paths.Tasks(root, "billing"), []byte(tasks), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, stderr, code := run(t, "spec", "validate", "billing", "--root", root)
	if code != ExitFindings {
		t.Errorf("exit = %d, want %d", code, ExitFindings)
	}
	if !strings.Contains(stderr, "task.missing-methodology") {
		t.Errorf("stderr = %q, want the rule slug so a CI job can filter on it", stderr)
	}

	// The same run under --json: findings on stdout, exit still 2.
	stdout, _, code := run(t, "spec", "validate", "billing", "--root", root, "--json")
	if code != ExitFindings {
		t.Errorf("--json exit = %d, want %d", code, ExitFindings)
	}
	var doc struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout is not valid JSON (%v): %q", err, stdout)
	}
	if doc.Count == 0 {
		t.Error("--json reported no findings while the exit code said there were some")
	}
}

func TestSpecValidateDefaultsToEverySpec(t *testing.T) {
	root := initWorkspace(t)
	for _, name := range []string{"alpha", "beta"} {
		if _, _, code := run(t, "spec", "new", name, "--root", root); code != ExitOK {
			t.Fatal("setup failed")
		}
	}
	if err := os.WriteFile(paths.Tasks(root, "beta"), []byte("# tasks\n\n- [ ] 1.1 No methodology — R1.1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, _, code := run(t, "spec", "validate", "--root", root); code != ExitFindings {
		t.Errorf("bare `spec validate` exited %d, want it to have checked every spec", code)
	}
}

func TestValidateRejectsHostileAndExtraArguments(t *testing.T) {
	root := initWorkspace(t)
	for _, args := range [][]string{
		{"spec", "validate", "..", "--root", root},
		{"spec", "validate", "a", "b", "--root", root},
		{"plan", "validate", "..", "--root", root},
		{"validate", "extra", "--root", root},
	} {
		if _, _, code := run(t, args...); code != ExitError {
			t.Errorf("%v: exit = %d, want %d", args, code, ExitError)
		}
	}
}

func TestSkillValidate(t *testing.T) {
	root := initWorkspace(t)
	// No skills ship, so there is nothing to find — and that is exit 0, not a
	// complaint about an absent directory.
	if _, _, code := run(t, "skill", "validate", "--root", root); code != ExitOK {
		t.Errorf("skill validate on a workspace with no skills exited %d, want %d", code, ExitOK)
	}

	dir := filepath.Join(paths.Claude.Skills(root), "pdf-processing")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	body := "---\nname: pdf-tools\ndescription: Does things with PDFs. Use when handling PDFs.\n---\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, stderr, code := run(t, "skill", "validate", "--root", root)
	if code != ExitFindings {
		t.Errorf("exit = %d, want %d", code, ExitFindings)
	}
	if !strings.Contains(stderr, "skill.name-mismatch") {
		t.Errorf("stderr = %q, want the name-mismatch finding", stderr)
	}
}

// Whatever skills scc ships must pass scc's own validator. A tool that ships
// non-conforming skills has no standing to check anyone else's.
func TestShippedSkillsPassTheirOwnValidator(t *testing.T) {
	root := initWorkspace(t)
	if _, stderr, code := run(t, "skill", "validate", "--root", root); code != ExitOK {
		t.Errorf("the scaffolded workspace's skills do not pass skill validate: exit %d (%s)", code, stderr)
	}
}

func TestValidateRequiresAWorkspace(t *testing.T) {
	bare := t.TempDir()
	for _, args := range [][]string{
		{"validate", "--root", bare},
		{"spec", "validate", "--root", bare},
		{"plan", "validate", "--root", bare},
		{"skill", "validate", "--root", bare},
	} {
		_, stderr, code := run(t, args...)
		if code != ExitError {
			t.Errorf("%v: exit = %d, want %d", args, code, ExitError)
		}
		if !strings.Contains(stderr, "not an scc workspace") {
			t.Errorf("%v: stderr = %q", args, stderr)
		}
	}
}

func TestSkillUsageAndUnknownSubcommand(t *testing.T) {
	if _, stderr, code := run(t, "skill"); code != ExitError || !strings.Contains(stderr, "Usage:") {
		t.Errorf("bare `skill` = (%d, %q), want a usage error", code, stderr)
	}
	if _, stderr, code := run(t, "skill", "nope"); code != ExitError || !strings.Contains(stderr, "unknown skill subcommand") {
		t.Errorf("`skill nope` = (%d, %q)", code, stderr)
	}
	if _, _, code := run(t, "skill", "help"); code != ExitOK {
		t.Error("`skill help` did not exit 0")
	}
}
