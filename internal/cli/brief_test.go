package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// schedulePlan exercises every answer --next has to have: an eligible task, a blocked
// one, a removed one, and a priority that beats file order.
const schedulePlan = `---
autonomy: auto
ci: wait
---

# Sweep

Replace the legacy path, one group at a time.

## Why

The old path cannot be extended without a rewrite.

## References

- ` + "`specs/cart-totals/`" + ` — the totals engine

## Out of scope

- the payment provider itself

## Tasks

- [x] 1.1 (Unit) Lay the foundation
- [ ] 1.2 (TDD) Build on it
  _Depends 1.1_
- [ ] 1.3 (Unit) Wait for 1.2
  _Depends 1.2_
- [ ] 1.4 (Unit) Dropped after the fact
  _Status removed_
  _Reason the upstream API made it unnecessary_
- [ ] 2.1 (Unit) The urgent one
  _Priority 1_

## Done when

- the legacy path is gone
`

// The guarantee that gives "never read the plan" its authority: brief is the header,
// tasks is the checklist, and neither returns the other.
func TestBriefReturnsTheHeaderAndNoTasks(t *testing.T) {
	root := initWorkspace(t)
	writePlanFile(t, root, "sweep", schedulePlan)

	stdout, stderr, code := run(t, "map", "brief", "sweep", "--root", root)
	if code != ExitOK {
		t.Fatalf("brief = %d (%s)", code, stderr)
	}
	out := stdout + stderr
	for _, want := range []string{"Sweep", "Replace the legacy path", "Why", "cart-totals",
		"payment provider", "the legacy path is gone"} {
		if !strings.Contains(out, want) {
			t.Errorf("brief is missing %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"Lay the foundation", "Build on it", "[ ]", "[x]"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("brief returned a task (%q); brief is the header only:\n%s", unwanted, out)
		}
	}
	if !strings.Contains(out, "1 done") || !strings.Contains(out, "1 removed") {
		t.Errorf("brief does not count the checklist:\n%s", out)
	}
}

func TestBriefJSONCarriesTheCounts(t *testing.T) {
	root := initWorkspace(t)
	writePlanFile(t, root, "sweep", schedulePlan)
	stdout, _, code := run(t, "map", "brief", "sweep", "--json", "--root", root)
	if code != ExitOK {
		t.Fatalf("brief --json = %d", code)
	}
	var got brief
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("brief --json is not JSON: %v\n%s", err, stdout)
	}
	if got.Tasks.Done != 1 || got.Tasks.Removed != 1 || got.Tasks.Blocked != 1 || got.Tasks.Ready != 2 {
		t.Errorf("counts = %+v", got.Tasks)
	}
	if len(got.Leaves) != 1 {
		t.Errorf("leaves = %v, want the one spec reference", got.Leaves)
	}
	for _, s := range got.Sections {
		if s.Slug == "tasks" {
			t.Error("brief returned the tasks section")
		}
	}
}

