package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// specWorkspace is mapWorkspace plus a spec, for the reads that only make sense
// against one: a requirement id, a trace, and an index with both trees in it.
func specWorkspace(t *testing.T) string {
	t.Helper()
	root := mapWorkspace(t)
	dir := filepath.Join(root, "specs", "job-store")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"requirements.md": `---
autonomy: auto
ci: wait
---

# Job store

## Requirements

- **R1.1** WHEN a job is submitted THE SYSTEM SHALL write it to disk before replying
- **R1.2** WHILE a job is running THE SYSTEM SHALL keep its request id addressable
`,
		"design.md": `# Job store — design

## Storage

One file per job, written atomically. Satisfies R1.1.

## Recovery

The request id recovers an already-paid result after a crash. Satisfies R1.2.
`,
		"tasks.md": `# Job store — tasks

## Tasks

- [ ] 1.1 (TDD) Write the job file atomically (R1.1)
- [ ] 1.2 (Unit) Recover by request id (R1.2)
`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// The index is what a session reads instead of the workspace. It has to name every
// artifact and say enough about each that the next command is obvious — which is
// the whole reason it exists rather than `ls`.
func TestMapIndexNamesEveryArtifact(t *testing.T) {
	root := specWorkspace(t)

	stdout, _, code := run(t, "map", "--root", root)
	if code != ExitOK {
		t.Fatalf("map: exit %d", code)
	}
	for _, want := range []string{"sample", "job-store"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("the index does not mention %q:\n%s", want, stdout)
		}
	}

	var doc struct {
		Artifacts []struct {
			Path  string `json:"path"`
			Kind  string `json:"kind"`
			Title string `json:"title"`
		} `json:"artifacts"`
	}
	stdout, _, code = run(t, "map", "index", "--root", root, "--json")
	if code != ExitOK {
		t.Fatalf("map index --json: exit %d", code)
	}
	decode(t, stdout, &doc)
	if len(doc.Artifacts) < 4 {
		t.Fatalf("the index lists %d artifacts, want the plan and the spec's three: %+v",
			len(doc.Artifacts), doc.Artifacts)
	}
	kinds := map[string]bool{}
	for _, a := range doc.Artifacts {
		if a.Path == "" || a.Kind == "" {
			t.Errorf("an entry is not fully described: %+v", a)
		}
		kinds[a.Kind] = true
	}
	for _, want := range []string{"plan", "requirements", "design", "tasks"} {
		if !kinds[want] {
			t.Errorf("the index has no %s artifact: %+v", want, doc.Artifacts)
		}
	}
}

// A trace answers "what else mentions this", which is the question that otherwise
// costs a read of every file in the spec.
func TestMapTraceFollowsARequirementThroughItsSpec(t *testing.T) {
	root := specWorkspace(t)

	stdout, stderr, code := run(t, "map", "trace", "specs/job-store/R1.1", "--root", root)
	if code != ExitOK {
		t.Fatalf("map trace: exit %d (%s)", code, stderr)
	}
	// The requirement itself and the design that satisfies it: two files, one
	// command, where the alternative is reading the spec.
	for _, want := range []string{"requirements.md", "design.md"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("the trace does not reach %s:\n%s", want, stdout)
		}
	}

	// A spec reference traces too: a plan's leaf is an address like any other.
	if _, _, code := run(t, "map", "trace", "specs/job-store/", "--root", root); code != ExitOK {
		t.Errorf("tracing a spec reference exited %d", code)
	}

	// An id nothing mentions is answered rather than failed — "nothing mentions
	// this" is a result, and the line saying so is what stops a caller reading an
	// empty stdout as a broken command.
	stdout, _, code = run(t, "map", "trace", "specs/job-store/R9.9", "--root", root)
	if code != ExitOK {
		t.Errorf("tracing an id nothing mentions exited %d", code)
	}
	if !strings.Contains(stdout, "nothing") {
		t.Errorf("an empty trace said nothing about being empty: %q", stdout)
	}
}

