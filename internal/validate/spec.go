package validate

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/ears"
	"github.com/protonspy/spec-claude-code/internal/finding"
	"github.com/protonspy/spec-claude-code/internal/paths"
)

// requirementRe matches a numbered requirement, and only in that position:
//
//   - **R1.2** (MODIFIED) When the manifest is missing, the CLI shall exit 1
//
// Prose elsewhere in requirements.md is not a requirement and gets no findings. The
// alternative — parsing every sentence as EARS — is a validator that fires on the
// document's own introduction, which is exactly how a tool teaches its user to
// disbelieve it.
var requirementRe = regexp.MustCompile(`^\s*[-*+]\s+\*\*(R\d+(?:\.\d+)+)\*\*\s*(?:\(([^)]*)\)\s*)?(.*)$`)

// The delta markers a change to an existing spec is written with. A change is
// proposed as a delta so that adopting scc does not mean writing the spec for
// everything that already works, and so two sessions can change one spec as long as
// they touch different requirements.
var deltaMarkers = map[string]bool{"ADDED": true, "MODIFIED": true, "REMOVED": true}

// The kickoff answers, validated only when present. A missing key means the run
// predates the convention, not that the user did something wrong.
var kickoffValues = map[string]map[string]bool{
	"autonomy": {"auto": true, "gated": true},
	"ci":       {"wait": true, "no-wait": true},
}

// requirement is one parsed requirement line.
type requirement struct {
	ID    string
	Line  int
	Delta string // "", "ADDED", "MODIFIED", "REMOVED"
	Text  string
}

// Specs validates every spec under specs/. A workspace with no specs is not a
// workspace with findings.
func Specs(root string) (*finding.Set, error) {
	set := &finding.Set{}
	entries, err := os.ReadDir(paths.Specs(root))
	if err != nil {
		if os.IsNotExist(err) {
			return set, nil
		}
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		one, err := Spec(root, name)
		if err != nil {
			return nil, err
		}
		set.Extend(one)
	}
	return set, nil
}

// Spec validates one feature's three artifacts.
func Spec(root, feature string) (*finding.Set, error) {
	set := &finding.Set{}
	dir := paths.Spec(root, feature)
	if !isDir(dir) {
		return nil, fmt.Errorf("no spec %q under %s", feature, paths.SpecsSeg)
	}

	reqs, ok, err := checkRequirements(set, root, feature)
	if err != nil {
		return nil, err
	}
	if err := checkDesign(set, root, feature, reqs, ok); err != nil {
		return nil, err
	}
	if err := checkTasks(set, root, feature, reqs, ok); err != nil {
		return nil, err
	}
	return set, nil
}

// checkRequirements parses requirements.md and returns the requirements it found. ok
// is false when the file is missing or unreadable, which every downstream check honors
// by staying silent: a traceability finding against requirements nobody could read is
// a finding about the wrong thing.
func checkRequirements(set *finding.Set, root, feature string) (map[string]requirement, bool, error) {
	path := paths.Requirements(root, feature)
	file := rel(root, path)
	if !isFile(path) {
		set.Addf(file, 0, "spec.missing-requirements",
			"a spec needs %s: what the feature must do, in EARS, numbered", paths.RequirementsSeg)
		return nil, false, nil
	}
	doc, err := read(root, path)
	if err != nil {
		if doc == nil {
			return nil, false, err
		}
		set.Addf(file, 1, "spec.frontmatter-unreadable", "%v", err)
		return nil, false, nil
	}
	checkKickoffAs(set, file, doc.Frontmatter, "spec.kickoff-invalid")

	reqs := map[string]requirement{}
	for i, line := range doc.Body {
		m := requirementRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		r := requirement{ID: m[1], Line: i + 1, Delta: m[2], Text: strings.TrimSpace(m[3])}

		if prior, dup := reqs[r.ID]; dup {
			set.Addf(file, r.Line, "spec.duplicate-requirement",
				"%s is already defined on line %d; a task citing it would be ambiguous", r.ID, prior.Line)
			continue
		}
		if r.Delta != "" && !deltaMarkers[strings.ToUpper(r.Delta)] {
			set.Addf(file, r.Line, "spec.delta-marker-invalid",
				"%q is not a delta marker; use ADDED, MODIFIED, or REMOVED", r.Delta)
		} else if r.Delta != "" && r.Delta != strings.ToUpper(r.Delta) {
			set.Addf(file, r.Line, "spec.delta-marker-invalid",
				"delta markers are upper case: write (%s)", strings.ToUpper(r.Delta))
		}
		r.Delta = strings.ToUpper(r.Delta)

		// A REMOVED requirement is a statement that it is gone. There is nothing left
		// to hold to EARS, and nothing for a task to implement.
		if r.Delta != "REMOVED" {
			if _, err := ears.Parse(r.Text); err != nil {
				set.Addf(file, r.Line, "ears.malformed", "%s: %v", r.ID, err)
			}
		}
		reqs[r.ID] = r
	}

	if len(reqs) == 0 {
		set.Addf(file, 0, "spec.no-requirements",
			"no numbered requirements found; write them as `- **R1.1** The <system> shall <response>`")
		return reqs, false, nil
	}
	return reqs, true, nil
}

