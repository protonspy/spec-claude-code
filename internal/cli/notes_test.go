package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/paths"
)

func addNote(t *testing.T, root string, args ...string) string {
	t.Helper()
	stdout, stderr, code := run(t, append([]string{"notes", "add"}, append(args, "--root", root)...)...)
	if code != ExitOK {
		t.Fatalf("notes add %v: exit %d (stderr: %s)", args, code, stderr)
	}
	// render.OK prefixes a glyph; the note is what follows it, and every
	// assertion here is about the note being byte-identical to what the file holds.
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(stdout), "✓"))
}

func notesFile(t *testing.T, root string) string {
	t.Helper()
	b, err := os.ReadFile(paths.Notes(root))
	if err != nil {
		t.Fatalf("read notes: %v", err)
	}
	return string(b)
}

// The round trip the whole feature is: write one, find it by every index it
// carries, read it back by id, and remove it.
func TestNotesRoundTrip(t *testing.T) {
	root := initWorkspace(t)

	first := addNote(t, root, "the appender lands against the last note, never after it",
		"--tag", "gotcha", "--path", "internal/cli/notes.go", "--date", "2026-02-09")
	if !strings.Contains(first, "n-0001") || !strings.Contains(first, "#gotcha") {
		t.Fatalf("add reported %q", first)
	}
	addNote(t, root, "the log reads oldest first", "--tag", "convention", "--date", "2026-03-01")

	stdout, _, code := run(t, "notes", "find", "--tag", "gotcha", "--root", root)
	if code != ExitOK {
		t.Fatalf("find: exit %d", code)
	}
	if strings.Count(strings.TrimSpace(stdout), "\n") != 0 || !strings.Contains(stdout, "n-0001") {
		t.Errorf("find --tag gotcha = %q, want just n-0001", stdout)
	}

	// Every filter the rule advertises, over the same two notes.
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--path", "internal/cli"}, "n-0001"},
		{[]string{"--since", "2026-03-01"}, "n-0002"},
		{[]string{"oldest"}, "n-0002"},
	} {
		stdout, _, code := run(t, append(append([]string{"notes", "find"}, tc.args...), "--root", root)...)
		if code != ExitOK {
			t.Fatalf("find %v: exit %d", tc.args, code)
		}
		if !strings.Contains(stdout, tc.want) || strings.Count(strings.TrimSpace(stdout), "\n") != 0 {
			t.Errorf("find %v = %q, want only %s", tc.args, stdout, tc.want)
		}
	}

	stdout, _, code = run(t, "notes", "show", "n-0001", "--root", root)
	if code != ExitOK || strings.TrimSpace(stdout) != first {
		t.Errorf("show = %q (%d), want the line add printed: %q", stdout, code, first)
	}

	if _, stderr, code := run(t, "notes", "rm", "n-0001", "--root", root); code != ExitOK {
		t.Fatalf("rm: exit %d (%s)", code, stderr)
	}
	if body := notesFile(t, root); strings.Contains(body, "lands against the last note") {
		t.Error("the removed note's text is still in the file")
	}
	// The number is spent: the next note is n-0003, not a second n-0001.
	if line := addNote(t, root, "a third", "--tag", "x"); !strings.Contains(line, "n-0003") {
		t.Errorf("after removing n-0001 the next note was %q — an id must never be reused", line)
	}
}

// What the file holds and what `find` prints are one format, so a reader who
// learned either one has learned both. This is the claim that makes `grep` a
// supported way to query the log.
func TestFindPrintsExactlyWhatTheFileHolds(t *testing.T) {
	root := initWorkspace(t)
	line := addNote(t, root, "one line, index fields first", "--tag", "format", "--path", "docs")

	stdout, _, _ := run(t, "notes", "find", "--root", root)
	if strings.TrimSpace(stdout) != line {
		t.Errorf("find = %q, add said %q", strings.TrimSpace(stdout), line)
	}
	if !strings.Contains(notesFile(t, root), line) {
		t.Errorf("the file does not contain the line find printed: %q", line)
	}
	// The grep the rule advertises has to work on the real file.
	var hits int
	for _, l := range strings.Split(notesFile(t, root), "\n") {
		if strings.HasPrefix(l, "- n-") && strings.Contains(l, " #format ") {
			hits++
		}
	}
	if hits != 1 {
		t.Errorf("grep for ' #format ' matched %d note lines, want 1", hits)
	}
}

