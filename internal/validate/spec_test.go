package validate

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/assets"
	"github.com/protonspy/spec-claude-code/internal/paths"
)

// writeSpec lays out specs/<feature>/ with the three artifacts. An empty string means
// "do not write this file", so the missing-artifact cases are expressible.
func writeSpec(t *testing.T, root, feature, requirements, design, tasks string) {
	t.Helper()
	dir := paths.Spec(root, feature)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for seg, content := range map[string]string{
		paths.RequirementsSeg: requirements,
		paths.DesignSeg:       design,
		paths.TasksSeg:        tasks,
	} {
		if content == "" {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, seg), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
}

// A spec that follows the rules, used as the baseline every negative case deviates
// from by exactly one thing.
const (
	goodRequirements = `---
autonomy: auto
ci: wait
---

# Billing — requirements

Some prose that is not a requirement and must not be parsed as one.

## R1 · totals

- **R1.1** The billing engine shall compute the order total
- **R1.2** When a coupon is applied, the billing engine shall recompute the total
- **R1.3** If the coupon has expired, then the billing engine shall reject it
`
	goodDesign = `# Billing — design

## What changes

A new total calculator, serving R1.1, R1.2, and R1.3.
`
	goodTasks = `# Billing — tasks

## 1 · totals

- [ ] 1.1 (TDD) Compute the order total — R1.1
- [ ] 1.2 (Unit) Recompute on coupon application — R1.2
- [x] 1.3 (TDD) Reject an expired coupon — R1.3
`
)

func specFindings(t *testing.T, root, feature string) []string {
	t.Helper()
	set, err := Spec(root, feature)
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}
	return rules(set)
}

func TestConformingSpecHasNoFindings(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, "billing", goodRequirements, goodDesign, goodTasks)
	if got := specFindings(t, root, "billing"); len(got) != 0 {
		t.Errorf("a conforming spec produced findings: %v", got)
	}
}

// The check the whole practice exists to produce: a task with no methodology is a task
// where nobody decided how it gets built.
func TestTaskMustCarryExactlyOneMethodology(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, "billing", goodRequirements, goodDesign, `# tasks

- [ ] 1.1 Compute the order total — R1.1
- [ ] 1.2 (Unit) (TDD) Recompute — R1.2
- [ ] 1.3 (unit) Reject — R1.3
`)
	got := specFindings(t, root, "billing")
	for _, want := range []string{"task.missing-methodology", "task.multiple-methodologies"} {
		if !contains(got, want) {
			t.Errorf("rules = %v, want %s", got, want)
		}
	}
	// A lowercase annotation is a near miss, and saying so beats "no methodology".
	set, _ := Spec(root, "billing")
	var sawLoose bool
	for _, f := range set.Sorted() {
		if strings.Contains(f.Message, "(unit)") {
			sawLoose = true
		}
	}
	if !sawLoose {
		t.Error("a lowercase (unit) was not called out specifically")
	}
}

// Traceability runs both ways, and this is the direction that catches the requirement
// everyone agreed on and nobody built.
func TestOrphanRequirement(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, "billing", goodRequirements, goodDesign, `# tasks

- [ ] 1.1 (TDD) Compute the order total — R1.1
- [ ] 1.2 (Unit) Recompute on coupon application — R1.2
`)
	got := specFindings(t, root, "billing")
	if !contains(got, "spec.orphan-requirement") {
		t.Errorf("rules = %v, want spec.orphan-requirement for R1.3", got)
	}
	// The finding lands on the requirement, in requirements.md, not on the task list.
	set, _ := Spec(root, "billing")
	for _, f := range set.Sorted() {
		if f.Rule == "spec.orphan-requirement" && !strings.HasSuffix(f.File, paths.RequirementsSeg) {
			t.Errorf("orphan finding reported against %s, want requirements.md", f.File)
		}
	}
}

