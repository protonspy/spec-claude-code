package artifact

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// flaggedPlan carries every shape the flag block can take: the canonical indented
// form, the column-0 form a person writes by hand, a removal, and a dependency
// nothing satisfies.
const flaggedPlan = `---
autonomy: auto
ci: wait
---

# Flagged

## Why

One paragraph.

## Tasks

- [x] 1.1 (Unit) Lay the foundation
- [ ] 1.2 (TDD) Build on it, with a description that runs onto a second line so the
      continuation is exercised too
  _Depends 1.1_
  _Priority 2_
- [ ] 1.3 (Unit) Wait for something that is not done

_Depends 1.2_

- [ ] 1.4 (Unit) Dropped after the fact
  _Status removed_
  _Reason the upstream API made it unnecessary_

## Done when

- everything above is ticked
`

func loadFlagged(t *testing.T) *Artifact {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "plans", "flagged.md")
	if err := os.WriteFile(path, []byte(flaggedPlan), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := Load(root, path)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func task(t *testing.T, a *Artifact, number string) Task {
	t.Helper()
	got, ok := a.Task(number)
	if !ok {
		t.Fatalf("no task %s", number)
	}
	return got
}

func TestFlagsParseInBothIndentations(t *testing.T) {
	a := loadFlagged(t)

	two := task(t, a, "1.2")
	if len(two.Depends) != 1 || two.Depends[0] != "1.1" {
		t.Fatalf("1.2 depends = %v, want [1.1]", two.Depends)
	}
	if two.Priority == nil || *two.Priority != 2 {
		t.Fatalf("1.2 priority = %v, want 2", two.Priority)
	}

	// Column 0, one blank line above it: the form the hand-written example uses.
	three := task(t, a, "1.3")
	if len(three.Depends) != 1 || three.Depends[0] != "1.2" {
		t.Fatalf("1.3 depends = %v, want [1.2]", three.Depends)
	}
}

// The flags have to be inside the task's region, or every command that addresses a
// task by number addresses only part of it: `show` hides the dependency, `rm` leaves
// it behind pointing at nothing, and the rollback cannot restore what it never saw.
func TestTaskRegionCoversItsFlags(t *testing.T) {
	a := loadFlagged(t)
	for _, tc := range []struct{ number, want string }{
		{"1.2", "_Priority 2_"},
		{"1.3", "_Depends 1.2_"},
		{"1.4", "_Reason the upstream API made it unnecessary_"},
	} {
		got := task(t, a, tc.number)
		text := a.Text(got.Line, got.End)
		if !strings.Contains(text, tc.want) {
			t.Errorf("task %s region %q does not carry %q", tc.number, text, tc.want)
		}
	}
}

// A flag is metadata, not description. Leaving `_Priority 2_` in Detail would index
// it as prose in the searcher and spend the caller's --width budget on it.
func TestDescriptionExcludesFlags(t *testing.T) {
	a := loadFlagged(t)
	for _, number := range []string{"1.2", "1.3", "1.4"} {
		if d := task(t, a, number).Detail; strings.Contains(d, "_") {
			t.Errorf("task %s detail carries a flag line: %q", number, d)
		}
	}
	if d := task(t, a, "1.2").Detail; !strings.Contains(d, "continuation is exercised") {
		t.Errorf("task 1.2 lost its continuation: %q", d)
	}
}

func TestRemovedTaskIsNeitherOpenNorDone(t *testing.T) {
	a := loadFlagged(t)
	four := task(t, a, "1.4")
	if !four.Removed() {
		t.Fatal("1.4 is not removed")
	}
	if four.Eligible || four.Blocked {
		t.Errorf("1.4 eligible=%v blocked=%v; a removed task is neither", four.Eligible, four.Blocked)
	}
	done, total := a.Done()
	if done != 1 || total != 3 {
		t.Errorf("Done() = %d/%d, want 1/3 — the removed task is in neither number", done, total)
	}
	if c := a.Counts(); c.Removed != 1 || c.Ready != 1 || c.Blocked != 1 || c.Done != 1 {
		t.Errorf("counts = %+v", c)
	}
}

func TestEligibilityFollowsDependencies(t *testing.T) {
	a := loadFlagged(t)
	if !task(t, a, "1.2").Eligible {
		t.Error("1.2 depends on 1.1, which is done, so it is eligible")
	}
	if !task(t, a, "1.3").Blocked {
		t.Error("1.3 depends on 1.2, which is open, so it is blocked")
	}
	next, ok := a.Next()
	if !ok || next.Number != "1.2" {
		t.Fatalf("next = %v %q, want 1.2", ok, next.Number)
	}
	if waiting := a.WaitingOn(task(t, a, "1.3")); len(waiting) != 1 || waiting[0] != "1.2" {
		t.Errorf("1.3 waiting on %v, want [1.2]", waiting)
	}
}

// R1: the failure this whole phase exists to prevent. Rewriting one field of a task
// must not take its dependencies with it.
func TestRewritingATaskKeepsItsFlagsAndContinuation(t *testing.T) {
	a := loadFlagged(t)
	method := "Unit"
	e := a.Edit()
	e.SetTask("1.2", TaskEdit{Methodology: &method})
	out, err := e.Content()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"_Depends 1.1_", "_Priority 2_", "continuation is exercised"} {
		if !strings.Contains(out, want) {
			t.Errorf("rewriting 1.2 lost %q", want)
		}
	}
}

