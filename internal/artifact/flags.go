package artifact

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/protonspy/spec-claude-code/internal/mdscan"
)

// A task's flags: the metadata that does not fit on the checkbox line.
//
// The vocabulary is closed, and that is the whole design. An italic one-liner is a
// shape prose uses too, so a parser that absorbed any of them would quietly eat a
// sentence and hand the task a region that is not the task. Four names are flags;
// anything else in that position is a fact the validator turns into a finding, and
// the parser leaves it exactly where it found it.
const (
	FlagDepends  = "Depends"
	FlagPriority = "Priority"
	FlagStatus   = "Status"
	FlagReason   = "Reason"
)

// StatusRemoved is the only value `_Status_` accepts.
//
// Not `open`, and not `completed`: the box is the state, and a flag that could
// restate it would be a second record of one fact — the defect
// `plan.item-has-two-records` already exists to catch.
const StatusRemoved = "removed"

// FlagIndent is what scc writes. Reading accepts column 0 as well, so a person who
// wrote the flag by hand is not punished for it, but everything scc emits is
// indented — and an indented line already belongs to the task under the same
// indentation rules the rest of the product uses.
const FlagIndent = "  "

// FlagNames is the canonical order flags are written back in. One order, so a file
// that has been through `scc patch` twice produces no diff the second time.
var FlagNames = []string{FlagDepends, FlagPriority, FlagStatus, FlagReason}

// flagLineRe matches a line that is nothing but one italic annotation: `_Depends 1.1_`.
// The whole line, because a flag is a line — an italic span inside a sentence is
// emphasis, and emphasis is prose.
var flagLineRe = regexp.MustCompile(`^[ \t]*_([A-Za-z][A-Za-z-]*)[ \t]*([^_]*)_[ \t]*$`)

// Flag is one annotation line as written, kept in file order so the validator can
// report a duplicate at the line that duplicated it.
type Flag struct {
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
	Line  int    `json:"line"`
}

// ItalicLine reads a line shaped like a flag, whatever it is called. It is what
// separates "a flag with a name nobody defined" from "a line that is not a flag at
// all" — the first is a typo worth reporting, the second is prose.
func ItalicLine(line string) (name, value string, ok bool) {
	m := flagLineRe.FindStringSubmatch(line)
	if m == nil {
		return "", "", false
	}
	return m[1], strings.TrimSpace(m[2]), true
}

// KnownFlag reads a line that is a flag in the vocabulary, returning the canonical
// spelling of its name.
func KnownFlag(line string) (name, value string, ok bool) {
	n, v, ok := ItalicLine(line)
	if !ok {
		return "", "", false
	}
	for _, want := range FlagNames {
		if strings.EqualFold(want, n) {
			return want, v, true
		}
	}
	return "", "", false
}

// Removed reports whether the task was struck out by discovery rather than done.
// A removed task keeps its line — the number it consumed is never reused, and the
// reason it went away is the record — so it is neither open nor complete.
func (t Task) Removed() bool { return t.Status == StatusRemoved }

// parseTaskFlags reads the flag block that belongs to t, extends t.End to cover it,
// and reports back which lines it claimed so they stay out of the description.
//
// Two runs, because a flag can be written two ways and both have to end up in the
// same region. An indented flag is already inside the task's block by the
// indentation rules that govern every list item, so it is found by walking back from
// the end of that block; a flag at column 0 belongs to nobody under those rules, so
// it is found by walking forward past at most one blank line. Either way the task's
// End covers its flags — which is what makes `map show 1.1` return them, `patch rm
// 1.1` remove them, and the rollback able to put them back.
func parseTaskFlags(doc *mdscan.Document, t *Task) map[int]bool {
	claimed := map[int]bool{}

	for n := t.End; n > t.Line; n-- {
		text := doc.Body[n-1]
		if strings.TrimSpace(text) == "" {
			continue
		}
		if _, _, ok := KnownFlag(text); !ok {
			// The line that stopped the run is prose — unless it is shaped like a flag,
			// in which case it is a flag whose name nobody defined, and saying so is
			// better than swallowing it into the description.
			if name, value, italic := ItalicLine(text); italic {
				t.UnknownFlags = append(t.UnknownFlags, Flag{Name: name, Value: value, Line: n})
			}
			break
		}
		claimed[n] = true
	}

	n := t.End + 1
	if n <= len(doc.Body) && strings.TrimSpace(doc.Body[n-1]) == "" {
		n++
	}
	for n <= len(doc.Body) {
		if _, _, ok := KnownFlag(doc.Body[n-1]); !ok {
			break
		}
		claimed[n] = true
		t.End = n
		n++
	}
	for n <= len(doc.Body) {
		name, value, italic := ItalicLine(doc.Body[n-1])
		if !italic {
			break
		}
		t.UnknownFlags = append(t.UnknownFlags, Flag{Name: name, Value: value, Line: n})
		n++
	}

	nums := make([]int, 0, len(claimed))
	for line := range claimed {
		nums = append(nums, line)
	}
	sort.Ints(nums)
	for _, line := range nums {
		name, value, _ := KnownFlag(doc.Body[line-1])
		t.Flags = append(t.Flags, Flag{Name: name, Value: value, Line: line})
	}
	applyFlags(t)
	sort.Slice(t.UnknownFlags, func(i, j int) bool { return t.UnknownFlags[i].Line < t.UnknownFlags[j].Line })
	return claimed
}

