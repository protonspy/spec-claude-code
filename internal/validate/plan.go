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
	checkSeal(set, file, doc.Frontmatter)
	checkPlanShape(set, file, doc)

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

// The plan's sections, and there are no others.
//
// A closed set is the whole mechanism. Nothing else caps a plan's size: the file is
// preloaded by every session that runs it, and a plan measured at 56KB got there
// because a heading nobody forbade — `## Notes` — grew to half the file. There is no
// line limit anywhere in this contract except on the description, because a limit
// that fires on a legitimate plan would be worse than the growth it prevents. What
// there is instead is nowhere for prose to go.
//
// Order is deliberately not checked. Every read of a plan addresses a section by its
// slug, so the order on disk changes no answer, and validating it would cost a rule
// and a migration to buy nothing.
var planSections = []PlanSection{
	{"Why", "why", true},
	{"Paths", "paths", false},
	{"References", "references", false},
	{"Out of scope", "out-of-scope", false},
	{"Tasks", "tasks", true},
	{"Done when", "done-when", true},
}

// PlanSection is one heading the contract allows. Exported because migration has to
// create exactly the sections this validator demands — two lists would drift, and the
// drift would show up as a migration whose output fails the validator that asked for it.
type PlanSection struct {
	Title    string
	Slug     string
	Required bool
}

// PlanSections is the contract, in the order the template writes it.
func PlanSections() []PlanSection { return append([]PlanSection(nil), planSections...) }

// maxDescriptionLines is how long the opening prose may run before it stops being a
// description of the work and starts being the essay the closed sections exist to
// prevent. Counted in non-blank lines, which is the unit a writer can see.
const maxDescriptionLines = 6

// checkPlanShape holds the plan to its contract: a title, a short description, the
// three required sections, and no heading nobody agreed to.
func checkPlanShape(set *finding.Set, file string, doc *mdscan.Document) {
	title := 0
	present := map[string]int{}
	for _, h := range doc.Headings {
		if h.Level == 1 && title == 0 {
			title = h.Line
		}
		if h.Level != 2 {
			continue
		}
		slug := h.Slug
		known := false
		for _, s := range planSections {
			if slug == s.Slug {
				known = true
				break
			}
		}
		if !known {
			// Notes gets its own sentence because it is the heading this contract was
			// written against, and because "unknown section" would read as a typo when
			// the answer is that the content belongs somewhere else entirely.
			if slug == "notes" {
				set.Addf(file, h.Line, "plan.unknown-section",
					"a plan has no notes section: what was decided and why is an ADR under %s/%s, "+
						"what changed is git, and a constraint on one item goes on that item's own line",
					paths.DocsSeg, paths.ADRSeg)
				continue
			}
			set.Addf(file, h.Line, "plan.unknown-section",
				"`## %s` is not one of a plan's sections (%s); a plan is a header and a checklist, and prose has nowhere else to go on purpose",
				h.Text, sectionTitles())
			continue
		}
		if _, dup := present[slug]; !dup {
			present[slug] = h.Line
		}
	}

	if title == 0 {
		set.Addf(file, 1, "plan.missing-title", "a plan opens with its title as an `# H1`")
	}
	for _, s := range planSections {
		if s.Required && present[s.Slug] == 0 {
			set.Addf(file, 1, "plan.missing-section", "a plan has a `## %s` section", s.Title)
		}
	}

	if title == 0 {
		return
	}
	stop := len(doc.Body)
	for _, h := range doc.Headings {
		if h.Line > title {
			stop = h.Line - 1
			break
		}
	}
	lines := 0
	for n := title + 1; n <= stop && n <= len(doc.Body); n++ {
		if strings.TrimSpace(doc.Body[n-1]) != "" {
			lines++
		}
	}
	switch {
	case lines == 0:
		set.Addf(file, title, "plan.missing-description",
			"a plan says what it is in one to three sentences under the title, before the first section")
	case lines > maxDescriptionLines:
		set.Addf(file, title, "plan.description-too-long",
			"the description runs to %d lines; it is one to three sentences, and the rest belongs in `## Why`", lines)
	}
}

func sectionTitles() string {
	out := make([]string, 0, len(planSections))
	for _, s := range planSections {
		out = append(out, s.Title)
	}
	return strings.Join(out, ", ")
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

// checkSeal validates the two keys `plan approve` writes. They are checked only when
// present, like every other frontmatter answer: a plan nobody has approved is not a
// plan with a defect.
//
// The pair has to travel together. A `status: approved` with no checksum is a plan
// claiming to be sealed by a seal that is not there, and every read of it would
// silently skip the check it was approved to get.
func checkSeal(set *finding.Set, file string, fm mdscan.Frontmatter) {
	status, has := fm.Get("status")
	if !has {
		return
	}
	if status != artifact.StatusDraft && status != artifact.StatusApproved {
		set.Addf(file, 1, "plan.status-invalid", "`status: %s` is not one of %s, %s",
			status, artifact.StatusDraft, artifact.StatusApproved)
		return
	}
	if sum, _ := fm.Get(artifact.KeyChecksum); status == artifact.StatusApproved && sum == "" {
		set.Addf(file, 1, "plan.unsealed",
			"`status: %s` with no `%s:` — nothing can be checked against, so the approval means nothing",
			artifact.StatusApproved, artifact.KeyChecksum)
	}
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
	for _, key := range []string{"autonomy", "ci", "lang"} {
		value, ok := fm.Get(key)
		if !ok {
			continue
		}
		if !kickoffValues[key][value] {
			set.Addf(file, 1, rule, "`%s: %s` is not one of %s", key, value, allowed(kickoffValues[key]))
		}
	}
}
