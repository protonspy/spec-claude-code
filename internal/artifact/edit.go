package artifact

import (
	"fmt"
	"sort"
	"strings"
)

// Editing an artifact without reading it.
//
// This is the half of the package that exists for a policy rather than for a cost:
// an agent harness makes a tool read a file before it may edit it, because a
// string-match edit against a file nobody looked at is a footgun. That guard is
// right for a text editor and wrong for a structured artifact — every edit here is
// addressed by a name the caller already knows (`1.2`, `#notes`, `R1.4`), applies to
// a region the parser found rather than to a string the caller guessed, and reports
// exactly which lines it replaced. The file never has to be in anyone's context.
//
// The safety that replaces reading-first is threefold: an address that does not
// resolve is an error and not an insert; overlapping edits are refused rather than
// merged; and the caller gets the before/after lines back, which is the confirmation
// a read was standing in for.

// LineWidth is where a rewritten task or paragraph wraps. It matches what the
// artifacts already do, so an edited file does not announce itself by its ragging.
const LineWidth = 88

// Change is one applied splice, in the form a caller can print as confirmation.
type Change struct {
	Op     string   `json:"op"`
	Ref    string   `json:"ref"`
	Line   int      `json:"line"`
	Before []string `json:"before,omitempty"`
	After  []string `json:"after,omitempty"`
}

// splice replaces del lines starting at line at (1-based) with lines. del == 0 is a
// pure insertion before at.
//
// seq is the order the caller asked for, and it matters only where several
// insertions share one position — `patch fm a=1 b=2 c=3` on a file that has none of
// them. Applying bottom-up means those have to go in reverse, or the caller's order
// comes out backwards in the file.
type splice struct {
	at    int
	del   int
	seq   int
	lines []string
	ch    Change
}

// Editor accumulates edits against one artifact and applies them at the end.
//
// Every address is resolved against the *original* file, and the splices are applied
// from the bottom up, so a batch of edits cannot invalidate its own line numbers
// halfway through. Two edits that touch the same region are a mistake rather than a
// merge, and are refused.
type Editor struct {
	a       *Artifact
	splices []splice
	err     error
}

// Edit opens an editor over the artifact.
func (a *Artifact) Edit() *Editor { return &Editor{a: a} }

// Err is the first failure, if any. Every method is a no-op once one has happened,
// so a caller can chain and check once.
func (e *Editor) Err() error { return e.err }

func (e *Editor) fail(format string, args ...any) {
	if e.err == nil {
		e.err = fmt.Errorf(format, args...)
	}
}

// Fail records a refusal from the caller's own policy — the approved-plan guards,
// which depend on what the file says and so cannot be decided before it is loaded.
// It reads back as an editor error so one path reports every reason a patch stopped.
func (e *Editor) Fail(format string, args ...any) { e.fail(format, args...) }

// Empty reports whether nothing would change.
func (e *Editor) Empty() bool { return len(e.splices) == 0 }

// Changes is what the editor did, in file order.
func (e *Editor) Changes() []Change {
	out := make([]Change, 0, len(e.splices))
	ordered := append([]splice(nil), e.splices...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].at < ordered[j].at })
	for _, s := range ordered {
		out = append(out, s.ch)
	}
	return out
}

// Content applies every splice and returns the new file, LF-terminated lines joined
// the way the whole tree writes Markdown.
func (e *Editor) Content() (string, error) {
	if e.err != nil {
		return "", e.err
	}
	ordered := append([]splice(nil), e.splices...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].at != ordered[j].at {
			return ordered[i].at > ordered[j].at
		}
		return ordered[i].seq > ordered[j].seq
	})
	for i := 1; i < len(ordered); i++ {
		prev, cur := ordered[i-1], ordered[i]
		if cur.at+cur.del > prev.at {
			return "", fmt.Errorf("two edits touch the same lines (%s and %s); apply them one at a time",
				cur.ch.Ref, prev.ch.Ref)
		}
	}
	lines := append([]string(nil), e.a.Lines...)
	for _, s := range ordered {
		head := append([]string(nil), lines[:s.at-1]...)
		tail := lines[s.at-1+s.del:]
		lines = append(append(head, s.lines...), tail...)
	}
	return strings.Join(lines, "\n"), nil
}