func TestTaskCitingAnUnknownRequirement(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, "billing", goodRequirements, goodDesign, `# tasks

- [ ] 1.1 (TDD) Compute — R1.1, R9.9
- [ ] 1.2 (Unit) Recompute — R1.2
- [ ] 1.3 (TDD) Reject — R1.3
`)
	if got := specFindings(t, root, "billing"); !contains(got, "spec.unknown-requirement") {
		t.Errorf("rules = %v, want spec.unknown-requirement", got)
	}
}

func TestTaskMustCiteARequirement(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, "billing", goodRequirements, goodDesign, `# tasks

- [ ] 1.1 (TDD) Compute the order total
- [ ] 1.2 (Unit) Recompute — R1.2
- [ ] 1.3 (TDD) Reject — R1.3
`)
	if got := specFindings(t, root, "billing"); !contains(got, "task.missing-requirement") {
		t.Errorf("rules = %v, want task.missing-requirement", got)
	}
}

func TestDuplicateIDs(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, "billing", `# requirements

- **R1.1** The engine shall compute the total
- **R1.1** The engine shall do something else
`, "# design\n\nServes R1.1.\n", `# tasks

- [ ] 1.1 (Unit) Compute — R1.1
- [ ] 1.1 (Unit) Something else — R1.1
`)
	got := specFindings(t, root, "billing")
	for _, want := range []string{"spec.duplicate-requirement", "task.duplicate-number"} {
		if !contains(got, want) {
			t.Errorf("rules = %v, want %s", got, want)
		}
	}
}

// Numbering gaps are deliberately NOT a finding. A requirement removed through a delta
// leaves a hole, which is legitimate — reporting it would be a false positive on the
// mechanism the design asks people to use.
func TestNumberingGapsAreNotFindings(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, "billing", `# requirements

- **R1.1** The engine shall compute the total
- **R1.4** The engine shall round half up
`, "# design\n\nServes R1.1 and R1.4.\n", `# tasks

- [ ] 1.1 (Unit) Compute — R1.1
- [ ] 3.7 (TDD) Round — R1.4
`)
	if got := specFindings(t, root, "billing"); len(got) != 0 {
		t.Errorf("a gap in numbering produced findings: %v", got)
	}
}

func TestEARSFindingsCarryTheRequirementID(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, "billing", `# requirements

- **R1.1** The engine computes the total
`, "# design\n\nServes R1.1.\n", "# tasks\n\n- [ ] 1.1 (Unit) Compute — R1.1\n")
	set, err := Spec(root, "billing")
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}
	if !contains(rules(set), "ears.malformed") {
		t.Fatalf("rules = %v, want ears.malformed", rules(set))
	}
	msg := set.Sorted()[0].Message
	if !strings.Contains(msg, "R1.1") || !strings.Contains(msg, "shall") {
		t.Errorf("message = %q, want the ID and what is missing", msg)
	}
}

// Delta markers are how a change to an existing spec is written. A REMOVED requirement
// has nothing left to hold to EARS and nothing for a task to implement.
func TestDeltaMarkers(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, "billing", `# requirements

- **R1.1** (MODIFIED) The engine shall compute the total in minor units
- **R1.2** (ADDED) When a refund is issued, the engine shall reverse the total
- **R1.3** (REMOVED)
- **R1.4** (modifed) The engine shall do something
`, "# design\n\nServes R1.1.\n", `# tasks

- [ ] 1.1 (TDD) Compute in minor units — R1.1
- [ ] 1.2 (Unit) Reverse on refund — R1.2
- [ ] 1.3 (Unit) Something — R1.4
`)
	got := specFindings(t, root, "billing")
	if !contains(got, "spec.delta-marker-invalid") {
		t.Errorf("rules = %v, want spec.delta-marker-invalid for the typo", got)
	}
	// A REMOVED requirement must not be reported as orphaned or as malformed EARS.
	for _, unwanted := range []string{"spec.orphan-requirement"} {
		if contains(got, unwanted) {
			t.Errorf("rules = %v, want no %s: a REMOVED requirement needs no task", got, unwanted)
		}
	}
}