// parse(render(parse(x))) == parse(x): patching twice must not produce a diff the
// second time, which is what makes a canonical form worth having.
func TestRenderingATaskIsIdempotent(t *testing.T) {
	a := loadFlagged(t)
	root := filepath.Dir(filepath.Dir(a.Abs))

	rewrite := func(in *Artifact) *Artifact {
		t.Helper()
		e := in.Edit()
		for _, task := range in.Tasks {
			text := task.Text
			e.SetTask(task.Number, TaskEdit{Text: &text})
		}
		content, err := e.Content()
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, "plans", "flagged.md")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		next, err := Load(root, path)
		if err != nil {
			t.Fatal(err)
		}
		return next
	}

	once := rewrite(a)
	first := once.Text(1, once.LineCount())
	twice := rewrite(once)
	if second := twice.Text(1, twice.LineCount()); second != first {
		t.Errorf("a second rewrite changed the file:\n--- first\n%s\n--- second\n%s", first, second)
	}
}

func TestStrikeTaskKeepsTheLineAndTheNumber(t *testing.T) {
	a := loadFlagged(t)
	e := a.Edit()
	e.StrikeTask("1.3", "the requirement was withdrawn")
	out, err := e.Content()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "1.3 (Unit) Wait for something") {
		t.Error("striking 1.3 deleted it; a removed task keeps its line")
	}
	if !strings.Contains(out, "_Status removed_") || !strings.Contains(out, "_Reason the requirement was withdrawn_") {
		t.Errorf("striking 1.3 did not record the removal:\n%s", out)
	}
	if e := a.Edit(); func() error { e.StrikeTask("1.3", "  "); return e.Err() }() == nil {
		t.Error("a removal with no reason should be refused")
	}
}

func TestHighWaterCountsRemovedTasks(t *testing.T) {
	a := loadFlagged(t)
	if got := a.HighWater("1"); got != 4 {
		t.Errorf("HighWater(1) = %d, want 4 — the removed 1.4 still holds its number", got)
	}
	if got := a.HighGroup(); got != 1 {
		t.Errorf("HighGroup() = %d, want 1", got)
	}
}

// 1.10 comes after 1.9. Sorted as text it does not, which is what made the loop
// hand out the tenth task before the ninth as soon as a group grew.
func TestNumbersCompareNumerically(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"1.9", "1.10", -1},
		{"1.10", "1.9", 1},
		{"2.1", "10.1", -1},
		{"1.2", "1.2", 0},
		{"1", "1.1", -1},
	} {
		if got := CompareNumbers(tc.a, tc.b); got != tc.want {
			t.Errorf("CompareNumbers(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestPriorityOrdersBeforeNumber(t *testing.T) {
	tasks := []Task{
		{Number: "1.9", Line: 3},
		{Number: "1.10", Line: 4},
		{Number: "2.1", Line: 5, Priority: ptr(1)},
	}
	SortTasks(tasks)
	want := []string{"2.1", "1.9", "1.10"}
	for i, t2 := range tasks {
		if t2.Number != want[i] {
			t.Fatalf("order = %v, want %v", numbersOf(tasks), want)
		}
	}
}

func TestDependencyCycleIsFound(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "plans", "cyclic.md")
	content := `# Cyclic

## Tasks

- [ ] 1.1 (Unit) One
  _Depends 1.2_
- [ ] 1.2 (Unit) Two
  _Depends 1.1_
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := Load(root, path)
	if err != nil {
		t.Fatal(err)
	}
	cycles := a.DependencyCycles()
	if len(cycles) != 1 {
		t.Fatalf("cycles = %v, want one", cycles)
	}
	if cycles[0][0] != "1.1" {
		t.Errorf("cycle = %v, want it to start at its smallest member", cycles[0])
	}
	if task(t, a, "1.1").Eligible || task(t, a, "1.2").Eligible {
		t.Error("neither task on a cycle can be eligible")
	}
}

func TestUnknownItalicLineIsReportedNotSwallowed(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "plans", "typo.md")
	content := `# Typo

## Tasks

- [ ] 1.1 (Unit) One
  _Depend 1.0_
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := Load(root, path)
	if err != nil {
		t.Fatal(err)
	}
	one := task(t, a, "1.1")
	if len(one.UnknownFlags) != 1 || one.UnknownFlags[0].Name != "Depend" {
		t.Fatalf("unknown flags = %+v, want the typo reported", one.UnknownFlags)
	}
	if len(one.Depends) != 0 {
		t.Errorf("a flag nobody defined must not be read as one: %v", one.Depends)
	}
}

func ptr(n int) *int { return &n }

func numbersOf(tasks []Task) []string {
	out := make([]string, len(tasks))
	for i, t := range tasks {
		out[i] = t.Number
	}
	return out
}
