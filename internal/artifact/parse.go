package artifact

import (
	"regexp"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/ears"
	"github.com/protonspy/spec-claude-code/internal/mdscan"
	"github.com/protonspy/spec-claude-code/internal/paths"
)

// The grammars, in one place. Every consumer reads the same regexes: the validators
// that report on them, and the reader that navigates by them. A reader that
// disagreed with the validator about what a task is would be worse than no reader —
// it would report a task list the validator says does not exist.
var (
	taskNumberRe = regexp.MustCompile(`^(\d+(?:\.\d+)*)\s+(.*)$`)
	// MethodologyRe matches the annotation that says how a task gets built.
	MethodologyRe = regexp.MustCompile(`\((Unit|TDD)\)`)
	// LooseMethodologyRe matches the near-misses, so a task annotated `(test-first)`
	// gets told what the vocabulary is instead of being told it has none.
	LooseMethodologyRe = regexp.MustCompile(`(?i)\((unit|tdd|test[- ]?first|none)\)`)
	// RequirementIDRe matches a citation of a requirement, anywhere.
	RequirementIDRe = regexp.MustCompile(`\bR\d+(?:\.\d+)+\b`)
	// RequirementRe matches a numbered requirement, and only in that position: a
	// bulleted item whose first content is the bolded id.
	RequirementRe = regexp.MustCompile(`^\s*[-*+]\s+\*\*(R\d+(?:\.\d+)+)\*\*\s*(?:\(([^)]*)\)\s*)?(.*)$`)
	// SpecRefRe matches a reference to a spec from inside a plan.
	SpecRefRe = regexp.MustCompile(`\b` + paths.SpecsSeg + `/([a-z0-9][a-z0-9-]*)/?`)

	listItemRe = regexp.MustCompile(`^([ \t]*)[-*+]\s+(.+)$`)
)

// CitationSeparator is the em dash that opens a task's requirement citations, per
// the grammar in the tasks rule.
const CitationSeparator = "—"

// Task is one checkbox read against the task grammar:
//
//   - [ ] 1.1 (Unit) Parse the manifest file — R1.2, R1.4
//
// The exported fields describe the task; the four unexported-by-convention grammar
// facts at the bottom describe what the *line* did or did not say, and exist so the
// validator can report a defect without parsing the line a second time. The parser
// states facts; turning a fact into a finding is the validator's job.
//
// The grammar is read from the checkbox's own line only. In practice a task's
// description runs on for several lines — every task in a real plan does — and
// Detail carries that continuation, but a citation wrapped onto the next line is a
// citation the validator already reports, and moving the goalposts here would change
// what eight validators say about files nobody edited.
type Task struct {
	Number       string   `json:"number"`
	Checked      bool     `json:"checked"`
	Methodology  string   `json:"methodology,omitempty"`
	Text         string   `json:"text"`
	Detail       string   `json:"detail,omitempty"`
	Requirements []string `json:"requirements,omitempty"`
	Line         int      `json:"line"`
	End          int      `json:"end"`
	Indent       int      `json:"-"`
	Section      string   `json:"section,omitempty"`

	// The flags, read from the annotation lines under the checkbox. See flags.go
	// for why the vocabulary is closed.
	Depends  []string `json:"depends,omitempty"`
	Priority *int     `json:"priority,omitempty"`
	Status   string   `json:"status,omitempty"`
	Reason   string   `json:"reason,omitempty"`

	// Derived, and in the JSON on purpose: the consumer is an agent, and an agent
	// that recomputed eligibility would be a second implementation of `--next`.
	Blocked  bool `json:"blocked"`
	Eligible bool `json:"eligible"`

	Methodologies int    `json:"-"` // how many annotations the line carried
	Loose         string `json:"-"` // a near-miss annotation, when there is no valid one
	HasCitation   bool   `json:"-"` // the line carried the em dash that opens citations
	Flags         []Flag `json:"-"` // every flag line, in file order, duplicates included
	UnknownFlags  []Flag `json:"-"` // italic one-liners in flag position that are not vocabulary
	BadPriority   string `json:"-"` // a `_Priority_` value that is not a positive integer
	BadStatus     string `json:"-"` // a `_Status_` value that is not `removed`

	// detail is the continuation verbatim, flag lines excluded, so a rewrite that
	// was not asked to change the description does not destroy it.
	detail []string
}

// Group is the task's number with its last component removed — the heading it
// belongs to under scc's numbering, and what `--group` filters on.
func (t Task) Group() string {
	if i := strings.LastIndex(t.Number, "."); i > 0 {
		return t.Number[:i]
	}
	return t.Number
}

