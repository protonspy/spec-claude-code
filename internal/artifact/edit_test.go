package artifact

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func edited(t *testing.T, apply func(*Editor)) (*Artifact, string) {
	t.Helper()
	a, _ := load(t)
	e := a.Edit()
	apply(e)
	content, err := e.Content()
	if err != nil {
		t.Fatalf("Content: %v", err)
	}
	return a, content
}

// Flipping a box must touch the box and nothing else. An edit that reflowed the line
// while checking it would put a diff in front of a reviewer that hides what changed.
func TestCheckTouchesOnlyTheBox(t *testing.T) {
	a, content := edited(t, func(e *Editor) { e.Check("1.2", true) })
	before, after := strings.Split(samplePlan, "\n"), strings.Split(content, "\n")
	if len(before) != len(after) {
		t.Fatalf("line count changed: %d → %d", len(before), len(after))
	}
	changed := 0
	for i := range before {
		if before[i] != after[i] {
			changed++
			if !strings.Contains(after[i], "[x] 1.2") {
				t.Errorf("line %d changed to %q, want the box flipped", i+1, after[i])
			}
		}
	}
	if changed != 1 {
		t.Errorf("%d lines changed, want exactly 1", changed)
	}
	_ = a
}

// Checking a task that is already checked is not an edit and not an error either.
// A loop that re-runs the same step must be able to say so without failing.
func TestCheckIsIdempotent(t *testing.T) {
	a, _ := load(t)
	e := a.Edit()
	e.Check("1.1", true)
	if !e.Empty() {
		t.Error("checking an already-checked task produced an edit")
	}
	if e.Err() != nil {
		t.Errorf("and reported an error: %v", e.Err())
	}
}

// Unchecking and re-checking must restore the file byte for byte. Anything less
// means every no-op pass of a loop leaves a diff behind.
func TestCheckRoundTripsExactly(t *testing.T) {
	a, root := load(t)
	e := a.Edit()
	e.Check("1.1", false)
	once, err := e.Content()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeAt(a.Abs, once); err != nil {
		t.Fatal(err)
	}

	b, err := Load(root, a.Abs)
	if err != nil {
		t.Fatal(err)
	}
	e2 := b.Edit()
	e2.Check("1.1", true)
	twice, err := e2.Content()
	if err != nil {
		t.Fatal(err)
	}
	if twice != samplePlan {
		t.Errorf("round trip did not restore the file:\n%q", twice)
	}
}

// An address that does not resolve is an error, never an insert at a guess. This is
// the property that lets a caller edit a file it has not read.
func TestUnknownAddressIsAnError(t *testing.T) {
	a, _ := load(t)
	for name, apply := range map[string]func(*Editor){
		"check":   func(e *Editor) { e.Check("9.9", true) },
		"set":     func(e *Editor) { e.SetTask("9.9", TaskEdit{}) },
		"remove":  func(e *Editor) { e.RemoveTask("9.9") },
		"append":  func(e *Editor) { e.Append("#nope", "x") },
		"replace": func(e *Editor) { e.Replace("#nope", "x") },
		"add":     func(e *Editor) { e.AddTask(NewTask{Section: "nope", Number: "3.1", Text: "x"}) },
	} {
		e := a.Edit()
		apply(e)
		if e.Err() == nil {
			t.Errorf("%s against a missing address succeeded", name)
		}
		if _, err := e.Content(); err == nil {
			t.Errorf("%s: Content() succeeded after a failed edit", name)
		}
	}
}

// A task's description lives mostly in its continuation, so rewriting one must
// replace the whole block. Leaving the tail would describe the old task.
func TestSetTaskReplacesTheWholeBlock(t *testing.T) {
	text := "Rewritten"
	_, content := edited(t, func(e *Editor) { e.SetTask("1.1", TaskEdit{Text: &text}) })
	if strings.Contains(content, "vendor the module") {
		t.Error("the old continuation survived the rewrite")
	}
	if !strings.Contains(content, "- [x] 1.1 (Unit) Rewritten") {
		t.Errorf("rewritten task not found:\n%s", content)
	}
}

func TestAddTaskLandsAfterTheLastTaskInItsSection(t *testing.T) {
	_, content := edited(t, func(e *Editor) {
		e.AddTask(NewTask{Section: "tasks", Number: "1.3", Methodology: "Unit", Text: "A third thing"})
	})
	lines := strings.Split(content, "\n")
	last, added, notes := -1, -1, -1
	for i, l := range lines {
		switch {
		case strings.Contains(l, "1.2 (TDD)"):
			last = i
		case strings.Contains(l, "1.3 (Unit)"):
			added = i
		case strings.HasPrefix(l, "## Notes"):
			notes = i
		}
	}
	if added < 0 {
		t.Fatalf("task not added:\n%s", content)
	}
	if !(added > last && added < notes) {
		t.Errorf("1.3 landed at %d; want between task 1.2 (%d) and ## Notes (%d)", added, last, notes)
	}
}