func TestMissingArtifacts(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, "bare", "", "", "")
	got := specFindings(t, root, "bare")
	for _, want := range []string{"spec.missing-requirements", "spec.missing-design", "spec.missing-tasks"} {
		if !contains(got, want) {
			t.Errorf("rules = %v, want %s", got, want)
		}
	}
	// Nothing else: a spec with no requirements cannot produce traceability findings,
	// and reporting them would be findings about the wrong thing.
	if len(got) != 3 {
		t.Errorf("rules = %v, want exactly the three missing artifacts", got)
	}
}

func TestDesignMustTraceToRequirements(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, "billing", goodRequirements, "# design\n\nSome prose citing nothing.\n", goodTasks)
	if got := specFindings(t, root, "billing"); !contains(got, "spec.design-cites-nothing") {
		t.Errorf("rules = %v, want spec.design-cites-nothing", got)
	}

	root = t.TempDir()
	writeSpec(t, root, "billing", goodRequirements, "# design\n\nServes R9.9.\n", goodTasks)
	if got := specFindings(t, root, "billing"); !contains(got, "spec.unknown-requirement") {
		t.Errorf("rules = %v, want spec.unknown-requirement from the design", got)
	}
}

// The kickoff answers are validated only when present: a missing key means the run
// predates the convention, not that the user did something wrong.
func TestKickoffAnswers(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, "billing", strings.Replace(goodRequirements, "autonomy: auto", "autonomy: automatic", 1), goodDesign, goodTasks)
	if got := specFindings(t, root, "billing"); !contains(got, "spec.kickoff-invalid") {
		t.Errorf("rules = %v, want spec.kickoff-invalid", got)
	}

	root = t.TempDir()
	noFrontmatter := strings.SplitN(goodRequirements, "---\n", 3)[2]
	writeSpec(t, root, "billing", noFrontmatter, goodDesign, goodTasks)
	if got := specFindings(t, root, "billing"); len(got) != 0 {
		t.Errorf("an artifact with no frontmatter produced findings: %v", got)
	}
}

// lang is the third kickoff answer, on the same terms as the other two: absent is
// the behavior that predates the question, present is checked. The values are the
// two the rules offer — a language name nobody documented is a typo, and the run it
// would silently change the register for is the whole run.
func TestKickoffLanguage(t *testing.T) {
	with := func(value string) string {
		return strings.Replace(goodRequirements, "ci: wait\n", "ci: wait\nlang: "+value+"\n", 1)
	}

	for _, value := range []string{"en", "wenyan"} {
		root := t.TempDir()
		writeSpec(t, root, "billing", with(value), goodDesign, goodTasks)
		if got := specFindings(t, root, "billing"); len(got) != 0 {
			t.Errorf("lang: %s reported %v", value, got)
		}
	}

	root := t.TempDir()
	writeSpec(t, root, "billing", with("classical-chinese"), goodDesign, goodTasks)
	if got := specFindings(t, root, "billing"); !contains(got, "spec.kickoff-invalid") {
		t.Errorf("rules = %v, want spec.kickoff-invalid", got)
	}
}

// The kickoff answers are asked by a rule and graded here, and the two have to name
// the same values. A value this validator accepts that autonomy.md never offers is one
// no agent will ever write; a value the rule offers under another name is a rollback on
// the command the rule tells the agent to type.
func TestTheRuleOffersEveryKickoffAnswerThisAccepts(t *testing.T) {
	rule, err := assets.Content("rules/autonomy.md")
	if err != nil {
		t.Fatal(err)
	}
	for key, values := range kickoffValues {
		for value := range values {
			if !regexp.MustCompile(`\b` + regexp.QuoteMeta(value) + `\b`).MatchString(rule) {
				t.Errorf("autonomy.md never offers `%s: %s`", key, value)
			}
		}
	}
}