// checkDesign holds design.md to the one thing the design says to check: that it
// exists and traces to its requirements.
//
// It deliberately does not check that any particular section is present. A required
// heading is a request for filler, and invented architecture is worse than verbose —
// the next session reads it as a decision somebody made and honors it.
func checkDesign(set *finding.Set, root, feature string, reqs map[string]requirement, reqsOK bool) error {
	path := paths.Design(root, feature)
	file := rel(root, path)
	if !isFile(path) {
		set.Addf(file, 0, "spec.missing-design",
			"a spec needs %s, even when the answer is a few paragraphs", paths.DesignSeg)
		return nil
	}
	doc, err := read(root, path)
	if err != nil {
		if doc == nil {
			return err
		}
		set.Addf(file, 1, "spec.frontmatter-unreadable", "%v", err)
		return nil
	}
	if !reqsOK {
		return nil
	}

	cited := map[string]bool{}
	for i, line := range doc.Body {
		for _, id := range requirementIDRe.FindAllString(line, -1) {
			if _, ok := reqs[id]; !ok {
				set.Addf(file, i+1, "spec.unknown-requirement",
					"%s is not defined in %s", id, paths.RequirementsSeg)
				continue
			}
			cited[id] = true
		}
	}
	if len(cited) == 0 {
		set.Addf(file, 0, "spec.design-cites-nothing",
			"the design names no requirement it serves; cite them (R1.1) so the trace from what to how is readable")
	}
	return nil
}

// checkTasks holds tasks.md to the grammar and closes the traceability loop in both
// directions: every task cites a requirement that exists, and every requirement
// reaches at least one task.
func checkTasks(set *finding.Set, root, feature string, reqs map[string]requirement, reqsOK bool) error {
	path := paths.Tasks(root, feature)
	file := rel(root, path)
	if !isFile(path) {
		set.Addf(file, 0, "spec.missing-tasks",
			"a spec needs %s: the work, each task annotated (Unit) or (TDD)", paths.TasksSeg)
		return nil
	}
	doc, err := read(root, path)
	if err != nil {
		if doc == nil {
			return err
		}
		set.Addf(file, 1, "spec.frontmatter-unreadable", "%v", err)
		return nil
	}

	tasks := parseTasks(set, file, doc, true)
	if len(tasks) == 0 {
		set.Addf(file, 0, "spec.no-tasks",
			"no tasks found; write them as `- [ ] 1.1 (Unit) <description> — R1.1`")
		return nil
	}
	if !reqsOK {
		return nil
	}

	covered := map[string]bool{}
	for _, t := range tasks {
		for _, id := range t.Requirements {
			if _, ok := reqs[id]; !ok {
				set.Addf(file, t.Line, "spec.unknown-requirement",
					"task %s cites %s, which is not defined in %s", t.Number, id, paths.RequirementsSeg)
				continue
			}
			covered[id] = true
		}
	}

	// Every requirement reaches a task. This is the direction that catches the
	// requirement everyone agreed on and nobody built.
	reqFile := rel(root, paths.Requirements(root, feature))
	ids := make([]string, 0, len(reqs))
	for id := range reqs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		r := reqs[id]
		if r.Delta == "REMOVED" || covered[id] {
			continue
		}
		set.Addf(reqFile, r.Line, "spec.orphan-requirement",
			"%s reaches no task in %s", id, paths.TasksSeg)
	}
	return nil
}

func allowed(values map[string]bool) string {
	out := make([]string, 0, len(values))
	for v := range values {
		out = append(out, v)
	}
	sort.Strings(out)
	return strings.Join(out, " | ")
}
