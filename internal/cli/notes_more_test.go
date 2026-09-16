package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func notesText(t *testing.T, root string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "docs", "notes.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return string(b)
}

// What `scc notes find` adds over grep is the questions a substring cannot answer:
// which tags exist, what this project knows about a path, and what is new since a
// date.
func TestNotesFindAnswersWhatGrepCannot(t *testing.T) {
	root := mapWorkspace(t)

	add := func(text, tag string, paths ...string) {
		t.Helper()
		args := []string{"notes", "add", text, "--tag", tag, "--root", root}
		if len(paths) > 0 {
			args = append(args, "--path", strings.Join(paths, ","))
		}
		if _, stderr, code := run(t, args...); code != ExitOK {
			t.Fatalf("notes add: exit %d (%s)", code, stderr)
		}
	}
	add("wrap writes MCP config to the agent's own file", "gotcha", "internal/cli/launch.go")
	add("global lock; per-account locks if throughput matters", "ceiling", "internal/cli/patch.go")
	add("the race detector needs cgo", "platform", "internal/cli/launch.go")

	stdout, _, code := run(t, "notes", "tags", "--root", root)
	if code != ExitOK {
		t.Fatalf("notes tags: exit %d", code)
	}
	for _, tag := range []string{"gotcha", "ceiling", "platform"} {
		if !strings.Contains(stdout, tag) {
			t.Errorf("notes tags does not list %q:\n%s", tag, stdout)
		}
	}

	stdout, _, code = run(t, "notes", "find", "--tag", "gotcha", "--root", root)
	if code != ExitOK {
		t.Fatalf("notes find --tag: exit %d", code)
	}
	if !strings.Contains(stdout, "MCP config") || strings.Contains(stdout, "global lock") {
		t.Errorf("--tag gotcha returned the wrong notes:\n%s", stdout)
	}

	stdout, _, code = run(t, "notes", "find", "--path", "internal/cli/launch.go", "--root", root)
	if code != ExitOK {
		t.Fatalf("notes find --path: exit %d", code)
	}
	if !strings.Contains(stdout, "MCP config") || !strings.Contains(stdout, "race detector") {
		t.Errorf("--path did not return both notes about that file:\n%s", stdout)
	}

	stdout, _, code = run(t, "notes", "paths", "--root", root)
	if code != ExitOK {
		t.Fatalf("notes paths: exit %d", code)
	}
	if !strings.Contains(stdout, "launch.go") {
		t.Errorf("notes paths does not list the paths notes are attached to:\n%s", stdout)
	}

	// A term search still works, and a term nothing matches is an answer rather
	// than a failure.
	if _, _, code := run(t, "notes", "find", "throughput", "--root", root); code != ExitOK {
		t.Errorf("a term search exited %d", code)
	}
	if _, _, code := run(t, "notes", "find", "nothing-matches-this", "--root", root); code != ExitOK {
		t.Errorf("a term nothing matches exited %d, and an empty result is an answer", code)
	}
}

// `--tag` is required and has no default, because a default would be one tag on
// everything, which is the drift the index exists to prevent.
func TestNotesAddRequiresATag(t *testing.T) {
	root := mapWorkspace(t)

	if _, _, code := run(t, "notes", "add", "a note with no tag", "--root", root); code == ExitOK {
		t.Error("notes add with no --tag exited 0")
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "notes.md")); err == nil {
		if strings.Contains(notesText(t, root), "a note with no tag") {
			t.Error("the untagged note was written anyway")
		}
	}
}

// A number is spent, never reused: rm takes the text out and leaves an HTML-comment
// tombstone, so a citation to n-0001 can dangle but can never come to mean a
// different note.
func TestNotesRemoveLeavesATombstoneAndTheNumberIsSpent(t *testing.T) {
	root := mapWorkspace(t)

	if _, stderr, code := run(t, "notes", "add", "the first note", "--tag", "gotcha", "--root", root); code != ExitOK {
		t.Fatalf("notes add: exit %d (%s)", code, stderr)
	}
	first := notesText(t, root)
	if !strings.Contains(first, "n-0001") {
		t.Fatalf("the first note is not n-0001:\n%s", first)
	}

	stdout, _, code := run(t, "notes", "show", "n-0001", "--root", root)
	if code != ExitOK {
		t.Fatalf("notes show: exit %d", code)
	}
	if !strings.Contains(stdout, "the first note") {
		t.Errorf("notes show did not return the note:\n%s", stdout)
	}

	if _, stderr, code := run(t, "notes", "rm", "n-0001", "--root", root); code != ExitOK {
		t.Fatalf("notes rm: exit %d (%s)", code, stderr)
	}
	after := notesText(t, root)
	if strings.Contains(after, "the first note") {
		t.Errorf("rm left the text behind:\n%s", after)
	}
	if !strings.Contains(after, "n-0001") {
		t.Errorf("rm left no tombstone, so the number could be reused:\n%s", after)
	}

	// The next note gets the next number rather than the spent one.
	if _, _, code := run(t, "notes", "add", "the second note", "--tag", "gotcha", "--root", root); code != ExitOK {
		t.Fatalf("notes add: exit %d", code)
	}
	if !strings.Contains(notesText(t, root), "n-0002") {
		t.Errorf("the next note did not get n-0002:\n%s", notesText(t, root))
	}

	// And an id nothing answers to is a miss rather than an empty success.
	if _, _, code := run(t, "notes", "show", "n-9999", "--root", root); code == ExitOK {
		t.Error("notes show on an id nothing answers to exited 0")
	}
	if _, _, code := run(t, "notes", "rm", "n-9999", "--root", root); code == ExitOK {
		t.Error("notes rm on an id nothing answers to exited 0")
	}
}

// The ninth validator reports the failure this file cannot tolerate quietly: a
// hand-written line that missed the grammar, which no query will ever return.
func TestNotesValidateReportsAHandWrittenLine(t *testing.T) {
	root := mapWorkspace(t)
	if _, _, code := run(t, "notes", "add", "a well-formed note", "--tag", "gotcha", "--root", root); code != ExitOK {
		t.Fatalf("notes add: exit %d", code)
	}
	if _, _, code := run(t, "notes", "validate", "--root", root); code != ExitOK {
		t.Errorf("a well-formed log reported findings")
	}

	path := filepath.Join(root, "docs", "notes.md")
	body := notesText(t, root) + "- this line missed the grammar entirely\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, _, code := run(t, "notes", "validate", "--root", root); code != ExitFindings {
		t.Errorf("a malformed line exited %d, want %d", code, ExitFindings)
	}
}