// add records a splice, capturing the lines it displaced for the report.
func (e *Editor) add(op, ref string, at, del int, lines []string) {
	if e.err != nil {
		return
	}
	var before []string
	if del > 0 {
		before = append([]string(nil), e.a.Lines[at-1:at-1+del]...)
	}
	e.splices = append(e.splices, splice{
		at: at, del: del, seq: len(e.splices), lines: lines,
		ch: Change{Op: op, Ref: ref, Line: at, Before: before, After: lines},
	})
}

// Check marks a task done or not done.
//
// It rewrites the box and nothing else — not the number, not the description, not
// the wrapping — because the state of a task is the one fact the box holds, and an
// edit that reflowed the line while flipping it would put a diff in front of a
// reviewer that hides what actually changed.
func (e *Editor) Check(number string, done bool) {
	if e.err != nil {
		return
	}
	t, ok := e.a.Task(number)
	if !ok {
		e.fail("%v", e.a.unknown("task", number))
		return
	}
	if t.Checked == done {
		return // already in that state; not an edit, and not an error either
	}
	line := e.a.Lines[t.Line-1]
	from, to := "[ ]", "[x]"
	if !done {
		from, to = "[x]", "[ ]"
	}
	i := strings.Index(strings.ToLower(line), strings.ToLower(from))
	if i < 0 {
		e.fail("task %s does not carry a checkbox on line %d", number, t.Line)
		return
	}
	op := "uncheck"
	if done {
		op = "check"
	}
	e.add(op, number, t.Line, 1, []string{line[:i] + to + line[i+3:]})
}

// TaskEdit is what SetTask may change. A nil field is left alone, which is what
// makes "change only the methodology" expressible without restating the description.
type TaskEdit struct {
	Text         *string
	Methodology  *string
	Requirements *[]string
	Number       *string
	Checked      *bool

	// The flags. Priority is two fields rather than one **int: "set it to 3" and
	// "it has none" are different edits, and a pointer to a pointer expresses that
	// at the cost of every caller having to read it twice.
	Depends       *[]string
	Priority      *int
	ClearPriority bool
	Status        *string
	Reason        *string
}

// SetTask rewrites one task in place, re-rendering it from its parts.
//
// The whole block is replaced, continuation lines included, because a task's
// description is mostly in its continuation — every task in a real plan runs past
// one line — and an edit that touched only the first line would leave the rest
// describing the old task.
func (e *Editor) SetTask(number string, edit TaskEdit) {
	if e.err != nil {
		return
	}
	t, ok := e.a.Task(number)
	if !ok {
		e.fail("%v", e.a.unknown("task", number))
		return
	}
	next := t
	if edit.Number != nil {
		next.Number = *edit.Number
	}
	if edit.Methodology != nil {
		next.Methodology = *edit.Methodology
	}
	if edit.Checked != nil {
		next.Checked = *edit.Checked
	}
	if edit.Requirements != nil {
		next.Requirements = *edit.Requirements
	}
	if edit.Text != nil {
		next.Text = strings.TrimSpace(*edit.Text)
		next.Detail = ""
		next.detail = nil
	}
	if edit.Depends != nil {
		next.Depends = *edit.Depends
	}
	if edit.ClearPriority {
		next.Priority, next.BadPriority = nil, ""
	}
	if edit.Priority != nil {
		p := *edit.Priority
		next.Priority, next.BadPriority = &p, ""
	}
	if edit.Status != nil {
		next.Status, next.BadStatus = *edit.Status, ""
	}
	if edit.Reason != nil {
		next.Reason = strings.TrimSpace(*edit.Reason)
	}
	e.add("set", number, t.Line, t.End-t.Line+1, renderTask(next))
}

