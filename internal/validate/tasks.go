package validate

import (
	"regexp"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/mdscan"
)

// The task grammar, in one place, because one grammar governs every task line —
// whether it sits in a spec's tasks.md or in a plan's checklist. The methodology is a
// property of the task, not of the vehicle that carried it, so the findings a user
// sees are identical in both and share one `task.` slug namespace.
//
//   - [ ] 1.1 (Unit) Parse the manifest file — R1.2, R1.4
var (
	taskNumberRe      = regexp.MustCompile(`^(\d+(?:\.\d+)*)\s+(.*)$`)
	methodologyRe     = regexp.MustCompile(`\((Unit|TDD)\)`)
	looseMethodology  = regexp.MustCompile(`(?i)\((unit|tdd|test[- ]?first|none)\)`)
	requirementIDRe   = regexp.MustCompile(`\bR\d+(?:\.\d+)+\b`)
	citationSeparator = "—" // an em dash, per the grammar in .claude/rules/tasks.md
)

// task is one parsed task line.
type task struct {
	Line         int
	Checked      bool
	Number       string
	Methodology  string // "Unit" or "TDD"
	Text         string
	Requirements []string
}

// parseTasks reads every checkbox in doc against the task grammar and reports what is
// missing. citations decides whether a task must cite requirements: in a spec's
// tasks.md it must, because that citation is what makes traceability checkable; a
// plan has no requirements to cite.
//
// Only checkboxes are considered, and mdscan has already excluded the ones inside
// fenced blocks and HTML comments — which is what keeps a rule file that documents
// the grammar, or a template that shows an example, from producing findings.
func parseTasks(set *finding.Set, file string, doc *mdscan.Document, citations bool) []task {
	var tasks []task
	seen := map[string]int{}

	for _, box := range doc.Checkboxes {
		t := task{Line: box.Line, Checked: box.Checked}
		rest := box.Text

		if m := taskNumberRe.FindStringSubmatch(rest); m != nil {
			t.Number, rest = m[1], m[2]
			if prior, dup := seen[t.Number]; dup {
				set.Addf(file, box.Line, "task.duplicate-number",
					"task %s is already used on line %d; a number has to identify one task", t.Number, prior)
			}
			seen[t.Number] = box.Line
		} else {
			set.Addf(file, box.Line, "task.missing-number",
				"a task opens with its number: `- [ ] 1.1 (Unit) …`")
		}

		switch found := methodologyRe.FindAllStringSubmatch(rest, -1); len(found) {
		case 0:
			// The one finding this whole practice exists to produce. A task with no
			// methodology is a task where nobody decided.
			if loose := looseMethodology.FindString(rest); loose != "" {
				set.Addf(file, box.Line, "task.missing-methodology",
					"%s is not a methodology: annotate the task `(Unit)` or `(TDD)`", loose)
			} else {
				set.Addf(file, box.Line, "task.missing-methodology",
					"every task carries `(Unit)` or `(TDD)`: without it, nobody decided how this gets built")
			}
		case 1:
			t.Methodology = found[0][1]
		default:
			set.Addf(file, box.Line, "task.multiple-methodologies",
				"a task is built one way: found %d methodology annotations", len(found))
		}

		text, tail, hasTail := strings.Cut(rest, citationSeparator)
		t.Text = strings.TrimSpace(methodologyRe.ReplaceAllString(text, ""))
		t.Requirements = requirementIDRe.FindAllString(tail, -1)

		if t.Text == "" {
			set.Addf(file, box.Line, "task.no-description", "the task says nothing about what to do")
		}
		if citations {
			switch {
			case !hasTail, len(t.Requirements) == 0:
				set.Addf(file, box.Line, "task.missing-requirement",
					"a task in a spec cites the requirements it satisfies: `… %s R1.1, R1.2`", citationSeparator)
			}
		}
		tasks = append(tasks, t)
	}
	return tasks
}
