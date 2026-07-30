package finding

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStderr swaps os.Stderr for a pipe around f. Package render writes to the
// real file, so this is the only way to assert on report output.
func captureStderr(t *testing.T, f func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = orig }()
	done := make(chan string, 1)
	go func() { b, _ := io.ReadAll(r); done <- string(b) }()
	f()
	_ = w.Close()
	return <-done
}

// A flat list of forty lines is the shape that gets skipped, and alert fatigue does
// not discriminate between the noise and the real findings. So the report leads with
// a count and groups by file.
func TestReportGroupsByFileAndLeadsWithACount(t *testing.T) {
	var s Set
	s.Addf("b.md", 4, "wiki.broken-link", "[[nowhere]] does not resolve")
	s.Addf("a.md", 2, "ears.missing-shall", "no `shall` clause")
	s.Addf("a.md", 9, "spec.orphan-requirement", "R1.9 reaches no task")

	out := captureStderr(t, func() { s.Report("spec") })
	if !strings.Contains(out, "3 findings") {
		t.Errorf("report does not lead with a count:\n%s", out)
	}
	// Each file heading appears once, and a.md sorts before b.md.
	if strings.Count(out, "a.md") != 1 || strings.Count(out, "b.md") != 1 {
		t.Errorf("files are not grouped:\n%s", out)
	}
	if strings.Index(out, "a.md") > strings.Index(out, "b.md") {
		t.Errorf("files are not sorted:\n%s", out)
	}
	// A CI job filters on the rule slug, so it has to be in the human output too.
	if !strings.Contains(out, "ears.missing-shall") {
		t.Errorf("rule slug missing from the report:\n%s", out)
	}
}

func TestReportSaysNothingWhenThereIsNothing(t *testing.T) {
	var s Set
	out := captureStderr(t, func() { s.Report("spec") })
	if strings.Contains(out, "finding") {
		t.Errorf("an empty set reported findings on stderr: %q", out)
	}
}

// A file-level finding has no line to print, and printing "0" would read as one.
func TestReportRendersAFileLevelFinding(t *testing.T) {
	var s Set
	s.Addf("docs/wiki/orphan.md", 0, "wiki.orphan-page", "not reachable from the index")
	out := captureStderr(t, func() { s.Report("wiki") })
	if strings.Contains(out, " 0 ") {
		t.Errorf("line 0 was printed as a location:\n%s", out)
	}
	if !strings.Contains(out, "not reachable") {
		t.Errorf("the message is missing:\n%s", out)
	}
}

// A validator emits findings in whatever order it walks the artifact; the output
// order is fixed here so two runs over the same workspace produce the same report.
func TestSortedIsStableAndTotal(t *testing.T) {
	var s Set
	s.Add(Finding{File: "b.md", Line: 2, Rule: "z.rule", Message: "second"})
	s.Add(Finding{File: "a.md", Line: 10, Rule: "a.rule", Message: "tenth"})
	s.Add(Finding{File: "a.md", Line: 2, Rule: "b.rule", Message: "b at 2"})
	s.Add(Finding{File: "a.md", Line: 2, Rule: "a.rule", Message: "a at 2"})
	s.Add(Finding{File: "a.md", Rule: "a.rule", Message: "whole file"})

	var got []string
	for _, f := range s.Sorted() {
		got = append(got, f.Message)
	}
	want := []string{"whole file", "a at 2", "b at 2", "tenth", "second"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("order = %v, want %v", got, want)
	}
}

// Line 10 must not sort before line 2. Sorting the numbers as strings is the
// classic way this goes wrong.
func TestSortedComparesLinesNumerically(t *testing.T) {
	var s Set
	s.Add(Finding{File: "a.md", Line: 10, Rule: "r", Message: "ten"})
	s.Add(Finding{File: "a.md", Line: 9, Rule: "r", Message: "nine"})
	if got := s.Sorted()[0].Message; got != "nine" {
		t.Errorf("first = %q, want %q", got, "nine")
	}
}

// The JSON shape is a contract with CI jobs and agents. Both keys, and findings
// as an array even when empty so a caller can index it without a nil check.
func TestDocumentShapeIsFrozen(t *testing.T) {
	var empty Set
	b, err := json.Marshal(empty.Document())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if got := string(b); got != `{"findings":[],"count":0}` {
		t.Errorf("empty document = %s, want {\"findings\":[],\"count\":0}", got)
	}

	var s Set
	s.Addf("a.md", 3, "ears.missing-shall", "requirement %s has no `shall`", "R1.2")
	b, err = json.Marshal(s.Document())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var doc struct {
		Findings []map[string]any `json:"findings"`
		Count    int              `json:"count"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if doc.Count != 1 || len(doc.Findings) != 1 {
		t.Fatalf("document = %s, want one finding and count 1", b)
	}
	for _, key := range []string{"file", "line", "rule", "message"} {
		if _, ok := doc.Findings[0][key]; !ok {
			t.Errorf("finding is missing key %q: %s", key, b)
		}
	}
	if doc.Findings[0]["message"] != "requirement R1.2 has no `shall`" {
		t.Errorf("Addf did not format the message: %s", b)
	}
}

// A file-level finding carries no line, and the key is omitted rather than
// reported as line 0 — which would read as a real location.
func TestLineIsOmittedWhenAbsent(t *testing.T) {
	var s Set
	s.Addf("a.md", 0, "wiki.orphan-page", "not reachable from the index")
	b, _ := json.Marshal(s.Document())
	if strings.Contains(string(b), `"line"`) {
		t.Errorf("line 0 was serialized: %s", b)
	}
}

// The exit-code contract: a finding is a legitimate answer, not a tool failure.
func TestExitCode(t *testing.T) {
	var s Set
	if got := s.ExitCode(); got != ExitOK {
		t.Errorf("empty set exit = %d, want %d", got, ExitOK)
	}
	s.Addf("a.md", 1, "r", "x")
	if got := s.ExitCode(); got != ExitFindings {
		t.Errorf("non-empty set exit = %d, want %d", got, ExitFindings)
	}
	if ExitOK != 0 || ExitFindings != 2 {
		t.Errorf("exit codes drifted: ok=%d findings=%d", ExitOK, ExitFindings)
	}
}

// `scc validate` is eight validators reporting through one exit code and one
// document, so merging has to be a first-class operation.
func TestExtend(t *testing.T) {
	var a, b Set
	a.Addf("a.md", 1, "r1", "one")
	b.Addf("b.md", 1, "r2", "two")
	a.Extend(&b)
	a.Extend(nil) // a validator with nothing to say may return nil
	if a.Len() != 2 {
		t.Errorf("Len = %d, want 2", a.Len())
	}
}

// The same artifact must report the same location on Windows and Linux, or a
// finding's identity depends on who ran the validator.
func TestRelIsSlashSeparated(t *testing.T) {
	got := Rel(filepath.Join("specs", "user-auth", "tasks.md"))
	if got != "specs/user-auth/tasks.md" {
		t.Errorf("Rel = %q, want slash-separated", got)
	}
	if Rel(`specs\user-auth\tasks.md`) != "specs/user-auth/tasks.md" {
		t.Errorf("Rel did not normalize backslashes: %q", Rel(`specs\user-auth\tasks.md`))
	}
}
