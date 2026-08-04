package artifact

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// samplePlan is shaped like the artifacts this package was measured against: tasks
// whose description runs past the checkbox line, a decomposition of leaves that
// carry no boxes, and a Notes section with no headings to navigate by.
const samplePlan = `---
autonomy: auto
ci: wait
---

# Sample — the pipeline

## Why

One paragraph about why this exists, and what done means.

## Decomposition

- ` + "`specs/job-store/`" + ` — one file per job, atomic writes, and the request id
  that recovers an already-paid result after a crash.
- ` + "`specs/model-registry/`" + ` — validation against the model's schema before
  anything is submitted.

## Tasks

- [x] 1.1 (Unit) Build the thing — write the wrapper, vendor the module, and prove
      with a test that it loads
- [ ] 1.2 (TDD) Guard the message before it lands, because HTTP clients embed the
      full URL and sometimes a credential with it

## Notes

**Order matters.** job-store first, because every other leaf writes through it and
a second writer would race the first.

**The free path wins.** The expensive command is the one that has to know about the
cheap one, never the other way round.
`

func writeWorkspace(t *testing.T) (root, planPath string) {
	t.Helper()
	root = t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	planPath = filepath.Join(root, "plans", "sample.md")
	if err := os.WriteFile(planPath, []byte(samplePlan), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, planPath
}

func load(t *testing.T) (*Artifact, string) {
	t.Helper()
	root, path := writeWorkspace(t)
	a, err := Load(root, path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return a, root
}

func TestLoadClassifiesAndTitles(t *testing.T) {
	a, _ := load(t)
	if a.Kind != KindPlan {
		t.Errorf("Kind = %q, want %q", a.Kind, KindPlan)
	}
	if a.Name != "sample" {
		t.Errorf("Name = %q, want %q", a.Name, "sample")
	}
	if a.Title != "Sample — the pipeline" {
		t.Errorf("Title = %q", a.Title)
	}
	if a.Path != "plans/sample.md" {
		t.Errorf("Path = %q, want slash-separated and root-relative", a.Path)
	}
	if a.Frontmatter["autonomy"] != "auto" {
		t.Errorf("frontmatter not read: %v", a.Frontmatter)
	}
}

// A task's description runs past its checkbox line in every real artifact. A model
// that ended a task at its first line would address two lines of a five-line item.
func TestTaskCoversItsContinuation(t *testing.T) {
	a, _ := load(t)
	if len(a.Tasks) != 2 {
		t.Fatalf("got %d tasks, want 2", len(a.Tasks))
	}
	one := a.Tasks[0]
	if one.End <= one.Line {
		t.Errorf("task 1.1 ends at %d, started at %d — continuation not captured", one.End, one.Line)
	}
	if !strings.Contains(one.Detail, "with a test that it loads") {
		t.Errorf("Detail missing the continuation: %q", one.Detail)
	}
	if strings.Contains(one.Text, "with a test") {
		t.Errorf("Text swallowed the continuation: %q — a listing prints one line per task", one.Text)
	}
	if one.Methodology != "Unit" || !one.Checked {
		t.Errorf("1.1 parsed as %+v", one)
	}
	if a.Tasks[1].Methodology != "TDD" || a.Tasks[1].Checked {
		t.Errorf("1.2 parsed as %+v", a.Tasks[1])
	}
}

// A leaf carries no checkbox: its state lives in the spec it names. Counting one as
// a task would double-count the work and report progress nobody made.
func TestLeavesAreNotTasks(t *testing.T) {
	a, _ := load(t)
	if len(a.Leaves) != 2 {
		t.Fatalf("got %d leaves, want 2: %+v", len(a.Leaves), a.Leaves)
	}
	if a.Leaves[0].Feature != "job-store" {
		t.Errorf("Feature = %q", a.Leaves[0].Feature)
	}
	if !strings.Contains(a.Leaves[0].Text, "one file per job") {
		t.Errorf("leaf text = %q", a.Leaves[0].Text)
	}
	done, total := a.Done()
	if done != 1 || total != 2 {
		t.Errorf("Done() = %d/%d, want 1/2 — leaves must not count as tasks", done, total)
	}
}

// A section's counts roll up from its children. Without that, `## Tasks` reports
// zero in any plan whose tasks sit under numbered subsections.
func TestSectionCountsRollUp(t *testing.T) {
	a, _ := load(t)
	s, ok := a.Section("tasks")
	if !ok {
		t.Fatal("no #tasks section")
	}
	if s.Tasks != 2 || s.Done != 1 {
		t.Errorf("#tasks = %d/%d, want 1/2", s.Done, s.Tasks)
	}
	if d, ok := a.Section("decomposition"); !ok || d.Leaves != 2 {
		t.Errorf("#decomposition leaves = %d, want 2", d.Leaves)
	}
}

func TestBlocksIndexParagraphs(t *testing.T) {
	a, _ := load(t)
	var notes []Block
	for _, b := range a.Blocks() {
		if b.Section == "notes" {
			notes = append(notes, b)
		}
	}
	if len(notes) != 2 {
		t.Fatalf("got %d note paragraphs, want 2: %+v", len(notes), notes)
	}
	if !strings.HasPrefix(notes[0].Lead, "Order matters.") {
		t.Errorf("lead = %q, want the emphasis stripped", notes[0].Lead)
	}
	if notes[0].End <= notes[0].Line {
		t.Errorf("block ends at %d, started at %d", notes[0].End, notes[0].Line)
	}
}

func TestFindResolvesEveryAddressForm(t *testing.T) {
	a, _ := load(t)
	for _, tc := range []struct {
		ref  string
		kind TargetKind
	}{
		{"1.2", TargetTask},
		{"specs/job-store/", TargetLeaf},
		{"#notes", TargetSection},
		{"Notes", TargetSection},
		{"notes:1", TargetBlock},
		{"L1-3", TargetRange},
	} {
		got, err := a.Find(tc.ref)
		if err != nil {
			t.Errorf("Find(%q): %v", tc.ref, err)
			continue
		}
		if got.Kind != tc.kind {
			t.Errorf("Find(%q).Kind = %q, want %q", tc.ref, got.Kind, tc.kind)
		}
		if got.Line < 1 || got.End < got.Line {
			t.Errorf("Find(%q) = lines %d-%d", tc.ref, got.Line, got.End)
		}
	}
}

// An address that misses must say what the file does have. The caller is an agent
// that cannot see the file, so "not found" alone costs it another round trip.
func TestFindNamesWhatItHasOnAMiss(t *testing.T) {
	a, _ := load(t)
	_, err := a.Find("9.9")
	if err == nil {
		t.Fatal("Find(9.9) succeeded")
	}
	if !strings.Contains(err.Error(), "1.1") || !strings.Contains(err.Error(), "1.2") {
		t.Errorf("error does not list the tasks that exist: %v", err)
	}
}

func TestResolveAcceptsPathAndBareName(t *testing.T) {
	root, planPath := writeWorkspace(t)
	for _, arg := range []string{"plans/sample.md", "sample", planPath} {
		got, err := Resolve(root, arg)
		if err != nil {
			t.Errorf("Resolve(%q): %v", arg, err)
			continue
		}
		if len(got) != 1 {
			t.Errorf("Resolve(%q) = %v, want one file", arg, got)
			continue
		}
		same, err := sameFile(got[0], planPath)
		if err != nil || !same {
			t.Errorf("Resolve(%q) = %q, want %q", arg, got[0], planPath)
		}
	}
}

// A name that would escape the workspace must not become a path segment.
func TestResolveRefusesEscape(t *testing.T) {
	root, _ := writeWorkspace(t)
	if got, err := Resolve(root, ".."); err == nil {
		t.Errorf("Resolve(..) = %v, want an error", got)
	}
}

func sameFile(a, b string) (bool, error) {
	ai, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	bi, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	return os.SameFile(ai, bi), nil
}

func TestSearchRanksAddressableUnits(t *testing.T) {
	a, _ := load(t)
	hits, err := Search([]*Artifact{a}, "credential URL", SearchOpts{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("no hits")
	}
	if hits[0].Ref != "1.2" {
		t.Errorf("top hit = %q, want task 1.2", hits[0].Ref)
	}
	if hits[0].Line < 1 || hits[0].Snippet == "" {
		t.Errorf("hit is not usable as an address: %+v", hits[0])
	}
}

// Terms are ANDed by default: a reader asking for two words means both, and an OR
// over a small corpus returns everything.
func TestSearchDefaultsToAllTerms(t *testing.T) {
	a, _ := load(t)
	all, _ := Search([]*Artifact{a}, "credential vendor", SearchOpts{})
	any, _ := Search([]*Artifact{a}, "credential vendor", SearchOpts{Any: true})
	if len(all) >= len(any) {
		t.Errorf("AND returned %d hits and OR returned %d; AND must be the narrower", len(all), len(any))
	}
}

func TestSearchByKind(t *testing.T) {
	a, _ := load(t)
	hits, _ := Search([]*Artifact{a}, "job", SearchOpts{Kind: string(TargetLeaf)})
	for _, h := range hits {
		if h.Kind != string(TargetLeaf) {
			t.Errorf("--kind leaf returned a %s", h.Kind)
		}
	}
}

// The forms a caller actually types after a listing printed the feature name at
// them. `<feature>/tasks.md` is the one that is easy to leave out and the one that
// gets typed most.
func TestResolveAcceptsSpecRelativeForms(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "specs", "job-store")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "tasks.md")
	if err := os.WriteFile(want, []byte("# T\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, arg := range []string{
		"specs/job-store/tasks.md",
		"job-store/tasks.md",
	} {
		got, err := Resolve(root, arg)
		if err != nil {
			t.Errorf("Resolve(%q): %v", arg, err)
			continue
		}
		if len(got) != 1 {
			t.Errorf("Resolve(%q) = %v, want one file", arg, got)
			continue
		}
		if same, err := sameFile(got[0], want); err != nil || !same {
			t.Errorf("Resolve(%q) = %q, want %q", arg, got[0], want)
		}
	}
	// And naming the spec itself is all three of its files, minus the ones absent.
	if got, err := Resolve(root, "job-store"); err != nil || len(got) != 1 {
		t.Errorf("Resolve(job-store) = %v, %v", got, err)
	}
}
