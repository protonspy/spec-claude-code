package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mapPlan is a plan with everything the commands address: a decomposition of leaves,
// a task list whose items run past their checkbox line, and a Notes section with no
// headings inside it — which is the shape that makes paragraph addressing necessary.
const mapPlan = `---
autonomy: auto
ci: wait
---

# Sample plan

## Why

What this is for, and what done means for the whole of it.

## Decomposition

- ` + "`specs/thing/`" + ` — the leaf that carries most of the work, described over
  two lines because that is what leaves do.

## Tasks

- [x] 1.1 (Unit) Build the parser, and prove with a test that it reads a fenced
      block without treating the example inside it as a task
- [ ] 1.2 (TDD) Guard the credential before the provider client lands
      so the secret is never the thing a first run discovers is missing

## Notes

**Order matters.** The parser first, because everything downstream reads through it.

**The free path wins.** The expensive command knows about the cheap one, never the
other way round.
`

// mapWorkspace scaffolds a real workspace and drops the plan into it. init is used
// rather than a hand-made directory so the marker file the workspace walk needs is
// the one the product actually writes.
func mapWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if _, _, code := run(t, "init", "--root", root); code != ExitOK {
		t.Fatalf("init: exit %d", code)
	}
	dir := filepath.Join(root, "plans")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sample.md"), []byte(mapPlan), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func planText(t *testing.T, root string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "plans", "sample.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The outline reports structure, so its size must track the structure and not the