// StrikeTask is discovery's removal: the task stays where it is, carrying the reason
// it stopped being work.
//
// A logical removal rather than a deletion, because the number is the address every
// other artifact and every earlier commit used, and because a plan that silently
// shrank would be a plan whose history is only in git. The high-water mark that
// stops the number being reused is derived from this line staying put.
func (e *Editor) StrikeTask(number, reason string) {
	if e.err != nil {
		return
	}
	t, ok := e.a.Task(number)
	if !ok {
		e.fail("%v", e.a.unknown("task", number))
		return
	}
	if strings.TrimSpace(reason) == "" {
		e.fail("removing task %s needs a reason: it is the whole record of why the work went away", number)
		return
	}
	if t.Removed() {
		return
	}
	next := t
	next.Status, next.BadStatus = StatusRemoved, ""
	next.Reason = strings.TrimSpace(reason)
	e.add("strike", number, t.Line, t.End-t.Line+1, renderTask(next))
}

// NewTask is a task about to be written. Section is where it lands: the slug of the
// heading whose task list it joins.
type NewTask struct {
	Section      string
	Number       string
	Methodology  string
	Text         string
	Requirements []string
	Checked      bool
	Depends      []string
	Priority     *int
	// Reason is why this task turned up after the plan was approved. It is the same
	// flag a removal carries, because it answers the same question — why this line is
	// not what the plan somebody agreed to said.
	Reason string
}

// AddTask appends a task to a section's list.
//
// It lands after the last task already in that section — not at the end of the
// section — so a section that ends in prose keeps that prose last, which is where
// every artifact scc writes puts it.
func (e *Editor) AddTask(t NewTask) {
	if e.err != nil {
		return
	}
	s, ok := e.a.Section(t.Section)
	if !ok {
		e.fail("%v", e.a.unknown("section", t.Section))
		return
	}
	if t.Number == "" {
		e.fail("a task needs a number: it is how every later command addresses it")
		return
	}
	if _, taken := e.a.Task(t.Number); taken {
		e.fail("task %s already exists in %s", t.Number, e.a.Path)
		return
	}
	at := s.Line + 1
	for _, existing := range e.a.Tasks {
		if existing.Line >= s.Line && existing.Line <= s.End && existing.End+1 > at {
			at = existing.End + 1
		}
	}
	if at == s.Line+1 {
		// No tasks yet: land after the section's own prose, before its first
		// subsection, past the blank lines that trail it.
		at = trimBlank(e.a.Lines, s.Line+1, s.BodyEnd) + 1
	}
	lines := renderTask(Task{
		Number: t.Number, Methodology: t.Methodology, Text: t.Text,
		Requirements: t.Requirements, Checked: t.Checked,
		Depends: t.Depends, Priority: t.Priority, Reason: t.Reason,
	})
	e.add("add", t.Number, at, 0, lines)
}

// RemoveTask deletes a task and its continuation.
func (e *Editor) RemoveTask(number string) {
	if e.err != nil {
		return
	}
	t, ok := e.a.Task(number)
	if !ok {
		e.fail("%v", e.a.unknown("task", number))
		return
	}
	e.add("remove", number, t.Line, t.End-t.Line+1, nil)
}

// Replace swaps the target's lines for text.
func (e *Editor) Replace(ref, text string) {
	if e.err != nil {
		return
	}
	target, err := e.a.Find(ref)
	if err != nil {
		e.fail("%v", err)
		return
	}
	if target.Kind == TargetSection {
		// Replacing a section means replacing what it says, not deleting its heading:
		// an edit that removed the heading would move every following section up a
		// level and silently re-parent the file.
		e.add("replace", ref, target.Line+1, target.End-target.Line, blockLines(text))
		return
	}
	e.add("replace", ref, target.Line, target.Lines(), blockLines(text))
}

// Append writes text after the target.
//
// For a section that means after its own prose and before its first subsection,
// never inside the last one — appending to `## Decomposition` must not silently join
// the milestone that happens to be last.
func (e *Editor) Append(ref, text string) {
	if e.err != nil {
		return
	}
	target, err := e.a.Find(ref)
	if err != nil {
		e.fail("%v", err)
		return
	}
	end := target.End
	if target.Kind == TargetSection {
		if s, ok := e.a.Section(target.Ref); ok {
			end = s.BodyEnd
		}
	}
	at := trimBlank(e.a.Lines, target.Line, end) + 1
	e.add("append", ref, at, 0, append([]string{""}, blockLines(text)...))
}

