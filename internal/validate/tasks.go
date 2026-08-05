package validate

import (
	"strings"

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
	checkFlags(set, file, tasks)
	return tasks
}

// The values that would make `_Status_` a second record of what the box already
// says. They are called out separately from any other bad value because the mistake
// is a different one: not a typo, but two places recording one fact — the defect
// `plan.item-has-two-records` exists to catch, arriving by another door.
var boxStates = map[string]bool{
	"open": true, "todo": true, "done": true, "completed": true,
	"complete": true, "in-progress": true, "wip": true,
}

// checkFlags turns what the parser found about a task's annotation lines into
// findings, and then checks the graph those annotations describe.
//
// The graph checks are the ones that earn their place: a dependency on a task that
// does not exist, or on one that was struck out, is a deadlock that `--next` cannot
// explain and a loop cannot escape — it simply reports nothing to do while open work
// remains.
func checkFlags(set *finding.Set, file string, tasks []artifact.Task) {
	byNumber := map[string]artifact.Task{}
	for _, t := range tasks {
		if _, seen := byNumber[t.Number]; !seen && t.Number != "" {
			byNumber[t.Number] = t
		}
	}

	for _, t := range tasks {
		for _, f := range t.UnknownFlags {
			set.Addf(file, f.Line, "task.unknown-flag",
				"`_%s_` is not one of the four flags a task carries: %s",
				f.Name, strings.Join(artifact.FlagNames, ", "))
		}
		seen := map[string]int{}
		for _, f := range t.Flags {
			if prior, dup := seen[f.Name]; dup {
				set.Addf(file, f.Line, "task.duplicate-flag",
					"`_%s_` is already on line %d; a task carries at most one of each", f.Name, prior)
				continue
			}
			seen[f.Name] = f.Line
		}

		if t.BadPriority != "" {
			set.Addf(file, t.Line, "task.invalid-priority",
				"`_Priority %s_` is not a whole number 1 or greater — lower is more urgent", t.BadPriority)
		}
		if t.BadStatus != "" {
			if boxStates[strings.ToLower(strings.TrimSpace(t.BadStatus))] {
				set.Addf(file, t.Line, "task.status-duplicates-box",
					"`_Status %s_` restates the checkbox; the box is the state, and `_Status_` only ever says `%s`",
					t.BadStatus, artifact.StatusRemoved)
			} else {
				set.Addf(file, t.Line, "task.invalid-status",
					"`_Status %s_` is not a status: the only one is `%s`", t.BadStatus, artifact.StatusRemoved)
			}
		}
		if t.Removed() {
			if t.Reason == "" {
				set.Addf(file, t.Line, "task.removed-without-reason",
					"a removed task carries `_Reason …_`: the reason is the whole record of why the work went away")
			}
			if t.Checked {
				set.Addf(file, t.Line, "task.removed-but-checked",
					"task %s is both removed and ticked; it was either done or it was not", t.Number)
			}
		}

		for _, d := range t.Depends {
			switch dep, ok := byNumber[d]; {
			case d == t.Number:
				set.Addf(file, t.Line, "task.self-dependency",
					"task %s depends on itself, so it can never start", t.Number)
			case !ok:
				set.Addf(file, t.Line, "task.unknown-dependency",
					"task %s depends on %s, which this file does not have", t.Number, d)
			case dep.Removed():
				set.Addf(file, t.Line, "task.depends-on-removed",
					"task %s depends on %s, which was removed — it will never be ticked, so %s can never start",
					t.Number, d, t.Number)
			}
		}
	}

	for _, cycle := range artifact.Cycles(tasks) {
		at := 0
		if t, ok := byNumber[cycle[0]]; ok {
			at = t.Line
		}
		set.Addf(file, at, "task.dependency-cycle",
			"these tasks wait on each other and none of them can start: %s",
			strings.Join(append(append([]string{}, cycle...), cycle[0]), " → "))
	}
}