// prose. That is the property that makes it worth running: measured on a real
// 56KB plan the outline came to 878 bytes, and it would come to about the same on a
// plan twice as wordy. A ratio on a small fixture proves nothing; this does.
func TestMapOutlineDoesNotGrowWithTheProse(t *testing.T) {
	root := mapWorkspace(t)
	lean, stderr, code := run(t, "map", "sample", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit %d (stderr: %s)", code, stderr)
	}
	for _, want := range []string{"Decomposition", "Tasks", "Notes", "1/2 tasks"} {
		if !strings.Contains(lean, want) {
			t.Errorf("outline missing %q:\n%s", want, lean)
		}
	}
	if strings.Contains(lean, "never the") {
		t.Error("the outline printed the prose it exists to replace")
	}

	// Same structure, an order of magnitude more words under the last heading.
	fat := mapPlan + strings.Repeat("\nMore prose that decides nothing and is here to be skipped.\n", 200)
	if err := os.WriteFile(filepath.Join(root, "plans", "sample.md"), []byte(fat), 0o644); err != nil {
		t.Fatal(err)
	}
	grown, _, code := run(t, "map", "sample", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if len(fat) < 10*len(mapPlan) {
		t.Fatalf("fixture is not much bigger: %d vs %d", len(fat), len(mapPlan))
	}
	// The one legitimate growth is the line count and the paragraph tally in the
	// report, which is a handful of characters — never a multiple.
	if len(grown) > len(lean)+120 {
		t.Errorf("outline grew from %d to %d bytes while the file grew %dx — it is summarizing prose",
			len(lean), len(grown), len(fat)/len(mapPlan))
	}
}

// --next means the next task to work on. Answering with the first task in the file,
// done or not, answers a question nobody asked.
func TestMapTasksNextIsTheFirstOpenOne(t *testing.T) {
	root := mapWorkspace(t)
	stdout, _, code := run(t, "map", "tasks", "sample", "--next", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout, "1.2") {
		t.Errorf("--next = %q, want task 1.2", stdout)
	}
	if strings.Contains(stdout, "1.1") {
		t.Error("--next returned a task that is already done")
	}
}

// Every task in a real plan runs past one line, and the line below the checkbox is
// where the decision usually sits. A listing clips it because a listing is a list;
// --next is one task, and clipping there sends the reader to the file — the exact
// cost `map` exists to remove.
func TestMapTasksNextPrintsTheWholeDescription(t *testing.T) {
	root := mapWorkspace(t)
	stdout, _, code := run(t, "map", "tasks", "sample", "--next", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout, "so the secret is never the thing a first run discovers is missing") {
		t.Errorf("--next dropped the continuation line:\n%s", stdout)
	}

	// And the listing still clips, or the reading surface has no cheap view left.
	list, _, code := run(t, "map", "tasks", "sample", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if strings.Contains(list, "so the secret is never") {
		t.Errorf("the listing printed a continuation:\n%s", list)
	}
}

func TestMapShowReturnsOnlyThatPiece(t *testing.T) {
	root := mapWorkspace(t)
	stdout, stderr, code := run(t, "map", "show", "sample", "notes:1", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit %d (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "Order matters") {
		t.Errorf("did not return the addressed paragraph:\n%s", stdout)
	}
	if strings.Contains(stdout, "The free path wins") {
		t.Error("returned the next paragraph too")
	}
	if strings.Contains(stdout, "## Decomposition") {
		t.Error("returned the rest of the file")
	}
}

// An address that misses must fail rather than return something adjacent, and must
// say what the file does have — the caller cannot see it.
func TestMapShowOnAMissNamesWhatExists(t *testing.T) {
	root := mapWorkspace(t)
	stdout, stderr, code := run(t, "map", "show", "sample", "9.9", "--root", root)
	if code != ExitError {
		t.Errorf("exit = %d, want %d", code, ExitError)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty on a miss", stdout)
	}
	if !strings.Contains(stderr, "1.1") {
		t.Errorf("stderr does not list the tasks that exist: %q", stderr)
	}
}

func TestMapFindReturnsAddresses(t *testing.T) {
	root := mapWorkspace(t)
	stdout, stderr, code := run(t, "map", "find", "credential provider", "--root", root, "--json")
	if code != ExitOK {
		t.Fatalf("exit %d (stderr: %s)", code, stderr)
	}
	var got struct {
		Hits []struct {
			Ref  string `json:"ref"`
			Kind string `json:"kind"`
			Line int    `json:"line"`
		} `json:"hits"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("not JSON (%v): %s", err, stdout)
	}
	if len(got.Hits) == 0 {
		t.Fatal("no hits")
	}
	if got.Hits[0].Ref != "1.2" {
		t.Errorf("top hit = %q, want the task that mentions both terms", got.Hits[0].Ref)
	}
	if got.Hits[0].Line < 1 {
		t.Error("a hit with no line is not an address")
	}
}

func TestPatchCheckWritesAndReValidates(t *testing.T) {
	root := mapWorkspace(t)
	_, stderr, code := run(t, "patch", "check", "sample", "1.2", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit %d (stderr: %s)", code, stderr)
	}
	if !strings.Contains(planText(t, root), "- [x] 1.2") {
		t.Error("the box was not flipped on disk")
	}
}

// The guarantee that replaces reading the file first: a change that introduces a
// finding is undone, reported, and exits 2 — never left on disk.
func TestPatchRollsBackAChangeThatIntroducesAFinding(t *testing.T) {
	root := mapWorkspace(t)
	before := planText(t, root)

	stdout, stderr, code := run(t, "patch", "add", "sample",
		"--section", "tasks", "--number", "1.3", "--method", "Unit",
		"--text", "Port the queue out of specs/thing/ into the runner", "--root", root)
	if code != ExitFindings {
		t.Errorf("exit = %d, want %d (stdout: %s stderr: %s)", code, ExitFindings, stdout, stderr)
	}
	if got := planText(t, root); got != before {
		t.Errorf("the file was left changed after a rollback:\n%s", got)
	}
	if !strings.Contains(stderr, "rolled back") {
		t.Errorf("stderr does not say it rolled back: %q", stderr)
	}
	if !strings.Contains(stderr, "item-has-two-records") {
		t.Errorf("stderr does not name the rule that fired: %q", stderr)
	}
}

// A pre-existing finding is not this edit's fault. Comparing whole sets rather than
// what the edit introduced would make every patch to an already-imperfect file fail.
func TestPatchIgnoresFindingsTheFileAlreadyHad(t *testing.T) {
	root := mapWorkspace(t)
	// specs/thing/ does not exist, so the plan already reports plan.unknown-spec.
	if _, _, code := run(t, "plan", "validate", "--root", root); code != ExitFindings {
		t.Fatal("this test needs a plan that already has findings")
	}
	if _, stderr, code := run(t, "patch", "check", "sample", "1.2", "--root", root); code != ExitOK {
		t.Errorf("exit = %d, want %d — a pre-existing finding blocked an unrelated edit (%s)",
			code, ExitOK, stderr)
	}
}

// An address is a name, not a span: `replace #notes` reads as a small edit and can
// resolve to most of the file. Since nobody read the file first, the scale of the
// deletion is the one thing the caller cannot have known.
func TestPatchRefusesALargeDeletionWithoutForce(t *testing.T) {
	root := mapWorkspace(t)
	path := writeBigPlan(t, root)
	before := readFile(t, path)

	_, stderr, code := run(t, "patch", "replace", "big", "#notes", "--text", "gone", "--root", root)
	if code != ExitError {
		t.Errorf("exit = %d, want %d", code, ExitError)
	}
	if readFile(t, path) != before {
		t.Error("the file was changed despite the refusal")
	}
	if !strings.Contains(stderr, "--force") {
		t.Errorf("the refusal does not say how to proceed deliberately: %q", stderr)
	}

	// --force is the separate decision, and it must actually work: a guard with no
	// way past it is a guard that gets worked around with a text editor.
	if _, stderr, code := run(t, "patch", "replace", "big", "#notes", "--text", "gone",
		"--force", "--root", root); code != ExitOK {
		t.Errorf("--force: exit = %d, want %d (%s)", code, ExitOK, stderr)
	}
	if after := readFile(t, path); !strings.Contains(after, "gone") || strings.Contains(after, "A line of prose") {
		t.Error("--force did not apply the replacement")
	}
}

// writeBigPlan drops in a plan whose one section is far past the deletion threshold.
func writeBigPlan(t *testing.T, root string) string {
	t.Helper()
	body := strings.Repeat("A line of prose that is here to make the section long.\n", 40)
	path := filepath.Join(root, "plans", "big.md")
	if err := os.WriteFile(path, []byte("# Big\n\n## Notes\n\n"+body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// And the refusal must not echo what it declined to delete: a confirmation that
// printed four hundred displaced lines would put the file in the caller's context by
// the back door, while reporting that it refused to touch it.
func TestPatchRefusalDoesNotEchoTheWholeRegion(t *testing.T) {
	root := mapWorkspace(t)
	writeBigPlan(t, root)
	stdout, stderr, code := run(t, "patch", "replace", "big", "#notes", "--text", "gone", "--root", root)
	if code != ExitError {
		t.Fatalf("exit = %d, want %d", code, ExitError)
	}
	if n := strings.Count(stdout+stderr, "A line of prose"); n > 8 {
		t.Errorf("the refusal echoed %d displaced lines; it must elide them", n)
	}
}

func TestPatchDryRunWritesNothing(t *testing.T) {
	root := mapWorkspace(t)
	before := planText(t, root)
	stdout, _, code := run(t, "patch", "check", "sample", "1.2", "--dry-run", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if got := planText(t, root); got != before {
		t.Error("--dry-run wrote to the file")
	}
	if !strings.Contains(stdout, "1.2") {
		t.Errorf("--dry-run did not show the change: %q", stdout)
	}
}

// Checking an already-checked task is a loop re-running a step it finished. It is
// not an edit and not an error.
func TestPatchCheckIsIdempotent(t *testing.T) {
	root := mapWorkspace(t)
	if _, _, code := run(t, "patch", "check", "sample", "1.1", "--root", root); code != ExitOK {
		t.Errorf("exit = %d, want %d", code, ExitOK)
	}
	if got := planText(t, root); got != mapPlan {
		t.Error("a no-op check rewrote the file")
	}
}

// The kickoff language is the one answer `spec new` and `plan new` take no flag for,
// so `patch fm` is the whole path to it — and the path is only useful if it is the one
// autonomy.md tells the agent to type. A wrong value takes the same road as any other
// bad edit: the validator that owns the file fires, the write is undone, exit 2.
func TestPatchFrontmatterWritesTheKickoffLanguage(t *testing.T) {
	root := mapWorkspace(t)
	if _, stderr, code := run(t, "patch", "fm", "sample", "lang=wenyan", "--root", root); code != ExitOK {
		t.Fatalf("exit = %d, want %d (%s)", code, ExitOK, stderr)
	}
	if !strings.Contains(planText(t, root), "lang: wenyan") {
		t.Errorf("the answer was not recorded:\n%s", planText(t, root))
	}

	before := planText(t, root)
	_, stderr, code := run(t, "patch", "fm", "sample", "lang=pt-BR", "--root", root)
	if code != ExitFindings {
		t.Errorf("exit = %d, want %d for an undocumented language (%s)", code, ExitFindings, stderr)
	}
	if got := planText(t, root); got != before {
		t.Errorf("the rejected language was left on disk:\n%s", got)
	}
}

// A requirement id is scoped to its own spec. Tracing an unscoped one across a
// workspace where several specs number theirs the same way is the same mistake as
// reading all of them.
func TestMapTraceRefusesToGuessARequirementScope(t *testing.T) {
	root := mapWorkspace(t)
	for _, feature := range []string{"alpha", "beta"} {
		if _, _, code := run(t, "spec", "new", feature, "--root", root); code != ExitOK {
			t.Fatalf("spec new %s: exit %d", feature, code)
		}
		path := filepath.Join(root, "specs", feature, "requirements.md")
		body := "# " + feature + "\n\n## R1\n\n- **R1.1** The system shall do the " + feature + " thing\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	stdout, stderr, code := run(t, "map", "trace", "R1.1", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stderr, "scoped") {
		t.Errorf("did not say the id is ambiguous: %q %q", stdout, stderr)
	}
	for _, want := range []string{"specs/alpha/R1.1", "specs/beta/R1.1"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("did not offer %q:\n%s", want, stdout)
		}
	}
}

func TestMapAndPatchRequireAWorkspace(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{
		{"map", "--root", dir},
		{"map", "tasks", "--root", dir},
		{"patch", "check", "x", "1.1", "--root", dir},
	} {
		if _, _, code := run(t, args...); code != ExitError {
			t.Errorf("%v: exit = %d, want %d outside a workspace", args, code, ExitError)
		}
	}
}

// Every command's --json must be a clean document on stdout with diagnostics on
// stderr, so a caller can pipe one into the next without filtering.
func TestMapJSONIsCleanOnStdout(t *testing.T) {
	root := mapWorkspace(t)
	for _, args := range [][]string{
		{"map", "--json", "--root", root},
		{"map", "sample", "--json", "--root", root},
		{"map", "tasks", "sample", "--json", "--root", root},
		{"map", "blocks", "sample", "--json", "--root", root},
	} {
		stdout, stderr, code := run(t, args...)
		if code != ExitOK {
			t.Errorf("%v: exit %d (%s)", args, code, stderr)
			continue
		}
		var v any
		if err := json.Unmarshal([]byte(stdout), &v); err != nil {
			t.Errorf("%v: stdout is not JSON (%v): %s", args, err, stdout)
		}
		if stderr != "" {
			t.Errorf("%v: stderr = %q, want empty", args, stderr)
		}
	}
}
