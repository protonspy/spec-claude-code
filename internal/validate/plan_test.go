package validate

import (
	"os"
	"strings"
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

// plan wraps a body in the sections the contract requires, so a test about one rule
// states that rule and not the whole contract. Anything the body opens with a `##`
// simply adds a section to the ones already here.
func plan(frontmatter, body string) string {
	return frontmatter + `# Sweep

What this work is, in one sentence.

## Why

Because the old path cannot be extended.

## Tasks

` + body + `

## Done when

- ` + "`scc validate`" + ` is clean
`
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

Replace the checkout path, one leaf at a time.

## Why

The old path cannot be extended without a rewrite of the totals engine.

## Paths

- `+"`internal/checkout/`"+`

## References

- `+"`specs/cart-totals/`"+` — the totals engine

## Out of scope

- the payment provider itself

## Tasks

- [ ] 1.1 (Unit) Rename the legacy endpoint
- [ ] 1.2 (TDD) Migrate the rounding helper
  _Depends 1.1_
  _Priority 2_

## Done when

- the legacy endpoint is gone and the suite is green
`)
	if got := planFindings(t, root, "checkout-revamp"); len(got) != 0 {
		t.Errorf("a conforming plan produced findings: %v", got)
	}
}

// The contract is closed, and that is the only thing capping a plan's size. The
// heading it was written against gets its own sentence, because "unknown section"
// would read as a typo when the answer is that the content belongs elsewhere.
func TestPlanSectionsAreClosed(t *testing.T) {
	root := t.TempDir()
	writePlan(t, root, "sweep", plan("", "- [ ] 1.1 (Unit) Do it\n\n## Notes\n\nA long essay begins here.\n"))
	got := planFindings(t, root, "sweep")
	if !contains(got, "plan.unknown-section") {
		t.Fatalf("rules = %v, want plan.unknown-section", got)
	}
	set, _ := Plan(root, "sweep")
	for _, f := range set.Sorted() {
		if f.Rule == "plan.unknown-section" && !strings.Contains(f.Message, paths.ADRSeg) {
			t.Errorf("the Notes message does not say where the content goes: %q", f.Message)
		}
	}

	writePlan(t, root, "other", plan("", "- [ ] 1.1 (Unit) Do it\n\n## Appendix\n\nMore.\n"))
	if got := planFindings(t, root, "other"); !contains(got, "plan.unknown-section") {
		t.Errorf("rules = %v, want plan.unknown-section for any other heading", got)
	}
}

func TestPlanRequiresItsThreeSections(t *testing.T) {
	root := t.TempDir()
	writePlan(t, root, "bare", "# Bare\n\nWhat this is.\n\n## Tasks\n\n- [ ] 1.1 (Unit) Do it\n")
	got := planFindings(t, root, "bare")
	if n := count(got, "plan.missing-section"); n != 2 {
		t.Errorf("rules = %v, want Why and Done when reported missing, got %d", got, n)
	}
}

func TestPlanNeedsATitleAndAShortDescription(t *testing.T) {
	root := t.TempDir()
	writePlan(t, root, "untitled", "## Why\n\nBecause.\n\n## Tasks\n\n- [ ] 1.1 (Unit) Do it\n\n## Done when\n\n- done\n")
	if got := planFindings(t, root, "untitled"); !contains(got, "plan.missing-title") {
		t.Errorf("rules = %v, want plan.missing-title", got)
	}

	writePlan(t, root, "silent", "# Silent\n\n## Why\n\nBecause.\n\n## Tasks\n\n- [ ] 1.1 (Unit) Do it\n\n## Done when\n\n- done\n")
	if got := planFindings(t, root, "silent"); !contains(got, "plan.missing-description") {
		t.Errorf("rules = %v, want plan.missing-description", got)
	}

	essay := strings.Repeat("A line of the essay this contract exists to prevent.\n", maxDescriptionLines+1)
	writePlan(t, root, "essay", "# Essay\n\n"+essay+"\n## Why\n\nBecause.\n\n## Tasks\n\n- [ ] 1.1 (Unit) Do it\n\n## Done when\n\n- done\n")
	if got := planFindings(t, root, "essay"); !contains(got, "plan.description-too-long") {
		t.Errorf("rules = %v, want plan.description-too-long", got)
	}
}

// One source of truth per item. An item that both carries a checkbox and references a
// spec keeps two records of one fact, and the copy is the one that goes stale.
func TestItemCannotBothCheckAndReference(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, "cart-totals", goodRequirements, goodDesign, goodTasks)
	writePlan(t, root, "checkout-revamp", plan("", "- [ ] 1.1 (Unit) Build `"+"specs/cart-totals/"+"`\n"))
	if got := planFindings(t, root, "checkout-revamp"); !contains(got, "plan.item-has-two-records") {
		t.Errorf("rules = %v, want plan.item-has-two-records", got)
	}
}

// D1 moved the spec references out of `## Decomposition` and into `## References`.
// The parser recognizes a leaf by the citation rather than by the heading above it,
// so the check that a referenced spec exists moved with them and nothing else did.
func TestPlanReferencesMustResolve(t *testing.T) {
	root := t.TempDir()
	writePlan(t, root, "checkout-revamp",
		plan("", "- [ ] 1.1 (Unit) Do it\n\n## References\n\n- specs/does-not-exist/ — nothing is here\n"))
	if got := planFindings(t, root, "checkout-revamp"); !contains(got, "plan.unknown-spec") {
		t.Errorf("rules = %v, want plan.unknown-spec", got)
	}
	if got := planFindings(t, root, "checkout-revamp"); contains(got, "plan.unknown-section") {
		t.Errorf("`## References` is part of the contract: %v", got)
	}
}

// A plan's tasks carry no requirement citations — a plan has no requirements — but the
// rest of the grammar is identical, because the methodology belongs to the task and not
// to the vehicle that carried it.
func TestPlanTasksUseTheSameGrammarWithoutCitations(t *testing.T) {
	root := t.TempDir()
	writePlan(t, root, "sweep", plan("", `- [ ] 1.1 (Unit) A task with no citation, which is fine here
- [ ] 1.2 A task with no methodology, which is not
`))
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
	writePlan(t, root, "hollow", plan("", "Some prose and no items at all.\n"))
	if got := planFindings(t, root, "hollow"); !contains(got, "plan.empty") {
		t.Errorf("rules = %v, want plan.empty", got)
	}
}

func TestPlanKickoffAnswers(t *testing.T) {
	root := t.TempDir()
	writePlan(t, root, "sweep", plan("---\nci: eventually\n---\n\n", "- [ ] 1.1 (Unit) Do it\n"))
	if got := planFindings(t, root, "sweep"); !contains(got, "plan.kickoff-invalid") {
		t.Errorf("rules = %v, want plan.kickoff-invalid", got)
	}

	// A plan is the vehicle a whole run is driven from, so it carries the language
	// answer on the same terms a spec does — one key, checked when present.
	root = t.TempDir()
	writePlan(t, root, "wide", plan("---\nautonomy: auto\nci: wait\nlang: wenyan\n---\n\n", "- [ ] 1.1 (Unit) Do it\n"))
	if got := planFindings(t, root, "wide"); len(got) != 0 {
		t.Errorf("a plan carrying every kickoff answer reported %v", got)
	}

	root = t.TempDir()
	writePlan(t, root, "narrow", plan("---\nlang: pt-BR\n---\n\n", "- [ ] 1.1 (Unit) Do it\n"))
	if got := planFindings(t, root, "narrow"); !contains(got, "plan.kickoff-invalid") {
		t.Errorf("rules = %v, want plan.kickoff-invalid for an undocumented language", got)
	}
}

// The answers `plan-run` writes back before it starts a loop. A wrong value is worth
// a finding because the skill writes these and every later session reads them —
// `merge: whenever` would quietly decide how the rest of the plan gets built.
func TestPlanLoopAnswers(t *testing.T) {
	root := t.TempDir()
	writePlan(t, root, "sweep",
		plan("---\nmerge: whenever\npr: sometimes\n---\n\n", "- [ ] 1.1 (Unit) Do it\n"))
	got := planFindings(t, root, "sweep")
	if n := count(got, "plan.loop-invalid"); n != 2 {
		t.Errorf("rules = %v, want two plan.loop-invalid findings, got %d", got, n)
	}
}

// `pr` decides whether the loop opens a PR per group or one at the end, which is the
// difference between the review subagents running once and running once per group.
// Both spellings have to survive, and a resumed session reads this key to know
// whether `main` or the plan's branch is where its position lives.
func TestPlanPRShapeAcceptsBothLoops(t *testing.T) {
	root := t.TempDir()
	for _, shape := range []string{"per-group", "per-plan"} {
		writePlan(t, root, "sweep", plan("---\npr: "+shape+"\n---\n\n", "- [ ] 1.1 (Unit) Do it\n"))
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
	writePlan(t, root, "sweep", plan("---\nautonomy: auto\nci: wait\n---\n\n", "- [ ] 1.1 (Unit) Do it\n"))
	if got := planFindings(t, root, "sweep"); contains(got, "plan.loop-invalid") {
		t.Errorf("rules = %v, want no plan.loop-invalid", got)
	}
	writePlan(t, root, "run",
		plan("---\nautonomy: auto\nci: wait\npr: per-plan\nmerge: auto\n---\n\n",
			"- [ ] 1.1 (Unit) Do it\n"))
	if got := planFindings(t, root, "run"); len(got) != 0 {
		t.Errorf("a plan carrying every valid answer reported %v", got)
	}
}

// A spec path inside a fenced block is an example, not a reference.
func TestPlanExamplesAreNotReferences(t *testing.T) {
	root := t.TempDir()
	writePlan(t, root, "documented", plan("", "- [ ] 1.1 (Unit) Real work\n\n"+
		"```\n- specs/an-example/ — from the docs\n```\n\n<!-- - specs/another-example/ -->\n"))
	if got := planFindings(t, root, "documented"); len(got) != 0 {
		t.Errorf("examples were treated as references: %v", got)
	}
}

func TestPlansValidatesEveryPlan(t *testing.T) {
	root := t.TempDir()
	writePlan(t, root, "zebra", plan("", "- [ ] 1.1 No methodology\n"))
	writePlan(t, root, "alpha", plan("", "- [ ] 1.1 No methodology\n"))
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