func TestAddTaskRefusesADuplicateNumber(t *testing.T) {
	a, _ := load(t)
	e := a.Edit()
	e.AddTask(NewTask{Section: "tasks", Number: "1.1", Text: "collides"})
	if e.Err() == nil {
		t.Error("adding a task with a taken number succeeded")
	}
}

// Replacing a section replaces what it says, never its heading: removing the heading
// would move every following section up a level and silently re-parent the file.
func TestReplaceSectionKeepsTheHeading(t *testing.T) {
	_, content := edited(t, func(e *Editor) { e.Replace("#why", "Something else entirely.") })
	if !strings.Contains(content, "## Why") {
		t.Error("the heading was removed")
	}
	if strings.Contains(content, "what done means") {
		t.Error("the old body survived")
	}
	if !strings.Contains(content, "Something else entirely.") {
		t.Error("the new body is missing")
	}
}

// Appending to a section with subsections must land in that section's own body, not
// inside whichever subsection happens to be last.
func TestAppendToSectionStopsAtTheFirstSubsection(t *testing.T) {
	const nested = `# T

## Outer

Intro.

### Inner

Inner body.

## After

Tail.
`
	a := parseString(t, nested)
	e := a.Edit()
	e.Append("#outer", "Appended.")
	content, err := e.Content()
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(content, "\n")
	appended, inner := -1, -1
	for i, l := range lines {
		if strings.HasPrefix(l, "Appended.") {
			appended = i
		}
		if strings.HasPrefix(l, "### Inner") {
			inner = i
		}
	}
	if appended < 0 || appended > inner {
		t.Errorf("appended at %d, want above ### Inner at %d:\n%s", appended, inner, content)
	}
}

func TestSetFrontmatterReplacesAndAdds(t *testing.T) {
	_, content := edited(t, func(e *Editor) {
		e.SetFrontmatter("ci", "no-wait")
		e.SetFrontmatter("pr", "per-plan")
	})
	if !strings.Contains(content, "ci: no-wait") || strings.Contains(content, "ci: wait\n") {
		t.Errorf("existing key not replaced:\n%s", firstLines(content, 8))
	}
	if !strings.Contains(content, "pr: per-plan") {
		t.Errorf("new key not added:\n%s", firstLines(content, 8))
	}
}

// Two edits to the same region are a mistake rather than a merge. Silently applying
// both would produce a file neither caller asked for.
func TestOverlappingEditsAreRefused(t *testing.T) {
	a, _ := load(t)
	e := a.Edit()
	text := "one"
	e.SetTask("1.1", TaskEdit{Text: &text})
	e.RemoveTask("1.1")
	if _, err := e.Content(); err == nil {
		t.Error("two edits to the same task were applied")
	}
}

// Every address is resolved against the original file, and splices apply bottom-up,
// so a batch cannot invalidate its own line numbers halfway through.
func TestBatchedEditsDoNotShiftEachOther(t *testing.T) {
	_, content := edited(t, func(e *Editor) {
		e.Check("1.2", true)
		e.Append("#why", "A second paragraph.")
		e.SetFrontmatter("pr", "per-group")
	})
	for _, want := range []string{"[x] 1.2", "A second paragraph.", "pr: per-group"} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q from the batch:\n%s", want, content)
		}
	}
	if !strings.Contains(content, "## Decomposition") {
		t.Error("the batch corrupted the document structure")
	}
}

func parseString(t *testing.T, content string) *Artifact {
	t.Helper()
	root := t.TempDir()
	path := root + "/plans/x.md"
	if err := writeAt(path, content); err != nil {
		t.Fatal(err)
	}
	a, err := Load(root, path)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

// writeAt is the test-local file writer; the package's own writes go through
// workspace.AtomicWrite from the CLI, which is not what these tests are exercising.
func writeAt(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// Several keys written in one call must come out in the order the caller asked for.
// Splices apply bottom-up, so insertions sharing one position have to be reversed on
// the way in or the file reads backwards.
func TestSetFrontmatterKeepsTheOrderAsked(t *testing.T) {
	_, content := edited(t, func(e *Editor) {
		e.SetFrontmatter("pr", "per-plan")
		e.SetFrontmatter("worktree", "per-group")
		e.SetFrontmatter("merge", "auto")
	})
	pr := strings.Index(content, "pr: per-plan")
	wt := strings.Index(content, "worktree: per-group")
	mg := strings.Index(content, "merge: auto")
	if pr < 0 || wt < 0 || mg < 0 {
		t.Fatalf("a key is missing:\n%s", firstLines(content, 10))
	}
	if !(pr < wt && wt < mg) {
		t.Errorf("keys came out reversed:\n%s", firstLines(content, 10))
	}
}

// Writing the value a key already has is not an edit. A resumed loop restates every
// answer, and reporting the unchanged ones would bury the one that moved.
func TestSetFrontmatterIsIdempotent(t *testing.T) {
	a, _ := load(t)
	e := a.Edit()
	e.SetFrontmatter("autonomy", "auto")
	e.SetFrontmatter("ci", "wait")
	if !e.Empty() {
		t.Errorf("re-writing the existing values produced %d changes", len(e.Changes()))
	}
}