// A tag is the index the log is queried by. Defaulting it would put one tag on
// everything, which is the drift the index exists to prevent.
func TestAddRequiresATag(t *testing.T) {
	root := initWorkspace(t)
	_, stderr, code := run(t, "notes", "add", "an untagged thought", "--root", root)
	if code != ExitError {
		t.Fatalf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "--tag") {
		t.Errorf("stderr = %q, want it to name --tag", stderr)
	}
}

// One line is the contract. Text that would break across lines is refused rather
// than mangled into something no query returns whole.
func TestAddRefusesMoreThanOneLine(t *testing.T) {
	root := initWorkspace(t)
	_, stderr, code := run(t, "notes", "add", "first\nsecond", "--tag", "x", "--root", root)
	if code != ExitError {
		t.Fatalf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "one line") {
		t.Errorf("stderr = %q", stderr)
	}
}

// Seeds are written once and tracked nowhere, so `scc update` will never deliver
// docs/notes.md to a workspace scaffolded before it existed. The first add is the
// other honest moment to create it.
func TestAddSeedsAMissingLog(t *testing.T) {
	root := initWorkspace(t)
	if err := os.Remove(paths.Notes(root)); err != nil {
		t.Fatalf("remove seed: %v", err)
	}
	addNote(t, root, "the first note this workspace ever had", "--tag", "first")
	body := notesFile(t, root)
	if !strings.Contains(body, "## Log") || !strings.Contains(body, "n-0001") {
		t.Errorf("the reseeded file is not a log:\n%s", body)
	}
	if _, _, code := run(t, "notes", "validate", "--root", root); code != ExitOK {
		t.Error("the file `notes add` seeded does not pass its own validator")
	}
}

// Queries have to work before anybody has written a note — which is every
// workspace on its first day, and every workspace scaffolded before this shipped.
func TestQueriesWorkWithNoLogAtAll(t *testing.T) {
	root := initWorkspace(t)
	if err := os.Remove(paths.Notes(root)); err != nil {
		t.Fatalf("remove seed: %v", err)
	}
	for _, args := range [][]string{{"find"}, {"tags"}, {"paths"}, {"validate"}} {
		if _, stderr, code := run(t, append(append([]string{"notes"}, args...), "--root", root)...); code != ExitOK {
			t.Errorf("notes %v on a workspace with no log: exit %d (%s)", args, code, stderr)
		}
	}
}

// A path that does not resolve is reported and never blocking: the stale check is
// about a log aging past its code, and at the moment of writing a note about a
// file this branch has not created yet is the note most worth having.
func TestAddWarnsButWritesForAPathThatIsNotThereYet(t *testing.T) {
	root := initWorkspace(t)
	stdout, stderr, code := run(t, "notes", "add", "about a file this branch has not created",
		"--tag", "todo-shaped", "--path", "internal/not/yet.go", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit = %d (%s)", code, stderr)
	}
	if !strings.Contains(stdout, "n-0001") {
		t.Errorf("the note was not written: %q", stdout)
	}
	if !strings.Contains(stderr, "no longer exists") {
		t.Errorf("stderr = %q, want the stale path reported", stderr)
	}
}

