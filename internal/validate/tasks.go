package validate

import (
	"github.com/protonspy/spec-claude-code/internal/artifact"
	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/mdscan"
)

// The task grammar governs every task line — whether it sits in a spec's tasks.md or
// in a plan's checklist — and it lives in internal/artifact, next to the reader that
// navigates by it. One grammar, two consumers: the reader that resolves `1.2` to a
// region, and this, which turns what the line failed to say into findings.
//
//   - [ ] 1.1 (Unit) Parse the manifest file — R1.2, R1.4
//
// The split is deliberate. The parser states facts about the line; deciding which
// fact is a defect is a validator's job, and keeping that decision here is what lets
// `scc map` read a malformed artifact instead of refusing it. A reader that enforced
// the grammar would be unusable on exactly the files a user most needs to inspect.
func parseTasks(set *finding.Set, file string, doc *mdscan.Document, citations bool) []artifact.Task {
	tasks := artifact.ParseTasks(doc)
	seen := map[string]int{}

	for _, t := range tasks {
		if t.Number == "" {
			set.Addf(file, t.Line, "task.missing-number",
				"a task opens with its number: `- [ ] 1.1 (Unit) …`")
		} else {
			if prior, dup := seen[t.Number]; dup {
				set.Addf(file, t.Line, "task.duplicate-number",
					"task %s is already used on line %d; a number has to identify one task", t.Number, prior)
			}
			seen[t.Number] = t.Line
		}

		switch t.Methodologies {
		case 0:
			// The one finding this whole practice exists to produce. A task with no
			// methodology is a task where nobody decided.
			if t.Loose != "" {
				set.Addf(file, t.Line, "task.missing-methodology",
					"%s is not a methodology: annotate the task `(Unit)` or `(TDD)`", t.Loose)
			} else {
				set.Addf(file, t.Line, "task.missing-methodology",
					"every task carries `(Unit)` or `(TDD)`: without it, nobody decided how this gets built")
			}
		case 1:
		default:
			set.Addf(file, t.Line, "task.multiple-methodologies",
				"a task is built one way: found %d methodology annotations", t.Methodologies)
		}

		if t.Text == "" {
			set.Addf(file, t.Line, "task.no-description", "the task says nothing about what to do")
		}
		if citations && (!t.HasCitation || len(t.Requirements) == 0) {
			set.Addf(file, t.Line, "task.missing-requirement",
				"a task in a spec cites the requirements it satisfies: `… %s R1.1, R1.2`",
				artifact.CitationSeparator)
		}
	}
	return tasks
}