// The templates ship examples and instructions inside HTML comments and fenced blocks.
// A validator that read them would report findings on the file scc just generated.
func TestExamplesInCommentsAndFencesAreNotContent(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, "billing", goodRequirements+`
<!--
- **R9.9** this is an example and not a requirement
-->

`+"```\n- **R8.8** neither is this\n```\n", goodDesign, goodTasks+`
<!-- - [ ] 9.9 an example task with no methodology -->
`+"```\n- [ ] 8.8 nor this one\n```\n")
	if got := specFindings(t, root, "billing"); len(got) != 0 {
		t.Errorf("examples were treated as content: %v", got)
	}
}

func TestSpecsValidatesEveryFeatureInOrder(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, "zebra", goodRequirements, goodDesign, "# tasks\n\n- [ ] 1.1 Missing methodology — R1.1\n")
	writeSpec(t, root, "alpha", goodRequirements, goodDesign, "# tasks\n\n- [ ] 1.1 Missing methodology — R1.1\n")
	set, err := Specs(root)
	if err != nil {
		t.Fatalf("Specs: %v", err)
	}
	sorted := set.Sorted()
	if len(sorted) == 0 {
		t.Fatal("no findings across two broken specs")
	}
	if !strings.Contains(sorted[0].File, "alpha") {
		t.Errorf("first finding is in %s, want the alpha spec", sorted[0].File)
	}
}

func TestSpecsOnAnEmptyWorkspaceIsSilent(t *testing.T) {
	set, err := Specs(t.TempDir())
	if err != nil {
		t.Fatalf("Specs: %v", err)
	}
	if !set.Empty() {
		t.Errorf("an empty workspace produced findings: %v", rules(set))
	}
}

func TestSpecOnAnUnknownFeatureIsAnError(t *testing.T) {
	if _, err := Spec(t.TempDir(), "ghost"); err == nil {
		t.Error("validating a spec that does not exist returned no error")
	}
}

func contains(haystack []string, want string) bool {
	return count(haystack, want) > 0
}

func count(haystack []string, want string) int {
	n := 0
	for _, s := range haystack {
		if s == want {
			n++
		}
	}
	return n
}

// The delivery record is graded when present and silent when absent — the same terms
// as every other frontmatter answer, and what lets every spec written before it
// existed keep passing.
func TestDeliveryRecordIsCheckedWhenPresent(t *testing.T) {
	for _, tc := range []struct {
		name string
		fm   string
		want string
	}{
		{"absent", "autonomy: auto\nci: wait", ""},
		{"a full record", "autonomy: auto\nbranch: feat/billing\npr: 28\ndelivery: in-review", ""},
		{"a branch on its own", "autonomy: auto\nbranch: feat/billing\ndelivery: in-progress", ""},
		{"a state outside the vocabulary", "branch: feat/x\ndelivery: shipping-soon", "spec.delivery-invalid"},
		{"a PR that is not a number", "branch: feat/x\npr: later\ndelivery: in-review", "spec.pr-invalid"},
		{"a PR that is not positive", "branch: feat/x\npr: 0\ndelivery: in-review", "spec.pr-invalid"},
		{"a branch with whitespace in it", "branch: two words\ndelivery: in-progress", "spec.branch-invalid"},
		{"a branch with nothing said about it", "branch: feat/x", "spec.delivery-unstated"},
		{"a PR with nothing said about it", "pr: 28", "spec.delivery-unstated"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			reqs := "---\n" + tc.fm + "\n---\n" +
				strings.SplitN(goodRequirements, "---\n", 3)[2]
			writeSpec(t, root, "billing", reqs, goodDesign, goodTasks)
			got := specFindings(t, root, "billing")
			if tc.want == "" {
				if len(got) != 0 {
					t.Errorf("findings on a legitimate record: %v", got)
				}
				return
			}
			if !contains(got, tc.want) {
				t.Errorf("findings = %v, want %s", got, tc.want)
			}
		})
	}
}
