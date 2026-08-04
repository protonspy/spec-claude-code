package validate

import (
	"os"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/paths"
)

func writePlan(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.MkdirAll(paths.Plans(root), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(paths.Plan(root, name), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func planFindings(t *testing.T, root, name string) []string {
	t.Helper()
	set, err := Plan(root, name)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	return rules(set)
}

func TestConformingPlanHasNoFindings(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, "cart-totals", goodRequirements, goodDesign, goodTasks)
	writePlan(t, root, "checkout-revamp", `---
autonomy: auto
ci: wait
---

# Checkout revamp

## Decomposition

- `+"`specs/cart-totals/`"+` — the totals engine

## Tasks

- [ ] 1.1 (Unit) Rename the legacy endpoint
- [ ] 1.2 (TDD) Migrate the rounding helper
`)
	if got := planFindings(t, root, "checkout-revamp"); len(got) != 0 {
		t.Errorf("a conforming plan produced findings: %v", got)
	}
}

// One source of truth per item. An item that both carries a checkbox and references a
// spec keeps two records of one fact, and the copy is the one that goes stale.
func TestItemCannotBothCheckAndReference(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, "cart-totals", goodRequirements, goodDesign, goodTasks)
	writePlan(t, root, "checkout-revamp", `# Checkout revamp

- [ ] 1.1 (Unit) Build `+"`specs/cart-totals/`"+`
`)
	if got := planFindings(t, root, "checkout-revamp"); !contains(got, "plan.item-has-two-records") {
		t.Errorf("rules = %v, want plan.item-has-two-records", got)
	}
}

func TestPlanReferencesMustResolve(t *testing.T) {
	root := t.TempDir()
	writePlan(t, root, "checkout-revamp", `# Checkout revamp

## Decomposition

- specs/does-not-exist/ — nothing is here
`)
	if got := planFindings(t, root, "checkout-revamp"); !contains(got, "plan.unknown-spec") {
		t.Errorf("rules = %v, want plan.unknown-spec", got)
	}
}

// A plan's tasks carry no requirement citations — a plan has no requirements — but the
// rest of the grammar is identical, because the methodology belongs to the task and not
// to the vehicle that carried it.
func TestPlanTasksUseTheSameGrammarWithoutCitations(t *testing.T) {
	root := t.TempDir()
	writePlan(t, root, "sweep", `# Sweep

- [ ] 1.1 (Unit) A task with no citation, which is fine here
- [ ] 1.2 A task with no methodology, which is not
`)
	got := planFindings(t, root, "sweep")
	if !contains(got, "task.missing-methodology") {
		t.Errorf("rules = %v, want task.missing-methodology", got)
	}
	if contains(got, "task.missing-requirement") {
		t.Errorf("rules = %v, want no requirement citation demanded of a plan", got)
	}
}

// A plan that is neither a checklist nor a decomposition records nothing, and recording
// the work is the only reason both vehicles write a file.
func TestEmptyPlan(t *testing.T) {
	root := t.TempDir()
	writePlan(t, root, "hollow", "# Hollow\n\nSome prose and no items at all.\n")
	if got := planFindings(t, root, "hollow"); !contains(got, "plan.empty") {
		t.Errorf("rules = %v, want plan.empty", got)
	}
}

func TestPlanKickoffAnswers(t *testing.T) {
	root := t.TempDir()
	writePlan(t, root, "sweep", "---\nci: eventually\n---\n\n# Sweep\n\n- [ ] 1.1 (Unit) Do it\n")
	if got := planFindings(t, root, "sweep"); !contains(got, "plan.kickoff-invalid") {
		t.Errorf("rules = %v, want plan.kickoff-invalid", got)
	}
}

// The answers `plan-run` writes back before it starts a loop. A wrong value is worth
// a finding because the skill writes these and every later session reads them —
// `worktree: yes` would quietly decide how the rest of the plan gets built.
func TestPlanLoopAnswers(t *testing.T) {
	root := t.TempDir()
	writePlan(t, root, "sweep",
		"---\nworktree: yes\nmerge: whenever\npr: sometimes\n---\n\n# Sweep\n\n- [ ] 1.1 (Unit) Do it\n")
	got := planFindings(t, root, "sweep")
	if n := count(got, "plan.loop-invalid"); n != 3 {
		t.Errorf("rules = %v, want three plan.loop-invalid findings, got %d", got, n)
	}
}

// `pr` decides whether the loop opens a PR per group or one at the end, which is the
// difference between the review subagents running once and running once per group.
// Both spellings have to survive, and a resumed session reads this key to know
// whether `main` or the plan's branch is where its position lives.
func TestPlanPRShapeAcceptsBothLoops(t *testing.T) {
	root := t.TempDir()
	for _, shape := range []string{"per-group", "per-plan"} {
		writePlan(t, root, "sweep",
			"---\npr: "+shape+"\n---\n\n# Sweep\n\n- [ ] 1.1 (Unit) Do it\n")
		if got := planFindings(t, root, "sweep"); contains(got, "plan.loop-invalid") {
			t.Errorf("pr: %s reported %v", shape, got)
		}
	}
}

// Absent is not wrong. A plan nobody has run a loop over carries neither key, and
// scaffolded plans never carry them — a validator firing on scc's own output is the
// one defect that teaches users to disbelieve the other seven.
func TestPlanLoopAnswersAreOptional(t *testing.T) {
	root := t.TempDir()
	writePlan(t, root, "sweep", "---\nautonomy: auto\nci: wait\n---\n\n# Sweep\n\n- [ ] 1.1 (Unit) Do it\n")
	if got := planFindings(t, root, "sweep"); contains(got, "plan.loop-invalid") {
		t.Errorf("rules = %v, want no plan.loop-invalid", got)
	}
	writePlan(t, root, "run",
		"---\nautonomy: auto\nci: wait\npr: per-plan\nworktree: per-group\nmerge: auto\n---\n\n# Run\n\n- [ ] 1.1 (Unit) Do it\n")
	if got := planFindings(t, root, "run"); len(got) != 0 {
		t.Errorf("a plan carrying every valid answer reported %v", got)
	}
}

// A spec path inside a fenced block is an example, not a reference.
func TestPlanExamplesAreNotReferences(t *testing.T) {
	root := t.TempDir()
	writePlan(t, root, "documented", "# Documented\n\n- [ ] 1.1 (Unit) Real work\n\n"+
		"```\n- specs/an-example/ — from the docs\n```\n\n<!-- - specs/another-example/ -->\n")
	if got := planFindings(t, root, "documented"); len(got) != 0 {
		t.Errorf("examples were treated as references: %v", got)
	}
}

func TestPlansValidatesEveryPlan(t *testing.T) {
	root := t.TempDir()
	writePlan(t, root, "zebra", "# Zebra\n\n- [ ] 1.1 No methodology\n")
	writePlan(t, root, "alpha", "# Alpha\n\n- [ ] 1.1 No methodology\n")
	set, err := Plans(root)
	if err != nil {
		t.Fatalf("Plans: %v", err)
	}
	if len(set.Sorted()) != 2 {
		t.Fatalf("findings = %v, want one per plan", rules(set))
	}
	if got := set.Sorted()[0].File; got != "plans/alpha.md" {
		t.Errorf("first finding is in %s, want plans/alpha.md", got)
	}
}

func TestPlansOnAnEmptyWorkspaceIsSilent(t *testing.T) {
	set, err := Plans(t.TempDir())
	if err != nil {
		t.Fatalf("Plans: %v", err)
	}
	if !set.Empty() {
		t.Errorf("an empty workspace produced findings: %v", rules(set))
	}
}

func TestPlanOnAnUnknownNameIsAnError(t *testing.T) {
	if _, err := Plan(t.TempDir(), "ghost"); err == nil {
		t.Error("validating a plan that does not exist returned no error")
	}
}