// Priority beats position, and a dependency beats both.
func TestNextIsDeterministic(t *testing.T) {
	root := initWorkspace(t)
	writePlanFile(t, root, "sweep", schedulePlan)

	stdout, _, code := run(t, "map", "tasks", "sweep", "--next", "--json", "--root", root)
	if code != ExitOK {
		t.Fatalf("--next = %d", code)
	}
	var got struct {
		Task *struct {
			Number   string   `json:"number"`
			Depends  []string `json:"depends"`
			Priority *int     `json:"priority"`
			Eligible bool     `json:"eligible"`
		} `json:"task"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, stdout)
	}
	if got.Task == nil || got.Task.Number != "2.1" {
		t.Fatalf("--next = %+v, want 2.1 (priority 1 outranks 1.2's file position)", got.Task)
	}
	if !got.Task.Eligible {
		t.Error("--next returned a task it does not consider eligible")
	}
}

func TestReadyBlockedAndDeps(t *testing.T) {
	root := initWorkspace(t)
	writePlanFile(t, root, "sweep", schedulePlan)

	stdout, _, _ := run(t, "map", "tasks", "sweep", "--ready", "--json", "--root", root)
	var ready struct {
		Tasks []struct {
			Number string `json:"number"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal([]byte(stdout), &ready); err != nil {
		t.Fatal(err)
	}
	if len(ready.Tasks) != 2 || ready.Tasks[0].Number != "2.1" || ready.Tasks[1].Number != "1.2" {
		t.Errorf("--ready = %+v, want 2.1 then 1.2", ready.Tasks)
	}

	stdout, _, _ = run(t, "map", "tasks", "sweep", "--blocked", "--json", "--root", root)
	var blocked struct {
		Blocked []blockedRow `json:"blocked"`
	}
	if err := json.Unmarshal([]byte(stdout), &blocked); err != nil {
		t.Fatal(err)
	}
	if len(blocked.Blocked) != 1 || blocked.Blocked[0].Number != "1.3" {
		t.Fatalf("--blocked = %+v, want just 1.3", blocked.Blocked)
	}
	if len(blocked.Blocked[0].WaitingOn) != 1 || blocked.Blocked[0].WaitingOn[0] != "1.2" {
		t.Errorf("an impasse that cannot name its blocker is not actionable: %+v", blocked.Blocked[0])
	}

	stdout, _, _ = run(t, "map", "tasks", "sweep", "--deps", "--root", root)
	if !strings.Contains(stdout, "1.3") || !strings.Contains(stdout, "←") {
		stdout, stderr, _ := run(t, "map", "tasks", "sweep", "--deps", "--root", root)
		t.Errorf("--deps printed no edges:\n%s%s", stdout, stderr)
	}
}

// The two answers a loop must be able to tell apart: finished, and stuck.
func TestNextSaysWhyThereIsNothingToDo(t *testing.T) {
	root := initWorkspace(t)
	writePlanFile(t, root, "done", plan2(`- [x] 1.1 (Unit) Only task`))
	stdout, _, code := run(t, "map", "tasks", "done", "--next", "--json", "--root", root)
	if code != ExitOK {
		t.Fatalf("--next on a finished plan = %d", code)
	}
	if !strings.Contains(stdout, `"done": true`) {
		t.Errorf("a finished plan must say so:\n%s", stdout)
	}

	writePlanFile(t, root, "stuck", plan2(
		"- [ ] 1.1 (Unit) One\n  _Depends 1.2_\n- [ ] 1.2 (Unit) Two\n  _Depends 1.3_\n- [ ] 1.3 (Unit) Three\n  _Depends 1.1_"))
	if _, _, code := run(t, "map", "tasks", "stuck", "--next", "--root", root); code != ExitFindings {
		t.Errorf("a dependency cycle = %d, want %d — a loop cannot recover from it silently", code, ExitFindings)
	}
}

// A removed task is not work: out of every listing except the one that asks for it.
func TestRemovedTasksAreOutOfTheListings(t *testing.T) {
	root := initWorkspace(t)
	writePlanFile(t, root, "sweep", schedulePlan)
	stdout, stderr, _ := run(t, "map", "tasks", "sweep", "--open", "--root", root)
	if strings.Contains(stdout+stderr, "Dropped after the fact") {
		t.Error("a removed task appeared in --open")
	}
	stdout, stderr, _ = run(t, "map", "tasks", "sweep", "--removed", "--root", root)
	out := stdout + stderr
	if !strings.Contains(out, "Dropped after the fact") || !strings.Contains(out, "upstream API") {
		t.Errorf("--removed did not report the struck-out task with its reason:\n%s", out)
	}
}

// plan2 wraps a checklist in the sections the contract requires.
func plan2(tasks string) string {
	return "# Sweep\n\nWhat this work is.\n\n## Why\n\nBecause.\n\n## Tasks\n\n" +
		tasks + "\n\n## Done when\n\n- it is done\n"
}
