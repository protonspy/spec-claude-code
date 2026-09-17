package validate

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// A pull request comment is the one part of the record nothing else reaches: no
// hook runs when a tool posts one, and the body was written long after the branch
// was pushed. `scc validate --pr` reads them for the same signatures it reads in
// the title and the body.
func TestAttributionPRReadsTheCommentsToo(t *testing.T) {
	root := gitRepo(t)
	const pr = `{"number":7,"state":"OPEN","headRefName":"feat/x","url":"https://example.invalid/pr/7","title":"feat: x","body":"why"}`
	const comments = `{"comments":[{"author":{"login":"someone"},"url":"https://example.invalid/pr/7#c1",` +
		`"body":"Part of this came from Claude Code."}],"reviews":[]}`
	stubGH(t, pr, comments)

	set, err := AttributionPR(root)
	if err != nil {
		t.Fatalf("AttributionPR: %v", err)
	}
	found := set.Sorted()
	if len(found) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(found), found)
	}
	// Filed under the pull request it is on, and named by who wrote it: a comment
	// has no title, and "PR #7" alone does not say which of ten comments to edit.
	if !strings.Contains(found[0].File, "#7") || !strings.Contains(found[0].Message, "someone") {
		t.Errorf("finding does not say where to look: %+v", found[0])
	}
}

// And a forge that answers about the pull request but not about its comments
// still gets the title and the body checked. The comment query degrades on its
// own, because half the record is better than none.
func TestAttributionPRSurvivesAForgeWithNoComments(t *testing.T) {
	root := gitRepo(t)
	const pr = `{"number":7,"state":"OPEN","headRefName":"feat/x","url":"https://example.invalid/pr/7",` +
		`"title":"feat: x","body":"Generated with Claude Code"}`
	stubGH(t, pr, "")

	set, err := AttributionPR(root)
	if err != nil {
		t.Fatalf("AttributionPR: %v", err)
	}
	if len(set.Sorted()) != 1 {
		t.Fatalf("the body's own footer went unreported: %+v", set.Sorted())
	}
}

// gitRepo is an empty repository, which is all IsRepo asks for.
func gitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = root
	if err := cmd.Run(); err != nil {
		t.Skip("git is not on PATH")
	}
	return root
}

// stubGH puts a gh on the front of PATH that answers the two queries this
// validator makes: one about the pull request, one about its comments. It tells
// them apart by the field list, which is the only thing that differs. An empty
// comments answer is a gh that exits non-zero, which is how a forge that cannot
// answer that question reports it.
func stubGH(t *testing.T, pr, comments string) {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		fail := "exit /b 1"
		if comments != "" {
			fail = "echo " + comments
		}
		body := "@echo off\r\n" +
			"echo %*| findstr /C:\"comments\" >nul\r\n" +
			"if errorlevel 1 (echo " + pr + ") else (" + fail + ")\r\n"
		writeStub(t, filepath.Join(dir, "gh.cmd"), body, 0o644)
	} else {
		fail := "exit 1"
		if comments != "" {
			fail = "printf '%s\n' " + quote(comments)
		}
		body := "#!/bin/sh\ncase \"$*\" in\n  *comments*) " + fail + " ;;\n" +
			"  *) printf '%s\n' " + quote(pr) + " ;;\nesac\n"
		writeStub(t, filepath.Join(dir, "gh"), body, 0o755)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func writeStub(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// A comment body is the least trusted text this validator reads: anybody with an
// account can post one, on somebody else's pull request, and pre-push prints what
// it finds to a real terminal. A raw ESC surviving that trip is a control sequence
// the reader's emulator obeys — enough to scroll the finding away and paint a
// clean report over the top of it, which is the one lie this check exists to
// prevent.
func TestAPRCommentCannotWriteControlSequencesToTheTerminal(t *testing.T) {
	root := gitRepo(t)
	const pr = `{"number":7,"state":"OPEN","headRefName":"feat/x","url":"https://example.invalid/pr/7","title":"feat: x","body":"why"}`
	// A screen-clearing sequence, a bell, and a right-to-left override, on a line
	// that is a signature by any reading. Built from code points rather than typed,
	// so the bytes under test survive every editor between here and the run — and
	// marshalled, because that is how the forge hands them over.
	esc, bel, rlo := string(rune(0x1b)), string(rune(0x07)), string(rune(0x202e))
	body := esc + "[2J" + esc + "[Hno findings" + bel + " generated with Claude Code" + rlo
	comments, err := json.Marshal(map[string]any{
		"comments": []map[string]any{{
			"author": map[string]string{"login": "someone"},
			"url":    "https://example.invalid/pr/7#c1",
			"body":   body,
		}},
		"reviews": []any{},
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	stubGH(t, pr, string(comments))

	set, err := AttributionPR(root)
	if err != nil {
		t.Fatalf("AttributionPR: %v", err)
	}
	found := set.Sorted()
	if len(found) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(found), found)
	}
	for _, r := range found[0].Message {
		if !unicode.IsGraphic(r) && r != ' ' {
			t.Fatalf("finding carries %U to the terminal: %q", r, found[0].Message)
		}
	}
	// And the line is still reported. Escaping it must not cost the reader the
	// evidence it was written to show them.
	if !strings.Contains(found[0].Message, "Claude Code") {
		t.Errorf("the offending line went missing: %q", found[0].Message)
	}
}

// clip is the one place that escaping happens, so the two edges it owns are
// pinned here: a cut on a rune boundary rather than a byte one, and text that was
// already clean coming back the way it went in.
func TestClipCutsRunesAndLeavesCleanTextAlone(t *testing.T) {
	if got := clip("  feat:   a  line  "); got != "feat: a line" {
		t.Errorf("clip collapsed to %q", got)
	}
	long := clip(strings.Repeat("é", 200))
	if !utf8.ValidString(long) {
		t.Errorf("clip cut a rune in half: %q", long)
	}
	if n := utf8.RuneCountInString(long); n != 64 {
		t.Errorf("clip returned %d runes, want 64", n)
	}
}
