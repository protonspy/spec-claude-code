package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validPlan is a plan under the v2 contract: six sections and no others, which is
// what `plan validate` and `plan approve` hold a file to. mapPlan is deliberately a
// v1 plan — it is what the map reads were measured against — so the seal tests need
// their own.
const validPlan = `---
autonomy: auto
ci: wait
---

# Sample plan

What this work is, in one sentence.

## Why

What this is for, and what done means for the whole of it.

## Out of scope

- The thing this is not.

## Tasks

- [x] 1.1 (Unit) Build the parser, and prove with a test that it reads a fenced
      block without treating the example inside it as a task
- [ ] 1.2 (TDD) Guard the credential before the provider client lands

## Done when

- The suite is green and the validators report nothing.
`

func sealWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if _, _, code := run(t, "init", "--no-rtk", "--no-hooks", "--root", root); code != ExitOK {
		t.Fatalf("init: exit %d", code)
	}
	dir := filepath.Join(root, "plans")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sample.md"), []byte(validPlan), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// `scc plan validate` is the same validators narrowed to one plan, and it returns 2
// on findings like every other lint question — a finding is a legitimate answer,
// not a failure of the tool, and collapsing the two makes the contract useless to
// the agent branching on it.
func TestPlanValidateReportsFindingsAsTwo(t *testing.T) {
	root := sealWorkspace(t)
	path := filepath.Join(root, "plans", "sample.md")

	if _, stderr, code := run(t, "plan", "validate", "sample", "--root", root); code != ExitOK {
		t.Fatalf("a clean plan: exit %d (%s)", code, stderr)
	}

	// A heading nobody agreed to is the finding the closed section set exists for:
	// the 56KB plan that motivated the contract got there through `## Notes`.
	if err := os.WriteFile(path, []byte(validPlan+"\n## Notes\n\nAn essay starts here.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, _, code := run(t, "plan", "validate", "sample", "--root", root); code != ExitFindings {
		t.Errorf("a plan with an unknown section exited %d, want %d", code, ExitFindings)
	}

	// A plan that is not there is a usage error rather than a finding.
	if _, _, code := run(t, "plan", "validate", "no-such-plan", "--root", root); code != ExitError {
		t.Errorf("a plan that does not exist exited %d, want %d", code, ExitError)
	}
}

// The seal is tamper-evidence, and reseal is the answer to a legitimate edit made
// outside the cycle — deliberately a separate forced command, because the same
// call made automatically would erase the evidence it exists to keep.
func TestResealNeedsForceOnceThePlanHasDrifted(t *testing.T) {
	root := sealWorkspace(t)
	path := filepath.Join(root, "plans", "sample.md")

	if _, stderr, code := run(t, "plan", "approve", "sample", "--root", root); code != ExitOK {
		t.Fatalf("plan approve: exit %d (%s)", code, stderr)
	}
	sealed, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(sealed), "status: approved") || !strings.Contains(string(sealed), "checksum:") {
		t.Fatalf("approve wrote neither half of the seal:\n%s", sealed)
	}

	// Edit it by hand, which is what the seal exists to notice.
	drifted := strings.Replace(string(sealed), "What this work is, in one sentence.",
		"What this work is, edited by hand.", 1)
	if err := os.WriteFile(path, []byte(drifted), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Drift is checked before an edit is applied rather than by a validator, and
	// that is the whole value: a harness that edited by hand and then patched
	// would otherwise have its edit resealed by the command that should report it.
	stdout, stderr, code := run(t, "patch", "check", "sample", "1.2", "--root", root)
	if code == ExitOK {
		t.Errorf("a patch onto a drifted plan succeeded: %s %s", stdout, stderr)
	}
	if !strings.Contains(stdout+stderr, "reseal") {
		t.Errorf("the refusal does not name the way past it: %s %s", stdout, stderr)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(after) != drifted {
		t.Error("the refused patch changed the file anyway")
	}

	if _, stderr, code := run(t, "plan", "reseal", "sample", "--force", "--root", root); code != ExitOK {
		t.Fatalf("reseal --force: exit %d (%s)", code, stderr)
	}
	if _, _, code := run(t, "patch", "check", "sample", "1.2", "--root", root); code != ExitOK {
		t.Errorf("a patch onto a resealed plan exited %d", code)
	}
	// And the status survives: reseal answers "this edit was legitimate", never
	// "approve this".
	final, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(final), "status: approved") {
		t.Errorf("reseal dropped the status: %s", final)
	}
}

// A plan name becomes a path segment, so it goes through SafeName first: without
// it `scc plan new ..` resolves to the workspace root.
func TestPlanNewRefusesHostileNamesAndDoesNotOverwrite(t *testing.T) {
	root := sealWorkspace(t)

	for _, name := range []string{"..", "../outside", "nested/name"} {
		if _, _, code := run(t, "plan", "new", name, "--root", root); code == ExitOK {
			t.Errorf("plan new %q exited 0", name)
		}
	}
	if _, stderr, code := run(t, "plan", "new", "second-plan", "--root", root); code != ExitOK {
		t.Errorf("plan new second-plan: exit %d (%s)", code, stderr)
	}
	// Twice is a refusal rather than an overwrite: the file is the user's once it
	// exists.
	if _, _, code := run(t, "plan", "new", "second-plan", "--root", root); code == ExitOK {
		t.Error("plan new overwrote a plan that was already there")
	}
}
