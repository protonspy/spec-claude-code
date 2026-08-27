package validate

import (
	"path/filepath"
	"testing"

	"github.com/protonspy/spec-claude-code/internal/paths"
)

// notesLog writes docs/notes.md with the given lines under the heading, and a
// fenced example above it — the shape the seed actually ships, so every test here
// also asserts the seed's example is not read as a note.
func notesLog(t *testing.T, root string, lines ...string) {
	t.Helper()
	body := "# Notes\n\nGuidance, with an example:\n\n```markdown\n" +
		"- n-0000 2020-01-01 #example @gone.go — an example, inside a fence\n```\n\n## Log\n\n"
	for _, l := range lines {
		body += l + "\n"
	}
	write(t, paths.Notes(root), body)
}

func TestNotesIsSilentWithoutALog(t *testing.T) {
	if got := runValidator(t, Notes, t.TempDir()); len(got) != 0 {
		t.Errorf("findings on a workspace with no notes: %v", got)
	}
}

func TestNotesAcceptsAWellFormedLog(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "internal", "cli", "notes.go"), "package cli\n")
	notesLog(t, root,
		"- n-0001 2026-02-09 #gotcha @internal/cli/notes.go — a note about a file that is there",
		"- n-0002 2026-03-01 #convention #cli — a note about nothing in particular",
	)
	if got := runValidator(t, Notes, root); len(got) != 0 {
		t.Errorf("findings on a clean log: %v", got)
	}
}

func TestNotesChecks(t *testing.T) {
	for _, tc := range []struct {
		name string
		line string
		want string
	}{
		{"a line that missed the grammar", "- somebody typed this by hand", "notes.malformed"},
		{"no tag to query it by", "- n-0001 2026-02-09 — untagged", "notes.untagged"},
		{"a tag the grammar cannot round-trip", "- n-0001 2026-02-09 #NotKebab — x", "notes.bad-tag"},
		{"a date that is not one", "- n-0001 2026-13-45 #x — x", "notes.bad-date"},
		{"a path that does not resolve", "- n-0001 2026-02-09 #x @internal/gone.go — x", "notes.stale-path"},
		{"a path that escapes the workspace", "- n-0001 2026-02-09 #x @../elsewhere — x", "notes.bad-path"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			notesLog(t, root, tc.line)
			got := runValidator(t, Notes, root)
			if !contains(got, tc.want) {
				t.Errorf("findings = %v, want %s", got, tc.want)
			}
		})
	}
}

// An id names one note, because an id is what a commit message or a spec cites.
func TestNotesReportsADuplicateID(t *testing.T) {
	root := t.TempDir()
	notesLog(t, root,
		"- n-0001 2026-02-09 #x — first",
		"- n-0001 2026-02-10 #x — second, wearing the first one's id",
	)
	if got := runValidator(t, Notes, root); !contains(got, "notes.duplicate-id") {
		t.Errorf("findings = %v, want notes.duplicate-id", got)
	}
}

// A well-formed note in the wrong place is worse than a malformed one: it looks
// right, and no query returns it.
func TestNotesReportsANoteOutsideTheLog(t *testing.T) {
	root := t.TempDir()
	write(t, paths.Notes(root), "# Notes\n\n- n-0001 2026-02-09 #x — stranded above the heading\n\n## Log\n")
	if got := runValidator(t, Notes, root); !contains(got, "notes.outside-log") {
		t.Errorf("findings = %v, want notes.outside-log", got)
	}
}

func TestNotesReportsAMissingLogSection(t *testing.T) {
	root := t.TempDir()
	write(t, paths.Notes(root), "# Notes\n\nJust prose, and nowhere for a note to go.\n")
	if got := runValidator(t, Notes, root); !contains(got, "notes.no-log-section") {
		t.Errorf("findings = %v, want notes.no-log-section", got)
	}
}

// A removed note leaves an HTML-comment tombstone so its number is never handed
// out twice. mdscan blanks comments, so nothing here may fire on one.
func TestNotesIgnoresATombstone(t *testing.T) {
	root := t.TempDir()
	notesLog(t, root, "<!-- n-0001 removed -->", "- n-0002 2026-02-09 #x — the one that stayed")
	if got := runValidator(t, Notes, root); len(got) != 0 {
		t.Errorf("findings on a log with a tombstone: %v", got)
	}
}
