package artifact

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The vocabulary is closed, and it is closed for the reason a task's flags are: an
// open one produces three spellings of "done" inside a month, and the question this
// record answers — what is still unfinished — is the one a synonym destroys.
func TestDeliveryVocabularyIsClosedAndOrdered(t *testing.T) {
	states := DeliveryStates()
	if len(states) != 4 {
		t.Fatalf("DeliveryStates = %v, want the four this record knows", states)
	}
	if states[0] != DeliveryInProgress || states[len(states)-1] != DeliveryAbandoned {
		t.Errorf("DeliveryStates = %v, want the order work moves through", states)
	}
	for _, s := range states {
		if !ValidDelivery(s) {
			t.Errorf("ValidDelivery(%q) = false for a state of its own list", s)
		}
	}
	for _, s := range []string{"", "done", "DONE", "in progress", "closed"} {
		if ValidDelivery(s) {
			t.Errorf("ValidDelivery(%q) = true for a word this vocabulary does not have", s)
		}
	}

	// Settled is what separates "still open" from "done with", which is the only
	// distinction a reader scanning for loose ends actually makes.
	for _, s := range []string{DeliveryMerged, DeliveryAbandoned} {
		if !Settled(s) {
			t.Errorf("Settled(%q) = false for a terminal state", s)
		}
	}
	for _, s := range []string{DeliveryInProgress, DeliveryInReview, ""} {
		if Settled(s) {
			t.Errorf("Settled(%q) = true for work that is still open", s)
		}
	}
}

// A `pr:` that is not a number comes back as 0 rather than as an error: reading is
// not where a malformed value is reported, and a reader that failed here would take
// out `scc spec list` for the whole workspace over one typo.
func TestReadDeliveryNeverFailsOnAMalformedValue(t *testing.T) {
	full := ReadDelivery(map[string]string{
		KeyBranch: "feat/x", KeyPR: "42", KeyDelivery: DeliveryInReview,
	})
	if full.Branch != "feat/x" || full.PR != 42 || full.State != DeliveryInReview {
		t.Errorf("ReadDelivery = %+v", full)
	}
	if !full.Tracked() {
		t.Error("a spec with a branch, a PR and a state reported as untracked")
	}

	for _, bad := range []string{"", "none", "#42", "0", "-3"} {
		got := ReadDelivery(map[string]string{KeyPR: bad})
		if got.PR != 0 {
			t.Errorf("ReadDelivery with pr=%q gave %d, want 0", bad, got.PR)
		}
	}

	// An untracked spec is not a defect — it is one nobody has started, or one
	// that predates the record.
	if ReadDelivery(nil).Tracked() {
		t.Error("an empty frontmatter reported as tracked")
	}
	// Any one of the three is enough to count as started.
	for _, fm := range []map[string]string{
		{KeyBranch: "feat/x"},
		{KeyPR: "1"},
		{KeyDelivery: DeliveryMerged},
	} {
		if !ReadDelivery(fm).Tracked() {
			t.Errorf("ReadDelivery(%v) reported untracked", fm)
		}
	}
}

// Scan is the index every other read is a narrowing of: plans in name order, then
// each spec's three files in phase order, so a caller listing a workspace gets the
// same order twice running.
func TestScanReadsPlansThenSpecsInPhaseOrder(t *testing.T) {
	root := t.TempDir()
	mkdirAll(t, filepath.Join(root, "plans"))
	mkdirAll(t, filepath.Join(root, "specs", "beta"))
	mkdirAll(t, filepath.Join(root, "specs", "alpha"))

	write(t, filepath.Join(root, "plans", "zulu.md"), "# Zulu\n")
	write(t, filepath.Join(root, "plans", "alpha.md"), "# Alpha plan\n")
	// A directory under plans/ is skipped rather than read: the scanner uses
	// ReadDir, which is what lets `plans/archive/` hold a migrated plan's notes.
	mkdirAll(t, filepath.Join(root, "plans", "archive"))
	write(t, filepath.Join(root, "plans", "archive", "old-notes.md"), "# Notes\n")
	// And a file that is not Markdown is not an artifact.
	write(t, filepath.Join(root, "plans", "notes.txt"), "not an artifact\n")

	write(t, filepath.Join(root, "specs", "alpha", "requirements.md"), "# Alpha\n")
	write(t, filepath.Join(root, "specs", "alpha", "tasks.md"), "# Alpha tasks\n")
	write(t, filepath.Join(root, "specs", "beta", "requirements.md"), "# Beta\n")

	all, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	var got []string
	for _, a := range all {
		got = append(got, a.Path)
	}
	want := []string{
		"plans/alpha.md", "plans/zulu.md",
		"specs/alpha/requirements.md", "specs/alpha/tasks.md",
		"specs/beta/requirements.md",
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("Scan =\n  %v\nwant\n  %v", got, want)
	}

	// A workspace with neither tree is empty rather than an error: that is a
	// young workspace, not a failure.
	empty, err := Scan(t.TempDir())
	if err != nil || len(empty) != 0 {
		t.Errorf("Scan on an empty workspace = %d artifacts, %v", len(empty), err)
	}
}

func mkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", dir, err)
	}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

// Text is what an editor prints and Prose is what a summary prints, and the
// difference is the whole reason there are two. On a freshly scaffolded artifact
// the instructions to the author are HTML comments and outweigh everything the
// author has written, so a brief that echoed them would spend its budget on text
// addressed to somebody else.
func TestProseDropsWhatIsNotContentAndTextDoesNot(t *testing.T) {
	root := t.TempDir()
	mkdirAll(t, filepath.Join(root, "plans"))
	path := filepath.Join(root, "plans", "p.md")
	write(t, path, "# P\n\n## Why\n\n<!-- an instruction to the author -->\nreal prose\n\n\n\n```\nfenced\n```\nmore prose\n")

	a, err := Load(root, path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	full := a.Text(1, a.LineCount())
	for _, want := range []string{"an instruction to the author", "fenced", "real prose"} {
		if !strings.Contains(full, want) {
			t.Errorf("Text dropped %q, and an editor needs the file as it is:\n%s", want, full)
		}
	}

	summary := a.Prose(1, a.LineCount())
	for _, gone := range []string{"an instruction to the author", "fenced"} {
		if strings.Contains(summary, gone) {
			t.Errorf("Prose kept %q, which is not the author's content:\n%s", gone, summary)
		}
	}
	for _, kept := range []string{"real prose", "more prose"} {
		if !strings.Contains(summary, kept) {
			t.Errorf("Prose dropped %q:\n%s", kept, summary)
		}
	}
	if strings.Contains(summary, "\n\n\n") {
		t.Errorf("Prose left a run of blank lines:\n%q", summary)
	}

	// Both clamp rather than panic: an address resolved against a file that has
	// since been edited is exactly where an out-of-range pair comes from.
	if got := a.Text(-5, 1); got != a.Lines[0] {
		t.Errorf("Text(-5, 1) = %q", got)
	}
	if got := a.Text(3, 2); got != "" {
		t.Errorf("Text with from > to = %q, want empty", got)
	}
	if got := a.Prose(-5, a.LineCount()+50); !strings.Contains(got, "real prose") {
		t.Errorf("Prose did not clamp: %q", got)
	}
	if a.Doc() == nil {
		t.Error("Doc returned nil, so the searcher has no blanked body to match against")
	}
}

// An editor that failed reports the reason and produces nothing, so one path
// reports every reason a patch stopped — including the approved-plan guards, which
// depend on what the file says and so cannot be decided before it is loaded.
func TestEditorFailIsReportedThroughTheSamePath(t *testing.T) {
	a, _ := load(t)

	e := a.Edit()
	if !e.Empty() {
		t.Error("a fresh editor is not empty")
	}
	e.Fail("the plan is approved, so %s is refused", "replace")
	if _, err := e.Content(); err == nil {
		t.Fatal("Content returned no error after Fail")
	} else if !strings.Contains(err.Error(), "replace") {
		t.Errorf("error = %q, which does not carry the reason", err)
	}

	// And a failed editor stays failed: a later edit does not clear it.
	e.Prepend("#why", "something")
	if _, err := e.Content(); err == nil {
		t.Error("an edit after a failure cleared the failure")
	}
}

// Prepend writes under a heading, above whatever it already says — the half of the
// pair that lets a caller put something where it will be read first.
func TestPrependWritesUnderTheHeading(t *testing.T) {
	a, _ := load(t)

	e := a.Edit()
	e.Prepend("#why", "A new first line.")
	out, err := e.Content()
	if err != nil {
		t.Fatalf("Content: %v", err)
	}
	lines := strings.Split(out, "\n")
	var heading, added, existing int
	for i, l := range lines {
		switch {
		case strings.HasPrefix(l, "## Why"):
			heading = i
		case strings.Contains(l, "A new first line."):
			added = i
		case strings.Contains(l, "One paragraph about why"):
			existing = i
		}
	}
	if !(heading < added && added < existing) {
		t.Errorf("prepend landed at %d, between heading %d and the existing text %d:\n%s",
			added, heading, existing, out)
	}
	if len(e.Changes()) != 1 {
		t.Errorf("Changes = %+v, want the one splice", e.Changes())
	}

	// An address that does not resolve is an error, never an insert at a guess.
	bad := a.Edit()
	bad.Prepend("#no-such-section", "x")
	if _, err := bad.Content(); err == nil {
		t.Error("prepend to an address that does not resolve produced a file")
	}
}

// The seal is tamper-evidence, not prevention: a file edited by hand after approval
// no longer matches what it recorded, and that is the whole of what it claims.
func TestApprovedAndDrift(t *testing.T) {
	root := t.TempDir()
	mkdirAll(t, filepath.Join(root, "plans"))
	path := filepath.Join(root, "plans", "p.md")

	// A plan with no status is never checked, which is what makes every plan
	// written before the seal shipped keep working.
	write(t, path, "---\nautonomy: auto\n---\n\n# P\n\n## Why\n\nBecause.\n")
	a, err := Load(root, path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if a.Approved() {
		t.Error("a plan with no status reported as approved")
	}
	if _, _, drifted := a.Drift(); drifted {
		t.Error("a plan with no status reported as drifted")
	}

	sealed := Approve("---\nautonomy: auto\n---\n\n# P\n\n## Why\n\nBecause.\n")
	write(t, path, sealed)
	a, err = Load(root, path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !a.Approved() {
		t.Fatalf("Approve did not stamp the status:\n%s", sealed)
	}
	recorded, actual, drifted := a.Drift()
	if drifted {
		t.Errorf("a freshly approved plan drifted: recorded %q, actual %q", recorded, actual)
	}

	// Edit it by hand, and the seal says so.
	write(t, path, strings.Replace(sealed, "Because.", "Because of something else.", 1))
	a, err = Load(root, path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	recorded, actual, drifted = a.Drift()
	if !drifted {
		t.Error("an edited approved plan did not report drift")
	}
	if recorded == actual || recorded == "" || actual == "" {
		t.Errorf("drift reported recorded %q and actual %q", recorded, actual)
	}

	// Reseal is the answer to a legitimate edit, and it leaves the status alone.
	resealed := Reseal(strings.Replace(sealed, "Because.", "Because of something else.", 1))
	write(t, path, resealed)
	a, err = Load(root, path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !a.Approved() {
		t.Error("Reseal dropped the approved status")
	}
	if _, _, drifted := a.Drift(); drifted {
		t.Error("a resealed plan still reports drift")
	}
}

// The schedule is one implementation shared by --next, --ready and --blocked: two
// notions of eligibility would be two answers to "what do I work on".
func TestReadyBlockedAndNextAreOneSchedule(t *testing.T) {
	root := t.TempDir()
	mkdirAll(t, filepath.Join(root, "plans"))
	path := filepath.Join(root, "plans", "p.md")
	write(t, path, `# P

## Tasks

- [x] 1.1 (Unit) Done already
- [ ] 1.9 (Unit) Ninth
- [ ] 1.10 (Unit) Tenth
- [ ] 1.2 (Unit) Urgent
  _Priority 1_
- [ ] 1.3 (Unit) Waits for the tenth
  _Depends 1.10_
`)
	a, err := Load(root, path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var ready []string
	for _, t := range a.Ready() {
		ready = append(ready, t.Number)
	}
	// Priority ascending with absent last, then number compared *numerically* —
	// which is also why 1.9 sorts before 1.10 rather than after it.
	want := []string{"1.2", "1.9", "1.10"}
	if strings.Join(ready, " ") != strings.Join(want, " ") {
		t.Errorf("Ready = %v, want %v", ready, want)
	}

	blocked := a.BlockedTasks()
	if len(blocked) != 1 || blocked[0].Number != "1.3" {
		t.Errorf("BlockedTasks = %+v, want the one waiting on 1.10", blocked)
	}
	if got := a.WaitingOn(blocked[0]); len(got) != 1 || got[0] != "1.10" {
		t.Errorf("WaitingOn = %v, want 1.10", got)
	}

	next, ok := a.Next()
	if !ok || next.Number != "1.2" {
		t.Errorf("Next = %+v, %v — want the highest-priority ready task", next, ok)
	}
	if next.Number != a.Ready()[0].Number {
		t.Error("Next and Ready disagree about what to work on")
	}

	// A plan with nothing open has no next task, and that is an answer.
	write(t, path, "# P\n\n## Tasks\n\n- [x] 1.1 (Unit) Done\n")
	done, err := Load(root, path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := done.Next(); ok {
		t.Error("Next found a task in a finished plan")
	}
	if len(done.Ready()) != 0 || len(done.BlockedTasks()) != 0 {
		t.Error("a finished plan reports ready or blocked tasks")
	}
}

// Within is the boundary between an artifact address and the rest of the
// filesystem: without it `scc patch append ../outside.md L3 --text X` wrote to a
// file outside the repository and exited 0.
func TestWithinIsTheBoundaryAnAddressCannotCross(t *testing.T) {
	root := t.TempDir()
	mkdirAll(t, filepath.Join(root, "plans"))
	write(t, filepath.Join(root, "plans", "p.md"), "# P\n")

	for _, in := range []string{
		filepath.Join(root, "plans", "p.md"),
		filepath.Join(root, "plans"),
		root,
		filepath.Join(root, "does", "not", "exist.md"),
	} {
		if !Within(root, in) {
			t.Errorf("Within(root, %q) = false for a path inside the workspace", in)
		}
	}

	outside := t.TempDir()
	for _, out := range []string{
		outside,
		filepath.Join(outside, "elsewhere.md"),
		filepath.Join(root, "..", "sibling.md"),
		filepath.Join(root, "plans", "..", "..", "escape.md"),
	} {
		if Within(root, out) {
			t.Errorf("Within(root, %q) = true for a path outside the workspace", out)
		}
	}

	// A sibling whose name merely starts with the root's is not inside it, which a
	// string-prefix test would get wrong.
	if Within(root, root+"-other") {
		t.Error("a sibling directory sharing the root's prefix was called inside it")
	}
}

// A task's continuation is the lines below the checkbox with the flags removed,
// which is where the decision usually sits — and the reason --next prints the task
// whole while the listings clip.
func TestContinuationIsTheTaskWithoutItsFlags(t *testing.T) {
	root := t.TempDir()
	mkdirAll(t, filepath.Join(root, "plans"))
	path := filepath.Join(root, "plans", "p.md")
	write(t, path, `# P

## Tasks

- [ ] 1.1 (TDD) A task whose description runs on
      and keeps running onto a second line
  _Depends 1.2_
  _Priority 2_
- [ ] 1.2 (Unit) Another
`)
	a, err := Load(root, path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	task, ok := a.Task("1.1")
	if !ok {
		t.Fatal("1.1 did not parse")
	}

	cont := strings.Join(task.Continuation(), "\n")
	if !strings.Contains(cont, "keeps running onto a second line") {
		t.Errorf("Continuation dropped the description: %q", cont)
	}
	for _, flag := range []string{"_Depends", "_Priority"} {
		if strings.Contains(cont, flag) {
			t.Errorf("Continuation kept %s, so a searcher would index a flag as prose: %q", flag, cont)
		}
	}
	// Detail is the same text on one line, for a row in a table.
	if strings.Contains(task.Detail, "\n") {
		t.Errorf("Detail spans lines: %q", task.Detail)
	}
	// End covers the flags, so removing the task removes them with it.
	if !strings.Contains(a.Text(task.Line, task.End), "_Priority 2_") {
		t.Errorf("the task's region stops short of its flags:\n%s", a.Text(task.Line, task.End))
	}

	// ParseTasks is the same grammar reachable without an Artifact, and it must
	// agree with it — a reader that disagreed with the validator about what a task
	// is would be worse than no reader.
	direct := ParseTasks(a.Doc())
	if len(direct) != len(a.Tasks) {
		t.Errorf("ParseTasks found %d tasks, Load found %d", len(direct), len(a.Tasks))
	}
}

// The forms that the existing address test does not reach, and the collisions the
// resolution order exists to settle.
func TestFindSettlesTheFormsThatCouldCollide(t *testing.T) {
	a, _ := load(t)

	// A bare number is a task before it is a line, because a caller who means a
	// line writes the L.
	task, err := a.Find("1.2")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if !strings.Contains(a.Text(task.Line, task.End), "1.2") {
		t.Errorf("Find(\"1.2\") resolved to %+v, which does not contain the task", task)
	}
	if got := task.Lines(); got != task.End-task.Line+1 {
		t.Errorf("Lines() = %d for %d..%d", got, task.Line, task.End)
	}

	// A single L is a one-line range rather than an error.
	one, err := a.Find("L1")
	if err != nil {
		t.Fatalf("Find(\"L1\"): %v", err)
	}
	if one.Lines() != 1 {
		t.Errorf("Find(\"L1\") covers %d lines", one.Lines())
	}

	// A section by title as written, not only by slug.
	if _, err := a.Find("Why"); err != nil {
		t.Errorf("Find by heading text: %v", err)
	}

	for _, ref := range []string{"", "#no-such-section", "specs/nothing/", "notes:99", "R1.1"} {
		if got, err := a.Find(ref); err == nil {
			t.Errorf("Find(%q) resolved to %+v in a plan that has no such thing", ref, got)
		}
	}
}

// Requirement is the lookup a trace is built on, and the id is matched without
// regard to case because a spec writes R1.2 and a citation sometimes writes r1.2.
func TestRequirementLookupIgnoresCase(t *testing.T) {
	root := t.TempDir()
	mkdirAll(t, filepath.Join(root, "specs", "auth"))
	path := filepath.Join(root, "specs", "auth", "requirements.md")
	write(t, path, `# Auth

## Requirements

- **R1.1** WHEN a token expires THE SYSTEM SHALL refuse the request
- **R1.2** WHILE a session is open THE SYSTEM SHALL refresh the token
`)
	a, err := Load(root, path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(a.Requirements) != 2 {
		t.Fatalf("parsed %d requirements: %+v", len(a.Requirements), a.Requirements)
	}
	for _, id := range []string{"R1.2", "r1.2"} {
		got, ok := a.Requirement(id)
		if !ok {
			t.Errorf("Requirement(%q) not found", id)
			continue
		}
		if !strings.Contains(got.Text, "refresh the token") {
			t.Errorf("Requirement(%q) = %+v", id, got)
		}
	}
	if _, ok := a.Requirement("R9.9"); ok {
		t.Error("Requirement found an id the spec does not define")
	}
}

// Migration has to write a status into a plan that may never have had a
// frontmatter block at all, so both halves are exported and both have to work on a
// file with none.
func TestEnsureAndSetFrontmatterOnAFileWithNone(t *testing.T) {
	lines := []string{"# P", "", "## Why", "", "Because."}

	got, n := EnsureFrontmatter(lines)
	if n < 2 {
		t.Fatalf("EnsureFrontmatter reported a %d-line block: %v", n, got)
	}
	if got[0] != "---" {
		t.Errorf("the block does not open with a delimiter: %v", got)
	}

	got, n = SetFrontmatterKey(got, n, KeyStatus, StatusApproved)
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, KeyStatus+": "+StatusApproved) {
		t.Errorf("the key was not written:\n%s", joined)
	}
	if !strings.Contains(joined, "# P") {
		t.Errorf("the document was lost:\n%s", joined)
	}

	// Writing the same key again replaces it rather than adding a second line.
	got, _ = SetFrontmatterKey(got, n, KeyStatus, "draft")
	if strings.Count(strings.Join(got, "\n"), KeyStatus+":") != 1 {
		t.Errorf("the key is recorded twice:\n%s", strings.Join(got, "\n"))
	}
}
