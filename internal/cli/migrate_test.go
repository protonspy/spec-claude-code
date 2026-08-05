package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// v1Plan is shaped like the plan this contract was written against: a decomposition,
// a checklist, and a Notes section that grew into half the file.
const v1Plan = `---
autonomy: auto
ci: wait
---

# Sweep

## Why

The old path cannot be extended.

## Decomposition

- ` + "`specs/cart-totals/`" + ` — the totals engine

## Tasks

- [ ] 1.1 (Unit) Lay the foundation
- [x] 1.2 (TDD) Build on it

## Notes

**Order matters.** cart-totals first, because everything writes through it.

**The free path wins.** The expensive command knows about the cheap one.
`

func TestMigrateMovesAPlanOntoTheContract(t *testing.T) {
	root := initWorkspace(t)
	if err := os.MkdirAll(filepath.Join(root, "specs", "cart-totals"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := writePlanFile(t, root, "sweep", v1Plan)

	stdout, stderr, code := run(t, "plan", "migrate", "sweep", "--root", root)
	// The plan is missing `## Done when` and a description, so migration creates the
	// section empty and lets the findings appear. A placeholder that satisfied the
	// validator would be a plan that lies about being complete.
	if code != ExitFindings {
		t.Fatalf("migrate = %d, want the remaining findings reported (%s%s)", code, stdout, stderr)
	}

	body := readFile(t, path)
	if strings.Contains(body, "## Notes") {
		t.Error("Notes survived the migration")
	}
	if strings.Contains(body, "## Decomposition") || !strings.Contains(body, "## References") {
		t.Errorf("Decomposition was not renamed:\n%s", body)
	}
	if !strings.Contains(body, "`specs/cart-totals/`") {
		t.Error("the spec reference did not survive the rename")
	}
	if !strings.Contains(body, "## Done when") {
		t.Error("the missing required section was not created")
	}
	if !strings.Contains(body, "status: draft") {
		t.Error("migration must never approve; approving is a human act")
	}
	if !strings.Contains(body, "- [x] 1.2") {
		t.Error("migration lost a task's state")
	}

	// Nothing is deleted. plans/archive/ is safe because the plan scanner reads
	// plans/ with ReadDir and skips directories.
	archived := readFile(t, filepath.Join(root, "plans", "archive", "sweep-notes.md"))
	for _, want := range []string{"Order matters", "The free path wins"} {
		if !strings.Contains(archived, want) {
			t.Errorf("the archive lost %q", want)
		}
	}
	if _, _, code := run(t, "plan", "list", "--root", root); code != ExitOK {
		t.Error("the archive directory broke the plan listing")
	}
	if _, _, code := run(t, "validate", "--root", root); code == ExitError {
		t.Error("the archive directory broke validation")
	}
}

func TestMigrateDryRunWritesNothing(t *testing.T) {
	root := initWorkspace(t)
	path := writePlanFile(t, root, "sweep", v1Plan)
	before := readFile(t, path)
	if _, _, code := run(t, "plan", "migrate", "sweep", "--dry-run", "--root", root); code != ExitOK {
		t.Fatal("dry run failed")
	}
	if readFile(t, path) != before {
		t.Error("--dry-run wrote to the file")
	}
	if _, err := os.Stat(filepath.Join(root, "plans", "archive")); err == nil {
		t.Error("--dry-run created the archive")
	}
}

// An approved plan is already on a contract somebody signed off, so there is nothing
// to migrate and rewriting it would be the one thing approval exists to prevent.
func TestMigrateRefusesAnApprovedPlan(t *testing.T) {
	root := initWorkspace(t)
	writePlanFile(t, root, "sweep", v2Plan)
	if _, _, code := run(t, "plan", "approve", "sweep", "--root", root); code != ExitOK {
		t.Fatal("approve failed")
	}
	if _, _, code := run(t, "plan", "migrate", "sweep", "--root", root); code != ExitError {
		t.Error("migrating an approved plan should be refused")
	}
}
