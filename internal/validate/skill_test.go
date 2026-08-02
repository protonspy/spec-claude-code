package validate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/paths"
)

// writeSkill lays out .claude/skills/<name>/SKILL.md with the given content.
func writeSkill(t *testing.T, root, name, content string) string {
	t.Helper()
	dir := filepath.Join(paths.Claude.Skills(root), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return dir
}

func skills(t *testing.T, root string) *finding.Set {
	t.Helper()
	set, err := Skills(root)
	if err != nil {
		t.Fatalf("Skills: %v", err)
	}
	return set
}

// rules returns the rule slugs of every finding, which is what a CI job filters on
// and therefore what these tests assert against rather than on prose.
func rules(set *finding.Set) []string {
	var out []string
	for _, f := range set.Sorted() {
		out = append(out, f.Rule)
	}
	return out
}

func hasRule(set *finding.Set, rule string) bool {
	for _, r := range rules(set) {
		if r == rule {
			return true
		}
	}
	return false
}

func TestValidSkillHasNoFindings(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "pdf-processing", `---
name: pdf-processing
description: Extracts text and tables from PDF files. Use when working with PDFs.
license: Apache-2.0
metadata:
  author: example-org
  version: "1.0"
---

# PDF processing

Step-by-step instructions here.
`)
	set := skills(t, root)
	if !set.Empty() {
		t.Errorf("a spec-conforming skill produced findings: %v", rules(set))
	}
}

// A workspace with no skills is not a workspace with findings.
func TestNoSkillsDirectoryIsSilent(t *testing.T) {
	set := skills(t, t.TempDir())
	if !set.Empty() {
		t.Errorf("an empty workspace produced findings: %v", rules(set))
	}
	if set.ExitCode() != finding.ExitOK {
		t.Errorf("exit = %d, want %d", set.ExitCode(), finding.ExitOK)
	}
}

// The check that matters most: a mismatched name means the skill does not load at
// all, in any tool that reads this format.
func TestNameMustMatchTheDirectory(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "pdf-processing", `---
name: pdf-tools
description: Does things with PDFs. Use when handling PDFs.
---

Body.
`)
	set := skills(t, root)
	if !hasRule(set, "skill.name-mismatch") {
		t.Errorf("rules = %v, want skill.name-mismatch", rules(set))
	}
	// The message has to name both halves, or the user cannot tell which to change.
	msg := set.Sorted()[0].Message
	if !strings.Contains(msg, "pdf-tools") || !strings.Contains(msg, "pdf-processing") {
		t.Errorf("message = %q, want both names", msg)
	}
	if set.ExitCode() != finding.ExitFindings {
		t.Errorf("exit = %d, want %d", set.ExitCode(), finding.ExitFindings)
	}
}

func TestNameCharsetRules(t *testing.T) {
	cases := map[string]bool{ // name -> should be reported invalid
		"pdf-processing":  false,
		"data-analysis2":  false,
		"a":               false,
		"PDF-Processing":  true, // uppercase
		"-pdf":            true, // leading hyphen
		"pdf-":            true, // trailing hyphen
		"pdf--processing": true, // consecutive hyphens
		"pdf_processing":  true, // underscore
		"pdf processing":  true, // space
	}
	for name, wantInvalid := range cases {
		root := t.TempDir()
		// The directory carries the same name, so name-mismatch cannot mask the
		// charset finding this case is about.
		writeSkill(t, root, name, "---\nname: "+name+"\ndescription: A skill. Use it when testing.\n---\n\nBody.\n")
		set := skills(t, root)
		if got := hasRule(set, "skill.name-invalid"); got != wantInvalid {
			t.Errorf("%q: name-invalid = %v, want %v (rules: %v)", name, got, wantInvalid, rules(set))
		}
	}
}