// Summary is the task's description clipped to n runes, for a listing where one
// task is one line. It is the first line only: a 61-line task exists, and printing
// it in a list would defeat the point of the list.
func (t Task) Summary(n int) string { return clip(t.Text, n) }

// Requirement is one numbered EARS requirement.
type Requirement struct {
	ID      string `json:"id"`
	Text    string `json:"text"`
	Pattern string `json:"pattern,omitempty"` // the EARS pattern, when the line parses
	Delta   string `json:"delta,omitempty"`   // the parenthesised marker, e.g. (REMOVED)
	Line    int    `json:"line"`
	End     int    `json:"end"`
	Section string `json:"section,omitempty"`
}

// Block is a paragraph: a run of body lines inside a section that is neither a task
// nor a decomposition leaf.
//
// It is here because of what the artifacts actually look like. A real plan's
// `## Notes` was measured at 411 lines — half the file — carrying no headings at
// all, so section addressing bottoms out there and a reader still has to load the
// whole thing. Every one of those paragraphs opened with a bolded lead sentence,
// which is the convention this type exploits: the leads alone are an index, and the
// index is a twentieth of the prose.
type Block struct {
	Section string `json:"section"`
	Index   int    `json:"index"` // 1-based within its section
	Slug    string `json:"slug"`  // from the lead, for an address that survives an insert
	Lead    string `json:"lead"`
	Line    int    `json:"line"`
	End     int    `json:"end"`
}

// ParseTasks reads every checkbox in doc against the task grammar.
//
// mdscan has already excluded the checkboxes inside fenced blocks and HTML
// comments, which is what keeps a rule file documenting the grammar — or a template
// showing an example — from producing tasks nobody wrote.
func ParseTasks(doc *mdscan.Document) []Task {
	return parseTasks(doc)
}

func parseTasks(doc *mdscan.Document) []Task {
	tasks := make([]Task, 0, len(doc.Checkboxes))
	for _, box := range doc.Checkboxes {
		t := Task{Line: box.Line, End: box.Line, Checked: box.Checked, Indent: box.Indent}
		rest := box.Text

		if m := taskNumberRe.FindStringSubmatch(rest); m != nil {
			t.Number, rest = m[1], m[2]
		}

		found := MethodologyRe.FindAllStringSubmatch(rest, -1)
		t.Methodologies = len(found)
		switch len(found) {
		case 0:
			t.Loose = LooseMethodologyRe.FindString(rest)
		case 1:
			t.Methodology = found[0][1]
		}

		text, tail, hasTail := strings.Cut(rest, CitationSeparator)
		t.Text = strings.TrimSpace(MethodologyRe.ReplaceAllString(text, ""))
		t.Requirements = RequirementIDRe.FindAllString(tail, -1)
		t.HasCitation = hasTail

		t.End = blockEnd(doc, box.Line, box.Indent)
		claimed := parseTaskFlags(doc, &t)
		t.Detail = joinLinesExcept(doc, box.Line+1, t.End, claimed)
		t.detail = rawLinesExcept(doc, box.Line+1, t.End, claimed)
		tasks = append(tasks, t)
	}
	return tasks
}

// parseRequirements reads the numbered requirements out of a requirements.md.
//
// Prose elsewhere in the file is not a requirement and is not reported as one: the
// anchor is the bulleted, bolded id, which is what the template writes and what the
// rule asks for.
func parseRequirements(doc *mdscan.Document) []Requirement {
	var out []Requirement
	for i, line := range doc.Body {
		m := RequirementRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		r := Requirement{ID: m[1], Delta: m[2], Text: strings.TrimSpace(m[3]), Line: i + 1}
		r.End = blockEnd(doc, r.Line, indentOf(line))
		if r.Delta == "" {
			if parsed, err := ears.Parse(r.Text); err == nil {
				r.Pattern = string(parsed.Pattern)
			}
		}
		out = append(out, r)
	}
	return out
}

// parseLeaves reads a plan's decomposition: the list items that reference a spec.
//
// A leaf never carries a checkbox — one source of truth per item, and a leaf's
// state lives in the spec it names — so a list item that has one is a task, and is
// skipped here rather than counted twice.
func parseLeaves(doc *mdscan.Document) []Leaf {
	boxes := map[int]bool{}
	for _, b := range doc.Checkboxes {
		boxes[b.Line] = true
	}
	var out []Leaf
	for i, line := range doc.Body {
		num := i + 1
		if boxes[num] {
			continue
		}
		m := listItemRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		ref := SpecRefRe.FindStringSubmatch(m[2])
		if ref == nil {
			continue
		}
		l := Leaf{
			Ref:     paths.SpecsSeg + "/" + ref[1] + "/",
			Feature: ref[1],
			Line:    num,
			End:     blockEnd(doc, num, indentOf(line)),
		}
		l.Text = leafText(m[2])
		out = append(out, l)
	}
	return out
}

