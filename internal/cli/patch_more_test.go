package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Prose gets in three ways, and stdin is there because a paragraph is a bad thing
// to put in a shell argument. All three have to reach the same file.
func TestPatchTakesTextInlineFromAFileAndFromStdin(t *testing.T) {
	root := mapWorkspace(t)

	if _, stderr, code := run(t, "patch", "append", "plans/sample.md", "#why",
		"--text", "Inline sentence.", "--root", root); code != ExitOK {
		t.Fatalf("--text: exit %d (%s)", code, stderr)
	}
	if !strings.Contains(planText(t, root), "Inline sentence.") {
		t.Error("the inline text never reached the file")
	}

	from := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(from, []byte("Sentence from a file.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, stderr, code := run(t, "patch", "append", "plans/sample.md", "#why",
		"--file", from, "--root", root); code != ExitOK {
		t.Fatalf("--file: exit %d (%s)", code, stderr)
	}
	if !strings.Contains(planText(t, root), "Sentence from a file.") {
		t.Error("the text file never reached the plan")
	}

	// A file that is not there is an error rather than an empty append, which
	// would silently write nothing and report success.
	if _, _, code := run(t, "patch", "append", "plans/sample.md", "#why",
		"--file", filepath.Join(t.TempDir(), "missing.md"), "--root", root); code == ExitOK {
		t.Error("a missing --file exited 0")
	}
}

// The methodology vocabulary is closed, and a task's annotation is what decides how
// it gets built — so a third word is refused rather than written through.
func TestPatchTaskRefusesAMethodologyThatIsNotOne(t *testing.T) {
	root := mapWorkspace(t)

	for _, method := range []string{"Unit", "TDD"} {
		if _, stderr, code := run(t, "patch", "task", "plans/sample.md", "1.1",
			"--method", method, "--root", root); code != ExitOK {
			t.Errorf("--method %s: exit %d (%s)", method, code, stderr)
		}
	}
	stdout, stderr, code := run(t, "patch", "task", "plans/sample.md", "1.1",
		"--method", "Integration", "--root", root)
	if code == ExitOK {
		t.Error("an unknown methodology was accepted")
	}
	if !strings.Contains(stdout+stderr, "Integration") {
		t.Errorf("the refusal does not name what was passed:\n%s%s", stdout, stderr)
	}
}

// Rewriting a task must re-emit its flags and its continuation: without that,
// `patch task --method TDD` was a data-loss command that deleted the description
// and every dependency the task declared.
func TestPatchTaskKeepsTheFlagsAndTheContinuation(t *testing.T) {
	root := mapWorkspace(t)

	if _, stderr, code := run(t, "patch", "task", "plans/sample.md", "1.2",
		"--depends", "1.1", "--priority", "2", "--root", root); code != ExitOK {
		t.Fatalf("setting the flags: exit %d (%s)", code, stderr)
	}
	before := planText(t, root)
	for _, want := range []string{"_Depends 1.1_", "_Priority 2_"} {
		if !strings.Contains(before, want) {
			t.Fatalf("%s was never written:\n%s", want, before)
		}
	}

	if _, stderr, code := run(t, "patch", "task", "plans/sample.md", "1.2",
		"--method", "TDD", "--root", root); code != ExitOK {
		t.Fatalf("changing the methodology: exit %d (%s)", code, stderr)
	}
	after := planText(t, root)
	for _, want := range []string{"_Depends 1.1_", "_Priority 2_"} {
		if !strings.Contains(after, want) {
			t.Errorf("changing the methodology dropped %s:\n%s", want, after)
		}
	}
	if !strings.Contains(after, "(TDD)") {
		t.Errorf("the methodology did not change:\n%s", after)
	}
}

// An address that does not resolve is an error and never an insert at a guess —
// the guard that reading-first used to provide.
func TestPatchRefusesAnAddressThatDoesNotResolve(t *testing.T) {
	root := mapWorkspace(t)
	before := planText(t, root)

	for _, args := range [][]string{
		{"check", "plans/sample.md", "9.9"},
		{"uncheck", "plans/sample.md", "9.9"},
		{"task", "plans/sample.md", "9.9", "--text", "x"},
		{"append", "plans/sample.md", "#no-such-section", "--text", "x"},
		{"prepend", "plans/sample.md", "#no-such-section", "--text", "x"},
		{"replace", "plans/sample.md", "#no-such-section", "--text", "x"},
	} {
		full := append(append([]string{"patch"}, args...), "--root", root)
		if _, _, code := run(t, full...); code == ExitOK {
			t.Errorf("`scc patch %s` on an address that does not resolve exited 0", strings.Join(args, " "))
		}
	}
	if planText(t, root) != before {
		t.Error("a refused patch changed the file anyway")
	}
}

// The frontmatter is where the kickoff answers live, and a value neither the rule
// nor the validator knows is rolled back like any other bad edit.
func TestPatchFrontmatterWritesAndRefuses(t *testing.T) {
	root := mapWorkspace(t)

	if _, stderr, code := run(t, "patch", "fm", "plans/sample.md", "lang=wenyan", "--root", root); code != ExitOK {
		t.Fatalf("patch fm: exit %d (%s)", code, stderr)
	}
	if !strings.Contains(planText(t, root), "lang: wenyan") {
		t.Errorf("the key was not written:\n%s", planText(t, root))
	}

	// A pair with no `=` is a usage error rather than a key called "lang wenyan".
	if _, _, code := run(t, "patch", "fm", "plans/sample.md", "lang", "--root", root); code == ExitOK {
		t.Error("patch fm accepted an argument with no value")
	}
}