// applyFlags turns the annotations into the fields, first occurrence winning. A
// value the vocabulary does not accept is kept as the fact that it was written —
// the validator reports it, and renderTask writes it back rather than deleting
// something a person meant.
func applyFlags(t *Task) {
	seen := map[string]bool{}
	for _, f := range t.Flags {
		if seen[f.Name] {
			continue
		}
		seen[f.Name] = true
		switch f.Name {
		case FlagDepends:
			t.Depends = splitIDs(f.Value)
		case FlagPriority:
			n, err := strconv.Atoi(strings.TrimSpace(f.Value))
			if err != nil || n < 1 {
				t.BadPriority = f.Value
				continue
			}
			t.Priority = &n
		case FlagStatus:
			if !strings.EqualFold(strings.TrimSpace(f.Value), StatusRemoved) {
				t.BadStatus = f.Value
				continue
			}
			t.Status = StatusRemoved
		case FlagReason:
			t.Reason = f.Value
		}
	}
}

// splitIDs reads a comma-separated dependency list.
func splitIDs(s string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// renderFlags writes a task's flags in the canonical order, one per line, indented.
//
// An invalid value is written back as it was found rather than dropped: the
// validator has already said it is wrong, and a rewrite that silently deleted it
// would turn a reported defect into a lost sentence.
func renderFlags(t Task) []string {
	var out []string
	if len(t.Depends) > 0 {
		out = append(out, flagLine(FlagDepends, strings.Join(t.Depends, ", ")))
	}
	switch {
	case t.Priority != nil:
		out = append(out, flagLine(FlagPriority, strconv.Itoa(*t.Priority)))
	case t.BadPriority != "":
		out = append(out, flagLine(FlagPriority, t.BadPriority))
	}
	switch {
	case t.Status != "":
		out = append(out, flagLine(FlagStatus, t.Status))
	case t.BadStatus != "":
		out = append(out, flagLine(FlagStatus, t.BadStatus))
	}
	if t.Reason != "" {
		out = append(out, flagLine(FlagReason, t.Reason))
	}
	return out
}

func flagLine(name, value string) string {
	if value == "" {
		return FlagIndent + "_" + name + "_"
	}
	return FlagIndent + "_" + name + " " + value + "_"
}

// itemOf is the last component of a dotted number, read as an integer. A component
// that is not a number is 0, which is below every real one and therefore never the
// high-water mark.
func itemOf(number string) int {
	if i := strings.LastIndex(number, "."); i >= 0 {
		number = number[i+1:]
	}
	n, err := strconv.Atoi(strings.TrimSpace(number))
	if err != nil {
		return 0
	}
	return n
}

// CompareNumbers orders two task numbers the way a reader does: component by
// component, numerically.
//
// This is a correction, not a preference. Sorted as text `1.10` comes before `1.9`,
// so a loop that asked for the next task got the tenth before the ninth as soon as a
// group grew past nine items.
func CompareNumbers(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		x, errA := strconv.Atoi(as[i])
		y, errB := strconv.Atoi(bs[i])
		if errA != nil || errB != nil {
			if as[i] != bs[i] {
				return strings.Compare(as[i], bs[i])
			}
			continue
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(as) < len(bs):
		return -1
	case len(as) > len(bs):
		return 1
	}
	return 0
}
