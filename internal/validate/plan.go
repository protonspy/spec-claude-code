package validate

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/artifact"
	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/mdscan"
	"github.com/protonspy/spec-claude-code/internal/paths"
)

// specReferenceRe matches a reference to a spec from inside a plan: `specs/<feature>/`,
// with or without backticks around it. Shared with the reader in internal/artifact,
// which resolves the same reference to a decomposition leaf.
var specReferenceRe = artifact.SpecRefRe

// Plans validates every plan under plans/.
func Plans(root string) (*finding.Set, error) {
	set := &finding.Set{}
	entries, err := os.ReadDir(paths.Plans(root))
	if err != nil {
		if os.IsNotExist(err) {
			return set, nil
		}
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".md"))
	}
	sort.Strings(names)
	for _, name := range names {
		one, err := Plan(root, name)
		if err != nil {
			return nil, err
		}
		set.Extend(one)
	}
	return set, nil
}

// Plan validates one plan.
//
// Three checks, and the third is the one that matters: **one source of truth per
// item.** An item either carries a checkbox — it is a task, and the box is its state —
// or it references a spec, whose state is read from that spec. An item that does both
// keeps two records of one fact, and the copy is the one that goes stale.
func Plan(root, name string) (*finding.Set, error) {
	set := &finding.Set{}
	planPath := paths.Plan(root, name)
	if !isFile(planPath) {
		return nil, fmt.Errorf("no plan %q under %s", name, paths.PlansSeg)
	}
	file := rel(root, planPath)

	doc, err := read(root, planPath)
	if err != nil {
		if doc == nil {
			return nil, err
		}
		set.Addf(file, 1, "plan.frontmatter-unreadable", "%v", err)
		return set, nil
	}
	checkKickoffAs(set, file, doc.Frontmatter, "plan.kickoff-invalid")
	checkLoopAnswers(set, file, doc.Frontmatter)

	// A plan's tasks carry no requirement citations: a plan has no requirements. The
	// rest of the grammar is identical, because the methodology is a property of the
	// task rather than of the vehicle that carried it.
	tasks := parseTasks(set, file, doc, false)

	checkboxLines := map[int]bool{}
	for _, t := range tasks {
		checkboxLines[t.Line] = true
	}

	referenced := 0
	for i, line := range doc.Body {
		matches := specReferenceRe.FindAllStringSubmatch(line, -1)
		if len(matches) == 0 {
			continue
		}
		referenced += len(matches)
		num := i + 1
		if checkboxLines[num] {
			set.Addf(file, num, "plan.item-has-two-records",
				"this item both carries a checkbox and references a spec; the state has to live in exactly one place")
		}
		for _, m := range matches {
			if !isDir(paths.Spec(root, m[1])) {
				set.Addf(file, num, "plan.unknown-spec",
					"%s/%s/ does not exist", paths.SpecsSeg, m[1])
			}
		}
	}

	if len(tasks) == 0 && referenced == 0 {
		set.Addf(file, 0, "plan.empty",
			"a plan holds a checklist of tasks, references to the specs it decomposes into, or both — this has neither")
	}
	return set, nil
}

// The answers the `plan-run` skill asks for before it starts a loop, and writes back
// into the plan so a resumed session reads them instead of asking a second time.
//
// Plan-only, and validated on exactly the terms the kickoff answers are: only when
// present, because a plan nobody has ever run a loop over is not a plan with a defect.
// They are checked at all for one reason — the skill writes them and every later
// session reads them, so `worktree: yes` instead of `per-group` would silently decide
// how the next ten groups get built.
var loopValues = map[string]map[string]bool{
	"worktree": {"per-group": true, "in-place": true},
	"merge":    {"auto": true, "manual": true},
	"pr":       {"per-group": true, "per-plan": true},
}

func checkLoopAnswers(set *finding.Set, file string, fm mdscan.Frontmatter) {
	for _, key := range []string{"worktree", "merge", "pr"} {
		value, ok := fm.Get(key)
		if !ok {
			continue
		}
		if !loopValues[key][value] {
			set.Addf(file, 1, "plan.loop-invalid", "`%s: %s` is not one of %s", key, value, allowed(loopValues[key]))
		}
	}
}

// checkKickoffAs is checkKickoff with the rule slug the caller's subject uses, so a
// plan's finding reads `plan.` and a spec's reads `spec.` for the same defect.
func checkKickoffAs(set *finding.Set, file string, fm mdscan.Frontmatter, rule string) {
	for _, key := range []string{"autonomy", "ci"} {
		value, ok := fm.Get(key)
		if !ok {
			continue
		}
		if !kickoffValues[key][value] {
			set.Addf(file, 1, rule, "`%s: %s` is not one of %s", key, value, allowed(kickoffValues[key]))
		}
	}
}