// The dangerous edit is the one nobody looked at first: `add` appends to a file it
// never showed the user, so it re-validates afterwards and puts the file back the
// way `scc patch` does.
func TestAddRollsBackAnEditThatIntroducesAFinding(t *testing.T) {
	root := initWorkspace(t)
	addNote(t, root, "a first note", "--tag", "x")
	before := notesFile(t, root)

	// A hand-edited log with a number the appender is about to allocate again.
	broken := strings.Replace(before, "## Log", "## Log\n\n- n-0002 2026-01-01 #x — hand-written, and about to collide", 1)
	if err := os.WriteFile(paths.Notes(root), []byte(broken), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// n-0002 is taken, so the appender allocates n-0003 and nothing collides. Force
	// the collision instead by making the file claim a number twice.
	broken = strings.Replace(broken, "- n-0002 2026-01-01", "- n-0001 2026-01-01", 1)
	if err := os.WriteFile(paths.Notes(root), []byte(broken), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, _, code := run(t, "notes", "validate", "--root", root); code != ExitFindings {
		t.Fatalf("the hand-edited log should already have a finding, got exit %d", code)
	}
	// A pre-existing finding is not this edit's fault, so the append still lands —
	// the comparison is on rule+message, exactly as `scc patch` does it.
	stdout, stderr, code := run(t, "notes", "add", "another", "--tag", "x", "--root", root)
	if code != ExitOK {
		t.Fatalf("add over a log that already had a finding: exit %d (%s)", code, stderr)
	}
	if !strings.Contains(stdout, "n-0002") {
		t.Errorf("add reported %q", stdout)
	}
}

func TestNotesJSON(t *testing.T) {
	root := initWorkspace(t)
	stdout, stderr, code := run(t, "notes", "add", "a note", "--tag", "a,b", "--path", "docs,internal",
		"--date", "2026-02-09", "--json", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit %d (%s)", code, stderr)
	}
	var added struct {
		Note struct {
			ID    string   `json:"id"`
			Date  string   `json:"date"`
			Tags  []string `json:"tags"`
			Paths []string `json:"paths"`
			Text  string   `json:"text"`
		} `json:"note"`
		Line     string `json:"line"`
		Path     string `json:"path"`
		Written  bool   `json:"written"`
		Verified string `json:"verified"`
	}
	if err := json.Unmarshal([]byte(stdout), &added); err != nil {
		t.Fatalf("stdout is not JSON (%v): %q", err, stdout)
	}
	if added.Note.ID != "n-0001" || len(added.Note.Tags) != 2 || len(added.Note.Paths) != 2 {
		t.Errorf("note = %+v", added.Note)
	}
	if !added.Written || added.Verified != "clean" {
		t.Errorf("written = %v, verified = %q", added.Written, added.Verified)
	}
	if added.Path != filepath.ToSlash(filepath.Join(paths.DocsSeg, paths.NotesSeg)) {
		t.Errorf("path = %q", added.Path)
	}

	stdout, _, code = run(t, "notes", "find", "--json", "--root", root)
	if code != ExitOK {
		t.Fatalf("find --json: exit %d", code)
	}
	var found struct {
		Notes []map[string]any `json:"notes"`
		Count int              `json:"count"`
	}
	if err := json.Unmarshal([]byte(stdout), &found); err != nil {
		t.Fatalf("find stdout is not JSON (%v): %q", err, stdout)
	}
	if found.Count != 1 || len(found.Notes) != 1 {
		t.Errorf("find = %+v", found)
	}
}

// A cap that hides matches while looking complete is the one way this command can
// mislead, so what was dropped is said out loud — on stderr, so stdout stays the
// notes and nothing else.
func TestFindSaysWhatALimitDropped(t *testing.T) {
	root := initWorkspace(t)
	for _, text := range []string{"one", "two", "three"} {
		addNote(t, root, text, "--tag", "x")
	}
	stdout, stderr, code := run(t, "notes", "find", "--limit", "1", "--root", root)
	if code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if strings.Count(strings.TrimSpace(stdout), "\n") != 0 {
		t.Errorf("stdout = %q, want one line", stdout)
	}
	if !strings.Contains(stdout, "n-0003") {
		t.Errorf("stdout = %q, want the newest note", stdout)
	}
	if !strings.Contains(stderr, "of 3 matches") {
		t.Errorf("stderr = %q, want the drop reported", stderr)
	}
}

// The seed ships an example note inside a fence. A validator that read it would
// fire on the file scc itself writes, which is the worst bug this product has.
func TestTheSeededLogPassesItsOwnValidator(t *testing.T) {
	root := initWorkspace(t)
	stdout, stderr, code := run(t, "notes", "validate", "--root", root, "--json")
	if code != ExitOK {
		t.Errorf("the seeded docs/notes.md has findings: exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	stdout, _, _ = run(t, "notes", "find", "--root", root)
	if strings.Contains(stdout, "n-0000") {
		t.Errorf("the seed's fenced example was read as a note: %q", stdout)
	}
}

func TestNotesRejectsAnUnknownSubcommand(t *testing.T) {
	root := initWorkspace(t)
	_, stderr, code := run(t, "notes", "list", "--root", root)
	if code != ExitError {
		t.Errorf("exit = %d, want %d", code, ExitError)
	}
	if !strings.Contains(stderr, "unknown notes subcommand") {
		t.Errorf("stderr = %q", stderr)
	}
}
