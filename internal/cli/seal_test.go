package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// v2Plan is a plan on the closed-section contract, ready to approve.
const v2Plan = `---
autonomy: auto
ci: wait
---

# Sweep

Replace the legacy path, one group at a time.

## Why

The old path cannot be extended without a rewrite.

## Tasks

- [ ] 1.1 (Unit) Lay the foundation
- [ ] 1.2 (TDD) Build on it
  _Depends 1.1_

## Done when

- the suite is green
`

func writePlanFile(t *testing.T, root, name, content string) string {
	t.Helper()
	dir := filepath.Join(root, "plans")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name+".md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPlanApproveSealsAValidPlan(t *testing.T) {
	root := initWorkspace(t)
	path := writePlanFile(t, root, "sweep", v2Plan)

	stdout, stderr, code := run(t, "plan", "approve", "sweep", "--root", root)
	if code != ExitOK {
		t.Fatalf("approve = %d (%s)", code, stderr)
	}
	if !strings.Contains(stdout+stderr, "approved") {
		t.Errorf("output = %q / %q", stdout, stderr)
	}
	body := readFile(t, path)
	if !strings.Contains(body, "status: approved") || !strings.Contains(body, "checksum: ") {
		t.Fatalf("the plan was not sealed:\n%s", body)
	}

	// And an unchanged sealed plan reads without complaint.
	if _, _, code := run(t, "map", "tasks", "sweep", "--root", root, "--next"); code != ExitOK {
		t.Errorf("reading a sealed, unchanged plan = %d", code)
	}
}

// Approving a plan that is already wrong would freeze the defect, and unfreezing it
// would then need --force. So approval is where the validator has to be clean.
func TestPlanApproveRefusesFindings(t *testing.T) {
	root := initWorkspace(t)
	writePlanFile(t, root, "broken", "# Broken\n\nWhat this is.\n\n## Tasks\n\n- [ ] 1.1 Do it\n")
	if _, _, code := run(t, "plan", "approve", "broken", "--root", root); code != ExitFindings {
		t.Errorf("approve of a plan with findings = %d, want %d", code, ExitFindings)
	}
	if body := readFile(t, filepath.Join(root, "plans", "broken.md")); strings.Contains(body, "status:") {
		t.Error("a refused approval still wrote the status")
	}
}