func TestRequiredFieldsAreRequired(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "no-name", "---\ndescription: A skill. Use it when testing.\n---\n\nBody.\n")
	if !hasRule(skills(t, root), "skill.missing-name") {
		t.Error("a skill with no name produced no missing-name finding")
	}

	root = t.TempDir()
	writeSkill(t, root, "no-description", "---\nname: no-description\n---\n\nBody.\n")
	if !hasRule(skills(t, root), "skill.missing-description") {
		t.Error("a skill with no description produced no missing-description finding")
	}

	root = t.TempDir()
	writeSkill(t, root, "blank-description", "---\nname: blank-description\ndescription: \"  \"\n---\n\nBody.\n")
	if !hasRule(skills(t, root), "skill.missing-description") {
		t.Error("a whitespace-only description was accepted")
	}
}

func TestLengthLimitsComeFromTheSpec(t *testing.T) {
	root := t.TempDir()
	long := strings.Repeat("a", skillDescriptionMax+1)
	writeSkill(t, root, "wordy", "---\nname: wordy\ndescription: "+long+"\n---\n\nBody.\n")
	set := skills(t, root)
	if !hasRule(set, "skill.description-too-long") {
		t.Errorf("rules = %v, want skill.description-too-long", rules(set))
	}
	if !strings.Contains(set.Sorted()[0].Message, "1024") {
		t.Errorf("message = %q, want it to quote the spec's limit", set.Sorted()[0].Message)
	}

	// One character under is fine: an off-by-one here is a false positive on a valid
	// skill, which costs more than a miss.
	root = t.TempDir()
	writeSkill(t, root, "wordy", "---\nname: wordy\ndescription: "+strings.Repeat("a", skillDescriptionMax)+"\n---\n\nBody.\n")
	if !skills(t, root).Empty() {
		t.Errorf("a description exactly at the limit was reported: %v", rules(skills(t, root)))
	}
}

func TestCompatibilityLimit(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "picky", "---\nname: picky\ndescription: A skill. Use when testing.\ncompatibility: "+
		strings.Repeat("x", skillCompatibilityMax+1)+"\n---\n\nBody.\n")
	if !hasRule(skills(t, root), "skill.compatibility-too-long") {
		t.Error("an over-long compatibility field was accepted")
	}
}

// The body budget is a recommendation in the spec and is reported as one — but it is
// still deterministic, so it is a finding rather than silence.
func TestBodyBudget(t *testing.T) {
	root := t.TempDir()
	head := "---\nname: verbose\ndescription: A skill. Use when testing.\n---\n"
	writeSkill(t, root, "verbose", head+strings.Repeat("line\n", skillBodyMaxLines+2))
	set := skills(t, root)
	if !hasRule(set, "skill.body-too-long") {
		t.Errorf("rules = %v, want skill.body-too-long", rules(set))
	}

	root = t.TempDir()
	writeSkill(t, root, "verbose", head+strings.Repeat("line\n", skillBodyMaxLines-1))
	if !skills(t, root).Empty() {
		t.Error("a body under the budget was reported")
	}
}

func TestMissingSkillMD(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(paths.Claude.Skills(root), "empty-dir"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	set := skills(t, root)
	if !hasRule(set, "skill.missing-skill-md") {
		t.Errorf("rules = %v, want skill.missing-skill-md", rules(set))
	}
}

func TestMissingFrontmatter(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "bare", "# Just a heading\n\nNo frontmatter at all.\n")
	if !hasRule(skills(t, root), "skill.missing-frontmatter") {
		t.Error("a SKILL.md with no frontmatter was accepted")
	}
}

// Frontmatter scc cannot read produces one finding saying so, not a pile of findings
// about fields it failed to extract.
func TestUnreadableFrontmatterReportsOnce(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "weird", "---\ntools:\n  - Read\n---\n\nBody.\n")
	set := skills(t, root)
	if got := rules(set); len(got) != 1 || got[0] != "skill.frontmatter-unreadable" {
		t.Errorf("rules = %v, want exactly [skill.frontmatter-unreadable]", got)
	}
}