// leafText is the leaf's description: what follows the em dash that separates the
// reference from what it covers, or the whole item when there is none.
func leafText(item string) string {
	if _, after, ok := strings.Cut(item, CitationSeparator); ok {
		return strings.TrimSpace(after)
	}
	return strings.TrimSpace(SpecRefRe.ReplaceAllString(item, ""))
}

// Blocks is the artifact's paragraphs, section by section.
func (a *Artifact) Blocks() []Block {
	claimed := map[int]bool{}
	for _, t := range a.Tasks {
		for n := t.Line; n <= t.End; n++ {
			claimed[n] = true
		}
	}
	for _, l := range a.Leaves {
		for n := l.Line; n <= l.End; n++ {
			claimed[n] = true
		}
	}
	headings := map[int]bool{}
	for _, h := range a.doc.Headings {
		headings[h.Line] = true
	}

	var out []Block
	for _, s := range a.Sections {
		index := 0
		n := s.Line + 1
		for n <= s.BodyEnd {
			if claimed[n] || headings[n] || strings.TrimSpace(a.doc.Body[n-1]) == "" {
				n++
				continue
			}
			start := n
			for n <= s.BodyEnd && !claimed[n] && !headings[n] && strings.TrimSpace(a.doc.Body[n-1]) != "" {
				n++
			}
			index++
			lead := leadText(a.Lines[start-1])
			out = append(out, Block{
				Section: s.Slug,
				Index:   index,
				Slug:    mdscan.Slug(clip(lead, 60)),
				Lead:    lead,
				Line:    start,
				End:     n - 1,
			})
		}
	}
	return out
}

// leadText is a paragraph's opening line with Markdown emphasis stripped, so the
// index reads as sentences rather than as syntax.
func leadText(line string) string {
	s := strings.TrimSpace(line)
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "`", "")
	return strings.TrimSpace(s)
}

// blockEnd is where a list item's continuation stops: the last line that is
// indented past the item's own marker, blank lines inside the run included, and
// stopping at the next heading or the next item at the same or a shallower indent.
//
// This is what makes a task addressable as a region rather than as a line. Measured
// on a real plan, every task ran to more than one line and the longest ran to
// sixty-one — a reader that returned only the checkbox line would return the number
// and none of the decision.
func blockEnd(doc *mdscan.Document, start, indent int) int {
	end := start
	for n := start + 1; n <= len(doc.Body); n++ {
		line := doc.Body[n-1]
		if strings.TrimSpace(line) == "" {
			continue
		}
		if indentOf(line) <= indent || isHeading(doc, n) {
			break
		}
		end = n
	}
	return end
}

func isHeading(doc *mdscan.Document, line int) bool {
	for _, h := range doc.Headings {
		if h.Line == line {
			return true
		}
	}
	return false
}

// indentOf counts leading whitespace with tabs expanded to four, matching how
// mdscan reports a checkbox's indent.
func indentOf(s string) int {
	n := 0
	for _, r := range s {
		switch r {
		case ' ':
			n++
		case '\t':
			n += 4
		default:
			return n
		}
	}
	return n
}

// joinLinesExcept collapses a line range into one whitespace-normalized string, with
// a set of lines held back — a task's flags, which belong to its region but not to
// its description. Keeping `_Priority 2_` out of Detail is what stops the searcher
// from indexing it as prose and keeps `--width` honest about how much of the
// description it clipped. A nil set is the plain join.
func joinLinesExcept(doc *mdscan.Document, from, to int, skip map[int]bool) string {
	if from > to {
		return ""
	}
	var parts []string
	for n := from; n <= to && n <= len(doc.Body); n++ {
		if skip[n] {
			continue
		}
		if f := strings.Fields(doc.Body[n-1]); len(f) > 0 {
			parts = append(parts, strings.Join(f, " "))
		}
	}
	return strings.Join(parts, " ")
}

// rawLinesExcept is the same range as it sits in the file, flags removed and
// trailing blanks trimmed. It is what a rewrite puts back when it was not asked to
// change the description.
func rawLinesExcept(doc *mdscan.Document, from, to int, skip map[int]bool) []string {
	var out []string
	for n := from; n <= to && n <= len(doc.Lines); n++ {
		if skip[n] {
			continue
		}
		out = append(out, doc.Lines[n-1])
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return out
}

// clip shortens s to n runes, marking that it was shortened. It counts runes rather
// than bytes because the artifacts are full of em dashes and box-drawing characters,
// and a byte clip would cut one in half.
func clip(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return strings.TrimRight(string(r[:n-1]), " ") + "…"
}