// Prepend writes text immediately after the target's first line — under a heading,
// above whatever it already says.
func (e *Editor) Prepend(ref, text string) {
	if e.err != nil {
		return
	}
	target, err := e.a.Find(ref)
	if err != nil {
		e.fail("%v", err)
		return
	}
	at := target.Line
	if target.Kind == TargetSection {
		at = target.Line + 1
	}
	e.add("prepend", ref, at, 0, append(blockLines(text), ""))
}

// SetFrontmatter writes one key in the leading block, adding the block if the file
// has none. It is how the plan-run loop records its answers — worktree, merge, pr —
// without the skill having to hold the file.
func (e *Editor) SetFrontmatter(key, value string) {
	if e.err != nil {
		return
	}
	line := key + ": " + value
	fm := e.a.doc.Frontmatter
	if !fm.Present {
		e.add("frontmatter", key, 1, 0, []string{"---", line, "---", ""})
		return
	}
	for n := 2; n < fm.Lines; n++ {
		k, v, ok := strings.Cut(e.a.Lines[n-1], ":")
		if !ok || strings.TrimSpace(k) != key {
			continue
		}
		// Writing the value it already has is not an edit. The plan-run loop restates
		// every answer on every resume, and half of them are unchanged; reporting those
		// as changes would bury the one that moved.
		if strings.TrimSpace(v) == value {
			return
		}
		e.add("frontmatter", key, n, 1, []string{line})
		return
	}
	e.add("frontmatter", key, fm.Lines, 0, []string{line})
}

// renderTask writes a task back out in the grammar, wrapped the way the artifacts
// wrap: the continuation indented to sit under the description rather than under the
// bullet, which is what makes a multi-line task read as one item.
//
// It re-emits the continuation and the flags, and both are load-bearing. A rewrite
// that dropped them would make `patch task --method TDD` a data-loss command:
// changing one word would silently delete a sixty-line description and every
// dependency the task declared. The flags come last, in the canonical order, so a
// file that has been through this twice produces no diff the second time.
func renderTask(t Task) []string {
	head := "- [ ] "
	if t.Checked {
		head = "- [x] "
	}
	body := t.Number
	if t.Methodology != "" {
		body += " (" + t.Methodology + ")"
	}
	if text := strings.TrimSpace(t.Text); text != "" {
		body += " " + text
	}
	if len(t.Requirements) > 0 {
		body += " " + CitationSeparator + " " + strings.Join(t.Requirements, ", ")
	}
	out := wrap(head, strings.Repeat(" ", len(head)), body, LineWidth)
	out = append(out, t.detail...)
	return append(out, renderFlags(t)...)
}

// wrap breaks text into lines that fit width, with prefix on the first and indent on
// the rest. A word longer than the budget goes on its own line rather than being
// broken: the artifacts are full of paths and backticked identifiers, and a broken
// one stops being a name.
func wrap(prefix, indent, text string, width int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{strings.TrimRight(prefix, " ")}
	}
	var out []string
	cur := prefix + words[0]
	for _, w := range words[1:] {
		if len([]rune(cur))+1+len([]rune(w)) > width {
			out = append(out, cur)
			cur = indent + w
			continue
		}
		cur += " " + w
	}
	return append(out, cur)
}

// blockLines splits caller-supplied text into lines, normalizing the line endings a
// shell on Windows may have handed over.
func blockLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

// trimBlank is the last non-blank line in [from, to], or from-1 when there is none.
func trimBlank(lines []string, from, to int) int {
	if to > len(lines) {
		to = len(lines)
	}
	for n := to; n >= from; n-- {
		if strings.TrimSpace(lines[n-1]) != "" {
			return n
		}
	}
	return from - 1
}