func TestReferenceChecks(t *testing.T) {
	root := t.TempDir()
	dir := writeSkill(t, root, "refs", `---
name: refs
description: A skill with references. Use when testing.
---

See [the reference](references/REFERENCE.md) for details.
And [the deep one](references/nested/deeper/DETAIL.md).
And [the missing one](references/GONE.md).
Also [an external link](https://example.com/page) and [an anchor](#section).
`)
	if err := os.MkdirAll(filepath.Join(dir, "references", "nested", "deeper"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for _, f := range []string{
		filepath.Join(dir, "references", "REFERENCE.md"),
		filepath.Join(dir, "references", "nested", "deeper", "DETAIL.md"),
	} {
		if err := os.WriteFile(f, []byte("x\n"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	set := skills(t, root)
	if !hasRule(set, "skill.reference-too-deep") {
		t.Errorf("rules = %v, want skill.reference-too-deep", rules(set))
	}
	if !hasRule(set, "skill.broken-reference") {
		t.Errorf("rules = %v, want skill.broken-reference", rules(set))
	}
	// A URL and an anchor are things this validator cannot resolve, and a check that
	// cannot resolve its subject says nothing.
	if n := len(rules(set)); n != 2 {
		t.Errorf("rules = %v, want exactly the two resolvable problems", rules(set))
	}
}

// A reference inside a fenced block is an example, not a reference. This is the
// scanner's job, and it is worth asserting from here too: it is the difference
// between a validator the user trusts and one they mute.
func TestExamplesInFencesAreNotReferences(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "documented", "---\nname: documented\ndescription: A skill. Use when testing.\n---\n\n"+
		"```markdown\nSee [the reference](references/DOES-NOT-EXIST.md)\n```\n")
	if set := skills(t, root); !set.Empty() {
		t.Errorf("an example inside a fence was treated as a reference: %v", rules(set))
	}
}

// Findings arrive sorted by file, so two skills report in a stable order.
func TestFindingsAreOrderedAcrossSkills(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "zebra", "---\nname: wrong\ndescription: A skill. Use when testing.\n---\n\nBody.\n")
	writeSkill(t, root, "alpha", "---\nname: wrong\ndescription: A skill. Use when testing.\n---\n\nBody.\n")
	got := skills(t, root).Sorted()
	if len(got) != 2 {
		t.Fatalf("findings = %d, want 2", len(got))
	}
	if !strings.Contains(got[0].File, "alpha") {
		t.Errorf("first finding is %s, want the alpha skill", got[0].File)
	}
}

// The spec's limits are in characters. Counting bytes agrees only for ASCII, and would
// fail a description that is inside its 1024 characters but past 1024 bytes — the
// false positive on a valid skill that TestLengthLimitsComeFromTheSpec guards for ASCII.
func TestLengthLimitsCountCharactersNotBytes(t *testing.T) {
	// Two bytes per rune, so a description exactly at the limit is twice over in bytes.
	desc := strings.Repeat("é", skillDescriptionMax)
	if len(desc) <= skillDescriptionMax {
		t.Fatalf("test premise broken: %d bytes is not over the %d limit", len(desc), skillDescriptionMax)
	}
	root := t.TempDir()
	writeSkill(t, root, "accented", "---\nname: accented\ndescription: "+desc+"\n---\n\nBody.\n")
	if hasRule(skills(t, root), "skill.description-too-long") {
		t.Errorf("a description of exactly %d characters was reported as too long", skillDescriptionMax)
	}

	// And one character over is still caught, so the fix did not simply disable the check.
	root = t.TempDir()
	writeSkill(t, root, "accented", "---\nname: accented\ndescription: "+desc+"é\n---\n\nBody.\n")
	if !hasRule(skills(t, root), "skill.description-too-long") {
		t.Error("a description one character over the limit was accepted")
	}
}

func TestCompatibilityLimitCountsCharacters(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "picky", "---\nname: picky\ndescription: A skill. Use when testing.\ncompatibility: "+
		strings.Repeat("é", skillCompatibilityMax)+"\n---\n\nBody.\n")
	if hasRule(skills(t, root), "skill.compatibility-too-long") {
		t.Errorf("a compatibility field of exactly %d characters was reported as too long", skillCompatibilityMax)
	}
}