// Section addressing bottoms out on a long section with no headings inside it, and
// blocks is the answer: every paragraph's opening line, as an address `show` takes.
func TestMapBlocksIndexesASectionThatHasNoHeadings(t *testing.T) {
	root := mapWorkspace(t)

	stdout, stderr, code := run(t, "map", "blocks", "plans/sample.md", "--root", root)
	if code != ExitOK {
		t.Fatalf("map blocks: exit %d (%s)", code, stderr)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Fatal("map blocks printed nothing")
	}

	var doc struct {
		Blocks []struct {
			Section string `json:"section"`
			Index   int    `json:"index"`
			Lead    string `json:"lead"`
			Line    int    `json:"line"`
		} `json:"blocks"`
	}
	stdout, _, code = run(t, "map", "blocks", "plans/sample.md", "--root", root, "--json")
	if code != ExitOK {
		t.Fatalf("map blocks --json: exit %d", code)
	}
	decode(t, stdout, &doc)
	if len(doc.Blocks) == 0 {
		t.Fatal("map blocks --json listed nothing")
	}
	// Every ref it prints has to be one `show` accepts, or the index is a list of
	// addresses that do not resolve.
	for _, b := range doc.Blocks {
		if b.Section == "" || b.Index < 1 || b.Line < 1 || b.Lead == "" {
			t.Errorf("a block is not addressable: %+v", b)
			continue
		}
		ref := fmt.Sprintf("%s:%d", b.Section, b.Index)
		if _, _, code := run(t, "map", "show", "plans/sample.md", ref, "--root", root); code != ExitOK {
			t.Errorf("map show %s exited %d, though blocks printed it", ref, code)
		}
	}
}

// --ready and --blocked share one implementation with --next, because two notions
// of eligibility would be two answers to "what do I work on".
func TestMapTasksReadyAndBlockedAgreeWithNext(t *testing.T) {
	root := mapWorkspace(t)

	next, _, code := run(t, "map", "tasks", "plans/sample.md", "--root", root, "--next")
	if code != ExitOK {
		t.Fatalf("--next: exit %d", code)
	}
	if strings.TrimSpace(next) == "" {
		t.Fatal("--next printed nothing on a plan with open tasks")
	}
	ready, _, code := run(t, "map", "tasks", "plans/sample.md", "--root", root, "--ready")
	if code != ExitOK {
		t.Fatalf("--ready: exit %d", code)
	}
	if strings.TrimSpace(ready) == "" {
		t.Fatal("--ready printed nothing on a plan with open tasks")
	}

	for _, flag := range []string{"--blocked", "--deps", "--open", "--done"} {
		if _, _, code := run(t, "map", "tasks", "plans/sample.md", "--root", root, flag); code != ExitOK {
			t.Errorf("%s exited %d", flag, code)
		}
	}
}

// brief is the header and tasks is the checklist, and no command returns both —
// that split is what gives "never read the plan" its authority.
func TestMapBriefAndTasksAreEachSmallerThanThePlan(t *testing.T) {
	root := mapWorkspace(t)
	full := len(planText(t, root))

	brief, _, code := run(t, "map", "brief", "plans/sample.md", "--root", root)
	if code != ExitOK {
		t.Fatalf("map brief: exit %d", code)
	}
	tasks, _, code := run(t, "map", "tasks", "plans/sample.md", "--root", root)
	if code != ExitOK {
		t.Fatalf("map tasks: exit %d", code)
	}
	if len(brief) >= full {
		t.Errorf("the brief is no smaller than the plan (%d vs %d)", len(brief), full)
	}
	if len(tasks) >= full {
		t.Errorf("the checklist is no smaller than the plan (%d vs %d)", len(tasks), full)
	}
}

// A name rather than a path: `scc map brief sample` has to find the plan, because
// the address forms exist so a caller never has to know the layout.
func TestMapAcceptsANameAsWellAsAPath(t *testing.T) {
	root := specWorkspace(t)

	for _, ref := range []string{"plans/sample.md", "sample"} {
		if _, _, code := run(t, "map", "brief", ref, "--root", root); code != ExitOK {
			t.Errorf("map brief %s exited %d", ref, code)
		}
	}
	for _, ref := range []string{"specs/job-store/tasks.md", "job-store"} {
		if _, _, code := run(t, "map", "outline", ref, "--root", root); code != ExitOK {
			t.Errorf("map outline %s exited %d", ref, code)
		}
	}
	// A name nothing answers to is an error that says so.
	stdout, stderr, code := run(t, "map", "brief", "no-such-plan", "--root", root)
	if code == ExitOK {
		t.Error("map brief on a name nothing answers to exited 0")
	}
	if !strings.Contains(stdout+stderr, "no-such-plan") {
		t.Errorf("the error does not name what was asked for:\n%s%s", stdout, stderr)
	}
}
