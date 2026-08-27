package notes

import (
	"strings"
	"testing"
)

// A minimal log: guidance above, notes below the heading. Every test builds on
// this rather than on a hand-rolled string, because the region rule — what counts
// as a note and what is prose the user owns — is the thing most of them are about.
const log = `# Notes

Guidance the user owns, with an example that must not be read as a note:

` + "```" + `markdown
- n-0000 2020-01-01 #example @nowhere.go — an example inside a fence
` + "```" + `

## Log

<!-- notes below -->
- n-0001 2026-02-09 #gotcha @internal/cli/launch.go — wrap writes MCP config, so it outlives the session
- n-0002 2026-03-01 #convention #cli @internal/cli — the log reads oldest first
`

func parse(t *testing.T, content string) *File {
	t.Helper()
	f, err := Parse("docs/notes.md", content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return f
}

func TestParseReadsEveryField(t *testing.T) {
	f := parse(t, log)
	if len(f.Notes) != 2 {
		t.Fatalf("notes = %d, want 2: %+v", len(f.Notes), f.Notes)
	}
	n := f.Notes[0]
	if n.ID != "n-0001" || n.Num != 1 {
		t.Errorf("id = %q/%d, want n-0001/1", n.ID, n.Num)
	}
	if n.Date != "2026-02-09" {
		t.Errorf("date = %q", n.Date)
	}
	if strings.Join(n.Tags, ",") != "gotcha" {
		t.Errorf("tags = %v", n.Tags)
	}
	if strings.Join(n.Paths, ",") != "internal/cli/launch.go" {
		t.Errorf("paths = %v", n.Paths)
	}
	if n.Text != "wrap writes MCP config, so it outlives the session" {
		t.Errorf("text = %q", n.Text)
	}
	if n.Line != 12 {
		t.Errorf("line = %d, want 12", n.Line)
	}
}

// The seed carries an example note, and a validator that read it would fire on the
// file scc itself writes — the worst bug this product can ship. mdscan blanks
// fenced blocks, and this is the test that says so for notes.
func TestAnExampleInAFenceIsNotANote(t *testing.T) {
	f := parse(t, log)
	for _, n := range append(append([]Note{}, f.Notes...), f.Outside...) {
		if n.ID == "n-0000" {
			t.Fatalf("the fenced example was parsed as a note: %s", n.Format())
		}
	}
}

// The one failure this file cannot tolerate quietly: a hand-written line that
// missed the grammar is invisible to every query, so it has to be reported.
func TestALooseLineInTheLogIsReported(t *testing.T) {
	f := parse(t, log+"- a bullet somebody typed by hand\n")
	if len(f.Loose) != 1 {
		t.Fatalf("loose = %+v, want one", f.Loose)
	}
	if !strings.Contains(f.Loose[0].Text, "typed by hand") {
		t.Errorf("loose text = %q", f.Loose[0].Text)
	}
}

// Prose above the heading is the user's, and reporting on it would make the seed
// itself a finding.
func TestProseAboveTheHeadingIsNotJudged(t *testing.T) {
	f := parse(t, "# Notes\n\n- a bullet in the guidance\n\n## Log\n")
	if len(f.Loose) != 0 {
		t.Errorf("loose = %+v, want none: guidance is not the log", f.Loose)
	}
}

// A well-formed note outside the region is worse than a malformed one inside it:
// it looks right and no query returns it.
func TestANoteOutsideTheLogIsFound(t *testing.T) {
	f := parse(t, "# Notes\n\n- n-0007 2026-01-01 #x — stranded above the heading\n\n## Log\n")
	if len(f.Outside) != 1 || len(f.Notes) != 0 {
		t.Fatalf("outside = %d, notes = %d, want 1 and 0", len(f.Outside), len(f.Notes))
	}
}

// Numbers are spent, not counted: a citation to n-0002 must never come to mean a
// different note because the first one was removed.
func TestNextIsTheHighWaterMark(t *testing.T) {
	f := parse(t, log)
	if got := f.Next(); got != 3 {
		t.Errorf("next = %d, want 3", got)
	}
	content, _, err := f.Remove("n-0002")
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got := parse(t, content).Next(); got != 3 {
		t.Errorf("next after removing the last note = %d, want 3 — numbers are never reused", got)
	}
}

func TestNextCountsNotesOutsideTheLog(t *testing.T) {
	f := parse(t, "# Notes\n\n- n-0009 2026-01-01 #x — stranded\n\n## Log\n")
	if got := f.Next(); got != 10 {
		t.Errorf("next = %d, want 10: a stranded note has still spent its number", got)
	}
}

func TestAppendLandsAgainstTheLastNote(t *testing.T) {
	f := parse(t, log)
	content, err := f.Append(Note{ID: "n-0003", Date: "2026-04-01", Tags: []string{"x"}, Text: "third"})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if last := lines[len(lines)-1]; last != "- n-0003 2026-04-01 #x — third" {
		t.Errorf("last line = %q", last)
	}
	if got := len(parse(t, content).Notes); got != 3 {
		t.Errorf("notes after append = %d, want 3", got)
	}
}

// The log is not always the last section. Appending after whatever follows it
// would put the note where no query looks.
func TestAppendStaysInsideTheLogSection(t *testing.T) {
	f := parse(t, log+"\n## Afterwards\n\nsomething else entirely\n")
	content, err := f.Append(Note{ID: "n-0003", Date: "2026-04-01", Tags: []string{"x"}, Text: "third"})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	after := parse(t, content)
	if len(after.Notes) != 3 || len(after.Outside) != 0 {
		t.Fatalf("notes = %d, outside = %d, want 3 and 0\n%s", len(after.Notes), len(after.Outside), content)
	}
}

// Without a heading there is nowhere a note could go that a query would find, so
// the write is refused rather than guessed at.
func TestAppendRefusesAFileWithNoLogSection(t *testing.T) {
	f := parse(t, "# Notes\n\njust prose\n")
	if _, err := f.Append(Note{ID: "n-0001", Date: "2026-01-01", Tags: []string{"x"}, Text: "y"}); err == nil {
		t.Fatal("append into a file with no log section succeeded")
	}
}

func TestFormatRoundTrips(t *testing.T) {
	n := Note{ID: "n-0042", Date: "2026-05-05", Tags: []string{"a", "b"}, Paths: []string{"x/y.go", "z"}, Text: "text — with an em dash in it"}
	f := parse(t, "# Notes\n\n## Log\n\n"+n.Format()+"\n")
	if len(f.Notes) != 1 {
		t.Fatalf("a formatted note did not parse back: %q", n.Format())
	}
	got := f.Notes[0]
	got.Line = 0
	got.Num = 0
	if got.Format() != n.Format() {
		t.Errorf("round trip: %q -> %q", n.Format(), got.Format())
	}
}

func TestMatch(t *testing.T) {
	f := parse(t, log)
	for _, tc := range []struct {
		name string
		q    Query
		want []string
	}{
		{"everything", Query{}, []string{"n-0001", "n-0002"}},
		{"one tag", Query{Tags: []string{"gotcha"}}, []string{"n-0001"}},
		{"tags are an or", Query{Tags: []string{"gotcha", "cli"}}, []string{"n-0001", "n-0002"}},
		{"an unknown tag matches nothing", Query{Tags: []string{"nope"}}, nil},
		{"an exact path", Query{Paths: []string{"internal/cli/launch.go"}}, []string{"n-0001"}},
		{"a directory covers what is under it", Query{Paths: []string{"internal"}}, []string{"n-0001", "n-0002"}},
		{"terms are an and", Query{Terms: []string{"log", "oldest"}}, []string{"n-0002"}},
		{"terms are case-insensitive", Query{Terms: []string{"WRAP"}}, []string{"n-0001"}},
		{"since drops what is older", Query{Since: "2026-03-01"}, []string{"n-0002"}},
		{"filters compose as an and", Query{Tags: []string{"gotcha"}, Terms: []string{"oldest"}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, n := range f.Match(tc.q) {
				got = append(got, n.ID)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("match = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTagsAndPathsAreOrderedByUse(t *testing.T) {
	f := parse(t, log)
	tags := f.Tags()
	if len(tags) != 3 || tags[0].Name != "cli" {
		// cli, convention, gotcha — one each, so alphabetical decides.
		t.Errorf("tags = %+v", tags)
	}
	if paths := f.Paths(); len(paths) != 2 {
		t.Errorf("paths = %+v", paths)
	}
}

func TestCheckPath(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		ok       bool
	}{
		{"internal/cli/notes.go", "internal/cli/notes.go", true},
		{`internal\cli\notes.go`, "internal/cli/notes.go", true},
		{"./internal/cli", "internal/cli", true},
		{"/etc/passwd", "", false},
		{`C:\Windows`, "", false},
		{"../outside", "", false},
		{"has space", "", false},
		{"", "", false},
	} {
		got, err := CheckPath(tc.in)
		if tc.ok != (err == nil) {
			t.Errorf("CheckPath(%q) error = %v, want ok=%v", tc.in, err, tc.ok)
			continue
		}
		if tc.ok && got != tc.want {
			t.Errorf("CheckPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCheckTag(t *testing.T) {
	for _, ok := range []string{"gotcha", "code-review", "v2"} {
		if err := CheckTag(ok); err != nil {
			t.Errorf("CheckTag(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"Gotcha", "two words", "trailing-", "", "under_score", "#hash"} {
		if err := CheckTag(bad); err == nil {
			t.Errorf("CheckTag(%q) accepted", bad)
		}
	}
}

// One line is the contract the whole file rests on: a match has to be a whole
// note, so text that would break across lines is refused rather than mangled.
func TestCheckTextRefusesMoreThanOneLine(t *testing.T) {
	if _, err := CheckText("first\nsecond"); err == nil {
		t.Error("multi-line text accepted")
	}
	if _, err := CheckText("first\r\nsecond"); err == nil {
		t.Error("CRLF text accepted")
	}
	if _, err := CheckText("   "); err == nil {
		t.Error("empty text accepted")
	}
	got, err := CheckText("  one line  ")
	if err != nil || got != "one line" {
		t.Errorf("CheckText = %q, %v", got, err)
	}
}

func TestID(t *testing.T) {
	if got := ID(7); got != "n-0007" {
		t.Errorf("ID(7) = %q", got)
	}
	if got := ID(12345); got != "n-12345" {
		t.Errorf("ID(12345) = %q — ids grow rather than wrap", got)
	}
}

// A CRLF checkout must produce the same notes at the same line numbers as a LF
// one, or a finding's line number depends on how git checked the file out.
func TestCRLFParsesIdentically(t *testing.T) {
	lf := parse(t, log)
	crlf := parse(t, strings.ReplaceAll(log, "\n", "\r\n"))
	if len(lf.Notes) != len(crlf.Notes) {
		t.Fatalf("notes = %d vs %d", len(lf.Notes), len(crlf.Notes))
	}
	for i := range lf.Notes {
		if lf.Notes[i].Format() != crlf.Notes[i].Format() || lf.Notes[i].Line != crlf.Notes[i].Line {
			t.Errorf("note %d differs: %q@%d vs %q@%d",
				i, lf.Notes[i].Format(), lf.Notes[i].Line, crlf.Notes[i].Format(), crlf.Notes[i].Line)
		}
	}
}

// A removed note leaves its number behind, not its text. The tombstone is an HTML
// comment, so it is invisible to a rendered read and to a grep for a tag — and
// visible to Next, which is the only reader that has to care.
func TestRemoveLeavesATombstone(t *testing.T) {
	f := parse(t, log)
	content, n, err := f.Remove("n-0002")
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if n.Text == "" {
		t.Error("remove returned no note; the caller has no copy of what went")
	}
	if strings.Contains(content, "the log reads oldest first") {
		t.Error("the note's text survived removal")
	}
	if !strings.Contains(content, Tombstone("n-0002")) {
		t.Errorf("no tombstone in:\n%s", content)
	}
	after := parse(t, content)
	if len(after.Notes) != 1 {
		t.Errorf("notes = %d, want 1", len(after.Notes))
	}
	if _, ok := after.Get("n-0002"); ok {
		t.Error("a removed note is still returned by Get")
	}
	if _, was := after.Retired["n-0002"]; !was {
		t.Errorf("retired = %v, want n-0002", after.Retired)
	}
	if _, _, err := after.Remove("n-0002"); err == nil {
		t.Error("removing an already-removed note succeeded")
	}
}