// Drift is the whole point: an edit made outside scc has to become visible at the
// next command that touches the file.
func TestDriftIsReportedAndActionable(t *testing.T) {
	root := initWorkspace(t)
	path := writePlanFile(t, root, "sweep", v2Plan)
	if _, _, code := run(t, "plan", "approve", "sweep", "--root", root); code != ExitOK {
		t.Fatalf("approve = %d", code)
	}

	body := readFile(t, path)
	if err := os.WriteFile(path, []byte(strings.Replace(body, "- [ ] 1.1", "- [x] 1.1", 1)), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := run(t, "map", "tasks", "sweep", "--root", root)
	if code != ExitFindings {
		t.Errorf("reading a drifted plan = %d, want %d", code, ExitFindings)
	}
	for _, want := range []string{"drift", "plan reseal"} {
		if !strings.Contains(stdout+stderr, want) {
			t.Errorf("the drift report does not mention %q:\n%s%s", want, stdout, stderr)
		}
	}

	// A patch has to refuse *before* it applies, or it reseals the hand edit on top
	// and destroys the evidence in the same command that should have reported it.
	if _, _, code := run(t, "patch", "check", "sweep", "1.2", "--root", root); code != ExitFindings {
		t.Errorf("patching a drifted plan = %d, want %d", code, ExitFindings)
	}

	// --no-verify is the diagnosis escape hatch, and nothing else.
	if _, _, code := run(t, "map", "tasks", "sweep", "--root", root, "--no-verify"); code != ExitOK {
		t.Errorf("--no-verify = %d", code)
	}

	if _, _, code := run(t, "plan", "reseal", "sweep", "--root", root); code != ExitError {
		t.Error("reseal without --force should refuse")
	}
	if _, _, code := run(t, "plan", "reseal", "sweep", "--force", "--root", root); code != ExitOK {
		t.Error("reseal --force should accept the edit")
	}
	if _, _, code := run(t, "map", "tasks", "sweep", "--root", root); code != ExitOK {
		t.Error("after reselling, the plan reads clean")
	}
}

// Ticking a box through scc has to leave the plan sealed, or the next read reports
// drift the tool itself caused.
func TestPatchReSealsWhatItWrites(t *testing.T) {
	root := initWorkspace(t)
	writePlanFile(t, root, "sweep", v2Plan)
	if _, _, code := run(t, "plan", "approve", "sweep", "--root", root); code != ExitOK {
		t.Fatalf("approve = %d", code)
	}
	if _, stderr, code := run(t, "patch", "check", "sweep", "1.1", "--root", root); code != ExitOK {
		t.Fatalf("patch check = %d (%s)", code, stderr)
	}
	if _, _, code := run(t, "map", "tasks", "sweep", "--root", root, "--next"); code != ExitOK {
		t.Error("a plan scc itself ticked reported drift")
	}
}

// What discovery may and may not do, once a plan is approved.
func TestApprovedPlanGuards(t *testing.T) {
	root := initWorkspace(t)
	writePlanFile(t, root, "sweep", v2Plan)
	if _, _, code := run(t, "plan", "approve", "sweep", "--root", root); code != ExitOK {
		t.Fatalf("approve = %d", code)
	}
	path := filepath.Join(root, "plans", "sweep.md")

	refused := [][]string{
		{"patch", "task", "sweep", "1.1", "--text", "something else"},
		{"patch", "task", "sweep", "1.1", "--method", "TDD"},
		{"patch", "task", "sweep", "1.1", "--number", "1.9"},
		{"patch", "append", "sweep", "#why", "--text", "and another reason"},
		{"patch", "replace", "sweep", "#done-when", "--text", "- nothing"},
		{"patch", "rm", "sweep", "1.2"},
		{"patch", "add", "sweep", "--text", "new work", "--number", "1.3", "--reason", "found it"},
		{"patch", "add", "sweep", "--text", "new work", "--group", "1"},
	}
	for _, args := range refused {
		if _, _, code := run(t, append(args, "--root", root)...); code != ExitError {
			t.Errorf("%v on an approved plan = %d, want refused", args, code)
		}
	}

	allowed := [][]string{
		{"patch", "check", "sweep", "1.1"},
		{"patch", "fm", "sweep", "pr=per-group"},
		{"patch", "task", "sweep", "1.2", "--priority", "1"},
	}
	for _, args := range allowed {
		if _, stderr, code := run(t, append(args, "--root", root)...); code != ExitOK {
			t.Errorf("%v on an approved plan = %d, want allowed (%s)", args, code, stderr)
		}
	}

	// Discovery: a number is allocated rather than chosen, and the reason is recorded.
	if _, stderr, code := run(t, "patch", "add", "sweep", "--text", "the thing nobody saw coming",
		"--group", "1", "--reason", "turned up while doing 1.1", "--root", root); code != ExitOK {
		t.Fatalf("discovery add = %d (%s)", code, stderr)
	}
	body := readFile(t, path)
	if !strings.Contains(body, "1.3 (Unit) the thing nobody saw coming") {
		t.Errorf("the added task did not take the next number:\n%s", body)
	}
	if !strings.Contains(body, "_Reason turned up while doing 1.1_") {
		t.Errorf("the reason was not recorded:\n%s", body)
	}

	// And a removal keeps its line, so the number is never handed out twice.
	if _, _, code := run(t, "patch", "rm", "sweep", "1.3", "--reason", "it was already covered", "--root", root); code != ExitOK {
		t.Fatalf("discovery rm = %d", code)
	}
	body = readFile(t, path)
	if !strings.Contains(body, "1.3 (Unit) the thing nobody saw coming") {
		t.Error("a discovery removal deleted the line")
	}
	if !strings.Contains(body, "_Status removed_") {
		t.Error("a discovery removal did not record the status")
	}
	if _, _, code := run(t, "patch", "add", "sweep", "--text", "another one",
		"--group", "1", "--reason", "and another", "--root", root); code != ExitOK {
		t.Fatal("second discovery add failed")
	}
	if body = readFile(t, path); !strings.Contains(body, "1.4 (Unit) another one") {
		t.Errorf("the number of a removed task was reused:\n%s", body)
	}
}

// A draft is authorship: everything is editable, rm deletes, and add takes a number.
func TestDraftPlanIsFullyEditable(t *testing.T) {
	root := initWorkspace(t)
	writePlanFile(t, root, "sweep", v2Plan)
	for _, args := range [][]string{
		{"patch", "task", "sweep", "1.1", "--text", "something else"},
		{"patch", "append", "sweep", "#why", "--text", "and another reason"},
		{"patch", "add", "sweep", "--text", "more work", "--number", "1.3"},
		{"patch", "rm", "sweep", "1.3"},
	} {
		if _, stderr, code := run(t, append(args, "--root", root)...); code != ExitOK {
			t.Errorf("%v on a draft = %d, want allowed (%s)", args, code, stderr)
		}
	}
	if body := readFile(t, filepath.Join(root, "plans", "sweep.md")); strings.Contains(body, "1.3") {
		t.Error("rm on a draft should delete the line outright")
	}
}
